// Package domain wires the training-data services (providers sync, analysis, reports, athlete data).
package domain

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"stride/internal/apimeter"
	"stride/internal/athlete"
	"stride/internal/cache"
	"stride/internal/egress"
	"stride/internal/env"
	"stride/internal/garmin"
	"stride/internal/provider/strava"
	"stride/internal/provider/weather"
	"stride/internal/secret"
	"stride/internal/store"
)

type Deps struct {
	Env     env.Config
	Store   *store.Store
	Cache   cache.Cache
	Sealer  *secret.Sealer
	Meter   apimeter.Meter
	Strava  strava.API       // nil when Strava is not configured
	Weather weather.Provider // nil when weather is disabled
	Log     *slog.Logger
	// BaseURLs overrides fixed external endpoints in tests (keys: gemini, anthropic, openai, telegram).
	BaseURLs map[string]string
}

type Services struct {
	deps Deps
	log  *slog.Logger
	// Now is overridable in tests.
	Now func() time.Time

	mu       sync.Mutex
	clients  map[int64]*garmin.Client
	running  map[int64]bool
	subsOnce sync.Once
	sf       singleflight.Group
	bg       sync.WaitGroup
	egress   *http.Client
	llmHTTP  *http.Client
}

func New(d Deps) (*Services, error) {
	if d.Store == nil || d.Cache == nil || d.Sealer == nil {
		return nil, fmt.Errorf("domain: store, cache and sealer are required")
	}
	if d.Meter == nil {
		d.Meter = apimeter.Nop()
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	s := &Services{deps: d, log: d.Log, Now: time.Now, clients: map[int64]*garmin.Client{}, running: map[int64]bool{},
		egress: egress.Client(d.Env.AllowPrivateEgress, llmTimeout), llmHTTP: &http.Client{Timeout: llmTimeout}}
	s.ensureStravaSubscription()
	return s, nil
}

func (s *Services) egressHTTP() *http.Client { return s.egress }

// baseURL returns the test override of a fixed external endpoint, or def.
func (s *Services) baseURL(key, def string) string {
	if u := s.deps.BaseURLs[key]; u != "" {
		return strings.TrimRight(u, "/")
	}
	return def
}

func (s *Services) telegramBase() string { return s.baseURL("telegram", "https://api.telegram.org") }

// LastSyncAt returns the most recent provider sync of a user (zero when never synced).
func (s *Services) LastSyncAt(ctx context.Context, userID int64) (time.Time, error) {
	return s.lastSync(ctx, userID)
}

func (s *Services) now() time.Time { return s.Now() }

func (s *Services) db() *sql.DB { return s.deps.Store.DB }

func (s *Services) repo() athlete.Repo { return athlete.Repo{DB: s.db(), Now: s.Now} }

// Wait blocks until background work (lazy syncs, webhook processing) is done; for tests and shutdown.
func (s *Services) Wait() { s.bg.Wait() }

func (s *Services) goBackground(name string, timeout time.Duration, fn func(ctx context.Context)) {
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("💥 Tâche de fond interrompue", "task", name, "panic", fmt.Sprint(r))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		fn(ctx)
	}()
}

// profile returns the athlete profile (defaults when never saved).
func (s *Services) profile(ctx context.Context, uid int64) (athlete.Profile, error) {
	return s.repo().Profile(ctx, uid, s.deps.Env.Timezone)
}

func (s *Services) location(p athlete.Profile) *time.Location { return p.Location() }

func (s *Services) today(p athlete.Profile) string {
	return s.now().In(s.location(p)).Format("2006-01-02")
}

// ---------- cache helpers ----------

func userKey(uid int64, parts ...string) string {
	return fmt.Sprintf("%scache:u:%d:%s", cache.Prefix, uid, strings.Join(parts, ":"))
}

func (s *Services) cacheGet(ctx context.Context, key string) ([]byte, bool) {
	b, ok, err := s.deps.Cache.Get(ctx, key)
	if err != nil {
		s.log.Warn("⚠️ Cache indisponible (lecture)", "err", err.Error())
		return nil, false
	}
	return b, ok
}

func (s *Services) cacheSet(ctx context.Context, key string, val []byte, ttl time.Duration) {
	if err := s.deps.Cache.Set(ctx, key, val, ttl); err != nil {
		s.log.Warn("⚠️ Cache indisponible (écriture)", "err", err.Error())
	}
}

// ---------- integration seams ----------

// MorningSync prefetches overnight wellness and new activities for one user (called by the scheduler).
func (s *Services) MorningSync(ctx context.Context, userID int64) error {
	return s.runSync(apimeter.WithUser(ctx, userID), userID, true)
}

// OnUserCreated bootstraps domain rows for a new user; the first admin gets the env Garmin key
// and Strava tokens if set.
func (s *Services) OnUserCreated(ctx context.Context, userID int64, firstAdmin bool) error {
	if !firstAdmin {
		return nil
	}
	if strings.TrimSpace(s.deps.Env.AdminGarminKey) != "" {
		if _, err := s.ConnectGarmin(ctx, userID, s.deps.Env.AdminGarminKey); err != nil {
			s.log.Warn("🔑 Clé Garmin de l'administrateur refusée ou injoignable", "user", userID, "err", errText(err))
		} else {
			s.log.Info("🔑 Garmin connecté automatiquement pour le premier administrateur", "user", userID)
		}
	}
	if err := s.seedStrava(ctx, userID); err != nil {
		s.log.Warn("🔗 Jetons Strava de l'environnement refusés ou Strava injoignable", "user", userID, "err", errText(err))
	}
	return nil
}

// PurgeUserCache drops cached entries of a user (DB rows go away via ON DELETE CASCADE).
func (s *Services) PurgeUserCache(ctx context.Context, userID int64) error {
	s.dropClient(userID)
	_, err := s.deps.Cache.DelPrefix(ctx, userKey(userID))
	return err
}

// ConnectedUserIDs lists active users with at least one connected provider (scheduler lister).
func (s *Services) ConnectedUserIDs(ctx context.Context) ([]int64, error) {
	return s.userIDs(ctx, 1<<62)
}

func (s *Services) userIDs(ctx context.Context, syncedBefore int64) ([]int64, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT pc.user_id FROM provider_connections pc JOIN users u ON u.id = pc.user_id
		WHERE u.status = 'active' AND pc.status <> 'disconnected'
		GROUP BY pc.user_id HAVING MAX(COALESCE(pc.last_sync_at, 0)) < ? ORDER BY pc.user_id`, syncedBefore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
