package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAllowWithinLimit(t *testing.T) {
	l := New(2, time.Minute)
	now := time.Now()
	if !l.Allow("a", now) || !l.Allow("a", now) {
		t.Fatal("two requests within the limit must be allowed")
	}
	if l.Allow("a", now) {
		t.Fatal("the third request must be refused")
	}
	if !l.Allow("b", now) {
		t.Fatal("a different key has its own budget")
	}
}

func TestWindowResets(t *testing.T) {
	l := New(1, time.Minute)
	now := time.Now()
	if !l.Allow("a", now) {
		t.Fatal("first request must be allowed")
	}
	if l.Allow("a", now) {
		t.Fatal("second request in the same window must be refused")
	}
	if !l.Allow("a", now.Add(time.Minute)) {
		t.Fatal("the counter must reset when the window elapses")
	}
}

func TestSetConfigAndUsed(t *testing.T) {
	l := New(3, time.Minute)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if !l.Allow("a", now) {
			t.Fatalf("request %d within the limit must be allowed", i+1)
		}
	}
	if got := l.Used("a", now); got != 3 {
		t.Fatalf("Used() = %d, want 3", got)
	}
	if got := l.Used("b", now); got != 0 {
		t.Fatalf("Used() for an unused key = %d, want 0", got)
	}
	// A new config restarts the window: one request is allowed, the next is refused.
	l.SetConfig(1, 2*time.Minute)
	if !l.Allow("a", now.Add(time.Minute)) {
		t.Fatal("the first request in the new window must be allowed")
	}
	if l.Allow("a", now.Add(time.Minute)) {
		t.Fatal("the second request must be refused under the new limit")
	}
	// An elapsed window reads 0 without clearing.
	l2 := New(5, time.Minute)
	l2.Allow("k", now)
	if got := l2.Used("k", now.Add(2*time.Minute)); got != 0 {
		t.Fatalf("Used() after the window elapsed = %d, want 0", got)
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	r.RemoteAddr = "203.0.113.7:41022"
	if got := ClientIP(r); got != "203.0.113.7" {
		t.Fatalf("ClientIP() = %q, want 203.0.113.7", got)
	}
	r.RemoteAddr = "203.0.113.7"
	if got := ClientIP(r); got != "203.0.113.7" {
		t.Fatalf("ClientIP() without port = %q, want 203.0.113.7", got)
	}
}
