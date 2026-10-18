package service

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/derand/links_blog/internal/links"
	"github.com/derand/links_blog/internal/meta"
	"github.com/derand/links_blog/internal/store"
)

// Fetcher gets a page's metadata; *meta.Fetcher is the real one.
type Fetcher interface {
	Fetch(ctx context.Context, url string) (meta.Metadata, error)
}

const (
	metaIdle     = 10 * time.Minute // how often to look for due retries when the queue is empty
	metaBatch    = 200              // links considered at once when choosing a host to ask
	maxErrLength = 300
)

// hostGap is the least time between two requests to one host: sites ban
// clients that ask too often (habr stopped answering at one request a second).
var hostGap = 30 * time.Second

// RunMetadata fills empty titles and site descriptions from the links' pages
// until ctx is done: one page at a time, pause apart, each host at most once
// per hostGap. New and edited links are picked up right away.
func (s *Service) RunMetadata(ctx context.Context, f Fetcher, pause time.Duration) {
	lastAsked := map[string]time.Time{} // host -> last request
	for {
		ls, err := s.st.NextForMetadata(ctx, s.now(), metaBatch)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Error("metadata queue", "err", err)
		}
		if len(ls) == 0 {
			if !sleep(ctx, metaIdle, s.wake) {
				return
			}
			continue
		}

		now := time.Now()
		for h, t := range lastAsked {
			if now.Sub(t) >= hostGap {
				delete(lastAsked, h)
			}
		}
		var (
			next *store.Link
			wait = hostGap
		)
		for _, l := range ls {
			t, busy := lastAsked[host(l.URL)]
			if !busy {
				next = l
				break
			}
			wait = min(wait, hostGap-now.Sub(t))
		}
		if next == nil {
			if !sleep(ctx, wait, s.wake) {
				return
			}
			continue
		}
		lastAsked[host(next.URL)] = now
		s.fetchMetadata(ctx, f, next)
		if !sleep(ctx, pause, nil) {
			return
		}
	}
}

func host(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

func (s *Service) fetchMetadata(ctx context.Context, f Fetcher, l *store.Link) {
	if links.Private(l.URL) {
		// Only a login page is there for a stranger.
		r := store.MetaResult{Status: store.MetaBlocked, Error: "private link"}
		if err := s.st.SaveMetadata(ctx, l.ID, r, s.now()); err != nil {
			slog.Error("save metadata", "link", l.ID, "err", err)
		}
		return
	}
	m, err := f.Fetch(ctx, l.URL)
	if ctx.Err() != nil {
		return // shutting down: not the page's fault
	}
	r := store.MetaResult{Status: store.MetaOK, Title: m.Title, SiteDescription: m.Description}
	if err != nil {
		var blocked *meta.BlockedError
		switch {
		case errors.As(err, &blocked):
			r.Status = store.MetaBlocked
		case meta.Permanent(err):
			r.Status = store.MetaDead
		default:
			r.Status = store.MetaError
		}
		r.Error = err.Error()
		if rs := []rune(r.Error); len(rs) > maxErrLength {
			r.Error = string(rs[:maxErrLength-1]) + "…"
		}
		slog.Info("metadata failed", "link", l.ID, "url", l.URL, "status", r.Status, "err", r.Error)
	} else {
		slog.Debug("metadata", "link", l.ID, "url", l.URL, "title", m.Title)
	}
	if err := s.st.SaveMetadata(ctx, l.ID, r, s.now()); err != nil {
		slog.Error("save metadata", "link", l.ID, "err", err)
	}
}

// sleep waits for d, a value from wake or the end of ctx; it reports false
// for the last.
func sleep(ctx context.Context, d time.Duration, wake <-chan struct{}) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
	case <-wake:
	}
	return true
}
