package garmin

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Durées de vie du cache mémoire.
const (
	TTLActivityList = 2 * time.Minute
	TTLWellness     = 10 * time.Minute
	TTLProfile      = time.Hour
)

// policy décrit où mettre en cache une réponse.
type policy struct {
	disk bool          // cache disque immuable
	ttl  time.Duration // cache mémoire (si !disk ou si persist refusé)
}

type memEntry struct {
	b   []byte
	exp time.Time
}

type memCache struct {
	mu sync.Mutex
	m  map[string]memEntry
}

func newMemCache() *memCache { return &memCache{m: map[string]memEntry{}} }

func (mc *memCache) get(k string, now time.Time) ([]byte, bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	e, ok := mc.m[k]
	if !ok {
		return nil, false
	}
	if now.After(e.exp) {
		delete(mc.m, k)
		return nil, false
	}
	return e.b, true
}

func (mc *memCache) set(k string, b []byte, exp time.Time, now time.Time) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if len(mc.m) > 512 {
		for kk, e := range mc.m {
			if now.After(e.exp) {
				delete(mc.m, kk)
			}
		}
	}
	mc.m[k] = memEntry{b: b, exp: exp}
}

func (mc *memCache) clear() {
	mc.mu.Lock()
	mc.m = map[string]memEntry{}
	mc.mu.Unlock()
}

// diskPath retourne le chemin disque d'une clé de cache ("sleep/2026-11-01.json").
func (c *Client) diskPath(key string) string {
	return filepath.Join(c.dataDir, "cache", filepath.FromSlash(key))
}

func isEmptyBody(b []byte) bool {
	t := bytes.TrimSpace(b)
	if len(t) == 0 {
		return true
	}
	switch string(t) {
	case "null", "{}", "[]", `""`:
		return true
	}
	return false
}

// cached retourne la réponse en cache ou appelle fetch (dédupliqué via singleflight).
// persist (optionnel) peut refuser la mise en cache disque d'une réponse précise
// (ex. activité terminée il y a peu) : elle est alors gardée en mémoire (ttl).
func (c *Client) cached(ctx context.Context, key string, pol policy, fetch func(context.Context) ([]byte, error), persist func([]byte) bool) ([]byte, error) {
	now := c.now()
	if pol.disk && c.dataDir != "" {
		if b, err := os.ReadFile(c.diskPath(key)); err == nil {
			c.log.Debug("💾 cache hit (disque) " + key)
			return b, nil
		}
	}
	if b, ok := c.mem.get(key, now); ok {
		c.log.Debug("💾 cache hit (mémoire) " + key)
		return b, nil
	}
	c.log.Debug("💾 cache miss " + key)
	v, err, _ := c.sf.Do(key, func() (any, error) {
		// La requête partagée ne doit pas être annulée par le premier appelant.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*requestTimeout)
		defer cancel()
		b, err := fetch(fctx)
		if err != nil {
			return nil, err
		}
		stored := false
		if pol.disk && c.dataDir != "" && !isEmptyBody(b) && (persist == nil || persist(b)) {
			p := c.diskPath(key)
			if err := os.MkdirAll(filepath.Dir(p), 0o700); err == nil {
				if err := writeFileAtomic(p, b, 0o600); err == nil {
					stored = true
					c.log.Debug("💾 écrit sur disque " + key)
				} else {
					c.log.Warn("⚠️ Écriture cache disque impossible", "clé", key, "err", err.Error())
				}
			}
		}
		if !stored {
			ttl := pol.ttl
			if ttl <= 0 {
				ttl = TTLWellness
			}
			c.mem.set(key, b, c.now().Add(ttl), c.now())
		}
		return b, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
}

// wellnessPolicy : disque si la date (fin de plage) est antérieure à aujourd'hui−1,
// sinon mémoire 10 min.
func (c *Client) wellnessPolicy(endDate string) policy {
	d, err := time.ParseInLocation("2006-01-02", endDate, c.loc())
	if err != nil {
		return policy{ttl: TTLWellness}
	}
	today := c.now().In(c.loc())
	limit := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, c.loc()).AddDate(0, 0, -1)
	if d.Before(limit) {
		return policy{disk: true}
	}
	return policy{ttl: TTLWellness}
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// safeKey normalise un composant de clé de cache pour un nom de fichier.
func safeKey(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		}
		return '_'
	}, s)
}
