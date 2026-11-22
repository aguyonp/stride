package domainapi

import (
	"net/http"
	"time"

	"stride/internal/ratelimit"
)

// shareRateLimit is the number of public share views allowed per client IP and minute.
const shareRateLimit = 30

// Portal writes (§15.1): 30 per 10 minutes and token, 60 per 10 minutes and client IP.
const (
	portalWriteWindow   = 10 * time.Minute
	portalWritesByToken = 30
	portalWritesByIP    = 60
)

// ipLimiter is the shared fixed-window limiter, keyed by client IP or token hash.
type ipLimiter = ratelimit.Limiter

func newIPLimiter(limit int) *ipLimiter { return ratelimit.New(limit, time.Minute) }

func newWindowLimiter(limit int, window time.Duration) *ipLimiter {
	return ratelimit.New(limit, window)
}

// clientIP is the TCP peer address (the reverse proxy's when one fronts Stride).
func clientIP(r *http.Request) string { return ratelimit.ClientIP(r) }
