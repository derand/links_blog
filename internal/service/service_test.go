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
	if l.URL != "https://www.Example.com/a/?utm_source=x" || l.Description != "Опис" {
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
