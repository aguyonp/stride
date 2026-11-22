// Package ratelimit implements a fixed-window, per-key request limiter with bounded memory:
// all keys share one window, so a single reset on window expiry keeps memory flat without a janitor.
package ratelimit

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Limiter counts requests per key (client IP, token hash, …) in a fixed window shared by all keys.
type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	start  time.Time
	hits   map[string]int
}

// New returns a limiter allowing limit requests per key per window.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, hits: map[string]int{}}
}

// Allow reports whether the request for key at time now stays within the limit.
func (l *Limiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.start) >= l.window {
		l.start = now
		clear(l.hits)
	}
	l.hits[key]++
	return l.hits[key] <= l.limit
}

// SetConfig changes the limit and the window at runtime (admin-tunable quota). The window is
// restarted so no counter outlives the settings it was counted under.
func (l *Limiter) SetConfig(limit int, window time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limit = limit
	l.window = window
	l.start = time.Time{}
	clear(l.hits)
}

// Used reports how many requests key has consumed in the current window (0 once the window
// has elapsed). Refused attempts count against the hits map, so the answer is capped at the
// limit: consumption can never exceed the quota. It never mutates the counters.
func (l *Limiter) Used(key string, now time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.start) >= l.window {
		return 0
	}
	if u := l.hits[key]; u < l.limit {
		return u
	}
	return l.limit
}

// ClientIP is the TCP peer address (the reverse proxy's when one fronts Stride).
func ClientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
