package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Link struct {
	ID              int64
	URL             string
	URLNorm         string
	Description     string
	Note            string // the author's full text if Description is its beginning
	Title           string
	SiteDescription string
	SiteAuthor      string // who wrote what the link points to: the nick of a tweet's, Reddit or TikTok post's author, without "@" or "u/"
	Day             string // YYYY-MM-DD
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Hidden          bool
	Source          string
	SourceRef       string // empty means none
	AddedBy         string // "<provider>:<user id>", e.g. "google:<sub>"; empty means unknown
	Tags            []string

	// Fetching page metadata, see NextForMetadata.
	MetaStatus    MetaStatus
	MetaCheckedAt time.Time // zero: never checked
	MetaError     string    // why the last check failed
	MetaFailures  int       // failed checks in a row
}

// MetaStatus is the outcome of the last metadata check of a link.
type MetaStatus string

const (
	MetaPending MetaStatus = ""        // not checked yet
	MetaOK      MetaStatus = "ok"      // fetched, maybe with nothing to take
	MetaBlocked MetaStatus = "blocked" // a bot check or login wall instead of the page: not retried
	MetaDead    MetaStatus = "dead"    // gone: 404, unknown host, broken certificate; not retried
	MetaError   MetaStatus = "error"   // failed for a reason worth a retry
)

// InsertLinks inserts links with their tags in a single transaction and sets their IDs.
func (s *Store) InsertLinks(ctx context.Context, links []*Link) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, l := range links {
		if err := insertLink(ctx, tx, l); err != nil {
			return fmt.Errorf("insert %q: %w", l.URL, err)
		}
	}
	return tx.Commit()
}

func insertLink(ctx context.Context, tx *sql.Tx, l *Link) error {
	if l.Source == "" {
		l.Source = "web"
	}
	if l.UpdatedAt.IsZero() {
		l.UpdatedAt = l.CreatedAt
	}
	var sourceRef any
	if l.SourceRef != "" {
		sourceRef = l.SourceRef
	}
	var addedBy any
	if l.AddedBy != "" {
		addedBy = l.AddedBy
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO links (url, url_norm, description, note, title, site_description, site_author,
		                   day, created_at, updated_at, hidden, source, source_ref, added_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.URL, l.URLNorm, l.Description, l.Note, l.Title, l.SiteDescription, l.SiteAuthor,
		l.Day, l.CreatedAt.Unix(), l.UpdatedAt.Unix(), l.Hidden, l.Source, sourceRef, addedBy)
	if err != nil {
		return err
	}
	if l.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	return insertTags(ctx, tx, l.ID, l.Tags)
}

func insertTags(ctx context.Context, tx *sql.Tx, linkID int64, tags []string) error {
	for i, name := range tags {
		var tagID int64
		err := tx.QueryRowContext(ctx, `
			INSERT INTO tags (name) VALUES (?)
			ON CONFLICT (name) DO UPDATE SET name = excluded.name
			RETURNING id`, name).Scan(&tagID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO link_tags (link_id, tag_id, pos) VALUES (?, ?, ?)`,
			linkID, tagID, i); err != nil {
			return err
		}
	}
	return nil
}

// CountLinks returns the total number of links, hidden included.
func (s *Store) CountLinks(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM links`).Scan(&n)
	return n, err
}

// LinkByURLNorm returns the link with the given dedup key, or nil if there is none.
func (s *Store) LinkByURLNorm(ctx context.Context, norm string) (*Link, error) {
	ls, err := s.queryLinks(ctx, `WHERE l.url_norm = ?`, norm)
	if err != nil || len(ls) == 0 {
		return nil, err
	}
	return ls[0], nil
}

// Dup tells which key matched an existing link in AddLink.
type Dup int

const (
	NotDup       Dup = iota
	DupSourceRef     // same source and source_ref: the message was already processed
	DupURL           // same url_norm: the link was posted before
)

// AddLink inserts l unless a link with the same source reference or URL already
// exists; then it returns that link and which key matched. Tags that differ from
// existing ones only in ASCII case take the existing spelling.
func (s *Store) AddLink(ctx context.Context, l *Link) (*Link, Dup, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, NotDup, err
	}
	defer tx.Rollback()

	var id int64
	dup := NotDup
	if l.SourceRef != "" {
		err = tx.QueryRowContext(ctx, `SELECT id FROM links WHERE source = ? AND source_ref = ?`,
			l.Source, l.SourceRef).Scan(&id)
		dup = DupSourceRef
	}
	if l.SourceRef == "" || err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx, `SELECT id FROM links WHERE url_norm = ?`, l.URLNorm).Scan(&id)
		dup = DupURL
	}
	switch {
	case err == nil:
		tx.Rollback()
		existing, err := s.LinkByID(ctx, id)
		return existing, dup, err
	case err != sql.ErrNoRows:
		return nil, NotDup, err
	}

	if err := knownTagSpelling(ctx, tx, l.Tags); err != nil {
		return nil, NotDup, err
	}
	if err := insertLink(ctx, tx, l); err != nil {
		return nil, NotDup, err
	}
	return l, NotDup, tx.Commit()
}

// knownTagSpelling replaces tags that differ from existing ones only in ASCII
// case with the existing spelling.
func knownTagSpelling(ctx context.Context, tx *sql.Tx, tags []string) error {
	for i, name := range tags {
		var known string
		err := tx.QueryRowContext(ctx, `SELECT name FROM tags WHERE name = ? COLLATE NOCASE ORDER BY id LIMIT 1`,
			name).Scan(&known)
		switch {
		case err == nil:
			tags[i] = known
		case err != sql.ErrNoRows:
			return err
		}
	}
	return nil
}

// ErrNotFound is returned when there is no link with the given ID.
var ErrNotFound = errors.New("link not found")

// LinkByID returns the link with the given ID, or ErrNotFound.
func (s *Store) LinkByID(ctx context.Context, id int64) (*Link, error) {
	ls, err := s.queryLinks(ctx, `WHERE l.id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(ls) == 0 {
		return nil, ErrNotFound
	}
	return ls[0], nil
}

// UpdateLink saves the editable fields of the link l.ID: URL, texts, day,
// visibility and tags; created_at and the source stay as they were. If another
// link already has l.URLNorm, nothing changes and that link is returned with
// DupURL. Tags take existing spellings as in AddLink. A new URL or a cleared
// title or site description puts the link back in the metadata queue.
func (s *Store) UpdateLink(ctx context.Context, l *Link) (*Link, Dup, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, NotDup, err
	}
	defer tx.Rollback()

	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM links WHERE url_norm = ? AND id <> ?`, l.URLNorm, l.ID).Scan(&id)
	switch {
	case err == nil:
		tx.Rollback()
		existing, err := s.LinkByID(ctx, id)
		return existing, DupURL, err
	case err != sql.ErrNoRows:
		return nil, NotDup, err
	}

	// SET expressions see the old values of the row.
	const refetch = `url_norm <> ?2 OR title <> '' AND ?4 = '' OR site_description <> '' AND ?5 = ''`
	res, err := tx.ExecContext(ctx, `
		UPDATE links
		   SET meta_status = CASE WHEN `+refetch+` THEN '' ELSE meta_status END,
		       meta_checked_at = CASE WHEN `+refetch+` THEN 0 ELSE meta_checked_at END,
		       meta_error = CASE WHEN `+refetch+` THEN '' ELSE meta_error END,
		       meta_failures = CASE WHEN `+refetch+` THEN 0 ELSE meta_failures END,
		       url = ?1, url_norm = ?2, description = ?3, title = ?4, site_description = ?5,
		       day = ?6, hidden = ?7, updated_at = ?8, note = ?10, site_author = ?11
		 WHERE id = ?9`,
		l.URL, l.URLNorm, l.Description, l.Title, l.SiteDescription, l.Day, l.Hidden, time.Now().Unix(), l.ID, l.Note,
		l.SiteAuthor)
	if err != nil {
		return nil, NotDup, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return nil, NotDup, err
	} else if n == 0 {
		return nil, NotDup, ErrNotFound
	}

	if err := knownTagSpelling(ctx, tx, l.Tags); err != nil {
		return nil, NotDup, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM link_tags WHERE link_id = ?`, l.ID); err != nil {
		return nil, NotDup, err
	}
	if err := insertTags(ctx, tx, l.ID, l.Tags); err != nil {
		return nil, NotDup, err
	}
	if err := dropUnusedTags(ctx, tx); err != nil {
		return nil, NotDup, err
	}
	if err := tx.Commit(); err != nil {
		return nil, NotDup, err
	}
	saved, err := s.LinkByID(ctx, l.ID)
	return saved, NotDup, err
}

// DeleteLink deletes the link with the given ID and returns it.
func (s *Store) DeleteLink(ctx context.Context, id int64) (*Link, error) {
	l, err := s.LinkByID(ctx, id)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM links WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if n == 0 {
		return nil, ErrNotFound
	}
	if err := dropUnusedTags(ctx, tx); err != nil {
		return nil, err
	}
	return l, tx.Commit()
}

func dropUnusedTags(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE id NOT IN (SELECT tag_id FROM link_tags)`)
	return err
}

// LinksByMessage returns the links added from one chat message: their
// source_ref is messageRef or starts with messageRef + ":".
func (s *Store) LinksByMessage(ctx context.Context, source, messageRef string) ([]*Link, error) {
	return s.queryLinks(ctx, `WHERE l.source = ? AND (l.source_ref = ? OR l.source_ref LIKE ? ESCAPE '\')
		ORDER BY l.source_ref`, source, messageRef, likeEscape(messageRef)+":%")
}

// FillMetadata sets the link's title and site description where they are
// still empty and reports whether anything changed.
func (s *Store) FillMetadata(ctx context.Context, id int64, title, siteDescription string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE links
		   SET title = CASE WHEN title = '' THEN ?1 ELSE title END,
		       site_description = CASE WHEN site_description = '' THEN ?2 ELSE site_description END,
		       updated_at = ?3
		 WHERE id = ?4 AND (title = '' AND ?1 <> '' OR site_description = '' AND ?2 <> '')`,
		title, siteDescription, time.Now().Unix(), id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// MetaRetries is how many failed metadata checks in a row a link gets when
// the failures are worth a retry.
const MetaRetries = 3

// NextForMetadata returns up to limit links whose title or site description
// is empty and that are due for a metadata check. Links never checked come
// first, newest first; one with MetaError is retried after failures² hours,
// up to MetaRetries checks.
func (s *Store) NextForMetadata(ctx context.Context, now time.Time, limit int) ([]*Link, error) {
	return s.queryLinks(ctx, `
		WHERE (l.title = '' OR l.site_description = '')
		  AND (l.meta_status = ''
		       OR l.meta_status = 'error' AND l.meta_failures < ?
		          AND l.meta_checked_at <= ? - 3600 * l.meta_failures * l.meta_failures)
		ORDER BY l.meta_status = '' DESC, l.id DESC
		LIMIT ?`, MetaRetries, now.Unix(), limit)
}

// MetaResult is the outcome of a metadata check.
type MetaResult struct {
	Status          MetaStatus
	Title           string // with MetaOK
	SiteDescription string
	SiteAuthor      string
	Error           string // otherwise
}

// SaveMetadata records a metadata check of the link: MetaOK fills the title,
// site description and site author where they are empty, MetaError counts a
// failure.
func (s *Store) SaveMetadata(ctx context.Context, id int64, r MetaResult, now time.Time) error {
	if r.Status == MetaOK {
		_, err := s.db.ExecContext(ctx, `
			UPDATE links
			   SET title = CASE WHEN title = '' THEN ?1 ELSE title END,
			       site_description = CASE WHEN site_description = '' THEN ?2 ELSE site_description END,
			       site_author = CASE WHEN site_author = '' THEN ?5 ELSE site_author END,
			       updated_at = CASE WHEN title = '' AND ?1 <> '' OR site_description = '' AND ?2 <> ''
			                          OR site_author = '' AND ?5 <> ''
			                    THEN ?3 ELSE updated_at END,
			       meta_status = 'ok', meta_checked_at = ?3, meta_error = '', meta_failures = 0
			 WHERE id = ?4`,
			r.Title, r.SiteDescription, now.Unix(), id, r.SiteAuthor)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE links
		   SET meta_status = ?1, meta_checked_at = ?2, meta_error = ?3,
		       meta_failures = CASE WHEN ?1 = 'error' THEN meta_failures + 1 ELSE meta_failures END
		 WHERE id = ?4`,
		r.Status, now.Unix(), r.Error, id)
	return err
}
