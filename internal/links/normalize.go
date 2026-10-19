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

// trackingParams are query parameters dropped from kept links and their dedup key.
var trackingParams = map[string]bool{
	"fbclid": true, "gclid": true, "dclid": true, "yclid": true, "msclkid": true,
	"mc_cid": true, "mc_eid": true, "igshid": true, "_hsenc": true, "_hsmi": true,
}

// Normalize returns the dedup key for a link URL: scheme, "www.", default port,
// trailing slash and tracking and sharing parameters are dropped, the host is
// lowercased and the remaining query parameters are sorted. A tweet's key is
// x.com/i/status/<id> whatever the host and nick, a YouTube video's is
// youtube.com/watch?v=<id> without its playlist. Two URLs with the same key are
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

	// The same tweet or video is reachable by several URLs.
	video := ""
	switch {
	case twitterHosts[host]:
		if id := TweetID(u.Path); id != "" {
			return "x.com/i/status/" + id, nil
		}
		host = "x.com"
	case youtubeHosts[host]:
		if video = youtubeVideo(host, u); video != "" {
			host, p = "youtube.com", "/watch"
		} else if host == "m.youtube.com" {
			host = "youtube.com"
		}
	}

	var params []string
	for _, kv := range strings.Split(u.RawQuery, "&") {
		if kv == "" {
			continue
		}
		key, val, _ := strings.Cut(kv, "=")
		if dropShared(host, u.Path, key, val) || video != "" && (key == "v" || key == "list") {
			continue
		}
		params = append(params, kv)
	}
	if video != "" {
		params = append(params, "v="+video)
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
