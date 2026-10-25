package meta

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		name, page string
		want       Metadata
	}{
		{"og wins", `<html><head><title>Plain</title>
			<meta name="description" content="plain desc">
			<meta property="og:title" content="OG &amp; title">
			<meta property="og:description" content="  OG
			  desc ">`, Metadata{"OG & title", "OG desc", ""}},
		{"twitter", `<head><title>Plain</title><meta name="twitter:title" content="TW">
			<meta name="twitter:description" content="tw desc"><meta name="description" content="plain">`,
			Metadata{"TW", "tw desc", ""}},
		{"plain", `<!doctype html><title> A  <b>title</b>
			</title><META NAME="Description" CONTENT="d">`, Metadata{"A <b>title</b>", "d", ""}},
		{"empty og falls back", `<title>T</title><meta property="og:title" content=" ">`, Metadata{"T", "", ""}},
		{"first meta wins", `<meta property="og:title" content="one"><meta property="og:title" content="two">`,
			Metadata{"one", "", ""}},
		{"stops at body", `<head><title>T</title></head><body><meta name="description" content="late">`,
			Metadata{"T", "", ""}},
		{"svg title in body ignored", `<body><svg><title>icon</title></svg>`, Metadata{}},
		{"long", `<title>` + strings.Repeat("я", 400) + `</title>`, Metadata{strings.Repeat("я", 299) + "…", "", ""}},
	} {
		if got, err := parse(strings.NewReader(tc.page)).metadata(nil); err != nil || got != tc.want {
			t.Errorf("%s: got %+v, %v, want %+v", tc.name, got, err, tc.want)
		}
	}
}

func TestGuardTitles(t *testing.T) {
	for _, tc := range []struct {
		url, page string
		want      string // title, or "blocked"
	}{
		{"https://www.reddit.com/r/flask/comments/1/x/", `<title>Reddit</title>`, "blocked"},
		{"https://www.facebook.com/permalink.php?story_fbid=1", `<title>Facebook</title>`, "blocked"},
		{"https://www.youtube.com/watch?v=x", `<title> - YouTube</title>`, "blocked"},
		{"https://arhivach.ng/thread/1/", `<title>arhivach.ng</title>`, "blocked"},
		{"https://example.com/a", `<title>Just a moment...</title>`, "blocked"},
		{"https://example.com/", `<title>Checking you are not a bot</title>`, "blocked"},
		{"https://trac.ffmpeg.org/wiki/x", `<title>Making sure you&#39;re not a bot!</title>`, "blocked"},
		{"https://pypi.python.org/pypi/Skype4Py/", `<title>Client Challenge</title>`, "blocked"},
		{"https://packages.debian.org/jessie/i386/fontconfig/filelist", `<title>I Challenge Thee</title>`, "blocked"},
		{"http://www.nightcoreuniverse.net/viewtopic.php?f=15", `<title>Bot Verification</title>`, "blocked"},
		{"http://www.gensokyo.org", `<title>Redirecting...</title>`, "blocked"},
		{"https://example.com/a", `<title>Redirecting stdout in Go</title>`, "Redirecting stdout in Go"},
		// The site's name is fine on its home page, and a better title wins.
		{"https://listen.moe/", `<title>LISTEN.moe</title>`, "LISTEN.moe"},
		{"https://help.ubuntu.ru/wiki/x", `<meta property="og:title" content="Ubuntu"><title>Настройка сети</title>`, "Настройка сети"},
		{"https://www.reddit.com/r/x/comments/1/y/", `<title>How to use Reddit</title>`, "How to use Reddit"},
	} {
		u, _ := url.Parse(tc.url)
		m, err := parse(strings.NewReader(tc.page)).metadata(u)
		var be *BlockedError
		switch {
		case tc.want == "blocked" && !errors.As(err, &be):
			t.Errorf("%s: %+v, %v; want blocked", tc.url, m, err)
		case tc.want != "blocked" && (err != nil || m.Title != tc.want):
			t.Errorf("%s: %+v, %v; want %q", tc.url, m, err, tc.want)
		}
	}
}

func allowAll(netip.Addr) bool { return true }

func TestFetch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.UserAgent(), "linksblog") {
			t.Errorf("User-Agent = %q", r.UserAgent())
		}
		w.Write([]byte(`<title>Page</title><meta property="og:description" content="About">`))
	})
	mux.HandleFunc("/cp1251", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=windows-1251")
		w.Write([]byte("<title>\xcf\xf0\xe8\xe2\xb3\xf2</title>")) // "Привіт"
	})
	mux.HandleFunc("/meta-charset", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<meta charset="windows-1251"><title>` + "\xcf\xf0\xe8\xe2\xb3\xf2</title>"))
	})
	mux.HandleFunc("/pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("<title>no</title>"))
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<head><!--" + strings.Repeat("x", maxBytes) + "--><title>late</title>"))
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Second)
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/page", http.StatusFound)
	})
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	})
	mux.HandleFunc("/challenge", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("/forbidden", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusForbidden)
	})
	mux.HandleFunc("/busy", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "later", http.StatusTooManyRequests)
	})
	mux.HandleFunc("/post/1", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/accounts/login/?next=/post/1", http.StatusFound)
	})
	mux.HandleFunc("/accounts/login/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<title>Log in</title>`))
	})
	mux.HandleFunc("/to-ftp", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "ftp://example.com/", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	f := newFetcher(300*time.Millisecond, allowAll)
	for path, want := range map[string]Metadata{
		"/page":         {"Page", "About", ""},
		"/redirect":     {"Page", "About", ""},
		"/cp1251":       {"Привіт", "", ""},
		"/meta-charset": {"Привіт", "", ""},
		"/pdf":          {},
		"/big":          {},
	} {
		got, err := f.Fetch(ctx, srv.URL+path)
		if err != nil || got != want {
			t.Errorf("%s: %+v, %v; want %+v", path, got, err, want)
		}
	}

	var se *StatusError
	if _, err := f.Fetch(ctx, srv.URL+"/missing"); !errors.As(err, &se) || se.Code != 404 || !Permanent(err) {
		t.Errorf("404: %v", err)
	}
	if _, err := f.Fetch(ctx, srv.URL+"/busy"); !errors.As(err, &se) || se.Code != 429 || Permanent(err) {
		t.Errorf("429: %v", err)
	}
	for _, path := range []string{"/challenge", "/forbidden", "/post/1"} {
		var be *BlockedError
		if _, err := f.Fetch(ctx, srv.URL+path); !errors.As(err, &be) || !Permanent(err) {
			t.Errorf("%s: %v, want BlockedError", path, err)
		}
	}
	// A login page asked for directly is just a page.
	if m, err := f.Fetch(ctx, srv.URL+"/accounts/login/"); err != nil || m.Title != "Log in" {
		t.Errorf("login page: %+v, %v", m, err)
	}
	for path, permanent := range map[string]bool{"/slow": false, "/loop": false, "/to-ftp": true} {
		if _, err := f.Fetch(ctx, srv.URL+path); err == nil || Permanent(err) != permanent {
			t.Errorf("%s: %v, permanent %v", path, err, Permanent(err))
		}
	}
	if _, err := f.Fetch(ctx, "https://no-such-host.invalid/"); err == nil || !Permanent(err) {
		t.Errorf("unknown host: %v, permanent %v", err, Permanent(err))
	}
	if _, err := f.Fetch(ctx, "file:///etc/passwd"); err == nil {
		t.Error("file: no error")
	}

	// The real fetcher refuses the local test server, directly and by name.
	real := New(time.Second)
	for _, u := range []string{srv.URL + "/page", strings.Replace(srv.URL, "127.0.0.1", "localhost", 1) + "/page"} {
		if _, err := real.Fetch(ctx, u); !errors.Is(err, ErrBlockedAddress) || !Permanent(err) {
			t.Errorf("%s: %v, want ErrBlockedAddress", u, err)
		}
	}
}

func TestPublicAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8":              true,
		"140.238.212.214":      true,
		"2a00:1450:4001::200e": true,
		"127.0.0.1":            false,
		"10.1.2.3":             false,
		"172.17.0.1":           false,
		"192.168.1.1":          false,
		"169.254.169.254":      false, // cloud metadata
		"100.64.0.1":           false,
		"0.0.0.0":              false,
		"255.255.255.255":      false,
		"::1":                  false,
		"fd00::1":              false,
		"fe80::1":              false,
		"64:ff9b::a00:1":       false,
		"2002:a00:1::":         false,
		"::ffff:127.0.0.1":     false,
	} {
		if got := publicAddr(netip.MustParseAddr(addr).Unmap()); got != want {
			t.Errorf("%s: %v, want %v", addr, got, want)
		}
	}
}
