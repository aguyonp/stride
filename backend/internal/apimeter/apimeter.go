// Package apimeter guards outbound provider calls (concurrency, cooldown after rate limiting)
// and keeps counters for the admin dashboard.
package apimeter

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	Garmin    = "garmin"
	Strava    = "strava"
	OpenMeteo = "open-meteo"
	LLM       = "llm"
	Webhook   = "webhook"
)

var ErrCooldown = errors.New("provider cooling down after rate limiting")

type CooldownError struct {
	Provider string
	Until    time.Time
}

func (e *CooldownError) Error() string {
	return fmt.Sprintf("%s: %v (until %s)", e.Provider, ErrCooldown, e.Until.Format(time.RFC3339))
}

func (e *CooldownError) Unwrap() error { return ErrCooldown }

// Call describes one outbound request. Endpoint is a path template without ids or secrets.
type Call struct {
	Provider   string
	UserID     int64
	Endpoint   string
	Status     int
	Duration   time.Duration
	Err        error
	RetryAfter time.Duration
}

type Quota struct {
	Limit15m   int       `json:"limit15m"`
	Usage15m   int       `json:"usage15m"`
	LimitDaily int       `json:"limitDaily"`
	UsageDaily int       `json:"usageDaily"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type ProviderStats struct {
	Provider       string     `json:"provider"`
	Requests1h     int        `json:"requests1h"`
	Requests24h    int        `json:"requests24h"`
	Errors1h       int        `json:"errors1h"`
	RateLimited24h int        `json:"rateLimited24h"`
	InFlight       int        `json:"inFlight"`
	CooldownUntil  *time.Time `json:"cooldownUntil"`
	Quota          *Quota     `json:"quota"`
	AvgLatencyMs   int        `json:"avgLatencyMs"`
}

type ErrorEvent struct {
	Time     time.Time `json:"time"`
	Provider string    `json:"provider"`
	UserID   int64     `json:"userId"`
	Endpoint string    `json:"endpoint"`
	Status   int       `json:"status"`
	Message  string    `json:"message"`
}

type Snapshot struct {
	Providers    []ProviderStats `json:"providers"`
	RecentErrors []ErrorEvent    `json:"recentErrors"`
}

type Meter interface {
	// Acquire waits for a concurrency slot; it fails fast with *CooldownError while the provider is cooling down.
	Acquire(ctx context.Context, provider string, userID int64) (release func(), err error)
	// Observe records the outcome; a 429 (or Status 0 with RetryAfter) starts or extends a cooldown.
	Observe(c Call)
	SetQuota(provider string, q Quota)
	Snapshot() Snapshot
}

type nop struct{}

// Nop never limits nor records; for tests.
func Nop() Meter { return nop{} }

func (nop) Acquire(context.Context, string, int64) (func(), error) { return func() {}, nil }
func (nop) Observe(Call)                                           {}
func (nop) SetQuota(string, Quota)                                 {}
func (nop) Snapshot() Snapshot                                     { return Snapshot{} }

type userKey struct{}

// WithUser tags outbound calls made with ctx so provider clients can attribute them.
func WithUser(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userKey{}, userID)
}

func UserFrom(ctx context.Context) int64 {
	id, _ := ctx.Value(userKey{}).(int64)
	return id
}
