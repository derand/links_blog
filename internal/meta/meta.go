// Package meta fetches a web page's title and description for links that come
// without them. It only talks to public addresses: the blog's server must not
// become a way to reach its own network.
package meta

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/net/html/charset"
)

const (
	maxBytes     = 1 << 20 // read at most this much of a page
	maxRedirects = 5
	maxTitle     = 300 // runes
	maxDesc      = 1000
	userAgent    = "Mozilla/5.0 (compatible; linksblog; +https://github.com/derand/links_blog)"
)

// Metadata is what a page says about itself; fields are empty if it says nothing.
type Metadata struct {
	Title       string
	Description string
	Author      string // a tweet's author nick without "@"
}

// StatusError is returned when the page answers with a non-2xx status.
type StatusError struct{ Code int }

func (e *StatusError) Error() string { return fmt.Sprintf("http status %d", e.Code) }

// ErrBlockedAddress is returned for hosts that resolve to non-public addresses.
var ErrBlockedAddress = errors.New("address is not public")

var errScheme = errors.New("unsupported scheme")

// BlockedError means the site answered with a guard instead of the page: a bot
// check, a captcha or a login wall. Asking again won't get past it.
type BlockedError struct{ Reason string }

func (e *BlockedError) Error() string { return "blocked: " + e.Reason }

// Permanent reports whether a Fetch error will most likely repeat: the page is
// blocked or gone, or its host doesn't exist, isn't public or has a broken
// certificate. Timeouts, refused connections, 429 and 5xx are worth a retry.
func Permanent(err error) bool {
	var (
		blocked *BlockedError
		status  *StatusError
		dns     *net.DNSError
		cert    *tls.CertificateVerificationError
	)
	switch {
	case errors.As(err, &blocked), errors.Is(err, ErrBlockedAddress), errors.Is(err, errScheme), errors.As(err, &cert):
		return true
	case errors.As(err, &status):
		return status.Code >= 400 && status.Code < 500 && status.Code != 408 && status.Code != 429
	case errors.As(err, &dns):
		return dns.IsNotFound
	}
	return false
}

type Fetcher struct {
	client *http.Client
	fxAPI  string // FxTwitter API base, see twitter.go
}

// New returns a fetcher whose requests time out after timeout.
func New(timeout time.Duration) *Fetcher {
	return newFetcher(timeout, publicAddr)
}

// newFetcher lets tests allow local test servers.
func newFetcher(timeout time.Duration, allowed func(netip.Addr) bool) *Fetcher {
	dialer := &net.Dialer{
		Timeout: timeout,
		// Checked on the resolved address of every connection, redirects
		// included, so DNS answers can't point the request inside.
		Control: func(network, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return err
			}
			if !allowed(ap.Addr().Unmap()) {
				return fmt.Errorf("%w: %s", ErrBlockedAddress, ap.Addr())
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy:                 nil, // a proxy would be the only address checked
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
	}
	return &Fetcher{fxAPI: fxTwitterAPI, client: &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			return checkScheme(req.URL.Scheme)
		},
	}}
}

func checkScheme(scheme string) error {
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w %q", errScheme, scheme)
	}
	return nil
}

// Fetch downloads the page at url and returns its metadata. Pages that are not
// HTML have none. Guard pages instead of the content give a *BlockedError.
// Tweets and X profiles are asked from FxTwitter.
func (f *Fetcher) Fetch(ctx context.Context, url string) (Metadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Metadata{}, err
	}
	if err := checkScheme(req.URL.Scheme); err != nil {
		return Metadata{}, err
	}
	if p := twitterAPIPath(req.URL); p != "" {
		return f.fetchTwitter(ctx, p)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.1")
	req.Header.Set("Accept-Language", "en,uk;q=0.9,*;q=0.5")
	resp, err := f.client.Do(req)
	if err != nil {
		return Metadata{}, err
	}
	defer resp.Body.Close()
	if resp.Header.Get("Cf-Mitigated") == "challenge" {
		return Metadata{}, &BlockedError{"cloudflare challenge"}
	}
	switch code := resp.StatusCode; {
	case code == 401 || code == 403 || code == 418:
		return Metadata{}, &BlockedError{fmt.Sprintf("http status %d", code)}
	case code < 200 || code > 299:
		return Metadata{}, &StatusError{code}
	}
	final := resp.Request.URL // after redirects
	if loginPath.MatchString(final.Path) && !loginPath.MatchString(req.URL.Path) {
		return Metadata{}, &BlockedError{"redirected to a login page"}
	}
	ct := resp.Header.Get("Content-Type")
	if mt, _, err := mime.ParseMediaType(ct); ct != "" && (err != nil || mt != "text/html" && mt != "application/xhtml+xml") {
		return Metadata{}, nil
	}
	body, err := charset.NewReader(io.LimitReader(resp.Body, maxBytes), ct)
	if err != nil {
		return Metadata{}, err
	}
	return parse(body).metadata(final)
}

var (
	loginPath = regexp.MustCompile(`(?i)/(log-?in|sign-?in|auth|checkpoint)(/|\.|$)`)
	botCheck  = regexp.MustCompile(`(?i)^(just a moment|attention required|checking (you are not a bot|your browser)|` +
		`verifying you are human|are you a robot|one more step|ddos-guard|access denied)`)
)

// head is what a page's <head> says about it.
type head struct {
	ogTitle, twitterTitle, title string
	ogDesc, twitterDesc, desc    string
}

// metadata picks the page's title and description: og: and twitter: tags win
// over <title> and the plain description. On a page below the site's root,
// a title that is just the site's name (Reddit, Facebook) is a login wall or
// a JavaScript stub, and a bot-check title is a guard: both are errors.
func (h head) metadata(u *url.URL) (Metadata, error) {
	m := Metadata{Description: first(maxDesc, h.ogDesc, h.twitterDesc, h.desc)}
	deep := u != nil && (strings.Trim(u.Path, "/") != "" || u.RawQuery != "")
	var generic string
	for _, t := range []string{h.ogTitle, h.twitterTitle, h.title} {
		t = first(maxTitle, t)
		switch {
		case t == "":
			continue
		case botCheck.MatchString(t):
			return Metadata{}, &BlockedError{"bot check page: " + t}
		case deep && siteName(t, u.Hostname()):
			generic = t
			continue
		}
		m.Title = t
		return m, nil
	}
	if generic != "" {
		return Metadata{}, &BlockedError{"only the site's name as the title: " + generic}
	}
	return m, nil
}

// siteName reports whether title is nothing but the site's name: "Reddit"
// for www.reddit.com, "- YouTube", "arhivach.ng".
func siteName(title, host string) bool {
	alnum := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return unicode.ToLower(r)
			}
			return -1
		}, s)
	}
	t := alnum(title)
	if t == "" {
		return false
	}
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	labels := strings.Split(host, ".")
	return t == alnum(host) || len(labels) >= 2 && t == labels[len(labels)-2]
}

// parse reads the page's <head>.
func parse(r io.Reader) head {
	var (
		z       = html.NewTokenizer(r)
		found   = map[string]string{}
		title   strings.Builder
		inTitle bool
	)
loop:
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			break loop
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch atom.Lookup(name) {
			case atom.Body:
				break loop
			case atom.Title:
				inTitle = tt == html.StartTagToken && title.Len() == 0
			case atom.Meta:
				var key, content string
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					switch string(k) {
					case "property", "name":
						key = strings.ToLower(strings.TrimSpace(string(v)))
					case "content":
						content = string(v)
					}
				}
				if _, ok := found[key]; !ok && key != "" {
					found[key] = content
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch atom.Lookup(name) {
			case atom.Head:
				break loop
			case atom.Title:
				inTitle = false
			}
		case html.TextToken:
			if inTitle {
				title.Write(z.Text())
			}
		}
	}
	return head{
		ogTitle: found["og:title"], twitterTitle: found["twitter:title"], title: title.String(),
		ogDesc: found["og:description"], twitterDesc: found["twitter:description"], desc: found["description"],
	}
}

// first returns the first value that is not blank, with whitespace collapsed
// and cut to max runes.
func first(max int, values ...string) string {
	for _, v := range values {
		v = strings.Join(strings.Fields(strings.ToValidUTF8(v, "")), " ")
		if v == "" {
			continue
		}
		if utf8.RuneCountInString(v) > max {
			v = string([]rune(v)[:max-1]) + "…"
		}
		return v
	}
	return ""
}

// Non-public ranges that netip's predicates don't cover.
var blocked = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),   // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // documentation
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // documentation
	netip.MustParsePrefix("203.0.113.0/24"),  // documentation
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, broadcast
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64: may map to private IPv4
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001:db8::/32"), // documentation
	netip.MustParsePrefix("2002::/16"),     // 6to4: may map to private IPv4
}

func publicAddr(a netip.Addr) bool {
	if !a.IsValid() || a.IsUnspecified() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() ||
		a.IsLinkLocalMulticast() || a.IsInterfaceLocalMulticast() || a.IsMulticast() {
		return false
	}
	for _, p := range blocked {
		if p.Contains(a) {
			return false
		}
	}
	return true
}
