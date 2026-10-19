package service

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/derand/links_blog/internal/store"
)

func newTest(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatal(err)
	}
	s := New(st, kyiv)
	s.now = func() time.Time { return time.Date(2026, 10, 16, 22, 30, 0, 0, time.UTC) }
	return s, st
}

func TestAddLink(t *testing.T) {
	ctx := context.Background()
	s, _ := newTest(t)

	res, err := s.AddLink(ctx, NewLink{
		URL:         " https://www.Example.com/a/?utm_source=x ",
		Description: "  Опис  ",
		Tags:        []string{"#go", " ", "Go", "sqlite"},
		Hidden:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	l := res.Link
	if res.Duplicate != store.NotDup || l.ID == 0 {
		t.Fatalf("got dup=%v id=%d, want a new link", res.Duplicate, l.ID)
	}
	if l.URL != "https://www.Example.com/a/" || l.Description != "Опис" {
		t.Errorf("url=%q description=%q", l.URL, l.Description)
	}
	// 22:30 UTC is already the next day in Kyiv.
	if l.Day != "2026-10-17" || l.Source != "web" || !l.Hidden {
		t.Errorf("day=%q source=%q hidden=%v", l.Day, l.Source, l.Hidden)
	}
	if want := []string{"go", "sqlite"}; !reflect.DeepEqual(l.Tags, want) {
		t.Errorf("tags = %q, want %q", l.Tags, want)
	}
}

func TestAddLinkCanonical(t *testing.T) {
	ctx := context.Background()
	s, _ := newTest(t)

	res, err := s.AddLink(ctx, NewLink{URL: "https://fixupx.com/i/status/123?s=20"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Link.URL != "https://x.com/i/status/123" {
		t.Errorf("url = %q, want https://x.com/i/status/123", res.Link.URL)
	}
	// The same tweet by its author's nick.
	res2, err := s.AddLink(ctx, NewLink{URL: "x.com/someone/status/123?t=abc"})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Duplicate != store.DupURL || res2.Link.ID != res.Link.ID {
		t.Errorf("got dup=%v id=%d, want DupURL of %d", res2.Duplicate, res2.Link.ID, res.Link.ID)
	}

	edit, err := s.UpdateLink(ctx, LinkEdit{ID: res.Link.ID, URL: "https://youtu.be/dQw4w9WgXcQ?si=x", Day: res.Link.Day})
	if err != nil {
		t.Fatal(err)
	}
	if edit.Link.URL != "https://youtu.be/dQw4w9WgXcQ" {
		t.Errorf("edited url = %q", edit.Link.URL)
	}
}

func TestAddLinkPrivate(t *testing.T) {
	ctx := context.Background()
	s, _ := newTest(t)
	for url, hidden := range map[string]bool{
		"https://t.me/c/1729400185/1549120": true,
		"https://t.me/Coin_Post/15792":      false,
	} {
		res, err := s.AddLink(ctx, NewLink{URL: url})
		if err != nil {
			t.Fatal(err)
		}
		if res.Link.Hidden != hidden {
			t.Errorf("%s: hidden = %v, want %v", url, res.Link.Hidden, hidden)
		}
	}
}

func TestAddLinkDuplicates(t *testing.T) {
	ctx := context.Background()
	s, st := newTest(t)

	first, err := s.AddLink(ctx, NewLink{URL: "https://example.com/x", Source: "discord", SourceRef: "1:2"})
	if err != nil {
		t.Fatal(err)
	}

	// Same URL in another form, from another message.
	res, err := s.AddLink(ctx, NewLink{URL: "http://example.com/x/", Source: "discord", SourceRef: "1:3"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Duplicate != store.DupURL || res.Link.ID != first.Link.ID {
		t.Errorf("same url: dup=%v id=%d, want DupURL id=%d", res.Duplicate, res.Link.ID, first.Link.ID)
	}

	// The same message processed again, even with another URL.
	res, err = s.AddLink(ctx, NewLink{URL: "https://other.org", Source: "discord", SourceRef: "1:2"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Duplicate != store.DupSourceRef || res.Link.ID != first.Link.ID {
		t.Errorf("same ref: dup=%v id=%d, want DupSourceRef id=%d", res.Duplicate, res.Link.ID, first.Link.ID)
	}

	// The same ref in another source is not a duplicate.
	res, err = s.AddLink(ctx, NewLink{URL: "https://third.org", Source: "telegram", SourceRef: "1:2"})
	if err != nil || res.Duplicate != store.NotDup {
		t.Errorf("other source: dup=%v err=%v, want a new link", res.Duplicate, err)
	}

	if n, _ := st.CountLinks(ctx); n != 2 {
		t.Errorf("links = %d, want 2", n)
	}
}

func TestAddLinkDayAndTime(t *testing.T) {
	ctx := context.Background()
	s, _ := newTest(t)

	at := time.Date(2026, 1, 31, 23, 10, 0, 0, time.UTC)
	res, err := s.AddLink(ctx, NewLink{URL: "a.example", At: at})
	if err != nil {
		t.Fatal(err)
	}
	if l := res.Link; l.Day != "2026-02-01" || !l.CreatedAt.Equal(at) || l.URL != "https://a.example" {
		t.Errorf("day=%q created=%v url=%q", l.Day, l.CreatedAt, l.URL)
	}

	res, err = s.AddLink(ctx, NewLink{URL: "https://b.example", Day: "2020-05-06"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Link.Day != "2020-05-06" {
		t.Errorf("explicit day = %q", res.Link.Day)
	}
}

func TestAddLinkReusesTagSpelling(t *testing.T) {
	ctx := context.Background()
	s, _ := newTest(t)

	if _, err := s.AddLink(ctx, NewLink{URL: "https://a.example", Tags: []string{"JavaScript"}}); err != nil {
		t.Fatal(err)
	}
	res, err := s.AddLink(ctx, NewLink{URL: "https://b.example", Tags: []string{"javascript", "new"}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"JavaScript", "new"}; !reflect.DeepEqual(res.Link.Tags, want) {
		t.Errorf("tags = %q, want %q", res.Link.Tags, want)
	}
}

func TestAddLinkInvalid(t *testing.T) {
	ctx := context.Background()
	s, _ := newTest(t)
	for _, in := range []NewLink{
		{URL: ""},
		{URL: "ftp://example.com"},
		{URL: "mailto:a@b.c"},
		{URL: "https://example.com", Day: "2020-13-01"},
	} {
		if _, err := s.AddLink(ctx, in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: err = %v, want ErrInvalid", in, err)
		}
	}
}

func TestUpdateLink(t *testing.T) {
	ctx := context.Background()
	s, _ := newTest(t)
	a, _ := s.AddLink(ctx, NewLink{URL: "a.example", Description: "A", Tags: []string{"x"}})
	b, _ := s.AddLink(ctx, NewLink{URL: "b.example"})

	res, err := s.UpdateLink(ctx, LinkEdit{ID: a.Link.ID, URL: " a2.example/p ", Description: " New ",
		Title: " T ", Tags: []string{"#y", " y", "z"}, Hidden: true, Day: "2026-01-02"})
	if err != nil || res.Duplicate != store.NotDup {
		t.Fatalf("update: %v %v", res.Duplicate, err)
	}
	l := res.Link
	if l.URL != "https://a2.example/p" || l.URLNorm != "a2.example/p" || l.Description != "New" || l.Title != "T" ||
		!l.Hidden || l.Day != "2026-01-02" || !reflect.DeepEqual(l.Tags, []string{"y", "z"}) {
		t.Errorf("updated: %+v", l)
	}

	res, err = s.UpdateLink(ctx, LinkEdit{ID: a.Link.ID, URL: "https://www.b.example/", Day: "2026-01-02"})
	if err != nil || res.Duplicate != store.DupURL || res.Link.ID != b.Link.ID {
		t.Errorf("taken URL: %v %v", res.Duplicate, err)
	}

	for _, in := range []LinkEdit{
		{ID: a.Link.ID, URL: "", Day: "2026-01-02"},
		{ID: a.Link.ID, URL: "a.example", Day: ""},
	} {
		if _, err := s.UpdateLink(ctx, in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: err = %v, want ErrInvalid", in, err)
		}
	}
	if _, err := s.UpdateLink(ctx, LinkEdit{ID: 999, URL: "c.example", Day: "2026-01-02"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}

	if _, err := s.DeleteLink(ctx, a.Link.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Link(ctx, a.Link.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
}
