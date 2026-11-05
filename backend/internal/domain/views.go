package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"stride/internal/analysis"
	"stride/internal/apimeter"
	"stride/internal/athlete"
	"stride/internal/garmin"
	"stride/internal/view"
)

const analysisTTL = time.Hour

type coreEnvelope struct {
	T    int64          `json:"t"`
	Core *analysis.Core `json:"core"`
}

// core returns the cached analysis of the bundle, computing it on a miss.
func (s *Services) core(ctx context.Context, uid int64, a *activityRow, b *bundle, p athlete.Profile, notesAt string, fr *fresh) *analysis.Core {
	key := userKey(uid, "analysis", fmt.Sprint(a.ID), fmt.Sprintf("v%d", analysis.Version), fmt.Sprint(a.BundleAt), p.Hash(), notesAt)
	if raw, ok := s.cacheGet(ctx, key); ok {
		var env coreEnvelope
		if json.Unmarshal(raw, &env) == nil && env.Core != nil {
			fr.mark(lvCache, time.Unix(env.T, 0))
			switch {
			case len(b.fit) > 0:
				b.sources = append(b.sources, "fit")
				if b.details != nil {
					b.sources = append(b.sources, "details")
				}
			case b.details != nil:
				b.sources = append(b.sources, "details")
			case b.streams != nil:
				b.sources = append(b.sources, "stravaStreams")
			}
			return env.Core
		}
	}
	gConn, _ := s.getConn(ctx, uid, pGarmin)
	in := analysis.Inputs{TypeKey: a.Type, Detail: b.detail, ListItem: b.listItem, Act: b.records(a.isRunning()),
		Thresholds: p.Thresholds, PaceTargets: p.PaceTargets, GarminConnected: gConn.connected() || a.GarminID != nil}
	in.HRZoneFloors = s.hrZoneFloors(ctx, uid, fr)
	c := analysis.Compute(in)
	if raw, err := json.Marshal(coreEnvelope{T: s.now().Unix(), Core: c}); err == nil {
		s.cacheSet(ctx, key, raw, analysisTTL)
	}
	return c
}

func (s *Services) hrZoneFloors(ctx context.Context, uid int64, fr *fresh) []float64 {
	key := userKey(uid, "hrzones")
	if raw, ok := s.cacheGet(ctx, key); ok {
		var z []float64
		if json.Unmarshal(raw, &z) == nil {
			return z
		}
	}
	g, err := s.garminClient(ctx, uid)
	if err != nil {
		return nil
	}
	b, err := g.HRZones(apimeter.WithUser(ctx, uid))
	if err != nil {
		return nil
	}
	fr.mark(lvLive, s.now())
	var zs []garmin.HRZoneConfig
	_ = json.Unmarshal(b, &zs)
	z := analysis.PickZones(zs)
	raw, _ := json.Marshal(z)
	s.cacheSet(ctx, key, raw, 12*time.Hour)
	return z
}

var windDirs = []string{"N", "NE", "E", "SE", "S", "SO", "O", "NO"}

func windDirLabel(deg *float64) *string {
	if deg == nil || math.IsNaN(*deg) {
		return nil
	}
	i := int(math.Round(math.Mod(math.Mod(*deg, 360)+360, 360)/45)) % 8
	return &windDirs[i]
}

func readiness(c *athlete.DailyCheckin) analysis.Readiness {
	if c != nil && (c.TrainingReadiness != nil || c.ReadinessStatus != nil) {
		src := "manual"
		return analysis.Readiness{Score: c.TrainingReadiness, Status: c.ReadinessStatus, Source: &src}
	}
	reason := "Non exposé par l'API personnelle Garmin (saisie manuelle possible)"
	return analysis.Readiness{Reason: &reason}
}

func sourceLabel(a *activityRow) string {
	switch {
	case a.GarminID != nil && a.StravaID != nil:
		return "Fusion"
	case a.StravaID != nil:
		return "Strava"
	}
	return "Garmin"
}

func detailOf(a *activityRow, b *bundle, sm view.ActivitySummary) view.ActivityDetail {
	d := view.ActivityDetail{ActivitySummary: sm}
	if it := b.listItem; it != nil {
		d.LocationName, d.MovingDurationS, d.MaxHR = it.LocationName, analysis.PR(it.MovingDuration, 1), analysis.PR(it.MaxHR, 0)
		d.NormPower, d.MaxCadenceSpm = analysis.PR(it.NormPower, 0), analysis.PR(it.MaxCadenceSpm, 0)
		d.AerobicTE, d.AnaerobicTE, d.TrainingEffectLabel = analysis.PR(it.AerobicTE, 1), analysis.PR(it.AnaerobicTE, 1), it.TELabel
		d.Steps, d.Calories, d.BodyBatteryDelta = analysis.PR(it.Steps, 0), analysis.PR(it.Calories, 0), analysis.PR(it.BodyBatteryDelta, 0)
	}
	if dt := b.detail; dt != nil {
		sm := dt.Summary
		or := func(cur, v *float64, n int) *float64 {
			if v != nil {
				return analysis.PR(v, n)
			}
			return cur
		}
		if dt.LocationName != nil {
			d.LocationName = dt.LocationName
		}
		d.MovingDurationS, d.MaxHR = or(d.MovingDurationS, sm.MovingDuration, 1), or(d.MaxHR, sm.MaxHR, 0)
		d.NormPower, d.MaxCadenceSpm = or(d.NormPower, sm.NormalizedPower, 0), or(d.MaxCadenceSpm, sm.MaxRunCadence, 0)
		d.AerobicTE, d.AnaerobicTE = or(d.AerobicTE, sm.TrainingEffect, 1), or(d.AnaerobicTE, sm.AnaerobicTE, 1)
		if sm.TrainingEffectLabel != nil {
			d.TrainingEffectLabel = sm.TrainingEffectLabel
		}
		d.ImpactLoad = analysis.PR(sm.ImpactLoad, 0)
		d.Steps, d.Calories, d.BodyBatteryDelta = or(d.Steps, sm.Steps, 0), or(d.Calories, sm.Calories, 0), or(d.BodyBatteryDelta, sm.DifferenceBodyBattery, 0)
		d.DistanceM, d.DurationS = or(d.DistanceM, sm.Distance, 1), or(d.DurationS, sm.Duration, 1)
		d.AvgHR, d.AvgPower = or(d.AvgHR, sm.AverageHR, 0), or(d.AvgPower, sm.AveragePower, 0)
		d.TrainingLoad = or(d.TrainingLoad, sm.ActivityTrainingLoad, 1)
		if a.isRunning() {
			d.AvgCadenceSpm = or(d.AvgCadenceSpm, sm.AverageRunCadence, 1)
		}
		if p := analysis.Num(analysis.PaceFromSpeed(analysis.PF(sm.AverageSpeed)), 0); p != nil {
			d.AvgPaceSecPerKm = p
		}
	}
	if st := b.stravaAct; st != nil {
		if d.MaxHR == nil {
			d.MaxHR = analysis.PR(st.MaxHeartrate, 0)
		}
		if d.NormPower == nil {
			d.NormPower = analysis.PR(st.WeightedAverageWatts, 0)
		}
		info := &view.StravaInfo{Title: st.Name, Description: st.Description, PrivateNote: st.PrivateNote,
			RPE: st.PerceivedExertion, RelativeEffort: analysis.PR(st.SufferScore, 0)}
		if st.Gear != nil && st.Gear.Name != "" {
			info.GearName = &st.Gear.Name
		}
		d.Strava = info
	}
	return d
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// loadsBetween returns the per-activity training loads of [from, to].
func loadsOf(rows []*activityRow) ([]analysis.DatedLoad, map[string]float64) {
	var out []analysis.DatedLoad
	byDay := map[string]float64{}
	for _, r := range rows {
		if r.Load != nil {
			out = append(out, analysis.DatedLoad{Date: r.LocalDate, StartLocal: r.StartLocal, Load: *r.Load})
			byDay[r.LocalDate] += *r.Load
		}
	}
	return out, byDay
}

// crossLoad covers J-1 (whole day) and J until `until` (whole day when empty).
func (s *Services) crossLoad(ctx context.Context, uid int64, date, until string, excludeID int64, w *analysis.Wellness, rows []*activityRow) (view.CrossLoad, error) {
	prev := analysis.DateAdd(date, -1)
	cl := view.CrossLoad{WindowStart: prev + "T00:00:00", WindowEnd: until, OtherActivities: []view.ActivitySummary{}}
	if until == "" {
		cl.WindowEnd = date + "T23:59:59"
	}
	type acc struct {
		rides         int
		dist, dur     float64
		hrW, hrSum    float64
		load          float64
		loadKnown     bool
	}
	days := map[string]*acc{prev: {}, date: {}}
	var others []*activityRow
	for _, r := range rows {
		if r.ID == excludeID || r.LocalDate < prev || r.LocalDate > date || (until != "" && r.StartLocal >= until) {
			continue
		}
		if r.Sport != "bike" {
			others = append(others, r)
			continue
		}
		ac := days[r.LocalDate]
		ac.rides++
		ac.dist += analysis.PF0(r.DistanceM)
		ac.dur += analysis.PF0(r.DurationS)
		if r.AvgHR != nil && r.DurationS != nil {
			ac.hrW += *r.DurationS
			ac.hrSum += *r.AvgHR * *r.DurationS
		}
		if r.Load != nil {
			ac.load += *r.Load
			ac.loadKnown = true
		}
	}
	toLoad := func(a *acc) view.BikeLoad {
		bl := view.BikeLoad{Rides: a.rides, DistanceKm: math.Round(a.dist/100) / 10, DurationS: math.Round(a.dur)}
		if a.hrW > 0 {
			bl.AvgHR = analysis.Num(a.hrSum/a.hrW, 0)
		}
		if a.loadKnown {
			bl.TrainingLoad = analysis.Num(a.load, 0)
		}
		return bl
	}
	tot := &acc{}
	for _, d := range []string{prev, date} {
		a := days[d]
		cl.BikeByDay = append(cl.BikeByDay, view.BikeDay{Date: d, BikeLoad: toLoad(a)})
		tot.rides += a.rides
		tot.dist += a.dist
		tot.dur += a.dur
		tot.hrW += a.hrW
		tot.hrSum += a.hrSum
		tot.load += a.load
		tot.loadKnown = tot.loadKnown || a.loadKnown
	}
	cl.Bike = toLoad(tot)
	if w != nil {
		cl.Floors.PrevDayUp, cl.Floors.PrevDayDown = w.FloorsOn(prev)
		cl.Floors.DayUp, cl.Floors.DayDown = w.FloorsOn(date)
		cl.Steps.PrevDay, _ = w.StepsOn(prev)
		cl.Steps.Day, _ = w.StepsOn(date)
		cl.StressPrevDay = w.StressOn(prev)
	}
	sums, err := s.summaries(ctx, uid, others)
	if err != nil {
		return cl, err
	}
	cl.OtherActivities = sums
	return cl, nil
}

// history builds the rows of [from, to] started up to untilLocal (inclusive; "" = no limit).
func (s *Services) history(ctx context.Context, uid int64, rows []*activityRow, from, to, untilLocal string) ([]view.HistoryRow, error) {
	var sel []*activityRow
	ids := []int64{}
	for _, r := range rows {
		if r.LocalDate < from || r.LocalDate > to || (untilLocal != "" && r.StartLocal > untilLocal) {
			continue
		}
		sel = append(sel, r)
		ids = append(ids, r.ID)
	}
	notes, err := s.repo().Notes(ctx, uid, ids)
	if err != nil {
		return nil, err
	}
	shoes, err := s.repo().Shoes(ctx, uid)
	if err != nil {
		return nil, err
	}
	gearName := map[string]string{}
	for _, sh := range shoes {
		if sh.StravaGearID != nil {
			gearName[*sh.StravaGearID] = sh.Name
		}
	}
	out := []view.HistoryRow{}
	for _, r := range sel {
		h := view.HistoryRow{ActivityID: r.ID, Date: r.LocalDate, Type: r.Type, TypeLabel: analysis.TypeLabel(r.Type),
			DistanceM: analysis.PR(r.DistanceM, 1), DurationS: analysis.PR(r.DurationS, 1), PaceSecPerKm: r.paceSecPerKm(),
			AvgCadenceSpm: analysis.PR(r.AvgCadence, 0), GctMs: analysis.PR(r.AvgGCT, 0), ElevLossM: analysis.PR(r.ElevLoss, 0),
			TrainingLoad: analysis.PR(r.Load, 0), IsRunning: r.isRunning()}
		if !h.IsRunning {
			h.PaceSecPerKm, h.AvgCadenceSpm = nil, nil
		}
		if n, ok := notes[r.ID]; ok {
			h.PainDuring, h.PainCold, h.PainNextMorning = n.PainDuring, n.PainCold, n.PainNextMorning
			if n.Shoe != nil {
				h.ShoeName = &n.Shoe.Name
			}
		}
		if h.ShoeName == nil && r.StravaGear != nil {
			if name, ok := gearName[*r.StravaGear]; ok {
				h.ShoeName = &name
			}
		}
		out = append(out, h)
	}
	return out, nil
}

func softFail(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe) || errors.Is(err, ErrNotConnected) || errors.Is(err, context.DeadlineExceeded)
}

// ActivityView assembles GET /api/activities/{id}.
func (s *Services) ActivityView(ctx context.Context, uid, id int64) (*view.ActivityView, error) {
	t0 := time.Now()
	a, err := s.activity(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	fr := &fresh{}
	fr.mark(lvDB, time.Unix(a.UpdatedAt, 0))
	var warnings []string
	if err := s.ensureBundle(ctx, uid, a, fr); err != nil {
		if !softFail(err) {
			return nil, err
		}
		if kinds, _ := s.blobKinds(ctx, uid, a.ID); len(kinds) <= 1 {
			return nil, err
		}
		warnings = append(warnings, "Données de séance partielles ("+errText(err)+")")
	}
	b, err := s.loadBundle(ctx, uid, a)
	if err != nil {
		return nil, err
	}
	r := s.repo()
	notes, err := r.NotesFor(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	notesAt := ""
	if notes.UpdatedAt != nil {
		notesAt = *notes.UpdatedAt
	}
	c := s.core(ctx, uid, a, b, p, notesAt, fr)

	date := a.LocalDate
	rows, err := s.activitiesBetween(ctx, uid, analysis.DateAdd(date, -27), date)
	if err != nil {
		return nil, err
	}
	loads, loadByDay := loadsOf(rows)
	wr := s.wellness(ctx, uid, p, analysis.DateAdd(date, -6), date, []string{analysis.DateAdd(date, -1), date}, fr)
	wr.w.Loads = loadByDay
	checkin, err := r.Checkin(ctx, uid, date)
	if err != nil {
		return nil, err
	}
	rc := analysis.BuildRecovery(wr.w)
	if rc == nil && checkin != nil {
		rc = &analysis.RecoveryContext{Date: date, Trend: analysis.BuildTrend(wr.w)}
	}
	if rc != nil {
		rc.Readiness = readiness(checkin)
	}
	cl, err := s.crossLoad(ctx, uid, date, a.StartLocal, a.ID, wr.w, rows)
	if err != nil {
		return nil, err
	}
	lr := analysis.ComputeLoadRatio(loads, date, a.StartLocal, p.Thresholds.LoadRatioMax)
	if lr == nil {
		warnings = append(warnings, "Ratio de charge indisponible (charge d'entraînement Garmin absente sur 28 jours)")
	}
	pm, err := s.matchPlan(ctx, uid, p, date, date)
	if err != nil {
		return nil, err
	}
	goals, err := r.Goals(ctx, uid, true)
	if err != nil {
		return nil, err
	}
	up, err := s.upcoming(ctx, uid, date, 3)
	if err != nil {
		return nil, err
	}
	hist, err := s.history(ctx, uid, rows, analysis.DateAdd(date, -13), date, a.StartLocal)
	if err != nil {
		return nil, err
	}
	sm := withNotes(a.summary(), &notes)
	if st, ok := pm.statuses[a.ID]; ok {
		sm.PlanStatus = &st
	}
	sm.GapSecPerKm = c.GapSecPerKm
	v := &view.ActivityView{
		GeneratedAt: s.now().In(p.Location()).Format(time.RFC3339), SourceLabel: sourceLabel(a),
		Activity: detailOf(a, b, sm), Biomechanics: c.Biomechanics, Segments: c.Segments, Series: c.Series,
		CrossLoad: cl, Recovery: rc, Notes: notes, Goals: goals, Upcoming: up, History: hist,
	}
	v.Activity.Stamina = c.Stamina
	if b.weather != nil {
		v.Weather = &view.Weather{Conditions: *b.weather, WindDirLabel: windDirLabel(b.weather.WindDirDeg)}
	}
	v.Load = analysis.Load{ElevGainM: c.ElevGainM, ElevLossM: c.ElevLossM, Drift: c.Drift, TrainingLoad: v.Activity.TrainingLoad,
		AerobicTE: v.Activity.AerobicTE, AnaerobicTE: v.Activity.AnaerobicTE, HrZonesS: c.HrZonesS, PowerZonesS: c.PowerZonesS,
		ImpactLoad: v.Activity.ImpactLoad, Stamina: c.Stamina, LoadRatio: lr}
	if v.Activity.Strava != nil {
		v.Load.StravaRelativeEffort = v.Activity.Strava.RelativeEffort
	}
	perf := c.Performance
	perf.ACWR = analysis.ACWRFrom(lr)
	v.Performance = &perf
	var plans []athlete.PlanMatch
	if m := pm.byAct[a.ID]; m != nil {
		v.Plan = m
		plans = append(plans, *m)
	}
	for i := range pm.planned {
		ps := pm.planned[i]
		if ps.Match != nil && ps.Match.ActivityID != nil && *ps.Match.ActivityID == a.ID {
			v.PlannedToday = &ps
		}
	}
	if v.PlannedToday == nil {
		for i := range pm.planned {
			if ps := pm.planned[i]; ps.Sport != "rest" {
				v.PlannedToday = &ps
				break
			}
		}
	}
	v.Alerts = analysis.BuildAlerts(analysis.AlertInput{
		IsRunning: a.isRunning(), Bio: &v.Biomechanics, ElevLossM: analysis.PF(c.ElevLossM), Drift: c.Drift, FadePct: analysis.PF(c.FadePct),
		Recovery: rc, LoadRatio: lr, FloorsPrevDay: analysis.PF(cl.Floors.PrevDayUp), FloorsDay: math.NaN(), Plans: plans,
		PainDuring: analysis.PF(notes.PainDuring), PainNextMorning: analysis.PF(notes.PainNextMorning),
	}, p)
	v.Warnings = uniq(append(append(append(warnings, b.warnings...), c.Warnings...), wr.warnings...))
	srcs := append([]string{}, b.sources...)
	if b.listItem != nil {
		srcs = append(srcs, "activityList")
	}
	if b.weather != nil {
		srcs = append(srcs, "weather")
	}
	v.DataSources = uniq(append(srcs, wr.sources...))
	v.Freshness = fr.view(s.now())
	s.log.Info(fmt.Sprintf("🧮 Vue séance en %s", time.Since(t0).Round(time.Millisecond)), "user", uid, "fraîcheur", v.Freshness.Source,
		"sources", len(v.DataSources), "avertissements", len(v.Warnings))
	return v, nil
}

// DayView assembles GET /api/days/{date}.
func (s *Services) DayView(ctx context.Context, uid int64, date string) (*view.DayView, error) {
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	fr := &fresh{}
	if err := s.firstSyncIfNeeded(ctx, uid, fr); err != nil {
		s.log.Warn("⚠️ Première synchronisation incomplète", "user", uid, "err", errText(err))
	}
	r := s.repo()
	rows, err := s.activitiesBetween(ctx, uid, analysis.DateAdd(date, -27), date)
	if err != nil {
		return nil, err
	}
	for _, x := range rows {
		fr.mark(lvDB, time.Unix(x.UpdatedAt, 0))
	}
	loads, loadByDay := loadsOf(rows)
	wr := s.wellness(ctx, uid, p, analysis.DateAdd(date, -6), date, []string{analysis.DateAdd(date, -1), date}, fr)
	wr.w.Loads = loadByDay
	checkin, err := r.Checkin(ctx, uid, date)
	if err != nil {
		return nil, err
	}
	v := &view.DayView{Date: date, GeneratedAt: s.now().In(p.Location()).Format(time.RFC3339), Day: analysis.BuildDayInfo(wr.w),
		Checkin: checkin, Activities: []view.ActivitySummary{}, Warnings: wr.warnings}
	if v.Warnings == nil {
		v.Warnings = []string{}
	}
	rc := analysis.BuildRecovery(wr.w)
	if rc == nil && checkin != nil {
		rc = &analysis.RecoveryContext{Date: date, Trend: analysis.BuildTrend(wr.w)}
	}
	if rc != nil {
		rc.Readiness = readiness(checkin)
		ids := []int64{}
		for _, x := range rows {
			ids = append(ids, x.ID)
		}
		notes, err := r.Notes(ctx, uid, ids)
		if err != nil {
			return nil, err
		}
		for i := range rc.Trend {
			for _, x := range rows {
				if n, ok := notes[x.ID]; ok && x.LocalDate == rc.Trend[i].Date && n.PainNextMorning != nil &&
					(rc.Trend[i].PainNextMorning == nil || *n.PainNextMorning > *rc.Trend[i].PainNextMorning) {
					rc.Trend[i].PainNextMorning = n.PainNextMorning
				}
			}
		}
	}
	v.Recovery = rc
	var today []*activityRow
	for _, x := range rows {
		if x.LocalDate == date {
			today = append(today, x)
		}
	}
	if v.Activities, err = s.summaries(ctx, uid, today); err != nil {
		return nil, err
	}
	if v.CrossLoad, err = s.crossLoad(ctx, uid, date, "", 0, wr.w, rows); err != nil {
		return nil, err
	}
	pm, err := s.matchPlan(ctx, uid, p, date, date)
	if err != nil {
		return nil, err
	}
	v.Planned = pm.planned
	if v.Goals, err = r.Goals(ctx, uid, true); err != nil {
		return nil, err
	}
	if v.Upcoming, err = s.upcoming(ctx, uid, date, 3); err != nil {
		return nil, err
	}
	if v.History, err = s.history(ctx, uid, rows, analysis.DateAdd(date, -13), date, ""); err != nil {
		return nil, err
	}
	var plans []athlete.PlanMatch
	for _, ps := range pm.planned {
		if ps.Match != nil {
			plans = append(plans, *ps.Match)
		}
	}
	lr := analysis.ComputeLoadRatio(loads, date, "", p.Thresholds.LoadRatioMax)
	v.Alerts = analysis.BuildAlerts(analysis.AlertInput{DayMode: true, Recovery: rc, LoadRatio: lr, FloorsDay: analysis.PF(v.Day.FloorsUp),
		ElevLossM: math.NaN(), FadePct: math.NaN(), FloorsPrevDay: math.NaN(), Plans: plans, PainDuring: math.NaN(), PainNextMorning: math.NaN()}, p)
	v.Freshness = fr.view(s.now())
	return v, nil
}

// ResolveDate turns "today" into the athlete's local date and validates the format.
func (s *Services) ResolveDate(ctx context.Context, uid int64, d string) (string, error) {
	p, err := s.profile(ctx, uid)
	if err != nil {
		return "", err
	}
	today := s.today(p)
	if d == "today" || d == "" {
		return today, nil
	}
	if !athlete.ValidDate(d) {
		return "", badRequest("Date attendue au format AAAA-MM-JJ.")
	}
	if d > today {
		return "", badRequest("La date ne peut pas être dans le futur.")
	}
	return d, nil
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
