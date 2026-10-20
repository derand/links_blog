package discord

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/derand/links_blog/internal/service"
	"github.com/derand/links_blog/internal/store"
)

const owner = "42"

func newTest(t *testing.T) (*Bot, *store.Store) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for _, c := range []*store.Channel{
		{Platform: platform, ChannelID: "100", DefaultTag: "news", Hidden: true, Enabled: true},
		{Platform: platform, ChannelID: "200", Enabled: false},
	} {
		if err := st.SaveChannel(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	b, err := New(Config{Token: "x", AllowedUsers: []string{owner}}, service.New(st, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return b, st
}

func msg(channel, id, author, content string, embeds ...*discordgo.MessageEmbed) *discordgo.Message {
	return &discordgo.Message{
		ID: id, ChannelID: channel, Content: content, Embeds: embeds,
		Author:    &discordgo.User{ID: author},
		Timestamp: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC),
	}
}

func TestHandle(t *testing.T) {
	ctx := context.Background()
	b, st := newTest(t)

	got := b.handle(ctx, msg("100", "1001", owner, "Цікаво https://a.example https://b.example #go",
		&discordgo.MessageEmbed{URL: "https://b.example", Title: "B"})).reactions()
	if want := []string{reactAdded}; !reflect.DeepEqual(got, want) {
		t.Errorf("reactions = %q, want %q", got, want)
	}
	ls, err := st.LinksByMessage(ctx, platform, "100:1001")
	if err != nil || len(ls) != 2 {
		t.Fatalf("links: %+v, %v", ls, err)
	}
	a := ls[0]
	if a.URL != "https://a.example" || a.Description != "Цікаво" || !a.Hidden || a.Day != "2026-10-17" ||
		a.SourceRef != "100:1001:0" || a.AddedBy != "discord:"+owner || !reflect.DeepEqual(a.Tags, []string{"news", "go"}) {
		t.Errorf("first link: %+v", a)
	}
	if ls[1].Title != "B" {
		t.Errorf("second link title = %q", ls[1].Title)
	}
	if c, _ := st.Channel(ctx, platform, "100"); c.LastMessageID != 1001 {
		t.Errorf("last message = %d", c.LastMessageID)
	}

	// One new link and one posted before.
	got = b.handle(ctx, msg("100", "1002", owner, "https://c.example https://a.example/")).reactions()
	if want := []string{reactAdded, reactDuplicate}; !reflect.DeepEqual(got, want) {
		t.Errorf("reactions = %q, want %q", got, want)
	}

	// The same message again (e.g. history import): the added link is known by
	// its source ref and not reported; the duplicate is reported again, which
	// Discord treats as the same reaction.
	got = b.handle(ctx, msg("100", "1002", owner, "https://c.example https://a.example/")).reactions()
	if want := []string{reactDuplicate}; !reflect.DeepEqual(got, want) {
		t.Errorf("replay reactions = %q, want %q", got, want)
	}

	// Ignored: other users, bots, unknown and disabled channels, no links.
	bot := msg("100", "1003", owner, "https://d.example")
	bot.Author.Bot = true
	for _, m := range []*discordgo.Message{
		msg("100", "1004", "7", "https://d.example"),
		bot,
		msg("300", "1005", owner, "https://d.example"),
		msg("200", "1006", owner, "https://d.example"),
		msg("100", "1007", owner, "just text"),
	} {
		if got := b.handle(ctx, m).reactions(); got != nil {
			t.Errorf("message %s: reactions = %q", m.ID, got)
		}
	}
	if n, _ := st.CountLinks(ctx); n != 3 {
		t.Errorf("links = %d, want 3", n)
	}
}

func TestHandleForward(t *testing.T) {
	ctx := context.Background()
	b, st := newTest(t)

	// A forward has no content of its own; the text is in the snapshot.
	m := msg("100", "1001", owner, "")
	m.MessageSnapshots = []discordgo.MessageSnapshot{{Message: &discordgo.Message{
		Content: "Тривалість https://a.example #go",
		Embeds:  []*discordgo.MessageEmbed{{URL: "https://a.example", Title: "A"}},
	}}}
	if got, want := b.handle(ctx, m).reactions(), []string{reactAdded}; !reflect.DeepEqual(got, want) {
		t.Errorf("reactions = %q, want %q", got, want)
	}
	ls, _ := st.LinksByMessage(ctx, platform, "100:1001")
	if len(ls) != 1 || ls[0].URL != "https://a.example" || ls[0].Description != "Тривалість" ||
		ls[0].Title != "A" || ls[0].Day != "2026-10-17" || !reflect.DeepEqual(ls[0].Tags, []string{"news", "go"}) {
		t.Errorf("links: %+v", ls)
	}

	// Embeds that come later are in the snapshot too.
	b.handle(ctx, &discordgo.Message{ID: "1002", ChannelID: "100", Author: &discordgo.User{ID: owner},
		MessageSnapshots: []discordgo.MessageSnapshot{{Message: &discordgo.Message{Content: "https://b.example"}}}})
	upd := &discordgo.Message{ID: "1002", ChannelID: "100", MessageSnapshots: []discordgo.MessageSnapshot{{
		Message: &discordgo.Message{Embeds: []*discordgo.MessageEmbed{{URL: "https://b.example", Title: "B"}}},
	}}}
	b.onUpdate(ctx, upd)
	if ls, _ := st.LinksByMessage(ctx, platform, "100:1002"); len(ls) != 1 || ls[0].Title != "B" {
		t.Errorf("forward metadata: %+v", ls)
	}
}

func TestFillMetadata(t *testing.T) {
	ctx := context.Background()
	b, st := newTest(t)

	b.handle(ctx, msg("100", "1001", owner, "https://youtu.be/abc"))
	// Discord adds the embed later, with another URL and without the author.
	upd := &discordgo.Message{ID: "1001", ChannelID: "100", Embeds: []*discordgo.MessageEmbed{
		{URL: "https://www.youtube.com/watch?v=abc", Title: "Video", Description: "About"},
	}}
	if err := b.fillMetadata(ctx, upd); err != nil {
		t.Fatal(err)
	}
	ls, _ := st.LinksByMessage(ctx, platform, "100:1001")
	if len(ls) != 1 || ls[0].Title != "Video" || ls[0].SiteDescription != "About" {
		t.Errorf("links: %+v", ls)
	}
}
