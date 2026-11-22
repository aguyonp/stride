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

// ClientIP is the TCP peer address (the reverse proxy's when one fronts Stride).
func ClientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
