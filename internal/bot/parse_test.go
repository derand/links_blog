package bot

import (
	"reflect"
	"testing"
)

func TestParseMessage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		text   string
		embeds []Embed
		want   Message
	}{
		{
			name: "url only",
			text: "https://example.com/a",
			want: Message{Links: []Link{{URL: "https://example.com/a"}}},
		},
		{
			name: "text, url and trailing hashtags",
			text: "Гарна стаття про #go і SQLite: https://example.com/a.\n#sqlite #neural_network",
			want: Message{
				Links:       []Link{{URL: "https://example.com/a"}},
				Description: "Гарна стаття про go і SQLite:",
				Tags:        []string{"go", "sqlite", "neural network"},
			},
		},
		{
			name: "several urls, duplicates and suppressed preview",
			text: "<https://a.example/x> https://b.example, http://www.a.example/x/",
			want: Message{Links: []Link{{URL: "https://a.example/x"}, {URL: "https://b.example"}}},
		},
		{
			name: "masked link keeps its text",
			text: "Див. [документацію](https://go.dev/doc/) тут",
			want: Message{Links: []Link{{URL: "https://go.dev/doc/"}}, Description: "Див. документацію тут"},
		},
		{
			name: "parentheses",
			text: "(see https://en.wikipedia.org/wiki/Go_(programming_language))",
			want: Message{
				Links:       []Link{{URL: "https://en.wikipedia.org/wiki/Go_(programming_language)"}},
				Description: "(see)",
			},
		},
		{
			name: "not hashtags",
			text: "https://x.example/#anchor <#123456> # Heading issue#5 #c++ #node.js.",
			want: Message{
				Links:       []Link{{URL: "https://x.example/#anchor"}},
				Description: "# Heading issue#5",
				Tags:        []string{"c++", "node.js"},
			},
		},
		{
			name: "discord markup",
			text: "## Included in **a16z** __newsletter__ ~~old~~ ||spoiler||:\n> *quoted* and _italic_ `code`\n" +
				"<@123> <@&456> <:pepe:789> thanks **https://a.example/x__y**",
			want: Message{
				Links:       []Link{{URL: "https://a.example/x__y"}},
				Description: "Included in a16z newsletter old spoiler:\nquoted and italic code\nthanks",
			},
		},
		{
			name: "not markup",
			text: "snake_case 2 * 3 * 4 \\*not italic\\* #neural_network tag\n* item\n```go\nx := 1\n```",
			want: Message{
				Description: "snake_case 2 * 3 * 4 *not italic* neural network tag\n* item\nx := 1",
				Tags:        []string{"neural network"},
			},
		},
		{
			name: "paragraphs",
			text: "  first  line \n\n\n\nsecond https://a.example\n\n",
			want: Message{Links: []Link{{URL: "https://a.example"}}, Description: "first line\n\nsecond"},
		},
		{
			name: "no urls",
			text: "just text #tag",
			want: Message{Description: "just text", Tags: []string{"tag"}},
		},
		{
			name: "embeds matched by url",
			text: "https://a.example/x https://b.example",
			embeds: []Embed{
				{URL: "https://b.example/", Title: " B ", Description: "about b"},
				{URL: "https://c.example", Title: "C"},
			},
			want: Message{Links: []Link{
				{URL: "https://a.example/x"},
				{URL: "https://b.example", Title: "B", SiteDescription: "about b"},
			}},
		},
		{
			name:   "single embed with a rewritten url",
			text:   "https://youtu.be/abc",
			embeds: []Embed{{URL: "https://www.youtube.com/watch?v=abc", Title: "Video"}},
			want:   Message{Links: []Link{{URL: "https://youtu.be/abc", Title: "Video"}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseMessage(tc.text, tc.embeds); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}
