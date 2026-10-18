// Package discord turns messages in configured Discord channels into blog links.
package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/derand/links_blog/internal/bot"
	"github.com/derand/links_blog/internal/service"
	"github.com/derand/links_blog/internal/store"
)

const (
	platform = "discord"

	reactAdded     = "✅"
	reactDuplicate = "🔁"
	reactError     = "❌"
)

type Config struct {
	Token        string
	AllowedUsers []string // Discord user IDs whose messages are accepted
}

type Bot struct {
	cfg   Config
	links service.LinkService
	log   *slog.Logger

	catchingUp sync.Mutex // one catch-up at a time if Discord reconnects during one
}

func New(cfg Config, links service.LinkService) (*Bot, error) {
	if cfg.Token == "" {
		return nil, errors.New("discord: no token")
	}
	if len(cfg.AllowedUsers) == 0 {
		return nil, errors.New("discord: no allowed users")
	}
	return &Bot{cfg: cfg, links: links, log: slog.With("bot", platform)}, nil
}

// Run connects to Discord and handles messages until ctx is done.
func (b *Bot) Run(ctx context.Context) error {
	s, err := discordgo.New("Bot " + b.cfg.Token)
	if err != nil {
		return err
	}
	s.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentMessageContent
	react := func(m *discordgo.Message, h handled) {
		for _, r := range h.reactions() {
			if err := s.MessageReactionAdd(m.ChannelID, m.ID, r); err != nil {
				b.log.Warn("reaction", "channel", m.ChannelID, "message", m.ID, "err", err)
			}
		}
	}
	fetch := func(channelID, afterID string) ([]*discordgo.Message, error) {
		return s.ChannelMessages(channelID, pageSize, "", afterID, "", discordgo.WithContext(ctx))
	}
	ih := &interactions{b: b, s: s, fetch: fetch, react: react}
	// Ready comes on every new session, after downtime or a lost connection;
	// a resumed session gets the missed events from Discord itself.
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		b.log.Info("connected", "user", r.User.Username, "guilds", len(r.Guilds))
		if _, err := s.ApplicationCommandBulkOverwrite(r.Application.ID, "", commands, discordgo.WithContext(ctx)); err != nil {
			b.log.Error("register commands", "err", err)
		}
		go func() {
			if !b.catchingUp.TryLock() {
				return
			}
			defer b.catchingUp.Unlock()
			b.catchUpAll(ctx, fetch, react)
		}()
	})
	s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		react(m.Message, b.handle(ctx, m.Message))
	})
	s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageUpdate) {
		b.onUpdate(ctx, m.Message)
	})
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		ih.handle(ctx, i)
	})
	if err := s.Open(); err != nil {
		return fmt.Errorf("discord: %w", err)
	}
	<-ctx.Done()
	return s.Close()
}

// handled counts the links of a message by outcome. Links of a message that
// was handled before are not counted.
type handled struct {
	added, duplicates, failed int
}

// reactions are the reactions to put on the message.
func (h handled) reactions() []string {
	var rs []string
	if h.added > 0 {
		rs = append(rs, reactAdded)
	}
	if h.duplicates > 0 {
		rs = append(rs, reactDuplicate)
	}
	if h.failed > 0 {
		rs = append(rs, reactError)
	}
	return rs
}

// handle adds the links of a new message.
func (b *Bot) handle(ctx context.Context, m *discordgo.Message) handled {
	var h handled
	if m.Author == nil || m.Author.Bot || !slices.Contains(b.cfg.AllowedUsers, m.Author.ID) {
		return h
	}
	ch, err := b.links.Channel(ctx, platform, m.ChannelID)
	if err != nil {
		b.log.Error("channel", "channel", m.ChannelID, "err", err)
		h.failed++
		return h
	}
	if ch == nil || !ch.Enabled {
		b.log.Info("message ignored: channel is not enabled", "guild", m.GuildID, "channel", m.ChannelID)
		return h
	}

	parsed := bot.ParseMessage(content(m), embeds(m))
	var tags []string
	if ch.DefaultTag != "" {
		tags = append(tags, ch.DefaultTag)
	}
	tags = append(tags, parsed.Tags...)

	for i, l := range parsed.Links {
		res, err := b.links.AddLink(ctx, service.NewLink{
			URL:             l.URL,
			Description:     parsed.Description,
			Note:            parsed.Note,
			Title:           l.Title,
			SiteDescription: l.SiteDescription,
			Tags:            tags,
			Hidden:          ch.Hidden,
			At:              m.Timestamp,
			Source:          platform,
			SourceRef:       fmt.Sprintf("%s:%d", messageRef(m), i),
		})
		switch {
		case err != nil:
			b.log.Error("add link", "url", l.URL, "message", m.ID, "err", err)
			h.failed++
		case res.Duplicate == store.DupURL:
			b.log.Info("duplicate", "url", l.URL, "of", res.Link.ID, "day", res.Link.Day)
			h.duplicates++
		case res.Duplicate == store.NotDup:
			b.log.Info("added", "url", l.URL, "id", res.Link.ID, "day", res.Link.Day)
			h.added++
		}
	}

	if id, err := strconv.ParseInt(m.ID, 10, 64); err == nil {
		if err := b.links.AdvanceChannel(ctx, platform, m.ChannelID, id); err != nil {
			b.log.Error("advance channel", "channel", m.ChannelID, "err", err)
		}
	}
	return h
}

// onUpdate fills empty metadata from embeds that Discord attaches after the
// message was created. Edits of the text are ignored.
func (b *Bot) onUpdate(ctx context.Context, m *discordgo.Message) {
	if len(embeds(m)) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := b.fillMetadata(ctx, m); err != nil {
		b.log.Error("fill metadata", "message", m.ID, "err", err)
	}
}

func (b *Bot) fillMetadata(ctx context.Context, m *discordgo.Message) error {
	ls, err := b.links.MessageLinks(ctx, platform, messageRef(m))
	if err != nil || len(ls) == 0 {
		return err
	}
	urls := make([]string, len(ls))
	for i, l := range ls {
		urls[i] = l.URL
	}
	for i, e := range bot.MatchEmbeds(urls, embeds(m)) {
		ok, err := b.links.FillMetadata(ctx, ls[i].ID, e.Title, e.Description)
		if err != nil {
			return err
		}
		if ok {
			b.log.Info("metadata filled", "id", ls[i].ID, "title", e.Title)
		}
	}
	return nil
}

func messageRef(m *discordgo.Message) string {
	return m.ChannelID + ":" + m.ID
}

// messages returns m and the messages forwarded in it. A forward has no text
// of its own: the text and embeds are in the snapshot.
func messages(m *discordgo.Message) []*discordgo.Message {
	ms := []*discordgo.Message{m}
	for _, s := range m.MessageSnapshots {
		if s.Message != nil {
			ms = append(ms, s.Message)
		}
	}
	return ms
}

func content(m *discordgo.Message) string {
	var parts []string
	for _, m := range messages(m) {
		if m.Content != "" {
			parts = append(parts, m.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}

func embeds(m *discordgo.Message) []bot.Embed {
	var out []bot.Embed
	for _, m := range messages(m) {
		for _, e := range m.Embeds {
			if e != nil && e.URL != "" {
				out = append(out, bot.Embed{URL: e.URL, Title: e.Title, Description: e.Description})
			}
		}
	}
	return out
}
