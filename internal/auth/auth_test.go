package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTest(t *testing.T) *Auth {
	t.Helper()
	a, err := New(Config{
		ClientID: "id", ClientSecret: "secret", BaseURL: "https://links.example.com/",
		Admins: []string{" Me@Example.com "}, Secret: []byte(strings.Repeat("s", 32)),
		SessionTTL: 10 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// whoami runs the middleware and returns the user it saw and the response.
func whoami(a *Auth, cookies ...*http.Cookie) (string, *httptest.ResponseRecorder) {
	var user string
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user = User(r.Context())
	}))
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return user, rec
}

func TestNewValidates(t *testing.T) {
	if _, err := New(Config{Secret: []byte("short")}); err == nil {
		t.Error("short secret accepted")
	}
	a := newTest(t)
	if !a.LoginEnabled() || !a.secure {
		t.Errorf("LoginEnabled = %v, secure = %v", a.LoginEnabled(), a.secure)
	}
	if a.cfg.BaseURL != "https://links.example.com" || a.cfg.Admins[0] != "me@example.com" {
		t.Errorf("config not normalized: %+v", a.cfg)
	}
	b, _ := New(Config{Secret: []byte(strings.Repeat("s", 32))})
	if b.LoginEnabled() {
		t.Error("login enabled without Google client")
	}
}

func TestSession(t *testing.T) {
	a := newTest(t)
	c := a.SessionCookie("me@example.com")
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie flags: %+v", c)
	}
	if user, rec := whoami(a, c); user != "me@example.com" || rec.Header().Get("Set-Cookie") != "" {
		t.Errorf("fresh session: user %q, Set-Cookie %q", user, rec.Header().Get("Set-Cookie"))
	}
	if user, _ := whoami(a); user != "" {
		t.Errorf("no cookie: user %q", user)
	}

	// Tampered payload or signature.
	p, sig, _ := strings.Cut(c.Value, ".")
	other := a.encode(session{Email: "me@example.com", Exp: 1 << 40})
	op, _, _ := strings.Cut(other, ".")
	for _, v := range []string{op + "." + sig, p + "." + sig[1:] + "A", p, "garbage"} {
		if user, _ := whoami(a, &http.Cookie{Name: sessionCookie, Value: v}); user != "" {
			t.Errorf("tampered cookie %q accepted", v)
		}
	}

	// Signed with another secret.
	b, _ := New(Config{Admins: []string{"me@example.com"}, Secret: []byte(strings.Repeat("x", 32))})
	if user, _ := whoami(a, b.SessionCookie("me@example.com")); user != "" {
		t.Error("cookie from another secret accepted")
	}

	// Not an admin any more.
	if user, _ := whoami(a, a.SessionCookie("someone@example.com")); user != "" {
		t.Error("non-admin accepted")
	}
}

func TestSessionExpiryAndRenewal(t *testing.T) {
	a := newTest(t)
	start := time.Now()
	a.now = func() time.Time { return start }
	c := a.SessionCookie("me@example.com")

	a.now = func() time.Time { return start.Add(6 * 24 * time.Hour) }
	user, rec := whoami(a, c)
	if user == "" {
		t.Fatal("session expired too early")
	}
	if !strings.HasPrefix(rec.Header().Get("Set-Cookie"), "session=") {
		t.Error("session past half of its life was not renewed")
	}

	a.now = func() time.Time { return start.Add(11 * 24 * time.Hour) }
	if user, _ := whoami(a, c); user != "" {
		t.Error("expired session accepted")
	}
}

func TestCallbackRejectsBadState(t *testing.T) {
	a := newTest(t)
	mux := http.NewServeMux()
	a.Routes(mux)
	st := a.cookie(stateCookie, a.encode(loginState{State: "good", Verifier: "v", Exp: time.Now().Add(time.Minute).Unix()}), time.Time{})
	expired := a.cookie(stateCookie, a.encode(loginState{State: "good", Verifier: "v", Exp: time.Now().Add(-time.Minute).Unix()}), time.Time{})

	for name, tc := range map[string]struct {
		query  string
		cookie *http.Cookie
	}{
		"no cookie":   {"?state=good&code=x", nil},
		"wrong state": {"?state=bad&code=x", st},
		"expired":     {"?state=good&code=x", expired},
	} {
		req := httptest.NewRequest("GET", "/oauth2callback"+tc.query, nil)
		if tc.cookie != nil {
			req.AddCookie(tc.cookie)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, rec.Code)
		}
	}
}
