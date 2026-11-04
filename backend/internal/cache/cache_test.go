package cache

import (
	"context"
	"testing"
	"time"
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
