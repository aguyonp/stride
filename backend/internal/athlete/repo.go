package athlete

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"stride/internal/workout"
)

var ErrNotFound = errors.New("not found")

// DB is satisfied by *sql.DB and *sql.Tx.
type DB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Repo runs every query with the user id given by the caller (taken from the session).
type Repo struct {
	DB  DB
	Now func() time.Time
}

func (r Repo) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r Repo) ts() int64 { return r.now().Unix() }

func rfc(ts int64) string { return time.Unix(ts, 0).UTC().Format(time.RFC3339) }

func nf(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

func ns(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- profile ----------

func (r Repo) Profile(ctx context.Context, uid int64, defaultTZ string) (Profile, error) {
	var area, label, tz, ov, pts string
	var side, lang sql.NullString
	err := r.DB.QueryRowContext(ctx, `SELECT focus_area, focus_label, injured_side, timezone, overrides, pace_targets, language FROM athlete_profiles WHERE user_id = ?`, uid).
		Scan(&area, &label, &side, &tz, &ov, &pts, &lang)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultProfile(defaultTZ), nil
	}
	if err != nil {
		return Profile{}, err
	}
	preset := PresetFor(area)
	var overrides map[string]float64
	_ = json.Unmarshal([]byte(ov), &overrides)
	p := Profile{FocusArea: preset.FocusArea, FocusLabel: label, InjuredSide: ns(side), Timezone: tz,
		Thresholds: applyOverrides(preset.Thresholds, overrides), PaceTargets: []PaceTarget{}, Language: ns(lang)}
	_ = json.Unmarshal([]byte(pts), &p.PaceTargets)
	if p.FocusLabel == "" {
		p.FocusLabel = preset.Label
	}
	if p.PaceTargets == nil {
		p.PaceTargets = []PaceTarget{}
	}
	return p, nil
}

func (r Repo) PutProfile(ctx context.Context, uid int64, p Profile) error {
	if err := p.Normalize(); err != nil {
		return err
	}
	ov, _ := json.Marshal(overrides(p.Thresholds, PresetFor(p.FocusArea).Thresholds))
	pts, _ := json.Marshal(p.PaceTargets)
	_, err := r.DB.ExecContext(ctx, `INSERT INTO athlete_profiles (user_id, focus_area, focus_label, injured_side, timezone, overrides, pace_targets, language, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET focus_area = excluded.focus_area, focus_label = excluded.focus_label,
		injured_side = excluded.injured_side, timezone = excluded.timezone, overrides = excluded.overrides,
		pace_targets = excluded.pace_targets, language = excluded.language, updated_at = excluded.updated_at`,
		uid, p.FocusArea, p.FocusLabel, p.InjuredSide, p.Timezone, string(ov), string(pts), p.Language, r.ts())
	return err
}

// ---------- goals ----------

const goalCols = `id, title, kind, target_value, target_unit, target_date, status, note`

func scanGoal(sc interface{ Scan(...any) error }) (Goal, error) {
	var g Goal
	var tv sql.NullFloat64
	var tu, td sql.NullString
	err := sc.Scan(&g.ID, &g.Title, &g.Kind, &tv, &tu, &td, &g.Status, &g.Note)
	g.TargetValue, g.TargetUnit, g.TargetDate = nf(tv), ns(tu), ns(td)
	return g, err
}

func (r Repo) Goals(ctx context.Context, uid int64, activeOnly bool) ([]Goal, error) {
	q := `SELECT ` + goalCols + ` FROM goals WHERE user_id = ?`
	if activeOnly {
		q += ` AND status = 'active'`
	}
	q += ` ORDER BY CASE kind WHEN 'macro' THEN 0 WHEN 'race' THEN 1 ELSE 2 END, COALESCE(target_date, '9999'), id`
	rows, err := r.DB.QueryContext(ctx, q, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Goal{}
	for rows.Next() {
		g, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r Repo) Goal(ctx context.Context, uid, id int64) (Goal, error) {
	g, err := scanGoal(r.DB.QueryRowContext(ctx, `SELECT `+goalCols+` FROM goals WHERE user_id = ? AND id = ?`, uid, id))
	if errors.Is(err, sql.ErrNoRows) {
		return g, ErrNotFound
	}
	return g, err
}

func (r Repo) CreateGoal(ctx context.Context, uid int64, g *Goal) error {
	if err := g.Normalize(); err != nil {
		return err
	}
	now := r.ts()
	res, err := r.DB.ExecContext(ctx, `INSERT INTO goals (user_id, title, kind, target_value, target_unit, target_date, status, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uid, g.Title, g.Kind, g.TargetValue, g.TargetUnit, g.TargetDate, g.Status, g.Note, now, now)
	if err != nil {
		return err
	}
	g.ID, err = res.LastInsertId()
	return err
}

func (r Repo) UpdateGoal(ctx context.Context, uid int64, g *Goal) error {
	if err := g.Normalize(); err != nil {
		return err
	}
	return affected(r.DB.ExecContext(ctx, `UPDATE goals SET title = ?, kind = ?, target_value = ?, target_unit = ?, target_date = ?, status = ?, note = ?, updated_at = ?
		WHERE user_id = ? AND id = ?`, g.Title, g.Kind, g.TargetValue, g.TargetUnit, g.TargetDate, g.Status, g.Note, r.ts(), uid, g.ID))
}

func (r Repo) DeleteGoal(ctx context.Context, uid, id int64) error {
	return affected(r.DB.ExecContext(ctx, `DELETE FROM goals WHERE user_id = ? AND id = ?`, uid, id))
}

// ---------- plan ----------

const planCols = `id, date, sport, title, target_duration_s, target_distance_m, target_pace_min, target_pace_max, target_cadence, instructions,
	steps, source, garmin_status, garmin_workout_id, garmin_pushed_at, garmin_error, share_link_id, prescribed_label, prescribed_author, morph_json`

func scanPlanned(sc interface{ Scan(...any) error }) (PlannedSession, error) {
	var p PlannedSession
	var d, dist, pmin, pmax, cad sql.NullFloat64
	var steps string
	var wid, pushed sql.NullInt64
	var gerr, label, author, morph sql.NullString
	var link sql.NullInt64
	err := sc.Scan(&p.ID, &p.Date, &p.Sport, &p.Title, &d, &dist, &pmin, &pmax, &cad, &p.Instructions,
		&steps, &p.Source, &p.Garmin.Status, &wid, &pushed, &gerr, &link, &label, &author, &morph)
	p.PrescribedBy, p.Morph = prescriber(link, label, author), morphOf(morph)
	p.TargetDurationS, p.TargetDistanceM, p.TargetPaceMinSecPerKm, p.TargetPaceMaxSecPerKm, p.TargetCadenceSpm = nf(d), nf(dist), nf(pmin), nf(pmax), nf(cad)
	_ = json.Unmarshal([]byte(steps), &p.Steps)
	if p.Steps == nil {
		p.Steps = []workout.Step{}
	}
	if wid.Valid {
		v := wid.Int64
		p.Garmin.WorkoutID = &v
	}
	if pushed.Valid {
		v := rfc(pushed.Int64)
		p.Garmin.PushedAt = &v
	}
	p.Garmin.Error = ns(gerr)
	return p, err
}

func (r Repo) queryPlanned(ctx context.Context, q string, args ...any) ([]PlannedSession, error) {
	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlannedSession{}
	for rows.Next() {
		p, err := scanPlanned(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Plan returns planned sessions with from <= date <= to (empty bounds are open).
func (r Repo) Plan(ctx context.Context, uid int64, from, to string) ([]PlannedSession, error) {
	if from == "" {
		from = "0000-00-00"
	}
	if to == "" {
		to = "9999-99-99"
	}
	return r.queryPlanned(ctx, `SELECT `+planCols+` FROM planned_sessions WHERE user_id = ? AND date >= ? AND date <= ? ORDER BY date, id`, uid, from, to)
}

// NextPlanned returns up to n sessions strictly after date.
func (r Repo) NextPlanned(ctx context.Context, uid int64, after string, n int) ([]PlannedSession, error) {
	return r.queryPlanned(ctx, `SELECT `+planCols+` FROM planned_sessions WHERE user_id = ? AND date > ? ORDER BY date, id LIMIT ?`, uid, after, n)
}

func (r Repo) Planned(ctx context.Context, uid, id int64) (PlannedSession, error) {
	p, err := scanPlanned(r.DB.QueryRowContext(ctx, `SELECT `+planCols+` FROM planned_sessions WHERE user_id = ? AND id = ?`, uid, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (r Repo) CreatePlanned(ctx context.Context, uid int64, p *PlannedSession) error {
	if err := p.Normalize(); err != nil {
		return err
	}
	now := r.ts()
	link, label, author := prescriberArgs(p.PrescribedBy)
	res, err := r.DB.ExecContext(ctx, `INSERT INTO planned_sessions (user_id, date, sport, title, target_duration_s, target_distance_m, target_pace_min, target_pace_max, target_cadence, instructions, steps, source, share_link_id, prescribed_label, prescribed_author, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uid, p.Date, p.Sport, p.Title, p.TargetDurationS, p.TargetDistanceM,
		p.TargetPaceMinSecPerKm, p.TargetPaceMaxSecPerKm, p.TargetCadenceSpm, p.Instructions, stepsJSON(p.Steps), p.Source, link, label, author, now, now)
	if err != nil {
		return err
	}
	p.ID, err = res.LastInsertId()
	return err
}

func (r Repo) UpdatePlanned(ctx context.Context, uid int64, p *PlannedSession) error {
	if err := p.Normalize(); err != nil {
		return err
	}
	return affected(r.DB.ExecContext(ctx, `UPDATE planned_sessions SET date = ?, sport = ?, title = ?, target_duration_s = ?, target_distance_m = ?,
		target_pace_min = ?, target_pace_max = ?, target_cadence = ?, instructions = ?, steps = ?, source = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		p.Date, p.Sport, p.Title, p.TargetDurationS, p.TargetDistanceM, p.TargetPaceMinSecPerKm, p.TargetPaceMaxSecPerKm,
		p.TargetCadenceSpm, p.Instructions, stepsJSON(p.Steps), p.Source, r.ts(), uid, p.ID))
}

func stepsJSON(steps []workout.Step) string {
	if len(steps) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(steps)
	return string(b)
}

// SetGarminPush records the outcome of a Garmin workout push.
func (r Repo) SetGarminPush(ctx context.Context, uid, id int64, status string, workoutID *int64, errMsg *string) error {
	var pushed any
	if status == "pushed" {
		pushed = r.ts()
	}
	return affected(r.DB.ExecContext(ctx, `UPDATE planned_sessions SET garmin_status = ?, garmin_workout_id = COALESCE(?, garmin_workout_id),
		garmin_pushed_at = COALESCE(?, garmin_pushed_at), garmin_error = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		status, workoutID, pushed, errMsg, r.ts(), uid, id))
}

func (r Repo) DeletePlanned(ctx context.Context, uid, id int64) error {
	return affected(r.DB.ExecContext(ctx, `DELETE FROM planned_sessions WHERE user_id = ? AND id = ?`, uid, id))
}

// ---------- shoes ----------

// A shoe's analyzed distance sums the activities whose note points to it, or (without a note shoe)
// whose Strava gear matches it. Its total is Strava's gear distance when the pair is linked and synced.
const shoeSelect = `SELECT s.id, s.name, s.brand, s.model, s.drop_mm, s.type, s.strava_gear_id, s.retired, s.note, s.strava_distance_m,
	s.strava_primary, s.strava_state, s.strava_synced_at,
	COALESCE((SELECT SUM(a.distance_m) FROM activities a LEFT JOIN activity_notes n ON n.activity_id = a.id
		WHERE a.user_id = s.user_id AND (n.shoe_id = s.id OR (n.shoe_id IS NULL AND s.strava_gear_id IS NOT NULL AND a.strava_gear = s.strava_gear_id))), 0)
	FROM shoes s`

func scanShoe(sc interface{ Scan(...any) error }) (Shoe, error) {
	var s Shoe
	var drop sql.NullFloat64
	var gear sql.NullString
	var retired int
	var distM float64
	var stravaM sql.NullFloat64
	var primary int
	var state sql.NullString
	var synced sql.NullInt64
	err := sc.Scan(&s.ID, &s.Name, &s.Brand, &s.Model, &drop, &s.Type, &gear, &retired, &s.Note, &stravaM, &primary, &state, &synced, &distM)
	s.DropMm, s.StravaGearID, s.Retired = nf(drop), ns(gear), retired != 0
	s.AnalyzedKm = float64(int64(distM/100+0.5)) / 10
	s.DistanceKm, s.DistanceSource = s.AnalyzedKm, "stride"
	if s.StravaGearID != nil { // an unlinked pair never shows a stale Strava figure
		s.StravaPrimary, s.StravaState = primary != 0, ns(state)
		if synced.Valid {
			at := rfc(synced.Int64)
			s.StravaSyncedAt = &at
		}
		if stravaM.Valid {
			km := float64(int64(stravaM.Float64/100+0.5)) / 10
			s.StravaDistanceKm = &km
			s.DistanceKm, s.DistanceSource = km, "strava"
		}
	}
	return s, err
}

func (r Repo) Shoes(ctx context.Context, uid int64) ([]Shoe, error) {
	rows, err := r.DB.QueryContext(ctx, shoeSelect+` WHERE s.user_id = ? ORDER BY s.retired, s.strava_primary DESC, s.name COLLATE NOCASE, s.id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Shoe{}
	for rows.Next() {
		s, err := scanShoe(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r Repo) Shoe(ctx context.Context, uid, id int64) (Shoe, error) {
	s, err := scanShoe(r.DB.QueryRowContext(ctx, shoeSelect+` WHERE s.user_id = ? AND s.id = ?`, uid, id))
	if errors.Is(err, sql.ErrNoRows) {
		return s, ErrNotFound
	}
	return s, err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (r Repo) CreateShoe(ctx context.Context, uid int64, s *Shoe) error {
	if err := s.Normalize(); err != nil {
		return err
	}
	now := r.ts()
	res, err := r.DB.ExecContext(ctx, `INSERT INTO shoes (user_id, name, brand, model, drop_mm, type, strava_gear_id, retired, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uid, s.Name, s.Brand, s.Model, s.DropMm, s.Type, s.StravaGearID, boolInt(s.Retired), s.Note, now, now)
	if err != nil {
		return conflict(err)
	}
	s.ID, err = res.LastInsertId()
	return err
}

func (r Repo) UpdateShoe(ctx context.Context, uid int64, s *Shoe) error {
	if err := s.Normalize(); err != nil {
		return err
	}
	return conflict(affected(r.DB.ExecContext(ctx, `UPDATE shoes SET name = ?, brand = ?, model = ?, drop_mm = ?, type = ?, strava_gear_id = ?, retired = ?, note = ?, updated_at = ?
		WHERE user_id = ? AND id = ?`, s.Name, s.Brand, s.Model, s.DropMm, s.Type, s.StravaGearID, boolInt(s.Retired), s.Note, r.ts(), uid, s.ID)))
}

func (r Repo) DeleteShoe(ctx context.Context, uid, id int64) error {
	return affected(r.DB.ExecContext(ctx, `DELETE FROM shoes WHERE user_id = ? AND id = ?`, uid, id))
}

var ErrConflict = errors.New("conflict")

func conflict(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return fmt.Errorf("%w : cette chaussure Strava est déjà associée", ErrConflict)
	}
	return err
}

// ---------- notes ----------

func (r Repo) Notes(ctx context.Context, uid int64, aids []int64) (map[int64]ActivityNotes, error) {
	out := map[int64]ActivityNotes{}
	if len(aids) == 0 {
		return out, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(aids)), ",")
	args := []any{uid}
	for _, id := range aids {
		args = append(args, id)
	}
	// Pains linked to the activity in the body map win over the legacy note columns (projection).
	rows, err := r.DB.QueryContext(ctx, `SELECT n.activity_id,
		COALESCE((SELECT MAX(e.pain_score) FROM body_stress_entries e WHERE e.user_id = n.user_id AND e.activity_id = n.activity_id), n.pain_during),
		n.pain_cold,
		COALESCE((SELECT MAX(e.cold_next_day_score) FROM body_stress_entries e WHERE e.user_id = n.user_id AND e.activity_id = n.activity_id), n.pain_next_morning),
		n.rpe, n.shoe_id, n.tags, n.note, n.updated_at, n.shoe_auto
		FROM activity_notes n WHERE n.user_id = ? AND n.activity_id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	shoeIDs := map[int64]bool{}
	for rows.Next() {
		var aid, upd int64
		var pd, pc, pn, rpe sql.NullFloat64
		var shoe sql.NullInt64
		var tags string
		var auto int
		n := ActivityNotes{}
		if err := rows.Scan(&aid, &pd, &pc, &pn, &rpe, &shoe, &tags, &n.Note, &upd, &auto); err != nil {
			return nil, err
		}
		n.PainDuring, n.PainCold, n.PainNextMorning, n.RPE = nf(pd), nf(pc), nf(pn), nf(rpe)
		if shoe.Valid {
			id := shoe.Int64
			n.ShoeID = &id
			n.ShoeAuto = auto != 0
			shoeIDs[id] = true
		}
		_ = json.Unmarshal([]byte(tags), &n.Tags)
		if n.Tags == nil {
			n.Tags = []string{}
		}
		u := rfc(upd)
		n.UpdatedAt = &u
		out[aid] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(shoeIDs) > 0 {
		shoes, err := r.Shoes(ctx, uid)
		if err != nil {
			return nil, err
		}
		for aid, n := range out {
			for i := range shoes {
				if n.ShoeID != nil && shoes[i].ID == *n.ShoeID {
					s := shoes[i]
					n.Shoe = &s
				}
			}
			out[aid] = n
		}
	}
	return out, nil
}

// EmptyNotes is the value returned when nothing was entered.
func EmptyNotes() ActivityNotes { return ActivityNotes{Tags: []string{}} }

func (r Repo) NotesFor(ctx context.Context, uid, aid int64) (ActivityNotes, error) {
	m, err := r.Notes(ctx, uid, []int64{aid})
	if err != nil {
		return ActivityNotes{}, err
	}
	if n, ok := m[aid]; ok {
		return n, nil
	}
	return EmptyNotes(), nil
}

// PutNotes stores the notes of an activity the caller already checked belongs to uid.
func (r Repo) PutNotes(ctx context.Context, uid, aid int64, n *ActivityNotes) error {
	if err := n.Normalize(); err != nil {
		return err
	}
	if n.ShoeID != nil {
		if _, err := r.Shoe(ctx, uid, *n.ShoeID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return errors.New("shoeId inconnu")
			}
			return err
		}
	}
	tags, _ := json.Marshal(n.Tags)
	_, err := r.DB.ExecContext(ctx, `INSERT INTO activity_notes (activity_id, user_id, pain_during, pain_cold, pain_next_morning, rpe, shoe_id, tags, note, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (activity_id) DO UPDATE SET pain_during = excluded.pain_during, pain_cold = excluded.pain_cold,
		pain_next_morning = excluded.pain_next_morning, rpe = excluded.rpe,
		shoe_auto = CASE WHEN excluded.shoe_id IS activity_notes.shoe_id THEN activity_notes.shoe_auto ELSE 0 END, shoe_id = excluded.shoe_id, tags = excluded.tags,
		note = excluded.note, updated_at = excluded.updated_at WHERE activity_notes.user_id = excluded.user_id`,
		aid, uid, n.PainDuring, n.PainCold, n.PainNextMorning, n.RPE, n.ShoeID, string(tags), n.Note, r.ts())
	return err
}

// ---------- check-ins ----------

func (r Repo) Checkins(ctx context.Context, uid int64, from, to string) (map[string]DailyCheckin, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT date, training_readiness, readiness_status, note, updated_at FROM daily_checkins
		WHERE user_id = ? AND date >= ? AND date <= ? ORDER BY date`, uid, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]DailyCheckin{}
	for rows.Next() {
		var c DailyCheckin
		var tr sql.NullFloat64
		var st sql.NullString
		var upd int64
		if err := rows.Scan(&c.Date, &tr, &st, &c.Note, &upd); err != nil {
			return nil, err
		}
		c.TrainingReadiness, c.ReadinessStatus, c.UpdatedAt = nf(tr), ns(st), rfc(upd)
		out[c.Date] = c
	}
	return out, rows.Err()
}

func (r Repo) Checkin(ctx context.Context, uid int64, date string) (*DailyCheckin, error) {
	m, err := r.Checkins(ctx, uid, date, date)
	if err != nil {
		return nil, err
	}
	if c, ok := m[date]; ok {
		return &c, nil
	}
	return nil, nil
}

func (r Repo) PutCheckin(ctx context.Context, uid int64, c *DailyCheckin) error {
	if err := c.Normalize(); err != nil {
		return err
	}
	now := r.ts()
	c.UpdatedAt = rfc(now)
	_, err := r.DB.ExecContext(ctx, `INSERT INTO daily_checkins (user_id, date, training_readiness, readiness_status, note, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (user_id, date) DO UPDATE SET training_readiness = excluded.training_readiness,
		readiness_status = excluded.readiness_status, note = excluded.note, updated_at = excluded.updated_at`,
		uid, c.Date, c.TrainingReadiness, c.ReadinessStatus, c.Note, now)
	return err
}

// WipeManual deletes the user's manual data (backup restore in replace mode).
func (r Repo) WipeManual(ctx context.Context, uid int64) error {
	for _, q := range []string{
		`DELETE FROM activity_notes WHERE user_id = ?`, `DELETE FROM daily_checkins WHERE user_id = ?`,
		`DELETE FROM planned_sessions WHERE user_id = ?`, `DELETE FROM goals WHERE user_id = ?`,
		`DELETE FROM shoes WHERE user_id = ?`, `DELETE FROM athlete_profiles WHERE user_id = ?`,
	} {
		if _, err := r.DB.ExecContext(ctx, q, uid); err != nil {
			return err
		}
	}
	return nil
}
