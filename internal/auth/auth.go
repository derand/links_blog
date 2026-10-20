// Package auth signs the blog's author in with Google and keeps the session
// in a signed cookie.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	sessionCookie = "session"
	stateCookie   = "oauth_state"
	stateTTL      = 10 * time.Minute
	googleIssuer  = "https://accounts.google.com"
	callbackPath  = "/oauth2callback" // registered in the Google console
)

type Config struct {
	ClientID     string
	ClientSecret string
	BaseURL      string   // public site URL, e.g. https://links.example.com
	Admins       []string // allowed emails
	Secret       []byte   // HMAC key for cookies, at least 32 bytes
	SessionTTL   time.Duration
}

type Auth struct {
	cfg    Config
	secure bool
	now    func() time.Time

	mu       sync.Mutex
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
}

func New(cfg Config) (*Auth, error) {
	if len(cfg.Secret) < 32 {
		return nil, errors.New("session secret must be at least 32 bytes")
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 30 * 24 * time.Hour
	}
	for i, e := range cfg.Admins {
		cfg.Admins[i] = strings.ToLower(strings.TrimSpace(e))
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Auth{cfg: cfg, secure: strings.HasPrefix(cfg.BaseURL, "https://"), now: time.Now}, nil
}

// LoginEnabled reports whether Google sign-in is configured.
func (a *Auth) LoginEnabled() bool {
	return a.cfg.ClientID != "" && a.cfg.ClientSecret != "" && a.cfg.BaseURL != "" && len(a.cfg.Admins) > 0
}

type ctxKey struct{}

// Middleware puts the signed-in author into the request context and extends
// the session when half of it has passed. Sessions without a Google subject
// (made before it was kept) are ignored, so the author signs in again.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var s session
		if c, err := r.Cookie(sessionCookie); err == nil && a.decode(c.Value, &s) == nil &&
			a.now().Unix() < s.Exp && s.Sub != "" && slices.Contains(a.cfg.Admins, s.Email) {
			if time.Unix(s.Exp, 0).Sub(a.now()) < a.cfg.SessionTTL/2 {
				http.SetCookie(w, a.SessionCookie(s.Email, s.Sub))
			}
			r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, s))
		}
		next.ServeHTTP(w, r)
	})
}

// User returns the signed-in author's email, or "" for a visitor.
func User(ctx context.Context) string {
	s, _ := ctx.Value(ctxKey{}).(session)
	return s.Email
}

// UserID returns the signed-in author's stable ID, "google:<sub>", or "" for
// a visitor. Unlike the email, it never changes for a Google account.
func UserID(ctx context.Context) string {
	s, _ := ctx.Value(ctxKey{}).(session)
	if s.Sub == "" {
		return ""
	}
	return "google:" + s.Sub
}

type session struct {
	Email string `json:"e"`
	Sub   string `json:"s"` // Google account ID, the ID token's subject
	Exp   int64  `json:"x"`
}

// SessionCookie returns a fresh session cookie for the Google account sub
// with the given email.
func (a *Auth) SessionCookie(email, sub string) *http.Cookie {
	exp := a.now().Add(a.cfg.SessionTTL)
	return a.cookie(sessionCookie, a.encode(session{Email: strings.ToLower(email), Sub: sub, Exp: exp.Unix()}), exp)
}

func (a *Auth) cookie(name, value string, exp time.Time) *http.Cookie {
	c := &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode}
	if exp.IsZero() {
		c.MaxAge = -1
	} else {
		c.Expires = exp
	}
	return c
}

// encode serializes v as base64(json).base64(hmac).
func (a *Auth) encode(v any) string {
	payload, _ := json.Marshal(v)
	p := base64.RawURLEncoding.EncodeToString(payload)
	return p + "." + base64.RawURLEncoding.EncodeToString(a.mac(p))
}

func (a *Auth) decode(s string, v any) error {
	p, sig, ok := strings.Cut(s, ".")
	if !ok {
		return errors.New("malformed cookie")
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, a.mac(p)) {
		return errors.New("bad signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, v)
}

func (a *Auth) mac(s string) []byte {
	h := hmac.New(sha256.New, a.cfg.Secret)
	h.Write([]byte(s))
	return h.Sum(nil)
}

func (a *Auth) provider(ctx context.Context) (*oauth2.Config, *oidc.IDTokenVerifier, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.oauth == nil {
		// Discovery is lazy so the site starts without reaching Google.
		p, err := oidc.NewProvider(ctx, googleIssuer)
		if err != nil {
			return nil, nil, err
		}
		a.verifier = p.Verifier(&oidc.Config{ClientID: a.cfg.ClientID})
		a.oauth = &oauth2.Config{
			ClientID:     a.cfg.ClientID,
			ClientSecret: a.cfg.ClientSecret,
			Endpoint:     p.Endpoint(),
			RedirectURL:  a.cfg.BaseURL + callbackPath,
			Scopes:       []string{oidc.ScopeOpenID, "email"},
		}
	}
	return a.oauth, a.verifier, nil
}

type loginState struct {
	State    string `json:"s"`
	Verifier string `json:"v"`
	Exp      int64  `json:"x"`
}

// Routes registers /login, /logout and the OAuth callback.
func (a *Auth) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", a.login)
	mux.HandleFunc("GET /logout", a.logout)
	mux.HandleFunc("GET "+callbackPath, a.callback)
}

func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	if User(r.Context()) != "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if !a.LoginEnabled() {
		http.NotFound(w, r)
		return
	}
	cfg, _, err := a.provider(r.Context())
	if err != nil {
		slog.Error("oidc discovery", "err", err)
		http.Error(w, "Sign-in is unavailable, try again later.", http.StatusBadGateway)
		return
	}
	st := loginState{State: rand.Text(), Verifier: oauth2.GenerateVerifier(), Exp: a.now().Add(stateTTL).Unix()}
	http.SetCookie(w, a.cookie(stateCookie, a.encode(st), a.now().Add(stateTTL)))
	http.Redirect(w, r, cfg.AuthCodeURL(st.State, oauth2.S256ChallengeOption(st.Verifier)), http.StatusFound)
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, a.cookie(sessionCookie, "", time.Time{}))
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *Auth) callback(w http.ResponseWriter, r *http.Request) {
	if !a.LoginEnabled() {
		http.NotFound(w, r)
		return
	}
	var st loginState
	c, err := r.Cookie(stateCookie)
	if err != nil || a.decode(c.Value, &st) != nil || a.now().Unix() > st.Exp ||
		!hmac.Equal([]byte(st.State), []byte(r.URL.Query().Get("state"))) {
		http.Error(w, "Sign-in expired, try again.", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, a.cookie(stateCookie, "", time.Time{}))
	if e := r.URL.Query().Get("error"); e != "" {
		http.Error(w, "Sign-in cancelled: "+e, http.StatusForbidden)
		return
	}

	cfg, verifier, err := a.provider(r.Context())
	if err != nil {
		slog.Error("oidc discovery", "err", err)
		http.Error(w, "Sign-in is unavailable, try again later.", http.StatusBadGateway)
		return
	}
	tok, err := cfg.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(st.Verifier))
	if err != nil {
		slog.Warn("oauth code exchange", "err", err)
		http.Error(w, "Sign-in failed.", http.StatusBadRequest)
		return
	}
	raw, _ := tok.Extra("id_token").(string)
	idt, err := verifier.Verify(r.Context(), raw)
	if err != nil {
		slog.Warn("id token", "err", err)
		http.Error(w, "Sign-in failed.", http.StatusBadRequest)
		return
	}
	var claims struct {
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
	}
	if err := idt.Claims(&claims); err != nil {
		http.Error(w, "Sign-in failed.", http.StatusBadRequest)
		return
	}
	email := strings.ToLower(claims.Email)
	if !claims.Verified || idt.Subject == "" || !slices.Contains(a.cfg.Admins, email) {
		slog.Warn("sign-in refused", "email", email)
		http.Error(w, "You don't have permissions on this site.", http.StatusForbidden)
		return
	}
	slog.Info("signed in", "email", email, "sub", idt.Subject)
	http.SetCookie(w, a.SessionCookie(email, idt.Subject))
	http.Redirect(w, r, "/", http.StatusFound)
}
