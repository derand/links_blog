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
	"strings"

	"github.com/derand/links_blog/internal/links"
)

// Reddit answers a bot with 403 or a page titled "Reddit", TikTok with its
// slogan, but both describe their posts through oEmbed. Reddit's knows only
// posts, not subreddits or users.
const (
	redditOEmbed = "https://www.reddit.com/oembed"
	tiktokOEmbed = "https://www.tiktok.com/oembed"
)

// errNoPost is TikTok's answer for a deleted post: 200 with neither a title
// nor an author.
var errNoPost = errors.New("oembed: no such post")

// oembedAPI returns the oEmbed endpoint that describes u, or "".
func (f *Fetcher) oembedAPI(u *url.URL) string {
	switch {
	case links.RedditPost(u):
		return f.redditAPI
	case links.TikTokPage(u):
		return f.tiktokAPI
	}
	return ""
}

type oembedResponse struct {
	Title          string `json:"title"`
	AuthorName     string `json:"author_name"`
	AuthorURL      string `json:"author_url"`
	AuthorUniqueID string `json:"author_unique_id"`
	EmbedType      string `json:"embed_type"` // TikTok: "video", "photo", "profile"
}

// fetchOEmbed asks api about page: the title is the post's (or a TikTok
// profile's name), the author the nick without "@" or "u/".
func (f *Fetcher) fetchOEmbed(ctx context.Context, api, page string) (Metadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"?url="+url.QueryEscape(page), nil)
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
	case code == 401 || code == 403:
		return Metadata{}, &BlockedError{fmt.Sprintf("oembed: http status %d", code)}
	case code < 200 || code > 299: // 404: deleted post; 400: TikTok's unknown user
		return Metadata{}, &StatusError{code}
	}
	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mt != "application/json" {
		return Metadata{}, fmt.Errorf("oembed: unexpected content type %q", mt)
	}
	var r oembedResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBytes)).Decode(&r); err != nil {
		return Metadata{}, fmt.Errorf("oembed: %w", err)
	}
	// TikTok's author_name is the display name, the nick is in author_url;
	// Reddit's author_name is the nick.
	nick := r.AuthorUniqueID
	if nick == "" && r.AuthorURL != "" {
		if au, err := url.Parse(r.AuthorURL); err == nil {
			nick = strings.TrimPrefix(strings.Trim(au.Path, "/"), "@")
		}
	}
	if nick == "" && r.AuthorURL == "" {
		nick = r.AuthorName
	}
	title := r.Title
	if r.EmbedType == "profile" || title == "" {
		title = r.AuthorName // not "Name's Creator Profile"
	}
	title = first(maxTitle, title)
	if title == "" && nick == "" {
		return Metadata{}, errNoPost
	}
	return Metadata{Title: title, Author: first(maxTitle, nick)}, nil
}
