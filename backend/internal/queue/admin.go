package queue

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"
)

// Admin control center: cross-user listing, worker pool load, cancellation and replay of failed sync jobs.

var (
	// ErrNotCancellable is returned by Cancel for a job that is finished or not held by this process.
	ErrNotCancellable = errors.New("job not cancellable")
	// ErrNotReplayable is returned by Retry for a job that did not fail or whose kind cannot be replayed.
	ErrNotReplayable = errors.New("job not replayable")
)

// replayableKinds store their payload (dates, flags, Strava event ids) so an administrator can replay
// them. Imports (file bytes), LLM jobs (prompts) and webhook deliveries (message bodies) never do.
var replayableKinds = []string{KindSyncProvider, KindSyncResync, KindSyncHistory, KindSyncStravaWebhook}

// Replayable tells whether jobs of this kind can be retried from the administration.
func Replayable(kind string) bool { return slices.Contains(replayableKinds, kind) }

func replayPayload(it item) *string {
	if !Replayable(it.kind) {
		return nil
	}
	s := string(it.payload)
	return &s
}

type delayedJob struct {
	timer *time.Timer
	it    item
}

// GroupStats is the live load of one worker pool (this process only).
type GroupStats struct {
	Group   string `json:"group"`
	Workers int    `json:"workers"`
	Running int    `json:"running"`
	Queued  int    `json:"queued"`
	Delayed int    `json:"delayed"`
}

// Stats returns the load of every worker pool in a fixed order (sync, llm, webhook).
func (m *Manager) Stats() []GroupStats {
	out := make([]GroupStats, 0, 3)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, g := range []string{GroupSync, GroupLLM, GroupWebhook} {
		gs := GroupStats{Group: g, Workers: m.concurrency[g], Running: m.active[g]}
		if l := m.lanes[g]; l != nil {
			gs.Queued = l.len()
		}
		for _, d := range m.delayed {
			if groupOf(d.it.kind) == g {
				gs.Delayed++
			}
		}
		out = append(out, gs)
	}
	return out
}

// AdminFilter selects jobs across users: "active" (queued, running, retrying), "failed" or "" (all).
type AdminFilter struct {
	State string
	Limit int
}

// ListAll returns the latest jobs of every user, newest first (default 50, max 200).
func (m *Manager) ListAll(ctx context.Context, f AdminFilter) ([]JobStatus, error) {
	out := []JobStatus{}
	if m.db == nil {
		return out, nil
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	where, args := "", []any{}
	switch f.State {
	case "active":
		where, args = ` WHERE state IN (?, ?, ?)`, []any{StateQueued, StateRunning, StateRetrying}
	case StateFailed:
		where, args = ` WHERE state = ?`, []any{StateFailed}
	}
	rows, err := m.db.QueryContext(ctx, `SELECT `+jobCols+` FROM async_jobs`+where+` ORDER BY updated_at DESC, rowid DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return out, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// CountFailedSince counts the jobs that ended in failure after since.
func (m *Manager) CountFailedSince(ctx context.Context, since time.Time) (int, error) {
	if m.db == nil {
		return 0, nil
	}
	var n int
	err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM async_jobs WHERE state = ? AND updated_at >= ?`, StateFailed, since.Unix()).Scan(&n)
	return n, err
}

// Lookup returns any user's job by id (administration only).
func (m *Manager) Lookup(ctx context.Context, jobID string) (*JobStatus, error) {
	if m.db == nil {
		return nil, ErrNotFound
	}
	j, err := scanJob(m.db.QueryRowContext(ctx, `SELECT `+jobCols+` FROM async_jobs WHERE id = ?`, jobID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// Cancel stops a job of any user: a queued or retry-scheduled job fails at once, a running one has
// its context cancelled and fails when its handler returns. Finished jobs give ErrNotCancellable.
func (m *Manager) Cancel(ctx context.Context, jobID string) (*JobStatus, error) {
	cur, err := m.Lookup(ctx, jobID)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if cancel, ok := m.runCancel[jobID]; ok {
		m.cancelled[jobID] = true
		m.mu.Unlock()
		cancel()
		return cur, nil
	}
	if d, ok := m.delayed[jobID]; ok && d.timer.Stop() {
		delete(m.delayed, jobID)
		delete(m.timers, d.timer)
		m.mu.Unlock()
		return m.abort(d.it), nil
	}
	m.mu.Unlock()
	for _, l := range m.lanes {
		if it, ok := l.remove(jobID); ok {
			return m.abort(it), nil
		}
	}
	return nil, ErrNotCancellable
}

// abort fails a job that never reached (or went back to) its worker.
func (m *Manager) abort(it item) *JobStatus {
	m.metrics.AddQueueDepth(it.queue, -1)
	m.finish(it, nil, Permanent(msgCancelled, nil), 0)
	st, err := m.Lookup(context.Background(), it.id)
	if err != nil {
		return &JobStatus{ID: it.id, UserID: it.userID, Kind: it.kind, Queue: it.queue, State: StateFailed, Label: it.label}
	}
	return st
}

// Retry enqueues a new job with the kind, label, attempts and stored payload of a failed replayable job.
func (m *Manager) Retry(ctx context.Context, jobID string) (*JobStatus, error) {
	if m.db == nil {
		return nil, ErrNotFound
	}
	var (
		uid                int64
		kind, queue, label string
		state              string
		maxAttempts        int
		payload            sql.NullString
	)
	err := m.db.QueryRowContext(ctx, `SELECT user_id, kind, queue, label, state, max_attempts, payload_json FROM async_jobs WHERE id = ?`, jobID).
		Scan(&uid, &kind, &queue, &label, &state, &maxAttempts, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if state != StateFailed || !Replayable(kind) || !payload.Valid {
		return nil, ErrNotReplayable
	}
	req := EnqueueRequest{UserID: uid, Kind: kind, Queue: queue, Label: label, MaxAttempts: maxAttempts}
	if payload.String != "" {
		req.Payload = []byte(payload.String)
	}
	return m.Enqueue(ctx, req)
}

func (l *lane) len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, q := range l.items {
		n += len(q)
	}
	return n
}

// remove takes a queued job out of the lane.
func (l *lane) remove(id string) (item, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for p, q := range l.items {
		for i, it := range q {
			if it.id == id {
				l.items[p] = slices.Delete(slices.Clone(q), i, i+1)
				return it, true
			}
		}
	}
	return item{}, false
}
