package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/derand/links_blog/internal/meta"
	"github.com/derand/links_blog/internal/store"
)

type fakeFetcher struct {
	mu    sync.Mutex
	pages map[string]meta.Metadata
	errs  map[string]error
	calls []string
	at    map[string]time.Time
}

func (f *fakeFetcher) Fetch(ctx context.Context, url string) (meta.Metadata, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, url)
	f.at[url] = time.Now()
	if err, ok := f.errs[url]; ok {
		return meta.Metadata{}, err
	}
	return f.pages[url], nil
}

func (f *fakeFetcher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// waitFor polls cond until it holds or two seconds pass.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestRunMetadata(t *testing.T) {
	defer func(g time.Duration) { hostGap = g }(hostGap)
	hostGap = 300 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	s, st := newTest(t)
	f := &fakeFetcher{
		pages: map[string]meta.Metadata{
			"https://a.example/1": {Title: "Page A1", Description: "About A1"},
			"https://a.example/2": {Title: "Page A2"},
			"https://b.example":   {Title: "Page B", Description: "About B"},
		},
		errs: map[string]error{
			"https://wall.example/x": &meta.BlockedError{Reason: "cloudflare challenge"},
			"https://gone.example/x": &meta.StatusError{Code: 404},
			"https://busy.example/x": &meta.StatusError{Code: 503},
		},
		at: map[string]time.Time{},
	}
	link := func(id int64) *store.Link {
		l, err := st.LinkByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	add := func(url string) int64 {
		r, err := s.AddLink(ctx, NewLink{URL: url, Description: "mine"})
		if err != nil {
			t.Fatal(err)
		}
		return r.Link.ID
	}
	a1, a2 := add("a.example/1"), add("a.example/2")
	wall, gone, busy := add("wall.example/x"), add("gone.example/x"), add("busy.example/x")
	private := add("https://discord.com/channels/1/2/3")

	done := make(chan struct{})
	go func() {
		s.RunMetadata(ctx, f, time.Millisecond)
		close(done)
	}()

	waitFor(t, "a.example/1", func() bool { return link(a1).Title != "" })
	if l := link(a1); l.Description != "mine" || l.Title != "Page A1" || l.SiteDescription != "About A1" ||
		l.MetaStatus != store.MetaOK {
		t.Errorf("a1: %+v", l)
	}
	if l := link(a2); l.Title != "Page A2" || l.MetaStatus != store.MetaOK {
		t.Errorf("a2: %+v", l)
	}
	f.mu.Lock()
	if gap := f.at["https://a.example/1"].Sub(f.at["https://a.example/2"]); gap < hostGap {
		t.Errorf("same host asked %v apart, want at least %v", gap, hostGap)
	}
	f.mu.Unlock()

	// Private links are not asked for (the fetch count below).
	for id, want := range map[int64]store.MetaStatus{
		wall: store.MetaBlocked, gone: store.MetaDead, busy: store.MetaError, private: store.MetaBlocked,
	} {
		if l := link(id); l.MetaStatus != want || l.MetaError == "" {
			t.Errorf("%s: status %q %q, want %q", l.URL, l.MetaStatus, l.MetaError, want)
		}
	}

	// A new link wakes the idle worker.
	b := add("https://b.example")
	waitFor(t, "b", func() bool { return link(b).Title == "Page B" })

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunMetadata did not stop")
	}
	if n := f.count(); n != 6 {
		t.Errorf("fetches: %d %v, want 6: none repeated", n, f.calls)
	}
}
