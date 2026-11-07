package sse_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"stride/internal/auth"
	"stride/internal/cache"
	"stride/internal/observability"
	"stride/internal/sse"
)

func testAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := strconv.ParseInt(r.Header.Get("X-Test-User"), 10, 64)
		if uid <= 0 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		role := auth.RoleUser
		if r.Header.Get("X-Test-Role") == "admin" {
			role = auth.RoleAdmin
		}
		ctx := auth.WithUser(r.Context(), auth.User{ID: uid, Role: role})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type sseFrame struct {
	event string
	data  string
}

func readFrames(r io.Reader, out chan<- sseFrame) {
	sc := bufio.NewScanner(r)
	var cur sseFrame
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, ":"):
			out <- sseFrame{event: ":comment", data: line}
		case strings.HasPrefix(line, "event: "):
			cur.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.data = strings.TrimPrefix(line, "data: ")
		case line == "" && cur.event != "":
			out <- cur
			cur = sseFrame{}
		}
	}
	close(out)
}

func TestHubConnectIsolationAndDisconnect(t *testing.T) {
	metrics := observability.New()
	hub := sse.NewHub(sse.Options{
		Metrics:           metrics,
		HeartbeatInterval: 40 * time.Millisecond,
	})
	defer hub.Close()

	srv := httptest.NewServer(testAuthMiddleware(hub))
	defer srv.Close()

	openStream := func(uid string, role string) (*http.Response, chan sseFrame, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/events/stream", nil)
		req.Header.Set("X-Test-User", uid)
		req.Header.Set("X-Test-Role", role)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("connect user %s: %v", uid, err)
		}
		ch := make(chan sseFrame, 16)
		go readFrames(resp.Body, ch)
		return resp, ch, cancel
	}

	resp1, ch1, cancel1 := openStream("1", "admin")
	defer resp1.Body.Close()
	defer cancel1()

	resp2, ch2, cancel2 := openStream("2", "user")
	defer resp2.Body.Close()
	defer cancel2()

	if ct := resp1.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("unexpected content-type: %q", ct)
	}
	if resp1.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("missing X-Accel-Buffering: %q", resp1.Header.Get("X-Accel-Buffering"))
	}

	// Both receive their own connected frame immediately.
	f1 := <-ch1
	f2 := <-ch2
	if f1.event != sse.EventConnected || !strings.Contains(f1.data, `"userId":1`) {
		t.Fatalf("user 1 connected frame: %+v", f1)
	}
	if f2.event != sse.EventConnected || !strings.Contains(f2.data, `"userId":2`) {
		t.Fatalf("user 2 connected frame: %+v", f2)
	}

	if !hub.HasSubscribers(1) || !hub.HasSubscribers(2) {
		t.Fatal("expected active subscribers for user 1 and 2")
	}

	// User isolation: publish to user 1 must never arrive on user 2.
	hub.PublishToUser(1, sse.EventLLMToken, sse.StreamLLMTokenEvent{
		JobID: "j1", Kind: "activity", Ref: "42", Delta: "Bonjour",
	})

	gotToken := false
	deadline := time.After(500 * time.Millisecond)
	for !gotToken {
		select {
		case fr := <-ch1:
			gotToken = fr.event == sse.EventLLMToken && strings.Contains(fr.data, `"delta":"Bonjour"`)
		case <-deadline:
			t.Fatal("timed out waiting for user 1 token event")
		}
	}

	// Verify user 2 only receives heartbeats (no user 1 event).
	sawHeartbeat := false
	checkTimer := time.After(90 * time.Millisecond)
drain2:
	for {
		select {
		case fr := <-ch2:
			if fr.event == ":comment" && strings.HasPrefix(fr.data, ": heartbeat ") {
				sawHeartbeat = true
			} else {
				t.Fatalf("user 2 received leaked event: %+v", fr)
			}
		case <-checkTimer:
			break drain2
		}
	}
	if !sawHeartbeat {
		t.Fatal("expected heartbeat comment on user 2 stream")
	}

	// Disconnect cleanup: cancelling user 2 must drop subscriber count cleanly.
	cancel2()
	deadline = time.After(500 * time.Millisecond)
	for hub.HasSubscribers(2) {
		select {
		case <-deadline:
			t.Fatal("user 2 subscriber was not cleaned up after disconnect")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestHubValkeyPubSubFanout(t *testing.T) {
	mr := miniredis.RunT(t)
	vk1, err := cache.NewValkey("redis://" + mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer vk1.Close()
	vk2, err := cache.NewValkey("redis://" + mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer vk2.Close()

	hubA := sse.NewHub(sse.Options{Valkey: vk1.Client(), HeartbeatInterval: time.Minute})
	defer hubA.Close()
	hubB := sse.NewHub(sse.Options{Valkey: vk2.Client(), HeartbeatInterval: time.Minute})
	defer hubB.Close()

	srvA := httptest.NewServer(testAuthMiddleware(hubA))
	defer srvA.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srvA.URL+"/api/events/stream", nil)
	req.Header.Set("X-Test-User", "7")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	ch := make(chan sseFrame, 8)
	go readFrames(resp.Body, ch)
	<-ch // connected

	// Wait briefly for hubA's SUBSCRIBE goroutine to attach to miniredis.
	time.Sleep(40 * time.Millisecond)

	// Publishing on hubB fans out over Valkey Pub/Sub to user 7 connected on hubA.
	hubB.PublishToUser(7, sse.EventSyncUpdated, map[string]any{"running": true})

	select {
	case fr := <-ch:
		if fr.event != sse.EventSyncUpdated || !strings.Contains(fr.data, `"running":true`) {
			t.Fatalf("unexpected fanout frame: %+v", fr)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Valkey Pub/Sub SSE fanout")
	}
}
