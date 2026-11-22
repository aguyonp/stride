// usage.go gives the admin a view of the public MCP route: the active quota (tunable,
// persisted in mcp_quota) and rolling in-memory counters per token (24 h, 5-min buckets),
// in the spirit of the provider meter. The counters reset on restart.
package mcp

import (
	"context"
	"database/sql"
	"encoding/hex"
	"sync"
	"time"

	"stride/internal/ratelimit"
)

// Quota bounds the public route POST /mcp/{token}: requests per window, window in minutes.
type Quota struct {
	TokenLimit  int `json:"tokenLimit"`
	TokenWindow int `json:"tokenWindow"` // minutes
	IPLimit     int `json:"ipLimit"`
	IPWindow    int `json:"ipWindow"` // minutes
}

// DefaultQuota is the seeded row and the fallback when mcp_quota is unreadable.
var DefaultQuota = Quota{TokenLimit: 100, TokenWindow: 10, IPLimit: 200, IPWindow: 10}

// Sanitize clamps admin input inside sane bounds instead of rejecting it.
func (q Quota) Sanitize() Quota {
	q.TokenLimit = clampInt(q.TokenLimit, 1, 100000)
	q.TokenWindow = clampInt(q.TokenWindow, 1, 1440)
	q.IPLimit = clampInt(q.IPLimit, 1, 100000)
	q.IPWindow = clampInt(q.IPWindow, 1, 1440)
	return q
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

const (
	usageBucket  = 5 * time.Minute
	usageBuckets = int(24 * time.Hour / usageBucket) // 288 buckets cover the last 24 h
)

// usageRing is a flat counter of the last 24 h in 5-minute buckets, like the provider meter:
// one bucket per slot, the slot's stamp resets it when it is overwritten.
type usageRing struct {
	stamp [usageBuckets]int64
	count [usageBuckets]int
}

func (r *usageRing) add(now time.Time) {
	s := now.Unix() / int64(usageBucket.Seconds())
	i := int(s % int64(usageBuckets))
	if r.stamp[i] != s {
		r.stamp[i] = s
		r.count[i] = 0
	}
	r.count[i]++
}

// total sums every slot: 288 consecutive buckets are exactly 24 h.
func (r *usageRing) total() int {
	sum := 0
	for _, c := range r.count {
		sum += c
	}
	return sum
}

// tokenUsage holds the rolling counters of one MCP token (requests served, 429 refused).
type tokenUsage struct {
	requests usageRing
	denied   usageRing
}

// meter owns the public route's rate limits and its rolling usage counters.
type meter struct {
	mu     sync.Mutex
	limT   *ratelimit.Limiter
	limI   *ratelimit.Limiter
	quota  Quota
	tokens map[string]*tokenUsage // key: hex SHA-256 of the token
	denied usageRing              // every 429 (IP, known and unknown tokens), 24 h
}

func newMeter(q Quota) *meter {
	return &meter{
		limT:   ratelimit.New(q.TokenLimit, time.Duration(q.TokenWindow)*time.Minute),
		limI:   ratelimit.New(q.IPLimit, time.Duration(q.IPWindow)*time.Minute),
		quota:  q,
		tokens: map[string]*tokenUsage{},
	}
}

// apply takes a new quota (already persisted by the caller) and restarts the limiters.
func (m *meter) apply(q Quota) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quota = q
	m.limT.SetConfig(q.TokenLimit, time.Duration(q.TokenWindow)*time.Minute)
	m.limI.SetConfig(q.IPLimit, time.Duration(q.IPWindow)*time.Minute)
}

func (m *meter) entry(key string) *tokenUsage {
	u := m.tokens[key]
	if u == nil {
		u = &tokenUsage{}
		m.tokens[key] = u
	}
	return u
}

// noteDenied counts one 429 (IP budget or an unknown token): global counter only.
func (m *meter) noteDenied(now time.Time) {
	m.mu.Lock()
	m.denied.add(now)
	m.mu.Unlock()
}

// noteTokenDenied counts one 429 of a known token: global and per-token counters.
func (m *meter) noteTokenDenied(key string, now time.Time) {
	m.mu.Lock()
	m.denied.add(now)
	m.entry(key).denied.add(now)
	m.mu.Unlock()
}

// noteRequest counts one served request of a known token.
func (m *meter) noteRequest(key string, now time.Time) {
	m.mu.Lock()
	m.entry(key).requests.add(now)
	m.mu.Unlock()
}

// snapshot returns a consistent view and drops stale entries (revoked tokens whose 24 h
// activity has rolled out), so the map stays bounded by the live token count.
func (m *meter) snapshot() (Quota, map[string]tokenUsage, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]tokenUsage, len(m.tokens))
	for k, u := range m.tokens {
		if u.requests.total() == 0 && u.denied.total() == 0 {
			delete(m.tokens, k)
			continue
		}
		out[k] = *u
	}
	return m.quota, out, m.denied.total()
}

// TokenUsage is the admin view of one MCP token.
type TokenUsage struct {
	Prefix      string     `json:"prefix"`
	UserID      int64      `json:"userId"`
	DisplayName string     `json:"displayName"`
	Enabled     bool       `json:"enabled"`
	LastUsedAt  *time.Time `json:"lastUsedAt"`
	Used        int        `json:"used"` // requests in the current token window
	Denied24h   int        `json:"denied24h"`
	Requests24h int        `json:"requests24h"`
}

// Usage is the admin view of the public MCP route: the active quota, the rolling counters
// per token and the global 429 count of the last 24 h.
type Usage struct {
	Quota       Quota        `json:"quota"`
	Tokens      []TokenUsage `json:"tokens"`
	Requests24h int          `json:"requests24h"`
	Denied24h   int          `json:"denied24h"`
	UpdatedAt   time.Time    `json:"updatedAt"`
}

// Usage builds the admin view: one row per stored token, joined with the rolling counters.
func (s *Server) Usage(ctx context.Context) (Usage, error) {
	rows, err := s.tok.read.QueryContext(ctx, `SELECT m.token_hash, m.token_prefix, m.user_id, m.enabled, m.last_used_at, COALESCE(u.display_name, '')
		FROM mcp_access m JOIN users u ON u.id = m.user_id
		WHERE m.token_hash IS NOT NULL
		ORDER BY COALESCE(m.last_used_at, 0) DESC, m.user_id`)
	if err != nil {
		return Usage{}, err
	}
	defer rows.Close()

	now := time.Now()
	q, counters, denied := s.met.snapshot()
	u := Usage{Quota: q, Tokens: []TokenUsage{}, Denied24h: denied, UpdatedAt: now.UTC()}
	for rows.Next() {
		var (
			hash   []byte
			prefix string
			uid    int64
			ena    int
			last   sql.NullInt64
			name   string
		)
		if err := rows.Scan(&hash, &prefix, &uid, &ena, &last, &name); err != nil {
			return Usage{}, err
		}
		key := hex.EncodeToString(hash)
		tu := TokenUsage{Prefix: prefix, UserID: uid, DisplayName: name, Enabled: ena != 0, Used: s.met.limT.Used(key, now)}
		if last.Valid {
			t := time.Unix(last.Int64, 0).UTC()
			tu.LastUsedAt = &t
		}
		if c, ok := counters[key]; ok {
			tu.Requests24h = c.requests.total()
			tu.Denied24h = c.denied.total()
		}
		u.Requests24h += tu.Requests24h
		u.Tokens = append(u.Tokens, tu)
	}
	return u, rows.Err()
}

// SetQuota persists q (clamped) and applies it to the live limiters.
func (s *Server) SetQuota(ctx context.Context, q Quota) (Quota, error) {
	q = q.Sanitize()
	if err := mcpSetQuota(ctx, s.tok.db, q); err != nil {
		return Quota{}, err
	}
	s.met.apply(q)
	return q, nil
}
