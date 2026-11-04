package cache

import (
	"context"
	"time"

	"github.com/valkey-io/valkey-go"
)

type Valkey struct {
	c valkey.Client
}

// NewValkey accepts redis://[user:pass@]host:port/db URLs.
func NewValkey(url string) (*Valkey, error) {
	opt, err := valkey.ParseURL(url)
	if err != nil {
		return nil, err
	}
	opt.DisableCache = true
	c, err := valkey.NewClient(opt)
	if err != nil {
		return nil, err
	}
	return &Valkey{c: c}, nil
}

func (v *Valkey) Get(ctx context.Context, key string) ([]byte, bool, error) {
	b, err := v.c.Do(ctx, v.c.B().Get().Key(key).Build()).AsBytes()
	if valkey.IsValkeyNil(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

func (v *Valkey) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	if ttl < time.Second {
		ttl = time.Second
	}
	return v.c.Do(ctx, v.c.B().Set().Key(key).Value(valkey.BinaryString(val)).Ex(ttl).Build()).Error()
}

func (v *Valkey) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return v.c.Do(ctx, v.c.B().Del().Key(keys...).Build()).Error()
}

func (v *Valkey) DelPrefix(ctx context.Context, prefix string) (int, error) {
	var cursor uint64
	n := 0
	for {
		e, err := v.c.Do(ctx, v.c.B().Scan().Cursor(cursor).Match(prefix+"*").Count(500).Build()).AsScanEntry()
		if err != nil {
			return n, err
		}
		if len(e.Elements) > 0 {
			if err := v.c.Do(ctx, v.c.B().Unlink().Key(e.Elements...).Build()).Error(); err != nil {
				return n, err
			}
			n += len(e.Elements)
		}
		if e.Cursor == 0 {
			return n, nil
		}
		cursor = e.Cursor
	}
}

func (v *Valkey) Ping(ctx context.Context) error {
	return v.c.Do(ctx, v.c.B().Ping().Build()).Error()
}

func (v *Valkey) Close() { v.c.Close() }
