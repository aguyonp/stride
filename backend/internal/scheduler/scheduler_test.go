package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"stride/internal/apimeter"
)

type clock struct {
	mu     sync.Mutex
	now    time.Time
	sleeps []time.Duration
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }

func (c *clock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
	c.mu.Unlock()
	return nil
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestNextRun(t *testing.T) {
	paris, _ := time.LoadLocation("Europe/Paris")
	cases := []struct{ now, want string }{
		{"2026-11-04T05:30:00+01:00", "2026-11-04T07:00:00+01:00"},
		{"2026-11-04T07:00:00+01:00", "2026-11-05T07:00:00+01:00"},
		{"2026-11-04T23:10:00+01:00", "2026-11-05T07:00:00+01:00"},
		{"2026-03-28T08:00:00+01:00", "2026-03-29T07:00:00+02:00"}, // DST change
	}
	for _, c := range cases {
		now, _ := time.Parse(time.RFC3339, c.now)
		want, _ := time.Parse(time.RFC3339, c.want)
		if got := NextRun(now, 7, paris); !got.Equal(want) {
			t.Errorf("NextRun(%s) = %s, want %s", c.now, got, want)
		}
	}
}

func TestCycleJitterSkipAndFailures(t *testing.T) {
	clk := &clock{now: time.Date(2026, 11, 4, 7, 0, 0, 0, time.UTC)}
	var synced []int64
	s := New(Options{
		JitterMin: 30 * time.Second, JitterMax: 90 * time.Second, Log: quiet(), Now: clk.Now, Sleep: clk.Sleep,
		Rand:  func(n int64) int64 { return n - 1 },
		Users: func(context.Context) ([]int64, error) { return []int64{1, 2, 3, 4, 5}, nil },
		Sync: func(ctx context.Context, id int64) error {
			if apimeter.UserFrom(ctx) != id {
				t.Errorf("sync context not attributed to user %d", id)
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Error("sync must have a timeout")
			}
			synced = append(synced, id)
			if id == 4 {
				return errors.New("provider down")
			}
			return nil
		},
		LastSync: func(_ context.Context, id int64) (time.Time, error) {
			if id == 5 {
				return clk.Now().Add(-4 * time.Hour), nil
			}
			return time.Time{}, nil
		},
	})
	s.MarkSynced(2, clk.Now().Add(-time.Hour))
	res := s.RunCycle(context.Background())
	if res != (Result{Synced: 3, Skipped: 1, Failed: 1}) {
		t.Fatalf("result %+v", res)
	}
	if len(synced) != 4 || synced[1] != 3 {
		t.Fatalf("order %v", synced)
	}
	if len(clk.sleeps) != 3 {
		t.Fatalf("jitter pauses between syncs: %v", clk.sleeps)
	}
	for _, d := range clk.sleeps {
		if d < 30*time.Second || d > 90*time.Second {
			t.Fatalf("jitter out of bounds: %s", d)
		}
	}

	// a second cycle right after skips everyone synced successfully
	synced = nil
	res = s.RunCycle(context.Background())
	if res.Skipped != 4 || res.Synced != 0 || res.Failed != 1 || len(synced) != 1 || synced[0] != 4 {
		t.Fatalf("second cycle %+v %v", res, synced)
	}
}

func TestJitterRangeWithRealRand(t *testing.T) {
	s := New(Options{JitterMin: 30 * time.Second, JitterMax: 90 * time.Second})
	for i := 0; i < 1000; i++ {
		if d := s.jitter(); d < 30*time.Second || d > 90*time.Second {
			t.Fatalf("jitter %s", d)
		}
	}
}

func TestCancelStopsCycleAndRun(t *testing.T) {
	clk := &clock{now: time.Date(2026, 11, 4, 6, 59, 0, 0, time.UTC)}
	ctx, cancel := context.WithCancel(context.Background())
	var synced []int64
	var purges int
	s := New(Options{
		Hour: 7, Location: time.UTC, JitterMin: time.Second, JitterMax: time.Second, Log: quiet(), Now: clk.Now,
		Sleep:      clk.Sleep,
		PurgeEvery: time.Hour,
		Users:      func(context.Context) ([]int64, error) { return []int64{1, 2, 3}, nil },
		Sync: func(_ context.Context, id int64) error {
			synced = append(synced, id)
			if id == 2 {
				cancel()
			}
			return nil
		},
		Purge: func(context.Context) error { purges++; return nil },
	})
	done := make(chan struct{})
	go func() {
		s.runDaily(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runDaily did not stop")
	}
	if len(synced) != 2 {
		t.Fatalf("cancel must stop the cycle: %v", synced)
	}
	if clk.sleeps[0] != time.Minute {
		t.Fatalf("first wait must reach 07:00: %v", clk.sleeps)
	}

	pctx, pcancel := context.WithCancel(context.Background())
	n := 0
	s.o.Sleep = func(ctx context.Context, d time.Duration) error {
		if n++; n == 3 {
			pcancel()
		}
		return ctx.Err()
	}
	s.runPurge(pctx)
	if purges != 3 {
		t.Fatalf("purges=%d", purges)
	}
}
