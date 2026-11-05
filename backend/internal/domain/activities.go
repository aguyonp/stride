package domain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"stride/internal/analysis"
	"stride/internal/athlete"
	"stride/internal/garmin"
	"stride/internal/provider/strava"
	"stride/internal/view"
)

const matchWindowS = 120

// activityRow is the normalized summary of one activity (Garmin and/or Strava).
type activityRow struct {
	ID         int64
	GarminID   *int64
	StravaID   *int64
	StartUTC   int64
	StartLocal string
	LocalDate  string
	Type       string
	Sport      string
	Name       string
	DistanceM  *float64
	DurationS  *float64
	MovingS    *float64
	ElapsedS   *float64
	AvgSpeed   *float64
	GapSpeed   *float64
	AvgHR      *float64
	MaxHR      *float64
	AvgPower   *float64
	AvgCadence *float64
	AvgGCT     *float64
	ElevGain   *float64
	ElevLoss   *float64
	Load       *float64
	StravaGear *string
	Complete   bool
	Final      bool
	BundleAt   int64
	UpdatedAt  int64
}

const actCols = `id, garmin_id, strava_id, start_utc, start_local, local_date, type, sport, name, distance_m, duration_s, moving_s,
	elapsed_s, avg_speed, gap_speed, avg_hr, max_hr, avg_power, avg_cadence, avg_gct, elev_gain, elev_loss, training_load,
	strava_gear, complete, final, COALESCE(bundle_at, 0), updated_at`

func nullF(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

func scanActivity(sc interface{ Scan(...any) error }) (*activityRow, error) {
	a := &activityRow{}
	var gid, sid sql.NullInt64
	var gear sql.NullString
	var f [14]sql.NullFloat64
	var complete, final int
	err := sc.Scan(&a.ID, &gid, &sid, &a.StartUTC, &a.StartLocal, &a.LocalDate, &a.Type, &a.Sport, &a.Name,
		&f[0], &f[1], &f[2], &f[3], &f[4], &f[5], &f[6], &f[7], &f[8], &f[9], &f[10], &f[11], &f[12], &f[13],
		&gear, &complete, &final, &a.BundleAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if gid.Valid {
		a.GarminID = &gid.Int64
	}
	if sid.Valid {
		a.StravaID = &sid.Int64
	}
	if gear.Valid {
		a.StravaGear = &gear.String
	}
	a.DistanceM, a.DurationS, a.MovingS, a.ElapsedS = nullF(f[0]), nullF(f[1]), nullF(f[2]), nullF(f[3])
	a.AvgSpeed, a.GapSpeed, a.AvgHR, a.MaxHR = nullF(f[4]), nullF(f[5]), nullF(f[6]), nullF(f[7])
	a.AvgPower, a.AvgCadence, a.AvgGCT, a.ElevGain = nullF(f[8]), nullF(f[9]), nullF(f[10]), nullF(f[11])
	a.ElevLoss, a.Load = nullF(f[12]), nullF(f[13])
	a.Complete, a.Final = complete != 0, final != 0
	return a, nil
}

func (s *Services) queryActivities(ctx context.Context, q string, args ...any) ([]*activityRow, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT `+actCols+` FROM activities `+q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*activityRow
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// activity returns the user's activity or ErrNotFound (ids of other users are not found).
func (s *Services) activity(ctx context.Context, uid, id int64) (*activityRow, error) {
	a, err := scanActivity(s.db().QueryRowContext(ctx, `SELECT `+actCols+` FROM activities WHERE user_id = ? AND id = ?`, uid, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// activitiesBetween returns activities whose local date is in [from, to], chronological.
func (s *Services) activitiesBetween(ctx context.Context, uid int64, from, to string) ([]*activityRow, error) {
	return s.queryActivities(ctx, `WHERE user_id = ? AND local_date >= ? AND local_date <= ? ORDER BY start_local, id`, uid, from, to)
}

func (a *activityRow) isRunning() bool { return analysis.IsRunningType(a.Type) }

func (a *activityRow) endUTC() int64 {
	d := analysis.PF(a.ElapsedS)
	if d != d {
		d = analysis.PF(a.DurationS)
	}
	if d != d {
		d = 0
	}
	return a.StartUTC + int64(d)
}

func (a *activityRow) paceSecPerKm() *float64 {
	if a.AvgSpeed != nil {
		if p := analysis.Num(analysis.PaceFromSpeed(*a.AvgSpeed), 0); p != nil {
			return p
		}
	}
	if a.DistanceM != nil && a.DurationS != nil && *a.DistanceM > 0 {
		return analysis.Num(*a.DurationS / *a.DistanceM * 1000, 0)
	}
	return nil
}

func (a *activityRow) sources() []string {
	out := []string{}
	if a.GarminID != nil {
		out = append(out, pGarmin)
	}
	if a.StravaID != nil {
		out = append(out, pStrava)
	}
	return out
}

// summary converts a row; notes and plan status are filled by the caller when known.
func (a *activityRow) summary() view.ActivitySummary {
	sm := view.ActivitySummary{
		ID: a.ID, GarminID: a.GarminID, StravaID: a.StravaID, Sources: a.sources(), Name: a.Name, Type: a.Type,
		TypeLabel: analysis.TypeLabel(a.Type), Sport: a.Sport, IsRunning: a.isRunning(), StartLocal: a.StartLocal, Date: a.LocalDate,
		DistanceM: analysis.PR(a.DistanceM, 1), DurationS: analysis.PR(a.DurationS, 1), AvgPaceSecPerKm: a.paceSecPerKm(),
		AvgHR: analysis.PR(a.AvgHR, 0), AvgPower: analysis.PR(a.AvgPower, 0), AvgCadenceSpm: analysis.PR(a.AvgCadence, 1),
		ElevGainM: analysis.PR(a.ElevGain, 1), ElevLossM: analysis.PR(a.ElevLoss, 1), TrainingLoad: analysis.PR(a.Load, 1),
	}
	if a.isRunning() && a.GapSpeed != nil {
		sm.GapSecPerKm = analysis.Num(analysis.PaceFromSpeed(*a.GapSpeed), 0)
	}
	return sm
}

func withNotes(sm view.ActivitySummary, n *athlete.ActivityNotes) view.ActivitySummary {
	if n != nil {
		sm.PainDuring = n.PainDuring
		sm.HasNotes = !n.IsEmpty()
	}
	return sm
}

// ---------- upserts ----------

type summaryCols struct {
	startUTC                                                 int64
	startLocal, date, typ, sport, name                       string
	dist, dur, moving, elapsed, speed, gap, hr, maxHR, power *float64
	cadence, gct, gain, loss, load                           *float64
}

func garminCols(it *garmin.ActivityListItem) (summaryCols, bool) {
	start, ok := it.StartGMT()
	if !ok || it.StartTimeLocal == "" {
		return summaryCols{}, false
	}
	tk := it.ActivityType.TypeKey
	name := ""
	if it.ActivityName != nil {
		name = *it.ActivityName
	}
	return summaryCols{startUTC: start.Unix(), startLocal: garmin.NormalizeLocal(it.StartTimeLocal), date: it.LocalDate(),
		typ: tk, sport: analysis.Sport(tk), name: name, dist: it.Distance, dur: it.Duration, moving: it.MovingDuration,
		elapsed: it.ElapsedDuration, speed: it.AverageSpeed, gap: it.AvgGradeAdjustedSpeed, hr: it.AverageHR, maxHR: it.MaxHR,
		power: it.AvgPower, cadence: it.AvgCadenceSpm, gct: it.AvgGCT, gain: it.ElevationGain, loss: it.ElevationLoss, load: it.TrainingLoad}, true
}

func stravaCols(a *strava.Activity) summaryCols {
	tk := analysis.StravaTypeKey(a.SportType)
	local := strings.TrimSuffix(a.StartDateLocal, "Z")
	if len(local) > 19 {
		local = local[:19]
	}
	if local == "" {
		local = a.StartDate.UTC().Format("2006-01-02T15:04:05")
	}
	c := summaryCols{startUTC: a.StartDate.Unix(), startLocal: local, date: local[:10], typ: tk, sport: analysis.Sport(tk), name: a.Name,
		dist: f64(a.DistanceM), dur: f64(a.MovingTimeS), moving: f64(a.MovingTimeS), elapsed: f64(a.ElapsedTimeS),
		speed: f64(a.AverageSpeedMS), hr: a.AverageHeartrate, maxHR: a.MaxHeartrate, power: a.AverageWatts, gain: f64(a.ElevationGainM)}
	if a.AverageCadenceRaw != nil {
		v := *a.AverageCadenceRaw
		if analysis.IsRunningType(tk) {
			v *= 2
		}
		c.cadence = &v
	}
	return c
}

func (s *Services) putBlobTx(ctx context.Context, tx *sql.Tx, uid, aid int64, kind string, data []byte) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO activity_blobs (activity_id, user_id, kind, data, fetched_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (activity_id, kind) DO UPDATE SET data = excluded.data, fetched_at = excluded.fetched_at`, aid, uid, kind, gz(data), s.now().Unix())
	return err
}

func (s *Services) putBlob(ctx context.Context, uid, aid int64, kind string, data []byte) error {
	return s.deps.Store.Tx(ctx, func(tx *sql.Tx) error { return s.putBlobTx(ctx, tx, uid, aid, kind, data) })
}

// blob returns the decompressed payload; ok=false when absent. An empty payload marks data the
// provider does not have (permanent).
func (s *Services) blob(ctx context.Context, uid, aid int64, kind string) ([]byte, int64, bool, error) {
	var data []byte
	var at int64
	err := s.db().QueryRowContext(ctx, `SELECT data, fetched_at FROM activity_blobs WHERE user_id = ? AND activity_id = ? AND kind = ?`, uid, aid, kind).Scan(&data, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	b, err := gunzip(data)
	return b, at, err == nil, err
}

func (s *Services) blobKinds(ctx context.Context, uid, aid int64) (map[string]int64, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT kind, fetched_at FROM activity_blobs WHERE user_id = ? AND activity_id = ?`, uid, aid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var k string
		var at int64
		if err := rows.Scan(&k, &at); err != nil {
			return nil, err
		}
		out[k] = at
	}
	return out, rows.Err()
}

// upsertGarmin stores a list item: matched by garmin id, else fused into a Strava-only activity
// started within 120 s with a compatible sport, else inserted.
func (s *Services) upsertGarmin(ctx context.Context, uid int64, raw json.RawMessage) (int64, bool, error) {
	it, err := garmin.Decode[garmin.ActivityListItem](raw)
	if err != nil || it.ActivityID == 0 {
		return 0, false, err
	}
	c, ok := garminCols(it)
	if !ok {
		return 0, false, nil
	}
	var id int64
	created := false
	err = s.deps.Store.Tx(ctx, func(tx *sql.Tx) error {
		now := s.now().Unix()
		err := tx.QueryRowContext(ctx, `SELECT id FROM activities WHERE user_id = ? AND garmin_id = ?`, uid, it.ActivityID).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			id, err = s.matchCandidate(ctx, tx, uid, "garmin_id", c)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if id == 0 {
			res, err := tx.ExecContext(ctx, `INSERT INTO activities (user_id, garmin_id, start_utc, start_local, local_date, type, sport, name, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uid, it.ActivityID, c.startUTC, c.startLocal, c.date, c.typ, c.sport, c.name, now, now)
			if err != nil {
				return err
			}
			id, _ = res.LastInsertId()
			created = true
		}
		_, err = tx.ExecContext(ctx, `UPDATE activities SET garmin_id = ?, start_utc = ?, start_local = ?, local_date = ?, type = ?, sport = ?, name = ?,
			distance_m = ?, duration_s = ?, moving_s = ?, elapsed_s = ?, avg_speed = ?, gap_speed = ?, avg_hr = ?, max_hr = ?, avg_power = ?,
			avg_cadence = ?, avg_gct = ?, elev_gain = ?, elev_loss = ?, training_load = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
			it.ActivityID, c.startUTC, c.startLocal, c.date, c.typ, c.sport, c.name, c.dist, c.dur, c.moving, c.elapsed, c.speed, c.gap,
			c.hr, c.maxHR, c.power, c.cadence, c.gct, c.gain, c.loss, c.load, now, id, uid)
		if err != nil {
			return err
		}
		return s.putBlobTx(ctx, tx, uid, id, "garmin_list", raw)
	})
	return id, created, err
}

// matchCandidate finds an activity of the other provider (column idCol still NULL) within 120 s.
func (s *Services) matchCandidate(ctx context.Context, tx *sql.Tx, uid int64, idCol string, c summaryCols) (int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, sport, ABS(start_utc - ?) AS d FROM activities WHERE user_id = ? AND `+idCol+` IS NULL
		AND start_utc BETWEEN ? AND ? ORDER BY d`, c.startUTC, uid, c.startUTC-matchWindowS, c.startUTC+matchWindowS)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, d int64
		var sport string
		if err := rows.Scan(&id, &sport, &d); err != nil {
			return 0, err
		}
		if analysis.CompatibleSports(sport, c.sport) {
			return id, nil
		}
	}
	return 0, rows.Err()
}

// upsertStrava stores a Strava activity. When insertAlone is false and no Garmin activity matches,
// nothing is written and id 0 is returned (the caller retries later).
func (s *Services) upsertStrava(ctx context.Context, uid int64, a *strava.Activity, raw []byte, insertAlone bool) (int64, bool, error) {
	c := stravaCols(a)
	var id int64
	created := false
	var gear any
	if a.GearID != "" {
		gear = a.GearID
	}
	err := s.deps.Store.Tx(ctx, func(tx *sql.Tx) error {
		now := s.now().Unix()
		var hasGarmin sql.NullInt64
		err := tx.QueryRowContext(ctx, `SELECT id, garmin_id FROM activities WHERE user_id = ? AND strava_id = ?`, uid, a.ID).Scan(&id, &hasGarmin)
		if errors.Is(err, sql.ErrNoRows) {
			if id, err = s.matchCandidate(ctx, tx, uid, "strava_id", c); err != nil {
				return err
			}
			if id != 0 {
				hasGarmin.Valid = true
			}
		} else if err != nil {
			return err
		}
		if id == 0 {
			if !insertAlone {
				return nil
			}
			res, err := tx.ExecContext(ctx, `INSERT INTO activities (user_id, strava_id, start_utc, start_local, local_date, type, sport, name, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uid, a.ID, c.startUTC, c.startLocal, c.date, c.typ, c.sport, c.name, now, now)
			if err != nil {
				return err
			}
			id, _ = res.LastInsertId()
			created = true
		}
		if hasGarmin.Valid {
			_, err = tx.ExecContext(ctx, `UPDATE activities SET final = CASE WHEN strava_id IS NULL THEN 0 ELSE final END,
				complete = CASE WHEN strava_id IS NULL THEN 0 ELSE complete END, strava_id = ?, strava_gear = ?, updated_at = ?
				WHERE id = ? AND user_id = ?`, a.ID, gear, now, id, uid)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE activities SET strava_id = ?, strava_gear = ?, start_utc = ?, start_local = ?, local_date = ?, type = ?, sport = ?,
				name = ?, distance_m = ?, duration_s = ?, moving_s = ?, elapsed_s = ?, avg_speed = ?, avg_hr = ?, max_hr = ?, avg_power = ?, avg_cadence = ?,
				elev_gain = ?, updated_at = ? WHERE id = ? AND user_id = ?`, a.ID, gear, c.startUTC, c.startLocal, c.date, c.typ, c.sport, c.name,
				c.dist, c.dur, c.moving, c.elapsed, c.speed, c.hr, c.maxHR, c.power, c.cadence, c.gain, now, id, uid)
		}
		if err != nil {
			return err
		}
		if raw != nil {
			return s.putBlobTx(ctx, tx, uid, id, "strava_detail", raw)
		}
		return nil
	})
	return id, created, err
}

// ListActivities serves GET /api/activities from SQLite.
func (s *Services) ListActivities(ctx context.Context, uid int64, limit int, before, from, to, sport string) (*view.ActivityList, error) {
	if limit <= 0 {
		limit = 30
	}
	q := `WHERE user_id = ?`
	args := []any{uid}
	if before != "" {
		q += ` AND start_local < ?`
		args = append(args, before)
	}
	if from != "" {
		q += ` AND local_date >= ?`
		args = append(args, from)
	}
	if to != "" {
		q += ` AND local_date <= ?`
		args = append(args, to)
	}
	if sport != "" {
		q += ` AND sport = ?`
		args = append(args, sport)
	}
	q += ` ORDER BY start_local DESC, id DESC LIMIT ?`
	args = append(args, limit+1)
	fr := &fresh{}
	if err := s.firstSyncIfNeeded(ctx, uid, fr); err != nil {
		s.log.Warn("⚠️ Première synchronisation incomplète", "user", uid, "err", errText(err))
	}
	rows, err := s.queryActivities(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out := &view.ActivityList{Items: []view.ActivitySummary{}}
	if len(rows) > limit {
		out.HasMore = true
		rows = rows[:limit]
	}
	items, err := s.summaries(ctx, uid, rows)
	if err != nil {
		return nil, err
	}
	out.Items = items
	if last, err := s.lastSync(ctx, uid); err == nil {
		fr.mark(lvDB, last)
	}
	out.Freshness = fr.view(s.now())
	return out, nil
}

// summaries converts rows with notes and plan status.
func (s *Services) summaries(ctx context.Context, uid int64, rows []*activityRow) ([]view.ActivitySummary, error) {
	out := make([]view.ActivitySummary, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, len(rows))
	minD, maxD := rows[0].LocalDate, rows[0].LocalDate
	for i, r := range rows {
		ids[i] = r.ID
		if r.LocalDate < minD {
			minD = r.LocalDate
		}
		if r.LocalDate > maxD {
			maxD = r.LocalDate
		}
	}
	notes, err := s.repo().Notes(ctx, uid, ids)
	if err != nil {
		return nil, err
	}
	statuses, err := s.planStatuses(ctx, uid, minD, maxD)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		var n *athlete.ActivityNotes
		if v, ok := notes[r.ID]; ok {
			n = &v
		}
		sm := withNotes(r.summary(), n)
		if st, ok := statuses[r.ID]; ok {
			st := st
			sm.PlanStatus = &st
		}
		out = append(out, sm)
	}
	return out, nil
}

func (s *Services) lastSync(ctx context.Context, uid int64) (time.Time, error) {
	var ts sql.NullInt64
	err := s.db().QueryRowContext(ctx, `SELECT MAX(last_sync_at) FROM provider_connections WHERE user_id = ?`, uid).Scan(&ts)
	if err != nil || !ts.Valid {
		return time.Time{}, err
	}
	return time.Unix(ts.Int64, 0), nil
}
