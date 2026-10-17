package store

import (
	"context"
	"time"
)

// Channel is a bot channel whose messages become links.
type Channel struct {
	Platform      string
	ChannelID     string
	GuildID       string
	Name          string
	DefaultTag    string
	Hidden        bool
	Enabled       bool
	LastMessageID int64 // 0 means no message processed yet
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// SaveChannel creates or updates a channel's settings. LastMessageID and
// CreatedAt of an existing channel are kept.
func (s *Store) SaveChannel(ctx context.Context, c *Channel) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO channels (platform, channel_id, guild_id, name, default_tag, hidden, enabled,
		                      last_message_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (platform, channel_id) DO UPDATE SET
		    guild_id = excluded.guild_id, name = excluded.name, default_tag = excluded.default_tag,
		    hidden = excluded.hidden, enabled = excluded.enabled, updated_at = excluded.updated_at`,
		c.Platform, c.ChannelID, c.GuildID, c.Name, c.DefaultTag, c.Hidden, c.Enabled,
		c.LastMessageID, now, now)
	return err
}

// Channel returns the channel, or nil if it was never configured.
func (s *Store) Channel(ctx context.Context, platform, channelID string) (*Channel, error) {
	cs, err := s.queryChannels(ctx, `WHERE platform = ? AND channel_id = ?`, platform, channelID)
	if err != nil || len(cs) == 0 {
		return nil, err
	}
	return cs[0], nil
}

// Channels returns all channels of the platform, disabled included.
func (s *Store) Channels(ctx context.Context, platform string) ([]*Channel, error) {
	return s.queryChannels(ctx, `WHERE platform = ? ORDER BY guild_id, name, channel_id`, platform)
}

// AdvanceChannel records messageID as processed unless a newer one already is,
// so live messages and history catch-up can run in any order.
func (s *Store) AdvanceChannel(ctx context.Context, platform, channelID string, messageID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE channels SET last_message_id = ?
		 WHERE platform = ? AND channel_id = ? AND last_message_id < ?`,
		messageID, platform, channelID, messageID)
	return err
}

func (s *Store) queryChannels(ctx context.Context, tail string, args ...any) ([]*Channel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT platform, channel_id, guild_id, name, default_tag, hidden, enabled,
		       last_message_id, created_at, updated_at
		  FROM channels `+tail, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cs []*Channel
	for rows.Next() {
		var (
			c                Channel
			created, updated int64
		)
		if err := rows.Scan(&c.Platform, &c.ChannelID, &c.GuildID, &c.Name, &c.DefaultTag,
			&c.Hidden, &c.Enabled, &c.LastMessageID, &created, &updated); err != nil {
			return nil, err
		}
		c.CreatedAt = time.Unix(created, 0)
		c.UpdatedAt = time.Unix(updated, 0)
		cs = append(cs, &c)
	}
	return cs, rows.Err()
}
