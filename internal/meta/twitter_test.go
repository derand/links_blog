package meta

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestTwitterAPIPath(t *testing.T) {
	for raw, want := range map[string]string{
		"https://x.com/jack/status/20":                    "/status/20",
		"https://twitter.com/jack/status/20?s=20&t=abc":   "/status/20",
		"https://mobile.twitter.com/jack/statuses/20":     "/status/20",
		"https://www.x.com/i/status/20":                   "/status/20",
		"https://x.com/i/web/status/20":                   "/status/20",
		"https://fixupx.com/jack/status/20/photo/1":       "/status/20",
		"https://vxtwitter.com/jack/status/20":            "/status/20",
		"https://X.COM/0xSoulProtocol":                    "/0xSoulProtocol",
		"https://x.com/jack/":                             "/jack",
		"https://x.com/home":                              "",
		"https://x.com/i/lists/1":                         "",
		"https://x.com/jack/likes":                        "",
		"https://x.com/":                                  "",
		"https://example.com/jack/status/20":              "",
		"https://notx.com/jack/status/20":                 "",
		"https://x.com/name_that_is_too_long/status/20":   "",
		"https://x.com/jack/status/20000000000000000000x": "",
	} {
		u, _ := url.Parse(raw)
		if got := twitterAPIPath(u); got != want {
			t.Errorf("%s: %q, want %q", raw, got, want)
		}
	}
}

func TestFetchTwitter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/status/20", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":200,"message":"OK","tweet":{"text":"just setting\nup my twttr",
			"author":{"name":"jack","screen_name":"jack","description":"bio"}}}`))
	})
	mux.HandleFunc("/status/30", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":200,"message":"OK","tweet":{"text":"","author":{"name":"Ann","screen_name":"ann"},
			"article":{"title":"How to scalp","preview_text":"A  long read"}}}`))
	})
	mux.HandleFunc("/jack", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write([]byte(`{"code":200,"message":"OK","user":{"name":"Jack D","screen_name":"jack","description":"no state"}}`))
	})
	mux.HandleFunc("/status/404", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		w.Write([]byte(`{"code":404,"message":"NOT_FOUND","tweet":null}`))
	})
	mux.HandleFunc("/status/401", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	})
	mux.HandleFunc("/status/500", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})
	var flaky, gone int // requests so far
	mux.HandleFunc("/flaky", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if flaky++; flaky == 1 {
			w.WriteHeader(404)
			w.Write([]byte(`{"code":404,"message":"User not found"}`))
			return
		}
		w.Write([]byte(`{"code":200,"message":"OK","user":{"name":"Soul Labs","screen_name":"flaky","description":"bio"}}`))
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, r *http.Request) {
		gone++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		w.Write([]byte(`{"code":404,"message":"User not found"}`))
	})
	mux.HandleFunc("/status/1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<title>FxTwitter</title>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	defer func(d time.Duration) { profileRetry = d }(profileRetry)
	profileRetry = time.Millisecond
	f := newFetcher(5*time.Second, allowAll)
	f.fxAPI = srv.URL
	ctx := context.Background()

	for _, tc := range []struct {
		url  string
		want Metadata
	}{
		{"https://x.com/jack/status/20?s=20", Metadata{"jack (@jack)", "just setting\nup my twttr"}},
		{"https://x.com/i/status/30", Metadata{"Ann (@ann)", "How to scalp\n\nA long read"}},
		{"https://twitter.com/jack", Metadata{"Jack D (@jack)", "no state"}},
		{"https://x.com/flaky", Metadata{"Soul Labs (@flaky)", "bio"}},
	} {
		if got, err := f.Fetch(ctx, tc.url); err != nil || got != tc.want {
			t.Errorf("%s: %+v, %v; want %+v", tc.url, got, err, tc.want)
		}
	}

	var be *BlockedError
	if _, err := f.Fetch(ctx, "https://x.com/i/status/401"); !errors.As(err, &be) {
		t.Errorf("private tweet: %v, want blocked", err)
	}
	for id, permanent := range map[string]bool{"404": true, "500": false, "1": false} {
		_, err := f.Fetch(ctx, "https://x.com/i/status/"+id)
		if err == nil || Permanent(err) != permanent {
			t.Errorf("status %s: %v, permanent %v; want permanent %v", id, err, Permanent(err), permanent)
		}
	}

	// A profile's 404 is asked twice and is never taken as a deleted account.
	if _, err := f.Fetch(ctx, "https://x.com/gone"); err == nil || Permanent(err) || gone != 2 {
		t.Errorf("missing profile: %v, permanent %v, %d requests; want a retryable error after 2", err, Permanent(err), gone)
	}
}

func TestKeepLines(t *testing.T) {
	for in, want := range map[string]string{
		"  one  two \n\n\n\nthree\n": "one two\n\nthree",
		"\n\nfirst\n  \nsecond":      "first\n\nsecond",
		"":                           "",
	} {
		if got := keepLines(100, in); got != want {
			t.Errorf("keepLines(%q) = %q, want %q", in, got, want)
		}
	}
}
