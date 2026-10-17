package discord

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// fakeChannel serves a channel's history like Discord: up to pageSize messages
// after the given ID, newest first.
type fakeChannel struct {
	msgs     []*discordgo.Message // oldest first
	requests int
}

func (f *fakeChannel) fetch(channelID, afterID string) ([]*discordgo.Message, error) {
	f.requests++
	var page []*discordgo.Message
	for _, m := range f.msgs {
		if compareIDs(m.ID, afterID) > 0 && len(page) < pageSize {
			c := *m
			c.ChannelID = "" // REST pages do not need it; catchUp sets it
			page = append(page, &c)
		}
	}
	slices.Reverse(page)
	return page, nil
}

func TestCatchUp(t *testing.T) {
	ctx := context.Background()
	b, st := newTest(t)

	// 250 messages, every 50th without a link, IDs crossing a digit boundary.
	f := &fakeChannel{}
	for i := 1; i <= 250; i++ {
		text := fmt.Sprintf("https://example.com/%d", i)
		if i%50 == 0 {
			text = "no link"
		}
		f.msgs = append(f.msgs, msg("100", strconv.Itoa(900+i), owner, text))
	}
	reacted := map[string][]string{}
	react := func(m *discordgo.Message, h handled) { reacted[m.ID] = h.reactions() }

	n, err := b.catchUp(ctx, "100", "0", f.fetch, react)
	if err != nil || n != 250 || f.requests != 3 {
		t.Fatalf("import: %d messages, %d requests, %v", n, f.requests, err)
	}
	if c, _ := st.CountLinks(ctx); c != 245 {
		t.Errorf("links = %d, want 245", c)
	}
	if r := reacted["901"]; !slices.Equal(r, []string{reactAdded}) {
		t.Errorf("reactions on the first message = %q", r)
	}
	if ch, _ := st.Channel(ctx, platform, "100"); ch.LastMessageID != 1150 {
		t.Errorf("last message = %d, want 1150", ch.LastMessageID)
	}

	// Two new messages while offline: catchUpAll reads from last_message_id.
	f.msgs = append(f.msgs, msg("100", "1151", owner, "https://new.example"), msg("100", "1152", owner, "https://example.com/1"))
	f.requests = 0
	reacted = map[string][]string{}
	b.catchUpAll(ctx, f.fetch, react)
	if f.requests != 1 || len(reacted) != 2 ||
		!slices.Equal(reacted["1151"], []string{reactAdded}) || !slices.Equal(reacted["1152"], []string{reactDuplicate}) {
		t.Errorf("catch-up: %d requests, reactions %q", f.requests, reacted)
	}

	// Importing again adds nothing.
	n, err = b.catchUp(ctx, "100", "0", f.fetch, func(*discordgo.Message, handled) {})
	if c, _ := st.CountLinks(ctx); err != nil || n != 252 || c != 246 {
		t.Errorf("re-import: %d messages, %d links, %v", n, c, err)
	}
}

func TestCatchUpAllSkipsChannels(t *testing.T) {
	ctx := context.Background()
	b, st := newTest(t)
	// Channel 100 never saw a message and 200 is disabled: nothing is fetched.
	if err := st.AdvanceChannel(ctx, platform, "200", 5); err != nil {
		t.Fatal(err)
	}
	fetched := 0
	b.catchUpAll(ctx, func(string, string) ([]*discordgo.Message, error) {
		fetched++
		return nil, nil
	}, func(*discordgo.Message, handled) {})
	if fetched != 0 {
		t.Errorf("fetched %d times", fetched)
	}
}

func TestCompareIDs(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"999", "1000", -1}, {"1000", "999", 1}, {"123", "124", -1}, {"5", "5", 0}, {"0", "1560954293958479982", -1},
	} {
		if got := compareIDs(tc.a, tc.b); (got > 0) != (tc.want > 0) || (got < 0) != (tc.want < 0) {
			t.Errorf("compareIDs(%s, %s) = %d, want sign of %d", tc.a, tc.b, got, tc.want)
		}
	}
}
