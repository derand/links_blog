package bot

import (
	"regexp"
	"strings"
)

var (
	escapeRe  = regexp.MustCompile("\\\\([*_~|`>#\\\\-])")
	fenceRe   = regexp.MustCompile("```(?:[\\w+-]*\\n)?")
	codeRe    = regexp.MustCompile("`([^`\\n]+)`")
	mentionRe = regexp.MustCompile(`<(?:@[!&]?|#)\d+>|<a?:\w+:\d+>`)
	prefixRe  = regexp.MustCompile(`(?m)^(?:#{1,3} +|-# +|>{1,3} )`)
	// Delimiters that Discord renders wherever they come in pairs.
	pairRes = []*regexp.Regexp{
		regexp.MustCompile(`(?s)\*\*(.+?)\*\*`),
		regexp.MustCompile(`(?s)__(.+?)__`),
		regexp.MustCompile(`(?s)~~(.+?)~~`),
		regexp.MustCompile(`(?s)\|\|(.+?)\|\|`),
	}
	// Italics only at word boundaries: snake_case and 2 * 3 stay as they are.
	italicRes = []*regexp.Regexp{
		regexp.MustCompile(`(^|[^\p{L}\p{N}*])\*([^\s*](?:[^*\n]*[^\s*])?)\*($|[^\p{L}\p{N}*])`),
		regexp.MustCompile(`(^|[^\p{L}\p{N}_])_([^\s_](?:[^_\n]*[^\s_])?)_($|[^\p{L}\p{N}_])`),
	}
)

// stripMarkup removes Discord markdown, mentions and custom emoji, keeping the
// text they wrap. Escaped characters ("\*") are kept as plain ones.
func stripMarkup(s string) string {
	// Escaped characters are hidden from the rules below in the private use area.
	s = escapeRe.ReplaceAllStringFunc(s, func(m string) string {
		return string(rune(0xE000) + rune(m[1]))
	})
	s = fenceRe.ReplaceAllString(s, "")
	s = codeRe.ReplaceAllString(s, "$1")
	s = mentionRe.ReplaceAllString(s, "")
	s = prefixRe.ReplaceAllString(s, "")
	for _, re := range pairRes {
		s = re.ReplaceAllString(s, "$1")
	}
	for _, re := range italicRes {
		// The boundaries are part of the match, so "*a* *b*" takes two passes.
		for prev := ""; prev != s; {
			prev, s = s, re.ReplaceAllString(s, "$1$2$3")
		}
	}
	return strings.Map(func(r rune) rune {
		if r > 0xE000 && r < 0xE080 {
			return r - 0xE000
		}
		return r
	}, s)
}
