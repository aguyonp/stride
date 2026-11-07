package cache

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"stride/internal/observability"
)

func TestMemoryTTLAndPrefix(t *testing.T) {
	ctx := context.Background()
	c := NewMemory()
	c.Set(ctx, Prefix+"cache:a", []byte("1"), time.Minute)
	c.Set(ctx, Prefix+"cache:b", []byte("2"), time.Millisecond)
	c.Set(ctx, Prefix+"webauthn:x", []byte("3"), time.Minute)
	time.Sleep(5 * time.Millisecond)
	if _, ok, _ := c.Get(ctx, Prefix+"cache:b"); ok {
		t.Fatal("expired key returned")
	}
	if n, _ := c.DelPrefix(ctx, Prefix+"cache:"); n != 1 {
		t.Fatalf("DelPrefix removed %d", n)
	}
	if _, ok, _ := c.Get(ctx, Prefix+"webauthn:x"); !ok {
		t.Fatal("other namespace must survive")
	}
}

func TestInstrumentedRecordsOperations(t *testing.T) {
	ctx := context.Background()
	m := observability.New()
	c := NewInstrumented(NewMemory(), "memory", m)
	_ = c.Set(ctx, Prefix+"k", []byte("v"), time.Minute)
	_, _, _ = c.Get(ctx, Prefix+"k")
	_, _, _ = c.Get(ctx, Prefix+"absent")
	_ = c.Del(ctx, Prefix+"k")
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	for _, series := range []string{
		`stride_cache_operations_total{backend="memory",operation="set",result="ok"} 1`,
		`stride_cache_operations_total{backend="memory",operation="get",result="hit"} 1`,
		`stride_cache_operations_total{backend="memory",operation="get",result="miss"} 1`,
		`stride_cache_operations_total{backend="memory",operation="del",result="ok"} 1`,
	} {
		if !strings.Contains(rec.Body.String(), series) {
			t.Fatalf("missing %s", series)
		}
	}
}
