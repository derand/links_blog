package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/derand/links_blog/internal/auth"
	"github.com/derand/links_blog/internal/links"
	"github.com/derand/links_blog/internal/store"
)

type postView struct {
	URL, Host, Text string
	Day             string
	Tags            []string
	Hidden          bool
	ShowDate        bool
}

func newPostView(l *store.Link, showDate bool) postView {
	host := l.URL
	if u, err := url.Parse(l.URL); err == nil && u.Host != "" {
		host = u.Host
	}
	text := l.Description
	if text == "" {
		text = l.Title
	}
	return postView{URL: l.URL, Host: host, Text: text, Day: l.Day, Tags: l.Tags, Hidden: l.Hidden, ShowDate: showDate}
}

type dayView struct {
	Day, Human string
	Posts      []postView
}

func groupByDay(ls []*store.Link) []dayView {
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
		d.Posts = append(d.Posts, newPostView(l, false))
	}
	return days
}

type indexPage struct {
	page
	Days       []dayView
	OneDay     bool
	Pagination []pageLink
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
	data := indexPage{page: s.page(r, ""), Days: groupByDay(ls)}
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
	days := groupByDay(ls)
	s.render(w, http.StatusOK, "index", indexPage{page: s.page(r, days[0].Human), Days: days, OneDay: true})
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
		data.Posts = append(data.Posts, newPostView(l, true))
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
