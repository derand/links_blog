package links

import (
	"net/url"
	"strings"
)

// Canonical returns the URL to keep for raw (which has a scheme): x.com's
// mirrors and old hosts point to https://x.com, so the post doesn't depend on
// a fixer that may disappear, and tracking and sharing parameters are dropped.
// The rest of the URL stays as written; one that doesn't parse is returned as is.
func Canonical(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	base, frag, hasFrag := strings.Cut(raw, "#")
	base, query, _ := strings.Cut(base, "?")
	changed := false

	host := strings.ToLower(u.Hostname())
	site := strings.TrimPrefix(host, "www.")
	if twitterHosts[site] && host != "x.com" && u.Port() == "" {
		base = "https://x.com" + u.EscapedPath()
		site = "x.com"
		changed = true
	}

	var keep []string
	if query != "" {
		for _, kv := range strings.Split(query, "&") {
			key, val, _ := strings.Cut(kv, "=")
			if kv == "" || dropShared(site, u.Path, key, val) {
				changed = true
				continue
			}
			keep = append(keep, kv)
		}
	}
	if !changed {
		return raw
	}
	if len(keep) > 0 {
		base += "?" + strings.Join(keep, "&")
	}
	if hasFrag {
		base += "#" + frag
	}
	return base
}

// dropShared reports whether a query parameter of a page on site (host
// without "www.") at path only tells how the link was shared.
func dropShared(site, path, key, val string) bool {
	if tracking(key) {
		return true
	}
	switch {
	case site == "x.com":
		// s is the app the tweet was shared from, t the sharer's account token.
		return key == "s" || key == "t"
	case youtubeHosts[site]:
		switch key {
		case "si", "pp", "feature", "ab_channel", "index":
			return true
		case "list":
			// Liked videos and Watch later: only their owner can open them.
			return path == "/watch" && (val == "LL" || val == "WL")
		}
	}
	return false
}
