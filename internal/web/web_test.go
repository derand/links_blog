package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/derand/links_blog/internal/auth"
	"github.com/derand/links_blog/internal/service"
	"github.com/derand/links_blog/internal/store"
)

const admin = "me@example.com"

type site struct {
	t    *testing.T
	h    http.Handler
	auth *auth.Auth
}

// newSite serves 12 visible days (2020-01-01..12, one link each), plus a
// hidden link on 2020-01-12 and a hidden-only day 2019-12-31.
func newSite(t *testing.T) *site {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	var ls []*store.Link
	add := func(day, url, desc string, hidden bool, tags ...string) {
		ls = append(ls, &store.Link{URL: url, URLNorm: url, Description: desc, Day: day,
			CreatedAt: time.Now(), Hidden: hidden, Tags: tags})
	}
	for i := 1; i <= 12; i++ {
		add(fmt.Sprintf("2020-01-%02d", i), fmt.Sprintf("https://site%d.example.com/page", i),
			fmt.Sprintf("Visible post %d", i), false, "common")
	}
	add("2020-01-12", "https://secret.example.com", "Secret neighbour", true, "common")
	add("2019-12-31", "https://alone.example.com", "Secret alone", true)
	add("2019-12-30", "https://www.tagged.example.com:8443/x", "Spaces <b>escaped</b>", false, "neural network")
	if err := st.InsertLinks(ctx, ls); err != nil {
		t.Fatal(err)
	}

	a, err := auth.New(auth.Config{Admins: []string{admin}, Secret: []byte(strings.Repeat("k", 32))})
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(st, service.New(st, time.UTC), a, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	return &site{t: t, h: h, auth: a}
}

type resp struct {
	code int
	body string
	hdr  http.Header
}

func (s *site) get(path string, signedIn bool) resp {
	s.t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if signedIn {
		req.AddCookie(s.auth.SessionCookie(admin))
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	return resp{rec.Code, rec.Body.String(), rec.Header()}
}

func (r resp) has(t *testing.T, path string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(r.body, w) {
			t.Errorf("%s: body has no %q", path, w)
		}
	}
}

func (r resp) hasNot(t *testing.T, path string, unwanted ...string) {
	t.Helper()
	for _, w := range unwanted {
		if strings.Contains(r.body, w) {
			t.Errorf("%s: body has %q", path, w)
		}
	}
}

func TestIndex(t *testing.T) {
	s := newSite(t)

	r := s.get("/", false)
	if r.code != 200 {
		t.Fatalf("/: %d", r.code)
	}
	r.has(t, "/", "Visible post 12", "Visible post 3", "12 Jan 2020", `href="/2020-01-12.html"`,
		`<a class="link" href="https://site12.example.com/page">site12.example.com</a>`,
		`href="/search?q=tag%3acommon"`, `href="/index.html?p=2"`)
	r.hasNot(t, "/", "Visible post 2<", "Secret", "log out")

	// Same page under the old URL.
	if r2 := s.get("/index.html", false); r2.code != 200 || r2.body != r.body {
		t.Errorf("/index.html differs from /")
	}

	r = s.get("/index.html?p=2", false)
	r.has(t, "p=2", "Visible post 2<", "Visible post 1<", "Spaces &lt;b&gt;escaped&lt;/b&gt;",
		"tagged.example.com:8443", `href="/search?q=tag%3a%22neural%20network%22"`)
	r.hasNot(t, "p=2", "Visible post 3<")

	for _, p := range []string{"/?p=3", "/index.html?p=99"} {
		if r := s.get(p, false); r.code != http.StatusFound || r.hdr.Get("Location") != "/" {
			t.Errorf("%s: %d -> %q, want redirect to /", p, r.code, r.hdr.Get("Location"))
		}
	}
	if r := s.get("/?p=abc", false); r.code != 200 {
		t.Errorf("/?p=abc: %d", r.code)
	}
}

func TestIndexSignedIn(t *testing.T) {
	s := newSite(t)
	r := s.get("/", true)
	r.has(t, "/", "Secret neighbour", `class="post hidden"`, admin, "log out")

	// The hidden-only day adds a day: page 2 now has 3 days.
	r = s.get("/index.html?p=2", true)
	r.has(t, "p=2", "Secret alone", "Visible post 1<", "Spaces")
}

func TestDayPage(t *testing.T) {
	s := newSite(t)

	r := s.get("/2020-01-05.html", false)
	if r.code != 200 {
		t.Fatalf("day: %d", r.code)
	}
	r.has(t, "day", `<h1 class="day-title">05 Jan 2020</h1>`, "Visible post 5", "<title>Links blog - 05 Jan 2020</title>")
	r.hasNot(t, "day", "Visible post 4", `class="pagination"`)

	for _, p := range []string{"/2020-02-30.html", "/2020-1-5.html", "/2020-01-05", "/2021-01-01.html", "/foo", "/2019-12-31.html"} {
		if r := s.get(p, false); r.code != 404 {
			t.Errorf("%s: %d, want 404", p, r.code)
		}
	}
	if r := s.get("/2019-12-31.html", true); r.code != 200 {
		t.Errorf("hidden-only day for the author: %d", r.code)
	}
}

func TestSearchPage(t *testing.T) {
	s := newSite(t)

	r := s.get("/search", false)
	if r.code != 200 {
		t.Fatalf("/search: %d", r.code)
	}
	r.has(t, "/search", "tag:name")
	r.hasNot(t, "/search", "Search results", `class="header-search"`)

	r = s.get("/search?q=tag:common", false)
	r.has(t, "tag", "Search results: 12", `<span class="date"><a href="/2020-01-12.html">2020-01-12</a></span>`, `value="tag:common"`)
	r.hasNot(t, "tag", "Secret")
	if r := s.get("/search?q=tag:common", true); !strings.Contains(r.body, "Search results: 13") {
		t.Error("signed in: hidden link not found")
	}

	r = s.get("/search?q="+urlq(`tag:"neural network"`), false)
	r.has(t, "phrase tag", "Search results: 1", "tagged.example.com")

	r = s.get("/search?q=visible+post+1", false)
	r.has(t, "text", "Search results: 4", "Visible post 12", "Visible post 1<") // prefix "1" matches 1, 10, 11, 12

	r = s.get("/search?q=", false)
	r.has(t, "empty", "Search results: 13", "Visible post 12")

	r = s.get("/search?q="+urlq(`a "b`), false)
	if r.code != 400 {
		t.Errorf("bad query: %d", r.code)
	}
	r.has(t, "bad query", "unclosed quote")

	r = s.get("/search?q=date:2020-01-0", false)
	if r.code != 400 {
		t.Errorf("bad date: %d", r.code)
	}

	r = s.get("/search?q=nothing-like-this", false)
	r.has(t, "none", "Search results: 0")
}

func TestHealthAndStatic(t *testing.T) {
	s := newSite(t)
	if r := s.get("/health", false); r.code != 200 || r.body != "OK" {
		t.Errorf("/health: %d %q", r.code, r.body)
	}

	idx := s.get("/", false)
	i := strings.Index(idx.body, `/static/style.css?v=`)
	if i < 0 {
		t.Fatal("no versioned stylesheet link")
	}
	href := idx.body[i : i+strings.IndexByte(idx.body[i:], '"')]
	r := s.get(href, false)
	if r.code != 200 || !strings.Contains(r.hdr.Get("Content-Type"), "text/css") ||
		!strings.Contains(r.hdr.Get("Cache-Control"), "immutable") {
		t.Errorf("%s: %d %q %q", href, r.code, r.hdr.Get("Content-Type"), r.hdr.Get("Cache-Control"))
	}
	if r := s.get("/static/missing.css", false); r.code != 404 {
		t.Errorf("missing static: %d", r.code)
	}
	if csp := idx.hdr.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP = %q", csp)
	}
}

func TestAuthRoutes(t *testing.T) {
	s := newSite(t)
	// Google sign-in is not configured in tests.
	if r := s.get("/login", false); r.code != 404 {
		t.Errorf("/login: %d, want 404", r.code)
	}
	if r := s.get("/login", true); r.code != http.StatusFound {
		t.Errorf("/login signed in: %d, want redirect", r.code)
	}
	r := s.get("/logout", true)
	if r.code != http.StatusFound || !strings.Contains(r.hdr.Get("Set-Cookie"), "session=;") {
		t.Errorf("/logout: %d, Set-Cookie %q", r.code, r.hdr.Get("Set-Cookie"))
	}
}

func urlq(s string) string {
	return strings.NewReplacer(" ", "+", `"`, "%22", ":", "%3A").Replace(s)
}

func (s *site) post(path string, form url.Values, signedIn bool, hdr ...string) resp {
	s.t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	if signedIn {
		req.AddCookie(s.auth.SessionCookie(admin))
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	return resp{rec.Code, rec.Body.String(), rec.Header()}
}

func TestAddForm(t *testing.T) {
	s := newSite(t)

	s.get("/", false).hasNot(t, "/ signed out", `action="/add"`, "add link")
	s.get("/", true).has(t, "/ signed in", `<details class="add-link">`, `action="/add"`, `name="day" type="date" value=""`, `href="/add"`)
	s.get("/2020-01-05.html", true).has(t, "day", `name="day" type="date" value="2020-01-05"`)

	if r := s.get("/add", false); r.code != http.StatusForbidden {
		t.Errorf("GET /add signed out: %d", r.code)
	}
	r := s.get("/add?url=example.org/x&description=Hi+there", true)
	r.has(t, "GET /add", `value="example.org/x"`, ">Hi there</textarea>")

	form := url.Values{"url": {" example.org/new "}, "description": {"New <thing>"}, "tags": {"go, Common ,,go"},
		"day": {"2020-01-05"}, "hidden": {"on"}}
	if r := s.post("/add", form, false); r.code != http.StatusForbidden {
		t.Errorf("POST /add signed out: %d", r.code)
	}
	if r := s.post("/add", form, true, "Sec-Fetch-Site", "cross-site"); r.code != http.StatusForbidden {
		t.Errorf("POST /add cross-site: %d", r.code)
	}

	r = s.post("/add", form, true)
	loc := r.hdr.Get("Location")
	if r.code != http.StatusSeeOther || !strings.HasPrefix(loc, "/2020-01-05.html#link-") {
		t.Fatalf("POST /add: %d -> %q", r.code, loc)
	}
	id := strings.TrimPrefix(loc, "/2020-01-05.html#")
	day := s.get("/2020-01-05.html", true)
	day.has(t, "added", `class="post hidden" id="`+id+`"`, `href="https://example.org/new">example.org</a>`,
		"New &lt;thing&gt;", `href="/search?q=tag%3ago"`, `>common</a>`)
	day.hasNot(t, "added", ">Common</a>")
	s.get("/2020-01-05.html", false).hasNot(t, "hidden", "New &lt;thing&gt;")

	r = s.post("/add", url.Values{"url": {"http://www.example.org/new/"}}, true)
	if r.code != http.StatusConflict {
		t.Errorf("duplicate: %d", r.code)
	}
	r.has(t, "duplicate", "Already posted:", `<a href="/2020-01-05.html">2020-01-05</a>`, `value="http://www.example.org/new/"`)

	for _, f := range []url.Values{{"url": {""}}, {"url": {"ftp://example.org"}}, {"url": {"example.org/y"}, "day": {"2020-13-01"}}} {
		r := s.post("/add", f, true)
		if r.code != http.StatusBadRequest {
			t.Errorf("%v: %d", f, r.code)
		}
		r.has(t, fmt.Sprint(f), `class="form-message error"`)
	}
}

func TestEditAndDelete(t *testing.T) {
	s := newSite(t)

	s.get("/", false).hasNot(t, "/ signed out", `class="edit"`)
	r := s.get("/2020-01-05.html", true)
	i := strings.Index(r.body, `<a class="edit" href="/edit/`)
	if i < 0 {
		t.Fatal("no edit link for the author")
	}
	edit := r.body[i+len(`<a class="edit" href="`):]
	edit = edit[:strings.IndexByte(edit, '"')]
	id := strings.TrimPrefix(edit, "/edit/")

	for _, p := range []string{"/edit/99999", "/edit/abc"} {
		if r := s.get(p, true); r.code != 404 {
			t.Errorf("%s: %d", p, r.code)
		}
	}
	if r := s.get(edit, false); r.code != http.StatusForbidden {
		t.Errorf("GET %s signed out: %d", edit, r.code)
	}
	r = s.get(edit, true)
	r.has(t, "edit page", `action="`+edit+`"`, `value="https://site5.example.com/page"`, ">Visible post 5</textarea>",
		`name="tags" value="common"`, `type="date" value="2020-01-05" required`, `name="title"`, ">Save</button>",
		`action="/delete/`+id+`"`, "Page metadata: waiting.")

	form := url.Values{"url": {"https://site5.example.com/other"}, "description": {"Edited post"},
		"note": {"Full zanzibar text"}, "title": {"Page T"}, "tags": {"common, more"}, "day": {"2020-01-06"}}
	if r := s.post(edit, form, false); r.code != http.StatusForbidden {
		t.Errorf("POST %s signed out: %d", edit, r.code)
	}
	r = s.post(edit, form, true)
	if loc := r.hdr.Get("Location"); r.code != http.StatusSeeOther || loc != "/2020-01-06.html#link-"+id {
		t.Fatalf("POST %s: %d -> %q", edit, r.code, loc)
	}
	if r := s.get("/2020-01-05.html", false); r.code != 404 {
		t.Errorf("old day still has the post: %d", r.code)
	}
	s.get("/2020-01-06.html", false).has(t, "moved", "Edited post", `href="https://site5.example.com/other"`, ">more</a>")
	s.get("/search?q=edited", false).has(t, "search", "Search results: 1")
	// The note is searched and edited, not shown.
	s.get("/2020-01-06.html", false).hasNot(t, "note in the feed", "zanzibar")
	s.get("/search?q=zanzibar", false).has(t, "note search", "Search results: 1")
	s.get(edit, true).has(t, "note on the edit page", ">Full zanzibar text</textarea>")

	// Taking another link's URL; a missing day.
	s.post("/add", url.Values{"url": {"https://taken.example.org"}, "description": {"Taken one"}}, true)
	r = s.post(edit, url.Values{"url": {"www.taken.example.org/"}, "day": {"2020-01-06"}}, true)
	if r.code != http.StatusConflict {
		t.Errorf("taken URL: %d", r.code)
	}
	r.has(t, "taken URL", "Already posted:", "Taken one", `value="www.taken.example.org/"`)
	if r := s.post(edit, url.Values{"url": {"site5.example.com"}}, true); r.code != http.StatusBadRequest {
		t.Errorf("no day: %d", r.code)
	}

	del := "/delete/" + id
	if r := s.post(del, url.Values{"confirm": {"on"}}, false); r.code != http.StatusForbidden {
		t.Errorf("delete signed out: %d", r.code)
	}
	if r := s.post(del, nil, true); r.code != http.StatusBadRequest {
		t.Errorf("delete without confirm: %d", r.code)
	}
	r = s.post(del, url.Values{"confirm": {"on"}}, true)
	if loc := r.hdr.Get("Location"); r.code != http.StatusSeeOther || loc != "/2020-01-06.html" {
		t.Errorf("delete: %d -> %q", r.code, loc)
	}
	s.get("/2020-01-06.html", true).hasNot(t, "deleted", "Edited post")
	if r := s.post(del, url.Values{"confirm": {"on"}}, true); r.code != 404 {
		t.Errorf("delete twice: %d", r.code)
	}
	s.get("/search?q=edited", true).has(t, "search after delete", "Search results: 0")

	// The last link of a day: back to the feed.
	r = s.get("/2019-12-31.html", true)
	i = strings.Index(r.body, `href="/edit/`)
	alone := r.body[i+len(`href="/edit/`):]
	alone = alone[:strings.IndexByte(alone, '"')]
	r = s.post("/delete/"+alone, url.Values{"confirm": {"on"}}, true)
	if loc := r.hdr.Get("Location"); loc != "/" {
		t.Errorf("delete the last link of a day: -> %q", loc)
	}
}
