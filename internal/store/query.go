package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode"

	"github.com/derand/links_blog/internal/links"
)

// Ping checks that the database answers.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

const visibleOnly = ` AND l.hidden = 0`

// Days returns up to limit days that have links, newest first, skipping offset
// days, and the total number of such days.
func (s *Store) Days(ctx context.Context, offset, limit int, withHidden bool) ([]string, int, error) {
	where := `WHERE 1 = 1`
	if !withHidden {
		where += visibleOnly
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT count(DISTINCT day) FROM links l `+where).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT day FROM links l `+where+` ORDER BY day DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var days []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, 0, err
		}
		days = append(days, d)
	}
	return days, total, rows.Err()
}

// LinksOnDays returns the links of the given days: newest day first, in
// posting order within a day.
func (s *Store) LinksOnDays(ctx context.Context, days []string, withHidden bool) ([]*Link, error) {
	if len(days) == 0 {
		return nil, nil
	}
	args := make([]any, len(days))
	for i, d := range days {
		args[i] = d
	}
	where := `WHERE l.day IN (?` + strings.Repeat(`, ?`, len(days)-1) + `)`
	if !withHidden {
		where += visibleOnly
	}
	return s.queryLinks(ctx, where+` ORDER BY l.day DESC, l.created_at, l.id`, args...)
}

// Search returns a page of links matching q, newest first, and the total number of matches.
func (s *Store) Search(ctx context.Context, q links.Query, offset, limit int, withHidden bool) ([]*Link, int, error) {
	var (
		where strings.Builder
		args  []any
	)
	where.WriteString(`WHERE 1 = 1`)
	if expr := ftsExpr(q.Terms); expr != "" {
		where.WriteString(` AND l.id IN (SELECT rowid FROM links_fts WHERE links_fts MATCH ?)`)
		args = append(args, expr)
	} else if len(q.Terms) > 0 {
		return nil, 0, nil // only punctuation: nothing can match
	}
	for _, t := range q.Tags {
		where.WriteString(` AND EXISTS (SELECT 1 FROM link_tags lt JOIN tags t ON t.id = lt.tag_id
		                                 WHERE lt.link_id = l.id AND t.name = ?)`)
		args = append(args, t)
	}
	if len(q.Dates) > 0 {
		where.WriteString(` AND (` + strings.Repeat(`l.day GLOB ? OR `, len(q.Dates)-1) + `l.day GLOB ?)`)
		for _, d := range q.Dates {
			args = append(args, d+"*")
		}
	}
	if !withHidden {
		where.WriteString(visibleOnly)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM links l `+where.String(), args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 || offset >= total {
		return nil, total, nil
	}
	ls, err := s.queryLinks(ctx, where.String()+` ORDER BY l.day DESC, l.created_at DESC, l.id DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	return ls, total, err
}

// ftsExpr turns search terms into an FTS5 query: every term is a quoted phrase
// matched as a prefix, all terms must match.
func ftsExpr(terms []string) string {
	var parts []string
	for _, t := range terms {
		if !strings.ContainsFunc(t, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
			continue
		}
		parts = append(parts, `"`+strings.ReplaceAll(t, `"`, `""`)+`"*`)
	}
	return strings.Join(parts, " ")
}

func (s *Store) queryLinks(ctx context.Context, tail string, args ...any) ([]*Link, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, l.url, l.url_norm, l.description, l.note, l.title, l.site_description,
		       l.day, l.created_at, l.updated_at, l.hidden, l.source, l.source_ref,
		       l.meta_status, l.meta_checked_at, l.meta_error, l.meta_failures
		  FROM links l `+tail, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var (
		ls   []*Link
		byID = map[int64]*Link{}
	)
	for rows.Next() {
		var (
			l                Link
			created, updated int64
			sourceRef        sql.NullString
			checked          int64
		)
		if err := rows.Scan(&l.ID, &l.URL, &l.URLNorm, &l.Description, &l.Note, &l.Title, &l.SiteDescription,
			&l.Day, &created, &updated, &l.Hidden, &l.Source, &sourceRef,
			&l.MetaStatus, &checked, &l.MetaError, &l.MetaFailures); err != nil {
			return nil, err
		}
		if checked != 0 {
			l.MetaCheckedAt = time.Unix(checked, 0)
		}
		l.CreatedAt = time.Unix(created, 0)
		l.UpdatedAt = time.Unix(updated, 0)
		l.SourceRef = sourceRef.String
		ls = append(ls, &l)
		byID[l.ID] = &l
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ls, s.loadTags(ctx, byID)
}

func (s *Store) loadTags(ctx context.Context, byID map[int64]*Link) error {
	if len(byID) == 0 {
		return nil
	}
	args := make([]any, 0, len(byID))
	for id := range byID {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT lt.link_id, t.name FROM link_tags lt JOIN tags t ON t.id = lt.tag_id
		 WHERE lt.link_id IN (?`+strings.Repeat(`, ?`, len(args)-1)+`)
		 ORDER BY lt.link_id, lt.pos`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id   int64
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		byID[id].Tags = append(byID[id].Tags, name)
	}
	return rows.Err()
}
