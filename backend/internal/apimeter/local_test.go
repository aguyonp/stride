package apimeter

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newClock() *fakeClock { return &fakeClock{t: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)} }

func TestGlobalConcurrencyLimit(t *testing.T) {
	m := NewLocal(Options{Limits: map[string]Limits{Strava: {Global: 2}}})
	var cur, peak atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rel, err := m.Acquire(context.Background(), Strava, int64(i))
			if err != nil {
				t.Error(err)
				return
			}
			n := cur.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			cur.Add(-1)
			rel()
			rel() // idempotent
		}()
	}
	wg.Wait()
	if peak.Load() != 2 {
		t.Fatalf("peak=%d, want 2", peak.Load())
	}
	if s := m.Snapshot(); s.Providers[1].InFlight != 0 {
		t.Fatalf("inflight leak: %+v", s.Providers[1])
	}
}

func TestPerUserLimitAndUserSlots(t *testing.T) {
	m := NewLocal(Options{Limits: map[string]Limits{Garmin: {Global: 2, PerUser: 3}}})
	ctx := context.Background()
	var rels []func()
	for i := 0; i < 3; i++ {
		r, err := m.Acquire(ctx, Garmin, 1)
		if err != nil {
			t.Fatal(err)
		}
		rels = append(rels, r)
	}
	short, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	if _, err := m.Acquire(short, Garmin, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("4th call for user 1 must wait: %v", err)
	}
	cancel()
	r2, err := m.Acquire(ctx, Garmin, 2)
	if err != nil {
		t.Fatal(err)
	}
	short, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
	if _, err := m.Acquire(short, Garmin, 3); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("third user must wait for a user slot: %v", err)
	}
	cancel()
	got := make(chan error, 1)
	go func() {
		r, err := m.Acquire(ctx, Garmin, 3)
		if err == nil {
			r()
		}
		got <- err
	}()
	r2()
	if err := <-got; err != nil {
		t.Fatal(err)
	}
	for _, r := range rels {
		r()
	}
	if s := m.Snapshot(); s.Providers[0].InFlight != 0 {
		t.Fatalf("inflight leak: %+v", s.Providers[0])
	}
}

func TestCooldownRetryAfterAndBackoff(t *testing.T) {
	clk := newClock()
	m := NewLocal(Options{Now: clk.Now})
	ctx := context.Background()

	m.Observe(Call{Provider: Strava, Endpoint: "GET /athlete/activities", Status: 429, RetryAfter: 2 * time.Minute})
	_, err := m.Acquire(ctx, Strava, 1)
	var ce *CooldownError
	if !errors.As(err, &ce) || !errors.Is(err, ErrCooldown) || !ce.Until.Equal(clk.Now().Add(2*time.Minute)) {
		t.Fatalf("cooldown: %v", err)
	}
	clk.Add(2*time.Minute + time.Second)
	rel, err := m.Acquire(ctx, Strava, 1)
	if err != nil {
		t.Fatal(err)
	}
	rel()

	// without Retry-After: 30s, 60s, 120s ... capped at 15 min
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute}
	for i, w := range want {
		m.Observe(Call{Provider: Garmin, Status: 429})
		snap := m.Snapshot()
		if got := snap.Providers[0].CooldownUntil.Sub(clk.Now()); got != w {
			t.Fatalf("step %d: cooldown %s, want %s", i, got, w)
		}
		clk.Add(w)
	}
	m.Observe(Call{Provider: Garmin, Status: 200})
	m.Observe(Call{Provider: Garmin, Status: 429})
	if got := m.Snapshot().Providers[0].CooldownUntil.Sub(clk.Now()); got != 30*time.Second {
		t.Fatalf("backoff not reset after success: %s", got)
	}
}

func TestCountersLatencyAndRecentErrors(t *testing.T) {
	clk := newClock()
	var mu sync.Mutex
	var persisted []ErrorEvent
	m := NewLocal(Options{Now: clk.Now, Sink: func(_ context.Context, evs []ErrorEvent) error {
		mu.Lock()
		persisted = append(persisted, evs...)
		mu.Unlock()
		return nil
	}, Recent: []ErrorEvent{{Provider: Garmin, Message: "old"}}})

	m.Observe(Call{Provider: OpenMeteo, Endpoint: "GET /v1/forecast", Status: 200, Duration: 100 * time.Millisecond})
	m.Observe(Call{Provider: OpenMeteo, Endpoint: "GET /v1/forecast", Status: 200, Duration: 300 * time.Millisecond})
	m.Observe(Call{Provider: OpenMeteo, Endpoint: "GET /v1/archive", Status: 404})
	m.Observe(Call{Provider: OpenMeteo, Endpoint: "GET /v1/archive", Err: context.Canceled})
	clk.Add(2 * time.Hour)
	m.Observe(Call{Provider: OpenMeteo, Endpoint: "GET /v1/archive", Status: 502, Duration: 50 * time.Millisecond})
	m.Observe(Call{Provider: OpenMeteo, Endpoint: "GET /v1/archive", Err: errors.New(`Get "https://x/v1/archive?latitude=1": timeout`)})

	snap := m.Snapshot()
	om := snap.Providers[2]
	if om.Provider != OpenMeteo || om.Requests24h != 6 || om.Requests1h != 2 || om.Errors1h != 2 || om.AvgLatencyMs != 50 {
		t.Fatalf("stats: %+v", om)
	}
	if len(snap.RecentErrors) != 3 || snap.RecentErrors[0].Status != 0 || snap.RecentErrors[2].Message != "old" {
		t.Fatalf("recent: %+v", snap.RecentErrors)
	}
	if msg := snap.RecentErrors[0].Message; msg != `Get "https://x/v1/archive?…": timeout` {
		t.Fatalf("query string must be stripped: %q", msg)
	}
	clk.Add(23 * time.Hour)
	if s := m.Snapshot().Providers[2]; s.Requests24h != 2 || s.Requests1h != 0 {
		t.Fatalf("window: %+v", s)
	}
	for i := 0; i < 60; i++ {
		m.Observe(Call{Provider: Strava, Status: 500})
	}
	if n := len(m.Snapshot().RecentErrors); n != 50 {
		t.Fatalf("ring size %d", n)
	}
	m.Close()
	m.Observe(Call{Provider: Strava, Status: 500})
	mu.Lock()
	defer mu.Unlock()
	if len(persisted) != 62 {
		t.Fatalf("persisted %d events, want 62", len(persisted))
	}
}

func TestQuotaAndCooldownErrorWhileWaiting(t *testing.T) {
	m := NewLocal(Options{Limits: map[string]Limits{Strava: {Global: 1}}})
	m.SetQuota(Strava, Quota{Limit15m: 100, Usage15m: 7, LimitDaily: 1000, UsageDaily: 70})
	if q := m.Snapshot().Providers[1].Quota; q == nil || q.Usage15m != 7 || q.UpdatedAt.IsZero() {
		t.Fatalf("quota: %+v", q)
	}
	rel, _ := m.Acquire(context.Background(), Strava, 0)
	done := make(chan error, 1)
	go func() {
		_, err := m.Acquire(context.Background(), Strava, 0)
		done <- err
	}()
	time.Sleep(10 * time.Millisecond)
	m.Observe(Call{Provider: Strava, Status: 429, RetryAfter: time.Minute})
	rel()
	if err := <-done; !errors.Is(err, ErrCooldown) {
		t.Fatalf("waiter must fail fast once cooling down: %v", err)
	}
}
