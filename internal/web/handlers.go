package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/derand/links_blog/internal/auth"
	"github.com/derand/links_blog/internal/links"
	"github.com/derand/links_blog/internal/store"
)

type postView struct {
	ID              int64
	URL, Host, Text string
	Full            string // the whole text when Text is cut
	Byline          string // a tweet's author when Text is the tweet
	Day             string
	Tags            []string
	Hidden          bool
	ShowDate        bool
	Editable        bool
}

func newPostView(l *store.Link, showDate, editable bool) postView {
	host := l.URL
	if u, err := url.Parse(l.URL); err == nil && u.Host != "" {
		host = u.Host
	}
	v := postView{ID: l.ID, URL: l.URL, Host: host, Text: l.Description, Day: l.Day, Tags: l.Tags, Hidden: l.Hidden,
		ShowDate: showDate, Editable: editable}
	if v.Text != "" {
		return v
	}
	// A tweet's title is only its author: show what it says instead.
	if links.Tweet(l.URL) {
		if text, cut := shortText(l.SiteDescription, maxPostText); text != "" {
			v.Text, v.Byline = text, tweetAuthor(l.Title)
			if cut {
				v.Full = l.SiteDescription
			}
			return v
		}
	}
	v.Text = l.Title
	return v
}

// maxPostText keeps a fetched text to about a line of the feed.
const maxPostText = 140

var (
	textURLRe   = regexp.MustCompile(`https?://\S+`)
	tweetNickRe = regexp.MustCompile(`\(@(\w+)\)$`)
)

// shortText returns the first line of s with text in it, URLs dropped, cut
// at a word to max runes, and whether anything was left out.
func shortText(s string, max int) (string, bool) {
	var line string
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && line == "" {
		line = strings.Join(strings.Fields(textURLRe.ReplaceAllString(lines[0], "")), " ")
		lines = lines[1:]
	}
	cut := strings.TrimSpace(textURLRe.ReplaceAllString(strings.Join(lines, " "), "")) != ""
	if r := []rune(line); len(r) > max {
		line = string(r[:max])
		if i := strings.LastIndexByte(line, ' '); i > 0 {
			line = line[:i]
		}
		cut = true
	}
	if cut {
		line = strings.TrimRight(line, " ,.:;-–—") + "…"
	}
	return line, cut
}

// tweetAuthor returns "@nick" from FxTwitter's "Name (@nick)" title.
func tweetAuthor(title string) string {
	if m := tweetNickRe.FindStringSubmatch(title); m != nil {
		return "@" + m[1]
	}
	return title
}

type dayView struct {
	Day, Human string
	Posts      []postView
}

func groupByDay(ls []*store.Link, editable bool) []dayView {
	var days []dayView
	for _, l := range ls {
		if len(days) == 0 || days[len(days)-1].Day != l.Day {
			human := l.Day
			if t, err := time.Parse(links.DayLayout, l.Day); err == nil {
				human = t.Format("02 Jan 2006")
			}
			days = append(days, dayView{Day: l.Day, Human: human})
		}
		d := &days[len(days)-1]
		d.Posts = append(d.Posts, newPostView(l, false, editable))
	}
	return days
}

type indexPage struct {
	page
	Days       []dayView
	OneDay     bool
	Pagination []pageLink
	AddForm    *linkForm // for the author
}

// pageParam returns the zero-based page from the 1-based ?p= parameter.
func pageParam(r *http.Request) int {
	p, err := strconv.Atoi(r.URL.Query().Get("p"))
	if err != nil || p < 1 {
		return 0
	}
	return p - 1
}

func pageCount(total, perPage int) int {
	return (total + perPage - 1) / perPage
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	withHidden := auth.User(ctx) != ""
	pg := pageParam(r)
	days, total, err := s.store.Days(ctx, pg*daysPerPage, daysPerPage, withHidden)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	pages := pageCount(total, daysPerPage)
	if pg > 0 && pg >= pages {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	ls, err := s.store.LinksOnDays(ctx, days, withHidden)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data := indexPage{page: s.page(r, ""), Days: groupByDay(ls, withHidden)}
	if withHidden {
		data.AddForm = s.newAddForm(ctx)
	}
	if pages > 1 {
		data.Pagination = pagination(pg, pages, 2, func(i int) string {
			if i == 0 {
				return "/"
			}
			return fmt.Sprintf("/index.html?p=%d", i+1)
		})
	}
	s.render(w, http.StatusOK, "index", data)
}

// file serves single-segment paths: /index.html and day pages /YYYY-MM-DD.html.
func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if name == "index.html" {
		s.index(w, r)
		return
	}
	day, ok := strings.CutSuffix(name, ".html")
	if t, err := time.Parse(links.DayLayout, day); !ok || err != nil || t.Format(links.DayLayout) != day {
		s.error(w, r, http.StatusNotFound, "Page not found.")
		return
	}
	s.day(w, r, day)
}

func (s *Server) day(w http.ResponseWriter, r *http.Request, day string) {
	ls, err := s.store.LinksOnDays(r.Context(), []string{day}, auth.User(r.Context()) != "")
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if len(ls) == 0 {
		s.error(w, r, http.StatusNotFound, "No links on this day.")
		return
	}
	signedIn := auth.User(r.Context()) != ""
	days := groupByDay(ls, signedIn)
	data := indexPage{page: s.page(r, days[0].Human), Days: days, OneDay: true}
	if signedIn {
		data.AddForm = s.newAddForm(r.Context())
		data.AddForm.Day = day
	}
	s.render(w, http.StatusOK, "index", data)
}

type searchPage struct {
	page
	Query      string
	Searched   bool
	Error      string
	Total      int
	Posts      []postView
	Pagination []pageLink
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	data := searchPage{page: s.page(r, "Search")}
	data.HideHeaderSearch = true
	qs, ok := r.URL.Query()["q"]
	if !ok {
		s.render(w, http.StatusOK, "search", data)
		return
	}
	data.Query = strings.TrimSpace(qs[0])
	data.Title = "Search: " + data.Query
	q, err := links.ParseQuery(data.Query)
	if err != nil {
		data.Error = "Can't parse the query: " + err.Error()
		s.render(w, http.StatusBadRequest, "search", data)
		return
	}

	pg := pageParam(r)
	ls, total, err := s.store.Search(r.Context(), q, pg*resultsPerPage, resultsPerPage, auth.User(r.Context()) != "")
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.Searched, data.Total = true, total
	for _, l := range ls {
		data.Posts = append(data.Posts, newPostView(l, true, auth.User(r.Context()) != ""))
	}
	if pages := pageCount(total, resultsPerPage); pages > 1 {
		base := "/search?q=" + url.QueryEscape(data.Query)
		data.Pagination = pagination(pg, pages, 2, func(i int) string {
			if i == 0 {
				return base
			}
			return fmt.Sprintf("%s&p=%d", base, i+1)
		})
	}
	s.render(w, http.StatusOK, "search", data)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		http.Error(w, "database error", http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("OK"))
}
