// Package queue implements the unified async worker manager backed by Valkey (hibiken/asynq)
// in production and a priority-weighted in-memory worker pool in local/test environments.
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
	"sync"
	"time"

	"github.com/hibiken/asynq"

	"stride/internal/observability"
	"stride/internal/sse"
)

const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
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
	KindImportActivity    = "import.activity"
	KindLLMAnalyze        = "llm.analyze"
	KindLLMPlanGenerate   = "llm.plan_generate"
	KindLLMPlanAdapt      = "llm.plan_adapt"
	KindWebhookDeliver    = "webhook.deliver"
)

var ErrNotFound = errors.New("job not found")

type nonRetryableError struct{ err error }

func (e nonRetryableError) Error() string { return e.err.Error() }
func (e nonRetryableError) Unwrap() error { return e.err }

// NonRetryable marks an error as permanent so the worker fails immediately without retrying.
func NonRetryable(err error) error {
	if err == nil {
		return nil
	}
	return nonRetryableError{err: err}
}

func isNonRetryable(err error) bool {
	var target nonRetryableError
	return errors.As(err, &target)
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

// JobContext is passed to registered worker handlers.
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
	var computedPct *float64
	if pct != nil {
		v := math.Max(0, math.Min(100, math.Round(*pct*10)/10))
		computedPct = &v
	} else if processed != nil && total != nil && *total > 0 {
		v := math.Max(0, math.Min(100, math.Round(float64(*processed)/float64(*total)*1000)/10))
		computedPct = &v
	}
	now := jc.mgr.now().Unix()
	if jc.persisted && jc.mgr.db != nil {
		_, _ = jc.mgr.db.ExecContext(context.Background(), `UPDATE async_jobs
			SET label = CASE WHEN ? <> '' THEN ? ELSE label END,
			    processed = ?, total = ?, progress_pct = ?, updated_at = ?
			WHERE id = ? AND user_id = ?`,
			label, label, processed, total, computedPct, now, jc.ID, jc.UserID)
		if st, err := jc.mgr.GetJob(context.Background(), jc.UserID, jc.ID); err == nil {
			jc.mgr.emitJob(st)
			return
		}
	}
	if jc.UserID > 0 {
		ts := time.Unix(now, 0).UTC().Format(time.RFC3339)
		jc.mgr.emitJob(&JobStatus{
			ID: jc.ID, UserID: jc.UserID, Kind: jc.Kind, Queue: jc.Queue, State: StateRunning,
			Label: label, ProgressPct: computedPct, Processed: processed, Total: total,
			Attempts: jc.Attempt, MaxAttempts: jc.MaxAttempts, CreatedAt: ts, UpdatedAt: ts,
		})
	}
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

type taskEnvelope struct {
	ID          string          `json:"id"`
	UserID      int64           `json:"userId"`
	Kind        string          `json:"kind"`
	Queue       string          `json:"queue"`
	Label       string          `json:"label"`
	Payload     json.RawMessage `json:"payload"`
	MaxAttempts int             `json:"maxAttempts"`
	Persisted   bool            `json:"persisted"`
}

type jobOutcome struct {
	status JobStatus
	result any
	err    error
}

type Options struct {
	DB          *sql.DB
	ValkeyURL   string
	SSE         *sse.Hub
	Metrics     *observability.Metrics
	Log         *slog.Logger
	Concurrency int
	BaseBackoff time.Duration
	Now         func() time.Time
}

type quietAsynqLogger struct{ log *slog.Logger }

func (l quietAsynqLogger) Debug(args ...any) {}
func (l quietAsynqLogger) Info(args ...any)  {}
func (l quietAsynqLogger) Warn(args ...any) {
	if l.log != nil {
		l.log.Warn("⚙️ Asynq", "msg", fmt.Sprint(args...))
	}
}
func (l quietAsynqLogger) Error(args ...any) {
	if l.log != nil {
		l.log.Error("❌ Asynq", "msg", fmt.Sprint(args...))
	}
}
func (l quietAsynqLogger) Fatal(args ...any) {
	if l.log != nil {
		l.log.Error("💥 Asynq fatal", "msg", fmt.Sprint(args...))
	}
}

// Manager coordinates job persistence, SSE notifications, metrics, and either Asynq or MemoryEngine.
type Manager struct {
	db          *sql.DB
	sse         *sse.Hub
	metrics     *observability.Metrics
	log         *slog.Logger
	now         func() time.Time
	baseBackoff time.Duration
	concurrency int

	mu       sync.RWMutex
	handlers map[string]HandlerFunc
	waiters  map[string]chan jobOutcome

	// Tracked in-flight/scheduled tasks for Wait()
	trackMu   sync.Mutex
	trackCond *sync.Cond
	pending   int

	// Asynq backend (when ValkeyURL != "")
	asynqClient *asynq.Client
	asynqServer *asynq.Server
	asynqMux    *asynq.ServeMux

	// MemoryEngine backend (when ValkeyURL == "")
	mem *memoryEngine

	startOnce sync.Once
	closeOnce sync.Once
}

func NewManager(opts Options) (*Manager, error) {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	conc := opts.Concurrency
	if conc <= 0 {
		conc = 6
	}
	backoff := opts.BaseBackoff
	if backoff <= 0 {
		backoff = 500 * time.Millisecond
	}

	m := &Manager{
		db:          opts.DB,
		sse:         opts.SSE,
		metrics:     opts.Metrics,
		log:         log,
		now:         nowFn,
		baseBackoff: backoff,
		concurrency: conc,
		handlers:    make(map[string]HandlerFunc),
		waiters:     make(map[string]chan jobOutcome),
	}
	m.trackCond = sync.NewCond(&m.trackMu)

	if opts.ValkeyURL != "" {
		redisOpt, err := asynq.ParseRedisURI(opts.ValkeyURL)
		if err != nil {
			return nil, fmt.Errorf("queue valkey url: %w", err)
		}
		m.asynqClient = asynq.NewClient(redisOpt)
		m.asynqMux = asynq.NewServeMux()
		m.asynqServer = asynq.NewServer(redisOpt, asynq.Config{
			Concurrency: conc,
			Queues: map[string]int{
				QueueCritical: 6,
				QueueDefault:  3,
				QueueLow:      1,
			},
			RetryDelayFunc: func(n int, _ error, _ *asynq.Task) time.Duration {
				return m.retryDelay(n + 1)
			},
			DelayedTaskCheckInterval: max(50*time.Millisecond, min(backoff, time.Second)),
			Logger:                   quietAsynqLogger{log: log},
		})
	} else {
		m.mem = newMemoryEngine(m, conc)
	}
	return m, nil
}

func (m *Manager) retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := min(attempt-1, 6)
	return m.baseBackoff * time.Duration(1<<shift)
}

// Register binds a job kind to its handler. Must be called before Start (or dynamically for MemoryEngine).
func (m *Manager) Register(kind string, fn HandlerFunc) {
	m.mu.Lock()
	m.handlers[kind] = fn
	m.mu.Unlock()
	if m.asynqMux != nil {
		m.asynqMux.HandleFunc(kind, func(ctx context.Context, t *asynq.Task) error {
			var env taskEnvelope
			if err := json.Unmarshal(t.Payload(), &env); err != nil {
				return asynq.SkipRetry
			}
			retried, _ := asynq.GetRetryCount(ctx)
			maxRetry, ok := asynq.GetMaxRetry(ctx)
			maxAttempts := env.MaxAttempts
			if ok && maxRetry+1 > maxAttempts {
				maxAttempts = maxRetry + 1
			}
			err := m.executeTask(ctx, env, retried+1, maxAttempts)
			if err != nil && isNonRetryable(err) {
				return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
			}
			return err
		})
	}
}

// Start launches background workers. Safe to call multiple times.
func (m *Manager) Start() error {
	var startErr error
	m.startOnce.Do(func() {
		if m.asynqServer != nil {
			startErr = m.asynqServer.Start(m.asynqMux)
		} else if m.mem != nil {
			m.mem.start()
		}
	})
	return startErr
}

// Wait blocks until all currently queued and active jobs reach a terminal state.
func (m *Manager) Wait() {
	if m == nil {
		return
	}
	m.trackMu.Lock()
	for m.pending > 0 {
		m.trackCond.Wait()
	}
	m.trackMu.Unlock()
}

// Close gracefully stops workers and releases connections.
func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.closeOnce.Do(func() {
		if m.mem != nil {
			m.mem.stop()
		}
		if m.asynqServer != nil {
			m.asynqServer.Shutdown()
		}
		if m.asynqClient != nil {
			_ = m.asynqClient.Close()
		}
		m.trackMu.Lock()
		m.pending = 0
		m.trackCond.Broadcast()
		m.trackMu.Unlock()
	})
}

func (m *Manager) addPending(delta int) {
	m.trackMu.Lock()
	m.pending += delta
	if m.pending <= 0 {
		m.pending = 0
		m.trackCond.Broadcast()
	}
	m.trackMu.Unlock()
}

func newJobID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "job_" + hex.EncodeToString(b[:])
}

func normalizeQueue(q, kind string) string {
	switch q {
	case QueueCritical, QueueDefault, QueueLow:
		return q
	}
	switch kind {
	case KindImportActivity, KindLLMAnalyze, KindLLMPlanGenerate, KindLLMPlanAdapt:
		return QueueCritical
	case KindSyncResync:
		return QueueLow
	default:
		return QueueDefault
	}
}

func defaultLabel(kind string) string {
	switch kind {
	case KindSyncProvider:
		return "Synchronisation des activités"
	case KindSyncStravaWebhook:
		return "Synchronisation webhook Strava"
	case KindSyncResync:
		return "Resynchronisation complète"
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
	default:
		return kind
	}
}

func (m *Manager) canPersistUser(ctx context.Context, uid int64) bool {
	if m.db == nil || uid <= 0 {
		return false
	}
	var exists int
	if err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = ?`, uid).Scan(&exists); err != nil {
		return false
	}
	return exists > 0
}

// Enqueue persists a queued JobStatus (when UserID belongs to a valid user) and dispatches it.
func (m *Manager) Enqueue(ctx context.Context, req EnqueueRequest) (*JobStatus, error) {
	_ = m.Start()
	id := req.ID
	if id == "" {
		id = newJobID()
	}
	q := normalizeQueue(req.Queue, req.Kind)
	label := req.Label
	if label == "" {
		label = defaultLabel(req.Kind)
	}
	maxAttempts := req.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 3
	}
	var rawPayload []byte
	if req.Payload != nil {
		if b, ok := req.Payload.([]byte); ok {
			rawPayload = b
		} else if raw, ok := req.Payload.(json.RawMessage); ok {
			rawPayload = raw
		} else {
			b, err := json.Marshal(req.Payload)
			if err != nil {
				return nil, err
			}
			rawPayload = b
		}
	}

	now := m.now().Unix()
	ts := time.Unix(now, 0).UTC().Format(time.RFC3339)
	persist := m.canPersistUser(ctx, req.UserID)

	st := &JobStatus{
		ID:          id,
		UserID:      req.UserID,
		Kind:        req.Kind,
		Queue:       q,
		State:       StateQueued,
		Label:       label,
		Attempts:    0,
		MaxAttempts: maxAttempts,
		CreatedAt:   ts,
		UpdatedAt:   ts,
	}

	if persist {
		_, err := m.db.ExecContext(ctx, `INSERT INTO async_jobs
			(id, user_id, kind, queue, state, label, attempts, max_attempts, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
			st.ID, st.UserID, st.Kind, st.Queue, st.State, st.Label, st.MaxAttempts, now, now)
		if err != nil {
			return nil, err
		}
	}
	m.emitJob(st)

	env := taskEnvelope{
		ID:          id,
		UserID:      req.UserID,
		Kind:        req.Kind,
		Queue:       q,
		Label:       label,
		Payload:     rawPayload,
		MaxAttempts: maxAttempts,
		Persisted:   persist,
	}

	m.addPending(1)
	m.metrics.AddQueueDepth(q, 1)

	if m.asynqClient != nil {
		body, _ := json.Marshal(env)
		opts := []asynq.Option{
			asynq.Queue(q),
			asynq.MaxRetry(maxAttempts - 1),
			asynq.TaskID(id),
		}
		if req.Delay > 0 {
			opts = append(opts, asynq.ProcessIn(req.Delay))
		}
		if _, err := m.asynqClient.EnqueueContext(ctx, asynq.NewTask(req.Kind, body), opts...); err != nil {
			m.metrics.AddQueueDepth(q, -1)
			m.addPending(-1)
			return nil, err
		}
		return st, nil
	}

	m.mem.submit(env, 1, req.Delay)
	return st, nil
}

// EnqueueAndWait enqueues a job onto the worker pool and blocks until completion or ctx cancellation.
func (m *Manager) EnqueueAndWait(ctx context.Context, req EnqueueRequest) (*JobStatus, any, error) {
	if req.ID == "" {
		req.ID = newJobID()
	}
	waitCh := make(chan jobOutcome, 1)
	m.mu.Lock()
	m.waiters[req.ID] = waitCh
	m.mu.Unlock()

	_, err := m.Enqueue(ctx, req)
	if err != nil {
		m.mu.Lock()
		delete(m.waiters, req.ID)
		m.mu.Unlock()
		return nil, nil, err
	}

	select {
	case out := <-waitCh:
		return &out.status, out.result, out.err
	case <-ctx.Done():
		m.mu.Lock()
		delete(m.waiters, req.ID)
		m.mu.Unlock()
		return nil, nil, ctx.Err()
	}
}

func (m *Manager) executeTask(ctx context.Context, env taskEnvelope, attempt, maxAttempts int) error {
	m.metrics.AddQueueDepth(env.Queue, -1)
	m.metrics.AddWorkerActive(env.Queue, env.Kind, 1)
	t0 := time.Now()
	defer m.metrics.AddWorkerActive(env.Queue, env.Kind, -1)

	m.mu.RLock()
	fn := m.handlers[env.Kind]
	m.mu.RUnlock()
	if fn == nil {
		err := NonRetryable(fmt.Errorf("no handler registered for %s", env.Kind))
		m.finishTerminal(env, attempt, maxAttempts, nil, err, time.Since(t0))
		return err
	}

	now := m.now().Unix()
	if env.Persisted && m.db != nil {
		_, _ = m.db.ExecContext(context.Background(), `UPDATE async_jobs SET state = ?, attempts = ?, error = NULL, updated_at = ? WHERE id = ?`,
			StateRunning, attempt, now, env.ID)
		if st, err := m.GetJob(context.Background(), env.UserID, env.ID); err == nil {
			m.emitJob(st)
		}
	} else if env.UserID > 0 {
		ts := time.Unix(now, 0).UTC().Format(time.RFC3339)
		m.emitJob(&JobStatus{
			ID: env.ID, UserID: env.UserID, Kind: env.Kind, Queue: env.Queue, State: StateRunning,
			Label: env.Label, Attempts: attempt, MaxAttempts: maxAttempts, CreatedAt: ts, UpdatedAt: ts,
		})
	}

	jc := &JobContext{
		ID:          env.ID,
		UserID:      env.UserID,
		Kind:        env.Kind,
		Queue:       env.Queue,
		Payload:     env.Payload,
		Attempt:     attempt,
		MaxAttempts: maxAttempts,
		mgr:         m,
		persisted:   env.Persisted,
	}

	res, err := fn(ctx, jc)
	dur := time.Since(t0)
	if err == nil {
		m.finishTerminal(env, attempt, maxAttempts, res, nil, dur)
		return nil
	}

	willRetry := !isNonRetryable(err) && attempt < maxAttempts
	if willRetry {
		m.metrics.ObserveWorkerJob(env.Queue, env.Kind, "retried", dur)
		errMsg := err.Error()
		nowRetry := m.now().Unix()
		if env.Persisted && m.db != nil {
			_, _ = m.db.ExecContext(context.Background(), `UPDATE async_jobs SET state = ?, attempts = ?, error = ?, updated_at = ? WHERE id = ?`,
				StateRetrying, attempt, errMsg, nowRetry, env.ID)
			if st, gerr := m.GetJob(context.Background(), env.UserID, env.ID); gerr == nil {
				m.emitJob(st)
			}
		} else if env.UserID > 0 {
			ts := time.Unix(nowRetry, 0).UTC().Format(time.RFC3339)
			m.emitJob(&JobStatus{
				ID: env.ID, UserID: env.UserID, Kind: env.Kind, Queue: env.Queue, State: StateRetrying,
				Label: env.Label, Attempts: attempt, MaxAttempts: maxAttempts, Error: &errMsg, CreatedAt: ts, UpdatedAt: ts,
			})
		}
		// Re-increment queue depth for the upcoming retry attempt.
		m.metrics.AddQueueDepth(env.Queue, 1)
		return err
	}

	m.finishTerminal(env, attempt, maxAttempts, nil, err, dur)
	return err
}

func (m *Manager) finishTerminal(env taskEnvelope, attempt, maxAttempts int, res any, err error, dur time.Duration) {
	now := m.now().Unix()
	ts := time.Unix(now, 0).UTC().Format(time.RFC3339)

	state := StateCompleted
	metricStatus := "completed"
	var errPtr *string
	var pct *float64
	if err != nil {
		state = StateFailed
		metricStatus = "failed"
		msg := err.Error()
		errPtr = &msg
	} else {
		hundred := 100.0
		pct = &hundred
	}
	m.metrics.ObserveWorkerJob(env.Queue, env.Kind, metricStatus, dur)

	var resMap map[string]any
	var resJSON *string
	if err == nil && res != nil {
		if b, jerr := json.Marshal(res); jerr == nil {
			_ = json.Unmarshal(b, &resMap)
			if resMap == nil {
				resMap = map[string]any{"value": res}
				b, _ = json.Marshal(resMap)
			}
			s := string(b)
			resJSON = &s
		}
	}

	var finalStatus JobStatus
	if env.Persisted && m.db != nil {
		_, _ = m.db.ExecContext(context.Background(), `UPDATE async_jobs
			SET state = ?, attempts = ?, progress_pct = COALESCE(?, progress_pct), error = ?, result_json = ?, updated_at = ?
			WHERE id = ?`,
			state, attempt, pct, errPtr, resJSON, now, env.ID)
		if st, gerr := m.GetJob(context.Background(), env.UserID, env.ID); gerr == nil {
			finalStatus = *st
		}
	}
	if finalStatus.ID == "" {
		finalStatus = JobStatus{
			ID: env.ID, UserID: env.UserID, Kind: env.Kind, Queue: env.Queue, State: state,
			Label: env.Label, ProgressPct: pct, Attempts: attempt, MaxAttempts: maxAttempts,
			Error: errPtr, Result: resMap, CreatedAt: ts, UpdatedAt: ts,
		}
	}
	m.emitJob(&finalStatus)

	m.mu.Lock()
	ch, hasWaiter := m.waiters[env.ID]
	if hasWaiter {
		delete(m.waiters, env.ID)
	}
	m.mu.Unlock()
	if hasWaiter {
		var unwrapped error
		if err != nil {
			var nr nonRetryableError
			if errors.As(err, &nr) {
				unwrapped = nr.err
			} else {
				unwrapped = err
			}
		}
		ch <- jobOutcome{status: finalStatus, result: res, err: unwrapped}
		close(ch)
	}

	m.addPending(-1)
}

func (m *Manager) emitJob(st *JobStatus) {
	if m.sse == nil || st == nil || st.UserID <= 0 {
		return
	}
	m.sse.PublishToUser(st.UserID, sse.EventJobUpdated, st)
}

func scanJob(sc interface{ Scan(...any) error }) (JobStatus, error) {
	var j JobStatus
	var pct sql.NullFloat64
	var proc, tot sql.NullInt64
	var errStr, resStr sql.NullString
	var created, updated int64
	err := sc.Scan(&j.ID, &j.UserID, &j.Kind, &j.Queue, &j.State, &j.Label,
		&pct, &proc, &tot, &j.Attempts, &j.MaxAttempts, &errStr, &resStr, &created, &updated)
	if err != nil {
		return j, err
	}
	if pct.Valid {
		v := pct.Float64
		j.ProgressPct = &v
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
		s := errStr.String
		j.Error = &s
	}
	if resStr.Valid && resStr.String != "" {
		var m map[string]any
		if json.Unmarshal([]byte(resStr.String), &m) == nil {
			j.Result = m
		}
	}
	j.CreatedAt = time.Unix(created, 0).UTC().Format(time.RFC3339)
	j.UpdatedAt = time.Unix(updated, 0).UTC().Format(time.RFC3339)
	return j, nil
}

const jobCols = `id, user_id, kind, queue, state, label, progress_pct, processed, total, attempts, max_attempts, error, result_json, created_at, updated_at`

// GetJob returns a single job owned by userID or ErrNotFound.
func (m *Manager) GetJob(ctx context.Context, userID int64, jobID string) (*JobStatus, error) {
	if m == nil || m.db == nil {
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
	if limit > 100 {
		limit = 100
	}
	out := JobListResponse{Items: []JobStatus{}}
	if m == nil || m.db == nil {
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

// ---------- MemoryEngine (weighted priority worker pool + exponential backoff) ----------

type memItem struct {
	env     taskEnvelope
	attempt int
}

type memoryEngine struct {
	mgr     *Manager
	workers int

	mu       sync.Mutex
	cond     *sync.Cond
	critical []memItem
	defQueue []memItem
	low      []memItem
	timers   map[*time.Timer]struct{}
	cursor   int
	stopped  bool
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// weightedSchedule implements 6:3:1 priority polling while guaranteeing starvation-free progress.
var weightedSchedule = []string{
	QueueCritical, QueueCritical, QueueDefault,
	QueueCritical, QueueCritical, QueueDefault,
	QueueCritical, QueueCritical, QueueDefault, QueueLow,
}

func newMemoryEngine(mgr *Manager, workers int) *memoryEngine {
	ctx, cancel := context.WithCancel(context.Background())
	e := &memoryEngine{
		mgr:     mgr,
		workers: workers,
		timers:  make(map[*time.Timer]struct{}),
		ctx:     ctx,
		cancel:  cancel,
	}
	e.cond = sync.NewCond(&e.mu)
	return e
}

func (e *memoryEngine) start() {
	for i := 0; i < e.workers; i++ {
		e.wg.Add(1)
		go e.workerLoop()
	}
}

func (e *memoryEngine) stop() {
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		return
	}
	e.stopped = true
	for tm := range e.timers {
		tm.Stop()
		delete(e.timers, tm)
	}
	e.cancel()
	e.cond.Broadcast()
	e.mu.Unlock()
	e.wg.Wait()
}

func (e *memoryEngine) submit(env taskEnvelope, attempt int, delay time.Duration) {
	if delay > 0 {
		e.mu.Lock()
		if e.stopped {
			e.mu.Unlock()
			e.mgr.metrics.AddQueueDepth(env.Queue, -1)
			e.mgr.addPending(-1)
			return
		}
		var tm *time.Timer
		tm = time.AfterFunc(delay, func() {
			e.mu.Lock()
			delete(e.timers, tm)
			if e.stopped {
				e.mu.Unlock()
				e.mgr.metrics.AddQueueDepth(env.Queue, -1)
				e.mgr.addPending(-1)
				return
			}
			e.pushLocked(memItem{env: env, attempt: attempt})
			e.cond.Signal()
			e.mu.Unlock()
		})
		e.timers[tm] = struct{}{}
		e.mu.Unlock()
		return
	}
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		e.mgr.metrics.AddQueueDepth(env.Queue, -1)
		e.mgr.addPending(-1)
		return
	}
	e.pushLocked(memItem{env: env, attempt: attempt})
	e.cond.Signal()
	e.mu.Unlock()
}

func (e *memoryEngine) pushLocked(it memItem) {
	switch it.env.Queue {
	case QueueCritical:
		e.critical = append(e.critical, it)
	case QueueLow:
		e.low = append(e.low, it)
	default:
		e.defQueue = append(e.defQueue, it)
	}
}

func (e *memoryEngine) popLocked() (memItem, bool) {
	if len(e.critical) == 0 && len(e.defQueue) == 0 && len(e.low) == 0 {
		return memItem{}, false
	}
	for range len(weightedSchedule) {
		slot := weightedSchedule[e.cursor%len(weightedSchedule)]
		e.cursor++
		switch slot {
		case QueueCritical:
			if len(e.critical) > 0 {
				it := e.critical[0]
				e.critical = e.critical[1:]
				return it, true
			}
		case QueueDefault:
			if len(e.defQueue) > 0 {
				it := e.defQueue[0]
				e.defQueue = e.defQueue[1:]
				return it, true
			}
		case QueueLow:
			if len(e.low) > 0 {
				it := e.low[0]
				e.low = e.low[1:]
				return it, true
			}
		}
	}
	// Fallback strictly by priority: critical -> default -> low
	if len(e.critical) > 0 {
		it := e.critical[0]
		e.critical = e.critical[1:]
		return it, true
	}
	if len(e.defQueue) > 0 {
		it := e.defQueue[0]
		e.defQueue = e.defQueue[1:]
		return it, true
	}
	it := e.low[0]
	e.low = e.low[1:]
	return it, true
}

func (e *memoryEngine) workerLoop() {
	defer e.wg.Done()
	for {
		e.mu.Lock()
		for !e.stopped && len(e.critical) == 0 && len(e.defQueue) == 0 && len(e.low) == 0 {
			e.cond.Wait()
		}
		if e.stopped {
			e.mu.Unlock()
			return
		}
		it, ok := e.popLocked()
		e.mu.Unlock()
		if !ok {
			continue
		}

		err := e.mgr.executeTask(e.ctx, it.env, it.attempt, it.env.MaxAttempts)
		if err != nil && !isNonRetryable(err) && it.attempt < it.env.MaxAttempts {
			delay := e.mgr.retryDelay(it.attempt)
			e.submit(it.env, it.attempt+1, delay)
		}
	}
}
