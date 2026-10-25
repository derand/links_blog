package web

import (
	"strings"
	"testing"

	"github.com/derand/links_blog/internal/store"
)

func TestShortText(t *testing.T) {
	long := strings.Repeat("word ", 40)
	tests := []struct {
		in, want string
		cut      bool
	}{
		{"", "", false},
		{"Discord Alternative that lives onchain", "Discord Alternative that lives onchain", false},
		{"\n\nHOW TO USE NANSEN?", "HOW TO USE NANSEN?", false},
		{"Big updates. See https://t.co/abc", "Big updates. See", false},
		{"@ssv_network staking \n\nhttps://t.co/tWVQ2C3YYv", "@ssv_network staking", false},
		{"https://t.co/x\nText after a link", "Text after a link", false},
		{"Here's the list:\n\n1. one\n2. two", "Here's the list…", true},
		{long, strings.TrimSpace(strings.Repeat("word ", 28)) + "…", true},
	}
	for _, tt := range tests {
		got, cut := shortText(tt.in, 140)
		if got != tt.want || cut != tt.cut {
			t.Errorf("shortText(%q) = %q, %v; want %q, %v", tt.in, got, cut, tt.want, tt.cut)
		}
	}
}

func TestPostViewText(t *testing.T) {
	tweet := "https://x.com/i/status/123"
	tests := []struct {
		name                   string
		l                      store.Link
		text, byline, fullText string
	}{
		{"description and author", store.Link{URL: tweet, Description: "Mine", Title: "A (@a)", SiteDescription: "Tweet", SiteAuthor: "a"}, "Mine", "@a", ""},
		{"tweet text", store.Link{URL: tweet, Title: "Ann (@ann_1)", SiteDescription: "First line\n\nmore", SiteAuthor: "ann_1"}, "First line…", "@ann_1", "First line\n\nmore"},
		{"author unknown", store.Link{URL: tweet, Title: "Ann (@ann_1)", SiteDescription: "Tweet"}, "Tweet", "", ""},
		{"tweet without text", store.Link{URL: tweet, Title: "Ann (@ann_1)", SiteAuthor: "ann_1"}, "Ann (@ann_1)", "", ""},
		{"not a tweet", store.Link{URL: "https://example.com", Title: "Page", SiteDescription: "About", SiteAuthor: "someone"}, "Page", "", ""},
		{"reddit post", store.Link{URL: "https://www.reddit.com/r/flask/comments/2321oc/x/", Title: "Host Flask", SiteAuthor: "executex"}, "Host Flask", "u/executex", ""},
		{"tiktok profile", store.Link{URL: "https://www.tiktok.com/@zory", Title: "Zoryana", SiteAuthor: "zory"}, "Zoryana", "@zory", ""},
		{"tiktok name is the nick", store.Link{URL: "https://www.tiktok.com/@flazerd", Title: "flazerd", SiteAuthor: "flazerd"}, "flazerd", "", ""},
		{"subreddit", store.Link{URL: "https://www.reddit.com/r/flask/", Title: "r/flask", SiteAuthor: "someone"}, "r/flask", "", ""},
	}
	for _, tt := range tests {
		v := newPostView(&tt.l, false, false)
		if v.Text != tt.text || v.Byline != tt.byline || v.Full != tt.fullText {
			t.Errorf("%s: text=%q byline=%q full=%q", tt.name, v.Text, v.Byline, v.Full)
		}
	}
}
