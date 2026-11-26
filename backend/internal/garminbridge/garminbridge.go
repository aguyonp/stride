// Package garminbridge pushes planned sessions to Garmin Connect through the *web* API, the one the
// official app uses (SPEC §23, issue #147).
//
// It exists because the Personal API (§11.3, docs/GARMIN_API.md) is read-only: `/workout-service/*`
// answers 404/403 there. This bridge is experimental by design: an unofficial API, a real risk of
// account suspension, and a flow that can break without notice. It is disabled by default, behind
// one admin switch, and it is isolated on purpose:
//
//   - no import of `domain`, `store` or `workout` — the caller converts a session into the Garmin
//     workout JSON, this package only speaks HTTP and OAuth;
//   - one narrow interface (Login, LoginMFA, Status, Push, Disconnect) and its own HTTP client;
//   - deleting this package, its two routes and its tokens removes the feature entirely.
//
// The password is used for the duration of one request: it is never stored, never logged, and never
// returned. Only the OAuth tokens are persisted, encrypted by the caller's TokenStore.
package garminbridge

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// States exposed to the interface. They are stable codes: the panel localizes them.
const (
	StateDisconnected = "disconnected"
	StateConnecting   = "connecting"
	StateMFA          = "mfa"
	StateVerifying    = "verifying"
	StateConnected    = "connected"
	StateExpired      = "expired"
	StateBlocked      = "blocked"
)

var (
	// ErrMFARequired means Garmin asked for a code: the caller must call LoginMFA.
	ErrMFARequired = errors.New("garmin bridge: code de vérification requis")
	// ErrBlocked is the anti-bot answer proper (429, 427, « unusual traffic »): retrying in a loop
	// only makes it worse, so the caller stops and offers the .FIT fallback.
	ErrBlocked = errors.New("garmin bridge: Garmin limite les tentatives (429/427), réessayez dans un moment ou utilisez le fichier .FIT")
	// ErrChallenge is a CAPTCHA wall: Garmin wants a human. The bridge cannot solve it, and no
	// amount of retrying will, so it is told apart from ErrCredentials (« wrong password »).
	ErrChallenge = errors.New("garmin bridge: Garmin demande une vérification anti-robot que le pont ne peut pas résoudre, utilisez le fichier .FIT")
	// ErrExpired means the stored tokens no longer work and no refresh could save them.
	ErrExpired = errors.New("garmin bridge: session Garmin expirée, reconnectez le pont")
	// ErrNotConnected means no tokens are stored for this user.
	ErrNotConnected = errors.New("garmin bridge: pont non connecté")
	// ErrDisabled is the kill switch (env or admin setting).
	ErrDisabled = errors.New("garmin bridge: pont désactivé")
	// ErrCredentials is a refused email/password pair.
	ErrCredentials = errors.New("garmin bridge: identifiants refusés par Garmin")
)

// Tokens are what the bridge persists. They never leave the server: the interface only knows whether
// they exist and when they expire.
type Tokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"` // OAuth1 pair "token:secret", which signs the refresh
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	DisplayName  string    `json:"display_name,omitempty"`
}

// TokenStore is implemented by the caller (encrypted at rest, per user, AAD `garmin_web:<uid>:token`).
type TokenStore interface {
	Load(ctx context.Context, userID int64) (*Tokens, error)
	Save(ctx context.Context, userID int64, t Tokens) error
	Delete(ctx context.Context, userID int64) error
}

// Status is the state of the bridge for one user.
type Status struct {
	State       string `json:"state"`
	DisplayName string `json:"displayName,omitempty"`
	Since       string `json:"since,omitempty"`
	Message     string `json:"message,omitempty"`
	// Locked is the domain's circuit breaker: repeated failures pause the bridge for a while.
	Locked bool `json:"locked,omitempty"`
	// Available says whether the instance allows the bridge at all (the administrator switched it
	// on). False keeps the settings card hidden: nothing to offer, nothing to explain.
	Available bool `json:"available"`
}

// PushRequest is one workout to send. Payload is the Garmin workout JSON, built by the caller with
// `workout.GarminWorkout` — the same payload the official app uploads.
type PushRequest struct {
	// Name is deterministic and carries the Stride session id: it is the idempotency key.
	Name string
	// Date is the calendar day to schedule on (YYYY-MM-DD).
	Date string
	// Payload is the `workout-service/workout` body.
	Payload json.RawMessage
	// WorkoutID is the id kept from a previous push, 0 when the workout is unknown.
	WorkoutID int64
}

// PushResult reports what happened.
type PushResult struct {
	WorkoutID   int64  `json:"workoutId"`
	Created     bool   `json:"created"`
	Scheduled   bool   `json:"scheduled"`
	ScheduleErr string `json:"scheduleError,omitempty"`
}

// Options tune the service. Every endpoint is injectable: the tests point them at httptest servers,
// and the CI never reaches Garmin.
type Options struct {
	// HTTP is the client used for every call. A nil value gets a client with a short timeout and a
	// strict redirect policy.
	HTTP *http.Client
	// Now is the clock (token expiry, pending MFA sessions).
	Now func() time.Time
	// SSOBase is the sign-in host (https://sso.garmin.com).
	SSOBase string
	// APIBase is the host serving /workout-service and /userprofile-service
	// (https://connectapi.garmin.com).
	APIBase string
	// Service is the SSO service parameter Garmin expects, and the service_url the ticket exchange
	// must repeat: Garmin binds a ticket to the service it was issued for.
	Service string
	// DIBase is the OAuth2 token host (https://diauth.garmin.com).
	DIBase string
	// UserAgent identifies the flow; the mobile one is less challenged than a browser UA.
	UserAgent string
	// ConsumerKey / ConsumerSecret sign the OAuth1 exchange. The defaults are the public values the
	// open-source Garmin clients (garth, python-garminconnect) ship, not private credentials.
	ConsumerKey    string
	ConsumerSecret string
	// MFATTL bounds how long a pending sign-in stays usable in memory.
	MFATTL time.Duration
	// Log receives one line per state change; it never receives a password, a cookie or a token.
	Log Logger
	// Store persists the tokens (encrypted, per user). Required.
	Store TokenStore
}

// Logger is the small surface the bridge needs from slog.
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}

// Public OAuth consumer credentials of the Garmin Connect apps, as published by the open-source
// clients (garth's oauth_consumer.json). They identify the client, not a user.
const (
	publicConsumerKey    = "fc3e99d2-118c-44b8-8ae3-03370dde24c0"
	publicConsumerSecret = "E08WAR897WEy2knn7aFBrvegVAf0AFdWBBF"
	// oauthUserAgent is the UA of the OAuth endpoints: it must match the Android consumer key, and
	// the SSO one (GCM-Android…) is not accepted there.
	oauthUserAgent = "com.garmin.android.apps.connectmobile"
	// mobileUserAgent is the Garmin Connect Android app: the SSO answers it with the mobile flow.
	mobileUserAgent = "GCM-Android-5.23; ; 4.4.2; 1.0"
	// serviceURL is where the SSO sends the ticket once the sign-in succeeds.
	serviceURL = "https://connect.garmin.com/modern/"
)

// Service is safe for concurrent use.
type Service struct {
	opt    Options
	client *http.Client

	mu      sync.Mutex
	pending map[int64]*pendingLogin
}

// pendingLogin is the SSO session kept between the password step and the MFA step. Memory only: a
// restart during MFA simply asks the user to sign in again, which is better than persisting cookies.
type pendingLogin struct {
	sess   *session
	csrf   string
	params url.Values
	// referer is the URL of the page that asked for the code: a browser posts the code form with
	// that page as its referer, and Garmin checks it.
	referer string
	at      time.Time
}

// New builds the service.
func New(opt Options) *Service {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.SSOBase == "" {
		opt.SSOBase = "https://sso.garmin.com"
	}
	if opt.APIBase == "" {
		opt.APIBase = "https://connectapi.garmin.com"
	}
	if opt.Service == "" {
		opt.Service = embedURL(opt)
	}
	if opt.DIBase == "" {
		opt.DIBase = "https://diauth.garmin.com"
	}
	if opt.UserAgent == "" {
		opt.UserAgent = mobileUserAgent
	}
	if opt.ConsumerKey == "" {
		opt.ConsumerKey = publicConsumerKey
	}
	if opt.ConsumerSecret == "" {
		opt.ConsumerSecret = publicConsumerSecret
	}
	if opt.MFATTL <= 0 {
		opt.MFATTL = 5 * time.Minute
	}
	s := &Service{opt: opt, pending: map[int64]*pendingLogin{}}
	if opt.HTTP != nil {
		s.client = opt.HTTP
	} else {
		s.client = &http.Client{
			Timeout: 25 * time.Second,
			Transport: &http.Transport{
				Proxy:               nil,
				ForceAttemptHTTP2:   true,
				MaxIdleConns:        8,
				IdleConnTimeout:     60 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
				TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 8 {
					return fmt.Errorf("garmin bridge: trop de redirections")
				}
				return nil
			},
		}
	}
	return s
}

func (s *Service) log() Logger {
	if s.opt.Log == nil {
		return nopLogger{}
	}
	return s.opt.Log
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any) {}
func (nopLogger) Warn(string, ...any) {}

// Status reports the bridge state from the stored tokens, without any network call.
func (s *Service) Status(ctx context.Context, userID int64) Status {
	t, err := s.opt.Store.Load(ctx, userID)
	if err != nil {
		s.log().Warn("🔗 Pont Garmin : lecture des jetons impossible", "user", userID, "err", err.Error())
		return Status{State: StateDisconnected}
	}
	if t == nil || t.AccessToken == "" {
		// The password step was already accepted and the user owes a code: reporting
		// « disconnected » here made the code step unreachable — the panel redisplayed the password
		// form while the pending session waited for a code nobody could type.
		if s.hasPending(userID) {
			return Status{State: StateMFA}
		}
		return Status{State: StateDisconnected}
	}
	if !t.ExpiresAt.IsZero() && s.opt.Now().After(t.ExpiresAt) {
		return Status{State: StateExpired, DisplayName: t.DisplayName}
	}
	return Status{State: StateConnected, DisplayName: t.DisplayName, Since: t.CreatedAt.UTC().Format(time.RFC3339)}
}

// Disconnect forgets the tokens; the caller decides whether to notify.
func (s *Service) Disconnect(ctx context.Context, userID int64) error {
	s.dropPending(userID)
	if err := s.opt.Store.Delete(ctx, userID); err != nil {
		return err
	}
	s.log().Info("🔗 Pont Garmin déconnecté", "user", userID)
	return nil
}

func (s *Service) takePending(userID int64) (*pendingLogin, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[userID]
	if !ok {
		return nil, false
	}
	if s.opt.Now().Sub(p.at) > s.opt.MFATTL {
		delete(s.pending, userID)
		return nil, false
	}
	return p, true
}

// hasPending reports a sign-in waiting for its code, and forgets it once stale.
func (s *Service) hasPending(userID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[userID]
	if !ok {
		return false
	}
	if s.opt.Now().Sub(p.at) > s.opt.MFATTL {
		delete(s.pending, userID)
		return false
	}
	return true
}

func (s *Service) keepPending(userID int64, p *pendingLogin) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.at = s.opt.Now()
	s.pending[userID] = p
}

func (s *Service) dropPending(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, userID)
}

// sansSecrets is the shape of a user profile answer we care about.
type profile struct {
	DisplayName    string `json:"displayName"`
	FullName       string `json:"fullName"`
	UserName       string `json:"userName"`
	ProfileID      int64  `json:"profileId"`
	UserProfileID  int64  `json:"userProfileId"`
	Location       string `json:"location"`
	PrimaryEmail   string `json:"primaryEmail"`
	AccountSuccess bool
}

func (p profile) name() string {
	for _, v := range []string{p.DisplayName, p.FullName, p.UserName} {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
