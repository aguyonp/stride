package analysis

import (
	"math"
	"strings"
	"testing"

	"stride/internal/fitdata"
	"stride/internal/garmin"
)

func fp(v float64) *float64 { return &v }
func sp(s string) *string   { return &s }

// synthActivity : 10 min de course, 1 min de marche, 10 min de course (1 Hz), sans GPS.
// FC qui monte pour produire une dérive mesurable.
func synthActivity(withSplits bool) *fitdata.Activity {
	a := &fitdata.Activity{Source: "fit"}
	n := fitdata.NaN
	a.Session = fitdata.Session{TimerS: 1260, ElapsedS: 1260, DistM: n, AvgHR: n, MaxHR: n, AvgPower: n, NormPower: n,
		AvgCadSpm: n, MaxCadSpm: n, AvgGCTMs: n, AvgVOCm: n, AvgVRPct: n, AvgStepM: n, AvgGCTBal: n, AscentM: n, DescentM: n}
	dist := 0.0
	for t := 0; t < 1260; t++ {
		r := fitdata.Record{T: float64(t), AltM: 100, VRPct: 7.6, StepM: 1.05, GCTMs: 248, VOCm: 8.1, GCTBalLeftPct: 49.2}
		switch {
		case t < 600:
			r.SpeedMS, r.CadenceSpm, r.PowerW = 3.0, 178+float64(t%3-1)*2, 290
			r.HR = 130 + 10*float64(t)/600
		case t < 660:
			r.SpeedMS, r.CadenceSpm, r.PowerW, r.HR = 1.3, 110, 150, 118
			r.GCTMs, r.VOCm = 320, 6
		default:
			r.SpeedMS, r.CadenceSpm, r.PowerW = 2.9, 178, 285
			r.HR = 140 + 10*float64(t-660)/600
		}
		dist += r.SpeedMS
		r.DistM = dist
		a.Records = append(a.Records, r)
	}
	if withSplits {
		a.Splits = []fitdata.Split{
			{Type: "interval_active", StartT: 0, EndT: 1259, TimerS: 1260, ElapsedS: 1260, DistM: dist, AscentM: n, DescentM: n},
			{Type: "rwd_run", StartT: 0, EndT: 599, TimerS: 600, ElapsedS: 600, DistM: 1800, AscentM: 0, DescentM: 0},
			{Type: "rwd_walk", StartT: 600, EndT: 659, TimerS: 60, ElapsedS: 60, DistM: 78, AscentM: 0, DescentM: 0},
			{Type: "rwd_run", StartT: 660, EndT: 1259, TimerS: 600, ElapsedS: 600, DistM: 1740, AscentM: 0, DescentM: 0},
		}
	}
	a.Laps = []fitdata.Lap{
		{StartT: 0, EndT: 333, TimerS: 333, ElapsedS: 333, DistM: 1000, AvgHR: 133, MaxHR: 136, AvgPower: 290, AvgCadSpm: 178, AvgGCTMs: 248, AvgVOCm: 8.1, AvgGCTBal: n, AscentM: 0, DescentM: 0, Intensity: "interval", Trigger: "distance"},
		{StartT: 333, EndT: 1259, TimerS: 926, ElapsedS: 926, DistM: 2618, AvgHR: 140, MaxHR: 150, AvgPower: n, AvgCadSpm: n, AvgGCTMs: n, AvgVOCm: n, AvgGCTBal: n, AscentM: n, DescentM: n, Intensity: "interval", Trigger: "session_end"},
	}
	return a
}

func TestRunMaskExcludesWalk(t *testing.T) {
	s := newSession(synthActivity(true), true)
	for i, r := range s.act.Records {
		inWalk := r.T >= 600 && r.T < 660
		if s.run[i] == inWalk {
			t.Fatalf("record t=%v : run=%v", r.T, s.run[i])
		}
	}
	// sans splits : exclusion par vitesse/cadence
	s2 := newSession(synthActivity(false), true)
	if s2.run[630] || !s2.run[100] {
		t.Fatal("masque course incorrect sans splits")
	}
}

func TestBiomechanics(t *testing.T) {
	p := testProfile()
	s := newSession(synthActivity(true), true)
	b := computeBiomech(s, nil, p.Thresholds, true, true)
	if got := pf(b.CadenceAvgSpm); got != 178 {
		t.Errorf("cadence moyenne = %v, attendu 178", got)
	}
	if got := pf(b.CadenceInTargetPct); got != 100 {
		t.Errorf("cadence dans la cible = %v, attendu 100", got)
	}
	if got := pf(b.CadenceBelow170Pct); got != 0 {
		t.Errorf("cadence < 170 = %v, attendu 0", got)
	}
	if pf(b.GroundContactTimeMs) != 248 || pf(b.VerticalOscillationCm) != 8.1 || pf(b.StrideLengthM) != 1.05 || pf(b.VerticalRatioPct) != 7.6 {
		t.Errorf("dynamique de course inattendue : %+v", b)
	}
	g := b.GctBalance
	if !g.Available || pf(g.LeftPct) != 49.2 || pf(g.RightPct) != 50.8 || pf(g.GapPct) != 1.6 {
		t.Fatalf("équilibre GCT inattendu : %+v", g)
	}
	if spv(g.WeakerSide) != "left" || spv(g.Alert) != "warn" {
		t.Errorf("côté faible / alerte : %v / %v", spv(g.WeakerSide), spv(g.Alert))
	}
}

func TestBiomechanicsNoBalanceAndFallback(t *testing.T) {
	a := synthActivity(true)
	for i := range a.Records {
		a.Records[i].GCTBalLeftPct = fitdata.NaN
		a.Records[i].VOCm = fitdata.NaN
	}
	det := &garmin.ActivityDetail{Summary: garmin.ActivitySumDTO{VerticalOscillation: fp(9.7), StrideLength: fp(112)}}
	b := computeBiomech(newSession(a, true), det, testProfile().Thresholds, true, true)
	if b.GctBalance.Available || b.GctBalance.Reason == nil || b.GctBalance.LeftPct != nil {
		t.Fatalf("équilibre absent mal géré : %+v", b.GctBalance)
	}
	if pf(b.VerticalOscillationCm) != 9.7 {
		t.Errorf("repli OV sur l'API attendu, obtenu %v", pf(b.VerticalOscillationCm))
	}
	// non-course : pas de biomécanique
	b2 := computeBiomech(newSession(a, false), det, testProfile().Thresholds, false, true)
	if b2.CadenceAvgSpm != nil || b2.GctBalance.Available {
		t.Error("biomécanique attendue vide hors course")
	}
}

func TestSegmentsFromSplits(t *testing.T) {
	sg := buildSegments(newSession(synthActivity(true), true))
	if sg.Primary != "blocks" || len(sg.Blocks) != 3 {
		t.Fatalf("segments : primary=%s blocks=%d", sg.Primary, len(sg.Blocks))
	}
	want := []struct{ kind, label string }{{"run", "Bloc 1"}, {"walk", "Pause 1"}, {"run", "Bloc 2"}}
	for i, w := range want {
		b := sg.Blocks[i]
		if b.Kind != w.kind || b.Label != w.label || b.Index != i+1 {
			t.Errorf("bloc %d = %s/%s", i, b.Kind, b.Label)
		}
	}
	b1 := sg.Blocks[0]
	if b1.DurationS != 600 || b1.DistanceM != 1800 || pf(b1.PaceSecPerKm) != 333 || pf(b1.AvgPower) != 290 || pf(b1.AvgCadenceSpm) != 178 {
		t.Errorf("stats bloc 1 inattendues : %+v", b1)
	}
	if pf(sg.Blocks[1].AvgHR) != 118 || sg.Blocks[1].StartOffsetS != 600 {
		t.Errorf("pause : %+v", sg.Blocks[1])
	}
	if len(sg.Laps) != 2 || sg.Laps[0].Label != "Km 1" || sg.Laps[1].Label != "Km 2" || sg.Laps[0].Kind != "lap" {
		t.Errorf("laps : %+v", sg.Laps)
	}
	// valeur de lap manquante → recalcul sur la fenêtre
	if sg.Laps[1].AvgPower == nil {
		t.Error("puissance du lap 2 attendue (recalcul records)")
	}
}

func TestNonRunningHasNoBlocksNorPaceDrift(t *testing.T) {
	s := newSession(synthActivity(false), false)
	sg := buildSegments(s)
	if sg.Primary != "laps" || len(sg.Blocks) != 0 || len(sg.Laps) != 2 {
		t.Errorf("hors course : %s %d %d", sg.Primary, len(sg.Blocks), len(sg.Laps))
	}
	a := synthActivity(false)
	for i := range a.Records {
		a.Records[i].PowerW = fitdata.NaN
	}
	if computeDrift(newSession(a, false)) != nil {
		t.Error("pas de dérive Pace:Hr hors course")
	}
	if d := computeDrift(s); d == nil || d.Method != "pw:hr" {
		t.Error("dérive Pw:Hr attendue hors course avec puissance")
	}
}

func TestDetectBlocksWithoutSplits(t *testing.T) {
	sg := buildSegments(newSession(synthActivity(false), true))
	if sg.Primary != "blocks" || len(sg.Blocks) != 3 {
		t.Fatalf("détection maison : primary=%s n=%d %+v", sg.Primary, len(sg.Blocks), sg.Blocks)
	}
	if sg.Blocks[1].Kind != "walk" || math.Abs(sg.Blocks[1].DurationS-60) > 12 {
		t.Errorf("pause détectée : %+v", sg.Blocks[1])
	}
}

func TestConsolidateMicroPauses(t *testing.T) {
	n := nan
	in := []rawBlock{
		{kind: "run", t0: 0, t1: 285, durS: 285, distM: 858, ascent: 5, descent: 8},
		{kind: "stand", t0: 314, t1: 317, durS: 3, distM: 5, standS: 3, ascent: 0, descent: 0},
		{kind: "walk", t0: 317, t1: 318, durS: 1, distM: 1, walkS: 1, ascent: n, descent: n},
		{kind: "run", t0: 318, t1: 367, durS: 49, distM: 130, ascent: 0, descent: 1},
		{kind: "walk", t0: 367, t1: 450, durS: 83, distM: 110, walkS: 83},
		{kind: "run", t0: 450, t1: 1308, durS: 858, distM: 2570},
		{kind: "walk", t0: 1308, t1: 1320, durS: 12, distM: 15, walkS: 12},
		{kind: "run", t0: 1320, t1: 1330, durS: 10, distM: 30},
		{kind: "stand", t0: 1330, t1: 1400, durS: 70, distM: 0, standS: 70},
	}
	out := consolidate(in)
	kinds := []string{}
	for _, b := range out {
		kinds = append(kinds, b.kind)
	}
	if got := strings.Join(kinds, ","); got != "run,walk,run,stand" {
		t.Fatalf("consolidation : %s", got)
	}
	if out[0].durS != 338 || out[0].distM != 994 || out[0].t1 != 367 {
		t.Errorf("bloc fusionné : %+v", out[0])
	}
	if out[3].durS != 92 || out[3].kind != "stand" {
		t.Errorf("pause finale fusionnée : %+v", out[3])
	}
}

func TestAlignPausesCoversTimerStop(t *testing.T) {
	// Chrono arrêté 285→314 s, split rwd_stand 314→317 s (elapsed Garmin 21 s), reprise à 318 s.
	in := []rawBlock{
		{kind: "run", t0: 0, t1: 285, durS: 284.7},
		{kind: "stand", t0: 314, t1: 317, durS: 21.1, standS: 21.1},
		{kind: "run", t0: 318, t1: 2092, durS: 1773.3},
	}
	out := consolidate(alignPauses(in))
	if len(out) != 3 {
		t.Fatalf("attendu run,stand,run : %+v", out)
	}
	p := out[1]
	if p.t0 != 285 || p.t1 != 317 || p.durS != 32 || p.kind != "stand" {
		t.Errorf("pause alignée : %+v", p)
	}
	if out[0].durS != 284.7 || out[2].t0 != 318 {
		t.Errorf("blocs de course modifiés : %+v / %+v", out[0], out[2])
	}
}

func TestDrift(t *testing.T) {
	d := computeDrift(newSession(synthActivity(true), true))
	if d == nil {
		t.Fatal("dérive attendue")
	}
	if d.Method != "pw:hr" {
		t.Errorf("méthode = %s", d.Method)
	}
	// EF1 = 290/135, EF2 = 285/145 → ≈ 8,5 %
	if math.Abs(d.DecouplingPct-8.5) > 0.3 || d.Verdict != "moderate" {
		t.Errorf("découplage = %v (%s)", d.DecouplingPct, d.Verdict)
	}
	if d.FirstHalfHR != 135 || d.SecondHalfHR != 145 || d.LastThirdHrRiseBpm < 5 {
		t.Errorf("FC moitiés/tiers : %+v", d)
	}
	// trop court → nil
	a := synthActivity(true)
	a.Records = a.Records[:300]
	if computeDrift(newSession(a, true)) != nil {
		t.Error("pas de dérive attendue sur 5 min")
	}
}

func TestSeriesDownsample(t *testing.T) {
	sr := buildSeries(newSession(synthActivity(true), true))
	n := len(sr.TS)
	if n == 0 || n > maxSeriesPoints {
		t.Fatalf("taille série = %d", n)
	}
	for _, l := range [][]*float64{sr.HR, sr.CadenceSpm, sr.PowerW, sr.PaceSecPerKm, sr.ElevationM, sr.GctMs, sr.VoCm, sr.GctBalanceLeftPct} {
		if len(l) != n {
			t.Fatal("séries de longueurs différentes")
		}
	}
	if sr.PaceSecPerKm[0] == nil || *sr.PaceSecPerKm[0] != 333 {
		t.Errorf("allure 1er point = %v", sr.PaceSecPerKm[0])
	}
}

func TestTargetPaceTime(t *testing.T) {
	s := newSession(synthActivity(true), true)
	secs, dist := targetPaceTime(s, 330, 340) // 3,0 m/s = 333 s/km
	if math.Abs(secs-600) > 3 || math.Abs(dist-1800) > 10 {
		t.Errorf("temps dans l'allure = %v s / %v m", secs, dist)
	}
}

func TestFadeEnd(t *testing.T) {
	sg := Segments{Blocks: []Segment{
		{Kind: "run", DurationS: 600, DistanceM: 2000, PaceSecPerKm: fp(300), AvgHR: fp(140)},
		{Kind: "walk", DurationS: 60, DistanceM: 80},
		{Kind: "run", DurationS: 600, DistanceM: 1800, PaceSecPerKm: fp(333), AvgHR: fp(141)},
	}}
	if v := fadeEnd(sg); !valid(v) || math.Round(v) != 11 {
		t.Errorf("FADE_END = %v", v)
	}
	sg.Blocks[2].AvgHR = fp(130) // FC plus basse : ralentissement volontaire
	if valid(fadeEnd(sg)) {
		t.Error("pas de FADE_END attendu si FC plus basse")
	}
}

func TestTypeLabels(t *testing.T) {
	cases := map[string]string{"running": "Course", "trail_running": "Trail", "cycling": "Vélo", "walking": "Marche", "foo_bar": "Foo bar", "kayaking_v2": "Kayak", "resort_skiing_snowboarding_ws": "Ski alpin"}
	for k, v := range cases {
		if TypeLabel(k) != v {
			t.Errorf("%s → %s", k, TypeLabel(k))
		}
	}
	if !IsRunningType("treadmill_running") || IsRunningType("cycling") {
		t.Error("IsRunningType")
	}
}
