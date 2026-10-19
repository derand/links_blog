// Package service is the single entry point for changing the blog's links:
// bots, the web form and the API all go through LinkService.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/derand/links_blog/internal/links"
	"github.com/derand/links_blog/internal/store"
)

// ErrInvalid marks errors caused by bad input rather than by the server.
var ErrInvalid = errors.New("invalid link")

// ErrNotFound is returned for an unknown link ID.
var ErrNotFound = store.ErrNotFound

type LinkService interface {
	AddLink(ctx context.Context, in NewLink) (AddResult, error)
	Link(ctx context.Context, id int64) (*store.Link, error)
	UpdateLink(ctx context.Context, in LinkEdit) (AddResult, error)
	// DeleteLink deletes a link and returns what it was.
	DeleteLink(ctx context.Context, id int64) (*store.Link, error)
	// MessageLinks returns the links added from a chat message, see store.LinksByMessage.
	MessageLinks(ctx context.Context, source, messageRef string) ([]*store.Link, error)
	// FillMetadata sets a link's title and site description where they are empty.
	FillMetadata(ctx context.Context, id int64, title, siteDescription string) (bool, error)

	Channel(ctx context.Context, platform, channelID string) (*store.Channel, error)
	Channels(ctx context.Context, platform string) ([]*store.Channel, error)
	SaveChannel(ctx context.Context, c *store.Channel) error
	AdvanceChannel(ctx context.Context, platform, channelID string, messageID int64) error
}

// NewLink is a link to add.
type NewLink struct {
	URL             string
	Description     string // author's text
	Note            string // author's full text if Description is its beginning
	Title           string // page metadata, if already known
	SiteDescription string
	Tags            []string
	Hidden          bool
	Day             string    // YYYY-MM-DD; empty means the day of At
	At              time.Time // when the link was posted; zero means now
	Source          string    // empty means "web"
	SourceRef       string    // e.g. "channel_id:message_id"; unique within Source
}

// LinkEdit is the new state of an existing link's editable fields.
type LinkEdit struct {
	ID              int64
	URL             string
	Description     string
	Note            string
	Title           string
	SiteDescription string
	Tags            []string
	Hidden          bool
	Day             string // YYYY-MM-DD
}

type AddResult struct {
	Link      *store.Link // the added or saved link, or the existing one if Dup != NotDup
	Duplicate store.Dup
}

type Service struct {
	st   *store.Store
	loc  *time.Location
	now  func() time.Time
	wake chan struct{} // tells RunMetadata that a link was added or changed
}

// New returns a service that dates links in loc, the blog's time zone.
func New(st *store.Store, loc *time.Location) *Service {
	return &Service{st: st, loc: loc, now: time.Now, wake: make(chan struct{}, 1)}
}

func (s *Service) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

var _ LinkService = (*Service)(nil)

// AddLink adds a link unless it is a duplicate by source reference or by URL.
func (s *Service) AddLink(ctx context.Context, in NewLink) (AddResult, error) {
	rawURL, norm, err := checkURL(in.URL)
	if err != nil {
		return AddResult{}, err
	}

	at := in.At
	if at.IsZero() {
		at = s.now()
	}
	day := strings.TrimSpace(in.Day)
	if day == "" {
		day = links.Day(at, s.loc)
	} else if err := checkDay(day); err != nil {
		return AddResult{}, err
	}

	l := &store.Link{
		URL:             rawURL,
		URLNorm:         norm,
		Description:     strings.TrimSpace(in.Description),
		Note:            strings.TrimSpace(in.Note),
		Title:           strings.TrimSpace(in.Title),
		SiteDescription: strings.TrimSpace(in.SiteDescription),
		Day:             day,
		CreatedAt:       at,
		Hidden:          in.Hidden || links.Private(rawURL),
		Source:          in.Source,
		SourceRef:       in.SourceRef,
		Tags:            cleanTags(in.Tags),
	}
	if l.Source == "" {
		l.Source = "web"
	}
	got, dup, err := s.st.AddLink(ctx, l)
	if err != nil {
		return AddResult{}, err
	}
	if dup == store.NotDup {
		s.poke()
	}
	return AddResult{Link: got, Duplicate: dup}, nil
}

// UpdateLink saves an edited link unless its new URL is already taken by
// another link; then the result holds that link with store.DupURL.
func (s *Service) UpdateLink(ctx context.Context, in LinkEdit) (AddResult, error) {
	rawURL, norm, err := checkURL(in.URL)
	if err != nil {
		return AddResult{}, err
	}
	day := strings.TrimSpace(in.Day)
	if err := checkDay(day); err != nil {
		return AddResult{}, err
	}
	got, dup, err := s.st.UpdateLink(ctx, &store.Link{
		ID:              in.ID,
		URL:             rawURL,
		URLNorm:         norm,
		Description:     strings.TrimSpace(in.Description),
		Note:            strings.TrimSpace(in.Note),
		Title:           strings.TrimSpace(in.Title),
		SiteDescription: strings.TrimSpace(in.SiteDescription),
		Day:             day,
		Hidden:          in.Hidden,
		Tags:            cleanTags(in.Tags),
	})
	if err != nil {
		return AddResult{}, err
	}
	if dup == store.NotDup {
		s.poke()
	}
	return AddResult{Link: got, Duplicate: dup}, nil
}

func (s *Service) Link(ctx context.Context, id int64) (*store.Link, error) {
	return s.st.LinkByID(ctx, id)
}

func (s *Service) DeleteLink(ctx context.Context, id int64) (*store.Link, error) {
	return s.st.DeleteLink(ctx, id)
}

// checkURL returns the URL to keep, with "https://" added if it has no
// scheme and made links.Canonical, and its dedup key.
func checkURL(raw string) (url, norm string, err error) {
	url = strings.TrimSpace(raw)
	norm, err = links.Normalize(url)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if !strings.Contains(url, "://") {
		url = "https://" + url
	}
	return links.Canonical(url), norm, nil
}

func checkDay(day string) error {
	if _, err := time.Parse(links.DayLayout, day); err != nil {
		return fmt.Errorf("%w: day %q is not YYYY-MM-DD", ErrInvalid, day)
	}
	return nil
}

// cleanTags trims tags and a leading "#", drops empty ones and repeats
// (ignoring case), keeping the author's order.
func cleanTags(tags []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out
}

func (s *Service) MessageLinks(ctx context.Context, source, messageRef string) ([]*store.Link, error) {
	return s.st.LinksByMessage(ctx, source, messageRef)
}

func (s *Service) FillMetadata(ctx context.Context, id int64, title, siteDescription string) (bool, error) {
	return s.st.FillMetadata(ctx, id, strings.TrimSpace(title), strings.TrimSpace(siteDescription))
}

func (s *Service) Channel(ctx context.Context, platform, channelID string) (*store.Channel, error) {
	return s.st.Channel(ctx, platform, channelID)
}

func (s *Service) Channels(ctx context.Context, platform string) ([]*store.Channel, error) {
	return s.st.Channels(ctx, platform)
}

func (s *Service) SaveChannel(ctx context.Context, c *store.Channel) error {
	return s.st.SaveChannel(ctx, c)
}

func (s *Service) AdvanceChannel(ctx context.Context, platform, channelID string, messageID int64) error {
	return s.st.AdvanceChannel(ctx, platform, channelID, messageID)
}
