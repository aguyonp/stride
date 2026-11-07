package domain

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

func validSport(v *string) bool {
	return v != nil && (*v == "run" || *v == "bike" || *v == "other")
}

func validStatus(v *CyclePlanStatus) bool {
	return v != nil && (*v == PlanStatusActive || *v == PlanStatusCompleted || *v == PlanStatusArchived)
}

func trimmed(v *string, def string) string {
	if v == nil {
		return def
	}
	return strings.TrimSpace(*v)
}

// ownGoal returns ErrNotFound unless the goal exists and belongs to uid (nil passes).
func (s *Services) ownGoal(ctx context.Context, uid int64, id *int64) error {
	if id == nil {
		return nil
	}
	_, err := s.repo().Goal(ctx, uid, *id)
	return repoErr(err)
}

// CreatePlan creates a macrocycle (archiving the previous active one), auto-scaffolding its phases
// and weeks unless autoScaffold is false.
func (s *Services) CreatePlan(ctx context.Context, uid int64, in TrainingPlanInput) (*TrainingCycleOverview, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || !isValidDate(in.StartDate) || !isValidDate(in.EndDate) || in.EndDate < in.StartDate {
		return nil, badRequest("Macrocycle invalide : nom et dates cohérentes requis.")
	}
	if err := s.ownGoal(ctx, uid, in.GoalID); err != nil {
		return nil, err
	}
	sport, status := "run", PlanStatusActive
	if validSport(in.Sport) {
		sport = *in.Sport
	}
	if validStatus(in.Status) {
		status = *in.Status
	}
	var planID int64
	err := s.deps.Store.Tx(ctx, func(tx *sql.Tx) error {
		now := s.now().Unix()
		if status == PlanStatusActive {
			if _, err := tx.ExecContext(ctx, `UPDATE training_plans SET status = 'archived', updated_at = ? WHERE user_id = ? AND status = 'active'`, now, uid); err != nil {
				return err
			}
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO training_plans (user_id, goal_id, name, sport, start_date, end_date, target_event_label, status, note, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uid, in.GoalID, name, sport, in.StartDate, in.EndDate, trimmed(in.TargetEventLabel, ""),
			string(status), trimmed(in.Note, ""), now, now)
		if err != nil {
			return err
		}
		if planID, err = res.LastInsertId(); err != nil {
			return err
		}
		if in.AutoScaffold != nil && !*in.AutoScaffold {
			return nil
		}
		peakKm := defaultPeakKm
		if in.PeakWeeklyVolumeKm != nil && *in.PeakWeeklyVolumeKm > 0 {
			peakKm = *in.PeakWeeklyVolumeKm
		}
		var peakLoad *float64
		if in.PeakWeeklyLoad != nil && *in.PeakWeeklyLoad > 0 {
			peakLoad = in.PeakWeeklyLoad
		}
		return insertScaffold(ctx, tx, uid, planID, now, scaffoldPhases(in.StartDate, in.EndDate, peakKm, peakLoad))
	})
	if err != nil {
		return nil, err
	}
	return s.Cycles(ctx, uid, &planID)
}

// PatchPlan modifies a training plan; activating it archives the other active plan.
func (s *Services) PatchPlan(ctx context.Context, uid, id int64, in TrainingPlanInput) (*TrainingPlan, error) {
	p, err := s.getPlan(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	if in.Name != "" {
		if p.Name = strings.TrimSpace(in.Name); p.Name == "" {
			return nil, badRequest("Le nom du macrocycle est requis.")
		}
	}
	if in.GoalID != nil {
		if err := s.ownGoal(ctx, uid, in.GoalID); err != nil {
			return nil, err
		}
		p.GoalID = in.GoalID
	}
	if validSport(in.Sport) {
		p.Sport = *in.Sport
	}
	for _, d := range []struct {
		in  string
		out *string
		msg string
	}{{in.StartDate, &p.StartDate, "Date de début invalide."}, {in.EndDate, &p.EndDate, "Date de fin invalide."}} {
		if d.in == "" {
			continue
		}
		if !isValidDate(d.in) {
			return nil, badRequest("%s", d.msg)
		}
		*d.out = d.in
	}
	if p.EndDate < p.StartDate {
		return nil, badRequest("La date de fin doit être postérieure à la date de début.")
	}
	if validStatus(in.Status) {
		p.Status = *in.Status
	}
	p.TargetEventLabel, p.Note = trimmed(in.TargetEventLabel, p.TargetEventLabel), trimmed(in.Note, p.Note)
	err = s.deps.Store.Tx(ctx, func(tx *sql.Tx) error {
		now := s.now().Unix()
		if p.Status == PlanStatusActive {
			if _, err := tx.ExecContext(ctx, `UPDATE training_plans SET status = 'archived', updated_at = ? WHERE user_id = ? AND id != ? AND status = 'active'`, now, uid, id); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `UPDATE training_plans SET name = ?, goal_id = ?, sport = ?, start_date = ?, end_date = ?, target_event_label = ?,
			status = ?, note = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
			p.Name, p.GoalID, p.Sport, p.StartDate, p.EndDate, p.TargetEventLabel, string(p.Status), p.Note, now, uid, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.getPlan(ctx, uid, id)
}

// DeletePlan deletes a plan with its phases and weeks.
func (s *Services) DeletePlan(ctx context.Context, uid, id int64) error {
	return s.deleteOwned(ctx, `DELETE FROM training_plans WHERE user_id = ? AND id = ?`, uid, id)
}

func (s *Services) deleteOwned(ctx context.Context, q string, uid, id int64) error {
	res, err := s.db().ExecContext(ctx, q, uid, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return cmpErr(err, ErrNotFound)
	}
	return nil
}

func cmpErr(err, def error) error {
	if err != nil {
		return err
	}
	return def
}

// builtOverview returns the overview of planID (nil: phases and weeks attached to no plan).
func (s *Services) builtOverview(ctx context.Context, uid int64, planID *int64) (*TrainingCycleOverview, error) {
	plans, err := s.listPlans(ctx, uid)
	if err != nil {
		return nil, err
	}
	var plan *TrainingPlan
	if planID != nil {
		if plan = findPlan(plans, *planID); plan == nil {
			return nil, ErrNotFound
		}
	}
	return s.overview(ctx, uid, plans, plan)
}

// phaseOf returns phase id as built in its plan overview (weeks and actuals).
func (s *Services) phaseOf(ctx context.Context, uid int64, planID *int64, id int64) (*TrainingPhase, error) {
	ov, err := s.builtOverview(ctx, uid, planID)
	if err != nil {
		return nil, err
	}
	for i := range ov.Phases {
		if ov.Phases[i].ID == id {
			return &ov.Phases[i], nil
		}
	}
	return nil, ErrNotFound
}

// CreatePhase creates a mesocycle in planId (which must belong to the user), else in the active plan.
func (s *Services) CreatePhase(ctx context.Context, uid int64, in TrainingPhaseInput) (*TrainingPhase, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || !isValidDate(in.StartDate) || !isValidDate(in.EndDate) || in.EndDate < in.StartDate {
		return nil, badRequest("Mésocycle invalide : nom et dates cohérentes requis.")
	}
	planID := in.PlanID
	if planID != nil {
		if _, err := s.getPlan(ctx, uid, *planID); err != nil {
			return nil, err
		}
	} else {
		var err error
		if planID, err = activePlanID(ctx, s.db(), uid); err != nil {
			return nil, err
		}
	}
	phaseType := PhaseBuild
	if in.PhaseType != nil && isValidPhaseType(*in.PhaseType) {
		phaseType = *in.PhaseType
	}
	var order int
	if in.OrderIndex != nil {
		order = *in.OrderIndex
	} else if err := s.db().QueryRowContext(ctx, `SELECT COUNT(*) FROM plan_phases WHERE user_id = ? AND plan_id IS ?`, uid, planID).Scan(&order); err != nil {
		return nil, err
	}
	now := s.now().Unix()
	res, err := s.db().ExecContext(ctx, `INSERT INTO plan_phases (user_id, plan_id, name, phase_type, order_index, start_date, end_date, focus,
		weekly_volume_km, target_weekly_load, target_weekly_duration_s, note, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, planID, name, string(phaseType), order, in.StartDate, in.EndDate, trimmed(in.Focus, ""), in.WeeklyVolumeKm, in.TargetWeeklyLoad,
		in.TargetWeeklyDurationS, trimmed(in.Note, ""), now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.phaseOf(ctx, uid, planID, id)
}

// PatchPhase modifies a mesocycle (its plan cannot change).
func (s *Services) PatchPhase(ctx context.Context, uid, id int64, in TrainingPhaseInput) (*TrainingPhase, error) {
	ph, err := s.getPhase(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	if in.Name != "" {
		ph.Name = strings.TrimSpace(in.Name)
	}
	for _, d := range []struct {
		in  string
		out *string
		msg string
	}{{in.StartDate, &ph.StartDate, "Date de début invalide."}, {in.EndDate, &ph.EndDate, "Date de fin invalide."}} {
		if d.in == "" {
			continue
		}
		if !isValidDate(d.in) {
			return nil, badRequest("%s", d.msg)
		}
		*d.out = d.in
	}
	if ph.EndDate < ph.StartDate {
		return nil, badRequest("La date de fin doit être postérieure à la date de début.")
	}
	if in.PhaseType != nil && isValidPhaseType(*in.PhaseType) {
		ph.PhaseType = *in.PhaseType
	}
	if in.OrderIndex != nil {
		ph.OrderIndex = *in.OrderIndex
	}
	ph.WeeklyVolumeKm = cmpOr(in.WeeklyVolumeKm, ph.WeeklyVolumeKm)
	ph.TargetWeeklyLoad = cmpOr(in.TargetWeeklyLoad, ph.TargetWeeklyLoad)
	ph.TargetWeeklyDurationS = cmpOr(in.TargetWeeklyDurationS, ph.TargetWeeklyDurationS)
	ph.Focus, ph.Note = trimmed(in.Focus, ph.Focus), trimmed(in.Note, ph.Note)
	if _, err := s.db().ExecContext(ctx, `UPDATE plan_phases SET name = ?, phase_type = ?, order_index = ?, start_date = ?, end_date = ?, focus = ?,
		weekly_volume_km = ?, target_weekly_load = ?, target_weekly_duration_s = ?, note = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		ph.Name, string(ph.PhaseType), ph.OrderIndex, ph.StartDate, ph.EndDate, ph.Focus, ph.WeeklyVolumeKm, ph.TargetWeeklyLoad,
		ph.TargetWeeklyDurationS, ph.Note, s.now().Unix(), uid, id); err != nil {
		return nil, err
	}
	return s.phaseOf(ctx, uid, ph.PlanID, id)
}

// DeletePhase deletes a mesocycle.
func (s *Services) DeletePhase(ctx context.Context, uid, id int64) error {
	return s.deleteOwned(ctx, `DELETE FROM plan_phases WHERE user_id = ? AND id = ?`, uid, id)
}

// PutWeek creates or updates a microcycle override. planId and phaseId must belong to the user
// and the phase to that plan.
func (s *Services) PutWeek(ctx context.Context, uid int64, in TrainingWeekInput) (*TrainingWeek, error) {
	if !isValidDate(in.WeekStart) {
		return nil, badRequest("weekStart invalide (format AAAA-MM-JJ attendu).")
	}
	if in.PlanID != nil {
		if _, err := s.getPlan(ctx, uid, *in.PlanID); err != nil {
			return nil, err
		}
	}
	if in.PhaseID != nil {
		ph, err := s.getPhase(ctx, uid, *in.PhaseID)
		if err != nil {
			return nil, err
		}
		if (ph.PlanID == nil) != (in.PlanID == nil) || (ph.PlanID != nil && *ph.PlanID != *in.PlanID) {
			return nil, badRequest("Le mésocycle n'appartient pas à ce macrocycle.")
		}
	}
	ws := weekStart(in.WeekStart)
	isAssim := 0
	if in.IsAssimilation != nil && *in.IsAssimilation {
		isAssim = 1
	}
	note, now := trimmed(in.Note, ""), s.now().Unix()
	var id int64
	err := s.db().QueryRowContext(ctx, `SELECT id FROM training_weeks WHERE user_id = ? AND plan_id IS ? AND week_start = ?`, uid, in.PlanID, ws).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err = s.db().ExecContext(ctx, `INSERT INTO training_weeks (user_id, plan_id, phase_id, week_start, week_end, is_assimilation,
			target_volume_km, target_load, target_duration_s, note, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uid, in.PlanID, in.PhaseID, ws, addDays(ws, 6), isAssim, in.TargetVolumeKm, in.TargetLoad, in.TargetDurationS, note, now, now)
	case err == nil:
		_, err = s.db().ExecContext(ctx, `UPDATE training_weeks SET phase_id = COALESCE(?, phase_id), is_assimilation = ?, target_volume_km = ?,
			target_load = ?, target_duration_s = ?, note = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
			in.PhaseID, isAssim, in.TargetVolumeKm, in.TargetLoad, in.TargetDurationS, note, now, id, uid)
	}
	if err != nil {
		return nil, err
	}
	ov, err := s.builtOverview(ctx, uid, in.PlanID)
	if err != nil {
		return nil, err
	}
	for _, ph := range ov.Phases {
		for _, w := range ph.Weeks {
			if w.WeekStart == ws {
				return &w, nil
			}
		}
	}
	for _, w := range ov.StandaloneWeeks {
		if w.WeekStart == ws {
			return &w, nil
		}
	}
	return nil, ErrNotFound
}

// defaultPlanID is the plan /api/cycles shows without planId (nil: phases attached to no plan).
func (s *Services) defaultPlanID(ctx context.Context, uid int64) (*int64, error) {
	plans, err := s.listPlans(ctx, uid)
	if err != nil {
		return nil, err
	}
	if p := defaultPlan(plans); p != nil {
		return &p.ID, nil
	}
	return nil, nil
}
