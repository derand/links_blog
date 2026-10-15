package links

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Query is a parsed search query:
//
//	tag:go tag:"neural network" date:2016-09 some "exact phrase"
type Query struct {
	Tags  []string // exact tag names; a link must have all of them
	Dates []string // YYYY, YYYY-MM or YYYY-MM-DD prefixes of Link.Day; any of them
	Terms []string // full-text words and phrases; all must match
}

func (q Query) Empty() bool {
	return len(q.Tags) == 0 && len(q.Dates) == 0 && len(q.Terms) == 0
}

var datePrefix = regexp.MustCompile(`^\d{4}(-\d{2}(-\d{2})?)?$`)

// ParseQuery splits a search string like a shell does: whitespace separates
// words, single or double quotes group them. Words starting with "tag:" or
// "date:" are filters.
func ParseQuery(s string) (Query, error) {
	words, err := splitWords(s)
	if err != nil {
		// An apostrophe in a word ("it's") is not a quote.
		if words, err = splitWords(strings.ReplaceAll(s, "'", "")); err != nil {
			return Query{}, err
		}
	}
	var q Query
	for _, w := range words {
		lower := strings.ToLower(w)
		switch {
		case strings.HasPrefix(lower, "tag:"):
			if t := strings.TrimSpace(w[len("tag:"):]); t != "" {
				q.Tags = append(q.Tags, t)
			}
		case strings.HasPrefix(lower, "date:"):
			d := strings.TrimSpace(w[len("date:"):])
			if !datePrefix.MatchString(d) {
				return Query{}, fmt.Errorf("bad date %q, use YYYY, YYYY-MM or YYYY-MM-DD", d)
			}
			q.Dates = append(q.Dates, d)
		default:
			q.Terms = append(q.Terms, w)
		}
	}
	return q, nil
}

func splitWords(s string) ([]string, error) {
	var (
		words  []string
		cur    strings.Builder
		inWord bool
		quote  rune
	)
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, inWord = r, true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unclosed quote")
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, nil
}
