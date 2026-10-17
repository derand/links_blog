package discord

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/derand/links_blog/internal/store"
)

// clearTag is the tag option value that removes the channel's tag.
const clearTag = "-"

var (
	manageGuild int64 = discordgo.PermissionManageGuild

	// commands are registered globally; only server managers see them, and
	// only the allowed users may run them.
	commands = []*discordgo.ApplicationCommand{{
		Name:                     "links",
		Description:              "Links blog",
		DefaultMemberPermissions: &manageGuild,
		Contexts:                 &[]discordgo.InteractionContextType{discordgo.InteractionContextGuild},
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "channel",
				Description: "Links from this channel",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "set",
						Description: "Add links from this channel to the blog",
						Options: []*discordgo.ApplicationCommandOption{
							{
								Type:        discordgo.ApplicationCommandOptionString,
								Name:        "tag",
								Description: `Tag for every link from the channel; "-" removes it`,
								MaxLength:   100,
							},
							{
								Type:        discordgo.ApplicationCommandOptionBoolean,
								Name:        "hidden",
								Description: "Links are visible only to the author",
							},
						},
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "show",
						Description: "Show the channel's settings",
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "off",
						Description: "Stop adding links from this channel",
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "import",
				Description: "Add links from the channel's whole history",
			},
		},
	}}
)

// command is a parsed /links invocation.
type command struct {
	Name          string // "channel set", "channel show", "channel off", "import"
	UserID        string
	GuildID       string
	ChannelID     string
	ChannelName   string
	InteractionID string // a snowflake, i.e. the time of the command
	Tag           *string
	Hidden        *bool
}

func parseCommand(i *discordgo.InteractionCreate) command {
	c := command{GuildID: i.GuildID, ChannelID: i.ChannelID, InteractionID: i.ID}
	if i.Member != nil && i.Member.User != nil {
		c.UserID = i.Member.User.ID
	} else if i.User != nil {
		c.UserID = i.User.ID
	}
	opts := i.ApplicationCommandData().Options
	var path []string
	for len(opts) == 1 && (opts[0].Type == discordgo.ApplicationCommandOptionSubCommandGroup ||
		opts[0].Type == discordgo.ApplicationCommandOptionSubCommand) {
		path = append(path, opts[0].Name)
		opts = opts[0].Options
	}
	c.Name = strings.Join(path, " ")
	for _, o := range opts {
		switch o.Name {
		case "tag":
			v := strings.TrimSpace(o.StringValue())
			c.Tag = &v
		case "hidden":
			v := o.BoolValue()
			c.Hidden = &v
		}
	}
	return c
}

// runChannelCommand handles the "channel" commands and returns the reply.
func (b *Bot) runChannelCommand(ctx context.Context, c command) (string, error) {
	ch, err := b.links.Channel(ctx, platform, c.ChannelID)
	if err != nil {
		return "", err
	}
	switch c.Name {
	case "channel show":
		if ch == nil {
			return "This channel is not set up. Use `/links channel set`.", nil
		}
		return describe(ch), nil

	case "channel off":
		if ch == nil || !ch.Enabled {
			return "Links from this channel are not added.", nil
		}
		ch.Enabled = false
		if err := b.links.SaveChannel(ctx, ch); err != nil {
			return "", err
		}
		return "Links from this channel are no longer added.", nil

	case "channel set":
		if ch == nil {
			ch = &store.Channel{Platform: platform, ChannelID: c.ChannelID}
			// Messages after the command are caught up even if the bot is
			// offline before the first one; older ones only by /links import.
			ch.LastMessageID, _ = strconv.ParseInt(c.InteractionID, 10, 64)
		}
		ch.GuildID = c.GuildID
		if c.ChannelName != "" {
			ch.Name = c.ChannelName
		}
		if c.Tag != nil {
			ch.DefaultTag = strings.TrimPrefix(*c.Tag, "#")
			if *c.Tag == clearTag {
				ch.DefaultTag = ""
			}
		}
		if c.Hidden != nil {
			ch.Hidden = *c.Hidden
		}
		ch.Enabled = true
		if err := b.links.SaveChannel(ctx, ch); err != nil {
			return "", err
		}
		return "Saved.\n" + describe(ch), nil
	}
	return "", fmt.Errorf("unknown command %q", c.Name)
}

func describe(ch *store.Channel) string {
	state := "on"
	if !ch.Enabled {
		state = "off"
	}
	tag := "none"
	if ch.DefaultTag != "" {
		tag = "`" + ch.DefaultTag + "`"
	}
	visibility := "public"
	if ch.Hidden {
		visibility = "hidden"
	}
	return fmt.Sprintf("Links from this channel: **%s**\nTag: %s\nVisibility: %s", state, tag, visibility)
}

// importStats counts what an import did.
type importStats struct {
	messages, added, duplicates, failed int
}

func (s importStats) String() string {
	return fmt.Sprintf("Messages read: %d. Links added: %d, duplicates: %d, errors: %d.",
		s.messages, s.added, s.duplicates, s.failed)
}

// importChannel handles the channel's whole history.
func (b *Bot) importChannel(ctx context.Context, channelID string, fetch fetchFunc, react reactFunc) (importStats, error) {
	var st importStats
	counting := func(m *discordgo.Message, h handled) {
		st.added += h.added
		st.duplicates += h.duplicates
		st.failed += h.failed
		react(m, h)
	}
	n, err := b.catchUp(ctx, channelID, "0", fetch, counting)
	st.messages = n
	return st, err
}

// interactions answers /links commands.
type interactions struct {
	b     *Bot
	s     *discordgo.Session
	fetch fetchFunc
	react reactFunc

	importing sync.Map // channel ID → struct{}, imports in progress
}

func (h *interactions) handle(ctx context.Context, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "links" {
		return
	}
	c := parseCommand(i)
	log := h.b.log.With("command", c.Name, "user", c.UserID, "channel", c.ChannelID)
	if !slices.Contains(h.b.cfg.AllowedUsers, c.UserID) {
		log.Warn("command refused: user is not allowed")
		h.reply(i, "You are not allowed to use this command.")
		return
	}
	log.Info("command")

	if c.Name == "import" {
		h.startImport(ctx, i, c)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if c.Name == "channel set" {
		if ch, err := h.s.Channel(c.ChannelID, discordgo.WithContext(ctx)); err == nil {
			c.ChannelName = ch.Name
		}
	}
	text, err := h.b.runChannelCommand(ctx, c)
	if err != nil {
		log.Error("command failed", "err", err)
		text = "Error: " + err.Error()
	}
	h.reply(i, text)
}

func (h *interactions) startImport(ctx context.Context, i *discordgo.InteractionCreate, c command) {
	ch, err := h.b.links.Channel(ctx, platform, c.ChannelID)
	switch {
	case err != nil:
		h.reply(i, "Error: "+err.Error())
		return
	case ch == nil || !ch.Enabled:
		h.reply(i, "Turn the channel on first: `/links channel set`.")
		return
	}
	if _, busy := h.importing.LoadOrStore(c.ChannelID, struct{}{}); busy {
		h.reply(i, "An import of this channel is already running.")
		return
	}
	// The import may take longer than the 3 seconds Discord waits for a reply.
	err = h.s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})
	if err != nil {
		h.importing.Delete(c.ChannelID)
		h.b.log.Error("import: respond", "err", err)
		return
	}
	go func() {
		defer h.importing.Delete(c.ChannelID)
		st, err := h.b.importChannel(ctx, c.ChannelID, h.fetch, h.react)
		text := "Import done. " + st.String()
		if err != nil {
			text = "Import stopped: " + err.Error() + "\n" + st.String()
		}
		h.b.log.Info("import", "channel", c.ChannelID, "messages", st.messages, "added", st.added,
			"duplicates", st.duplicates, "errors", st.failed, "err", err)
		// The interaction token lasts 15 minutes; a longer import is only logged.
		if _, err := h.s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &text}); err != nil {
			h.b.log.Warn("import: reply", "err", err)
		}
	}()
}

// reply answers with a message only the user sees.
func (h *interactions) reply(i *discordgo.InteractionCreate, text string) {
	err := h.s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: text, Flags: discordgo.MessageFlagsEphemeral},
	})
	if err != nil {
		h.b.log.Error("reply", "err", err)
	}
}
