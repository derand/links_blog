-- State of fetching page metadata (title, site_description) for each link.
-- A link waits for a fetch while one of the two is empty and it was never
-- checked, or its last checks failed for a reason worth a retry.

ALTER TABLE links ADD COLUMN meta_status     TEXT    NOT NULL DEFAULT ''  -- see store.MetaStatus
    CHECK (meta_status IN ('', 'ok', 'blocked', 'dead', 'error'));
ALTER TABLE links ADD COLUMN meta_checked_at INTEGER NOT NULL DEFAULT 0;  -- unix seconds, 0 = never
ALTER TABLE links ADD COLUMN meta_error      TEXT    NOT NULL DEFAULT ''; -- why the last check failed
ALTER TABLE links ADD COLUMN meta_failures   INTEGER NOT NULL DEFAULT 0;  -- failed checks in a row
