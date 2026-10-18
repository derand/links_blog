// Package bot holds what chat bots share: turning a message into links.
package bot

import (
	"regexp"
	"strings"

	"github.com/derand/links_blog/internal/links"
)

// Embed is a link preview the chat platform attached to a message.
type Embed struct {
	URL         string
	Title       string
	Description string
}

// Message is the parsed text of a chat message.
type Message struct {
	Links       []Link
	Description string   // the text without URLs and trailing hashtags
	Tags        []string // from #hashtags, "_" read as a space
}

type Link struct {
	URL             string
	Title           string // from the matching embed, if any
	SiteDescription string
}

var (
	maskedRe  = regexp.MustCompile(`\[([^\]\n]*)\]\(<?(https?://[^\s)>]+)>?\)`)
	urlRe     = regexp.MustCompile(`<?https?://[^\s<>]+>?`)
	hashtagRe = regexp.MustCompile(`(^|\s)#([\p{L}\p{N}_][\p{L}\p{N}_+.-]*)`)
)

// ParseMessage splits a message into links, a description shared by all of them
// and tags. Embeds fill the links' metadata.
func ParseMessage(text string, embeds []Embed) Message {
	var (
		m    Message
		seen = map[string]bool{}
	)
	addURL := func(u string) {
		norm, err := links.Normalize(u)
		if err != nil || seen[norm] {
			return
		}
		seen[norm] = true
		m.Links = append(m.Links, Link{URL: u})
	}

	// [text](url): the text stays in the description.
	text = maskedRe.ReplaceAllStringFunc(text, func(s string) string {
		sub := maskedRe.FindStringSubmatch(s)
		addURL(sub[2])
		return sub[1]
	})
	// Bare URLs, <url> included (Discord's form without a preview), are removed.
	text = urlRe.ReplaceAllStringFunc(text, func(s string) string {
		u, rest := trimURL(strings.TrimSuffix(strings.TrimPrefix(s, "<"), ">"))
		addURL(u)
		return rest
	})
	text = stripMarkup(text)

	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = hashtagRe.ReplaceAllStringFunc(line, func(s string) string {
			sub := hashtagRe.FindStringSubmatch(s)
			tag := strings.TrimRight(sub[2], ".-")
			m.Tags = append(m.Tags, strings.ReplaceAll(tag, "_", " "))
			return sub[1] + "#" + sub[2]
		})
		lines = append(lines, cleanLine(line))
	}
	m.Description = joinLines(lines)
	urls := make([]string, len(m.Links))
	for i, l := range m.Links {
		urls[i] = l.URL
	}
	for i, e := range MatchEmbeds(urls, embeds) {
		m.Links[i].Title, m.Links[i].SiteDescription = e.Title, e.Description
	}
	return m
}

// trimURL drops punctuation that ends the sentence rather than the URL and
// returns it separately. A ")" stays if the URL has a matching "(".
func trimURL(u string) (string, string) {
	end := len(u)
	for end > 0 {
		c := u[end-1]
		if strings.IndexByte(".,;:!?'\"*_~", c) >= 0 ||
			c == ')' && strings.Count(u[:end], "(") < strings.Count(u[:end], ")") {
			end--
			continue
		}
		break
	}
	return u[:end], u[end:]
}

// cleanLine removes hashtags at the end of the line, the "#" of the others and
// punctuation or markup left alone by removed URLs ("**<url>**"), and collapses spaces.
func cleanLine(line string) string {
	var words []string
	for _, w := range strings.Fields(line) {
		if strings.Trim(w, ".,;:!?") != "" && (len(w) < 2 || strings.Trim(w, "*_~|") != "") {
			words = append(words, w)
		}
	}
	for len(words) > 0 && hashtagRe.MatchString(words[len(words)-1]) {
		words = words[:len(words)-1]
	}
	for i, w := range words {
		if hashtagRe.MatchString(w) {
			words[i] = strings.ReplaceAll(w[1:], "_", " ")
		}
	}
	s := strings.Join(words, " ")
	return strings.ReplaceAll(strings.ReplaceAll(s, "( ", "("), " )", ")")
}

// joinLines joins non-empty lines, keeping at most one empty line between paragraphs.
func joinLines(lines []string) string {
	var out []string
	for _, l := range lines {
		if l == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, l)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// MatchEmbeds returns the embed for each URL: the one with the same normalized
// URL, or the only embed for the only URL, since platforms may rewrite it
// (youtu.be → youtube.com). URLs without an embed get a zero Embed.
func MatchEmbeds(urls []string, embeds []Embed) []Embed {
	byNorm := map[string]Embed{}
	for _, e := range embeds {
		if norm, err := links.Normalize(e.URL); err == nil {
			if _, ok := byNorm[norm]; !ok {
				byNorm[norm] = e
			}
		}
	}
	out := make([]Embed, len(urls))
	for i, u := range urls {
		norm, _ := links.Normalize(u)
		e, ok := byNorm[norm]
		if !ok && len(urls) == 1 && len(embeds) == 1 {
			e = embeds[0]
		}
		e.Title = strings.TrimSpace(e.Title)
		e.Description = strings.TrimSpace(e.Description)
		out[i] = e
	}
	return out
}
