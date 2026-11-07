package domain

import (
	"context"
	"database/sql"
	"strings"
	"unicode"

	"stride/internal/analysis"
	"stride/internal/apimeter"
	"stride/internal/athlete"
)

const dedupMarker = "v3_dedup_done"

// sameActivity is the §11.1 rule between two stored rows; rows of the same provider never merge.
func sameActivity(a, b *activityRow) bool {
	if (a.GarminID != nil && b.GarminID != nil) || (a.StravaID != nil && b.StravaID != nil) {
		return false
	}
	d := a.StartUTC - b.StartUTC
	if d < 0 {
		d = -d
	}
	return d <= matchWindowS && analysis.CompatibleSports(a.Sport, b.Sport) && distanceCompatible(a.DistanceM, b.DistanceM)
}

// DedupOnce merges duplicate rows left by V2 (one-shot, recorded in app_settings).
func (s *Services) DedupOnce(ctx context.Context) error {
	var done int
	if err := s.db().QueryRowContext(ctx, `SELECT COUNT(*) FROM app_settings WHERE key = ?`, dedupMarker).Scan(&done); err != nil || done > 0 {
		return err
	}
	var uids []int64
	rows, err := s.db().QueryContext(ctx, `SELECT DISTINCT user_id FROM activities`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		uids = append(uids, id)
	}
	rows.Close()
	total := 0
	for _, uid := range uids {
		n, err := s.dedupUser(ctx, uid)
		if err != nil {
			return err
		}
		total += n
	}
	if total > 0 {
		s.log.Info("🧹 Doublons Garmin/Strava fusionnés", "fusions", total)
	}
	return s.deps.Store.SetSetting(ctx, dedupMarker, "1", s.now())
}

func (s *Services) dedupUser(ctx context.Context, uid int64) (int, error) {
	rows, err := s.queryActivities(ctx, `WHERE user_id = ? ORDER BY start_utc, id`, uid)
	if err != nil {
		return 0, err
	}
	notes, err := s.repo().Notes(ctx, uid, idsOf(rows))
	if err != nil {
		return 0, err
	}
	merged := 0
	gone := map[int64]bool{}
	for i, a := range rows {
		if gone[a.ID] {
			continue
		}
		for j := i + 1; j < len(rows) && rows[j].StartUTC-a.StartUTC <= matchWindowS; j++ {
			b := rows[j]
			if gone[b.ID] || !sameActivity(a, b) {
				continue
			}
			keep, drop := a, b
			an, aok := notes[a.ID]
			bn, bok := notes[b.ID]
			aNotes, bNotes := aok && !an.IsEmpty(), bok && !bn.IsEmpty()
			if (bNotes && !aNotes) || (aNotes == bNotes && b.GarminID != nil && a.GarminID == nil) {
				keep, drop = b, a
			}
			if err := s.deps.Store.Tx(ctx, func(tx *sql.Tx) error { return s.mergeRows(ctx, tx, uid, keep, drop) }); err != nil {
				return merged, err
			}
			merged++
			gone[drop.ID] = true
			fresh, err := s.activity(ctx, uid, keep.ID)
			if err != nil {
				return merged, err
			}
			*keep = *fresh
			if drop == a {
				break
			}
		}
	}
	return merged, nil
}

func idsOf(rows []*activityRow) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids
}

// mergeRows folds drop into keep: blobs and notes move (keep wins on conflicts), ids join and the
// field precedence of §11.1 applies (Garmin summary, Strava title and gear).
func (s *Services) mergeRows(ctx context.Context, tx *sql.Tx, uid int64, keep, drop *activityRow) error {
	if _, err := tx.ExecContext(ctx, `UPDATE activity_blobs SET activity_id = ? WHERE activity_id = ? AND user_id = ?
		AND kind NOT IN (SELECT kind FROM activity_blobs WHERE activity_id = ?)`, keep.ID, drop.ID, uid, keep.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE activity_notes AS k SET pain_during = COALESCE(k.pain_during, d.pain_during),
		pain_cold = COALESCE(k.pain_cold, d.pain_cold), pain_next_morning = COALESCE(k.pain_next_morning, d.pain_next_morning),
		rpe = COALESCE(k.rpe, d.rpe), shoe_auto = CASE WHEN k.shoe_id IS NULL THEN d.shoe_auto ELSE k.shoe_auto END,
		shoe_id = COALESCE(k.shoe_id, d.shoe_id), tags = CASE WHEN k.tags = '[]' THEN d.tags ELSE k.tags END,
		note = CASE WHEN k.note = '' THEN d.note ELSE k.note END
		FROM activity_notes AS d WHERE k.activity_id = ? AND d.activity_id = ? AND k.user_id = ?`, keep.ID, drop.ID, uid); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE activity_notes SET activity_id = ? WHERE activity_id = ? AND user_id = ?
		AND NOT EXISTS (SELECT 1 FROM activity_notes WHERE activity_id = ?)`, keep.ID, drop.ID, uid, keep.ID); err != nil {
		return err
	}
	var notified sql.NullInt64
	_ = tx.QueryRowContext(ctx, `SELECT MAX(COALESCE(notified_at, 0)) FROM activities WHERE id IN (?, ?)`, keep.ID, drop.ID).Scan(&notified)
	if _, err := tx.ExecContext(ctx, `DELETE FROM activity_metrics WHERE activity_id IN (?, ?)`, keep.ID, drop.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM activities WHERE id = ? AND user_id = ?`, drop.ID, uid); err != nil {
		return err
	}
	now := s.now().Unix()
	if drop.GarminID != nil {
		name := drop.Name
		if keep.StravaID != nil {
			name = keep.Name
		}
		if _, err := tx.ExecContext(ctx, `UPDATE activities SET garmin_id = ?, start_utc = ?, start_local = ?, local_date = ?, type = ?, sport = ?, name = ?,
			distance_m = ?, duration_s = ?, moving_s = ?, elapsed_s = ?, avg_speed = ?, gap_speed = ?, avg_hr = ?, max_hr = ?, avg_power = ?,
			avg_cadence = ?, avg_gct = ?, elev_gain = ?, elev_loss = ?, training_load = ? WHERE id = ? AND user_id = ?`,
			drop.GarminID, drop.StartUTC, drop.StartLocal, drop.LocalDate, drop.Type, drop.Sport, name, drop.DistanceM, drop.DurationS,
			drop.MovingS, drop.ElapsedS, drop.AvgSpeed, drop.GapSpeed, drop.AvgHR, drop.MaxHR, drop.AvgPower, drop.AvgCadence, drop.AvgGCT,
			drop.ElevGain, drop.ElevLoss, drop.Load, keep.ID, uid); err != nil {
			return err
		}
	}
	if drop.StravaID != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE activities SET strava_id = ?, strava_gear = ?, name = ? WHERE id = ? AND user_id = ?`,
			drop.StravaID, drop.StravaGear, drop.Name, keep.ID, uid); err != nil {
			return err
		}
	}
	var notifiedAt any
	if notified.Valid && notified.Int64 > 0 {
		notifiedAt = notified.Int64
	}
	_, err := tx.ExecContext(ctx, `UPDATE activities SET import_format = COALESCE(import_format, ?), notified_at = ?, complete = 0, final = 0,
		updated_at = ? WHERE id = ? AND user_id = ?`, drop.ImportFormat, notifiedAt, now, keep.ID, uid)
	return err
}

// ---------- shoes ----------

func normShoe(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		for _, r := range strings.ToLower(p) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// sameShoe applies the dedup rule after the Strava gear id: normalized name, or brand+model.
func sameShoe(sh athlete.Shoe, name, brand, model string) bool {
	if n := normShoe(name); n != "" && (n == normShoe(sh.Name) || n == normShoe(sh.Brand, sh.Model)) {
		return true
	}
	bm := normShoe(brand, model)
	return bm != "" && normShoe(brand) != "" && normShoe(model) != "" && (bm == normShoe(sh.Brand, sh.Model) || bm == normShoe(sh.Name))
}

// syncStravaGear refreshes the Strava shoes (real mileage, retired/deleted pairs, see shoegear.go)
// then assigns them to activities without a chosen shoe. Without profile:read_all it stays silent:
// the UI shows the reconnect hint from the integration's missingScopes.
func (s *Services) syncStravaGear(ctx context.Context, uid int64) {
	if cn, err := s.getConn(ctx, uid, pStrava); err != nil || (cn.Scope.Valid && !strings.Contains(cn.Scope.String, "profile:read_all")) {
		return
	}
	if _, err := s.refreshStravaShoes(apimeter.WithUser(ctx, uid), uid, false); err != nil {
		s.log.Warn("⚠️ Import des chaussures Strava en échec", "user", uid, "err", errText(err))
	}
}

// autoAssignShoes sets the shoe matching the Strava gear on activities whose shoe was not chosen
// by the athlete (notes.shoeAuto = true).
func (s *Services) autoAssignShoes(ctx context.Context, uid int64) error {
	_, err := s.db().ExecContext(ctx, `INSERT INTO activity_notes (activity_id, user_id, shoe_id, shoe_auto, updated_at)
		SELECT a.id, a.user_id, sh.id, 1, ? FROM activities a JOIN shoes sh ON sh.user_id = a.user_id AND sh.strava_gear_id = a.strava_gear
		WHERE a.user_id = ? AND a.strava_gear IS NOT NULL
		ON CONFLICT (activity_id) DO UPDATE SET shoe_id = excluded.shoe_id, shoe_auto = 1, updated_at = excluded.updated_at
		WHERE (activity_notes.shoe_id IS NULL OR activity_notes.shoe_auto = 1) AND activity_notes.shoe_id IS NOT excluded.shoe_id`,
		s.now().Unix(), uid)
	return err
}

// MergeShoe moves the notes of shoe id onto intoID, keeps the Strava link and deletes the source.
func (s *Services) MergeShoe(ctx context.Context, uid, id, intoID int64) (athlete.Shoe, error) {
	r := s.repo()
	if id == intoID {
		return athlete.Shoe{}, badRequest("Impossible de fusionner une chaussure avec elle-même.")
	}
	src, err := r.Shoe(ctx, uid, id)
	if err != nil {
		return athlete.Shoe{}, repoErr(err)
	}
	if _, err := r.Shoe(ctx, uid, intoID); err != nil {
		return athlete.Shoe{}, repoErr(err)
	}
	err = s.deps.Store.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE activity_notes SET shoe_id = ? WHERE shoe_id = ? AND user_id = ?`, intoID, id, uid); err != nil {
			return err
		}
		var dist sql.NullFloat64
		var primary int
		var state sql.NullString
		var synced sql.NullInt64
		_ = tx.QueryRowContext(ctx, `SELECT strava_distance_m, strava_primary, strava_state, strava_synced_at FROM shoes WHERE id = ?`, id).Scan(&dist, &primary, &state, &synced)
		if _, err := tx.ExecContext(ctx, `DELETE FROM shoes WHERE id = ? AND user_id = ?`, id, uid); err != nil {
			return err
		}
		if src.StravaGearID != nil { // the target takes the Strava link (and its gear state) when it has none
			_, err := tx.ExecContext(ctx, `UPDATE shoes SET strava_primary = CASE WHEN strava_gear_id IS NULL THEN ? ELSE strava_primary END,
				strava_state = CASE WHEN strava_gear_id IS NULL THEN ? ELSE strava_state END,
				strava_synced_at = CASE WHEN strava_gear_id IS NULL THEN ? ELSE strava_synced_at END,
				strava_gear_id = COALESCE(strava_gear_id, ?), strava_distance_m = COALESCE(strava_distance_m, ?),
				updated_at = ? WHERE id = ? AND user_id = ?`, primary, state, synced, *src.StravaGearID, dist, s.now().Unix(), intoID, uid)
			return err
		}
		return nil
	})
	if err != nil {
		return athlete.Shoe{}, err
	}
	sh, err := r.Shoe(ctx, uid, intoID)
	return sh, repoErr(err)
}
