package domain

import (
	"context"
	"math"
	"sort"
	"time"

	"stride/internal/analysis"
	"stride/internal/report"
	"stride/internal/view"
)

const (
	maxExportWeeks    = 26
	maxExportSessions = 60
	maxExportFetches  = 10
)

type meanAcc struct{ sum, w float64 }

func (m *meanAcc) add(v *float64, w float64) {
	if v != nil && w > 0 && !math.IsNaN(*v) {
		m.sum += *v * w
		m.w += w
	}
}

func (m *meanAcc) mean(n int) *float64 {
	if m.w <= 0 {
		return nil
	}
	return analysis.Num(m.sum/m.w, n)
}

// ActivityReport renders §6.2 for an activity.
func (s *Services) ActivityReport(ctx context.Context, uid, id int64, include string) (string, error) {
	inc, err := report.ParseInclude(include)
	if err != nil {
		return "", badRequest("%s", err.Error())
	}
	v, err := s.ActivityView(ctx, uid, id)
	if err != nil {
		return "", err
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return "", err
	}
	if v.Records, err = s.records(ctx, uid); err != nil {
		return "", err
	}
	if v.BodyStress, v.BodyCorrelations, err = s.bodyPromptContext(ctx, uid); err != nil {
		return "", err
	}
	if v.Health, err = s.trackedZones(ctx, uid); err != nil {
		return "", err
	}
	return report.Activity(v, p, inc), nil
}

// DayReport renders §6.3.
func (s *Services) DayReport(ctx context.Context, uid int64, date, include string) (string, error) {
	inc, err := report.ParseInclude(include)
	if err != nil {
		return "", badRequest("%s", err.Error())
	}
	v, err := s.DayView(ctx, uid, date)
	if err != nil {
		return "", err
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return "", err
	}
	if v.Records, err = s.records(ctx, uid); err != nil {
		return "", err
	}
	if v.BodyStress, v.BodyCorrelations, err = s.bodyPromptContext(ctx, uid); err != nil {
		return "", err
	}
	if v.Health, err = s.trackedZones(ctx, uid); err != nil {
		return "", err
	}
	return report.Day(v, p, inc), nil
}

// MultiExport renders §6.4 for explicit activities or the last N weeks of running.
func (s *Services) MultiExport(ctx context.Context, uid int64, ids []int64, weeks int, include []string) (*view.MultiExportResult, error) {
	inc, err := report.ParseIncludeList(include)
	if err != nil {
		return nil, badRequest("%s", err.Error())
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	today := s.today(p)
	var rows []*activityRow
	var from, to string
	if len(ids) > 0 {
		if len(ids) > maxExportSessions {
			return nil, badRequest("%d séances maximum par export.", maxExportSessions)
		}
		seen := map[int64]bool{}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			a, err := s.activity(ctx, uid, id)
			if err != nil {
				return nil, err
			}
			rows = append(rows, a)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].StartLocal < rows[j].StartLocal })
		from, to = weekStart(rows[0].LocalDate), rows[len(rows)-1].LocalDate
		weeks = int(math.Ceil(float64(daysBetween(from, to)+1) / 7))
	} else {
		if weeks <= 0 {
			weeks = 4
		}
		if weeks > maxExportWeeks {
			return nil, badRequest("%d semaines maximum.", maxExportWeeks)
		}
		from, to = analysis.DateAdd(weekStart(today), -7*(weeks-1)), today
		all, err := s.activitiesBetween(ctx, uid, from, to)
		if err != nil {
			return nil, err
		}
		for _, a := range all {
			if a.isRunning() {
				rows = append(rows, a)
			}
		}
		if len(rows) > maxExportSessions {
			rows = rows[len(rows)-maxExportSessions:]
		}
	}
	fr := &fresh{}
	if err := s.ensureListCoverage(ctx, uid, analysis.DateAdd(from, -28), fr); err != nil {
		s.log.Warn("⚠️ Historique Garmin incomplet pour l'export", "user", uid, "err", errText(err))
	}
	ctxRows, err := s.activitiesBetween(ctx, uid, analysis.DateAdd(from, -28), to)
	if err != nil {
		return nil, err
	}
	loads, _ := loadsOf(ctxRows)
	ids = ids[:0]
	for _, a := range rows {
		ids = append(ids, a.ID)
	}
	r := s.repo()
	notes, err := r.Notes(ctx, uid, ids)
	if err != nil {
		return nil, err
	}
	sums, err := s.summaries(ctx, uid, rows)
	if err != nil {
		return nil, err
	}
	m := &view.MultiExport{From: from, To: to, Weeks: weeks, NextWeeks: 4}
	pm, err := s.matchPlan(ctx, uid, p, from, to)
	if err != nil {
		return nil, err
	}
	structured := structuredPlans(pm)
	alertCount := map[string]*view.AlertCount{}
	fetches := 0
	type perWeek struct {
		n          int
		dist, dur  float64
		cad, gct   meanAcc
		pd, pc, pn meanAcc
		ef         meanAcc
		joint      float64
		jointKnown bool
	}
	wk := map[string]*perWeek{}
	for i, a := range rows {
		ms := view.MultiSession{Summary: sums[i], GctMs: analysis.PR(a.AvgGCT, 0)}
		n := notes[a.ID]
		ms.PainDuring, ms.PainCold, ms.PainNextMorning = n.PainDuring, n.PainCold, n.PainNextMorning
		if n.Shoe != nil {
			ms.ShoeName = &n.Shoe.Name
		}
		if !a.Final && fetches < maxExportFetches {
			fetches++
			if err := s.ensureBundle(ctx, uid, a, fr); err != nil {
				s.log.Warn("⚠️ Séance non analysée pour l'export", "user", uid, "err", errText(err))
			}
		}
		var mb *bundle
		if kinds, _ := s.blobKinds(ctx, uid, a.ID); len(kinds) > 1 {
			b, err := s.loadBundle(ctx, uid, a)
			if err != nil {
				return nil, err
			}
			mb = b
			notesAt := ""
			if n.UpdatedAt != nil {
				notesAt = *n.UpdatedAt
			}
			c := s.core(ctx, uid, a, b, p, notesAt, fr)
			if ms.GctMs == nil {
				ms.GctMs = c.Biomechanics.GroundContactTimeMs
			}
			if fd := c.Biomechanics.FatigueDrift; fd != nil {
				ms.GctDriftPct = fd.GctDeltaPct
			}
			if c.GapSecPerKm != nil {
				ms.Summary.GapSecPerKm = c.GapSecPerKm
			}
			lr := analysis.ComputeLoadRatio(loads, a.LocalDate, a.StartLocal, p.Thresholds.LoadRatioMax)
			for _, al := range analysis.BuildAlerts(analysis.AlertInput{IsRunning: true, Bio: &c.Biomechanics, ElevLossM: analysis.PF(c.ElevLossM),
				Drift: c.Drift, FadePct: analysis.PF(c.FadePct), LoadRatio: lr, FloorsPrevDay: math.NaN(), FloorsDay: math.NaN(),
				PainDuring: analysis.PF(n.PainDuring), PainNextMorning: analysis.PF(n.PainNextMorning)}, p) {
				if al.Level == "ok" || al.Code == "GCT_BALANCE_NA" {
					continue
				}
				key := al.Code + "|" + al.Title
				if ac, ok := alertCount[key]; ok {
					ac.Count++
					if levelRank(al.Level) < levelRank(ac.Alert.Level) {
						ac.Alert = al
					}
				} else {
					alertCount[key] = &view.AlertCount{Alert: al, Count: 1}
				}
			}
		} else {
			m.Unanalyzed++
		}
		m.Sessions = append(m.Sessions, ms)
		w := wk[weekStart(a.LocalDate)]
		if w == nil {
			w = &perWeek{}
			wk[weekStart(a.LocalDate)] = w
		}
		if mt, err := s.metricsFor(ctx, uid, a, mb); err == nil {
			if ef := withPlanStructure(mt.Efficiency, structured[a.ID]); ef.Eligible {
				w.ef.add(ef.EfSpeed, 1)
			}
			if mt.Impact != nil && mt.Impact.JointIndex != nil {
				w.joint += *mt.Impact.JointIndex
				w.jointKnown = true
			}
		}
		d := analysis.PF0(a.DurationS)
		w.n++
		w.dist += analysis.PF0(a.DistanceM)
		w.dur += d
		w.cad.add(a.AvgCadence, d)
		w.gct.add(ms.GctMs, d)
		w.pd.add(n.PainDuring, 1)
		w.pc.add(n.PainCold, 1)
		w.pn.add(n.PainNextMorning, 1)
	}
	for _, ac := range alertCount {
		m.Alerts = append(m.Alerts, *ac)
	}
	sort.Slice(m.Alerts, func(i, j int) bool {
		if m.Alerts[i].Count != m.Alerts[j].Count {
			return m.Alerts[i].Count > m.Alerts[j].Count
		}
		return m.Alerts[i].Alert.Code < m.Alerts[j].Alert.Code
	})

	wr := s.wellness(ctx, uid, p, from, to, nil, fr)
	for ws := weekStart(from); ws <= to; ws = analysis.DateAdd(ws, 7) {
		we := analysis.DateAdd(ws, 6)
		if we > to {
			we = to
		}
		t := view.WeekTrend{WeekStart: ws}
		if w := wk[ws]; w != nil {
			t.Sessions, t.DistanceM, t.DurationS = w.n, math.Round(w.dist), math.Round(w.dur)
			t.CadenceAvg, t.GctAvg = w.cad.mean(0), w.gct.mean(0)
			t.EfAvg = w.ef.mean(2)
			if w.jointKnown {
				t.JointIndex = analysis.Num(w.joint, 1)
			}
			t.PainDuring, t.PainCold, t.PainNextMorning = w.pd.mean(1), w.pc.mean(1), w.pn.mean(1)
		}
		load, known := 0.0, false
		for _, x := range ctxRows {
			if x.LocalDate >= ws && x.LocalDate <= we && x.Load != nil {
				load += *x.Load
				known = true
			}
		}
		if known {
			t.Load = analysis.Num(load, 0)
		}
		if lr := analysis.ComputeLoadRatio(loads, we, "", p.Thresholds.LoadRatioMax); lr != nil {
			t.LoadRatio = lr.Ratio
		}
		var hrv, sleep meanAcc
		for _, d := range analysis.DateRange(ws, we) {
			if h := wr.w.HRV[d]; h != nil {
				hrv.add(h.LastNightAvg, 1)
			}
			if sl := wr.w.Sleep[d]; sl != nil {
				sc, _ := sl.Score()
				sleep.add(sc, 1)
			}
		}
		t.HrvAvg, t.SleepScoreAvg = hrv.mean(0), sleep.mean(0)
		m.Weekly = append(m.Weekly, t)
	}

	var plannedDist, doneDist float64
	for _, ps := range pm.planned {
		if ps.Sport == "rest" || ps.Match == nil {
			continue
		}
		m.Plan.Planned++
		switch ps.Match.Status {
		case "done":
			m.Plan.Done++
		case "partial":
			m.Plan.Partial++
		case "missed":
			m.Plan.Missed++
		}
		if ps.TargetDistanceM != nil {
			plannedDist += *ps.TargetDistanceM
			if ps.Match.ActivityID != nil {
				for _, x := range ctxRows {
					if x.ID == *ps.Match.ActivityID {
						doneDist += analysis.PF0(x.DistanceM)
					}
				}
			}
		}
	}
	if plannedDist > 0 {
		m.Plan.VolumeDeltaPct = analysis.Num((doneDist-plannedDist)/plannedDist*100, 0)
	}
	if m.Goals, err = r.Goals(ctx, uid, true); err != nil {
		return nil, err
	}
	if m.Upcoming, err = s.upcoming(ctx, uid, today, 3); err != nil {
		return nil, err
	}
	if m.Records, err = s.records(ctx, uid); err != nil {
		return nil, err
	}
	if m.BodyStress, m.BodyCorrelations, err = s.bodyPromptContext(ctx, uid); err != nil {
		return nil, err
	}
	if m.Health, err = s.trackedZones(ctx, uid); err != nil {
		return nil, err
	}
	md := report.Multi(m, p, inc)
	return &view.MultiExportResult{Markdown: md, TokensEstimate: report.EstimateTokens(md), Sessions: len(m.Sessions)}, nil
}

func levelRank(l string) int {
	switch l {
	case "danger":
		return 0
	case "warn":
		return 1
	case "info":
		return 2
	}
	return 3
}

func daysBetween(a, b string) int {
	ta, err1 := time.Parse("2006-01-02", a)
	tb, err2 := time.Parse("2006-01-02", b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(tb.Sub(ta).Hours() / 24)
}

// ---------- admin ----------

func (s *Services) AdminConnections(ctx context.Context) ([]view.AdminConnection, error) {
	rows, err := s.db().QueryContext(ctx, `SELECT user_id, provider, status, COALESCE(last_sync_at, 0), last_error, COALESCE(last_error_at, 0)
		FROM provider_connections ORDER BY user_id, provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []view.AdminConnection{}
	for rows.Next() {
		var c view.AdminConnection
		var ls, le int64
		var lerr *string
		if err := rows.Scan(&c.UserID, &c.Provider, &c.Status, &ls, &lerr, &le); err != nil {
			return nil, err
		}
		c.LastSyncAt, c.LastError, c.LastErrorAt = rfc3339(ls), lerr, rfc3339(le)
		out = append(out, c)
	}
	return out, rows.Err()
}

// UserExists tells whether a user id exists (admin routes on other users).
func (s *Services) UserExists(ctx context.Context, uid int64) (bool, error) {
	var n int
	err := s.db().QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = ?`, uid).Scan(&n)
	return n > 0, err
}
