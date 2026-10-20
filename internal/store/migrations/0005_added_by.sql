-- Who added a link: '<provider>:<user id>', e.g. 'google:<sub>' for the web
-- form or 'discord:<user id>' for the bot. NULL when unknown (imported links).

ALTER TABLE links ADD COLUMN added_by TEXT;
