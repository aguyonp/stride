package domainapi

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// shareRateLimit is the number of public share views allowed per client IP and minute.
const shareRateLimit = 30

// Portal writes (§15.1): 30 per 10 minutes and token, 60 per 10 minutes and client IP.
const (
	portalWriteWindow   = 10 * time.Minute
	portalWritesByToken = 30
	portalWritesByIP    = 60
)

// ipLimiter counts requests per key (client IP, token hash) in a fixed window shared by all keys,
// which keeps memory bounded without a janitor.
type ipLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	start  time.Time
	hits   map[string]int
}

func newIPLimiter(limit int) *ipLimiter { return newWindowLimiter(limit, time.Minute) }

func newWindowLimiter(limit int, window time.Duration) *ipLimiter {
	return &ipLimiter{limit: limit, window: window, hits: map[string]int{}}
}

func (l *ipLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.start) >= l.window {
		l.start = now
		clear(l.hits)
	}
	l.hits[key]++
	return l.hits[key] <= l.limit
}

// clientIP is the TCP peer address (the reverse proxy's when one fronts Stride).
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
