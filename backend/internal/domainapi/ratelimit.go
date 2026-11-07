package domainapi

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// shareRateLimit is the number of public share views allowed per client IP and minute.
const shareRateLimit = 30

// ipLimiter counts requests per client IP in a fixed one-minute window shared by all IPs,
// which keeps memory bounded without a janitor.
type ipLimiter struct {
	mu    sync.Mutex
	limit int
	start time.Time
	hits  map[string]int
}

func newIPLimiter(limit int) *ipLimiter { return &ipLimiter{limit: limit, hits: map[string]int{}} }

func (l *ipLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.start) >= time.Minute {
		l.start = now
		clear(l.hits)
	}
	l.hits[ip]++
	return l.hits[ip] <= l.limit
}

// clientIP is the TCP peer address (the reverse proxy's when one fronts Stride).
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
