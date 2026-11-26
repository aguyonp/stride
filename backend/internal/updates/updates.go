// Package updates detects a newer Stride release on GitHub (SPEC §22, issue #143).
//
// The check runs on the server: a shared cache keeps one request per TTL whatever the number of
// tabs, devices or admins, the users' IP addresses never reach GitHub, and the answer is the same
// for everyone. A failure is never an error for the caller — no banner, no notification, a log
// line and a shorter TTL to try again.
package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Repository is constant on purpose: the link shown to the user is built from it, never from the
// URL in GitHub's answer (an arbitrary URL is never followed or displayed).
const Repository = "aguyonp/stride"

// ReleaseURL is the page the banner and the notification open.
func ReleaseURL(tag string) string {
	return "https://github.com/" + Repository + "/releases/tag/" + tag
}

const (
	// maxBody bounds the answer: a release payload is a few kilobytes.
	maxBody = 64 << 10
	// SuccessTTL and FailureTTL are the cache lifetimes: a release is published every few days, and
	// a failed check must not hammer GitHub.
	SuccessTTL = 6 * time.Hour
	FailureTTL = time.Hour
)

// Status is what GET /api/updates returns. `available` is true only when the latest stable release
// is strictly newer than the running version.
type Status struct {
	Current     string `json:"current"`
	Latest      string `json:"latest,omitempty"`
	Available   bool   `json:"available"`
	URL         string `json:"url,omitempty"`
	PublishedAt string `json:"publishedAt,omitempty"`
	CheckedAt   string `json:"checkedAt,omitempty"`
	// Disabled mirrors the admin setting, so the panel can say « vérification désactivée » instead
	// of pretending no release exists.
	Disabled bool `json:"disabled,omitempty"`
	// Notification is the caller's centre entry for `latest` (§21), when one exists.
	Notification int64 `json:"notificationId,omitempty"`
	// Read is the read state of that entry: the banner shows until it is read.
	Read bool `json:"read,omitempty"`
}

// Service performs and caches the check.
type Service struct {
	base   string
	client *http.Client
	now    func() time.Time

	mu       sync.Mutex
	latest   release
	fetched  time.Time
	lastErr  string
	okTTL    time.Duration
	failTTL  time.Duration
	requests int
}

type release struct {
	Tag         string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
	Prerelease  bool   `json:"prerelease"`
	Draft       bool   `json:"draft"`
}

// Option tunes the service (tests use the base URL, the clock and the TTLs).
type Option func(*Service)

// WithBaseURL points the service at another API (an httptest server in tests).
func WithBaseURL(u string) Option { return func(s *Service) { s.base = strings.TrimSuffix(u, "/") } }

// WithClock replaces the clock (cache TTL tests).
func WithClock(f func() time.Time) Option { return func(s *Service) { s.now = f } }

// WithTTL overrides the cache lifetimes.
func WithTTL(ok, fail time.Duration) Option {
	return func(s *Service) { s.okTTL, s.failTTL = ok, fail }
}

// New builds the service. The client must come from egress (§6) so a redirected or forged answer
// can never reach a private address.
func New(client *http.Client, opts ...Option) *Service {
	s := &Service{base: "https://api.github.com", client: client, now: time.Now, okTTL: SuccessTTL, failTTL: FailureTTL}
	for _, o := range opts {
		o(s)
	}
	if s.client == nil {
		s.client = &http.Client{Timeout: 5 * time.Second}
	}
	return s
}

// Check answers the status for the running version, from the cache when it is fresh.
func (s *Service) Check(ctx context.Context, current string) Status {
	st := Status{Current: current}
	rel, at, err := s.fetch(ctx)
	if err != nil {
		return st
	}
	st.CheckedAt = at.UTC().Format(time.RFC3339)
	// Reported normalized when the tag parses (`v4.7.1` → `4.7.1`), raw otherwise.
	if norm, ok := normalize(rel.Tag); ok {
		st.Latest = norm
	} else {
		st.Latest = rel.Tag
	}
	st.PublishedAt = rel.PublishedAt
	// A dev build has no version to compare: the release is still reported, nothing is announced.
	if !Comparable(current) {
		return st
	}
	// A pre-release or a malformed tag never counts as newer (Newer normalizes both sides), and the
	// link is built from our constant repository plus the validated tag, never from the payload.
	if !Newer(rel.Tag, current) {
		return st
	}
	st.Available = true
	st.URL = ReleaseURL(st.Latest)
	return st
}

// fetch returns the cached release, or refreshes it. Failures are cached for FailureTTL: GitHub
// being down must not turn a page load into a request storm.
func (s *Service) fetch(ctx context.Context) (release, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ttl := s.okTTL
	if s.lastErr != "" {
		ttl = s.failTTL
	}
	if !s.fetched.IsZero() && s.now().Sub(s.fetched) < ttl {
		return s.latest, s.fetched, errString(s.lastErr)
	}
	rel, err := s.get(ctx)
	s.fetched = s.now()
	s.requests++
	// « No stable release yet » is an answer, not a failure: cache it like a success, silently.
	if errors.Is(err, errNoStable) {
		s.lastErr = ""
		s.latest = release{}
		return s.latest, s.fetched, nil
	}
	if err != nil {
		s.lastErr = err.Error()
		s.latest = release{}
		return s.latest, s.fetched, err
	}
	s.lastErr = ""
	s.latest = rel
	return rel, s.fetched, nil
}

func errString(s string) error {
	if s == "" {
		return nil
	}
	return fmt.Errorf("%s", s)
}

// get performs the single HTTP call. Everything is bounded: 5 s through the client, 64 KiB read.
func (s *Service) get(ctx context.Context) (release, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/repos/"+Repository+"/releases/latest", nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "stride-update-check")
	res, err := s.client.Do(req)
	if err != nil {
		return release{}, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		// 403/429 (rate limit) and 5xx land here: no answer, no notification.
		return release{}, fmt.Errorf("github: %s", res.Status)
	}
	var rel release
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBody)).Decode(&rel); err != nil {
		return release{}, fmt.Errorf("github: réponse illisible: %w", err)
	}
	if rel.Tag == "" {
		return release{}, fmt.Errorf("github: étiquette absente")
	}
	// Pre-releases and drafts are out of scope (§22): the banner is about a stable release.
	if rel.Prerelease || rel.Draft {
		return release{}, errNoStable
	}
	return rel, nil
}

// errNoStable marks "we asked, there is simply nothing to compare": it is cached like a success
// (the repository only holds pre-releases for now) but never surfaces as an error.
var errNoStable = fmt.Errorf("github: pas de release stable")

// Requests reports how many calls reached the API (tests assert the cache).
func (s *Service) Requests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests
}
