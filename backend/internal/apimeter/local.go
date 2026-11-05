package apimeter

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Limits caps concurrent calls. When PerUser > 0, Global counts users holding at least one slot
// (Garmin keys are per user) instead of individual calls.
type Limits struct {
	Global  int
	PerUser int
}

func DefaultLimits() map[string]Limits {
	return map[string]Limits{
		Garmin:    {Global: 4, PerUser: 6},
		Strava:    {Global: 2},
		OpenMeteo: {Global: 2},
		LLM:       {Global: 4},
		Webhook:   {Global: 4},
	}
}

const (
	recentErrorsMax = 50
	bucketCount     = 24 * 60
	cooldownBase    = 30 * time.Second
	cooldownMax     = 15 * time.Minute
	retryAfterMax   = 24 * time.Hour
)

type Options struct {
	Limits map[string]Limits
	// Sink persists error events asynchronously (e.g. into api_events); optional.
	Sink func(context.Context, []ErrorEvent) error
	// Recent preloads the recent errors ring (newest first), e.g. from api_events at startup.
	Recent []ErrorEvent
	// OnCall receives every observed call (audit trail); it must not block.
	OnCall func(Call)
	Now    func() time.Time
	Log    *slog.Logger
}

type bucket struct {
	minute      int64
	requests    int
	errors      int
	rateLimited int
	latency     time.Duration
	latencyN    int
}

type providerState struct {
	limits      Limits
	inflight    int
	activeUsers int
	users       map[int64]int
	changed     chan struct{}

	cooldownUntil time.Time
	consecutive   int
	quota         *Quota
	buckets       [bucketCount]bucket
}

// Local is the in-process Meter implementation.
type Local struct {
	mu        sync.Mutex
	providers map[string]*providerState
	recent    []ErrorEvent
	limits    map[string]Limits
	now       func() time.Time
	log       *slog.Logger

	sink   func(context.Context, []ErrorEvent) error
	onCall func(Call)
	events chan ErrorEvent
	done   chan struct{}
	closed bool
}

func NewLocal(o Options) *Local {
	m := &Local{
		providers: map[string]*providerState{},
		limits:    o.Limits,
		now:       o.Now,
		log:       o.Log,
		sink:      o.Sink,
		onCall:    o.OnCall,
	}
	if m.limits == nil {
		m.limits = DefaultLimits()
	}
	if m.now == nil {
		m.now = time.Now
	}
	if m.log == nil {
		m.log = slog.Default()
	}
	for _, p := range []string{Garmin, Strava, OpenMeteo} {
		m.state(p)
	}
	for i := len(o.Recent) - 1; i >= 0; i-- {
		m.pushRecent(o.Recent[i])
	}
	if m.sink != nil {
		m.events = make(chan ErrorEvent, 256)
		m.done = make(chan struct{})
		go m.persistLoop()
	}
	return m
}

func (m *Local) state(provider string) *providerState {
	p, ok := m.providers[provider]
	if !ok {
		p = &providerState{limits: m.limits[provider], users: map[int64]int{}, changed: make(chan struct{})}
		m.providers[provider] = p
	}
	return p
}

func (m *Local) Acquire(ctx context.Context, provider string, userID int64) (func(), error) {
	for {
		m.mu.Lock()
		p := m.state(provider)
		if now := m.now(); now.Before(p.cooldownUntil) {
			until := p.cooldownUntil
			m.mu.Unlock()
			return nil, &CooldownError{Provider: provider, Until: until}
		}
		if p.admit(userID) {
			m.mu.Unlock()
			var once sync.Once
			return func() { once.Do(func() { m.release(provider, userID) }) }, nil
		}
		wait := p.changed
		m.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (p *providerState) admit(userID int64) bool {
	l := p.limits
	if l.PerUser > 0 {
		held := p.users[userID]
		if held >= l.PerUser || (held == 0 && l.Global > 0 && p.activeUsers >= l.Global) {
			return false
		}
		if held == 0 {
			p.activeUsers++
		}
		p.users[userID] = held + 1
	} else if l.Global > 0 && p.inflight >= l.Global {
		return false
	}
	p.inflight++
	return true
}

func (m *Local) release(provider string, userID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.state(provider)
	p.inflight--
	if p.limits.PerUser > 0 {
		if p.users[userID]--; p.users[userID] <= 0 {
			delete(p.users, userID)
			p.activeUsers--
		}
	}
	close(p.changed)
	p.changed = make(chan struct{})
}

func (m *Local) Observe(c Call) {
	if m.onCall != nil {
		m.onCall(c)
	}
	now := m.now()
	m.mu.Lock()
	p := m.state(c.Provider)
	b := p.bucket(now)
	b.requests++
	if c.Duration > 0 {
		b.latency += c.Duration
		b.latencyN++
	}
	rateLimited := c.Status == http.StatusTooManyRequests || (c.Status == 0 && c.RetryAfter > 0)
	isErr := rateLimited || isFailure(c)
	switch {
	case rateLimited:
		b.rateLimited++
		p.consecutive++
		d := c.RetryAfter
		if d <= 0 {
			d = backoff(p.consecutive)
		}
		d = min(d, retryAfterMax)
		if until := now.Add(d); until.After(p.cooldownUntil) {
			p.cooldownUntil = until
		}
	case !isErr && c.Status > 0:
		p.consecutive = 0
	}
	if isErr {
		b.errors++
		ev := ErrorEvent{Time: now.UTC(), Provider: c.Provider, UserID: c.UserID, Endpoint: c.Endpoint, Status: c.Status, Message: errMessage(c)}
		m.pushRecent(ev)
		if m.events != nil && !m.closed {
			select {
			case m.events <- ev:
			default:
			}
		}
	}
	m.mu.Unlock()
	if rateLimited {
		m.log.Warn("🚦 Limite de débit atteinte, pause du fournisseur", "provider", c.Provider, "endpoint", c.Endpoint, "status", c.Status)
	}
}

func isFailure(c Call) bool {
	if c.Err != nil {
		return !errors.Is(c.Err, context.Canceled)
	}
	return c.Status >= 400 && c.Status != http.StatusNotFound
}

func backoff(n int) time.Duration {
	d := cooldownBase
	for i := 1; i < n && d < cooldownMax; i++ {
		d *= 2
	}
	return min(d, cooldownMax)
}

var queryRe = regexp.MustCompile(`\?[^\s"']*`)

func errMessage(c Call) string {
	msg := ""
	if c.Err != nil {
		msg = queryRe.ReplaceAllString(c.Err.Error(), "?…")
	} else if c.Status > 0 {
		msg = http.StatusText(c.Status)
	}
	if r := []rune(msg); len(r) > 300 {
		msg = string(r[:300]) + "…"
	}
	return msg
}

func (p *providerState) bucket(now time.Time) *bucket {
	minute := now.Unix() / 60
	b := &p.buckets[minute%bucketCount]
	if b.minute != minute {
		*b = bucket{minute: minute}
	}
	return b
}

func (m *Local) pushRecent(e ErrorEvent) {
	m.recent = append(m.recent, e)
	if len(m.recent) > recentErrorsMax {
		m.recent = m.recent[len(m.recent)-recentErrorsMax:]
	}
}

func (m *Local) SetQuota(provider string, q Quota) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if q.UpdatedAt.IsZero() {
		q.UpdatedAt = m.now().UTC()
	}
	m.state(provider).quota = &q
}

func (m *Local) Snapshot() Snapshot {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	minute := now.Unix() / 60
	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	order := map[string]int{Garmin: 0, Strava: 1, OpenMeteo: 2, LLM: 3, Webhook: 4}
	sort.Slice(names, func(i, j int) bool {
		oi, iok := order[names[i]]
		oj, jok := order[names[j]]
		if iok != jok {
			return iok
		}
		if iok {
			return oi < oj
		}
		return names[i] < names[j]
	})
	snap := Snapshot{Providers: make([]ProviderStats, 0, len(names)), RecentErrors: make([]ErrorEvent, 0, len(m.recent))}
	for _, name := range names {
		p := m.providers[name]
		st := ProviderStats{Provider: name, InFlight: p.inflight}
		var lat1h, lat24h time.Duration
		var n1h, n24h int
		for i := range p.buckets {
			b := &p.buckets[i]
			age := minute - b.minute
			if b.minute == 0 || age < 0 || age >= bucketCount {
				continue
			}
			st.Requests24h += b.requests
			st.RateLimited24h += b.rateLimited
			lat24h += b.latency
			n24h += b.latencyN
			if age < 60 {
				st.Requests1h += b.requests
				st.Errors1h += b.errors
				lat1h += b.latency
				n1h += b.latencyN
			}
		}
		switch {
		case n1h > 0:
			st.AvgLatencyMs = int((lat1h / time.Duration(n1h)).Milliseconds())
		case n24h > 0:
			st.AvgLatencyMs = int((lat24h / time.Duration(n24h)).Milliseconds())
		}
		if now.Before(p.cooldownUntil) {
			u := p.cooldownUntil.UTC()
			st.CooldownUntil = &u
		}
		if p.quota != nil {
			q := *p.quota
			st.Quota = &q
		}
		snap.Providers = append(snap.Providers, st)
	}
	for i := len(m.recent) - 1; i >= 0; i-- {
		snap.RecentErrors = append(snap.RecentErrors, m.recent[i])
	}
	return snap
}

func (m *Local) persistLoop() {
	defer close(m.done)
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	var batch []ErrorEvent
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := m.sink(ctx, batch); err != nil {
			m.log.Warn("⚠️ Persistance des événements API impossible", "err", err.Error(), "events", len(batch))
		}
		cancel()
		batch = nil
	}
	for {
		select {
		case ev, ok := <-m.events:
			if !ok {
				flush()
				return
			}
			if batch = append(batch, ev); len(batch) >= 50 {
				flush()
			}
		case <-tick.C:
			flush()
		}
	}
}

// Close flushes pending events; later errors are kept in memory only.
func (m *Local) Close() {
	m.mu.Lock()
	if m.closed || m.events == nil {
		m.closed = true
		m.mu.Unlock()
		return
	}
	m.closed = true
	close(m.events)
	m.mu.Unlock()
	<-m.done
}
