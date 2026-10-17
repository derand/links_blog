package store

import (
	"context"
	"testing"
)

func TestChannels(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	if c, err := s.Channel(ctx, "discord", "1"); c != nil || err != nil {
		t.Fatalf("missing channel: %+v, %v", c, err)
	}

	c := &Channel{Platform: "discord", ChannelID: "1", GuildID: "g", Name: "links",
		DefaultTag: "go", Hidden: true, Enabled: true}
	if err := s.SaveChannel(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := s.AdvanceChannel(ctx, "discord", "1", 100); err != nil {
		t.Fatal(err)
	}
	// An older message does not move the position back.
	if err := s.AdvanceChannel(ctx, "discord", "1", 50); err != nil {
		t.Fatal(err)
	}

	// Saving settings keeps the position.
	c.DefaultTag, c.Hidden, c.Enabled = "", false, false
	if err := s.SaveChannel(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, err := s.Channel(ctx, "discord", "1")
	if err != nil || got == nil {
		t.Fatalf("channel: %+v, %v", got, err)
	}
	if got.LastMessageID != 100 || got.DefaultTag != "" || got.Hidden || got.Enabled || got.Name != "links" {
		t.Errorf("got %+v", got)
	}

	if err := s.SaveChannel(ctx, &Channel{Platform: "telegram", ChannelID: "1", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	cs, err := s.Channels(ctx, "discord")
	if err != nil || len(cs) != 1 || cs[0].ChannelID != "1" {
		t.Errorf("discord channels: %+v, %v", cs, err)
	}
}
