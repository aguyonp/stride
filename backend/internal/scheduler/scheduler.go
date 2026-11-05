// Package scheduler runs the daily morning sync (users one by one, with random jitter) and the hourly maintenance.
package scheduler

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"stride/internal/apimeter"
	"stride/internal/logx"
)

type Options struct {
	Hour        int
	JitterMin   time.Duration
	JitterMax   time.Duration
	Location    *time.Location
	MinInterval time.Duration // skip users synced more recently (default 3 h)
	SyncTimeout time.Duration // per user (default 10 min)
	PurgeEvery  time.Duration // default 1 h

	// Users lists the users to prefetch (active users with a connected provider).
	Users func(ctx context.Context) ([]int64, error)
	Sync  func(ctx context.Context, userID int64) error
	// LastSync optionally reports the last successful sync of a user; defaults to the scheduler's own record.
	LastSync func(ctx context.Context, userID int64) (time.Time, error)
	// Purge runs at startup then every PurgeEvery (expired sessions, invitations, old api events).
	Purge func(ctx context.Context) error

	Log   *slog.Logger
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration) error
	Rand  func(n int64) int64
}

type Result struct {
	Synced, Skipped, Failed int
}

type Scheduler struct {
	o    Options
	mu   sync.Mutex
	last map[int64]time.Time
}

func New(o Options) *Scheduler {
	if o.Location == nil {
		o.Location = time.Local
	}
	if o.MinInterval <= 0 {
		o.MinInterval = 3 * time.Hour
	}
	if o.SyncTimeout <= 0 {
		o.SyncTimeout = 10 * time.Minute
	}
	if o.PurgeEvery <= 0 {
		o.PurgeEvery = time.Hour
	}
	if o.JitterMax < o.JitterMin {
		o.JitterMax = o.JitterMin
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Sleep == nil {
		o.Sleep = sleep
	}
	if o.Rand == nil {
		o.Rand = rand.Int64N
	}
	return &Scheduler{o: o, last: map[int64]time.Time{}}
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// NextRun returns the next occurrence of hour:00 in loc strictly after now.
func NextRun(now time.Time, hour int, loc *time.Location) time.Time {
	t := now.In(loc)
	next := time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, loc)
	if !next.After(t) {
		next = time.Date(t.Year(), t.Month(), t.Day()+1, hour, 0, 0, 0, loc)
	}
	return next
}

// MarkSynced records a successful sync (also usable by the lazy sync path).
func (s *Scheduler) MarkSynced(userID int64, at time.Time) {
	s.mu.Lock()
	s.last[userID] = at
	s.mu.Unlock()
}

func (s *Scheduler) lastSync(ctx context.Context, userID int64) time.Time {
	s.mu.Lock()
	t := s.last[userID]
	s.mu.Unlock()
	if s.o.LastSync != nil {
		if ext, err := s.o.LastSync(ctx, userID); err == nil && ext.After(t) {
			t = ext
		}
	}
	return t
}

func (s *Scheduler) jitter() time.Duration {
	span := int64(s.o.JitterMax - s.o.JitterMin)
	if span <= 0 {
		return s.o.JitterMin
	}
	return s.o.JitterMin + time.Duration(s.o.Rand(span+1))
}

// RunCycle syncs every listed user sequentially, pausing a random jitter between two syncs.
func (s *Scheduler) RunCycle(ctx context.Context) Result {
	var res Result
	ids, err := s.o.Users(ctx)
	if err != nil {
		s.o.Log.Error("❌ Liste des utilisateurs à synchroniser indisponible", "err", err.Error())
		return res
	}
	started := false
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		if last := s.lastSync(ctx, id); !last.IsZero() && s.o.Now().Sub(last) < s.o.MinInterval {
			res.Skipped++
			s.o.Log.Debug("⏭️ Synchronisation récente, utilisateur ignoré", "userId", id)
			continue
		}
		if started {
			if err := s.o.Sleep(ctx, s.jitter()); err != nil {
				break
			}
		}
		started = true
		start := s.o.Now()
		sctx, cancel := context.WithTimeout(apimeter.WithUser(ctx, id), s.o.SyncTimeout)
		err := s.o.Sync(sctx, id)
		cancel()
		if err != nil {
			res.Failed++
			s.o.Log.Warn("⚠️ Synchronisation matinale en échec", "userId", id, "err", err.Error())
			continue
		}
		res.Synced++
		s.MarkSynced(id, s.o.Now())
		s.o.Log.Info("🌅 Utilisateur synchronisé", "userId", id, "durée", logx.Ms(s.o.Now().Sub(start)))
	}
	return res
}

// waitUntil sleeps in steps of at most one hour so clock changes are noticed.
func (s *Scheduler) waitUntil(ctx context.Context, t time.Time) error {
	for {
		d := t.Sub(s.o.Now())
		if d <= 0 {
			return ctx.Err()
		}
		if err := s.o.Sleep(ctx, min(d, time.Hour)); err != nil {
			return err
		}
	}
}

func (s *Scheduler) runDaily(ctx context.Context) {
	for {
		next := NextRun(s.o.Now(), s.o.Hour, s.o.Location)
		s.o.Log.Info("⏰ Prochaine synchronisation matinale planifiée", "à", next.Format("2006-01-02 15:04 MST"))
		if err := s.waitUntil(ctx, next); err != nil {
			return
		}
		start := s.o.Now()
		res := s.RunCycle(ctx)
		s.o.Log.Info("🌅 Synchronisation matinale terminée", "synchronisés", res.Synced, "ignorés", res.Skipped,
			"échecs", res.Failed, "durée", logx.Ms(s.o.Now().Sub(start)))
		if ctx.Err() != nil {
			return
		}
	}
}

func (s *Scheduler) runPurge(ctx context.Context) {
	if s.o.Purge == nil {
		return
	}
	for {
		if err := s.o.Purge(ctx); err != nil && ctx.Err() == nil {
			s.o.Log.Warn("⚠️ Maintenance horaire en échec", "err", err.Error())
		}
		if err := s.o.Sleep(ctx, s.o.PurgeEvery); err != nil {
			return
		}
	}
}

// Run blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.runPurge(ctx)
	}()
	s.runDaily(ctx)
	wg.Wait()
}
