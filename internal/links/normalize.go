// Package links holds the domain rules for blog links.
package links

import (
	"errors"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"
)

// DayLayout is the format of Link.Day.
const DayLayout = "2006-01-02"

// Day returns t's calendar day in loc, formatted as DayLayout.
func Day(t time.Time, loc *time.Location) string {
	return t.In(loc).Format(DayLayout)
}

// trackingParams are query parameters dropped from the dedup key.
var trackingParams = map[string]bool{
	"fbclid": true, "gclid": true, "dclid": true, "yclid": true, "msclkid": true,
	"mc_cid": true, "mc_eid": true, "igshid": true, "_hsenc": true, "_hsmi": true,
}

// Normalize returns the dedup key for a link URL: scheme, "www.", default port,
// trailing slash and tracking parameters are dropped, the host is lowercased and
// the remaining query parameters are sorted. Two URLs with the same key are
// considered the same link. The key is not meant to be opened.
func Normalize(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty url")
	}
	if !strings.Contains(raw, "://") {
		// "mailto:x@y" has a scheme, "example.com:8080/x" is a host with a port.
		if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Opaque != "" &&
			(u.Opaque[0] < '0' || u.Opaque[0] > '9') {
			return "", errors.New("unsupported scheme " + u.Scheme)
		}
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("unsupported scheme " + u.Scheme)
	}

	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", errors.New("missing host")
	}
	host = strings.TrimPrefix(host, "www.")
	if port := u.Port(); port != "" && !(scheme == "http" && port == "80") && !(scheme == "https" && port == "443") {
		host = net.JoinHostPort(host, port)
	}

	p := strings.TrimRight(u.EscapedPath(), "/")

	var params []string
	for _, kv := range strings.Split(u.RawQuery, "&") {
		if kv == "" {
			continue
		}
		key, _, _ := strings.Cut(kv, "=")
		if k := strings.ToLower(key); strings.HasPrefix(k, "utm_") || trackingParams[k] {
			continue
		}
		params = append(params, kv)
	}
	sort.Strings(params)

	key := host + p
	if len(params) > 0 {
		key += "?" + strings.Join(params, "&")
	}
	if u.Fragment != "" {
		key += "#" + u.EscapedFragment()
	}
	return key, nil
}
