package queue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"stride/internal/queue"
)

func waitState(t *testing.T, m *queue.Manager, id, want string) *queue.JobStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		j, err := m.Lookup(context.Background(), id)
		if err == nil && j.State == want {
			return j
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s: want %s, got %+v (%v)", id, want, j, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAdminCancelStatsListAndRetry(t *testing.T) {
	st := openTestDBWithUsers(t)
	m := newManager(t, queue.Options{DB: st.DB, Concurrency: map[string]int{queue.GroupSync: 1}})
	started := make(chan struct{}, 4)
	payloads := make(chan string, 4)
	m.Register(queue.KindSyncHistory, func(ctx context.Context, jc *queue.JobContext) (any, error) {
		payloads <- string(jc.Payload)
		started <- struct{}{}
		<-ctx.Done()
		return nil, queue.Transient("Fournisseur indisponible.", ctx.Err())
	})
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	running, err := m.Enqueue(ctx, queue.EnqueueRequest{UserID: 1, Kind: queue.KindSyncHistory, MaxAttempts: 3, Payload: map[string]string{"from": "2024-01-01"}})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	queued, err := m.Enqueue(ctx, queue.EnqueueRequest{UserID: 2, Kind: queue.KindSyncHistory, Payload: map[string]string{"from": "2020-01-01"}})
	if err != nil {
		t.Fatal(err)
	}

	stats := m.Stats()
	if len(stats) != 3 || stats[0].Group != queue.GroupSync || stats[0].Workers != 1 || stats[0].Running != 1 || stats[0].Queued != 1 {
		t.Fatalf("stats: %+v", stats)
	}
	active, err := m.ListAll(ctx, queue.AdminFilter{State: "active"})
	if err != nil || len(active) != 2 || active[0].UserID == active[1].UserID {
		t.Fatalf("list across users: %+v %v", active, err)
	}

	// A queued job fails at once; a running one when its handler returns, without retry.
	if j, err := m.Cancel(ctx, queued.ID); err != nil || j.State != queue.StateFailed || j.Error == nil || *j.Error != "Tâche annulée par un administrateur." {
		t.Fatalf("cancel queued: %+v %v", j, err)
	}
	if _, err := m.Cancel(ctx, running.ID); err != nil {
		t.Fatalf("cancel running: %v", err)
	}
	if j := waitState(t, m, running.ID, queue.StateFailed); j.Attempts != 1 || *j.Error != "Tâche annulée par un administrateur." {
		t.Fatalf("running job must fail once, cancelled: %+v", j)
	}
	if _, err := m.Cancel(ctx, running.ID); !errors.Is(err, queue.ErrNotCancellable) {
		t.Fatalf("finished job: %v", err)
	}
	if _, err := m.Cancel(ctx, "job_unknown"); !errors.Is(err, queue.ErrNotFound) {
		t.Fatalf("unknown job: %v", err)
	}
	if n, err := m.CountFailedSince(ctx, time.Now().Add(-time.Hour)); err != nil || n != 2 {
		t.Fatalf("failed count: %d %v", n, err)
	}

	// Retry replays the stored payload for the same user.
	<-payloads
	retried, err := m.Retry(ctx, queued.ID)
	if err != nil || retried.UserID != 2 || retried.ID == queued.ID || retried.Kind != queue.KindSyncHistory {
		t.Fatalf("retry: %+v %v", retried, err)
	}
	select {
	case p := <-payloads:
		if p != `{"from":"2020-01-01"}` {
			t.Fatalf("replayed payload: %s", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("retried job never ran")
	}
	if _, err := m.Cancel(ctx, retried.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, retried.ID, queue.StateFailed)

	// Non-replayable kinds never store their payload and cannot be retried.
	m.Register(queue.KindLLMAnalyze, func(context.Context, *queue.JobContext) (any, error) {
		return nil, queue.Permanent("Analyse impossible.", nil)
	})
	llm, err := m.Enqueue(ctx, queue.EnqueueRequest{UserID: 1, Kind: queue.KindLLMAnalyze, Payload: map[string]string{"prompt": "données personnelles"}})
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, m, llm.ID, queue.StateFailed)
	var stored *string
	if err := st.DB.QueryRow(`SELECT payload_json FROM async_jobs WHERE id = ?`, llm.ID).Scan(&stored); err != nil || stored != nil {
		t.Fatalf("llm payload must not be stored: %v %v", stored, err)
	}
	if _, err := m.Retry(ctx, llm.ID); !errors.Is(err, queue.ErrNotReplayable) {
		t.Fatalf("llm retry: %v", err)
	}
	if _, err := m.Retry(ctx, retried.ID+"x"); !errors.Is(err, queue.ErrNotFound) {
		t.Fatalf("unknown retry: %v", err)
	}
}
