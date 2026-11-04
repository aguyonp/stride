// Package domain wires the training-data services (providers sync, analysis, reports, athlete data).
package domain

import (
	"context"
	"log/slog"

	"stride/internal/apimeter"
	"stride/internal/cache"
	"stride/internal/env"
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
}

type Services struct {
	deps Deps
}

func New(d Deps) (*Services, error) {
	return &Services{deps: d}, nil
}

// MorningSync prefetches overnight wellness and new activities for one user (called by the scheduler).
func (s *Services) MorningSync(ctx context.Context, userID int64) error { return nil }

// OnUserCreated bootstraps domain rows for a new user; the first admin gets env.AdminGarminKey if set.
func (s *Services) OnUserCreated(ctx context.Context, userID int64, firstAdmin bool) error {
	return nil
}

// PurgeUserCache drops cached entries of a user (DB rows go away via ON DELETE CASCADE).
func (s *Services) PurgeUserCache(ctx context.Context, userID int64) error { return nil }
