-- Links, tags and full-text search.

CREATE TABLE links (
    id               INTEGER PRIMARY KEY,
    url              TEXT    NOT NULL,
    url_norm         TEXT    NOT NULL UNIQUE,      -- dedup key, see links.Normalize
    description      TEXT    NOT NULL DEFAULT '',  -- author's text
    title            TEXT    NOT NULL DEFAULT '',  -- fetched from the page
    site_description TEXT    NOT NULL DEFAULT '',  -- fetched from the page
    day              TEXT    NOT NULL              -- 'YYYY-MM-DD' in the blog's time zone
                     CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),
    created_at       INTEGER NOT NULL,             -- unix seconds
    updated_at       INTEGER NOT NULL,
    hidden           INTEGER NOT NULL DEFAULT 0 CHECK (hidden IN (0, 1)),
    source           TEXT    NOT NULL DEFAULT 'web', -- 'web' | 'mongo' | 'discord' | ...
    source_ref       TEXT,                           -- e.g. 'channel_id:message_id'
    UNIQUE (source, source_ref)
);

CREATE INDEX links_day ON links (day DESC, created_at);

CREATE TABLE tags (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE link_tags (
    link_id INTEGER NOT NULL REFERENCES links (id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    pos     INTEGER NOT NULL DEFAULT 0,            -- keeps the author's tag order
    PRIMARY KEY (link_id, tag_id)
) WITHOUT ROWID;

CREATE INDEX link_tags_tag ON link_tags (tag_id);

-- Search index; rowid = links.id. Kept in sync by the triggers below.
CREATE VIRTUAL TABLE links_fts USING fts5 (
    description, title, site_description, url, tags,
    tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER links_ai AFTER INSERT ON links BEGIN
    INSERT INTO links_fts (rowid, description, title, site_description, url, tags)
    VALUES (new.id, new.description, new.title, new.site_description, new.url, '');
END;

CREATE TRIGGER links_au AFTER UPDATE OF description, title, site_description, url ON links BEGIN
    UPDATE links_fts
       SET description = new.description, title = new.title,
           site_description = new.site_description, url = new.url
     WHERE rowid = new.id;
END;

CREATE TRIGGER links_ad AFTER DELETE ON links BEGIN
    DELETE FROM links_fts WHERE rowid = old.id;
END;

CREATE TRIGGER link_tags_ai AFTER INSERT ON link_tags BEGIN
    UPDATE links_fts
       SET tags = (SELECT coalesce(group_concat(t.name, ' '), '')
                     FROM link_tags lt JOIN tags t ON t.id = lt.tag_id
                    WHERE lt.link_id = new.link_id)
     WHERE rowid = new.link_id;
END;

CREATE TRIGGER link_tags_ad AFTER DELETE ON link_tags BEGIN
    UPDATE links_fts
       SET tags = (SELECT coalesce(group_concat(t.name, ' '), '')
                     FROM link_tags lt JOIN tags t ON t.id = lt.tag_id
                    WHERE lt.link_id = old.link_id)
     WHERE rowid = old.link_id;
END;
