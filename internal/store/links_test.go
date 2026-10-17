package store

import (
	"context"
	"testing"
	"time"
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
