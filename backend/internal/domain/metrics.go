package domain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"stride/internal/analysis"
	"stride/internal/athlete"
	"stride/internal/view"
	"stride/internal/workout"
)

const trendWeeksActivity = 8

// metricsFor returns the persisted efficiency / impact metrics of an activity, recomputing them
// when the bundle or the formula version changed. b may be nil.
func (s *Services) metricsFor(ctx context.Context, uid int64, a *activityRow, b *bundle) (analysis.Metrics, error) {
	var m analysis.Metrics
	var ver, at int64
	var data string
	err := s.db().QueryRowContext(ctx, `SELECT version, bundle_at, data FROM activity_metrics WHERE activity_id = ? AND user_id = ?`, a.ID, uid).
		Scan(&ver, &at, &data)
	if err == nil && ver == analysis.MetricsVersion && at == a.BundleAt && json.Unmarshal([]byte(data), &m) == nil {
		return m, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return m, err
	}
	if b == nil {
		if b, err = s.loadBundle(ctx, uid, a); err != nil {
			return m, err
		}
	}
	in := analysis.MetricsInput{Sport: a.Sport, IsRunning: a.isRunning(), Act: b.records(a.isRunning()), DistanceM: a.DistanceM,
		MovingS: a.MovingS, DurationS: a.DurationS, AvgHR: a.AvgHR, AvgPower: a.AvgPower, AvgCadenceSpm: a.AvgCadence, ElevLossM: a.ElevLoss}
	if b.listItem != nil {
		in.AvgVOCm = b.listItem.AvgVO
	}
	if b.detail != nil && b.detail.Summary.VerticalOscillation != nil {
		in.AvgVOCm = b.detail.Summary.VerticalOscillation
	}
	m = analysis.ComputeMetrics(in)
	raw, _ := json.Marshal(m)
	_, err = s.db().ExecContext(ctx, `INSERT INTO activity_metrics (activity_id, user_id, version, bundle_at, data) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (activity_id) DO UPDATE SET version = excluded.version, bundle_at = excluded.bundle_at, data = excluded.data`,
		a.ID, uid, analysis.MetricsVersion, a.BundleAt, string(raw))
	return m, err
}

// structuredPlans returns the activities matched to an interval-structured planned session.
func structuredPlans(pm *planMatches) map[int64]bool {
	out := map[int64]bool{}
	for _, ps := range pm.planned {
		if ps.Match != nil && ps.Match.ActivityID != nil && workout.Structured(ps.Steps) {
			out[*ps.Match.ActivityID] = true
		}
	}
	return out
}

func withPlanStructure(ef analysis.Efficiency, structured bool) analysis.Efficiency {
	if structured && ef.Eligible {
		r := "Séance prévue fractionnée : efficience non comparable"
		ef = analysis.Efficiency{Reason: &r}
	}
	return ef
}

type efPoint struct {
	row *activityRow
	m   analysis.Metrics
}

// efficiencyPoints lists the eligible activities of sport between from and to (oldest first).
func (s *Services) efficiencyPoints(ctx context.Context, uid int64, p athlete.Profile, sport, from, to string) ([]efPoint, error) {
	rows, err := s.queryActivities(ctx, `WHERE user_id = ? AND local_date >= ? AND local_date <= ? AND sport = ? ORDER BY start_utc`, uid, from, to, sport)
	if err != nil {
		return nil, err
	}
	pm, err := s.matchPlan(ctx, uid, p, from, to)
	if err != nil {
		return nil, err
	}
	structured := structuredPlans(pm)
	out := []efPoint{}
	for _, a := range rows {
		m, err := s.metricsFor(ctx, uid, a, nil)
		if err != nil {
			return nil, err
		}
		m.Efficiency = withPlanStructure(m.Efficiency, structured[a.ID])
		if m.Efficiency.Eligible && m.Efficiency.EfSpeed != nil {
			out = append(out, efPoint{a, m})
		}
	}
	return out, nil
}

func changeOf(points []efPoint, from, to string) *float64 {
	dates := make([]string, len(points))
	vals := make([]float64, len(points))
	for i, pt := range points {
		dates[i], vals[i] = pt.row.LocalDate, *pt.m.Efficiency.EfSpeed
	}
	return analysis.EfficiencyChangePct(dates, vals, from, to)
}

// activityMetrics completes the persisted metrics of a with the 8-week EF trend and the 7-day
// joint index.
func (s *Services) activityMetrics(ctx context.Context, uid int64, a *activityRow, b *bundle, p athlete.Profile, structured bool) (*analysis.Efficiency, *analysis.Impact, error) {
	m, err := s.metricsFor(ctx, uid, a, b)
	if err != nil {
		return nil, nil, err
	}
	ef := withPlanStructure(m.Efficiency, structured)
	if ef.Eligible {
		from := analysis.DateAdd(a.LocalDate, -7*trendWeeksActivity+1)
		pts, err := s.efficiencyPoints(ctx, uid, p, a.Sport, from, a.LocalDate)
		if err != nil {
			return nil, nil, err
		}
		ef.TrendChangePct = changeOf(pts, from, a.LocalDate)
	}
	imp := m.Impact
	if imp != nil && imp.JointIndex != nil {
		rows, err := s.queryActivities(ctx, `WHERE user_id = ? AND local_date >= ? AND start_utc <= ? ORDER BY start_utc`,
			uid, analysis.DateAdd(a.LocalDate, -6), a.StartUTC)
		if err != nil {
			return nil, nil, err
		}
		sum := 0.0
		for _, r := range rows {
			if !r.isRunning() {
				continue
			}
			rm := m
			if r.ID != a.ID {
				if rm, err = s.metricsFor(ctx, uid, r, nil); err != nil {
					return nil, nil, err
				}
			}
			if rm.Impact != nil && rm.Impact.JointIndex != nil {
				sum += *rm.Impact.JointIndex
			}
		}
		cp := *imp
		cp.JointIndex7d = analysis.Num(sum, 1)
		imp = &cp
	}
	return &ef, imp, nil
}

// EfficiencyTrend serves GET /api/trends/efficiency (running sessions).
func (s *Services) EfficiencyTrend(ctx context.Context, uid int64, weeks int) (*view.EfficiencyTrend, error) {
	if weeks <= 0 {
		weeks = 12
	}
	if weeks < 2 || weeks > 52 {
		return nil, badRequest("weeks doit être compris entre 2 et 52.")
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	to := s.today(p)
	from := analysis.DateAdd(to, -7*weeks+1)
	pts, err := s.efficiencyPoints(ctx, uid, p, "run", from, to)
	if err != nil {
		return nil, err
	}
	out := &view.EfficiencyTrend{Points: []view.EfficiencyPoint{}, RollingEfSpeed: []*float64{}, ChangePct: changeOf(pts, from, to)}
	for i, pt := range pts {
		out.Points = append(out.Points, view.EfficiencyPoint{Date: pt.row.LocalDate, ActivityID: pt.row.ID, EfSpeed: pt.m.Efficiency.EfSpeed,
			EfPower: pt.m.Efficiency.EfPower, AvgHR: pt.m.AvgHR, PaceSecPerKm: pt.m.PaceSecPerKm})
		lo := analysis.DateAdd(pt.row.LocalDate, -27)
		var vals []float64
		for j := 0; j <= i; j++ {
			if pts[j].row.LocalDate >= lo {
				vals = append(vals, *pts[j].m.Efficiency.EfSpeed)
			}
		}
		var roll *float64
		if len(vals) >= 2 {
			sum := 0.0
			for _, v := range vals {
				sum += v
			}
			roll = analysis.Num(sum/float64(len(vals)), 3)
		}
		out.RollingEfSpeed = append(out.RollingEfSpeed, roll)
	}
	return out, nil
}
