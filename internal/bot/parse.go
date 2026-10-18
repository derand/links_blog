// Package bot holds what chat bots share: turning a message into links.
package bot

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

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
	Description string   // the text without URLs and trailing hashtags, its first paragraph if long
	Note        string   // the whole text if Description is only its first paragraph
	Tags        []string // from #hashtags, "_" read as a space
}

type Link struct {
	URL             string
	Title           string // from the matching embed, if any
	SiteDescription string
}

// maxDescription is the longest text kept whole as the description; a longer
// one, or one of several paragraphs, is cut to its first paragraph.
const maxDescription = 300

var (
	maskedRe  = regexp.MustCompile(`\[([^\]\n]*)\]\(<?(https?://[^\s)>]+)>?\)`)
	urlRe     = regexp.MustCompile(`<?https?://[^\s<>]+>?`)
	hashtagRe = regexp.MustCompile(`(^|\s)#([\p{L}\p{N}_][\p{L}\p{N}_+.-]*)`)
)

// ParseMessage splits a message into links, a description shared by all of them
// and tags. Embeds fill the links' metadata.
//
// Bare URLs are the links. A URL inside a sentence is replaced by its domain,
// one at the start or the end of a line is removed. Masked links ([text](url))
// keep their text in the description and are links only if there are no bare URLs.
func ParseMessage(text string, embeds []Embed) Message {
	var (
		m      Message
		seen   = map[string]bool{}
		masked []string
	)
	addURL := func(u string) {
		norm, err := links.Normalize(u)
		if err != nil || seen[norm] {
			return
		}
		seen[norm] = true
		m.Links = append(m.Links, Link{URL: u})
	}

	text = maskedRe.ReplaceAllStringFunc(text, func(s string) string {
		sub := maskedRe.FindStringSubmatch(s)
		masked = append(masked, sub[2])
		return sub[1]
	})
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		// A line of only a URL goes away, not to split a paragraph.
		if l := replaceURLs(line, addURL); strings.TrimSpace(l) != "" || strings.TrimSpace(line) == "" {
			lines = append(lines, l)
		}
	}
	if len(m.Links) == 0 {
		for _, u := range masked {
			addURL(u)
		}
	}
	text = stripMarkup(strings.Join(lines, "\n"))

	lines = lines[:0]
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
	if first, _, more := strings.Cut(m.Description, "\n\n"); more || utf8.RuneCountInString(m.Description) > maxDescription {
		m.Description, m.Note = first, m.Description
	}
	urls := make([]string, len(m.Links))
	for i, l := range m.Links {
		urls[i] = l.URL
	}
	for i, e := range MatchEmbeds(urls, embeds) {
		m.Links[i].Title, m.Links[i].SiteDescription = e.Title, e.Description
	}
	return m
}

// replaceURLs passes the bare URLs of a line ("<url>" included, Discord's form
// without a preview) to add and removes them, or, if a URL stands between
// words, replaces each URL with words before it by its domain:
// "two: <url1> and <url2>" → "two: a.example and b.example".
func replaceURLs(line string, add func(string)) string {
	locs := urlRe.FindAllStringIndex(line, -1)
	if locs == nil {
		return line
	}
	var (
		texts = []string{line[:locs[0][0]]} // text around the URLs: before, between, after
		urls  []string
	)
	for i, loc := range locs {
		u, rest := trimURL(strings.TrimSuffix(strings.TrimPrefix(line[loc[0]:loc[1]], "<"), ">"))
		add(u)
		urls = append(urls, u)
		end := len(line)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		texts = append(texts, rest+line[loc[1]:end])
	}
	before := func(i int) bool { return hasWords(strings.Join(texts[:i+1], " ")) }
	after := func(i int) bool { return hasWords(strings.Join(texts[i+1:], " ")) }
	inline := false
	for i := range urls {
		inline = inline || before(i) && after(i)
	}

	var b strings.Builder
	b.WriteString(texts[0])
	for i, u := range urls {
		if inline && before(i) {
			b.WriteString(domain(u))
		}
		b.WriteString(texts[i+1])
	}
	return b.String()
}

// hasWords reports whether s has text other than punctuation and hashtags.
func hasWords(s string) bool {
	return strings.ContainsFunc(hashtagRe.ReplaceAllString(s, ""), func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	})
}

func domain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return rawURL
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
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
