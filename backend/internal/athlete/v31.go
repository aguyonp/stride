package athlete

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
)

// ---------- personal records ----------

type PersonalRecord struct {
	Key          string   `json:"key"`
	Label        string   `json:"label"`
	DistanceM    float64  `json:"distanceM"`
	TimeS        *float64 `json:"timeS"`
	PaceSecPerKm *float64 `json:"paceSecPerKm"`
	Date         *string  `json:"date"`
	ActivityID   *int64   `json:"activityId"`
	Source       string   `json:"source"` // computed | strava | manual
}

type RecordDef struct {
	Key, Label string
	DistanceM  float64 // 0 for "longest"
}

const RecordLongest = "longest"

var RecordDefs = []RecordDef{
	{"1k", "1 km", 1000}, {"5k", "5 km", 5000}, {"10k", "10 km", 10000},
	{"half", "Semi-marathon", 21097.5}, {"marathon", "Marathon", 42195}, {RecordLongest, "Plus longue sortie", 0},
}

func RecordDefOf(key string) (RecordDef, bool) {
	for _, d := range RecordDefs {
		if d.Key == key {
			return d, true
		}
	}
	return RecordDef{}, false
}

// ManualRecord is the athlete's override of one record.
type ManualRecord struct {
	DistanceM float64
	TimeS     *float64
	Date      *string
}

// NormalizeManualRecord validates a PUT /api/records/{key} body against the record definition.
func NormalizeManualRecord(def RecordDef, timeS, distanceM *float64, date *string) (ManualRecord, error) {
	m := ManualRecord{DistanceM: def.DistanceM, TimeS: timeS, Date: trimPtr(date)}
	var errs []error
	if def.Key == RecordLongest {
		if distanceM == nil || !between(*distanceM, 100, 500000) {
			errs = append(errs, errors.New("distanceM requise pour la plus longue sortie (100 m à 500 km)"))
		} else {
			m.DistanceM = *distanceM
		}
		if timeS != nil && !between(*timeS, 60, 7*86400) {
			errs = append(errs, errors.New("timeS hors limites"))
		}
	} else {
		if distanceM != nil && math.Abs(*distanceM-def.DistanceM) > 1 {
			errs = append(errs, fmt.Errorf("distanceM fixée à %g m pour ce record", def.DistanceM))
		}
		if timeS == nil || !between(*timeS, def.DistanceM/1000*120, def.DistanceM/1000*1800) {
			errs = append(errs, errors.New("timeS requis (allure comprise entre 2:00 et 30:00/km)"))
		}
	}
	if m.Date != nil && !ValidDate(*m.Date) {
		errs = append(errs, errors.New("date attendue au format AAAA-MM-JJ"))
	}
	return m, errors.Join(errs...)
}

func (r Repo) ManualRecords(ctx context.Context, uid int64) (map[string]ManualRecord, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT key, distance_m, time_s, date FROM personal_records WHERE user_id = ?`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ManualRecord{}
	for rows.Next() {
		var k string
		var m ManualRecord
		var t sql.NullFloat64
		var d sql.NullString
		if err := rows.Scan(&k, &m.DistanceM, &t, &d); err != nil {
			return nil, err
		}
		m.TimeS, m.Date = nf(t), ns(d)
		out[k] = m
	}
	return out, rows.Err()
}

func (r Repo) PutManualRecord(ctx context.Context, uid int64, key string, m ManualRecord) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO personal_records (user_id, key, distance_m, time_s, date, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, key) DO UPDATE SET distance_m = excluded.distance_m, time_s = excluded.time_s, date = excluded.date, updated_at = excluded.updated_at`,
		uid, key, m.DistanceM, m.TimeS, m.Date, r.ts())
	return err
}

func (r Repo) DeleteManualRecord(ctx context.Context, uid int64, key string) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM personal_records WHERE user_id = ? AND key = ?`, uid, key)
	return err
}

// ---------- calendar events ----------

type CalendarEvent struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
	Severity  int    `json:"severity"`
	Note      string `json:"note"`
}

var (
	EventKinds    = []string{"fatigue", "pain", "illness", "unavailable", "forced_rest"}
	EventFields   = []string{"kind", "startDate", "endDate", "severity", "note"}
	EventReadOnly = []string{"id"}
)

var eventLabels = map[string]string{"fatigue": "Fatigue", "pain": "Douleur", "illness": "Maladie", "unavailable": "Indisponible", "forced_rest": "Repos forcé"}

func EventLabel(kind string) string {
	if l, ok := eventLabels[kind]; ok {
		return l
	}
	return kind
}

// Covers reports whether the event includes date (inclusive bounds).
func (e CalendarEvent) Covers(date string) bool { return date >= e.StartDate && date <= e.EndDate }

func (e *CalendarEvent) Normalize() error {
	e.Note = strings.TrimSpace(e.Note)
	if e.EndDate == "" {
		e.EndDate = e.StartDate
	}
	if e.Severity == 0 {
		e.Severity = 2
	}
	var errs []error
	if !oneOf(e.Kind, EventKinds) {
		errs = append(errs, fmt.Errorf("kind invalide (%s)", strings.Join(EventKinds, ", ")))
	}
	if !ValidDate(e.StartDate) || !ValidDate(e.EndDate) {
		errs = append(errs, errors.New("startDate et endDate attendues au format AAAA-MM-JJ"))
	} else if e.EndDate < e.StartDate {
		errs = append(errs, errors.New("endDate doit être ≥ startDate"))
	}
	if e.Severity < 1 || e.Severity > 3 {
		errs = append(errs, errors.New("severity attendue entre 1 et 3"))
	}
	errs = append(errs, maxLen(e.Note, 1000, "note"))
	return errors.Join(errs...)
}

const eventCols = `id, kind, start_date, end_date, severity, note`

func scanEvent(sc interface{ Scan(...any) error }) (CalendarEvent, error) {
	var e CalendarEvent
	err := sc.Scan(&e.ID, &e.Kind, &e.StartDate, &e.EndDate, &e.Severity, &e.Note)
	return e, err
}

// Events returns the events overlapping [from, to] (empty bounds are open), by start date.
func (r Repo) Events(ctx context.Context, uid int64, from, to string) ([]CalendarEvent, error) {
	if from == "" {
		from = "0000-00-00"
	}
	if to == "" {
		to = "9999-99-99"
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT `+eventCols+` FROM calendar_events WHERE user_id = ? AND end_date >= ? AND start_date <= ?
		ORDER BY start_date, id`, uid, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CalendarEvent{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r Repo) Event(ctx context.Context, uid, id int64) (CalendarEvent, error) {
	e, err := scanEvent(r.DB.QueryRowContext(ctx, `SELECT `+eventCols+` FROM calendar_events WHERE user_id = ? AND id = ?`, uid, id))
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}

func (r Repo) CreateEvent(ctx context.Context, uid int64, e *CalendarEvent) error {
	now := r.ts()
	res, err := r.DB.ExecContext(ctx, `INSERT INTO calendar_events (user_id, kind, start_date, end_date, severity, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, uid, e.Kind, e.StartDate, e.EndDate, e.Severity, e.Note, now, now)
	if err != nil {
		return err
	}
	e.ID, err = res.LastInsertId()
	return err
}

func (r Repo) UpdateEvent(ctx context.Context, uid int64, e *CalendarEvent) error {
	return affected(r.DB.ExecContext(ctx, `UPDATE calendar_events SET kind = ?, start_date = ?, end_date = ?, severity = ?, note = ?, updated_at = ?
		WHERE user_id = ? AND id = ?`, e.Kind, e.StartDate, e.EndDate, e.Severity, e.Note, r.ts(), uid, e.ID))
}

func (r Repo) DeleteEvent(ctx context.Context, uid, id int64) error {
	return affected(r.DB.ExecContext(ctx, `DELETE FROM calendar_events WHERE user_id = ? AND id = ?`, uid, id))
}

// ---------- plan phases ----------

type PlanPhase struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	StartDate      string   `json:"startDate"`
	EndDate        string   `json:"endDate"`
	Focus          string   `json:"focus"`
	WeeklyVolumeKm *float64 `json:"weeklyVolumeKm"`
	Note           string   `json:"note"`
}

func (p *PlanPhase) Normalize() error {
	p.Name, p.Focus, p.Note = strings.TrimSpace(p.Name), strings.TrimSpace(p.Focus), strings.TrimSpace(p.Note)
	var errs []error
	if p.Name == "" {
		errs = append(errs, errors.New("nom de phase requis"))
	}
	if !ValidDate(p.StartDate) || !ValidDate(p.EndDate) {
		errs = append(errs, errors.New("startDate et endDate attendues au format AAAA-MM-JJ"))
	} else if p.EndDate < p.StartDate {
		errs = append(errs, errors.New("endDate doit être ≥ startDate"))
	}
	if p.WeeklyVolumeKm != nil && !between(*p.WeeklyVolumeKm, 0, 500) {
		errs = append(errs, errors.New("weeklyVolumeKm hors limites [0, 500]"))
	}
	errs = append(errs, maxLen(p.Name, 80, "nom"), maxLen(p.Focus, 300, "focus"), maxLen(p.Note, 1000, "note"))
	return errors.Join(errs...)
}

// Phases returns the phases of a plan (planID nil: phases attached to no plan) overlapping
// [from, to] (empty bounds are open), by start date.
func (r Repo) Phases(ctx context.Context, uid int64, planID *int64, from, to string) ([]PlanPhase, error) {
	if from == "" {
		from = "0000-00-00"
	}
	if to == "" {
		to = "9999-99-99"
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT id, name, start_date, end_date, focus, weekly_volume_km, note FROM plan_phases
		WHERE user_id = ? AND plan_id IS ? AND end_date >= ? AND start_date <= ? ORDER BY start_date, id`, uid, planID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlanPhase{}
	for rows.Next() {
		var p PlanPhase
		var vol sql.NullFloat64
		if err := rows.Scan(&p.ID, &p.Name, &p.StartDate, &p.EndDate, &p.Focus, &vol, &p.Note); err != nil {
			return nil, err
		}
		p.WeeklyVolumeKm = nf(vol)
		out = append(out, p)
	}
	return out, rows.Err()
}

// ReplacePhases attaches the new phases to planID (nil: no plan) after deleting that plan's phases
// overlapping them, then renumbers the plan's phases by date (use in a transaction).
func (r Repo) ReplacePhases(ctx context.Context, uid int64, planID *int64, phases []PlanPhase) error {
	if len(phases) == 0 {
		return nil
	}
	now := r.ts()
	for _, p := range phases {
		if _, err := r.DB.ExecContext(ctx, `DELETE FROM plan_phases WHERE user_id = ? AND plan_id IS ? AND end_date >= ? AND start_date <= ?`,
			uid, planID, p.StartDate, p.EndDate); err != nil {
			return err
		}
	}
	for i := range phases {
		p := &phases[i]
		res, err := r.DB.ExecContext(ctx, `INSERT INTO plan_phases (user_id, plan_id, name, start_date, end_date, focus, weekly_volume_km, note, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uid, planID, p.Name, p.StartDate, p.EndDate, p.Focus, p.WeeklyVolumeKm, p.Note, now, now)
		if err != nil {
			return err
		}
		if p.ID, err = res.LastInsertId(); err != nil {
			return err
		}
	}
	_, err := r.DB.ExecContext(ctx, `UPDATE plan_phases SET order_index = (SELECT COUNT(*) FROM plan_phases o WHERE o.user_id = plan_phases.user_id
		AND o.plan_id IS plan_phases.plan_id AND (o.start_date < plan_phases.start_date OR (o.start_date = plan_phases.start_date AND o.id < plan_phases.id)))
		WHERE user_id = ? AND plan_id IS ?`, uid, planID)
	return err
}
