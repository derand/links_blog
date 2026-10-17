package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Link struct {
	ID              int64
	URL             string
	URLNorm         string
	Description     string
	Title           string
	SiteDescription string
	Day             string // YYYY-MM-DD
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Hidden          bool
	Source          string
	SourceRef       string // empty means none
	Tags            []string
}

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
	res, err := tx.ExecContext(ctx, `
		INSERT INTO links (url, url_norm, description, title, site_description,
		                   day, created_at, updated_at, hidden, source, source_ref)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.URL, l.URLNorm, l.Description, l.Title, l.SiteDescription,
		l.Day, l.CreatedAt.Unix(), l.UpdatedAt.Unix(), l.Hidden, l.Source, sourceRef)
	if err != nil {
		return err
	}
	if l.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	for i, name := range l.Tags {
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
			l.ID, tagID, i); err != nil {
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
		existing, err := s.linkByID(ctx, id)
		return existing, dup, err
	case err != sql.ErrNoRows:
		return nil, NotDup, err
	}

	for i, name := range l.Tags {
		var known string
		err := tx.QueryRowContext(ctx, `SELECT name FROM tags WHERE name = ? COLLATE NOCASE ORDER BY id LIMIT 1`,
			name).Scan(&known)
		switch {
		case err == nil:
			l.Tags[i] = known
		case err != sql.ErrNoRows:
			return nil, NotDup, err
		}
	}
	if err := insertLink(ctx, tx, l); err != nil {
		return nil, NotDup, err
	}
	return l, NotDup, tx.Commit()
}

func (s *Store) linkByID(ctx context.Context, id int64) (*Link, error) {
	ls, err := s.queryLinks(ctx, `WHERE l.id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(ls) == 0 {
		return nil, sql.ErrNoRows
	}
	return ls[0], nil
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
