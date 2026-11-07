// Package queue runs background jobs in process with one worker pool per kind group (sync and
// imports, LLM calls, outbound webhooks) so slow LLM calls never starve synchronisation. Job
// statuses are persisted in async_jobs and pushed over SSE; only public French messages are stored.
package queue

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"stride/internal/observability"
	"stride/internal/sse"
)

const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
)

// Kind groups, each served by its own worker pool.
const (
	GroupSync    = "sync"
	GroupLLM     = "llm"
	GroupWebhook = "webhook"
)

const (
	StateQueued    = "queued"
	StateRunning   = "running"
	StateRetrying  = "retrying"
	StateCompleted = "completed"
	StateFailed    = "failed"
)

const (
	KindSyncProvider      = "sync.provider"
	KindSyncStravaWebhook = "sync.strava_webhook"
	KindSyncResync        = "sync.resync"
	KindSyncHistory       = "sync.history"
	KindImportActivity    = "import.activity"
	KindLLMAnalyze        = "llm.analyze"
	KindLLMPlanGenerate   = "llm.plan_generate"
	KindLLMPlanAdapt      = "llm.plan_adapt"
	KindWebhookDeliver    = "webhook.deliver"
)

var (
	ErrNotFound = errors.New("job not found")
	// ErrPending is returned by EnqueueAndWait when the job outlives the wait; it keeps running.
	ErrPending = errors.New("job still running")
	ErrClosed  = errors.New("queue closed")
)

const (
	defaultBackoff = 10 * time.Second
	maxBackoff     = 15 * time.Minute
	backoffFactor  = 6

	msgGeneric     = "La tâche a échoué (détails dans les journaux du serveur)."
	msgInterrupted = "Tâche interrompue par un redémarrage du serveur."
	msgCancelled   = "Tâche annulée par un administrateur."
)

var defaultConcurrency = map[string]int{GroupSync: 2, GroupLLM: 2, GroupWebhook: 4}

// Failure is a job error whose Message is safe to persist and publish (French, never a secret).
// The cause only reaches in-process synchronous callers.
type Failure struct {
	Message   string
	Retryable bool
	cause     error
}

func (f *Failure) Error() string { return f.Message }
func (f *Failure) Unwrap() error { return f.cause }

// Permanent fails the job at once with a public message.
func Permanent(msg string, cause error) error { return &Failure{Message: msg, cause: cause} }

// Transient lets the job be retried with backoff, then fail with the public message.
func Transient(msg string, cause error) error {
	return &Failure{Message: msg, Retryable: true, cause: cause}
}

// publicMessage never exposes the text of an untyped error (it may embed URLs with tokens).
func publicMessage(err error) (msg string, retryable bool) {
	var f *Failure
	if errors.As(err, &f) {
		return f.Message, f.Retryable
	}
	return msgGeneric, false
}

func causeOf(err error) error {
	var f *Failure
	if errors.As(err, &f) && f.cause != nil {
		return f.cause
	}
	return err
}

// JobStatus matches the Stride 4.0.0 JobStatus JSON contract verbatim.
type JobStatus struct {
	ID          string         `json:"id"`
	UserID      int64          `json:"userId"`
	Kind        string         `json:"kind"`
	Queue       string         `json:"queue"`
	State       string         `json:"state"`
	Label       string         `json:"label"`
	ProgressPct *float64       `json:"progressPct"`
	Processed   *int           `json:"processed"`
	Total       *int           `json:"total"`
	Attempts    int            `json:"attempts"`
	MaxAttempts int            `json:"maxAttempts"`
	Error       *string        `json:"error"`
	Result      map[string]any `json:"result"`
	CreatedAt   string         `json:"createdAt"`
	UpdatedAt   string         `json:"updatedAt"`
}

type JobListResponse struct {
	Items []JobStatus `json:"items"`
}

// JobContext is passed to registered handlers.
type JobContext struct {
	ID          string
	UserID      int64
	Kind        string
	Queue       string
	Payload     []byte
	Attempt     int
	MaxAttempts int
	mgr         *Manager
	persisted   bool
}

// UpdateProgress persists granular progress and broadcasts job.updated over SSE.
func (jc *JobContext) UpdateProgress(label string, processed, total *int, pct *float64) {
	if jc == nil || jc.mgr == nil {
		return
	}
	var computed *float64
	if pct != nil {
		v := math.Max(0, math.Min(100, math.Round(*pct*10)/10))
		computed = &v
	} else if processed != nil && total != nil && *total > 0 {
		v := math.Max(0, math.Min(100, math.Round(float64(*processed)/float64(*total)*1000)/10))
		computed = &v
	}
	m := jc.mgr
	now := m.now().Unix()
	if jc.persisted {
		_, err := m.db.ExecContext(context.Background(), `UPDATE async_jobs
			SET label = CASE WHEN ? <> '' THEN ? ELSE label END, processed = ?, total = ?, progress_pct = ?, updated_at = ?
			WHERE id = ? AND user_id = ?`, label, label, processed, total, computed, now, jc.ID, jc.UserID)
		if err == nil {
			m.emitStored(jc.UserID, jc.ID)
			return
		}
	}
	ts := stamp(now)
	m.emit(&JobStatus{ID: jc.ID, UserID: jc.UserID, Kind: jc.Kind, Queue: jc.Queue, State: StateRunning, Label: label,
		ProgressPct: computed, Processed: processed, Total: total, Attempts: jc.Attempt, MaxAttempts: jc.MaxAttempts,
		CreatedAt: ts, UpdatedAt: ts})
}

// HandlerFunc processes a job and optionally returns a JSON-serializable result object.
type HandlerFunc func(ctx context.Context, jc *JobContext) (any, error)

type EnqueueRequest struct {
	ID          string
	UserID      int64
	Kind        string
	Queue       string
	Label       string
	Payload     any
	MaxAttempts int
	Delay       time.Duration
}

type item struct {
	id          string
	userID      int64
	kind        string
	queue       string
	label       string
	payload     []byte
	maxAttempts int
	attempt     int
	persisted   bool
	createdAt   string
}

type jobOutcome struct {
	status JobStatus
	result any
	err    error
}

type Options struct {
	DB      *sql.DB
	SSE     *sse.Hub
	Metrics *observability.Metrics
	Log     *slog.Logger
	// Concurrency overrides the worker count per group (GroupSync, GroupLLM, GroupWebhook).
	Concurrency map[string]int
	// BaseBackoff is the first retry delay (default 10 s, ×6 per attempt, capped at 15 min).
	BaseBackoff time.Duration
	Now         func() time.Time
}

// Manager persists job statuses, dispatches jobs to the worker pool of their group and drains on shutdown.
type Manager struct {
	db          *sql.DB
	sse         *sse.Hub
	metrics     *observability.Metrics
	log         *slog.Logger
	now         func() time.Time
	backoff     time.Duration
	concurrency map[string]int
	lanes       map[string]*lane

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.Mutex
	idle     *sync.Cond
	handlers map[string]HandlerFunc
	waiters  map[string]chan jobOutcome
	timers   map[*time.Timer]struct{}
	pending  int
	closed   bool

	// Admin control center: retry timers by job id, running job cancellations, busy workers per group.
	delayed   map[string]delayedJob
	runCancel map[string]context.CancelFunc
	cancelled map[string]bool
	active    map[string]int

	startOnce sync.Once
}

func NewManager(opts Options) *Manager {
	m := &Manager{
		db: opts.DB, sse: opts.SSE, metrics: opts.Metrics, log: opts.Log, now: opts.Now, backoff: opts.BaseBackoff,
		concurrency: map[string]int{},
		lanes:       map[string]*lane{},
		handlers:    map[string]HandlerFunc{},
		waiters:     map[string]chan jobOutcome{},
		timers:      map[*time.Timer]struct{}{},
		delayed:     map[string]delayedJob{},
		runCancel:   map[string]context.CancelFunc{},
		cancelled:   map[string]bool{},
		active:      map[string]int{},
	}
	if m.log == nil {
		m.log = slog.Default()
	}
	if m.now == nil {
		m.now = time.Now
	}
	if m.backoff <= 0 {
		m.backoff = defaultBackoff
	}
	for g, n := range defaultConcurrency {
		if c := opts.Concurrency[g]; c > 0 {
			n = c
		}
		m.concurrency[g] = n
		m.lanes[g] = newLane()
	}
	m.idle = sync.NewCond(&m.mu)
	m.ctx, m.cancel = context.WithCancel(context.Background())
	return m
}

func groupOf(kind string) string {
	switch {
	case strings.HasPrefix(kind, "llm."):
		return GroupLLM
	case strings.HasPrefix(kind, "webhook."):
		return GroupWebhook
	}
	return GroupSync
}

func (m *Manager) retryDelay(attempt int) time.Duration {
	d := m.backoff
	for i := 1; i < attempt; i++ {
		if d *= backoffFactor; d >= maxBackoff {
			return maxBackoff
		}
	}
	return min(d, maxBackoff)
}

// Register binds a job kind to its handler.
func (m *Manager) Register(kind string, fn HandlerFunc) {
	m.mu.Lock()
	m.handlers[kind] = fn
	m.mu.Unlock()
}

// Start fails the jobs a previous process left unfinished, then launches the workers. Idempotent.
func (m *Manager) Start() error {
	var err error
	m.startOnce.Do(func() {
		if m.db != nil {
			var res sql.Result
			res, err = m.db.ExecContext(context.Background(), `UPDATE async_jobs SET state = ?, error = ?, updated_at = ?
				WHERE state IN (?, ?, ?)`, StateFailed, msgInterrupted, m.now().Unix(), StateQueued, StateRunning, StateRetrying)
			if err != nil {
				return
			}
			if n, _ := res.RowsAffected(); n > 0 {
				m.log.Warn("⚙️ Tâches interrompues par le redémarrage marquées en échec", "tâches", n)
			}
		}
		for g, n := range m.concurrency {
			for range n {
				m.wg.Add(1)
				go m.worker(m.lanes[g])
			}
		}
	})
	return err
}

// Go runs fn in a goroutine counted by Wait, so chained background work drains as one unit.
func (m *Manager) Go(fn func()) {
	m.track(1)
	go func() {
		defer m.track(-1)
		fn()
	}()
}

// Wait blocks until no job is queued, running or scheduled for retry and no tracked goroutine runs.
func (m *Manager) Wait() {
	m.mu.Lock()
	for m.pending > 0 {
		m.idle.Wait()
	}
	m.mu.Unlock()
}

// Shutdown stops accepting jobs, drops queued and scheduled ones (failed at next start) and waits
// for running jobs until ctx expires, then cancels them.
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	for t := range m.timers {
		t.Stop()
	}
	m.timers = map[*time.Timer]struct{}{}
	m.delayed = map[string]delayedJob{}
	m.mu.Unlock()
	for _, l := range m.lanes {
		l.stop()
	}
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		m.cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}
	m.cancel()
	m.mu.Lock()
	m.pending = 0
	m.idle.Broadcast()
	m.mu.Unlock()
}

func (m *Manager) track(delta int) {
	m.mu.Lock()
	m.pending = max(0, m.pending+delta)
	if m.pending == 0 {
		m.idle.Broadcast()
	}
	m.mu.Unlock()
}

func newJobID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "job_" + hex.EncodeToString(b[:])
}

func stamp(unix int64) string { return time.Unix(unix, 0).UTC().Format(time.RFC3339) }

func normalizeQueue(q, kind string) string {
	switch q {
	case QueueCritical, QueueDefault, QueueLow:
		return q
	}
	switch kind {
	case KindImportActivity, KindLLMAnalyze, KindLLMPlanGenerate, KindLLMPlanAdapt:
		return QueueCritical
	case KindSyncResync, KindSyncHistory:
		return QueueLow
	}
	return QueueDefault
}

func defaultLabel(kind string) string {
	switch kind {
	case KindSyncProvider:
		return "Synchronisation des activités"
	case KindSyncStravaWebhook:
		return "Synchronisation webhook Strava"
	case KindSyncResync:
		return "Resynchronisation complète"
	case KindSyncHistory:
		return "Import de l'historique complet"
	case KindImportActivity:
		return "Import de fichier d'activité"
	case KindLLMAnalyze:
		return "Analyse coach IA"
	case KindLLMPlanGenerate:
		return "Génération du plan IA"
	case KindLLMPlanAdapt:
		return "Adaptation du plan IA"
	case KindWebhookDeliver:
		return "Envoi de notification externe"
	}
	return kind
}

func (m *Manager) canPersistUser(ctx context.Context, uid int64) bool {
	if m.db == nil || uid <= 0 {
		return false
	}
	var n int
	return m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = ?`, uid).Scan(&n) == nil && n > 0
}

func encodePayload(p any) ([]byte, error) {
	switch v := p.(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case json.RawMessage:
		return v, nil
	}
	return json.Marshal(p)
}

// Enqueue persists a queued JobStatus (when UserID is a real user) and dispatches the job.
func (m *Manager) Enqueue(ctx context.Context, req EnqueueRequest) (*JobStatus, error) {
	payload, err := encodePayload(req.Payload)
	if err != nil {
		return nil, err
	}
	it := item{id: req.ID, userID: req.UserID, kind: req.Kind, queue: normalizeQueue(req.Queue, req.Kind), label: req.Label,
		payload: payload, maxAttempts: max(1, req.MaxAttempts), attempt: 1}
	if it.id == "" {
		it.id = newJobID()
	}
	if it.label == "" {
		it.label = defaultLabel(req.Kind)
	}
	now := m.now().Unix()
	it.createdAt = stamp(now)
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return nil, ErrClosed
	}
	if it.persisted = m.canPersistUser(ctx, it.userID); it.persisted {
		if _, err := m.db.ExecContext(ctx, `INSERT INTO async_jobs (id, user_id, kind, queue, state, label, attempts, max_attempts, created_at, updated_at, payload_json)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?)`, it.id, it.userID, it.kind, it.queue, StateQueued, it.label, it.maxAttempts, now, now, replayPayload(it)); err != nil {
			return nil, err
		}
	}
	st := &JobStatus{ID: it.id, UserID: it.userID, Kind: it.kind, Queue: it.queue, State: StateQueued, Label: it.label,
		MaxAttempts: it.maxAttempts, CreatedAt: it.createdAt, UpdatedAt: it.createdAt}
	m.emit(st)
	m.track(1)
	m.metrics.AddQueueDepth(it.queue, 1)
	m.schedule(it, req.Delay)
	return st, nil
}

// EnqueueAndWait enqueues a job and waits for its outcome at most maxWait. Past that it returns the
// current status with ErrPending: the job keeps running and its end is pushed over SSE.
func (m *Manager) EnqueueAndWait(ctx context.Context, req EnqueueRequest, maxWait time.Duration) (*JobStatus, any, error) {
	if req.ID == "" {
		req.ID = newJobID()
	}
	ch := make(chan jobOutcome, 1)
	m.mu.Lock()
	m.waiters[req.ID] = ch
	m.mu.Unlock()
	forget := func() {
		m.mu.Lock()
		delete(m.waiters, req.ID)
		m.mu.Unlock()
	}
	st, err := m.Enqueue(ctx, req)
	if err != nil {
		forget()
		return nil, nil, err
	}
	timer := time.NewTimer(maxWait)
	defer timer.Stop()
	select {
	case out := <-ch:
		return &out.status, out.result, out.err
	case <-ctx.Done():
		forget()
		return nil, nil, ctx.Err()
	case <-timer.C:
		forget()
		if cur, err := m.GetJob(ctx, st.UserID, st.ID); err == nil {
			st = cur
		}
		return st, nil, ErrPending
	}
}

func (m *Manager) schedule(it item, delay time.Duration) {
	l := m.lanes[groupOf(it.kind)]
	if delay <= 0 {
		if !l.push(it) {
			m.drop(it)
		}
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		m.dropLocked(it)
		return
	}
	var t *time.Timer
	t = time.AfterFunc(delay, func() {
		m.mu.Lock()
		delete(m.timers, t)
		delete(m.delayed, it.id)
		m.mu.Unlock()
		if !l.push(it) {
			m.drop(it)
		}
	})
	m.timers[t] = struct{}{}
	m.delayed[it.id] = delayedJob{timer: t, it: it}
}

func (m *Manager) drop(it item) {
	m.mu.Lock()
	m.dropLocked(it)
	m.mu.Unlock()
}

func (m *Manager) dropLocked(it item) {
	m.metrics.AddQueueDepth(it.queue, -1)
	m.pending = max(0, m.pending-1)
	if m.pending == 0 {
		m.idle.Broadcast()
	}
}

func (m *Manager) worker(l *lane) {
	defer m.wg.Done()
	for {
		it, ok := l.pop()
		if !ok {
			return
		}
		m.run(it)
	}
}

func (m *Manager) run(it item) {
	m.metrics.AddQueueDepth(it.queue, -1)
	m.metrics.AddWorkerActive(it.queue, it.kind, 1)
	group := groupOf(it.kind)
	ctx, cancel := context.WithCancel(m.ctx)
	m.mu.Lock()
	m.runCancel[it.id] = cancel
	m.active[group]++
	m.mu.Unlock()
	t0 := time.Now()
	res, err := m.execute(ctx, it)
	cancel()
	m.mu.Lock()
	delete(m.runCancel, it.id)
	m.active[group]--
	aborted := m.cancelled[it.id]
	delete(m.cancelled, it.id)
	m.mu.Unlock()
	m.metrics.AddWorkerActive(it.queue, it.kind, -1)
	dur := time.Since(t0)
	if err != nil && aborted {
		m.finish(it, nil, Permanent(msgCancelled, err), dur)
		return
	}
	if err == nil {
		m.finish(it, res, nil, dur)
		return
	}
	msg, retry := publicMessage(err)
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if !retry || it.attempt >= it.maxAttempts || closed {
		m.finish(it, nil, err, dur)
		return
	}
	m.metrics.ObserveWorkerJob(it.queue, it.kind, "retried", dur)
	delay := m.retryDelay(it.attempt)
	m.log.Info("⚙️ Tâche en échec, nouvelle tentative programmée", "kind", it.kind, "user", it.userID,
		"tentative", it.attempt, "dans", delay.String(), "motif", msg)
	m.setState(it, StateRetrying, &msg, nil, nil)
	it.attempt++
	m.metrics.AddQueueDepth(it.queue, 1)
	m.schedule(it, delay)
}

func (m *Manager) execute(ctx context.Context, it item) (res any, err error) {
	m.mu.Lock()
	fn := m.handlers[it.kind]
	m.mu.Unlock()
	if fn == nil {
		return nil, Permanent(msgGeneric, fmt.Errorf("no handler registered for %s", it.kind))
	}
	m.setState(it, StateRunning, nil, nil, nil)
	defer func() {
		if r := recover(); r != nil {
			m.log.Error("💥 Tâche interrompue par une panique", "kind", it.kind, "panic", fmt.Sprint(r))
			res, err = nil, Permanent(msgGeneric, fmt.Errorf("panic: %v", r))
		}
	}()
	return fn(ctx, &JobContext{ID: it.id, UserID: it.userID, Kind: it.kind, Queue: it.queue, Payload: it.payload,
		Attempt: it.attempt, MaxAttempts: it.maxAttempts, mgr: m, persisted: it.persisted})
}

// setState persists and publishes a state change; errMsg is always a public message.
func (m *Manager) setState(it item, state string, errMsg *string, pct *float64, result map[string]any) JobStatus {
	now := m.now().Unix()
	if it.persisted {
		var resJSON *string
		if result != nil {
			b, _ := json.Marshal(result)
			s := string(b)
			resJSON = &s
		}
		_, err := m.db.ExecContext(context.Background(), `UPDATE async_jobs SET state = ?, attempts = ?, error = ?,
			progress_pct = COALESCE(?, progress_pct), result_json = COALESCE(?, result_json), updated_at = ? WHERE id = ?`,
			state, it.attempt, errMsg, pct, resJSON, now, it.id)
		if err == nil {
			if st, err := m.GetJob(context.Background(), it.userID, it.id); err == nil {
				m.emit(st)
				return *st
			}
		}
	}
	st := JobStatus{ID: it.id, UserID: it.userID, Kind: it.kind, Queue: it.queue, State: state, Label: it.label,
		ProgressPct: pct, Attempts: it.attempt, MaxAttempts: it.maxAttempts, Error: errMsg, Result: result,
		CreatedAt: it.createdAt, UpdatedAt: stamp(now)}
	m.emit(&st)
	return st
}

func resultMap(res any) map[string]any {
	if res == nil {
		return nil
	}
	b, err := json.Marshal(res)
	if err != nil {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(b, &out) != nil || out == nil {
		return map[string]any{"value": res}
	}
	return out
}

func (m *Manager) finish(it item, res any, err error, dur time.Duration) {
	var st JobStatus
	if err != nil {
		msg, _ := publicMessage(err)
		m.metrics.ObserveWorkerJob(it.queue, it.kind, "failed", dur)
		m.log.Warn("⚙️ Tâche en échec", "kind", it.kind, "user", it.userID, "tentative", it.attempt, "motif", msg)
		st = m.setState(it, StateFailed, &msg, nil, nil)
	} else {
		hundred := 100.0
		m.metrics.ObserveWorkerJob(it.queue, it.kind, "completed", dur)
		st = m.setState(it, StateCompleted, nil, &hundred, resultMap(res))
	}
	m.mu.Lock()
	ch := m.waiters[it.id]
	delete(m.waiters, it.id)
	m.mu.Unlock()
	if ch != nil {
		if err != nil {
			err = causeOf(err)
		}
		ch <- jobOutcome{status: st, result: res, err: err}
	}
	m.track(-1)
}

func (m *Manager) emit(st *JobStatus) {
	if m.sse == nil || st == nil || st.UserID <= 0 {
		return
	}
	m.sse.PublishToUser(st.UserID, sse.EventJobUpdated, st)
}

func (m *Manager) emitStored(uid int64, id string) {
	if st, err := m.GetJob(context.Background(), uid, id); err == nil {
		m.emit(st)
	}
}

func scanJob(sc interface{ Scan(...any) error }) (JobStatus, error) {
	var j JobStatus
	var pct sql.NullFloat64
	var proc, tot sql.NullInt64
	var errStr, resStr sql.NullString
	var created, updated int64
	if err := sc.Scan(&j.ID, &j.UserID, &j.Kind, &j.Queue, &j.State, &j.Label,
		&pct, &proc, &tot, &j.Attempts, &j.MaxAttempts, &errStr, &resStr, &created, &updated); err != nil {
		return j, err
	}
	if pct.Valid {
		j.ProgressPct = &pct.Float64
	}
	if proc.Valid {
		v := int(proc.Int64)
		j.Processed = &v
	}
	if tot.Valid {
		v := int(tot.Int64)
		j.Total = &v
	}
	if errStr.Valid {
		j.Error = &errStr.String
	}
	if resStr.Valid && resStr.String != "" {
		_ = json.Unmarshal([]byte(resStr.String), &j.Result)
	}
	j.CreatedAt, j.UpdatedAt = stamp(created), stamp(updated)
	return j, nil
}

const jobCols = `id, user_id, kind, queue, state, label, progress_pct, processed, total, attempts, max_attempts, error, result_json, created_at, updated_at`

// GetJob returns a single job owned by userID or ErrNotFound.
func (m *Manager) GetJob(ctx context.Context, userID int64, jobID string) (*JobStatus, error) {
	if m.db == nil {
		return nil, ErrNotFound
	}
	j, err := scanJob(m.db.QueryRowContext(ctx, `SELECT `+jobCols+` FROM async_jobs WHERE id = ? AND user_id = ?`, jobID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// ListJobs returns the latest jobs owned by userID (default 20, max 100).
func (m *Manager) ListJobs(ctx context.Context, userID int64, limit int) (JobListResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 100)
	out := JobListResponse{Items: []JobStatus{}}
	if m.db == nil {
		return out, nil
	}
	rows, err := m.db.QueryContext(ctx, `SELECT `+jobCols+` FROM async_jobs WHERE user_id = ? ORDER BY created_at DESC, rowid DESC LIMIT ?`, userID, limit)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, j)
	}
	return out, rows.Err()
}

// Purge deletes terminal jobs last updated before cutoff.
func (m *Manager) Purge(ctx context.Context, cutoff time.Time) (int64, error) {
	if m.db == nil {
		return 0, nil
	}
	res, err := m.db.ExecContext(ctx, `DELETE FROM async_jobs WHERE state IN (?, ?) AND updated_at < ?`,
		StateCompleted, StateFailed, cutoff.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// lane is the FIFO of one group, served by priority (critical, default, low).
type lane struct {
	mu      sync.Mutex
	cond    *sync.Cond
	items   [3][]item
	stopped bool
}

func newLane() *lane {
	l := &lane{}
	l.cond = sync.NewCond(&l.mu)
	return l
}

func priority(q string) int {
	switch q {
	case QueueCritical:
		return 0
	case QueueLow:
		return 2
	}
	return 1
}

func (l *lane) push(it item) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopped {
		return false
	}
	p := priority(it.queue)
	l.items[p] = append(l.items[p], it)
	l.cond.Signal()
	return true
}

func (l *lane) pop() (item, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for !l.stopped {
		for p := range l.items {
			if len(l.items[p]) > 0 {
				it := l.items[p][0]
				l.items[p] = l.items[p][1:]
				return it, true
			}
		}
		l.cond.Wait()
	}
	return item{}, false
}

func (l *lane) stop() {
	l.mu.Lock()
	l.stopped = true
	l.items = [3][]item{}
	l.cond.Broadcast()
	l.mu.Unlock()
}
