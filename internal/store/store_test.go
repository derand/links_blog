package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	for i := 0; i < 2; i++ {
		s, err := Open(ctx, path)
		if err != nil {
			t.Fatalf("open #%d: %v", i+1, err)
		}
		v, err := s.SchemaVersion(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if v != 6 {
			t.Errorf("open #%d: schema version = %d, want 6", i+1, v)
		}
		s.Close()
	}
}

func TestInsertAndGet(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	created := time.Date(2016, 9, 8, 10, 0, 0, 0, time.UTC)
	l := &Link{
		URL: "https://example.com/a", URLNorm: "example.com/a",
		Description: "Приклад", Day: "2016-09-08", CreatedAt: created,
		Hidden: true, Source: "mongo", Tags: []string{"go", "sqlite", "go"},
	}
	if err := s.InsertLinks(ctx, []*Link{l}); err != nil {
		t.Fatal(err)
	}
	if l.ID == 0 {
		t.Fatal("ID not set")
	}

	got, err := s.LinkByURLNorm(ctx, "example.com/a")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("link not found")
	}
	if got.Description != "Приклад" || !got.Hidden || got.Day != "2016-09-08" ||
		!got.CreatedAt.Equal(created) || !got.UpdatedAt.Equal(created) || got.SourceRef != "" {
		t.Errorf("unexpected link: %+v", got)
	}
	if want := []string{"go", "sqlite"}; !reflect.DeepEqual(got.Tags, want) {
		t.Errorf("tags = %v, want %v", got.Tags, want)
	}

	if got, err := s.LinkByURLNorm(ctx, "missing"); err != nil || got != nil {
		t.Errorf("missing link: got %v, %v", got, err)
	}
}

func TestConstraints(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := time.Now()
	ok := &Link{URL: "https://a.com", URLNorm: "a.com", Day: "2020-01-02", CreatedAt: now}
	if err := s.InsertLinks(ctx, []*Link{ok}); err != nil {
		t.Fatal(err)
	}
	bad := []*Link{
		{URL: "https://www.a.com", URLNorm: "a.com", Day: "2020-01-02", CreatedAt: now},
		{URL: "https://b.com", URLNorm: "b.com", Day: "2020102", CreatedAt: now},
	}
	for _, l := range bad {
		if err := s.InsertLinks(ctx, []*Link{l}); err == nil {
			t.Errorf("insert %+v: want error", l)
		}
	}
	// A failed batch is rolled back as a whole.
	batch := []*Link{
		{URL: "https://c.com", URLNorm: "c.com", Day: "2020-01-02", CreatedAt: now},
		{URL: "https://a.com/", URLNorm: "a.com", Day: "2020-01-02", CreatedAt: now},
	}
	if err := s.InsertLinks(ctx, batch); err == nil {
		t.Error("batch with duplicate: want error")
	}
	if n, _ := s.CountLinks(ctx); n != 1 {
		t.Errorf("count = %d, want 1", n)
	}
}

func ftsMatch(t *testing.T, s *Store, query string) []int64 {
	t.Helper()
	rows, err := s.db.Query(`SELECT rowid FROM links_fts WHERE links_fts MATCH ? ORDER BY rowid`, query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestFTSSync(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := time.Now()
	a := &Link{URL: "https://github.com/derand/x", URLNorm: "github.com/derand/x",
		Description: "Нейронна мережа для тексту", Note: "Повний текст про трансформери",
		Day: "2020-01-02", CreatedAt: now, Tags: []string{"neural network", "ml"}}
	b := &Link{URL: "https://example.org", URLNorm: "example.org",
		Description: "Something else", Day: "2020-01-03", CreatedAt: now}
	if err := s.InsertLinks(ctx, []*Link{a, b}); err != nil {
		t.Fatal(err)
	}

	check := func(query string, want ...int64) {
		t.Helper()
		if got := ftsMatch(t, s, query); !reflect.DeepEqual(got, want) {
			t.Errorf("MATCH %q = %v, want %v", query, got, want)
		}
	}
	check("мережа", a.ID)
	check("МЕРЕЖА", a.ID)
	check("github", a.ID)           // url
	check("ml", a.ID)               // tags
	check(`"neural network"`, a.ID) // tag phrase
	check("something", b.ID)
	check("трансформери", a.ID) // note

	// Update of the text columns is reflected.
	if _, err := s.db.Exec(`UPDATE links SET description = 'Інший опис', note = 'Нотатка' WHERE id = ?`, a.ID); err != nil {
		t.Fatal(err)
	}
	check("мережа")
	check("трансформери")
	check("опис", a.ID)
	check("нотатка", a.ID)
	check("ml", a.ID) // tags survive the update

	// Removing a tag updates the index.
	if _, err := s.db.Exec(`DELETE FROM link_tags WHERE link_id = ? AND tag_id = (SELECT id FROM tags WHERE name = 'ml')`, a.ID); err != nil {
		t.Fatal(err)
	}
	check("ml")
	check("neural", a.ID)

	// Deleting a link removes it from the index and cascades to link_tags.
	if _, err := s.db.Exec(`DELETE FROM links WHERE id = ?`, a.ID); err != nil {
		t.Fatal(err)
	}
	check("neural")
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM link_tags WHERE link_id = ?`, a.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("link_tags left: %d, %v", n, err)
	}
}
