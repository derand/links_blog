package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/derand/links_blog/internal/auth"
	"github.com/derand/links_blog/internal/service"
	"github.com/derand/links_blog/internal/store"
)

const maxFormBytes = 64 << 10

// addForm holds the add-link form's values; Error and Existing explain why
// the last attempt did not add the link.
type addForm struct {
	URL, Description, Tags, Day string
	Hidden                      bool
	Error                       string
	Existing                    *postView
}

type addPage struct {
	page
	Form addForm
}

// requireUser answers 403 to anyone but the signed-in author.
func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) bool {
	if auth.User(r.Context()) != "" {
		return true
	}
	s.error(w, r, http.StatusForbidden, "Only the author can change links.")
	return false
}

// addPage shows the form; query parameters prefill it, so a bookmarklet can
// open /add?url=...&description=...
func (s *Server) addPage(w http.ResponseWriter, r *http.Request) {
	if !s.requireUser(w, r) {
		return
	}
	q := r.URL.Query()
	s.render(w, http.StatusOK, "add", addPage{page: s.page(r, "Add link"), Form: addForm{
		URL: q.Get("url"), Description: q.Get("description"), Tags: q.Get("tags"), Day: q.Get("day"),
	}})
}

// add adds a link from the form and shows it on its day's page.
func (s *Server) add(w http.ResponseWriter, r *http.Request) {
	if !s.requireUser(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		s.error(w, r, http.StatusBadRequest, "Can't read the form.")
		return
	}
	f := r.PostForm
	data := addPage{page: s.page(r, "Add link"), Form: addForm{
		URL:         strings.TrimSpace(f.Get("url")),
		Description: f.Get("description"),
		Tags:        f.Get("tags"),
		Day:         strings.TrimSpace(f.Get("day")),
		Hidden:      f.Get("hidden") != "",
	}}
	if data.Form.URL == "" {
		data.Form.Error = "URL is required."
		s.render(w, http.StatusBadRequest, "add", data)
		return
	}

	res, err := s.links.AddLink(r.Context(), service.NewLink{
		URL:         data.Form.URL,
		Description: data.Form.Description,
		Tags:        strings.Split(data.Form.Tags, ","),
		Hidden:      data.Form.Hidden,
		Day:         data.Form.Day,
	})
	switch {
	case errors.Is(err, service.ErrInvalid):
		data.Form.Error = "Can't add: " + strings.TrimPrefix(err.Error(), service.ErrInvalid.Error()+": ") + "."
		s.render(w, http.StatusBadRequest, "add", data)
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	case res.Duplicate != store.NotDup:
		pv := newPostView(res.Link, true)
		data.Form.Error = "Already posted:"
		data.Form.Existing = &pv
		s.render(w, http.StatusConflict, "add", data)
		return
	}
	http.Redirect(w, r, postURL(res.Link), http.StatusSeeOther)
}

// postURL is the link's place on its day's page.
func postURL(l *store.Link) string {
	return fmt.Sprintf("/%s.html#link-%d", l.Day, l.ID)
}
