package main

import (
	"reflect"
	"testing"

	"github.com/derand/links_blog/internal/store"
)

func TestPlan(t *testing.T) {
	us := []store.LinkURL{
		{ID: 1, URL: "https://example.com/a", URLNorm: "example.com/a"},
		{ID: 2, URL: "https://example.com/b?utm_source=x", URLNorm: "example.com/b"},
		{ID: 3, URL: "https://youtu.be/dQw4w9WgXcQ", URLNorm: "youtu.be/dQw4w9WgXcQ"},
		// The same tweet twice, unnoticed until now.
		{ID: 4, URL: "https://x.com/i/status/123", URLNorm: "x.com/i/status/123"},
		{ID: 5, URL: "https://fixupx.com/someone/status/123", URLNorm: "fixupx.com/someone/status/123"},
	}
	r := plan(us)
	wantChanges := []store.LinkURL{
		{ID: 2, URL: "https://example.com/b", URLNorm: "example.com/b"},
		{ID: 3, URL: "https://youtu.be/dQw4w9WgXcQ", URLNorm: "youtube.com/watch?v=dQw4w9WgXcQ"},
	}
	if !reflect.DeepEqual(r.changes, wantChanges) {
		t.Errorf("changes = %+v, want %+v", r.changes, wantChanges)
	}
	wantConflicts := []conflict{{"x.com/i/status/123", []int64{4, 5}}}
	if !reflect.DeepEqual(r.conflicts, wantConflicts) {
		t.Errorf("conflicts = %+v, want %+v", r.conflicts, wantConflicts)
	}
}
