-- The author's full text when the description is only its beginning (a long
-- bot message). Searched, not shown in the feed.

ALTER TABLE links ADD COLUMN note TEXT NOT NULL DEFAULT '';

-- FTS5 can't add a column: the index and the triggers on links are rebuilt.
-- The triggers on link_tags only set tags and work with the new table as is.
DROP TRIGGER links_ai;
DROP TRIGGER links_au;
DROP TRIGGER links_ad;
DROP TABLE links_fts;

CREATE VIRTUAL TABLE links_fts USING fts5 (
    description, title, site_description, url, tags, note,
    tokenize = 'unicode61 remove_diacritics 2'
);

INSERT INTO links_fts (rowid, description, title, site_description, url, tags, note)
SELECT l.id, l.description, l.title, l.site_description, l.url,
       (SELECT coalesce(group_concat(t.name, ' '), '')
          FROM link_tags lt JOIN tags t ON t.id = lt.tag_id
         WHERE lt.link_id = l.id),
       l.note
  FROM links l;

CREATE TRIGGER links_ai AFTER INSERT ON links BEGIN
    INSERT INTO links_fts (rowid, description, title, site_description, url, tags, note)
    VALUES (new.id, new.description, new.title, new.site_description, new.url, '', new.note);
END;

CREATE TRIGGER links_au AFTER UPDATE OF description, title, site_description, url, note ON links BEGIN
    UPDATE links_fts
       SET description = new.description, title = new.title,
           site_description = new.site_description, url = new.url, note = new.note
     WHERE rowid = new.id;
END;

CREATE TRIGGER links_ad AFTER DELETE ON links BEGIN
    DELETE FROM links_fts WHERE rowid = old.id;
END;
