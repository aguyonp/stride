package queue_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"stride/internal/auth"
	"stride/internal/observability"
	"stride/internal/queue"
	"stride/internal/sse"
	"stride/internal/store"
)

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

func runEngineSuite(t *testing.T, valkeyURL string, fastTime func(time.Duration)) {
	t.Helper()
	st := openTestDBWithUsers(t)
	metrics := observability.New()
	hub := sse.NewHub(sse.Options{
		Metrics:           metrics,
		HeartbeatInterval: time.Minute,
	})
	defer hub.Close()

	mgr, err := queue.NewManager(queue.Options{
		DB:          st.DB,
		ValkeyURL:   valkeyURL,
		SSE:         hub,
		Metrics:     metrics,
		Concurrency: 2,
		BaseBackoff: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer mgr.Close()

	// Connect SSE listener for user 1 to verify real-time job.updated frames.
	sseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.User{ID: 1, Role: auth.RoleUser})))
	}))
	defer sseSrv.Close()

	sseCtx, cancelSSE := context.WithCancel(context.Background())
	defer cancelSSE()
	req, _ := http.NewRequestWithContext(sseCtx, "GET", sseSrv.URL+"/api/events/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect sse: %v", err)
	}
	defer resp.Body.Close()

	var sseMu sync.Mutex
	var jobEvents []queue.JobStatus
	readyCh := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(resp.Body)
		var ev, data string
		first := true
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "event: ") {
				ev = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				data = strings.TrimPrefix(line, "data: ")
			} else if line == "" && ev != "" {
				if first && ev == sse.EventConnected {
					first = false
					close(readyCh)
				} else if ev == sse.EventJobUpdated {
					var js queue.JobStatus
					if json.Unmarshal([]byte(data), &js) == nil {
						sseMu.Lock()
						jobEvents = append(jobEvents, js)
						sseMu.Unlock()
					}
				}
				ev, data = "", ""
			}
		}
	}()
	<-readyCh

	// 1. Register handlers: progress + result, transient retry, non-retryable failure.
	mgr.Register(queue.KindImportActivity, func(_ context.Context, jc *queue.JobContext) (any, error) {
		proc, tot := 2, 4
		jc.UpdateProgress("Import FIT : 2/4 blocs", &proc, &tot, nil)
		return map[string]any{"activityId": 42, "created": true}, nil
	})

	var webhookAttempts atomic.Int32
	mgr.Register(queue.KindWebhookDeliver, func(_ context.Context, _ *queue.JobContext) (any, error) {
		n := webhookAttempts.Add(1)
		if n < 3 {
			if fastTime != nil {
				go func() {
					time.Sleep(25 * time.Millisecond)
					fastTime(500 * time.Millisecond)
				}()
			}
			return nil, errors.New("502 upstream gateway")
		}
		return map[string]any{"delivered": true}, nil
	})

	var nonRetryAttempts atomic.Int32
	mgr.Register(queue.KindLLMAnalyze, func(_ context.Context, _ *queue.JobContext) (any, error) {
		nonRetryAttempts.Add(1)
		return nil, queue.NonRetryable(errors.New("format invalide"))
	})

	if err := mgr.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Execute synchronous EnqueueAndWait on critical queue with progress + result.
	ctx := context.Background()
	stImport, res, err := mgr.EnqueueAndWait(ctx, queue.EnqueueRequest{
		UserID:  1,
		Kind:    queue.KindImportActivity,
		Payload: map[string]any{"filename": "run.fit"},
	})
	if err != nil {
		t.Fatalf("EnqueueAndWait import: %v", err)
	}
	if stImport.State != queue.StateCompleted || stImport.Queue != queue.QueueCritical {
		t.Fatalf("unexpected import status: %+v", stImport)
	}
	if res == nil || stImport.Result["created"] != true {
		t.Fatalf("missing result in job status: %+v", stImport)
	}

	// User isolation check: user 2 cannot see user 1's job.
	if _, err := mgr.GetJob(ctx, 2, stImport.ID); !errors.Is(err, queue.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for wrong user, got %v", err)
	}
	listU2, err := mgr.ListJobs(ctx, 2, 20)
	if err != nil || len(listU2.Items) != 0 {
		t.Fatalf("expected empty job list for user 2, got %+v (%v)", listU2, err)
	}

	// Retry with exponential backoff: fails twice, succeeds on 3rd attempt.
	retryJob, err := mgr.Enqueue(ctx, queue.EnqueueRequest{
		UserID:      1,
		Kind:        queue.KindWebhookDeliver,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("Enqueue retry job: %v", err)
	}

	// Non-retryable error fails on attempt 1 despite MaxAttempts=4.
	failJob, _, failErr := mgr.EnqueueAndWait(ctx, queue.EnqueueRequest{
		UserID:      2,
		Kind:        queue.KindLLMAnalyze,
		MaxAttempts: 4,
	})
	if failErr == nil {
		t.Fatal("expected error from non-retryable job")
	}
	if failJob.State != queue.StateFailed || nonRetryAttempts.Load() != 1 {
		t.Fatalf("expected immediate failure on attempt 1, got state=%s attempts=%d", failJob.State, nonRetryAttempts.Load())
	}

	// Wait for retryJob to complete.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if fastTime != nil {
			fastTime(300 * time.Millisecond)
		}
		got, err := mgr.GetJob(ctx, 1, retryJob.ID)
		if err == nil && got.State == queue.StateCompleted {
			if got.Attempts != 3 {
				t.Fatalf("expected 3 attempts, got %d", got.Attempts)
			}
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	gotRetry, err := mgr.GetJob(ctx, 1, retryJob.ID)
	if err != nil || gotRetry.State != queue.StateCompleted {
		t.Fatalf("retry job did not complete: %+v (%v)", gotRetry, err)
	}

	mgr.Wait()

	// Verify SSE job.updated notifications arrived for user 1 (and none for user 2!).
	time.Sleep(30 * time.Millisecond)
	sseMu.Lock()
	defer sseMu.Unlock()
	if len(jobEvents) < 4 {
		t.Fatalf("expected multiple SSE job.updated frames, got %d", len(jobEvents))
	}
	sawProgress := false
	for _, ev := range jobEvents {
		if ev.UserID != 1 {
			t.Fatalf("SSE leaked job event for user %d", ev.UserID)
		}
		if ev.ID == stImport.ID && ev.ProgressPct != nil && *ev.ProgressPct == 50 {
			sawProgress = true
		}
	}
	if !sawProgress {
		t.Fatalf("expected 50%% progress SSE event for import job, got %+v", jobEvents)
	}
}

func TestMemoryEngineSuite(t *testing.T) {
	runEngineSuite(t, "", nil)
}

func TestMemoryPriorityOrdering(t *testing.T) {
	mgr, err := queue.NewManager(queue.Options{
		Concurrency: 1,
		BaseBackoff: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	gate := make(chan struct{})
	var mu sync.Mutex
	var order []string

	mgr.Register("blocker", func(_ context.Context, _ *queue.JobContext) (any, error) {
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
	// Occupy the single worker so subsequent submissions queue up across priorities.
	_, _ = mgr.Enqueue(ctx, queue.EnqueueRequest{Kind: "blocker", Queue: queue.QueueDefault})
	time.Sleep(20 * time.Millisecond)

	_, _ = mgr.Enqueue(ctx, queue.EnqueueRequest{Kind: "record", Queue: queue.QueueLow, Payload: []byte("low-1")})
	_, _ = mgr.Enqueue(ctx, queue.EnqueueRequest{Kind: "record", Queue: queue.QueueDefault, Payload: []byte("default-1")})
	_, _ = mgr.Enqueue(ctx, queue.EnqueueRequest{Kind: "record", Queue: queue.QueueCritical, Payload: []byte("critical-1")})
	_, _ = mgr.Enqueue(ctx, queue.EnqueueRequest{Kind: "record", Queue: queue.QueueCritical, Payload: []byte("critical-2")})

	close(gate)
	mgr.Wait()

	if len(order) != 4 || order[0] != "critical-1" || order[1] != "critical-2" {
		t.Fatalf("expected critical tasks processed first, got order=%v", order)
	}
}

func TestAsynqValkeyEngineSuite(t *testing.T) {
	mr := miniredis.RunT(t)
	runEngineSuite(t, "redis://"+mr.Addr(), mr.FastForward)
}
