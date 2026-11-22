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
