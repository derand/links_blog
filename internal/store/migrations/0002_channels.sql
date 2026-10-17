-- Bot channels: where links come from and how they are filed.

CREATE TABLE channels (
    platform        TEXT    NOT NULL,              -- 'discord' | 'telegram' | ...
    channel_id      TEXT    NOT NULL,
    guild_id        TEXT    NOT NULL DEFAULT '',   -- Discord server; '' for DMs and other platforms
    name            TEXT    NOT NULL DEFAULT '',   -- for display only
    default_tag     TEXT    NOT NULL DEFAULT '',   -- added to every link from the channel
    hidden          INTEGER NOT NULL DEFAULT 0 CHECK (hidden IN (0, 1)),
    enabled         INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    last_message_id INTEGER NOT NULL DEFAULT 0,    -- newest processed message; 0 = none yet
    created_at      INTEGER NOT NULL,              -- unix seconds
    updated_at      INTEGER NOT NULL,
    PRIMARY KEY (platform, channel_id)
) WITHOUT ROWID;
