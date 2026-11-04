package analysis

import (
	"math"
	"time"

	"patella/internal/garmin"
)

// wellness regroupe les données bien-être récupérées pour une date J.
type wellness struct {
	date     string
	sleep    *garmin.DailySleep
	hrvDay   *garmin.HRVSummary
	hrvRange []garmin.HRVSummary
	bb       []garmin.BodyBatteryDay
	dhr      *garmin.DailyHeartRate
	sums     []garmin.DailySummary
	steps    []garmin.StepsDay
	sleeps   []garmin.DailySleep
	acts     []garmin.ActivityListItem
	actsOK   bool
	zones    []garmin.HRZoneConfig
}

func (w *wellness) summary(date string) *garmin.DailySummary {
	for i := range w.sums {
		if w.sums[i].CalendarDate == date {
			return &w.sums[i]
		}
	}
	return nil
}

func (w *wellness) hrvFor(date string) *garmin.HRVSummary {
	if date == w.date && w.hrvDay != nil {
		return w.hrvDay
	}
	for i := range w.hrvRange {
		if w.hrvRange[i].CalendarDate == date {
			return &w.hrvRange[i]
		}
	}
	return nil
}

func (w *wellness) sleepFor(date string) *garmin.DailySleep {
	if date == w.date && w.sleep.HasData() {
		return w.sleep
	}
	for i := range w.sleeps {
		if w.sleeps[i].CalendarDate == date && w.sleeps[i].HasData() {
			return &w.sleeps[i]
		}
	}
	return nil
}

func (w *wellness) stepsFor(date string) *garmin.StepsDay {
	for i := range w.steps {
		if w.steps[i].CalendarDate == date {
			return &w.steps[i]
		}
	}
	return nil
}

func (w *wellness) bbDay(date string) *garmin.BodyBatteryDay {
	for i := range w.bb {
		if w.bb[i].Date == date {
			return &w.bb[i]
		}
	}
	return nil
}

var hrvStatusLabels = map[string]string{
	"BALANCED":   "Équilibré",
	"UNBALANCED": "Déséquilibré",
	"LOW":        "Bas",
	"POOR":       "Faible",
}

// HRVStatusLabel traduit un statut VFC Garmin.
func HRVStatusLabel(s string) string {
	if l, ok := hrvStatusLabels[s]; ok {
		return l
	}
	return s
}

// bodyBatteryAtWake : point le plus proche du réveil (sleepEndTimestampGMT), à ±3 h max.
// Résolution réelle ~6 points/jour (sonde 2026-11-03 : le point le plus proche est à
// < 45 min du réveil et correspond au maximum du jour).
func bodyBatteryAtWake(days []garmin.BodyBatteryDay, wakeMs float64) float64 {
	if !valid(wakeMs) {
		return nan
	}
	best, bestD := nan, math.Inf(1)
	for i := range days {
		for _, p := range days[i].Points() {
			d := math.Abs(p.TsMs - wakeMs)
			if d < bestD {
				best, bestD = p.Level, d
			}
		}
	}
	if bestD > float64(3*time.Hour/time.Millisecond) {
		return nan
	}
	return best
}

// fmtLocalClock formate un horodatage "local" Garmin (ms epoch décalé) en "HH:MM".
func fmtLocalClock(ms *float64) *string {
	if ms == nil || *ms <= 0 {
		return nil
	}
	s := time.UnixMilli(int64(*ms)).UTC().Format("15:04")
	return &s
}

// buildRecovery construit le contexte de récupération (nil si aucune donnée).
func buildRecovery(w *wellness) *RecoveryContext {
	rc := &RecoveryContext{Date: w.date, Trend: []TrendDay{}}
	any := false

	if h := w.hrvFor(w.date); h != nil && (h.LastNightAvg != nil || h.Status != nil) {
		hc := &HRVContext{
			LastNightAvgMs:      pr(h.LastNightAvg, 0),
			LastNight5MinHighMs: pr(h.LastNight5MinHigh, 0),
			WeeklyAvgMs:         pr(h.WeeklyAvg, 0),
			Status:              h.Status,
		}
		if h.Baseline != nil {
			hc.BaselineLowMs = pr(h.Baseline.BalancedLow, 0)
			hc.BaselineHighMs = pr(h.Baseline.BalancedUpper, 0)
		}
		if h.Status != nil {
			hc.StatusLabel = strp(HRVStatusLabel(*h.Status))
		}
		if ln, wk := pf(h.LastNightAvg), pf(h.WeeklyAvg); valid(ln) && valid(wk) && wk > 0 {
			hc.DeltaVsWeeklyPct = num((ln-wk)/wk*100, 1)
		}
		rc.HRV = hc
		any = true
	}

	sl := w.sleepFor(w.date)
	if sl != nil {
		score, qual := sl.Score()
		rc.Sleep = &SleepContext{
			Score:        pr(score, 0),
			Qualifier:    qual,
			DurationS:    pr(sl.SleepTimeSeconds, 0),
			DeepS:        pr(sl.DeepSleepSeconds, 0),
			RemS:         pr(sl.RemSleepSeconds, 0),
			LightS:       pr(sl.LightSleepSeconds, 0),
			AwakeS:       pr(sl.AwakeSleepSeconds, 0),
			AvgSleepHR:   pr(sl.AvgHeartRate, 0),
			BedtimeLocal: fmtLocalClock(sl.SleepStartTimestampLocal),
			WakeLocal:    fmtLocalClock(sl.SleepEndTimestampLocal),
		}
		any = true
	}

	sum := w.summary(w.date)
	rhr, avg7 := nan, nan
	if w.dhr != nil {
		rhr, avg7 = pf(w.dhr.RestingHeartRate), pf(w.dhr.LastSevenDaysAvgRHR)
	}
	if !valid(rhr) && sum != nil && sum.HeartRate != nil {
		rhr = pf(sum.HeartRate.RestingValue)
	}
	if !valid(avg7) {
		var a wacc
		for _, d := range dateRange(dateAdd(w.date, -6), w.date) {
			if s := w.summary(d); s != nil && s.HeartRate != nil {
				a.add(pf(s.HeartRate.RestingValue), 1)
			}
		}
		avg7 = a.mean()
	}
	if valid(rhr) || valid(avg7) {
		r := &RestingHR{Bpm: num(rhr, 0), Avg7d: num(avg7, 0)}
		if valid(rhr) && valid(avg7) {
			r.Delta = num(math.Round(rhr)-math.Round(avg7), 0)
		}
		rc.RestingHR = r
		any = true
	}

	bbd := w.bbDay(w.date)
	bb := &BodyBattery{}
	if sl != nil {
		bb.AtWake = num(bodyBatteryAtWake(w.bb, pf(sl.SleepEndTimestampGMT)), 0)
	}
	if sum != nil && sum.BodyBattery != nil {
		bb.Max = pr(sum.BodyBattery.MaxValue, 0)
		bb.Min = pr(sum.BodyBattery.MinValue, 0)
		bb.Charged = pr(sum.BodyBattery.ChargedValue, 0)
		bb.Drained = pr(sum.BodyBattery.DrainedValue, 0)
	}
	if bbd != nil {
		if bbd.Charged != nil {
			bb.Charged = pr(bbd.Charged, 0)
		}
		if bbd.Drained != nil {
			bb.Drained = pr(bbd.Drained, 0)
		}
		if bb.Max == nil || bb.Min == nil {
			mx, mn := nan, nan
			for _, p := range bbd.Points() {
				if !valid(mx) || p.Level > mx {
					mx = p.Level
				}
				if !valid(mn) || p.Level < mn {
					mn = p.Level
				}
			}
			if bb.Max == nil {
				bb.Max = num(mx, 0)
			}
			if bb.Min == nil {
				bb.Min = num(mn, 0)
			}
		}
	}
	if bb.AtWake != nil || bb.Max != nil || bb.Charged != nil || bb.Drained != nil {
		rc.BodyBattery = bb
		any = true
	}
	if sum != nil && sum.Stress != nil {
		rc.StressAvg = pr(sum.Stress.AvgLevel, 0)
		if rc.StressAvg != nil && *rc.StressAvg < 0 {
			rc.StressAvg = nil // -1/-2 = pas assez de données
		}
		if rc.StressAvg != nil {
			any = true
		}
	}

	rc.Trend = buildTrend(w)
	for _, t := range rc.Trend {
		if t.HrvMs != nil || t.SleepS != nil || t.RestingHR != nil || t.Steps != nil {
			any = true
			break
		}
	}
	if !any {
		return nil
	}
	return rc
}

// buildTrend : tendance J-6..J (ordre chronologique).
func buildTrend(w *wellness) []TrendDay {
	out := []TrendDay{}
	for _, d := range dateRange(dateAdd(w.date, -6), w.date) {
		td := TrendDay{Date: d}
		if h := w.hrvFor(d); h != nil {
			td.HrvMs = pr(h.LastNightAvg, 0)
			td.HrvStatus = h.Status
		}
		if sl := w.sleepFor(d); sl != nil {
			score, _ := sl.Score()
			td.SleepScore = pr(score, 0)
			td.SleepS = pr(sl.SleepTimeSeconds, 0)
			td.DeepS = pr(sl.DeepSleepSeconds, 0)
		}
		sum := w.summary(d)
		if sum != nil {
			if sum.HeartRate != nil {
				td.RestingHR = pr(sum.HeartRate.RestingValue, 0)
			}
			td.FloorsUp = pr(sum.FloorsUp(), 0)
			if sum.BodyBattery != nil {
				td.BodyBatteryMax = pr(sum.BodyBattery.MaxValue, 0)
			}
		}
		if d == w.date && w.dhr != nil && w.dhr.RestingHeartRate != nil {
			td.RestingHR = pr(w.dhr.RestingHeartRate, 0)
		}
		if st := w.stepsFor(d); st != nil && st.TotalSteps != nil {
			td.Steps = pr(st.TotalSteps, 0)
		} else if sum != nil {
			v, _ := sum.StepsValue()
			td.Steps = pr(v, 0)
		}
		if w.actsOK {
			load := 0.0
			for i := range w.acts {
				if w.acts[i].LocalDate() == d && w.acts[i].TrainingLoad != nil {
					load += *w.acts[i].TrainingLoad
				}
			}
			td.TrainingLoad = num(load, 0)
		}
		out = append(out, td)
	}
	return out
}

// buildDayInfo : activité de la journée (Mode 2).
func buildDayInfo(w *wellness) DayInfo {
	var di DayInfo
	sum := w.summary(w.date)
	if st := w.stepsFor(w.date); st != nil {
		di.Steps = pr(st.TotalSteps, 0)
		di.StepGoal = pr(st.StepGoal, 0)
	}
	if sum != nil {
		v, g := sum.StepsValue()
		if di.Steps == nil {
			di.Steps = pr(v, 0)
		}
		if di.StepGoal == nil {
			di.StepGoal = pr(g, 0)
		}
		di.FloorsUp = pr(sum.FloorsUp(), 0)
		di.FloorsDown = pr(sum.FloorsDown(), 0)
		if sum.IntensityMinutes != nil {
			di.IntensityModerateMin = pr(sum.IntensityMinutes.Moderate, 0)
			di.IntensityVigorousMin = pr(sum.IntensityMinutes.Vigorous, 0)
		}
		if sum.Calories != nil {
			di.ActiveCalories = pr(sum.Calories.BurnedActive, 0)
		}
		if sum.Stress != nil && pf(sum.Stress.AvgLevel) >= 0 {
			di.StressAvg = pr(sum.Stress.AvgLevel, 0)
		}
		if sum.BodyBattery != nil {
			di.BodyBatteryCharged = pr(sum.BodyBattery.ChargedValue, 0)
			di.BodyBatteryDrained = pr(sum.BodyBattery.DrainedValue, 0)
		}
	}
	if bbd := w.bbDay(w.date); bbd != nil {
		if bbd.Charged != nil {
			di.BodyBatteryCharged = pr(bbd.Charged, 0)
		}
		if bbd.Drained != nil {
			di.BodyBatteryDrained = pr(bbd.Drained, 0)
		}
	}
	return di
}
