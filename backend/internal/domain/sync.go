package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"stride/internal/analysis"
	"stride/internal/apimeter"
	"stride/internal/athlete"
	"stride/internal/garmin"
	"stride/internal/view"
)

const (
	lazyCooldown    = 5 * time.Minute
	initialHistory  = 56 // days listed on the first sync (load ratio + weekly trends)
	listTTL         = 2 * time.Minute
	prefetchDays    = 7
	prefetchMax     = 6
	syncTimeout     = 5 * time.Minute
	firstSyncBudget = 40 * time.Second
)

func (s *Services) setRunning(uid int64, v bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v && s.running[uid] {
		return false
	}
	if v {
		s.running[uid] = true
	} else {
		delete(s.running, uid)
	}
	return true
}

func (s *Services) isRunning(uid int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running[uid]
}

// startSync launches a background sync for uid (no-op when one is running).
func (s *Services) startSync(uid int64) {
	s.goBackground("sync", syncTimeout, func(ctx context.Context) {
		if err := s.runSync(apimeter.WithUser(ctx, uid), uid, false); err != nil && !errors.Is(err, errSyncRunning) {
			s.log.Warn("🔄 Synchronisation en échec", "user", uid, "err", errText(err))
		}
	})
}

var errSyncRunning = errors.New("sync already running")

// Sync is the lazy sync of POST /api/sync: non-blocking, at most every 5 minutes.
func (s *Services) Sync(ctx context.Context, uid int64) (*view.SyncStatus, error) {
	st, err := s.SyncStatus(ctx, uid)
	if err != nil {
		return nil, err
	}
	if st.Running || st.CooldownUntil != nil {
		return st, nil
	}
	if st.Providers.Garmin == nil && st.Providers.Strava == nil {
		return st, nil
	}
	s.startSync(uid)
	st.Running = true
	return st, nil
}

func (s *Services) SyncStatus(ctx context.Context, uid int64) (*view.SyncStatus, error) {
	st := &view.SyncStatus{Running: s.isRunning(uid)}
	var lastSync, lastLive int64
	for _, p := range []string{pGarmin, pStrava} {
		cn, err := s.getConn(ctx, uid, p)
		if err != nil {
			return nil, err
		}
		if !cn.connected() {
			continue
		}
		ps := &view.ProviderSync{LastSyncAt: rfc3339(cn.LastSyncAt.Int64), NewActivities: cn.LastNew}
		if cn.LastError.Valid && cn.LastErrorAt.Int64 >= cn.LastSyncAt.Int64 {
			ps.Error = &cn.LastError.String
		}
		if p == pGarmin {
			st.Providers.Garmin = ps
		} else {
			st.Providers.Strava = ps
		}
		lastSync = max(lastSync, cn.LastSyncAt.Int64)
		lastLive = max(lastLive, cn.LastLiveAt.Int64)
	}
	st.LastSyncAt, st.LastLiveAt = rfc3339(lastSync), rfc3339(lastLive)
	until := time.Unix(lastSync, 0).Add(lazyCooldown)
	for _, ps := range s.deps.Meter.Snapshot().Providers {
		if ps.CooldownUntil != nil && ps.CooldownUntil.After(until) && (ps.Provider == apimeter.Garmin || ps.Provider == apimeter.Strava) {
			until = *ps.CooldownUntil
		}
	}
	if lastSync > 0 && until.After(s.now()) {
		u := until.UTC().Format(time.RFC3339)
		st.CooldownUntil = &u
	}
	return st, nil
}

// runSync lists new activities (Garmin then Strava, which fuses into Garmin activities), prefetches
// the bundles of recent new activities and the recent wellness days.
func (s *Services) runSync(ctx context.Context, uid int64, morning bool) error {
	if !s.setRunning(uid, true) {
		return errSyncRunning
	}
	defer s.setRunning(uid, false)
	t0 := time.Now()
	p, err := s.profile(ctx, uid)
	if err != nil {
		return err
	}
	today := s.today(p)
	var errs []error
	newG, newS := 0, 0
	if cn, err := s.getConn(ctx, uid, pGarmin); err == nil && cn.connected() {
		from := initialFrom(cn, today)
		n, ids, err := s.syncGarminList(ctx, uid, from, today)
		newG = n
		s.finishProviderSync(ctx, uid, pGarmin, n, err, cn.HistoryFrom.String == "", analysis.DateAdd(today, -initialHistory))
		if err != nil {
			errs = append(errs, err)
		} else {
			s.prefetchBundles(ctx, uid, ids)
			wellFrom := analysis.DateAdd(today, -1)
			if morning {
				wellFrom = analysis.DateAdd(today, -6)
			}
			wr := s.wellness(ctx, uid, p, wellFrom, today, []string{analysis.DateAdd(today, -1), today}, &fresh{})
			for _, w := range wr.warnings {
				s.log.Warn("⚠️ "+w, "user", uid)
			}
		}
	}
	if cn, err := s.getConn(ctx, uid, pStrava); err == nil && cn.connected() && s.stravaAvailable() == nil {
		n, err := s.syncStravaList(ctx, uid, cn, today)
		newS = n
		s.finishProviderSync(ctx, uid, pStrava, n, err, cn.HistoryFrom.String == "", analysis.DateAdd(today, -initialHistory))
		if err != nil {
			errs = append(errs, err)
		}
	}
	_, _ = s.deps.Cache.DelPrefix(ctx, userKey(uid, "glist"))
	s.log.Info(fmt.Sprintf("🔄 Synchro %s en %s", map[bool]string{true: "matinale", false: "à la demande"}[morning], time.Since(t0).Round(time.Millisecond)),
		"user", uid, "nouvellesGarmin", newG, "nouvellesStrava", newS, "erreurs", len(errs))
	return errors.Join(errs...)
}

func initialFrom(cn *conn, today string) string {
	if cn.LastSyncAt.Valid && cn.HistoryFrom.Valid {
		return analysis.DateAdd(time.Unix(cn.LastSyncAt.Int64, 0).UTC().Format("2006-01-02"), -2)
	}
	return analysis.DateAdd(today, -initialHistory)
}

func (s *Services) finishProviderSync(ctx context.Context, uid int64, provider string, n int, err error, first bool, historyFrom string) {
	now := s.now().Unix()
	if err != nil {
		s.setConnError(ctx, uid, provider, err)
		return
	}
	q := `UPDATE provider_connections SET status = 'ok', last_sync_at = ?, last_live_at = ?, last_new_activities = ?, last_error = NULL, last_error_at = NULL, updated_at = ?`
	args := []any{now, now, n, now}
	if first {
		q += `, history_from = ?`
		args = append(args, historyFrom)
	}
	q += ` WHERE user_id = ? AND provider = ?`
	args = append(args, uid, provider)
	_, _ = s.db().ExecContext(ctx, q, args...)
}

// garminList returns the raw activity list between two dates (cached 2 min).
func (s *Services) garminList(ctx context.Context, uid int64, g *garmin.Client, from, to string) ([]json.RawMessage, error) {
	h := sha256.Sum256([]byte(from + "|" + to))
	key := userKey(uid, "glist", hex.EncodeToString(h[:6]))
	if raw, ok := s.cacheGet(ctx, key); ok {
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) == nil {
			return items, nil
		}
	}
	items, err := g.ListActivitiesRange(ctx, from, to)
	if err != nil {
		return nil, err
	}
	if b, err := json.Marshal(items); err == nil {
		s.cacheSet(ctx, key, b, listTTL)
	}
	return items, nil
}

// syncGarminList upserts the listed activities and returns how many are new (and their ids).
func (s *Services) syncGarminList(ctx context.Context, uid int64, from, to string) (int, []int64, error) {
	g, err := s.garminClient(ctx, uid)
	if err != nil {
		return 0, nil, err
	}
	items, err := s.garminList(ctx, uid, g, from, to)
	if err != nil {
		return 0, nil, providerErr(pGarmin, err)
	}
	n := 0
	var ids []int64
	for _, raw := range items {
		id, created, err := s.upsertGarmin(ctx, uid, raw)
		if err != nil {
			return n, ids, err
		}
		if created {
			n++
			ids = append(ids, id)
		}
	}
	return n, ids, nil
}

// prefetchBundles fetches the bundles of the newest activities of the last 7 days.
func (s *Services) prefetchBundles(ctx context.Context, uid int64, ids []int64) {
	limit := s.now().Add(-prefetchDays * 24 * time.Hour).Unix()
	count := 0
	for i := len(ids) - 1; i >= 0 && count < prefetchMax; i-- {
		a, err := s.activity(ctx, uid, ids[i])
		if err != nil || a.StartUTC < limit || a.Final {
			continue
		}
		count++
		if err := s.ensureBundle(ctx, uid, a, &fresh{}); err != nil {
			s.log.Warn("⚠️ Préchargement d'activité en échec", "user", uid, "err", errText(err))
			if errors.Is(err, apimeter.ErrCooldown) {
				return
			}
		}
	}
}

// ensureListCoverage lists Garmin activities back to from when the history is shorter.
func (s *Services) ensureListCoverage(ctx context.Context, uid int64, from string, fr *fresh) error {
	cn, err := s.getConn(ctx, uid, pGarmin)
	if err != nil || !cn.connected() || !cn.HistoryFrom.Valid || cn.HistoryFrom.String <= from {
		return err
	}
	if _, _, err := s.syncGarminList(apimeter.WithUser(ctx, uid), uid, from, analysis.DateAdd(cn.HistoryFrom.String, -1)); err != nil {
		return err
	}
	fr.mark(lvLive, s.now())
	_, err = s.db().ExecContext(ctx, `UPDATE provider_connections SET history_from = ? WHERE user_id = ? AND provider = 'garmin'`, from, uid)
	return err
}

// firstSyncIfNeeded runs a bounded synchronous sync for users connected but never synced.
func (s *Services) firstSyncIfNeeded(ctx context.Context, uid int64, fr *fresh) error {
	need := false
	for _, p := range []string{pGarmin, pStrava} {
		if cn, err := s.getConn(ctx, uid, p); err == nil && cn.connected() && !cn.LastSyncAt.Valid {
			need = true
		}
	}
	if !need || s.isRunning(uid) {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, firstSyncBudget)
	defer cancel()
	fr.mark(lvLive, s.now())
	err := s.runSync(apimeter.WithUser(cctx, uid), uid, false)
	if errors.Is(err, errSyncRunning) {
		return nil
	}
	return err
}

// syncStravaList lists Strava activities since the last sync and fuses them with Garmin ones.
func (s *Services) syncStravaList(ctx context.Context, uid int64, cn *conn, today string) (int, error) {
	tok, err := s.stravaToken(ctx, uid)
	if err != nil {
		return 0, err
	}
	after := s.now().AddDate(0, 0, -initialHistory)
	if cn.LastSyncAt.Valid {
		after = time.Unix(cn.LastSyncAt.Int64, 0).Add(-48 * time.Hour)
	}
	n := 0
	for page := 1; page <= 10; page++ {
		acts, err := s.deps.Strava.ListActivities(ctx, tok, after, s.now().Add(time.Hour), page, 100)
		if err != nil {
			return n, providerErr(pStrava, err)
		}
		for i := range acts {
			if acts[i].Manual {
				continue
			}
			// list items are summaries: the detailed activity is fetched with the bundle
			_, created, err := s.upsertStrava(ctx, uid, &acts[i], nil, true)
			if err != nil {
				return n, err
			}
			if created {
				n++
			}
		}
		if len(acts) < 100 {
			break
		}
	}
	return n, nil
}

// Resync drops cached data and non-final bundles of a user, then enqueues a sync (admin).
func (s *Services) Resync(ctx context.Context, uid int64) error {
	if _, err := s.db().ExecContext(ctx, `DELETE FROM activity_blobs WHERE user_id = ? AND activity_id IN (SELECT id FROM activities WHERE user_id = ? AND final = 0)`, uid, uid); err != nil {
		return err
	}
	if _, err := s.db().ExecContext(ctx, `UPDATE activities SET complete = 0, bundle_at = NULL WHERE user_id = ? AND final = 0`, uid); err != nil {
		return err
	}
	if _, err := s.db().ExecContext(ctx, `DELETE FROM wellness_days WHERE user_id = ? AND final = 0`, uid); err != nil {
		return err
	}
	if _, err := s.db().ExecContext(ctx, `UPDATE provider_connections SET last_sync_at = NULL WHERE user_id = ? AND history_from IS NOT NULL`, uid); err != nil {
		return err
	}
	if err := s.PurgeUserCache(ctx, uid); err != nil {
		return err
	}
	s.startSync(uid)
	return nil
}

// profileHash is part of the analysis cache key.
func profileHash(p athlete.Profile) string { return p.Hash() }
