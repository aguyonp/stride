// Package cache is a TTL key/value cache: Valkey in production, in-memory for dev and tests.
// Every key must have a TTL; keys are namespaced with Prefix.
package cache

import (
	"context"
	"strings"
	"sync"
	"time"
)

const Prefix = "stride:"

type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	// DelPrefix removes every key starting with prefix and returns how many were removed.
	DelPrefix(ctx context.Context, prefix string) (int, error)
	Ping(ctx context.Context) error
	Close()
}

type memEntry struct {
	val []byte
	exp time.Time
}

type Memory struct {
	mu sync.Mutex
	m  map[string]memEntry
}

func NewMemory() *Memory { return &Memory{m: map[string]memEntry{}} }

func (c *Memory) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok {
		return nil, false, nil
	}
	if time.Now().After(e.exp) {
		delete(c.m, key)
		return nil, false, nil
	}
	return append([]byte(nil), e.val...), true, nil
}

func (c *Memory) Set(_ context.Context, key string, val []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if len(c.m) > 4096 {
		for k, e := range c.m {
			if now.After(e.exp) {
				delete(c.m, k)
			}
		}
	}
	c.m[key] = memEntry{val: append([]byte(nil), val...), exp: now.Add(ttl)}
	return nil
}

func (c *Memory) Del(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.m, k)
	}
	return nil
}

func (c *Memory) DelPrefix(_ context.Context, prefix string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for k := range c.m {
		if strings.HasPrefix(k, prefix) {
			delete(c.m, k)
			n++
		}
	}
	return n, nil
}

func (c *Memory) Ping(context.Context) error { return nil }
func (c *Memory) Close()                     {}
