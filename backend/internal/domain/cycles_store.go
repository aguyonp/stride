package domain

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"
)

const (
	planCols  = `id, goal_id, name, sport, start_date, end_date, target_event_label, status, note, created_at, updated_at`
	phaseCols = `id, plan_id, name, phase_type, order_index, start_date, end_date, focus, weekly_volume_km, target_weekly_load, target_weekly_duration_s, note`
	weekCols  = `id, plan_id, phase_id, week_start, week_end, week_number, is_assimilation, target_volume_km, target_load, target_duration_s, note`
)

func (s *Services) listPlans(ctx context.Context, uid int64) ([]TrainingPlan, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT `+planCols+` FROM training_plans WHERE user_id = ? ORDER BY start_date DESC, id DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrainingPlan{}
	for rows.Next() {
		p, err := scanTrainingPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Services) getPlan(ctx context.Context, uid, id int64) (*TrainingPlan, error) {
	p, err := scanTrainingPlan(s.db().QueryRowContext(ctx, `SELECT `+planCols+` FROM training_plans WHERE user_id = ? AND id = ?`, uid, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

// defaultPlan is the plan /api/cycles shows without planId: the active one, else the latest (nil without plan).
func defaultPlan(plans []TrainingPlan) *TrainingPlan {
	for i := range plans {
		if plans[i].Status == PlanStatusActive {
			return &plans[i]
		}
	}
	if len(plans) > 0 {
		return &plans[0]
	}
	return nil
}

// activePlanID returns the user's active plan (nil when none); q may be a transaction.
func activePlanID(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, uid int64) (*int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM training_plans WHERE user_id = ? AND status = 'active' ORDER BY start_date DESC, id DESC LIMIT 1`, uid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// listPhases returns the phases of a plan (planID nil: the phases attached to no plan).
func (s *Services) listPhases(ctx context.Context, uid int64, planID *int64) ([]TrainingPhase, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT `+phaseCols+` FROM plan_phases WHERE user_id = ? AND plan_id IS ? ORDER BY order_index, start_date, id`, uid, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrainingPhase{}
	for rows.Next() {
		ph, err := scanTrainingPhase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ph)
	}
	return out, rows.Err()
}

func (s *Services) getPhase(ctx context.Context, uid, id int64) (*TrainingPhase, error) {
	ph, err := scanTrainingPhase(s.db().QueryRowContext(ctx, `SELECT `+phaseCols+` FROM plan_phases WHERE user_id = ? AND id = ?`, uid, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &ph, err
}

// listStoredWeeks returns the week overrides of a plan (planID nil: standalone weeks).
func (s *Services) listStoredWeeks(ctx context.Context, uid int64, planID *int64) ([]storedWeekRow, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT `+weekCols+` FROM training_weeks WHERE user_id = ? AND plan_id IS ? ORDER BY week_start`, uid, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []storedWeekRow
	for rows.Next() {
		var w storedWeekRow
		var pid, phid sql.NullInt64
		var isAssim int
		var vol, ld, dur sql.NullFloat64
		if err := rows.Scan(&w.ID, &pid, &phid, &w.WeekStart, &w.WeekEnd, &w.WeekNumber, &isAssim, &vol, &ld, &dur, &w.Note); err != nil {
			return nil, err
		}
		w.PlanID, w.PhaseID, w.IsAssimilation = ni(pid), ni(phid), isAssim != 0
		w.TargetVolumeKm, w.TargetLoad, w.TargetDurationS = nfl(vol), nfl(ld), nfl(dur)
		out = append(out, w)
	}
	return out, rows.Err()
}

func ni(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func nfl(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func scanTrainingPlan(sc interface{ Scan(...any) error }) (TrainingPlan, error) {
	var p TrainingPlan
	var gid sql.NullInt64
	var status string
	var cat, uat int64
	if err := sc.Scan(&p.ID, &gid, &p.Name, &p.Sport, &p.StartDate, &p.EndDate, &p.TargetEventLabel, &status, &p.Note, &cat, &uat); err != nil {
		return p, err
	}
	p.GoalID, p.Status = ni(gid), CyclePlanStatus(status)
	p.CreatedAt, p.UpdatedAt = *rfc3339(cat), *rfc3339(uat)
	return p, nil
}

func scanTrainingPhase(sc interface{ Scan(...any) error }) (TrainingPhase, error) {
	var ph TrainingPhase
	var pid sql.NullInt64
	var pType string
	var vol, ld, dur sql.NullFloat64
	if err := sc.Scan(&ph.ID, &pid, &ph.Name, &pType, &ph.OrderIndex, &ph.StartDate, &ph.EndDate, &ph.Focus, &vol, &ld, &dur, &ph.Note); err != nil {
		return ph, err
	}
	ph.PlanID, ph.PhaseType = ni(pid), PhaseType(pType)
	ph.WeeklyVolumeKm, ph.TargetWeeklyLoad, ph.TargetWeeklyDurationS = nfl(vol), nfl(ld), nfl(dur)
	return ph, nil
}

func isValidPhaseType(pt PhaseType) bool {
	for _, k := range AllPhaseTypes {
		if k == pt {
			return true
		}
	}
	return false
}

func addDays(date string, days int) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, days).Format("2006-01-02")
}

func diffDays(from, to string) int {
	t1, err1 := time.Parse("2006-01-02", from)
	t2, err2 := time.Parse("2006-01-02", to)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(math.Round(t2.Sub(t1).Hours() / 24))
}
