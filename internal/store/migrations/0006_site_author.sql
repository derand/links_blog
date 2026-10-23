-- The author of what a link points to, as the site knows them: a tweet's
-- author nick without "@". Not the blog author who added the link (added_by).
-- Filled like title and site_description: by the metadata fetcher where empty,
-- editable by hand.

ALTER TABLE links ADD COLUMN site_author TEXT NOT NULL DEFAULT '';

-- Existing x.com links: the nick from the title the fetcher made, "Name (@nick)",
-- else from the URL, /<nick>/status/<id>. Anything that is not a nick is dropped.
UPDATE links
   SET site_author = rtrim(substr(title, instr(title, ' (@') + 3), ')')
 WHERE url LIKE 'https://x.com/%' AND title LIKE '% (@%)';

UPDATE links
   SET site_author = substr(url, 15, instr(substr(url, 15), '/') - 1)
 WHERE site_author = '' AND url GLOB 'https://x.com/*/status/*'
   AND url NOT GLOB 'https://x.com/i/*';

UPDATE links
   SET site_author = ''
 WHERE site_author GLOB '*[^A-Za-z0-9_]*' OR length(site_author) > 15;
