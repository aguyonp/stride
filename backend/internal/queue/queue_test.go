package queue_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"stride/internal/auth"
	"stride/internal/observability"
	"stride/internal/queue"
	"stride/internal/sse"
	"stride/internal/store"
)

const secretURL = "https://discord.com/api/webhooks/123/s3cr3t-t0ken"

func openTestDBWithUsers(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "stride.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Now().Unix()
	for _, u := range []struct {
		id   int64
		name string
	}{{1, "alice"}, {2, "bob"}} {
		if _, err := st.DB.Exec(`INSERT INTO users (id, display_name, role, status, created_at) VALUES (?, ?, 'user', 'active', ?)`,
			u.id, u.name, now); err != nil {
			t.Fatalf("seed user %d: %v", u.id, err)
		}
	}
	return st
}

func newManager(t *testing.T, opts queue.Options) *queue.Manager {
	t.Helper()
	if opts.BaseBackoff == 0 {
		opts.BaseBackoff = 5 * time.Millisecond
	}
	m := queue.NewManager(opts)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})
	return m
}

// listenSSE collects the raw job.updated frames published to user 1.
func listenSSE(t *testing.T, hub *sse.Hub) func() []string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.User{ID: 1, Role: auth.RoleUser})))
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/events/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect sse: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	var mu sync.Mutex
	var frames []string
	ready := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(resp.Body)
		var ev string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				ev = strings.TrimPrefix(line, "event: ")
				if ev == sse.EventConnected {
					close(ready)
				}
			case strings.HasPrefix(line, "data: ") && ev == sse.EventJobUpdated:
				mu.Lock()
				frames = append(frames, strings.TrimPrefix(line, "data: "))
				mu.Unlock()
			}
		}
	}()
	<-ready
	return func() []string {
		time.Sleep(30 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), frames...)
	}
}

func TestEngineLifecycleIsolationAndSSE(t *testing.T) {
	st := openTestDBWithUsers(t)
	metrics := observability.New()
	hub := sse.NewHub(sse.Options{Metrics: metrics, HeartbeatInterval: time.Minute})
	defer hub.Close()
	mgr := newManager(t, queue.Options{DB: st.DB, SSE: hub, Metrics: metrics})
	frames := listenSSE(t, hub)

	mgr.Register(queue.KindImportActivity, func(_ context.Context, jc *queue.JobContext) (any, error) {
		proc, tot := 2, 4
		jc.UpdateProgress("Import FIT : 2/4 blocs", &proc, &tot, nil)
		return map[string]any{"activityId": 42, "created": true}, nil
	})
	var webhookAttempts atomic.Int32
	mgr.Register(queue.KindWebhookDeliver, func(_ context.Context, _ *queue.JobContext) (any, error) {
		if webhookAttempts.Add(1) < 3 {
			return nil, queue.Transient("Échec de l'envoi : HTTP 502.", errors.New("502 upstream gateway"))
		}
		return map[string]any{"delivered": true}, nil
	})
	var permanentAttempts atomic.Int32
	mgr.Register(queue.KindLLMAnalyze, func(_ context.Context, _ *queue.JobContext) (any, error) {
		permanentAttempts.Add(1)
		return nil, queue.Permanent("Format invalide.", errors.New("bad format"))
	})
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	stImport, res, err := mgr.EnqueueAndWait(ctx, queue.EnqueueRequest{UserID: 1, Kind: queue.KindImportActivity}, 5*time.Second)
	if err != nil {
		t.Fatalf("EnqueueAndWait import: %v", err)
	}
	if stImport.State != queue.StateCompleted || stImport.Queue != queue.QueueCritical || res == nil || stImport.Result["created"] != true {
		t.Fatalf("unexpected import status: %+v", stImport)
	}
	if _, err := mgr.GetJob(ctx, 2, stImport.ID); !errors.Is(err, queue.ErrNotFound) {
		t.Fatalf("user 2 must not see user 1's job, got %v", err)
	}
	if l, err := mgr.ListJobs(ctx, 2, 20); err != nil || len(l.Items) != 0 {
		t.Fatalf("user 2 job list: %+v (%v)", l, err)
	}

	retryJob, err := mgr.Enqueue(ctx, queue.EnqueueRequest{UserID: 1, Kind: queue.KindWebhookDeliver, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	failJob, _, failErr := mgr.EnqueueAndWait(ctx, queue.EnqueueRequest{UserID: 2, Kind: queue.KindLLMAnalyze, MaxAttempts: 4}, 5*time.Second)
	if failErr == nil || failErr.Error() != "bad format" {
		t.Fatalf("synchronous callers get the cause, got %v", failErr)
	}
	if failJob.State != queue.StateFailed || permanentAttempts.Load() != 1 || failJob.Error == nil || *failJob.Error != "Format invalide." {
		t.Fatalf("permanent failure must not be retried: %+v attempts=%d", failJob, permanentAttempts.Load())
	}
	mgr.Wait()
	got, err := mgr.GetJob(ctx, 1, retryJob.ID)
	if err != nil || got.State != queue.StateCompleted || got.Attempts != 3 || got.Error != nil {
		t.Fatalf("transient failures retried until success: %+v (%v)", got, err)
	}

	sawProgress := false
	for _, f := range frames() {
		var js queue.JobStatus
		if err := json.Unmarshal([]byte(f), &js); err != nil || js.UserID != 1 {
			t.Fatalf("SSE leaked a foreign or invalid frame: %s", f)
		}
		if js.ID == stImport.ID && js.ProgressPct != nil && *js.ProgressPct == 50 {
			sawProgress = true
		}
	}
	if !sawProgress {
		t.Fatal("expected a 50% progress frame for the import job")
	}
}

func TestFailuresNeverPersistOrPublishRawErrors(t *testing.T) {
	st := openTestDBWithUsers(t)
	hub := sse.NewHub(sse.Options{HeartbeatInterval: time.Minute})
	defer hub.Close()
	mgr := newManager(t, queue.Options{DB: st.DB, SSE: hub})
	frames := listenSSE(t, hub)
	urlErr := fmt.Errorf("Post %q: dial tcp: connection refused", secretURL)
	mgr.Register("raw", func(context.Context, *queue.JobContext) (any, error) { return nil, urlErr })
	mgr.Register("typed", func(context.Context, *queue.JobContext) (any, error) {
		return nil, queue.Transient("Échec de l'envoi : service injoignable.", urlErr)
	})
	mgr.Register("boom", func(context.Context, *queue.JobContext) (any, error) { panic(secretURL) })
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, kind := range []string{"raw", "typed", "boom"} {
		if _, err := mgr.Enqueue(ctx, queue.EnqueueRequest{UserID: 1, Kind: kind, MaxAttempts: 2}); err != nil {
			t.Fatal(err)
		}
	}
	mgr.Wait()
	list, err := mgr.ListJobs(ctx, 1, 10)
	if err != nil || len(list.Items) != 3 {
		t.Fatalf("jobs: %+v (%v)", list, err)
	}
	for _, j := range list.Items {
		if j.State != queue.StateFailed || j.Error == nil || strings.Contains(*j.Error, "s3cr3t") || strings.Contains(*j.Error, "discord") {
			t.Fatalf("job %s stored an unsafe error: %+v", j.Kind, j)
		}
	}
	var stored int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM async_jobs WHERE error LIKE '%s3cr3t%' OR result_json LIKE '%s3cr3t%'`).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("secret persisted in async_jobs (%d, %v)", stored, err)
	}
	for _, f := range frames() {
		if strings.Contains(f, "s3cr3t") {
			t.Fatalf("secret pushed over SSE: %s", f)
		}
	}
}

func TestPriorityWithinGroup(t *testing.T) {
	mgr := newManager(t, queue.Options{Concurrency: map[string]int{queue.GroupSync: 1}})
	gate := make(chan struct{})
	var mu sync.Mutex
	var order []string
	mgr.Register("blocker", func(context.Context, *queue.JobContext) (any, error) {
		<-gate
		return nil, nil
	})
	mgr.Register("record", func(_ context.Context, jc *queue.JobContext) (any, error) {
		mu.Lock()
		order = append(order, string(jc.Payload))
		mu.Unlock()
		return nil, nil
	})
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, _ = mgr.Enqueue(ctx, queue.EnqueueRequest{Kind: "blocker", Queue: queue.QueueDefault})
	time.Sleep(20 * time.Millisecond)
	for _, p := range []struct{ q, name string }{
		{queue.QueueLow, "low-1"}, {queue.QueueDefault, "default-1"}, {queue.QueueCritical, "critical-1"}, {queue.QueueCritical, "critical-2"},
	} {
		_, _ = mgr.Enqueue(ctx, queue.EnqueueRequest{Kind: "record", Queue: p.q, Payload: []byte(p.name)})
	}
	close(gate)
	mgr.Wait()
	if strings.Join(order, ",") != "critical-1,critical-2,default-1,low-1" {
		t.Fatalf("order = %v", order)
	}
}

func TestSlowLLMJobsNeverStarveSync(t *testing.T) {
	mgr := newManager(t, queue.Options{Concurrency: map[string]int{queue.GroupLLM: 1, queue.GroupSync: 1}})
	gate := make(chan struct{})
	mgr.Register(queue.KindLLMAnalyze, func(ctx context.Context, _ *queue.JobContext) (any, error) {
		select {
		case <-gate:
		case <-ctx.Done():
		}
		return nil, nil
	})
	mgr.Register(queue.KindSyncProvider, func(context.Context, *queue.JobContext) (any, error) { return "ok", nil })
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for range 3 {
		_, _ = mgr.Enqueue(ctx, queue.EnqueueRequest{Kind: queue.KindLLMAnalyze})
	}
	if _, _, err := mgr.EnqueueAndWait(ctx, queue.EnqueueRequest{Kind: queue.KindSyncProvider}, time.Second); err != nil {
		t.Fatalf("sync starved by LLM jobs: %v", err)
	}
	close(gate)
	mgr.Wait()
}

func TestEnqueueAndWaitIsBounded(t *testing.T) {
	st := openTestDBWithUsers(t)
	mgr := newManager(t, queue.Options{DB: st.DB})
	release := make(chan struct{})
	mgr.Register(queue.KindLLMPlanGenerate, func(context.Context, *queue.JobContext) (any, error) {
		<-release
		return map[string]any{"drafts": 1}, nil
	})
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	js, res, err := mgr.EnqueueAndWait(context.Background(), queue.EnqueueRequest{UserID: 1, Kind: queue.KindLLMPlanGenerate}, 50*time.Millisecond)
	if !errors.Is(err, queue.ErrPending) || res != nil || js == nil || js.State != queue.StateRunning {
		t.Fatalf("want ErrPending with a running status, got %+v %v %v", js, res, err)
	}
	if time.Since(t0) > time.Second {
		t.Fatal("wait not bounded")
	}
	close(release)
	mgr.Wait()
	if got, err := mgr.GetJob(context.Background(), 1, js.ID); err != nil || got.State != queue.StateCompleted {
		t.Fatalf("job must complete after the caller gave up: %+v (%v)", got, err)
	}
}

func TestStartFailsInterruptedJobs(t *testing.T) {
	st := openTestDBWithUsers(t)
	for i, state := range []string{"queued", "running", "retrying", "completed"} {
		if _, err := st.DB.Exec(`INSERT INTO async_jobs (id, user_id, kind, queue, state, label, attempts, max_attempts, created_at, updated_at)
			VALUES (?, 1, 'sync.provider', 'default', ?, '', 1, 1, 1, 1)`, fmt.Sprintf("job_%d", i), state); err != nil {
			t.Fatal(err)
		}
	}
	mgr := newManager(t, queue.Options{DB: st.DB, Now: func() time.Time { return time.Unix(500, 0) }})
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	list, err := mgr.ListJobs(context.Background(), 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range list.Items {
		want := queue.StateFailed
		if j.ID == "job_3" {
			want = queue.StateCompleted
		}
		if j.State != want || (want == queue.StateFailed && (j.Error == nil || !strings.Contains(*j.Error, "redémarrage"))) {
			t.Fatalf("job %s after restart: %+v", j.ID, j)
		}
	}
}

func TestPurgeKeepsActiveAndRecentJobs(t *testing.T) {
	st := openTestDBWithUsers(t)
	rows := []struct {
		id, state string
		at        int64
	}{{"old_done", "completed", 100}, {"old_failed", "failed", 100}, {"old_running", "running", 100}, {"new_done", "completed", 900}}
	for _, r := range rows {
		if _, err := st.DB.Exec(`INSERT INTO async_jobs (id, user_id, kind, queue, state, label, attempts, max_attempts, created_at, updated_at)
			VALUES (?, 1, 'sync.provider', 'default', ?, '', 1, 1, ?, ?)`, r.id, r.state, r.at, r.at); err != nil {
			t.Fatal(err)
		}
	}
	mgr := newManager(t, queue.Options{DB: st.DB})
	n, err := mgr.Purge(context.Background(), time.Unix(500, 0))
	if err != nil || n != 2 {
		t.Fatalf("purged %d (%v), want 2", n, err)
	}
}

func TestShutdownDrainsAndRejects(t *testing.T) {
	mgr := queue.NewManager(queue.Options{Concurrency: map[string]int{queue.GroupSync: 1}})
	started := make(chan struct{})
	var finished atomic.Bool
	mgr.Register("slow", func(ctx context.Context, _ *queue.JobContext) (any, error) {
		close(started)
		time.Sleep(50 * time.Millisecond)
		finished.Store(true)
		return nil, nil
	})
	var ran atomic.Int32
	mgr.Register("queued", func(context.Context, *queue.JobContext) (any, error) { ran.Add(1); return nil, nil })
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, _ = mgr.Enqueue(ctx, queue.EnqueueRequest{Kind: "slow"})
	_, _ = mgr.Enqueue(ctx, queue.EnqueueRequest{Kind: "queued"})
	_, _ = mgr.Enqueue(ctx, queue.EnqueueRequest{Kind: "queued", Delay: time.Hour})
	<-started
	sctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	mgr.Shutdown(sctx)
	mgr.Wait()
	if !finished.Load() || ran.Load() != 0 {
		t.Fatalf("running job must finish and queued ones be dropped (finished=%v ran=%d)", finished.Load(), ran.Load())
	}
	if _, err := mgr.Enqueue(ctx, queue.EnqueueRequest{Kind: "queued"}); !errors.Is(err, queue.ErrClosed) {
		t.Fatalf("enqueue after shutdown: %v", err)
	}
}

func TestWaitCoversTrackedGoroutinesAndChainedJobs(t *testing.T) {
	mgr := newManager(t, queue.Options{})
	var done atomic.Int32
	mgr.Register("child", func(context.Context, *queue.JobContext) (any, error) { done.Add(1); return nil, nil })
	mgr.Register("parent", func(ctx context.Context, _ *queue.JobContext) (any, error) {
		mgr.Go(func() {
			time.Sleep(20 * time.Millisecond)
			_, _ = mgr.Enqueue(context.Background(), queue.EnqueueRequest{Kind: "child"})
		})
		return nil, nil
	})
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	_, _ = mgr.Enqueue(context.Background(), queue.EnqueueRequest{Kind: "parent"})
	mgr.Wait()
	if done.Load() != 1 {
		t.Fatal("Wait returned before the chained job ran")
	}
}
