package analysis

import (
	"math"
	"strings"
	"testing"

	"stride/internal/athlete"
	"stride/internal/fitdata"
	"stride/internal/garmin"
)

func testProfile() athlete.Profile {
	p := athlete.DefaultProfile("Europe/Paris")
	pr := athlete.PresetFor(athlete.FocusKnee)
	left := "left"
	p.FocusArea, p.FocusLabel, p.Thresholds, p.InjuredSide = pr.FocusArea, pr.Label, pr.Thresholds, &left
	p.PaceTargets = []athlete.PaceTarget{{Label: "10 km", MinSecPerKm: 330, MaxSecPerKm: 340}}
	return p
}

func TestLoadRatio(t *testing.T) {
	loads := []DatedLoad{
		{"2026-11-02", "2026-11-02T18:00:00", 100},
		{"2026-11-02", "2026-11-02T20:00:00", 500}, // after the analysed session
		{"2026-10-30", "2026-10-30T10:00:00", 100},
		{"2026-10-20", "2026-10-20T10:00:00", 100},
		{"2026-10-10", "2026-10-10T10:00:00", 100},
		{"2026-10-01", "2026-10-01T10:00:00", 999}, // outside 28 days
	}
	lr := ComputeLoadRatio(loads, "2026-11-02", "2026-11-02T18:00:00", 1.3)
	if lr == nil || lr.Acute != 200 || lr.Chronic != 100 || pf(lr.Ratio) != 2 || lr.Verdict != "danger" || lr.Source != "computed" {
		t.Fatalf("load ratio = %+v", lr)
	}
	if ComputeLoadRatio(nil, "2026-11-02", "", 1.3) != nil {
		t.Error("nil expected without chronic load")
	}
	if a := ACWRFrom(lr); a.Acute7d != 200 || a.Chronic28d != 100 {
		t.Errorf("acwr = %+v", a)
	}
}

func TestAlerts(t *testing.T) {
	p := testProfile()
	bio := &Biomechanics{CadenceAvgSpm: fp(165), VerticalOscillationCm: fp(10.6), GctBalance: gctBalance(48.0, p.Thresholds),
		FatigueDrift: &FatigueDrift{GctDeltaPct: fp(6.2), Verdict: "high"},
		Slope:        &Slope{DownhillCadenceDropSpm: fp(7)}}
	rc := &RecoveryContext{
		HRV:         &HRVContext{Status: sp("LOW"), LastNightAvgMs: fp(38), BaselineLowMs: fp(42), BaselineHighMs: fp(55)},
		RestingHR:   &RestingHR{Bpm: fp(54), Avg7d: fp(48), Delta: fp(6)},
		Sleep:       &SleepContext{Score: fp(55), DeepS: fp(2400)},
		BodyBattery: &BodyBattery{AtWake: fp(25)},
	}
	plan := athlete.PlanMatch{Status: "partial", DurationDeltaPct: fp(-30), Summary: "Durée 26:00 / 38:00 prévues (-32 %)"}
	in := AlertInput{IsRunning: true, Bio: bio, ElevLossM: 65, Drift: &Drift{Method: "pace:hr", DecouplingPct: 11}, FadePct: 12,
		Recovery: rc, LoadRatio: &LoadRatio{Ratio: fp(1.4), Verdict: "high"}, FloorsPrevDay: 18, FloorsDay: nan,
		Plans: []athlete.PlanMatch{plan}, PainDuring: 3, PainNextMorning: 4}
	out := BuildAlerts(in, p)
	got := map[string]string{}
	for _, a := range out {
		if _, dup := got[a.Code]; !dup {
			got[a.Code] = a.Level
		}
		if strings.Contains(a.Message, " : ") || strings.Contains(a.Message, "SFP") {
			t.Errorf("message format: %q", a.Message)
		}
	}
	want := map[string]string{"CADENCE_LOW": "danger", "VO_HIGH": "danger", "GCT_BALANCE": "danger", "DESCENT_HIGH": "danger",
		"DRIFT_HIGH": "danger", "FADE_END": "warn", "HRV_LOW": "danger", "RHR_HIGH": "warn", "SLEEP_LOW": "warn", "BB_LOW": "danger",
		"LOAD_RATIO": "warn", "GCT_DRIFT": "warn", "DOWNHILL_CADENCE_DROP": "warn", "FLOORS_PREV_DAY": "warn",
		"PLAN_DEVIATION": "warn", "PAIN_DURING": "warn", "PAIN_NEXT_MORNING": "warn"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("alert %s = %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["ALL_GOOD"]; ok || out[0].Level != "danger" || out[len(out)-1].Level == "danger" {
		t.Error("severity order / ALL_GOOD")
	}
	for _, a := range out {
		if a.Code == "GCT_BALANCE" && !strings.Contains(a.Message, "côté surveillé") {
			t.Errorf("GCT message: %q", a.Message)
		}
	}
	// performance preset: injury alerts off, load alert kept
	perf := athlete.DefaultProfile("Europe/Paris")
	pout := BuildAlerts(AlertInput{IsRunning: true, Bio: &Biomechanics{CadenceAvgSpm: fp(178), GctBalance: gctBalance(50, perf.Thresholds)},
		ElevLossM: 400, FloorsPrevDay: 50, FadePct: nan, PainDuring: nan, PainNextMorning: nan, LoadRatio: &LoadRatio{Ratio: fp(1.4), Verdict: "high"}}, perf)
	for _, a := range pout {
		if a.Code == "DESCENT_HIGH" || a.Code == "FLOORS_PREV_DAY" {
			t.Errorf("injury alert with the performance preset: %+v", a)
		}
	}
	if pout[0].Code != "LOAD_RATIO" {
		t.Errorf("load alert expected first: %+v", pout)
	}
	ok := BuildAlerts(AlertInput{IsRunning: true, Bio: &Biomechanics{CadenceAvgSpm: fp(178), GctBalance: gctBalance(50, p.Thresholds)},
		ElevLossM: 10, FadePct: nan, FloorsPrevDay: nan, PainDuring: nan, PainNextMorning: nan}, p)
	if len(ok) != 1 || ok[0].Code != "ALL_GOOD" {
		t.Errorf("ALL_GOOD expected: %+v", ok)
	}
	na := BuildAlerts(AlertInput{IsRunning: true, Bio: &Biomechanics{CadenceAvgSpm: fp(169), GctBalance: GctBalance{Reason: sp(NoGarminReason)}},
		ElevLossM: nan, FadePct: nan, FloorsPrevDay: nan, PainDuring: nan, PainNextMorning: nan}, p)
	if na[0].Code != "CADENCE_LOW" || na[0].Level != "warn" || na[1].Code != "GCT_BALANCE_NA" || !strings.Contains(na[1].Message, "Garmin non connecté") {
		t.Errorf("alerts: %+v", na)
	}
	day := BuildAlerts(AlertInput{DayMode: true, FloorsDay: 24, ElevLossM: nan, FadePct: nan, FloorsPrevDay: nan, PainDuring: nan, PainNextMorning: nan}, p)
	if day[0].Code != "FLOORS_HIGH" {
		t.Errorf("FLOORS_HIGH expected: %+v", day)
	}
}

func TestRecoveryFromWellness(t *testing.T) {
	w := NewWellness("2026-11-02")
	w.Sleep["2026-11-02"] = &garmin.DailySleep{CalendarDate: "2026-11-02", SleepTimeSeconds: fp(25920), DeepSleepSeconds: fp(4920),
		SleepEndTimestampLocal: fp(1.7621e12 + 3600e3)}
	w.HRV["2026-11-02"] = &garmin.HRVSummary{CalendarDate: "2026-11-02", LastNightAvg: fp(45), WeeklyAvg: fp(50), Status: sp("BALANCED")}
	w.User["2026-11-02"] = &garmin.UserSummary{RestingHeartRate: fp(48), LastSevenDaysAvgRestingHeartRate: fp(46), BodyBatteryAtWakeTime: fp(82),
		BodyBatteryChargedValue: fp(60), BodyBatteryDrainedValue: fp(40), AverageStressLevel: fp(-1), FloorsAscended: fp(12), TotalSteps: fp(9000),
		DailyStepGoal: fp(8000), ModerateIntensityMinutes: fp(20)}
	w.Daily["2026-11-02"] = &garmin.DailySummary{Stress: &struct {
		AvgLevel  *float64 `json:"avgLevel"`
		MaxLevel  *float64 `json:"maxLevel"`
		Qualifier *string  `json:"qualifier"`
	}{AvgLevel: fp(31)}}
	w.Loads = map[string]float64{"2026-10-30": 80}
	rc := BuildRecovery(w)
	if rc == nil || rc.HRV == nil || spv(rc.HRV.StatusLabel) != "Équilibré" || pf(rc.HRV.DeltaVsWeeklyPct) != -10 {
		t.Fatalf("hrv: %+v", rc)
	}
	if pf(rc.RestingHR.Delta) != 2 || pf(rc.BodyBattery.AtWake) != 82 || pf(rc.BodyBattery.Charged) != 60 || pf(rc.StressAvg) != 31 {
		t.Errorf("rhr / bb / stress: %+v %+v %v", rc.RestingHR, rc.BodyBattery, pf(rc.StressAvg))
	}
	if len(rc.Trend) != 7 || rc.Trend[0].Date != "2026-10-27" || pf(rc.Trend[3].TrainingLoad) != 80 || pf(rc.Trend[4].TrainingLoad) != 0 {
		t.Fatalf("trend: %+v", rc.Trend)
	}
	di := BuildDayInfo(w)
	if pf(di.Steps) != 9000 || pf(di.StepGoal) != 8000 || pf(di.FloorsUp) != 12 || pf(di.StressAvg) != 31 || pf(di.IntensityModerateMin) != 20 {
		t.Errorf("day info: %+v", di)
	}
	if BuildRecovery(NewWellness("2026-11-02")) != nil {
		t.Error("nil expected without data")
	}
}

// hillyRun: 30 min at 1 Hz, flat then a 6 % descent, GCT rising in the last third.
func hillyRun() *fitdata.Activity {
	a := &fitdata.Activity{Source: "fit", Session: fitdata.EmptySession()}
	dist, alt := 0.0, 100.0
	for t := 0; t < 1800; t++ {
		r := fitdata.Record{T: float64(t), HR: 140, PowerW: fitdata.NaN, VRPct: 7.5, GCTBalLeftPct: fitdata.NaN,
			GAPSpeedMS: fitdata.NaN, StaminaPct: 100 - float64(t)/90, AvailStaminaPct: 90 - float64(t)/60, GradePct: fitdata.NaN}
		r.SpeedMS, r.CadenceSpm, r.StepM, r.VOCm = 3.0, 176, 1.02, 8.5
		r.GCTMs = 240
		if t >= 1200 {
			r.GCTMs = 258
			r.CadenceSpm = 172
		}
		if t >= 900 && t < 1300 {
			alt -= 3.0 * 0.06
			r.CadenceSpm, r.StepM = 168, 1.1
		}
		dist += r.SpeedMS
		r.DistM, r.AltM = dist, alt
		a.Records = append(a.Records, r)
	}
	return a
}

func TestFatigueDriftSlopeAndGAP(t *testing.T) {
	s := newSession(hillyRun(), true)
	fd := s.fatigueDrift()
	if fd == nil || pf(fd.GctStartMs) != 240 || pf(fd.GctEndMs) != 258 || pf(fd.GctDeltaMs) != 18 || pf(fd.GctDeltaPct) != 7.5 || fd.Verdict != "high" {
		t.Fatalf("fatigue drift: %+v", fd)
	}
	if pf(fd.CadenceDeltaSpm) >= 0 || pf(fd.VoDeltaCm) != 0 {
		t.Errorf("cadence / vo drift: %+v", fd)
	}
	sl := s.slope()
	if sl == nil || sl.Flat == nil || sl.Downhill == nil || sl.ThresholdPct != 1.5 {
		t.Fatalf("slope: %+v", sl)
	}
	if pf(sl.Downhill.CadenceSpm) != 168 || pf(sl.DownhillCadenceDropSpm) < 4 || pf(sl.Downhill.StrideM) != 1.1 || pf(sl.Downhill.StrideMaxM) != 1.1 {
		t.Errorf("downhill bucket: %+v drop %v", sl.Downhill, pf(sl.DownhillCadenceDropSpm))
	}
	if math.Abs(sl.Downhill.DurationS-400) > 40 {
		t.Errorf("downhill duration %v", sl.Downhill.DurationS)
	}
	if g := ModelGAPSpeed(3, -6); !(g < 3) || ModelGAPSpeed(3, 6) <= 3 || ModelGAPSpeed(3, 0) != 3 {
		t.Errorf("GAP model: %v", g)
	}
	c := Compute(Inputs{TypeKey: "running", Act: hillyRun(), Thresholds: testProfile().Thresholds, PaceTargets: testProfile().PaceTargets, GarminConnected: true})
	if c.GapSecPerKm == nil || c.Biomechanics.FatigueDrift == nil || c.Biomechanics.Slope == nil || c.Stamina == nil || pf(c.Stamina.StartPct) != 100 {
		t.Fatalf("core: gap=%v stamina=%+v", c.GapSecPerKm, c.Stamina)
	}
	if len(c.Series.GapSecPerKm) != len(c.Series.TS) || len(c.Series.StaminaPct) != len(c.Series.TS) || c.Series.StaminaPct[0] == nil {
		t.Error("series gap/stamina")
	}
	if !strings.Contains(strings.Join(c.Warnings, "|"), "Minetti") {
		t.Errorf("modelled GAP warning expected: %v", c.Warnings)
	}
	if len(c.Performance.TargetPaces) != 1 || c.Performance.TargetPaces[0].Label != "Allure 10 km" {
		t.Errorf("target paces: %+v", c.Performance.TargetPaces)
	}
}

func TestComputeStravaOnly(t *testing.T) {
	a := hillyRun()
	for i := range a.Records {
		a.Records[i].GCTMs, a.Records[i].VOCm, a.Records[i].StepM, a.Records[i].StaminaPct, a.Records[i].AvailStaminaPct = fitdata.NaN, fitdata.NaN, fitdata.NaN, fitdata.NaN, fitdata.NaN
	}
	c := Compute(Inputs{TypeKey: "running", Act: a, Thresholds: testProfile().Thresholds})
	b := c.Biomechanics
	if b.GroundContactTimeMs != nil || b.VerticalOscillationCm != nil || b.GctBalance.Available || spv(b.GctBalance.Reason) != NoGarminReason {
		t.Errorf("garmin-only metrics must be null: %+v", b)
	}
	if b.StrideLengthM == nil || b.CadenceAvgSpm == nil || c.Stamina != nil {
		t.Errorf("stride (derived) / cadence / stamina: %+v %v", b, c.Stamina)
	}
	if b.FatigueDrift == nil || b.FatigueDrift.GctDeltaPct != nil || b.FatigueDrift.CadenceDeltaSpm == nil {
		t.Errorf("cadence-only fatigue drift expected: %+v", b.FatigueDrift)
	}
	if !strings.Contains(strings.Join(c.Warnings, "|"), "Garmin non connecté") {
		t.Errorf("warnings: %v", c.Warnings)
	}
}
