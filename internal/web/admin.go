package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/derand/links_blog/internal/auth"
	"github.com/derand/links_blog/internal/service"
	"github.com/derand/links_blog/internal/store"
)

const maxFormBytes = 64 << 10

// linkForm holds the add or edit form's values; Error and Existing explain
// why the last attempt was not saved.
type linkForm struct {
	Action, Submit              string
	Edit                        bool // editing: day is required, page metadata is shown
	URL, Description, Tags, Day string
	Note                        string
	Title, SiteDescription      string
	Hidden                      bool
	Error                       string
	Existing                    *postView
}

func newAddForm() *linkForm {
	return &linkForm{Action: "/add", Submit: "Add link"}
}

type addPage struct {
	page
	Form *linkForm
}

type editPage struct {
	page
	Form             *linkForm
	ID               int64
	Created, Updated string
	Source           string
	Meta             string // state of fetching the page's metadata
}

// requireUser answers 403 to anyone but the signed-in author.
func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) bool {
	if auth.User(r.Context()) != "" {
		return true
	}
	s.error(w, r, http.StatusForbidden, "Only the author can change links.")
	return false
}

// parseForm reads the form fields shared by adding and editing.
func (s *Server) parseForm(w http.ResponseWriter, r *http.Request, f *linkForm) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		s.error(w, r, http.StatusBadRequest, "Can't read the form.")
		return false
	}
	v := r.PostForm
	f.URL = strings.TrimSpace(v.Get("url"))
	f.Description = v.Get("description")
	f.Note = v.Get("note")
	f.Title = v.Get("title")
	f.SiteDescription = v.Get("site_description")
	f.Tags = v.Get("tags")
	f.Day = strings.TrimSpace(v.Get("day"))
	f.Hidden = v.Get("hidden") != ""
	return true
}

// saveFailed shows why a link was not saved: bad input, a duplicate or a server
// error. It reports false if there was nothing to show.
func (s *Server) saveFailed(w http.ResponseWriter, r *http.Request, name string, data any, f *linkForm,
	res service.AddResult, err error) bool {
	switch {
	case f.URL == "":
		f.Error = "URL is required."
		s.render(w, http.StatusBadRequest, name, data)
	case errors.Is(err, service.ErrNotFound):
		s.error(w, r, http.StatusNotFound, "No such link.")
	case errors.Is(err, service.ErrInvalid):
		f.Error = "Can't save: " + strings.TrimPrefix(err.Error(), service.ErrInvalid.Error()+": ") + "."
		s.render(w, http.StatusBadRequest, name, data)
	case err != nil:
		s.serverError(w, r, err)
	case res.Duplicate != store.NotDup:
		pv := newPostView(res.Link, true, true)
		f.Error = "Already posted:"
		f.Existing = &pv
		s.render(w, http.StatusConflict, name, data)
	default:
		return false
	}
	return true
}

// addPage shows the form; query parameters prefill it, so a bookmarklet can
// open /add?url=...&description=...
func (s *Server) addPage(w http.ResponseWriter, r *http.Request) {
	if !s.requireUser(w, r) {
		return
	}
	q := r.URL.Query()
	f := newAddForm()
	f.URL, f.Description, f.Tags, f.Day = q.Get("url"), q.Get("description"), q.Get("tags"), q.Get("day")
	s.render(w, http.StatusOK, "add", addPage{page: s.page(r, "Add link"), Form: f})
}

// add adds a link from the form and shows it on its day's page.
func (s *Server) add(w http.ResponseWriter, r *http.Request) {
	if !s.requireUser(w, r) {
		return
	}
	data := addPage{page: s.page(r, "Add link"), Form: newAddForm()}
	f := data.Form
	if !s.parseForm(w, r, f) {
		return
	}
	var (
		res service.AddResult
		err error
	)
	if f.URL != "" {
		res, err = s.links.AddLink(r.Context(), service.NewLink{
			URL:         f.URL,
			Description: f.Description,
			Tags:        strings.Split(f.Tags, ","),
			Hidden:      f.Hidden,
			Day:         f.Day,
		})
	}
	if s.saveFailed(w, r, "add", data, f, res, err) {
		return
	}
	http.Redirect(w, r, postURL(res.Link), http.StatusSeeOther)
}

// linkParam loads the link named by the {id} path segment; it answers 404 if
// there is none.
func (s *Server) linkParam(w http.ResponseWriter, r *http.Request) (*store.Link, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.error(w, r, http.StatusNotFound, "No such link.")
		return nil, false
	}
	l, err := s.links.Link(r.Context(), id)
	switch {
	case errors.Is(err, service.ErrNotFound):
		s.error(w, r, http.StatusNotFound, "No such link.")
		return nil, false
	case err != nil:
		s.serverError(w, r, err)
		return nil, false
	}
	return l, true
}

func (s *Server) newEditPage(r *http.Request, l *store.Link) editPage {
	const layout = "2006-01-02 15:04"
	return editPage{
		page:    s.page(r, "Edit link"),
		Form:    &linkForm{Action: fmt.Sprintf("/edit/%d", l.ID), Submit: "Save", Edit: true},
		ID:      l.ID,
		Created: l.CreatedAt.In(s.loc).Format(layout),
		Updated: l.UpdatedAt.In(s.loc).Format(layout),
		Source:  l.Source,
		Meta:    s.metaState(l),
	}
}

func (s *Server) metaState(l *store.Link) string {
	when := l.MetaCheckedAt.In(s.loc).Format("2006-01-02 15:04")
	switch l.MetaStatus {
	case store.MetaPending:
		if l.Title != "" && l.SiteDescription != "" {
			return ""
		}
		return "Page metadata: waiting."
	case store.MetaOK:
		return "Page metadata: fetched " + when + "."
	case store.MetaBlocked:
		return "Page metadata: the site shows a guard instead of the page (" + when + ", " + l.MetaError + "); not retried."
	case store.MetaDead:
		return "Page metadata: the page is gone (" + when + ", " + l.MetaError + "); not retried."
	}
	return fmt.Sprintf("Page metadata: failed %d time(s), last %s: %s.", l.MetaFailures, when, l.MetaError)
}

func (s *Server) editPage(w http.ResponseWriter, r *http.Request) {
	if !s.requireUser(w, r) {
		return
	}
	l, ok := s.linkParam(w, r)
	if !ok {
		return
	}
	data := s.newEditPage(r, l)
	f := data.Form
	f.URL, f.Description, f.Tags, f.Day = l.URL, l.Description, strings.Join(l.Tags, ", "), l.Day
	f.Note, f.Title, f.SiteDescription, f.Hidden = l.Note, l.Title, l.SiteDescription, l.Hidden
	s.render(w, http.StatusOK, "edit", data)
}

// edit saves the edited link and shows it on its day's page.
func (s *Server) edit(w http.ResponseWriter, r *http.Request) {
	if !s.requireUser(w, r) {
		return
	}
	l, ok := s.linkParam(w, r)
	if !ok {
		return
	}
	data := s.newEditPage(r, l)
	f := data.Form
	if !s.parseForm(w, r, f) {
		return
	}
	var (
		res service.AddResult
		err error
	)
	if f.URL != "" {
		res, err = s.links.UpdateLink(r.Context(), service.LinkEdit{
			ID:              l.ID,
			URL:             f.URL,
			Description:     f.Description,
			Note:            f.Note,
			Title:           f.Title,
			SiteDescription: f.SiteDescription,
			Tags:            strings.Split(f.Tags, ","),
			Hidden:          f.Hidden,
			Day:             f.Day,
		})
	}
	if s.saveFailed(w, r, "edit", data, f, res, err) {
		return
	}
	http.Redirect(w, r, postURL(res.Link), http.StatusSeeOther)
}

// delete deletes a link and goes back to its day, or to the feed if the day
// is now empty.
func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	if !s.requireUser(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.error(w, r, http.StatusNotFound, "No such link.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil || r.PostForm.Get("confirm") == "" {
		s.error(w, r, http.StatusBadRequest, "Deleting needs a confirmation.")
		return
	}
	l, err := s.links.DeleteLink(r.Context(), id)
	switch {
	case errors.Is(err, service.ErrNotFound):
		s.error(w, r, http.StatusNotFound, "No such link.")
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	}
	next := "/"
	if rest, err := s.store.LinksOnDays(r.Context(), []string{l.Day}, true); err == nil && len(rest) > 0 {
		next = "/" + l.Day + ".html"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// postURL is the link's place on its day's page.
func postURL(l *store.Link) string {
	return fmt.Sprintf("/%s.html#link-%d", l.Day, l.ID)
}
