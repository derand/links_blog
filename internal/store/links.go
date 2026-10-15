package store

import (
	"context"
	"database/sql"
	"fmt"
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
