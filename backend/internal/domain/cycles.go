package domain

import (
	"context"
	"math"

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

// Cycles returns the cycle overview of a plan (requestedPlanID, else the active plan, else the latest).
func (s *Services) Cycles(ctx context.Context, uid int64, requestedPlanID *int64) (*TrainingCycleOverview, error) {
	plans, err := s.listPlans(ctx, uid)
	if err != nil {
		return nil, err
	}
	plan := defaultPlan(plans)
	if requestedPlanID != nil {
		if plan = findPlan(plans, *requestedPlanID); plan == nil {
			return nil, ErrNotFound
		}
	}
	return s.overview(ctx, uid, plans, plan)
}

func findPlan(plans []TrainingPlan, id int64) *TrainingPlan {
	for i := range plans {
		if plans[i].ID == id {
			return &plans[i]
		}
	}
	return nil
}

// overview builds the phases and weeks of plan (nil: those attached to no plan). Week actuals
// come from one read of the activities and plan matching of the whole span.
func (s *Services) overview(ctx context.Context, uid int64, plans []TrainingPlan, plan *TrainingPlan) (*TrainingCycleOverview, error) {
	var planID *int64
	if plan != nil {
		planID = &plan.ID
	}
	phases, err := s.listPhases(ctx, uid, planID)
	if err != nil {
		return nil, err
	}
	stored, err := s.listStoredWeeks(ctx, uid, planID)
	if err != nil {
		return nil, err
	}
	b := cycleBuilder{plan: plan, weeks: map[string]storedWeekRow{}, covered: map[string]bool{}}
	for _, w := range stored {
		b.weeks[w.WeekStart] = w
	}
	for i := range phases {
		phases[i].Weeks = b.phaseWeeks(phases[i])
	}
	var standalone []TrainingWeek
	for _, w := range stored {
		if !b.covered[w.WeekStart] {
			standalone = append(standalone, TrainingWeek{ID: w.ID, PlanID: w.PlanID, PhaseID: w.PhaseID, WeekStart: w.WeekStart,
				WeekEnd: addDays(w.WeekStart, 6), WeekNumber: w.WeekNumber, IsAssimilation: w.IsAssimilation, TargetVolumeKm: w.TargetVolumeKm,
				TargetLoad: w.TargetLoad, TargetDurationS: w.TargetDurationS, Note: w.Note})
		}
	}
	if err := s.fillActuals(ctx, uid, phases, standalone); err != nil {
		return nil, err
	}
	var all []CycleActuals
	for i := range phases {
		var weeks []CycleActuals
		for _, w := range phases[i].Weeks {
			weeks = append(weeks, w.Actuals)
		}
		phases[i].Actuals = sumActuals(weeks)
		all = append(all, weeks...)
	}
	for _, w := range standalone {
		all = append(all, w.Actuals)
	}
	if standalone == nil {
		standalone = []TrainingWeek{}
	}
	return &TrainingCycleOverview{Plans: plans, ActivePlan: plan, Phases: phases, StandaloneWeeks: standalone, Totals: sumActuals(all)}, nil
}

type cycleBuilder struct {
	plan    *TrainingPlan
	weeks   map[string]storedWeekRow // overrides by week start
	covered map[string]bool
}

// phaseWeeks lays out the weeks of a phase; stored overrides replace the phase targets.
func (b *cycleBuilder) phaseWeeks(ph TrainingPhase) []TrainingWeek {
	start := weekStart(ph.StartDate)
	n := max(1, diffDays(start, weekStart(ph.EndDate))/7+1)
	out := make([]TrainingWeek, 0, n)
	for i := range n {
		ws := addDays(start, i*7)
		b.covered[ws] = true
		phaseID := ph.ID
		w := TrainingWeek{ID: 100000 + ph.ID*100 + int64(i), PhaseID: &phaseID, WeekStart: ws, WeekEnd: addDays(ws, 6), WeekNumber: i + 1,
			IsAssimilation: ph.PhaseType == PhaseAssimilation, TargetVolumeKm: ph.WeeklyVolumeKm, TargetLoad: ph.TargetWeeklyLoad,
			TargetDurationS: ph.TargetWeeklyDurationS}
		if b.plan != nil {
			w.PlanID = &b.plan.ID
			w.WeekNumber = max(1, diffDays(weekStart(b.plan.StartDate), ws)/7+1)
		}
		if ov, ok := b.weeks[ws]; ok {
			w.ID, w.IsAssimilation, w.Note = ov.ID, ov.IsAssimilation, ov.Note
			w.TargetVolumeKm = cmpOr(ov.TargetVolumeKm, w.TargetVolumeKm)
			w.TargetLoad = cmpOr(ov.TargetLoad, w.TargetLoad)
			w.TargetDurationS = cmpOr(ov.TargetDurationS, w.TargetDurationS)
		}
		out = append(out, w)
	}
	return out
}

func cmpOr(v, def *float64) *float64 {
	if v != nil {
		return v
	}
	return def
}

// fillActuals computes every week's actuals from one read of the planned sessions, their
// matches and the activities of the whole span.
func (s *Services) fillActuals(ctx context.Context, uid int64, phases []TrainingPhase, standalone []TrainingWeek) error {
	var weeks []*TrainingWeek
	for i := range phases {
		for j := range phases[i].Weeks {
			weeks = append(weeks, &phases[i].Weeks[j])
		}
	}
	for i := range standalone {
		weeks = append(weeks, &standalone[i])
	}
	if len(weeks) == 0 {
		return nil
	}
	from, to := weeks[0].WeekStart, weeks[0].WeekEnd
	for _, w := range weeks {
		from, to = min(from, w.WeekStart), max(to, w.WeekEnd)
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return err
	}
	sessions, err := s.repo().Plan(ctx, uid, from, to)
	if err != nil {
		return err
	}
	acts, err := s.activitiesBetween(ctx, uid, from, to)
	if err != nil {
		return err
	}
	pm := matchSessions(sessions, acts, s.today(p), p.Thresholds)
	planned := map[string][]athlete.PlannedSession{}
	for _, ps := range pm.planned {
		planned[weekStart(ps.Date)] = append(planned[weekStart(ps.Date)], ps)
	}
	done := map[string][]*activityRow{}
	for _, a := range acts {
		done[weekStart(a.LocalDate)] = append(done[weekStart(a.LocalDate)], a)
	}
	for _, w := range weeks {
		w.Workouts = planned[w.WeekStart]
		if w.Workouts == nil {
			w.Workouts = []athlete.PlannedSession{}
		}
		w.Actuals = weekActuals(w.Workouts, done[w.WeekStart], w.TargetVolumeKm, w.TargetLoad, w.TargetDurationS)
	}
	return nil
}

// weekActuals compares a week's targets (else the planned sessions' sums) with what was done.
// Load is the activities' training load; completed sessions are the matched planned ones.
func weekActuals(planned []athlete.PlannedSession, acts []*activityRow, targetKm, targetLoad, targetDur *float64) CycleActuals {
	var out CycleActuals
	for _, ps := range planned {
		if ps.Sport == "rest" {
			continue
		}
		out.SessionsPlanned++
		if ps.TargetDistanceM != nil {
			out.PlannedVolumeKm += *ps.TargetDistanceM / 1000
		}
		if ps.TargetDurationS != nil {
			out.PlannedDurationS += *ps.TargetDurationS
		}
		if ps.Match != nil && ps.Match.ActivityID != nil {
			out.SessionsCompleted++
		}
	}
	if targetKm != nil {
		out.PlannedVolumeKm = *targetKm
	}
	if targetDur != nil {
		out.PlannedDurationS = *targetDur
	}
	if targetLoad != nil {
		out.PlannedLoad = *targetLoad
	}
	for _, a := range acts {
		if a.isRunning() && a.DistanceM != nil {
			out.ActualVolumeKm += *a.DistanceM / 1000
		}
		if a.DurationS != nil {
			out.ActualDurationS += *a.DurationS
		}
		if a.Load != nil {
			out.ActualLoad += *a.Load
		}
	}
	return sumActuals([]CycleActuals{out})
}

// sumActuals adds actuals up, rounds them and derives the completion percentages.
func sumActuals(list []CycleActuals) CycleActuals {
	var t CycleActuals
	for _, a := range list {
		t.PlannedVolumeKm += a.PlannedVolumeKm
		t.ActualVolumeKm += a.ActualVolumeKm
		t.PlannedLoad += a.PlannedLoad
		t.ActualLoad += a.ActualLoad
		t.PlannedDurationS += a.PlannedDurationS
		t.ActualDurationS += a.ActualDurationS
		t.SessionsPlanned += a.SessionsPlanned
		t.SessionsCompleted += a.SessionsCompleted
	}
	t.PlannedVolumeKm, t.ActualVolumeKm = math.Round(t.PlannedVolumeKm*10)/10, math.Round(t.ActualVolumeKm*10)/10
	t.PlannedLoad, t.ActualLoad = math.Round(t.PlannedLoad), math.Round(t.ActualLoad)
	pct := func(actual, planned float64) *float64 {
		if planned <= 0 {
			return nil
		}
		v := math.Round(actual/planned*1000) / 10
		return &v
	}
	t.VolumeCompletionPct, t.LoadCompletionPct = pct(t.ActualVolumeKm, t.PlannedVolumeKm), pct(t.ActualLoad, t.PlannedLoad)
	return t
}
