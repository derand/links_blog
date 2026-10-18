package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// X shows a page without tweets to anything but a browser, so tweets and
// profiles come from FxTwitter's API instead. It is a third-party service and
// may disappear: then these links just stay without metadata.
const fxTwitterAPI = "https://api.fxtwitter.com"

// Hosts that serve x.com's paths: the site itself and the embed fixers.
var twitterHosts = map[string]bool{
	"x.com": true, "twitter.com": true, "mobile.twitter.com": true, "mobile.x.com": true,
	"fxtwitter.com": true, "fixupx.com": true, "vxtwitter.com": true, "fixvx.com": true,
}

var (
	tweetPath   = regexp.MustCompile(`^/(?:[A-Za-z0-9_]{1,15}|i(?:/web)?)/status(?:es)?/(\d{1,20})(?:/|$)`)
	profilePath = regexp.MustCompile(`^/([A-Za-z0-9_]{1,15})/?$`)
	// x.com pages whose path looks like a user name.
	notProfiles = map[string]bool{
		"home": true, "i": true, "explore": true, "search": true, "settings": true, "notifications": true,
		"messages": true, "hashtag": true, "intent": true, "share": true, "login": true, "signup": true,
		"logout": true, "compose": true, "tos": true, "privacy": true, "about": true, "jobs": true,
	}
)

// twitterAPIPath returns the FxTwitter API path for a tweet or profile URL,
// or "" if u is not one.
func twitterAPIPath(u *url.URL) string {
	if !twitterHosts[strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")] {
		return ""
	}
	if m := tweetPath.FindStringSubmatch(u.Path); m != nil {
		return "/status/" + m[1]
	}
	if m := profilePath.FindStringSubmatch(u.Path); m != nil && !notProfiles[strings.ToLower(m[1])] {
		return "/" + m[1]
	}
	return ""
}

type fxAuthor struct {
	Name        string `json:"name"`
	ScreenName  string `json:"screen_name"`
	Description string `json:"description"`
}

type fxResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Tweet   *struct {
		Text   string   `json:"text"`
		Author fxAuthor `json:"author"`
	} `json:"tweet"`
	User *fxAuthor `json:"user"`
}

// FxTwitter's profile lookup answers 404 for about one request in five to
// existing accounts, so it is asked again right away, and a 404 that repeats
// is still not taken as a deleted account. Its 404 for tweets is reliable.
var profileRetry = time.Second

// fetchTwitter asks FxTwitter about the tweet or profile at apiPath: the
// title is "Name (@nick)", the description the tweet's text or the bio.
func (f *Fetcher) fetchTwitter(ctx context.Context, apiPath string) (Metadata, error) {
	m, err := f.askFxTwitter(ctx, apiPath)
	var status *StatusError
	if strings.HasPrefix(apiPath, "/status/") || !errors.As(err, &status) || status.Code != 404 {
		return m, err
	}
	t := time.NewTimer(profileRetry)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return Metadata{}, ctx.Err()
	case <-t.C:
	}
	m, err = f.askFxTwitter(ctx, apiPath)
	if errors.As(err, &status) && status.Code == 404 {
		return Metadata{}, errors.New("fxtwitter: user not found, may be a false 404")
	}
	return m, err
}

func (f *Fetcher) askFxTwitter(ctx context.Context, apiPath string) (Metadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.fxAPI+apiPath, nil)
	if err != nil {
		return Metadata{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return Metadata{}, err
	}
	defer resp.Body.Close()
	switch code := resp.StatusCode; {
	case code == 401 || code == 403: // private tweet or protected account
		return Metadata{}, &BlockedError{fmt.Sprintf("fxtwitter: http status %d", code)}
	case code < 200 || code > 299:
		return Metadata{}, &StatusError{code}
	}
	// Ids it can't parse get its HTML home page with 200: that's FxTwitter
	// misbehaving, not the link, so it's worth asking again later.
	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mt != "application/json" {
		return Metadata{}, fmt.Errorf("fxtwitter: unexpected content type %q", mt)
	}
	var r fxResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBytes)).Decode(&r); err != nil {
		return Metadata{}, fmt.Errorf("fxtwitter: %w", err)
	}
	var who *fxAuthor
	var text string
	switch {
	case r.Tweet != nil:
		who, text = &r.Tweet.Author, r.Tweet.Text
	case r.User != nil:
		who, text = r.User, r.User.Description
	default:
		return Metadata{}, errors.New("fxtwitter: no tweet or user in the answer: " + r.Message)
	}
	title := who.Name
	if who.ScreenName != "" {
		title = strings.TrimSpace(title + " (@" + who.ScreenName + ")")
	}
	return Metadata{Title: first(maxTitle, title), Description: first(maxDesc, text)}, nil
}
