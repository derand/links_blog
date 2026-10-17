package discord

import (
	"context"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func interaction(user string, opts ...*discordgo.ApplicationCommandInteractionDataOption) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		ID: "5000", Type: discordgo.InteractionApplicationCommand, GuildID: "g1", ChannelID: "300",
		Member: &discordgo.Member{User: &discordgo.User{ID: user}},
		Data:   discordgo.ApplicationCommandInteractionData{Name: "links", Options: opts},
	}}
}

func sub(name string, opts ...*discordgo.ApplicationCommandInteractionDataOption) *discordgo.ApplicationCommandInteractionDataOption {
	return &discordgo.ApplicationCommandInteractionDataOption{
		Name: name, Type: discordgo.ApplicationCommandOptionSubCommand, Options: opts}
}

func TestParseCommand(t *testing.T) {
	group := &discordgo.ApplicationCommandInteractionDataOption{
		Name: "channel", Type: discordgo.ApplicationCommandOptionSubCommandGroup,
		Options: []*discordgo.ApplicationCommandInteractionDataOption{sub("set",
			&discordgo.ApplicationCommandInteractionDataOption{Name: "tag", Type: discordgo.ApplicationCommandOptionString, Value: " go "},
			&discordgo.ApplicationCommandInteractionDataOption{Name: "hidden", Type: discordgo.ApplicationCommandOptionBoolean, Value: true},
		)},
	}
	c := parseCommand(interaction(owner, group))
	if c.Name != "channel set" || c.UserID != owner || c.GuildID != "g1" || c.ChannelID != "300" ||
		c.InteractionID != "5000" || c.Tag == nil || *c.Tag != "go" || c.Hidden == nil || !*c.Hidden {
		t.Errorf("got %+v", c)
	}
	if c := parseCommand(interaction(owner, sub("import"))); c.Name != "import" || c.Tag != nil || c.Hidden != nil {
		t.Errorf("import: %+v", c)
	}
}

func TestChannelCommands(t *testing.T) {
	ctx := context.Background()
	b, st := newTest(t)
	run := func(c command) string {
		t.Helper()
		c.UserID, c.GuildID, c.ChannelID, c.InteractionID = owner, "g1", "300", "5000"
		text, err := b.runChannelCommand(ctx, c)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		return text
	}
	str := func(s string) *string { return &s }
	yes, no := true, false

	if got := run(command{Name: "channel show"}); !strings.Contains(got, "not set up") {
		t.Errorf("show before set: %q", got)
	}

	got := run(command{Name: "channel set", ChannelName: "links", Tag: str("#go"), Hidden: &yes})
	if !strings.Contains(got, "**on**") || !strings.Contains(got, "`go`") || !strings.Contains(got, "hidden") {
		t.Errorf("set: %q", got)
	}
	ch, _ := st.Channel(ctx, platform, "300")
	if ch == nil || !ch.Enabled || ch.DefaultTag != "go" || !ch.Hidden || ch.Name != "links" ||
		ch.GuildID != "g1" || ch.LastMessageID != 5000 {
		t.Fatalf("after set: %+v", ch)
	}

	// Messages arrive, then settings change: options left out are kept.
	if err := st.AdvanceChannel(ctx, platform, "300", 6000); err != nil {
		t.Fatal(err)
	}
	run(command{Name: "channel set", Hidden: &no})
	ch, _ = st.Channel(ctx, platform, "300")
	if ch.DefaultTag != "go" || ch.Hidden || ch.LastMessageID != 6000 || ch.Name != "links" {
		t.Errorf("after second set: %+v", ch)
	}

	run(command{Name: "channel set", Tag: str(clearTag)})
	if ch, _ = st.Channel(ctx, platform, "300"); ch.DefaultTag != "" {
		t.Errorf("tag not cleared: %+v", ch)
	}

	if got := run(command{Name: "channel off"}); !strings.Contains(got, "no longer") {
		t.Errorf("off: %q", got)
	}
	if got := run(command{Name: "channel show"}); !strings.Contains(got, "**off**") {
		t.Errorf("show after off: %q", got)
	}
	if ch, _ = st.Channel(ctx, platform, "300"); ch.Enabled {
		t.Errorf("still enabled")
	}
}

func TestImportChannel(t *testing.T) {
	ctx := context.Background()
	b, _ := newTest(t)
	f := &fakeChannel{msgs: []*discordgo.Message{
		msg("100", "1", owner, "https://a.example"),
		msg("100", "2", owner, "https://b.example https://c.example https://a.example"),
		msg("100", "3", owner, "no links"),
		msg("100", "4", owner, "ftp://bad.example"),
	}}
	st, err := b.importChannel(ctx, "100", f.fetch, func(*discordgo.Message, handled) {})
	if err != nil || st != (importStats{messages: 4, added: 3, duplicates: 1}) {
		t.Errorf("import: %+v, %v", st, err)
	}
}
