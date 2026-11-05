package analysis

import (
	"math"
	"time"

	"stride/internal/garmin"
)

// Wellness gathers the per-day Garmin wellness payloads around Date (from SQLite or cache).
type Wellness struct {
	Date  string
	Sleep map[string]*garmin.DailySleep
	HRV   map[string]*garmin.HRVSummary
	Daily map[string]*garmin.DailySummary
	User  map[string]*garmin.UserSummary
	// Loads is the summed training load per local date; nil when unknown.
	Loads map[string]float64
}

func NewWellness(date string) *Wellness {
	return &Wellness{Date: date, Sleep: map[string]*garmin.DailySleep{}, HRV: map[string]*garmin.HRVSummary{},
		Daily: map[string]*garmin.DailySummary{}, User: map[string]*garmin.UserSummary{}}
}

func (w *Wellness) sleepFor(d string) *garmin.DailySleep {
	if s := w.Sleep[d]; s.HasData() {
		return s
	}
	return nil
}

var hrvStatusLabels = map[string]string{
	"BALANCED":   "Équilibré",
	"UNBALANCED": "Déséquilibré",
	"LOW":        "Bas",
	"POOR":       "Faible",
}

func HRVStatusLabel(s string) string {
	if l, ok := hrvStatusLabels[s]; ok {
		return l
	}
	return s
}

// fmtLocalClock formats a Garmin "local" timestamp (shifted epoch ms) as HH:MM.
func fmtLocalClock(ms *float64) *string {
	if ms == nil || *ms <= 0 {
		return nil
	}
	s := time.UnixMilli(int64(*ms)).UTC().Format("15:04")
	return &s
}

func nonNegative(p *float64) *float64 {
	if p == nil || *p < 0 {
		return nil
	}
	return pr(p, 0)
}

// BuildRecovery returns the recovery context of the night ending on w.Date (nil without any data).
func BuildRecovery(w *Wellness) *RecoveryContext {
	rc := &RecoveryContext{Date: w.Date, Trend: []TrendDay{}}
	found := false

	if h := w.HRV[w.Date]; h != nil && (h.LastNightAvg != nil || h.Status != nil) {
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
		found = true
	}

	if sl := w.sleepFor(w.Date); sl != nil {
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
		found = true
	}

	us, ds := w.User[w.Date], w.Daily[w.Date]
	rhr, avg7 := nan, nan
	if us != nil {
		rhr, avg7 = pf(us.RestingHeartRate), pf(us.LastSevenDaysAvgRestingHeartRate)
	}
	if !valid(rhr) && ds != nil && ds.HeartRate != nil {
		rhr = pf(ds.HeartRate.RestingValue)
	}
	if !valid(avg7) {
		var a wacc
		for _, d := range dateRange(dateAdd(w.Date, -6), w.Date) {
			if s := w.Daily[d]; s != nil && s.HeartRate != nil {
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
		found = true
	}

	bb := &BodyBattery{}
	if us != nil {
		bb.AtWake = nonNegative(us.BodyBatteryAtWakeTime)
		bb.Max, bb.Min = nonNegative(us.BodyBatteryHighestValue), nonNegative(us.BodyBatteryLowestValue)
		bb.Charged, bb.Drained = nonNegative(us.BodyBatteryChargedValue), nonNegative(us.BodyBatteryDrainedValue)
	}
	if ds != nil && ds.BodyBattery != nil {
		if bb.Max == nil {
			bb.Max = nonNegative(ds.BodyBattery.MaxValue)
		}
		if bb.Min == nil {
			bb.Min = nonNegative(ds.BodyBattery.MinValue)
		}
		if bb.Charged == nil {
			bb.Charged = nonNegative(ds.BodyBattery.ChargedValue)
		}
		if bb.Drained == nil {
			bb.Drained = nonNegative(ds.BodyBattery.DrainedValue)
		}
	}
	if bb.AtWake != nil || bb.Max != nil || bb.Charged != nil || bb.Drained != nil {
		rc.BodyBattery = bb
		found = true
	}
	rc.StressAvg = stressOf(us, ds)
	if rc.StressAvg != nil {
		found = true
	}

	rc.Trend = BuildTrend(w)
	for _, t := range rc.Trend {
		if t.HrvMs != nil || t.SleepS != nil || t.RestingHR != nil || t.Steps != nil {
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	return rc
}

func stressOf(us *garmin.UserSummary, ds *garmin.DailySummary) *float64 {
	if us != nil {
		if v := nonNegative(us.AverageStressLevel); v != nil {
			return v
		}
	}
	if ds != nil && ds.Stress != nil {
		return nonNegative(ds.Stress.AvgLevel)
	}
	return nil
}

// StressOn returns the average stress of a day (nil when unknown or insufficient data).
func (w *Wellness) StressOn(d string) *float64 { return stressOf(w.User[d], w.Daily[d]) }

// FloorsOn returns floors climbed and descended on a day. Garmin omits the value on days without
// floors: a day with step data but no floors counts as 0.
func (w *Wellness) FloorsOn(d string) (*float64, *float64) {
	zero := func(p *float64, hasDay bool) *float64 {
		if p == nil && hasDay {
			return num(0, 0)
		}
		return pr(p, 0)
	}
	if us := w.User[d]; us != nil && (us.FloorsAscended != nil || us.FloorsDescended != nil || us.TotalSteps != nil) {
		return zero(us.FloorsAscended, us.TotalSteps != nil), zero(us.FloorsDescended, us.TotalSteps != nil)
	}
	if ds := w.Daily[d]; ds != nil {
		steps, _ := ds.StepsValue()
		return zero(ds.FloorsUp(), steps != nil), zero(ds.FloorsDown(), steps != nil)
	}
	return nil, nil
}

// StepsOn returns the step count and goal of a day.
func (w *Wellness) StepsOn(d string) (*float64, *float64) {
	if us := w.User[d]; us != nil && us.TotalSteps != nil {
		return pr(us.TotalSteps, 0), pr(us.DailyStepGoal, 0)
	}
	if ds := w.Daily[d]; ds != nil {
		v, g := ds.StepsValue()
		return pr(v, 0), pr(g, 0)
	}
	return nil, nil
}

// BuildTrend returns the J-6..J trend in chronological order.
func BuildTrend(w *Wellness) []TrendDay {
	out := []TrendDay{}
	for _, d := range dateRange(dateAdd(w.Date, -6), w.Date) {
		td := TrendDay{Date: d}
		if h := w.HRV[d]; h != nil {
			td.HrvMs = pr(h.LastNightAvg, 0)
			td.HrvStatus = h.Status
		}
		if sl := w.sleepFor(d); sl != nil {
			score, _ := sl.Score()
			td.SleepScore = pr(score, 0)
			td.SleepS = pr(sl.SleepTimeSeconds, 0)
			td.DeepS = pr(sl.DeepSleepSeconds, 0)
		}
		if us := w.User[d]; us != nil {
			td.RestingHR = pr(us.RestingHeartRate, 0)
			td.BodyBatteryMax = nonNegative(us.BodyBatteryHighestValue)
		}
		if ds := w.Daily[d]; ds != nil {
			if td.RestingHR == nil && ds.HeartRate != nil {
				td.RestingHR = pr(ds.HeartRate.RestingValue, 0)
			}
			if td.BodyBatteryMax == nil && ds.BodyBattery != nil {
				td.BodyBatteryMax = nonNegative(ds.BodyBattery.MaxValue)
			}
		}
		td.Steps, _ = w.StepsOn(d)
		td.FloorsUp, _ = w.FloorsOn(d)
		if w.Loads != nil {
			td.TrainingLoad = num(w.Loads[d], 0)
		}
		out = append(out, td)
	}
	return out
}

// BuildDayInfo returns the daily activity numbers of w.Date.
func BuildDayInfo(w *Wellness) DayInfo {
	var di DayInfo
	d := w.Date
	di.Steps, di.StepGoal = w.StepsOn(d)
	di.FloorsUp, di.FloorsDown = w.FloorsOn(d)
	di.StressAvg = w.StressOn(d)
	us, ds := w.User[d], w.Daily[d]
	if us != nil {
		di.IntensityModerateMin, di.IntensityVigorousMin = pr(us.ModerateIntensityMinutes, 0), pr(us.VigorousIntensityMinutes, 0)
		di.ActiveCalories = pr(us.ActiveKilocalories, 0)
		di.BodyBatteryCharged, di.BodyBatteryDrained = nonNegative(us.BodyBatteryChargedValue), nonNegative(us.BodyBatteryDrainedValue)
	}
	if ds != nil {
		if ds.IntensityMinutes != nil && di.IntensityModerateMin == nil && di.IntensityVigorousMin == nil {
			di.IntensityModerateMin, di.IntensityVigorousMin = pr(ds.IntensityMinutes.Moderate, 0), pr(ds.IntensityMinutes.Vigorous, 0)
		}
		if ds.Calories != nil && di.ActiveCalories == nil {
			di.ActiveCalories = pr(ds.Calories.BurnedActive, 0)
		}
		if ds.BodyBattery != nil {
			if di.BodyBatteryCharged == nil {
				di.BodyBatteryCharged = nonNegative(ds.BodyBattery.ChargedValue)
			}
			if di.BodyBatteryDrained == nil {
				di.BodyBatteryDrained = nonNegative(ds.BodyBattery.DrainedValue)
			}
		}
	}
	return di
}
