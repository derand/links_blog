package bot

import (
	"reflect"
	"strings"
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
			name: "paragraphs: the first is the description, all are the note",
			text: "  first  line \n\n\n\nsecond https://a.example\n\n",
			want: Message{
				Links:       []Link{{URL: "https://a.example"}},
				Description: "first line",
				Note:        "first line\n\nsecond",
			},
		},
		{
			name: "a line of only a url does not split a paragraph",
			text: "5 правил:\n1. Відповідальність\nhttps://a.example\nІ коментар до відео:",
			want: Message{
				Links:       []Link{{URL: "https://a.example"}},
				Description: "5 правил:\n1. Відповідальність\nІ коментар до відео:",
			},
		},
		{
			name: "long text",
			text: strings.Repeat("слово ", 60) + "https://a.example",
			want: Message{
				Links:       []Link{{URL: "https://a.example"}},
				Description: strings.TrimSpace(strings.Repeat("слово ", 60)),
				Note:        strings.TrimSpace(strings.Repeat("слово ", 60)),
			},
		},
		{
			name: "urls inside a sentence become domains",
			text: "Дві штуки: https://www.sqlite.org/fts5.html і <https://pkg.go.dev/x>, обидві #go\n" +
				"знайшов сайт https://t-j.ru/guide/duration/ там приклад\n" +
				"https://a.example/start за репост дякую)\nв кінці https://b.example/end.",
			want: Message{
				Links: []Link{
					{URL: "https://www.sqlite.org/fts5.html"}, {URL: "https://pkg.go.dev/x"},
					{URL: "https://t-j.ru/guide/duration/"}, {URL: "https://a.example/start"}, {URL: "https://b.example/end"},
				},
				Description: "Дві штуки: sqlite.org і pkg.go.dev, обидві\nзнайшов сайт t-j.ru там приклад\n" +
					"за репост дякую)\nв кінці",
				Tags: []string{"go"},
			},
		},
		{
			name: "masked links are text when there are bare urls",
			text: "She introduces [Soul Protocol](https://x.com/0xSoulProtocol), a layer.\nhttps://x.com/stacy_muur/status/1",
			want: Message{
				Links:       []Link{{URL: "https://x.com/stacy_muur/status/1"}},
				Description: "She introduces Soul Protocol, a layer.",
			},
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

func TestMatchEmbedsSkipsTweets(t *testing.T) {
	got := MatchEmbeds([]string{"https://fixupx.com/a/status/1"},
		[]Embed{{URL: "https://fixupx.com/a/status/1", Title: "FixupX", Description: "Sorry, that post doesn't exist :("}})
	if got[0] != (Embed{}) {
		t.Errorf("got %+v, want no metadata for a tweet", got[0])
	}
}
