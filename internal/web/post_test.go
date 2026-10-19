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
		{"description wins", store.Link{URL: tweet, Description: "Mine", Title: "A (@a)", SiteDescription: "Tweet"}, "Mine", "", ""},
		{"tweet text", store.Link{URL: tweet, Title: "Ann (@ann_1)", SiteDescription: "First line\n\nmore"}, "First line…", "@ann_1", "First line\n\nmore"},
		{"tweet without text", store.Link{URL: tweet, Title: "Ann (@ann_1)"}, "Ann (@ann_1)", "", ""},
		{"not a tweet", store.Link{URL: "https://example.com", Title: "Page", SiteDescription: "About"}, "Page", "", ""},
	}
	for _, tt := range tests {
		v := newPostView(&tt.l, false, false)
		if v.Text != tt.text || v.Byline != tt.byline || v.Full != tt.fullText {
			t.Errorf("%s: text=%q byline=%q full=%q", tt.name, v.Text, v.Byline, v.Full)
		}
	}
}
