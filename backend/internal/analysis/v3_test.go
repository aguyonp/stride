package analysis

import (
	"encoding/json"
	"math"
	"testing"

	"stride/internal/fitdata"
	"stride/internal/garmin"
)

func steadyRun(n int, withRestLaps bool) *fitdata.Activity {
	a := &fitdata.Activity{Session: fitdata.EmptySession()}
	for i := 0; i < n; i++ {
		r := fitdata.Record{T: float64(i), HR: 150, CadenceSpm: 170, SpeedMS: 3, PowerW: 250, AltM: math.NaN(), DistM: float64(i) * 3,
			GCTMs: math.NaN(), VOCm: 8.8, VRPct: math.NaN(), StepM: math.NaN(), GCTBalLeftPct: math.NaN(), GAPSpeedMS: math.NaN(),
			StaminaPct: math.NaN(), AvailStaminaPct: math.NaN(), GradePct: math.NaN()}
		if i >= 600 && i < 660 {
			r.SpeedMS = 0 // one-minute pause, excluded
		}
		a.Records = append(a.Records, r)
	}
	if withRestLaps {
		a.Laps = []fitdata.Lap{{Intensity: "rest"}, {Intensity: "recovery"}}
	}
	return a
}

func TestComputeMetricsSteady(t *testing.T) {
	loss := 54.0
	m := ComputeMetrics(MetricsInput{Sport: "run", IsRunning: true, Act: steadyRun(1861, false), ElevLossM: &loss})
	ef := m.Efficiency
	if !ef.Eligible || *ef.EfSpeed != 1.2 || *ef.EfPower != 1.667 {
		t.Fatalf("efficiency: %+v", ef)
	}
	imp := m.Impact
	// 30 min moving × 170 spm = 5100 steps; VO 8.8 → ×1.1; D- 54 m over 5.4 km → ×1.1
	if *imp.Steps != 5100 || *imp.VoFactor != 1.1 || *imp.DescentFactor != 1.1 || *imp.JointIndex != 6.2 || *imp.StepsPerKm != 944 {
		t.Fatalf("impact: %+v", *imp)
	}
	if *m.PaceSecPerKm != 333 {
		t.Fatalf("pace %v", *m.PaceSecPerKm)
	}
}

func TestComputeMetricsIneligible(t *testing.T) {
	m := ComputeMetrics(MetricsInput{Sport: "run", IsRunning: true, Act: steadyRun(1861, true)})
	if m.Efficiency.Eligible || m.Efficiency.Reason == nil || m.Impact == nil {
		t.Fatalf("interval: %+v", m)
	}
	short := ComputeMetrics(MetricsInput{Sport: "run", IsRunning: true, Act: steadyRun(600, false)})
	if short.Efficiency.Eligible {
		t.Fatal("short run eligible")
	}
	dist, mov, hr, cad := 10000.0, 3000.0, 140.0, 160.0
	sum := ComputeMetrics(MetricsInput{Sport: "run", IsRunning: true, DistanceM: &dist, MovingS: &mov, AvgHR: &hr, AvgCadenceSpm: &cad})
	if !sum.Efficiency.Eligible || *sum.Efficiency.EfSpeed != 1.429 || *sum.Impact.Steps != 8000 || sum.Impact.VoFactor != nil || *sum.Impact.JointIndex != 8 {
		t.Fatalf("summary fallback: %+v %+v", sum.Efficiency, *sum.Impact)
	}
	if b := ComputeMetrics(MetricsInput{Sport: "strength"}); b.Efficiency.Eligible || b.Impact != nil {
		t.Fatal("strength")
	}
}

func TestEfficiencyChangePct(t *testing.T) {
	dates := []string{"2026-09-01", "2026-09-08", "2026-10-20", "2026-10-25"}
	got := EfficiencyChangePct(dates, []float64{1.0, 1.0, 1.1, 1.1}, "2026-09-01", "2026-10-26")
	if got == nil || *got != 10 {
		t.Fatalf("%v", got)
	}
	if EfficiencyChangePct(dates[:3], []float64{1, 1, 1}, "2026-09-01", "2026-10-26") != nil {
		t.Fatal("needs 2 points per side")
	}
}

func hrvDay(d string, v, low float64) *garmin.HRVSummary {
	var h garmin.HRVSummary
	_ = json.Unmarshal([]byte(`{"calendarDate":"`+d+`","lastNightAvg":`+jsonNum(v)+`,"baseline":{"balancedLow":`+jsonNum(low)+`}}`), &h)
	return &h
}

func jsonNum(v float64) string { b, _ := json.Marshal(v); return string(b) }

func TestReadinessBanner(t *testing.T) {
	w := NewWellness("2026-11-05")
	for i, v := range []float64{50, 44, 43, 42, 41, 50, 40} {
		d := DateAdd("2026-11-05", i-6)
		w.HRV[d] = hrvDay(d, v, 45)
	}
	ratio := 1.4
	b := BuildReadinessBanner(w, &LoadRatio{Ratio: &ratio}, nil)
	if b.Level != "red" || *b.NightsBelowBaseline != 5 || *b.Hrv7dAvg != 44 || len(b.Reasons) != 3 {
		t.Fatalf("red: %+v", b)
	}
	low := 1.1
	if b := BuildReadinessBanner(w, &LoadRatio{Ratio: &low}, nil); b.Level != "orange" {
		t.Fatalf("orange: %+v", b)
	}
	if b := BuildReadinessBanner(NewWellness("2026-11-05"), nil, nil); b.Level != "unknown" || b.Title == "" {
		t.Fatalf("unknown: %+v", b)
	}
	ok := NewWellness("2026-11-05")
	ok.HRV["2026-11-05"] = hrvDay("2026-11-05", 55, 45)
	if b := BuildReadinessBanner(ok, &LoadRatio{Ratio: &low}, nil); b.Level != "green" {
		t.Fatalf("green: %+v", b)
	}
}
