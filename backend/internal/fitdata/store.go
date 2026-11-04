package fitdata

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"patella/internal/garmin"
	"patella/internal/logx"
)

// Store télécharge (via le cache disque du client) et décode les FIT,
// en gardant en mémoire les derniers modèles décodés.
type Store struct {
	g     *garmin.Client
	log   *slog.Logger
	mu    sync.Mutex
	m     map[int64]*Activity
	order []int64
	max   int
}

// NewStore crée un store (max 24 activités décodées en mémoire).
func NewStore(g *garmin.Client, log *slog.Logger) *Store {
	return &Store{g: g, log: log, m: map[int64]*Activity{}, max: 24}
}

// Load retourne l'activité décodée depuis le FIT.
func (s *Store) Load(ctx context.Context, id int64) (*Activity, error) {
	s.mu.Lock()
	if a, ok := s.m[id]; ok {
		s.mu.Unlock()
		s.log.Debug(fmt.Sprintf("💾 cache hit (mémoire) fit-décodé/%d", id))
		return a, nil
	}
	s.mu.Unlock()

	b, err := s.g.ActivityFIT(ctx, id)
	if err != nil {
		return nil, err
	}
	t0 := time.Now()
	a, err := Decode(b)
	if err != nil {
		return nil, err
	}
	s.log.Info(fmt.Sprintf("📦 FIT décodé #%d · %d records · %d laps · %d splits · %s",
		id, len(a.Records), len(a.Laps), len(a.Splits), logx.Ms(time.Since(t0))))

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[id]; !ok {
		s.m[id] = a
		s.order = append(s.order, id)
		for len(s.order) > s.max {
			delete(s.m, s.order[0])
			s.order = s.order[1:]
		}
	}
	return a, nil
}
