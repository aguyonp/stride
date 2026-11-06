package domain

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"time"

	"stride/internal/athlete"
)

type PhaseType string

const (
	PhaseFoundation   PhaseType = "foundation"
	PhaseBuild        PhaseType = "build"
	PhaseThreshold    PhaseType = "threshold"
	PhaseSpecific     PhaseType = "specific"
	PhaseAssimilation PhaseType = "assimilation"
	PhaseTaper        PhaseType = "taper"
	PhaseRecovery     PhaseType = "recovery"
)

var AllPhaseTypes = []PhaseType{
	PhaseFoundation,
	PhaseBuild,
	PhaseThreshold,
	PhaseSpecific,
	PhaseAssimilation,
	PhaseTaper,
	PhaseRecovery,
}

type CyclePlanStatus string

const (
	PlanStatusActive    CyclePlanStatus = "active"
	PlanStatusCompleted CyclePlanStatus = "completed"
	PlanStatusArchived  CyclePlanStatus = "archived"
)

type CycleActuals struct {
	PlannedVolumeKm     float64  `json:"plannedVolumeKm"`
	ActualVolumeKm      float64  `json:"actualVolumeKm"`
	VolumeCompletionPct *float64 `json:"volumeCompletionPct"`
	PlannedLoad         float64  `json:"plannedLoad"`
	ActualLoad          float64  `json:"actualLoad"`
	LoadCompletionPct   *float64 `json:"loadCompletionPct"`
	PlannedDurationS    float64  `json:"plannedDurationS"`
	ActualDurationS     float64  `json:"actualDurationS"`
	SessionsPlanned     int      `json:"sessionsPlanned"`
	SessionsCompleted   int      `json:"sessionsCompleted"`
}

type TrainingPlan struct {
	ID               int64           `json:"id"`
	GoalID           *int64          `json:"goalId"`
	Name             string          `json:"name"`
	Sport            string          `json:"sport"`
	StartDate        string          `json:"startDate"`
	EndDate          string          `json:"endDate"`
	TargetEventLabel string          `json:"targetEventLabel"`
	Status           CyclePlanStatus `json:"status"`
	Note             string          `json:"note"`
	CreatedAt        string          `json:"createdAt"`
	UpdatedAt        string          `json:"updatedAt"`
}

type TrainingPlanInput struct {
	Name               string           `json:"name"`
	GoalID             *int64           `json:"goalId,omitempty"`
	Sport              *string          `json:"sport,omitempty"`
	StartDate          string           `json:"startDate"`
	EndDate            string           `json:"endDate"`
	TargetEventLabel   *string          `json:"targetEventLabel,omitempty"`
	Status             *CyclePlanStatus `json:"status,omitempty"`
	Note               *string          `json:"note,omitempty"`
	AutoScaffold       *bool            `json:"autoScaffold,omitempty"`
	PeakWeeklyVolumeKm *float64         `json:"peakWeeklyVolumeKm,omitempty"`
	PeakWeeklyLoad     *float64         `json:"peakWeeklyLoad,omitempty"`
}

type TrainingWeek struct {
	ID              int64                    `json:"id"`
	PlanID          *int64                   `json:"planId"`
	PhaseID         *int64                   `json:"phaseId"`
	WeekStart       string                   `json:"weekStart"`
	WeekEnd         string                   `json:"weekEnd"`
	WeekNumber      int                      `json:"weekNumber"`
	IsAssimilation  bool                     `json:"isAssimilation"`
	TargetVolumeKm  *float64                 `json:"targetVolumeKm"`
	TargetLoad      *float64                 `json:"targetLoad"`
	TargetDurationS *float64                 `json:"targetDurationS"`
	Note            string                   `json:"note"`
	Actuals         CycleActuals             `json:"actuals"`
	Workouts        []athlete.PlannedSession `json:"workouts"`
}

type TrainingWeekInput struct {
	PlanID          *int64   `json:"planId,omitempty"`
	PhaseID         *int64   `json:"phaseId,omitempty"`
	WeekStart       string   `json:"weekStart"`
	IsAssimilation  *bool    `json:"isAssimilation,omitempty"`
	TargetVolumeKm  *float64 `json:"targetVolumeKm,omitempty"`
	TargetLoad      *float64 `json:"targetLoad,omitempty"`
	TargetDurationS *float64 `json:"targetDurationS,omitempty"`
	Note            *string  `json:"note,omitempty"`
}

type TrainingPhase struct {
	ID                    int64          `json:"id"`
	PlanID                *int64         `json:"planId"`
	Name                  string         `json:"name"`
	PhaseType             PhaseType      `json:"phaseType"`
	OrderIndex            int            `json:"orderIndex"`
	StartDate             string         `json:"startDate"`
	EndDate               string         `json:"endDate"`
	Focus                 string         `json:"focus"`
	WeeklyVolumeKm        *float64       `json:"weeklyVolumeKm"`
	TargetWeeklyLoad      *float64       `json:"targetWeeklyLoad"`
	TargetWeeklyDurationS *float64       `json:"targetWeeklyDurationS"`
	Note                  string         `json:"note"`
	Actuals               CycleActuals   `json:"actuals"`
	Weeks                 []TrainingWeek `json:"weeks"`
}

type TrainingPhaseInput struct {
	PlanID                *int64     `json:"planId,omitempty"`
	Name                  string     `json:"name"`
	PhaseType             *PhaseType `json:"phaseType,omitempty"`
	OrderIndex            *int       `json:"orderIndex,omitempty"`
	StartDate             string     `json:"startDate"`
	EndDate               string     `json:"endDate"`
	Focus                 *string    `json:"focus,omitempty"`
	WeeklyVolumeKm        *float64   `json:"weeklyVolumeKm,omitempty"`
	TargetWeeklyLoad      *float64   `json:"targetWeeklyLoad,omitempty"`
	TargetWeeklyDurationS *float64   `json:"targetWeeklyDurationS,omitempty"`
	Note                  *string    `json:"note,omitempty"`
}

type TrainingCycleOverview struct {
	Plans           []TrainingPlan  `json:"plans"`
	ActivePlan      *TrainingPlan   `json:"activePlan"`
	Phases          []TrainingPhase `json:"phases"`
	StandaloneWeeks []TrainingWeek  `json:"standaloneWeeks"`
	Totals          CycleActuals    `json:"totals"`
}

type storedWeekRow struct {
	ID              int64
	PlanID          *int64
	PhaseID         *int64
	WeekStart       string
	WeekEnd         string
	WeekNumber      int
	IsAssimilation  bool
	TargetVolumeKm  *float64
	TargetLoad      *float64
	TargetDurationS *float64
	Note            string
}

// Cycles retrieves the full cycle overview for a plan or the currently active plan.
func (s *Services) Cycles(ctx context.Context, uid int64, requestedPlanID *int64) (*TrainingCycleOverview, error) {
	// 1. Fetch all training plans for user
	plans, err := s.listPlans(ctx, uid)
	if err != nil {
		return nil, err
	}

	var activePlan *TrainingPlan
	if requestedPlanID != nil {
		for i := range plans {
			if plans[i].ID == *requestedPlanID {
				activePlan = &plans[i]
				break
			}
		}
	}
	if activePlan == nil {
		for i := range plans {
			if plans[i].Status == PlanStatusActive {
				activePlan = &plans[i]
				break
			}
		}
	}
	if activePlan == nil && len(plans) > 0 {
		activePlan = &plans[0]
	}

	// 2. Fetch raw phases for active plan (or orphaned phases if no plan)
	var rawPhases []TrainingPhase
	if activePlan != nil {
		rawPhases, err = s.listPhasesForPlan(ctx, uid, activePlan.ID)
	} else {
		rawPhases, err = s.listPhasesOrphaned(ctx, uid)
	}
	if err != nil {
		return nil, err
	}

	// 3. Fetch all stored training_weeks overrides for this plan (or standalone)
	storedWeeks, err := s.listStoredWeeks(ctx, uid, activePlan)
	if err != nil {
		return nil, err
	}
	weekByStart := make(map[string]storedWeekRow, len(storedWeeks))
	for _, w := range storedWeeks {
		weekByStart[w.WeekStart] = w
	}

	var planStartMon *string
	if activePlan != nil {
		m := mondayOf(activePlan.StartDate)
		planStartMon = &m
	}

	// 4. Build phases with weekly progression
	builtPhases := make([]TrainingPhase, 0, len(rawPhases))
	coveredWeekStarts := make(map[string]bool)

	for _, ph := range rawPhases {
		startMon := mondayOf(ph.StartDate)
		endMon := mondayOf(ph.EndDate)
		weeksCount := max(1, diffDays(startMon, endMon)/7+1)

		phaseWeeks := make([]TrainingWeek, 0, weeksCount)
		for i := 0; i < weeksCount; i++ {
			wStart := addDays(startMon, i*7)
			wEnd := addDays(wStart, 6)
			coveredWeekStarts[wStart] = true

			ov, hasOv := weekByStart[wStart]

			targetKm := ph.WeeklyVolumeKm
			if hasOv && ov.TargetVolumeKm != nil {
				targetKm = ov.TargetVolumeKm
			}
			targetLd := ph.TargetWeeklyLoad
			if hasOv && ov.TargetLoad != nil {
				targetLd = ov.TargetLoad
			}
			targetDur := ph.TargetWeeklyDurationS
			if hasOv && ov.TargetDurationS != nil {
				targetDur = ov.TargetDurationS
			}

			actuals, workouts, err := s.computeActualsForRange(ctx, uid, wStart, wEnd, targetKm, targetLd, targetDur)
			if err != nil {
				return nil, err
			}

			weekNumber := i + 1
			if planStartMon != nil {
				weekNumber = max(1, diffDays(*planStartMon, wStart)/7+1)
			}

			isAssim := ph.PhaseType == PhaseAssimilation
			if hasOv {
				isAssim = ov.IsAssimilation
			}

			var wid int64
			if hasOv {
				wid = ov.ID
			} else {
				wid = 100000 + ph.ID*100 + int64(i)
			}

			var pid *int64
			if activePlan != nil {
				pid = &activePlan.ID
			}

			phaseId := ph.ID
			note := ""
			if hasOv {
				note = ov.Note
			}

			phaseWeeks = append(phaseWeeks, TrainingWeek{
				ID:              wid,
				PlanID:          pid,
				PhaseID:         &phaseId,
				WeekStart:       wStart,
				WeekEnd:         wEnd,
				WeekNumber:      weekNumber,
				IsAssimilation:  isAssim,
				TargetVolumeKm:  targetKm,
				TargetLoad:      targetLd,
				TargetDurationS: targetDur,
				Note:            note,
				Actuals:         actuals,
				Workouts:        workouts,
			})
		}

		pPlannedKm := 0.0
		pActualKm := 0.0
		pPlannedLd := 0.0
		pActualLd := 0.0
		pPlannedDur := 0.0
		pActualDur := 0.0
		pSessPlan := 0
		pSessComp := 0
		for _, w := range phaseWeeks {
			pPlannedKm += w.Actuals.PlannedVolumeKm
			pActualKm += w.Actuals.ActualVolumeKm
			pPlannedLd += w.Actuals.PlannedLoad
			pActualLd += w.Actuals.ActualLoad
			pPlannedDur += w.Actuals.PlannedDurationS
			pActualDur += w.Actuals.ActualDurationS
			pSessPlan += w.Actuals.SessionsPlanned
			pSessComp += w.Actuals.SessionsCompleted
		}
		pPlannedKm = math.Round(pPlannedKm*10) / 10
		pActualKm = math.Round(pActualKm*10) / 10
		pPlannedLd = math.Round(pPlannedLd)
		pActualLd = math.Round(pActualLd)

		var volPct *float64
		if pPlannedKm > 0 {
			v := math.Round((pActualKm/pPlannedKm)*1000) / 10
			volPct = &v
		}
		var ldPct *float64
		if pPlannedLd > 0 {
			v := math.Round((pActualLd/pPlannedLd)*1000) / 10
			ldPct = &v
		}

		ph.Actuals = CycleActuals{
			PlannedVolumeKm:     pPlannedKm,
			ActualVolumeKm:      pActualKm,
			VolumeCompletionPct: volPct,
			PlannedLoad:         pPlannedLd,
			ActualLoad:          pActualLd,
			LoadCompletionPct:   ldPct,
			PlannedDurationS:    pPlannedDur,
			ActualDurationS:     pActualDur,
			SessionsPlanned:     pSessPlan,
			SessionsCompleted:   pSessComp,
		}
		ph.Weeks = phaseWeeks
		builtPhases = append(builtPhases, ph)
	}

	// 5. Standalone weeks not covered by any phase
	var standaloneWeeks []TrainingWeek
	for _, ov := range storedWeeks {
		if coveredWeekStarts[ov.WeekStart] {
			continue
		}
		wEnd := addDays(ov.WeekStart, 6)
		actuals, workouts, err := s.computeActualsForRange(ctx, uid, ov.WeekStart, wEnd, ov.TargetVolumeKm, ov.TargetLoad, ov.TargetDurationS)
		if err != nil {
			return nil, err
		}
		standaloneWeeks = append(standaloneWeeks, TrainingWeek{
			ID:              ov.ID,
			PlanID:          ov.PlanID,
			PhaseID:         ov.PhaseID,
			WeekStart:       ov.WeekStart,
			WeekEnd:         wEnd,
			WeekNumber:      ov.WeekNumber,
			IsAssimilation:  ov.IsAssimilation,
			TargetVolumeKm:  ov.TargetVolumeKm,
			TargetLoad:      ov.TargetLoad,
			TargetDurationS: ov.TargetDurationS,
			Note:            ov.Note,
			Actuals:         actuals,
			Workouts:        workouts,
		})
	}

	// 6. Aggregate totals across all weeks
	var allWeeks []TrainingWeek
	for _, p := range builtPhases {
		allWeeks = append(allWeeks, p.Weeks...)
	}
	allWeeks = append(allWeeks, standaloneWeeks...)

	totPlannedKm := 0.0
	totActualKm := 0.0
	totPlannedLd := 0.0
	totActualLd := 0.0
	totPlannedDur := 0.0
	totActualDur := 0.0
	totSessPlan := 0
	totSessComp := 0

	for _, w := range allWeeks {
		totPlannedKm += w.Actuals.PlannedVolumeKm
		totActualKm += w.Actuals.ActualVolumeKm
		totPlannedLd += w.Actuals.PlannedLoad
		totActualLd += w.Actuals.ActualLoad
		totPlannedDur += w.Actuals.PlannedDurationS
		totActualDur += w.Actuals.ActualDurationS
		totSessPlan += w.Actuals.SessionsPlanned
		totSessComp += w.Actuals.SessionsCompleted
	}
	totPlannedKm = math.Round(totPlannedKm*10) / 10
	totActualKm = math.Round(totActualKm*10) / 10
	totPlannedLd = math.Round(totPlannedLd)
	totActualLd = math.Round(totActualLd)

	var totVolPct *float64
	if totPlannedKm > 0 {
		v := math.Round((totActualKm/totPlannedKm)*1000) / 10
		totVolPct = &v
	}
	var totLdPct *float64
	if totPlannedLd > 0 {
		v := math.Round((totActualLd/totPlannedLd)*1000) / 10
		totLdPct = &v
	}

	totals := CycleActuals{
		PlannedVolumeKm:     totPlannedKm,
		ActualVolumeKm:      totActualKm,
		VolumeCompletionPct: totVolPct,
		PlannedLoad:         totPlannedLd,
		ActualLoad:          totActualLd,
		LoadCompletionPct:   totLdPct,
		PlannedDurationS:    totPlannedDur,
		ActualDurationS:     totActualDur,
		SessionsPlanned:     totSessPlan,
		SessionsCompleted:   totSessComp,
	}

	return &TrainingCycleOverview{
		Plans:           plans,
		ActivePlan:      activePlan,
		Phases:          builtPhases,
		StandaloneWeeks: standaloneWeeks,
		Totals:          totals,
	}, nil
}

// CreatePlan creates a new macrocycle, optionally auto-scaffolding phases and weeks.
func (s *Services) CreatePlan(ctx context.Context, uid int64, in TrainingPlanInput) (*TrainingCycleOverview, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, badRequest("Le nom du macrocycle est requis.")
	}
	if !isValidDate(in.StartDate) || !isValidDate(in.EndDate) || in.EndDate < in.StartDate {
		return nil, badRequest("Macrocycle invalide : nom et dates cohérentes requis.")
	}

	sport := "run"
	if in.Sport != nil && (*in.Sport == "run" || *in.Sport == "bike" || *in.Sport == "other") {
		sport = *in.Sport
	}

	status := PlanStatusActive
	if in.Status != nil && (*in.Status == PlanStatusActive || *in.Status == PlanStatusCompleted || *in.Status == PlanStatusArchived) {
		status = *in.Status
	}

	targetEvent := ""
	if in.TargetEventLabel != nil {
		targetEvent = strings.TrimSpace(*in.TargetEventLabel)
	}

	note := ""
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
	}

	now := s.now().Unix()

	// If active, archive existing active plans
	if status == PlanStatusActive {
		_, _ = s.db().ExecContext(ctx, `UPDATE training_plans SET status = 'archived', updated_at = ? WHERE user_id = ? AND status = 'active'`, now, uid)
	}

	res, err := s.db().ExecContext(ctx, `INSERT INTO training_plans
		(user_id, goal_id, name, sport, start_date, end_date, target_event_label, status, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, in.GoalID, name, sport, in.StartDate, in.EndDate, targetEvent, string(status), note, now, now)
	if err != nil {
		return nil, err
	}

	planID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	// Auto-scaffolding
	autoScaffold := true
	if in.AutoScaffold != nil {
		autoScaffold = *in.AutoScaffold
	}

	if autoScaffold {
		peakKm := 50.0
		if in.PeakWeeklyVolumeKm != nil && *in.PeakWeeklyVolumeKm > 0 {
			peakKm = *in.PeakWeeklyVolumeKm
		}
		peakLoad := 380.0
		if in.PeakWeeklyLoad != nil && *in.PeakWeeklyLoad > 0 {
			peakLoad = *in.PeakWeeklyLoad
		}
		if err := s.scaffoldPhases(ctx, uid, planID, in.StartDate, in.EndDate, peakKm, peakLoad); err != nil {
			return nil, err
		}
	}

	return s.Cycles(ctx, uid, &planID)
}

// PatchPlan modifies a training plan.
func (s *Services) PatchPlan(ctx context.Context, uid, id int64, in TrainingPlanInput) (*TrainingPlan, error) {
	p, err := s.getPlan(ctx, uid, id)
	if err != nil {
		return nil, err
	}

	name := p.Name
	if in.Name != "" {
		name = strings.TrimSpace(in.Name)
		if name == "" {
			return nil, badRequest("Le nom du macrocycle est requis.")
		}
	}

	goalID := p.GoalID
	if in.GoalID != nil {
		goalID = in.GoalID
	}

	sport := p.Sport
	if in.Sport != nil && (*in.Sport == "run" || *in.Sport == "bike" || *in.Sport == "other") {
		sport = *in.Sport
	}

	startDate := p.StartDate
	if in.StartDate != "" {
		if !isValidDate(in.StartDate) {
			return nil, badRequest("Date de début invalide.")
		}
		startDate = in.StartDate
	}

	endDate := p.EndDate
	if in.EndDate != "" {
		if !isValidDate(in.EndDate) {
			return nil, badRequest("Date de fin invalide.")
		}
		endDate = in.EndDate
	}
	if endDate < startDate {
		return nil, badRequest("La date de fin doit être postérieure à la date de début.")
	}

	targetEvent := p.TargetEventLabel
	if in.TargetEventLabel != nil {
		targetEvent = strings.TrimSpace(*in.TargetEventLabel)
	}

	status := p.Status
	now := s.now().Unix()
	if in.Status != nil && (*in.Status == PlanStatusActive || *in.Status == PlanStatusCompleted || *in.Status == PlanStatusArchived) {
		status = *in.Status
		if status == PlanStatusActive {
			_, _ = s.db().ExecContext(ctx, `UPDATE training_plans SET status = 'archived', updated_at = ? WHERE user_id = ? AND id != ? AND status = 'active'`, now, uid, id)
		}
	}

	note := p.Note
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
	}

	_, err = s.db().ExecContext(ctx, `UPDATE training_plans
		SET name = ?, goal_id = ?, sport = ?, start_date = ?, end_date = ?, target_event_label = ?, status = ?, note = ?, updated_at = ?
		WHERE user_id = ? AND id = ?`,
		name, goalID, sport, startDate, endDate, targetEvent, string(status), note, now, uid, id)
	if err != nil {
		return nil, err
	}

	return s.getPlan(ctx, uid, id)
}

// DeletePlan deletes a plan and all cascading phases and weeks.
func (s *Services) DeletePlan(ctx context.Context, uid, id int64) error {
	res, err := s.db().ExecContext(ctx, `DELETE FROM training_plans WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreatePhase creates a mesocycle within a plan.
func (s *Services) CreatePhase(ctx context.Context, uid int64, in TrainingPhaseInput) (*TrainingPhase, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || !isValidDate(in.StartDate) || !isValidDate(in.EndDate) || in.EndDate < in.StartDate {
		return nil, badRequest("Mésocycle invalide : nom et dates cohérentes requis.")
	}

	planID := in.PlanID
	if planID == nil {
		// Default to active plan if one exists
		var apID int64
		err := s.db().QueryRowContext(ctx, `SELECT id FROM training_plans WHERE user_id = ? AND status = 'active' LIMIT 1`, uid).Scan(&apID)
		if err == nil {
			planID = &apID
		}
	}

	phaseType := PhaseBuild
	if in.PhaseType != nil && isValidPhaseType(*in.PhaseType) {
		phaseType = *in.PhaseType
	}

	orderIndex := 0
	if in.OrderIndex != nil {
		orderIndex = *in.OrderIndex
	} else {
		var cnt int
		_ = s.db().QueryRowContext(ctx, `SELECT COUNT(*) FROM plan_phases WHERE user_id = ? AND plan_id IS ?`, uid, planID).Scan(&cnt)
		orderIndex = cnt
	}

	focus := ""
	if in.Focus != nil {
		focus = strings.TrimSpace(*in.Focus)
	}

	note := ""
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
	}

	now := s.now().Unix()
	res, err := s.db().ExecContext(ctx, `INSERT INTO plan_phases
		(user_id, plan_id, name, phase_type, order_index, start_date, end_date, focus, weekly_volume_km, target_weekly_load, target_weekly_duration_s, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, planID, name, string(phaseType), orderIndex, in.StartDate, in.EndDate, focus, in.WeeklyVolumeKm, in.TargetWeeklyLoad, in.TargetWeeklyDurationS, note, now, now)
	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	// Return built phase with weeks and actuals
	overview, err := s.Cycles(ctx, uid, planID)
	if err != nil {
		return nil, err
	}
	for _, ph := range overview.Phases {
		if ph.ID == id {
			return &ph, nil
		}
	}

	return s.getPhase(ctx, uid, id)
}

// PatchPhase modifies a mesocycle.
func (s *Services) PatchPhase(ctx context.Context, uid, id int64, in TrainingPhaseInput) (*TrainingPhase, error) {
	ph, err := s.getPhase(ctx, uid, id)
	if err != nil {
		return nil, err
	}

	name := ph.Name
	if in.Name != "" {
		name = strings.TrimSpace(in.Name)
	}

	startDate := ph.StartDate
	if in.StartDate != "" {
		if !isValidDate(in.StartDate) {
			return nil, badRequest("Date de début invalide.")
		}
		startDate = in.StartDate
	}

	endDate := ph.EndDate
	if in.EndDate != "" {
		if !isValidDate(in.EndDate) {
			return nil, badRequest("Date de fin invalide.")
		}
		endDate = in.EndDate
	}
	if endDate < startDate {
		return nil, badRequest("La date de fin doit être postérieure à la date de début.")
	}

	phaseType := ph.PhaseType
	if in.PhaseType != nil && isValidPhaseType(*in.PhaseType) {
		phaseType = *in.PhaseType
	}

	orderIndex := ph.OrderIndex
	if in.OrderIndex != nil {
		orderIndex = *in.OrderIndex
	}

	focus := ph.Focus
	if in.Focus != nil {
		focus = strings.TrimSpace(*in.Focus)
	}

	weeklyVol := ph.WeeklyVolumeKm
	if in.WeeklyVolumeKm != nil {
		weeklyVol = in.WeeklyVolumeKm
	}

	weeklyLoad := ph.TargetWeeklyLoad
	if in.TargetWeeklyLoad != nil {
		weeklyLoad = in.TargetWeeklyLoad
	}

	weeklyDur := ph.TargetWeeklyDurationS
	if in.TargetWeeklyDurationS != nil {
		weeklyDur = in.TargetWeeklyDurationS
	}

	note := ph.Note
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
	}

	now := s.now().Unix()
	_, err = s.db().ExecContext(ctx, `UPDATE plan_phases
		SET name = ?, phase_type = ?, order_index = ?, start_date = ?, end_date = ?, focus = ?,
		    weekly_volume_km = ?, target_weekly_load = ?, target_weekly_duration_s = ?, note = ?, updated_at = ?
		WHERE user_id = ? AND id = ?`,
		name, string(phaseType), orderIndex, startDate, endDate, focus, weeklyVol, weeklyLoad, weeklyDur, note, now, uid, id)
	if err != nil {
		return nil, err
	}

	overview, err := s.Cycles(ctx, uid, ph.PlanID)
	if err != nil {
		return nil, err
	}
	for _, p := range overview.Phases {
		if p.ID == id {
			return &p, nil
		}
	}
	return s.getPhase(ctx, uid, id)
}

// DeletePhase deletes a mesocycle.
func (s *Services) DeletePhase(ctx context.Context, uid, id int64) error {
	res, err := s.db().ExecContext(ctx, `DELETE FROM plan_phases WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// PutWeek creates or updates a microcycle override (weekly targets, note, assimilation flag).
func (s *Services) PutWeek(ctx context.Context, uid int64, in TrainingWeekInput) (*TrainingWeek, error) {
	if !isValidDate(in.WeekStart) {
		return nil, badRequest("weekStart invalide (format AAAA-MM-JJ attendu).")
	}

	wStart := mondayOf(in.WeekStart)
	wEnd := addDays(wStart, 6)

	isAssim := 0
	if in.IsAssimilation != nil && *in.IsAssimilation {
		isAssim = 1
	}

	note := ""
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
	}

	now := s.now().Unix()

	// Check if record exists
	var existingID int64
	var queryErr error
	if in.PlanID != nil {
		queryErr = s.db().QueryRowContext(ctx, `SELECT id FROM training_weeks WHERE user_id = ? AND plan_id = ? AND week_start = ?`, uid, *in.PlanID, wStart).Scan(&existingID)
	} else {
		queryErr = s.db().QueryRowContext(ctx, `SELECT id FROM training_weeks WHERE user_id = ? AND plan_id IS NULL AND week_start = ?`, uid, wStart).Scan(&existingID)
	}

	if errors.Is(queryErr, sql.ErrNoRows) {
		_, err := s.db().ExecContext(ctx, `INSERT INTO training_weeks
			(user_id, plan_id, phase_id, week_start, week_end, is_assimilation, target_volume_km, target_load, target_duration_s, note, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uid, in.PlanID, in.PhaseID, wStart, wEnd, isAssim, in.TargetVolumeKm, in.TargetLoad, in.TargetDurationS, note, now, now)
		if err != nil {
			return nil, err
		}
	} else if queryErr == nil {
		_, err := s.db().ExecContext(ctx, `UPDATE training_weeks
			SET phase_id = COALESCE(?, phase_id), is_assimilation = ?, target_volume_km = ?, target_load = ?, target_duration_s = ?, note = ?, updated_at = ?
			WHERE id = ?`,
			in.PhaseID, isAssim, in.TargetVolumeKm, in.TargetLoad, in.TargetDurationS, note, now, existingID)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, queryErr
	}

	// Fetch overview to retrieve computed actuals and workouts
	overview, err := s.Cycles(ctx, uid, in.PlanID)
	if err != nil {
		return nil, err
	}

	for _, ph := range overview.Phases {
		for _, w := range ph.Weeks {
			if w.WeekStart == wStart {
				return &w, nil
			}
		}
	}
	for _, w := range overview.StandaloneWeeks {
		if w.WeekStart == wStart {
			return &w, nil
		}
	}

	// Fallback computation
	actuals, workouts, err := s.computeActualsForRange(ctx, uid, wStart, wEnd, in.TargetVolumeKm, in.TargetLoad, in.TargetDurationS)
	if err != nil {
		return nil, err
	}

	return &TrainingWeek{
		ID:              existingID,
		PlanID:          in.PlanID,
		PhaseID:         in.PhaseID,
		WeekStart:       wStart,
		WeekEnd:         wEnd,
		WeekNumber:      1,
		IsAssimilation:  isAssim == 1,
		TargetVolumeKm:  in.TargetVolumeKm,
		TargetLoad:      in.TargetLoad,
		TargetDurationS: in.TargetDurationS,
		Note:            note,
		Actuals:         actuals,
		Workouts:        workouts,
	}, nil
}

func (s *Services) computeActualsForRange(
	ctx context.Context,
	uid int64,
	from, to string,
	targetVolumeKm *float64,
	targetLoad *float64,
	targetDurationS *float64,
) (CycleActuals, []athlete.PlannedSession, error) {
	planned, err := s.repo().Plan(ctx, uid, from, to)
	if err != nil {
		return CycleActuals{}, nil, err
	}

	var nonRestPlanned []athlete.PlannedSession
	sumPlannedKm := 0.0
	sumPlannedDur := 0.0
	for _, p := range planned {
		if p.Sport != "rest" {
			nonRestPlanned = append(nonRestPlanned, p)
			if p.TargetDistanceM != nil {
				sumPlannedKm += *p.TargetDistanceM / 1000.0
			}
			if p.TargetDurationS != nil {
				sumPlannedDur += *p.TargetDurationS
			}
		}
	}
	sumPlannedKm = math.Round(sumPlannedKm*10) / 10

	plannedVol := sumPlannedKm
	if targetVolumeKm != nil {
		plannedVol = *targetVolumeKm
	}

	plannedDur := sumPlannedDur
	if targetDurationS != nil {
		plannedDur = *targetDurationS
	}

	plannedLd := math.Round(plannedVol * 7.5)
	if targetLoad != nil {
		plannedLd = *targetLoad
	}

	acts, err := s.activitiesBetween(ctx, uid, from, to)
	if err != nil {
		return CycleActuals{}, nil, err
	}

	actualKm := 0.0
	actualDur := 0.0
	actualLd := 0.0

	for _, a := range acts {
		if a.isRunning() && a.DistanceM != nil {
			actualKm += *a.DistanceM / 1000.0
		}
		if a.DurationS != nil {
			actualDur += *a.DurationS
		}
		if a.Load != nil {
			actualLd += *a.Load
		} else if a.DurationS != nil {
			actualLd += (*a.DurationS / 60.0) * 1.4
		}
	}
	actualKm = math.Round(actualKm*10) / 10
	actualLd = math.Round(actualLd)

	sessPlanned := len(nonRestPlanned)
	sessCompleted := min(sessPlanned, len(acts))

	var volPct *float64
	if plannedVol > 0 {
		v := math.Round((actualKm/plannedVol)*1000) / 10
		volPct = &v
	}
	var ldPct *float64
	if plannedLd > 0 {
		v := math.Round((actualLd/plannedLd)*1000) / 10
		ldPct = &v
	}

	return CycleActuals{
		PlannedVolumeKm:     plannedVol,
		ActualVolumeKm:      actualKm,
		VolumeCompletionPct: volPct,
		PlannedLoad:         plannedLd,
		ActualLoad:          actualLd,
		LoadCompletionPct:   ldPct,
		PlannedDurationS:    plannedDur,
		ActualDurationS:     actualDur,
		SessionsPlanned:     sessPlanned,
		SessionsCompleted:   sessCompleted,
	}, planned, nil
}

func (s *Services) scaffoldPhases(ctx context.Context, uid, planID int64, start, end string, peakKm, peakLoad float64) error {
	startMon := mondayOf(start)
	endMon := mondayOf(end)
	totalWeeks := max(1, diffDays(startMon, endMon)/7+1)

	type phaseSpec struct {
		name      string
		phaseType PhaseType
		focus     string
		share     float64
		volFactor float64
	}

	var specs []phaseSpec
	if totalWeeks >= 6 {
		specs = []phaseSpec{
			{name: "Fondation aérobie", phaseType: PhaseFoundation, focus: "Endurance fondamentale & régularité", share: 0.34, volFactor: 0.78},
			{name: "Développement & Seuil", phaseType: PhaseBuild, focus: "Volume progressif & tempo contrôlé", share: 0.34, volFactor: 0.92},
			{name: "Spécifique objectif", phaseType: PhaseSpecific, focus: "Allure cible & économie de course", share: 0.22, volFactor: 1.0},
			{name: "Affûtage", phaseType: PhaseTaper, focus: "Fraîcheur neuromusculaire avant course", share: 0.10, volFactor: 0.62},
		}
	} else {
		specs = []phaseSpec{
			{name: "Bloc principal", phaseType: PhaseSpecific, focus: "Travail spécifique objectif", share: 0.75, volFactor: 0.95},
			{name: "Affûtage", phaseType: PhaseTaper, focus: "Assimilation pré-compétition", share: 0.25, volFactor: 0.65},
		}
	}

	cursorWeek := 0
	now := s.now().Unix()

	for idx, sp := range specs {
		remaining := totalWeeks - cursorWeek
		if remaining <= 0 {
			break
		}

		count := remaining
		if idx < len(specs)-1 {
			target := int(math.Round(float64(totalWeeks) * sp.share))
			maxAllowed := remaining - (len(specs) - 1 - idx)
			count = max(1, min(maxAllowed, target))
		}

		pStart := addDays(startMon, cursorWeek*7)
		pEnd := addDays(pStart, count*7-1)
		baseKm := math.Round(peakKm * sp.volFactor)
		baseLoad := math.Round(peakLoad * sp.volFactor)
		baseDur := math.Round(baseKm * 330)

		res, err := s.db().ExecContext(ctx, `INSERT INTO plan_phases
			(user_id, plan_id, name, phase_type, order_index, start_date, end_date, focus, weekly_volume_km, target_weekly_load, target_weekly_duration_s, note, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uid, planID, sp.name, string(sp.phaseType), idx, pStart, pEnd, sp.focus, baseKm, baseLoad, baseDur, "", now, now)
		if err != nil {
			return err
		}
		phID, err := res.LastInsertId()
		if err != nil {
			return err
		}

		for w := 0; w < count; w++ {
			globalWeek := cursorWeek + w + 1
			wStart := addDays(pStart, w*7)
			wEnd := addDays(wStart, 6)
			isAssim := sp.phaseType != PhaseTaper && globalWeek%4 == 0

			factor := 0.94 + math.Min(0.10, float64(w)*0.04)
			note := ""
			if isAssim {
				factor = 0.74
				note = "Semaine d’assimilation (-25 % volume)"
			}

			wkKm := math.Round(baseKm*factor*10) / 10
			wkLoad := math.Round(baseLoad * factor)
			wkDur := math.Round(wkKm * 330)
			isAssimInt := 0
			if isAssim {
				isAssimInt = 1
			}

			_, err = s.db().ExecContext(ctx, `INSERT INTO training_weeks
				(user_id, plan_id, phase_id, week_start, week_end, week_number, is_assimilation, target_volume_km, target_load, target_duration_s, note, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				uid, planID, phID, wStart, wEnd, globalWeek, isAssimInt, wkKm, wkLoad, wkDur, note, now, now)
			if err != nil {
				return err
			}
		}

		cursorWeek += count
	}

	return nil
}

func (s *Services) listPlans(ctx context.Context, uid int64) ([]TrainingPlan, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT id, goal_id, name, sport, start_date, end_date, target_event_label, status, note, created_at, updated_at
		FROM training_plans WHERE user_id = ? ORDER BY start_date DESC, id DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TrainingPlan
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
	row := s.db().QueryRowContext(ctx, `SELECT id, goal_id, name, sport, start_date, end_date, target_event_label, status, note, created_at, updated_at
		FROM training_plans WHERE user_id = ? AND id = ?`, uid, id)
	p, err := scanTrainingPlan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

func (s *Services) listPhasesForPlan(ctx context.Context, uid, planID int64) ([]TrainingPhase, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT id, plan_id, name, phase_type, order_index, start_date, end_date, focus, weekly_volume_km, target_weekly_load, target_weekly_duration_s, note
		FROM plan_phases WHERE user_id = ? AND plan_id = ? ORDER BY order_index, start_date`, uid, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TrainingPhase
	for rows.Next() {
		ph, err := scanTrainingPhase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ph)
	}
	return out, rows.Err()
}

func (s *Services) listPhasesOrphaned(ctx context.Context, uid int64) ([]TrainingPhase, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT id, plan_id, name, phase_type, order_index, start_date, end_date, focus, weekly_volume_km, target_weekly_load, target_weekly_duration_s, note
		FROM plan_phases WHERE user_id = ? AND plan_id IS NULL ORDER BY order_index, start_date`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TrainingPhase
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
	row := s.db().QueryRowContext(ctx, `SELECT id, plan_id, name, phase_type, order_index, start_date, end_date, focus, weekly_volume_km, target_weekly_load, target_weekly_duration_s, note
		FROM plan_phases WHERE user_id = ? AND id = ?`, uid, id)
	ph, err := scanTrainingPhase(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &ph, err
}

func (s *Services) listStoredWeeks(ctx context.Context, uid int64, activePlan *TrainingPlan) ([]storedWeekRow, error) {
	var rows *sql.Rows
	var err error
	if activePlan != nil {
		rows, err = s.db().QueryContext(ctx, `SELECT id, plan_id, phase_id, week_start, week_end, week_number, is_assimilation, target_volume_km, target_load, target_duration_s, note
			FROM training_weeks WHERE user_id = ? AND plan_id = ? ORDER BY week_start`, uid, activePlan.ID)
	} else {
		rows, err = s.db().QueryContext(ctx, `SELECT id, plan_id, phase_id, week_start, week_end, week_number, is_assimilation, target_volume_km, target_load, target_duration_s, note
			FROM training_weeks WHERE user_id = ? AND plan_id IS NULL ORDER BY week_start`, uid)
	}
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
		if pid.Valid {
			w.PlanID = &pid.Int64
		}
		if phid.Valid {
			w.PhaseID = &phid.Int64
		}
		w.IsAssimilation = isAssim != 0
		if vol.Valid {
			w.TargetVolumeKm = &vol.Float64
		}
		if ld.Valid {
			w.TargetLoad = &ld.Float64
		}
		if dur.Valid {
			w.TargetDurationS = &dur.Float64
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func scanTrainingPlan(sc interface{ Scan(...any) error }) (TrainingPlan, error) {
	var p TrainingPlan
	var gid sql.NullInt64
	var status string
	var cat, uat int64
	err := sc.Scan(&p.ID, &gid, &p.Name, &p.Sport, &p.StartDate, &p.EndDate, &p.TargetEventLabel, &status, &p.Note, &cat, &uat)
	if err != nil {
		return p, err
	}
	if gid.Valid {
		p.GoalID = &gid.Int64
	}
	p.Status = CyclePlanStatus(status)
	p.CreatedAt = time.Unix(cat, 0).UTC().Format(time.RFC3339)
	p.UpdatedAt = time.Unix(uat, 0).UTC().Format(time.RFC3339)
	return p, nil
}

func scanTrainingPhase(sc interface{ Scan(...any) error }) (TrainingPhase, error) {
	var ph TrainingPhase
	var pid sql.NullInt64
	var pType string
	var vol, ld, dur sql.NullFloat64
	err := sc.Scan(&ph.ID, &pid, &ph.Name, &pType, &ph.OrderIndex, &ph.StartDate, &ph.EndDate, &ph.Focus, &vol, &ld, &dur, &ph.Note)
	if err != nil {
		return ph, err
	}
	if pid.Valid {
		ph.PlanID = &pid.Int64
	}
	ph.PhaseType = PhaseType(pType)
	if vol.Valid {
		ph.WeeklyVolumeKm = &vol.Float64
	}
	if ld.Valid {
		ph.TargetWeeklyLoad = &ld.Float64
	}
	if dur.Valid {
		ph.TargetWeeklyDurationS = &dur.Float64
	}
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

func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

func formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func mondayOf(dateStr string) string {
	t, err := parseDate(dateStr)
	if err != nil {
		return dateStr
	}
	wd := t.Weekday()
	offset := 0
	if wd == time.Sunday {
		offset = -6
	} else {
		offset = -(int(wd) - 1)
	}
	return formatDate(t.AddDate(0, 0, offset))
}

func addDays(dateStr string, days int) string {
	t, err := parseDate(dateStr)
	if err != nil {
		return dateStr
	}
	return formatDate(t.AddDate(0, 0, days))
}

func diffDays(d1, d2 string) int {
	t1, err1 := parseDate(d1)
	t2, err2 := parseDate(d2)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(math.Round(t2.Sub(t1).Hours() / 24.0))
}
