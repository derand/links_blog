// Package web serves the blog's pages.
package web

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/derand/links_blog/internal/auth"
	"github.com/derand/links_blog/internal/service"
	"github.com/derand/links_blog/internal/store"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

const (
	daysPerPage    = 10
	resultsPerPage = 50
)

type Server struct {
	store   *store.Store
	links   service.LinkService
	auth    *auth.Auth
	loc     *time.Location
	pages   map[string]*template.Template
	version map[string]string // static file name -> content hash
}

// New returns the site's handler with all middleware applied. Pages read
// from st; the author's changes go through ls.
func New(st *store.Store, ls service.LinkService, a *auth.Auth, loc *time.Location) (http.Handler, error) {
	s := &Server{store: st, links: ls, auth: a, loc: loc, version: map[string]string{}}

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, err
	}
	err = fs.WalkDir(static, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(static, path)
		sum := sha256.Sum256(data)
		s.version[path] = hex.EncodeToString(sum[:4])
		return err
	})
	if err != nil {
		return nil, err
	}

	funcs := template.FuncMap{
		"static": func(name string) (string, error) {
			v, ok := s.version[name]
			if !ok {
				return "", fmt.Errorf("unknown static file %q", name)
			}
			return "/static/" + name + "?v=" + v, nil
		},
		"tagQuery": func(tag string) string {
			if strings.ContainsAny(tag, " \t") {
				tag = `"` + tag + `"`
			}
			return "tag:" + tag
		},
	}
	s.pages = map[string]*template.Template{}
	for _, name := range []string{"index", "search", "add", "edit", "error"} {
		t, err := template.New(name).Funcs(funcs).ParseFS(templatesFS, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, err
		}
		s.pages[name] = t
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /{file}", s.file)
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /add", s.addPage)
	mux.HandleFunc("POST /add", s.add)
	mux.HandleFunc("GET /edit/{id}", s.editPage)
	mux.HandleFunc("POST /edit/{id}", s.edit)
	mux.HandleFunc("POST /delete/{id}", s.delete)
	mux.Handle("GET /static/", http.StripPrefix("/static/", staticHandler(http.FileServerFS(static))))
	a.Routes(mux)

	var h http.Handler = mux
	h = a.Middleware(h)
	h = http.NewCrossOriginProtection().Handler(h)
	h = securityHeaders(h)
	h = logRequests(h)
	h = recoverPanics(h)
	return h, nil
}

// page holds the fields every page template uses.
type page struct {
	Title            string
	User             string
	Year             int
	HideHeaderSearch bool
}

func (s *Server) page(r *http.Request, title string) page {
	return page{Title: title, User: auth.User(r.Context()), Year: time.Now().In(s.loc).Year()}
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := s.pages[name].ExecuteTemplate(&buf, "layout", data); err != nil {
		slog.Error("render", "template", name, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

func (s *Server) error(w http.ResponseWriter, r *http.Request, status int, message string) {
	s.render(w, status, "error", struct {
		page
		Status  int
		Message string
	}{s.page(r, http.StatusText(status)), status, message})
}

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("request failed", "path", r.URL.Path, "err", err)
	s.error(w, r, http.StatusInternalServerError, "Something went wrong.")
}

func staticHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; "+
			"style-src 'self' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; "+
			"img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request", "method", r.Method, "path", r.URL.RequestURI(), "status", rec.status,
			"duration", time.Since(start).Round(time.Microsecond))
	})
}

func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.Error("panic", "path", r.URL.Path, "err", v)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
