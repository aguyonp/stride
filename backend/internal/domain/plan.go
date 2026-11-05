package domain

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"stride/internal/analysis"
	"stride/internal/athlete"
	f "stride/internal/frfmt"
	"stride/internal/view"
)

func plannedAccepts(plannedSport string, a *activityRow) bool {
	switch plannedSport {
	case "run":
		return a.Sport == "run"
	case "bike":
		return a.Sport == "bike"
	case "strength":
		return a.Sport == "strength"
	case "mobility":
		return a.Sport == "other" || a.Sport == "strength"
	case "other":
		return a.Sport != "walk"
	}
	return false
}

// countsAsTraining excludes everyday walks from "extra" sessions.
func countsAsTraining(a *activityRow) bool { return a.Sport != "walk" && a.Sport != "hike" }

func weekStart(d string) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	off := (int(t.Weekday()) + 6) % 7
	return t.AddDate(0, 0, -off).Format("2006-01-02")
}

type planMatches struct {
	planned  []athlete.PlannedSession // with Match
	byAct    map[int64]*athlete.PlanMatch
	statuses map[int64]string // activity id → done | partial | extra
}

// matchPlan matches the planned sessions of [from, to] with the activities of the same days.
func (s *Services) matchPlan(ctx context.Context, uid int64, p athlete.Profile, from, to string) (*planMatches, error) {
	planned, err := s.repo().Plan(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	acts, err := s.activitiesBetween(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	return matchSessions(planned, acts, s.today(p), p.Thresholds), nil
}

func matchSessions(planned []athlete.PlannedSession, acts []*activityRow, today string, th athlete.Thresholds) *planMatches {
	out := &planMatches{planned: planned, byAct: map[int64]*athlete.PlanMatch{}, statuses: map[int64]string{}}
	weeks := map[string]bool{}
	for _, ps := range planned {
		weeks[weekStart(ps.Date)] = true
	}
	used := map[int64]bool{}
	byDate := map[string][]*activityRow{}
	for _, a := range acts {
		byDate[a.LocalDate] = append(byDate[a.LocalDate], a)
	}
	for i := range out.planned {
		ps := &out.planned[i]
		m := &athlete.PlanMatch{PlannedID: ps.ID}
		if ps.Sport == "rest" {
			trained := false
			for _, a := range byDate[ps.Date] {
				trained = trained || countsAsTraining(a)
			}
			switch {
			case ps.Date > today:
				m.Status, m.Summary = "upcoming", "Repos prévu"
			case trained:
				m.Status, m.Summary = "partial", "Repos prévu mais activité enregistrée"
			default:
				m.Status, m.Summary = "done", "Repos respecté"
			}
			ps.Match = m
			continue
		}
		var best *activityRow
		for _, a := range byDate[ps.Date] {
			if used[a.ID] || !plannedAccepts(ps.Sport, a) {
				continue
			}
			if best == nil || closer(ps, a, best) {
				best = a
			}
		}
		if best == nil {
			if ps.Date < today {
				m.Status, m.Summary = "missed", "Séance prévue non réalisée"
			} else {
				m.Status, m.Summary = "upcoming", "Séance à venir"
			}
			ps.Match = m
			continue
		}
		used[best.ID] = true
		id := best.ID
		m.ActivityID = &id
		fillMatch(m, ps, best, th)
		ps.Match = m
		out.byAct[best.ID] = m
		out.statuses[best.ID] = m.Status
	}
	for _, a := range acts {
		if _, ok := out.statuses[a.ID]; !ok && weeks[weekStart(a.LocalDate)] && countsAsTraining(a) {
			out.statuses[a.ID] = "extra"
		}
	}
	return out
}

func closer(ps *athlete.PlannedSession, a, b *activityRow) bool {
	if ps.TargetDurationS == nil {
		return analysis.PF(a.DurationS) > analysis.PF(b.DurationS)
	}
	t := *ps.TargetDurationS
	return math.Abs(analysis.PF(a.DurationS)-t) < math.Abs(analysis.PF(b.DurationS)-t)
}

func pct(actual, target *float64) *float64 {
	if actual == nil || target == nil || *target <= 0 {
		return nil
	}
	return analysis.Num((*actual-*target) / *target * 100, 0)
}

func fillMatch(m *athlete.PlanMatch, ps *athlete.PlannedSession, a *activityRow, th athlete.Thresholds) {
	m.DurationDeltaPct = pct(a.DurationS, ps.TargetDurationS)
	m.DistanceDeltaPct = pct(a.DistanceM, ps.TargetDistanceM)
	pace := a.paceSecPerKm()
	var parts []string
	if ps.TargetDurationS != nil && a.DurationS != nil {
		parts = append(parts, fmt.Sprintf("durée %s pour %s prévues (%s %%)", f.Duration(*a.DurationS), f.Duration(*ps.TargetDurationS), f.Signed(analysis.PF(m.DurationDeltaPct), 0)))
	}
	if ps.TargetDistanceM != nil && a.DistanceM != nil {
		parts = append(parts, fmt.Sprintf("distance %s pour %s prévus (%s %%)", f.Km(*a.DistanceM, 2), f.Km(*ps.TargetDistanceM, 2), f.Signed(analysis.PF(m.DistanceDeltaPct), 0)))
	}
	if pace != nil && (ps.TargetPaceMinSecPerKm != nil || ps.TargetPaceMaxSecPerKm != nil) {
		lo, hi := analysis.PF(ps.TargetPaceMinSecPerKm), analysis.PF(ps.TargetPaceMaxSecPerKm)
		if lo != lo {
			lo = hi
		}
		if hi != hi {
			hi = lo
		}
		d := 0.0
		switch {
		case *pace < lo:
			d = *pace - lo
		case *pace > hi:
			d = *pace - hi
		}
		m.PaceDeltaSecPerKm = analysis.Num(d, 0)
		target := f.PaceShort(lo) + "–" + f.PaceShort(hi)
		if lo == hi {
			target = f.PaceShort(lo)
		}
		if d == 0 {
			parts = append(parts, fmt.Sprintf("allure %s dans la cible %s/km", f.Pace(*pace), target))
		} else {
			parts = append(parts, fmt.Sprintf("allure %s hors cible %s/km (%s s/km)", f.Pace(*pace), target, f.Signed(d, 0)))
		}
	}
	if ps.TargetCadenceSpm != nil && a.AvgCadence != nil && a.Sport == "run" {
		m.CadenceDeltaSpm = analysis.Num(*a.AvgCadence-*ps.TargetCadenceSpm, 0)
		parts = append(parts, fmt.Sprintf("cadence %s spm pour %s visés (%s)", f.Int(*a.AvgCadence), f.Int(*ps.TargetCadenceSpm), f.Signed(analysis.PF(m.CadenceDeltaSpm), 0)))
	}
	m.Status = "done"
	for _, d := range []*float64{m.DurationDeltaPct, m.DistanceDeltaPct} {
		if d != nil && *d < -th.PlanDeviationPct {
			m.Status = "partial"
		}
	}
	label := "Réalisée"
	if m.Status == "partial" {
		label = "Partielle"
	}
	if len(parts) == 0 {
		m.Summary = label + ", aucune cible chiffrée"
		return
	}
	m.Summary = label + ", " + strings.Join(parts, ", ")
}

// planStatuses returns the plan status of the activities of [from, to].
func (s *Services) planStatuses(ctx context.Context, uid int64, from, to string) (map[int64]string, error) {
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	pm, err := s.matchPlan(ctx, uid, p, weekStart(from), to)
	if err != nil {
		return nil, err
	}
	return pm.statuses, nil
}

// Plan returns planned sessions of [from, to] with their match.
func (s *Services) Plan(ctx context.Context, uid int64, from, to string) ([]athlete.PlannedSession, error) {
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	if from == "" && to == "" {
		today := s.today(p)
		from, to = analysis.DateAdd(weekStart(today), -28), analysis.DateAdd(today, 60)
	}
	pm, err := s.matchPlan(ctx, uid, p, from, to)
	if err != nil {
		return nil, err
	}
	return pm.planned, nil
}

func (s *Services) plannedWithMatch(ctx context.Context, uid int64, id int64) (athlete.PlannedSession, error) {
	ps, err := s.repo().Planned(ctx, uid, id)
	if err != nil {
		return ps, err
	}
	list, err := s.Plan(ctx, uid, ps.Date, ps.Date)
	if err != nil {
		return ps, err
	}
	for _, x := range list {
		if x.ID == id {
			return x, nil
		}
	}
	return ps, nil
}

// WeekPlan returns the Monday-based week containing date.
func (s *Services) WeekPlan(ctx context.Context, uid int64, date string) (*view.WeekPlan, error) {
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	if date == "" {
		date = s.today(p)
	}
	ws := weekStart(date)
	we := analysis.DateAdd(ws, 6)
	pm, err := s.matchPlan(ctx, uid, p, ws, we)
	if err != nil {
		return nil, err
	}
	acts, err := s.activitiesBetween(ctx, uid, ws, we)
	if err != nil {
		return nil, err
	}
	sums, err := s.summaries(ctx, uid, acts)
	if err != nil {
		return nil, err
	}
	wp := &view.WeekPlan{WeekStart: ws}
	for _, d := range analysis.DateRange(ws, we) {
		day := view.WeekDay{Date: d, Planned: []athlete.PlannedSession{}, Done: []view.ActivitySummary{}}
		for _, ps := range pm.planned {
			if ps.Date != d {
				continue
			}
			day.Planned = append(day.Planned, ps)
			if ps.Sport == "rest" {
				continue
			}
			wp.Totals.SessionsPlanned++
			wp.Totals.PlannedDurationS += analysis.PF0(ps.TargetDurationS)
			wp.Totals.PlannedDistanceM += analysis.PF0(ps.TargetDistanceM)
			if ps.Match != nil && (ps.Match.Status == "done" || ps.Match.Status == "partial") {
				wp.Totals.SessionsDone++
			}
		}
		for i, a := range acts {
			if a.LocalDate != d {
				continue
			}
			day.Done = append(day.Done, sums[i])
			if _, planned := pm.byAct[a.ID]; planned || countsAsTraining(a) {
				wp.Totals.DoneDurationS += analysis.PF0(a.DurationS)
				wp.Totals.DoneDistanceM += analysis.PF0(a.DistanceM)
			}
		}
		wp.Days = append(wp.Days, day)
	}
	wp.Totals.DoneDurationS = math.Round(wp.Totals.DoneDurationS)
	wp.Totals.DoneDistanceM = math.Round(wp.Totals.DoneDistanceM)
	return wp, nil
}

// upcoming returns the next n planned sessions after date (rest days excluded).
func (s *Services) upcoming(ctx context.Context, uid int64, after string, n int) ([]athlete.PlannedSession, error) {
	list, err := s.repo().NextPlanned(ctx, uid, after, n+7)
	if err != nil {
		return nil, err
	}
	out := []athlete.PlannedSession{}
	for _, ps := range list {
		if ps.Sport != "rest" && len(out) < n {
			out = append(out, ps)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}
