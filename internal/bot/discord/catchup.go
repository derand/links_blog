package discord

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"
)

// pageSize is the most messages Discord returns per request.
const pageSize = 100

// fetchFunc returns up to pageSize messages of a channel newer than afterID,
// in any order.
type fetchFunc func(channelID, afterID string) ([]*discordgo.Message, error)

// reactFunc reports a handled message, e.g. with reactions.
type reactFunc func(m *discordgo.Message, h handled)

// catchUpAll handles the messages each enabled channel received while the bot
// was offline. Channels that never saw a message are skipped: their history is
// imported only on request.
func (b *Bot) catchUpAll(ctx context.Context, fetch fetchFunc, react reactFunc) {
	chs, err := b.links.Channels(ctx, platform)
	if err != nil {
		b.log.Error("catch-up: channels", "err", err)
		return
	}
	for _, ch := range chs {
		if !ch.Enabled || ch.LastMessageID == 0 {
			continue
		}
		after := strconv.FormatInt(ch.LastMessageID, 10)
		n, err := b.catchUp(ctx, ch.ChannelID, after, fetch, react)
		if err != nil {
			b.log.Error("catch-up", "channel", ch.ChannelID, "name", ch.Name, "handled", n, "err", err)
			continue
		}
		if n > 0 {
			b.log.Info("catch-up done", "channel", ch.ChannelID, "name", ch.Name, "messages", n)
		}
	}
}

// catchUp handles the channel's messages after afterID ("0" for the whole
// history), oldest first, and returns how many it read. Messages handled
// before are recognized by their source ref, so running it twice is safe.
func (b *Bot) catchUp(ctx context.Context, channelID, afterID string, fetch fetchFunc, react reactFunc) (int, error) {
	n := 0
	for {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		page, err := fetch(channelID, afterID)
		if err != nil {
			return n, err
		}
		slices.SortFunc(page, func(a, b *discordgo.Message) int { return compareIDs(a.ID, b.ID) })
		start := afterID
		for _, m := range page {
			if compareIDs(m.ID, afterID) <= 0 {
				continue
			}
			m.ChannelID = channelID
			hctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			h := b.handle(hctx, m)
			cancel()
			react(m, h)
			afterID = m.ID
			n++
		}
		if len(page) < pageSize || afterID == start {
			return n, nil
		}
	}
}

// compareIDs orders Discord snowflakes, which are decimal strings.
func compareIDs(a, b string) int {
	if len(a) != len(b) {
		return len(a) - len(b)
	}
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
