package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/derand/links_blog/internal/links"
)

func TestLinksByMessageAndFillMetadata(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := time.Now()
	add := func(norm, ref, title string) *Link {
		t.Helper()
		l := &Link{URL: "https://" + norm, URLNorm: norm, Day: "2026-10-17", CreatedAt: now,
			Source: "discord", SourceRef: ref, Title: title}
		if _, _, err := s.AddLink(ctx, l); err != nil {
			t.Fatal(err)
		}
		return l
	}
	a := add("a.example", "1:2:0", "")
	b := add("b.example", "1:2:1", "Kept")
	add("c.example", "1:20:0", "")

	ls, err := s.LinksByMessage(ctx, "discord", "1:2")
	if err != nil || len(ls) != 2 || ls[0].ID != a.ID || ls[1].ID != b.ID {
		t.Fatalf("LinksByMessage = %+v, %v", ls, err)
	}

	if ok, err := s.FillMetadata(ctx, a.ID, "Title A", "About A"); !ok || err != nil {
		t.Errorf("fill a: %v, %v", ok, err)
	}
	if ok, err := s.FillMetadata(ctx, b.ID, "New", "About B"); !ok || err != nil {
		t.Errorf("fill b: %v, %v", ok, err)
	}
	// Nothing left to fill.
	if ok, err := s.FillMetadata(ctx, b.ID, "Newer", "Other"); ok || err != nil {
		t.Errorf("fill b again: %v, %v", ok, err)
	}
	ls, _ = s.LinksByMessage(ctx, "discord", "1:2")
	if ls[0].Title != "Title A" || ls[0].SiteDescription != "About A" ||
		ls[1].Title != "Kept" || ls[1].SiteDescription != "About B" {
		t.Errorf("after fill: %+v %+v", ls[0], ls[1])
	}
}

func TestUpdateAndDeleteLink(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	created := time.Unix(1_700_000_000, 0)
	add := func(norm string, tags ...string) *Link {
		t.Helper()
		l := &Link{URL: "https://" + norm, URLNorm: norm, Description: "old words", Day: "2026-10-17",
			CreatedAt: created, Source: "discord", SourceRef: norm, Tags: tags}
		if _, _, err := s.AddLink(ctx, l); err != nil {
			t.Fatal(err)
		}
		return l
	}
	a := add("a.example", "Go", "only-a")
	b := add("b.example", "go")

	edit := &Link{ID: a.ID, URL: "https://a2.example/x", URLNorm: "a2.example/x", Description: "fresh text",
		Title: "T", SiteDescription: "SD", Day: "2026-10-01", Hidden: true, Tags: []string{"new", "GO"}}
	got, dup, err := s.UpdateLink(ctx, edit)
	if err != nil || dup != NotDup {
		t.Fatalf("update: %v %v", dup, err)
	}
	if got.URL != edit.URL || got.Description != "fresh text" || got.Title != "T" || got.Day != "2026-10-01" ||
		!got.Hidden || got.Source != "discord" || got.SourceRef != "a.example" || !got.CreatedAt.Equal(created) ||
		strings.Join(got.Tags, ",") != "new,Go" {
		t.Errorf("updated link: %+v", got)
	}

	search := func(q string) int {
		t.Helper()
		pq, err := links.ParseQuery(q)
		if err != nil {
			t.Fatal(err)
		}
		_, n, err := s.Search(ctx, pq, 0, 10, true)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if search("fresh") != 1 || search("old") != 1 || search("tag:new") != 1 || search("tag:only-a") != 0 {
		t.Error("search index is stale after update")
	}
	var unused int
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM tags WHERE name = 'only-a'`).Scan(&unused)
	if unused != 0 {
		t.Error("unused tag kept")
	}

	// Taking b's URL is refused.
	got, dup, err = s.UpdateLink(ctx, &Link{ID: a.ID, URL: "http://b.example", URLNorm: "b.example", Day: "2026-10-01"})
	if err != nil || dup != DupURL || got.ID != b.ID {
		t.Errorf("update to a taken URL: %v %v %+v", dup, err, got)
	}
	if _, _, err := s.UpdateLink(ctx, &Link{ID: 999, URLNorm: "z.example", Day: "2026-10-01"}); err != ErrNotFound {
		t.Errorf("update missing: %v", err)
	}

	if l, err := s.DeleteLink(ctx, a.ID); err != nil || l.ID != a.ID {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.LinkByID(ctx, a.ID); err != ErrNotFound {
		t.Errorf("deleted link still there: %v", err)
	}
	if _, err := s.DeleteLink(ctx, a.ID); err != ErrNotFound {
		t.Errorf("delete twice: %v", err)
	}
	if search("fresh") != 0 || search("tag:new") != 0 || search("tag:Go") != 1 {
		t.Error("search index is stale after delete")
	}
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM tags`).Scan(&unused)
	if unused != 1 {
		t.Errorf("tags left: %d, want 1 (Go)", unused)
	}
}

func TestMetadataQueue(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := time.Unix(1_800_000_000, 0)
	add := func(norm, title, sd string) *Link {
		t.Helper()
		l := &Link{URL: "https://" + norm, URLNorm: norm, Day: "2026-10-18", CreatedAt: now, Title: title, SiteDescription: sd}
		if _, _, err := s.AddLink(ctx, l); err != nil {
			t.Fatal(err)
		}
		return l
	}
	next := func(at time.Time) []int64 {
		t.Helper()
		ls, err := s.NextForMetadata(ctx, at, 10)
		if err != nil {
			t.Fatal(err)
		}
		var ids []int64
		for _, l := range ls {
			ids = append(ids, l.ID)
		}
		return ids
	}
	save := func(id int64, r MetaResult, at time.Time) {
		t.Helper()
		if err := s.SaveMetadata(ctx, id, r, at); err != nil {
			t.Fatal(err)
		}
	}
	old := add("old.example", "", "")
	add("full.example", "T", "D")
	half := add("half.example", "Discord title", "")
	blocked := add("blocked.example", "", "")
	dead := add("dead.example", "", "")

	if got := fmt.Sprint(next(now)); got != fmt.Sprint([]int64{dead.ID, blocked.ID, half.ID, old.ID}) {
		t.Fatalf("queue: %v", got)
	}
	// Success fills only what is empty.
	save(half.ID, MetaResult{Status: MetaOK, Title: "Page title", SiteDescription: "Page desc"}, now)
	l, _ := s.LinkByID(ctx, half.ID)
	if l.Title != "Discord title" || l.SiteDescription != "Page desc" || l.MetaStatus != MetaOK || !l.MetaCheckedAt.Equal(now) {
		t.Errorf("after success: %+v", l)
	}
	// Blocked and dead links are not retried.
	save(blocked.ID, MetaResult{Status: MetaBlocked, Error: "blocked: cloudflare challenge"}, now)
	save(dead.ID, MetaResult{Status: MetaDead, Error: "http status 404"}, now)
	if got := fmt.Sprint(next(now.Add(1000 * time.Hour))); got != fmt.Sprint([]int64{old.ID}) {
		t.Fatalf("after blocked and dead: %v", got)
	}
	l, _ = s.LinkByID(ctx, blocked.ID)
	if l.MetaStatus != MetaBlocked || l.MetaError != "blocked: cloudflare challenge" || l.MetaFailures != 0 {
		t.Errorf("blocked: %+v", l)
	}

	// Failures worth a retry wait 1 h, then 4 h, then give up.
	at := now
	for i, wait := range []time.Duration{time.Hour, 4 * time.Hour} {
		save(old.ID, MetaResult{Status: MetaError, Error: "http status 503"}, at)
		if got := next(at.Add(wait - time.Second)); len(got) != 0 {
			t.Errorf("failure %d: retried too early", i+1)
		}
		at = at.Add(wait)
		if got := next(at); len(got) != 1 || got[0] != old.ID {
			t.Errorf("failure %d: not retried after %v", i+1, wait)
		}
	}
	save(old.ID, MetaResult{Status: MetaError, Error: "http status 503"}, at)
	if got := next(at.Add(1000 * time.Hour)); len(got) != 0 {
		t.Errorf("retried after %d failures", MetaRetries)
	}
	l, _ = s.LinkByID(ctx, old.ID)
	if l.MetaStatus != MetaError || l.MetaError != "http status 503" || l.MetaFailures != 3 {
		t.Errorf("failure state: %+v", l)
	}

	// Editing: the same URL and texts keep the state, a new URL resets it.
	l.Description = "edited"
	s.UpdateLink(ctx, l)
	if got := next(at.Add(1000 * time.Hour)); len(got) != 0 {
		t.Error("edit without changes requeued the link")
	}
	l.URL, l.URLNorm = "https://new.example", "new.example"
	s.UpdateLink(ctx, l)
	l, _ = s.LinkByID(ctx, old.ID)
	if l.MetaStatus != MetaPending || l.MetaFailures != 0 || l.MetaError != "" || !l.MetaCheckedAt.IsZero() {
		t.Errorf("state after new URL: %+v", l)
	}

	// Clearing a fetched title requeues a checked link.
	h, _ := s.LinkByID(ctx, half.ID)
	h.Title = ""
	s.UpdateLink(ctx, h)
	if got := fmt.Sprint(next(now)); got != fmt.Sprint([]int64{half.ID, old.ID}) {
		t.Errorf("after clearing a title: %v", got)
	}
}
