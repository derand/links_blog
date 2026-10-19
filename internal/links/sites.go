package links

import (
	"net/url"
	"regexp"
	"strings"
)

// twitterHosts serve x.com's paths: the site itself and the embed fixers
// people paste into Discord for a preview. Hosts without "www.".
var twitterHosts = map[string]bool{
	"x.com": true, "twitter.com": true, "mobile.twitter.com": true, "mobile.x.com": true,
	"fxtwitter.com": true, "fixupx.com": true, "vxtwitter.com": true, "fixvx.com": true,
}

var tweetPath = regexp.MustCompile(`^/(?:[A-Za-z0-9_]{1,15}|i(?:/web)?)/status(?:es)?/(\d{1,20})(?:/|$)`)

// TwitterHost reports whether host (any case, with or without "www.") serves
// x.com's pages.
func TwitterHost(host string) bool {
	return twitterHosts[strings.TrimPrefix(strings.ToLower(host), "www.")]
}

// TweetID returns the tweet's id for a path on a TwitterHost:
// /<nick>/status/<id>, /i/status/<id>, /i/web/status/<id>; "" otherwise.
func TweetID(path string) string {
	if m := tweetPath.FindStringSubmatch(path); m != nil {
		return m[1]
	}
	return ""
}

// Tweet reports whether raw is a tweet's URL on x.com or one of its mirrors.
func Tweet(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && TwitterHost(u.Hostname()) && TweetID(u.Path) != ""
}

// youtubeHosts are YouTube's hosts without "www.".
var youtubeHosts = map[string]bool{
	"youtube.com": true, "m.youtube.com": true, "youtu.be": true,
}

var (
	videoID   = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	videoPath = regexp.MustCompile(`^/(?:shorts|live|embed)/([A-Za-z0-9_-]{11})/?$`)
)

// youtubeVideo returns the video id for a YouTube URL whose host (lowercase,
// without "www.") is in youtubeHosts: youtu.be/ID, /watch?v=ID, /shorts/ID,
// /live/ID, /embed/ID; "" for other pages.
func youtubeVideo(host string, u *url.URL) string {
	if host == "youtu.be" {
		if id := strings.Trim(u.Path, "/"); videoID.MatchString(id) {
			return id
		}
		return ""
	}
	if u.Path == "/watch" {
		if id := u.Query().Get("v"); videoID.MatchString(id) {
			return id
		}
		return ""
	}
	if m := videoPath.FindStringSubmatch(u.Path); m != nil {
		return m[1]
	}
	return ""
}

// tracking reports whether a query parameter only says where the visitor
// came from.
func tracking(key string) bool {
	k := strings.ToLower(key)
	return strings.HasPrefix(k, "utm_") || trackingParams[k]
}
