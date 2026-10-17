// Package meta fetches a web page's title and description for links that come
// without them. It only talks to public addresses: the blog's server must not
// become a way to reach its own network.
package meta

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"syscall"
	"time"
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
}

// StatusError is returned when the page answers with a non-2xx status.
type StatusError struct{ Code int }

func (e *StatusError) Error() string { return fmt.Sprintf("http status %d", e.Code) }

// ErrBlockedAddress is returned for hosts that resolve to non-public addresses.
var ErrBlockedAddress = errors.New("address is not public")

type Fetcher struct {
	client *http.Client
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
	return &Fetcher{client: &http.Client{
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
		return fmt.Errorf("unsupported scheme %q", scheme)
	}
	return nil
}

// Fetch downloads the page at url and returns its metadata. Pages that are not
// HTML have none.
func (f *Fetcher) Fetch(ctx context.Context, url string) (Metadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Metadata{}, err
	}
	if err := checkScheme(req.URL.Scheme); err != nil {
		return Metadata{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.1")
	req.Header.Set("Accept-Language", "en,uk;q=0.9,*;q=0.5")
	resp, err := f.client.Do(req)
	if err != nil {
		return Metadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Metadata{}, &StatusError{resp.StatusCode}
	}
	ct := resp.Header.Get("Content-Type")
	if mt, _, err := mime.ParseMediaType(ct); ct != "" && (err != nil || mt != "text/html" && mt != "application/xhtml+xml") {
		return Metadata{}, nil
	}
	body, err := charset.NewReader(io.LimitReader(resp.Body, maxBytes), ct)
	if err != nil {
		return Metadata{}, err
	}
	return parse(body), nil
}

// parse reads the page's <head>: og: and twitter: tags win over <title> and
// the plain description.
func parse(r io.Reader) Metadata {
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
	return Metadata{
		Title:       first(maxTitle, found["og:title"], found["twitter:title"], title.String()),
		Description: first(maxDesc, found["og:description"], found["twitter:description"], found["description"]),
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
