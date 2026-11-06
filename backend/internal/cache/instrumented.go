package cache

import (
	"context"
	"time"

	"stride/internal/observability"
)

// Store is an alias for Cache matching the v4 contract terminology.
type Store = Cache

// Instrumented wraps a Cache implementation and records Prometheus cache metrics.
type Instrumented struct {
	inner   Cache
	backend string
	metrics *observability.Metrics
}

// NewInstrumented wraps c ("valkey" or "memory") with Prometheus metrics instrumentation.
func NewInstrumented(c Cache, backend string, m *observability.Metrics) *Instrumented {
	if backend != "valkey" {
		backend = "memory"
	}
	return &Instrumented{inner: c, backend: backend, metrics: m}
}

func (c *Instrumented) Unwrap() Cache { return c.inner }

func (c *Instrumented) Get(ctx context.Context, key string) ([]byte, bool, error) {
	t0 := time.Now()
	b, ok, err := c.inner.Get(ctx, key)
	res := "miss"
	switch {
	case err != nil:
		res = "error"
	case ok:
		res = "hit"
	}
	c.metrics.ObserveCache(c.backend, "get", res, time.Since(t0))
	return b, ok, err
}

func (c *Instrumented) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	t0 := time.Now()
	err := c.inner.Set(ctx, key, val, ttl)
	res := "ok"
	if err != nil {
		res = "error"
	}
	c.metrics.ObserveCache(c.backend, "set", res, time.Since(t0))
	return err
}

func (c *Instrumented) Del(ctx context.Context, keys ...string) error {
	t0 := time.Now()
	err := c.inner.Del(ctx, keys...)
	res := "ok"
	if err != nil {
		res = "error"
	}
	c.metrics.ObserveCache(c.backend, "del", res, time.Since(t0))
	return err
}

func (c *Instrumented) DelPrefix(ctx context.Context, prefix string) (int, error) {
	t0 := time.Now()
	n, err := c.inner.DelPrefix(ctx, prefix)
	res := "ok"
	if err != nil {
		res = "error"
	}
	c.metrics.ObserveCache(c.backend, "del_prefix", res, time.Since(t0))
	return n, err
}

func (c *Instrumented) Ping(ctx context.Context) error { return c.inner.Ping(ctx) }
func (c *Instrumented) Close()                         { c.inner.Close() }
