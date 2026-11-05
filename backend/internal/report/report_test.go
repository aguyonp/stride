package report

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"stride/internal/analysis"
	"stride/internal/athlete"
	"stride/internal/provider/weather"
	"stride/internal/view"
)

var update = flag.Bool("update", false, "rewrite testdata golden files")

func fp(v float64) *float64 { return &v }
func sp(s string) *string   { return &s }
func ip(v int64) *int64     { return &v }

func kneeProfile() athlete.Profile {
	p := athlete.DefaultProfile("Europe/Paris")
	pr := athlete.PresetFor(athlete.FocusKnee)
	p.FocusArea, p.FocusLabel, p.Thresholds = pr.FocusArea, pr.Label, pr.Thresholds
	p.InjuredSide = sp("left")
	return p
}

func goals() []athlete.Goal {
	return []athlete.Goal{
		{ID: 2, Title: "Cadence 175 spm", Kind: "cadence", TargetValue: fp(175), TargetUnit: sp("spm"), Status: "active"},
		{ID: 1, Title: "Courir 45 min continu sans gêne", Kind: "macro", TargetDate: sp("2026-11-15"), Status: "active", Note: "Puis reprise perf en décembre"},
	}
}

func upcoming() []athlete.PlannedSession {
	return []athlete.PlannedSession{
		{ID: 11, Date: "2026-11-04", Sport: "run", Title: "Footing", TargetDurationS: fp(2400), TargetPaceMinSecPerKm: fp(320), TargetPaceMaxSecPerKm: fp(330)},
		{ID: 12, Date: "2026-11-05", Sport: "strength", Title: "Renfo quadriceps"},
		{ID: 13, Date: "2026-11-07", Sport: "run", TargetDistanceM: fp(8000)},
	}
}

func history() []view.HistoryRow {
	return []view.HistoryRow{
		{ActivityID: 3, Date: "2026-10-28", Type: "running", TypeLabel: "Course", IsRunning: true, DistanceM: fp(6000), DurationS: fp(2100),
			PaceSecPerKm: fp(350), AvgCadenceSpm: fp(171), GctMs: fp(252), ElevLossM: fp(20), TrainingLoad: fp(70),
			ShoeName: sp("Route souple"), PainDuring: fp(2), PainCold: fp(1.5), PainNextMorning: fp(1)},
		{ActivityID: 4, Date: "2026-10-30", Type: "cycling", TypeLabel: "Vélo", DistanceM: fp(12000), DurationS: fp(1800), TrainingLoad: fp(40)},
		{ActivityID: 5, Date: "2026-11-02", Type: "running", TypeLabel: "Course", IsRunning: true, DistanceM: fp(7020), DurationS: fp(2530),
			PaceSecPerKm: fp(360), AvgCadenceSpm: fp(172), GctMs: fp(250), ElevLossM: fp(35), TrainingLoad: fp(85), ShoeName: sp("Route souple"), PainDuring: fp(2)},
	}
}

func recovery() *analysis.RecoveryContext {
	return &analysis.RecoveryContext{
		Date: "2026-11-02",
		HRV: &analysis.HRVContext{LastNightAvgMs: fp(45), WeeklyAvgMs: fp(48), BaselineLowMs: fp(40), BaselineHighMs: fp(55),
			Status: sp("BALANCED"), StatusLabel: sp("Équilibré")},
		Sleep:       &analysis.SleepContext{Score: fp(78), DeepS: fp(4920), DurationS: fp(25920)},
		RestingHR:   &analysis.RestingHR{Bpm: fp(48), Avg7d: fp(46), Delta: fp(2)},
		BodyBattery: &analysis.BodyBattery{AtWake: fp(82)},
		Trend: []analysis.TrendDay{
			{Date: "2026-11-01", HrvMs: fp(46), HrvStatus: sp("BALANCED"), SleepScore: fp(81), DeepS: fp(5460), RestingHR: fp(47), Steps: fp(9120), FloorsUp: fp(8), TrainingLoad: fp(0)},
			{Date: "2026-11-02", HrvMs: fp(45), HrvStatus: sp("UNBALANCED"), SleepScore: fp(78), DeepS: fp(4920), RestingHR: fp(48), Steps: fp(12345), FloorsUp: fp(16), TrainingLoad: fp(85), PainNextMorning: fp(3)},
		},
		Readiness: analysis.Readiness{Score: fp(72), Status: sp("Prêt"), Source: sp("manual")},
	}
}

func fullActivity() *view.ActivityView {
	planned := athlete.PlannedSession{ID: 10, Date: "2026-11-02", Sport: "run", Title: "Continu", TargetDurationS: fp(2280),
		TargetPaceMinSecPerKm: fp(315), TargetPaceMaxSecPerKm: fp(325), TargetCadenceSpm: fp(170), Instructions: "Rester  souple\nen descente"}
	return &view.ActivityView{
		SourceLabel: "Fusion",
		Activity: view.ActivityDetail{
			ActivitySummary: view.ActivitySummary{ID: 5, Type: "running", TypeLabel: "Course", Sport: "run", IsRunning: true,
				StartLocal: "2026-11-02T18:30:12", Date: "2026-11-02", DistanceM: fp(7020), DurationS: fp(2530),
				AvgPaceSecPerKm: fp(360), GapSecPerKm: fp(352), AvgPower: fp(285), TrainingLoad: fp(85)},
			ImpactLoad: fp(1234), AerobicTE: fp(3.1),
			Strava: &view.StravaInfo{Title: "Sortie", Description: "Bonnes\nsensations", PrivateNote: "genou ok", RPE: fp(5), RelativeEffort: fp(45)},
		},
		Weather: &view.Weather{Conditions: weather.Conditions{TemperatureC: fp(12.4), HumidityPct: fp(80), WindKph: fp(15.2)}, WindDirLabel: sp("SO")},
		Biomechanics: analysis.Biomechanics{
			GctBalance:          analysis.GctBalance{Available: true, LeftPct: fp(49.5), RightPct: fp(50.5), GapPct: fp(1)},
			GroundContactTimeMs: fp(250), CadenceAvgSpm: fp(172), CadenceMaxSpm: fp(181), CadenceInTargetPct: fp(64),
			VerticalOscillationCm: fp(8.9), VerticalRatioPct: fp(7.8), StrideLengthM: fp(1.02),
			FatigueDrift: &analysis.FatigueDrift{GctStartMs: fp(245), GctEndMs: fp(257), GctDeltaMs: fp(12), GctDeltaPct: fp(4.9),
				VoStartCm: fp(8.8), VoEndCm: fp(9.1), VoDeltaCm: fp(0.3), CadenceStartSpm: fp(173), CadenceEndSpm: fp(171), CadenceDeltaSpm: fp(-2), Verdict: "moderate"},
			Slope: &analysis.Slope{ThresholdPct: 1.5,
				Flat:                   &analysis.SlopeBucket{CadenceSpm: fp(173), StrideM: fp(1.01), DurationS: 1800, DistanceM: 5000},
				Downhill:               &analysis.SlopeBucket{CadenceSpm: fp(168), StrideM: fp(1.08), DurationS: 300, DistanceM: 900},
				DownhillCadenceDropSpm: fp(5)},
		},
		Load: analysis.Load{ElevGainM: fp(32), ElevLossM: fp(35), TrainingLoad: fp(85), AerobicTE: fp(3.1), ImpactLoad: fp(1234),
			Drift:                &analysis.Drift{Method: "pw:hr", DecouplingPct: 3.2, Verdict: "stable"},
			Stamina:              &analysis.Stamina{StartPct: fp(100), EndPct: fp(78), MinPct: fp(75)},
			LoadRatio:            &analysis.LoadRatio{Acute: 410, Chronic: 365, Ratio: fp(1.12), Verdict: "optimal", Source: "computed"},
			StravaRelativeEffort: fp(45)},
		Segments: analysis.Segments{Primary: "blocks", Blocks: []analysis.Segment{
			{Index: 1, Kind: "run", Label: "Bloc 1", DurationS: 600, DistanceM: 1750, PaceSecPerKm: fp(343), GapSecPerKm: fp(340), AvgHR: fp(134),
				AvgPower: fp(290), AvgCadenceSpm: fp(179), GctMs: fp(248), ElevGainM: fp(5), ElevLossM: fp(3)},
			{Index: 2, Kind: "walk", Label: "Pause 1", DurationS: 60, DistanceM: 80, AvgHR: fp(118), ElevGainM: fp(0), ElevLossM: fp(1)},
			{Index: 3, Kind: "stand", Label: "Pause 2", DurationS: 30, DistanceM: 0},
		}},
		CrossLoad: view.CrossLoad{WindowStart: "2026-11-01T00:00:00", WindowEnd: "2026-11-02T18:30:12",
			Bike:   view.BikeLoad{Rides: 1, DistanceKm: 12.34, DurationS: 2400, AvgHR: fp(110), TrainingLoad: fp(25)},
			Floors: view.Floors{PrevDayUp: fp(16), PrevDayDown: fp(14), DayUp: fp(5), DayDown: fp(4)},
			Steps:  view.Steps{PrevDay: fp(12345), Day: fp(4000)}, StressPrevDay: fp(32)},
		Recovery:     recovery(),
		Plan:         &athlete.PlanMatch{PlannedID: 10, ActivityID: ip(5), Status: "done", DurationDeltaPct: fp(11), Summary: "42:10 pour 38:00 prévues (+11 %)"},
		PlannedToday: &planned,
		Notes: athlete.ActivityNotes{PainDuring: fp(2), PainCold: fp(1.5), RPE: fp(6), Note: "Genou calme",
			Tags: []string{"metronome", "desk_sitting"},
			Shoe: &athlete.Shoe{ID: 1, Name: "Route souple", Brand: "Marque", Model: "Modèle 1", DropMm: fp(8), Type: "soft"}},
		Goals:    goals(),
		Upcoming: upcoming(),
		History:  history(),
		Alerts: []analysis.Alert{
			{Level: "warn", Code: "FLOORS_PREV_DAY", Title: "Escaliers la veille", Message: "16 étages montés la veille (> 15), charge cumulée avant la séance."},
			{Level: "info", Code: "SLEEP_LOW", Title: "Sommeil profond court", Message: "Profond 0h56 (< 1h), récupération tissulaire réduite."},
		},
	}
}

// stravaOnly mimics a Strava-only, non-running activity with almost everything missing.
func stravaOnly() *view.ActivityView {
	return &view.ActivityView{
		SourceLabel: "Strava",
		Activity: view.ActivityDetail{ActivitySummary: view.ActivitySummary{ID: 9, Type: "cycling", TypeLabel: "Vélo", Sport: "bike",
			StartLocal: "2026-11-03T08:00:00", Date: "2026-11-03", DistanceM: fp(20000), DurationS: fp(3600)}},
		Biomechanics: analysis.Biomechanics{GctBalance: analysis.GctBalance{Reason: sp("Garmin non connecté")}},
		Segments:     analysis.Segments{Primary: "laps", Blocks: []analysis.Segment{}, Laps: []analysis.Segment{}},
		Notes:        athlete.EmptyNotes(),
		Alerts:       []analysis.Alert{{Level: "ok", Code: "ALL_GOOD", Title: "RAS", Message: "Aucun signal d'alerte sur cette séance."}},
	}
}

func dayView() *view.DayView {
	return &view.DayView{
		Date:     "2026-11-02",
		Recovery: recovery(),
		Day: analysis.DayInfo{Steps: fp(8432), StepGoal: fp(8000), FloorsUp: fp(12), FloorsDown: fp(10), IntensityModerateMin: fp(25),
			IntensityVigorousMin: fp(5), ActiveCalories: fp(640), StressAvg: fp(32), BodyBatteryCharged: fp(45), BodyBatteryDrained: fp(60)},
		CrossLoad: view.CrossLoad{BikeByDay: []view.BikeDay{{Date: "2026-11-01", BikeLoad: view.BikeLoad{Rides: 1, DistanceKm: 5, DurationS: 900}},
			{Date: "2026-11-02", BikeLoad: view.BikeLoad{Rides: 2, DistanceKm: 12.2, DurationS: 2640, TrainingLoad: fp(18)}}}},
		Activities: []view.ActivitySummary{
			{ID: 20, Type: "commuting", TypeLabel: "Vélotaf", Sport: "bike", DistanceM: fp(6100), DurationS: fp(1330)},
			{ID: 21, Type: "walking", TypeLabel: "Marche", Sport: "walk", DistanceM: fp(2800), DurationS: fp(2100)},
		},
		Planned: []athlete.PlannedSession{{ID: 10, Date: "2026-11-02", Sport: "run", Title: "Continu", TargetDurationS: fp(2280),
			Match: &athlete.PlanMatch{PlannedID: 10, Status: "missed", Summary: "Séance non réalisée"}}},
		Goals:    goals(),
		Upcoming: upcoming(),
		History:  history(),
		Alerts:   []analysis.Alert{{Level: "ok", Code: "ALL_GOOD", Title: "RAS", Message: "Aucun signal d'alerte sur cette journée."}},
	}
}

func multiExport() *view.MultiExport {
	h := history()
	s1 := view.ActivitySummary{ID: 3, Type: "running", TypeLabel: "Course", Sport: "run", IsRunning: true, Date: "2026-10-28",
		DistanceM: fp(6000), DurationS: fp(2100), AvgPaceSecPerKm: fp(350), GapSecPerKm: fp(345), AvgHR: fp(140),
		AvgCadenceSpm: fp(171), ElevLossM: fp(20), TrainingLoad: fp(70)}
	s2 := view.ActivitySummary{ID: 5, Type: "trail_running", TypeLabel: "Trail", Sport: "run", IsRunning: true, Date: "2026-11-02",
		DistanceM: fp(7020), DurationS: fp(2530), AvgPaceSecPerKm: fp(360), AvgHR: fp(145), AvgCadenceSpm: fp(172), ElevLossM: fp(35)}
	return &view.MultiExport{
		From: "2026-10-26", To: "2026-11-08", Weeks: 2, NextWeeks: 3, Goals: goals(), Upcoming: upcoming()[:1], Unanalyzed: 1,
		Sessions: []view.MultiSession{
			{Summary: s1, GctMs: fp(252), GctDriftPct: fp(2.1), ShoeName: h[0].ShoeName, PainDuring: fp(2), PainCold: fp(1.5), PainNextMorning: fp(1)},
			{Summary: s2, PainDuring: fp(2)},
		},
		Weekly: []view.WeekTrend{
			{WeekStart: "2026-10-26", Sessions: 1, DistanceM: 6000, DurationS: 2100, Load: fp(110), LoadRatio: fp(0.95), CadenceAvg: fp(171),
				GctAvg: fp(252), HrvAvg: fp(47), SleepScoreAvg: fp(80), PainDuring: fp(2), PainCold: fp(1.5), PainNextMorning: fp(1)},
			{WeekStart: "2026-11-02", Sessions: 1, DistanceM: 7020, DurationS: 2530},
		},
		Plan: view.PlanVsDone{Planned: 4, Done: 2, Partial: 1, Missed: 1, VolumeDeltaPct: fp(-12)},
		Alerts: []view.AlertCount{
			{Alert: analysis.Alert{Level: "info", Title: "Sommeil profond court"}, Count: 3},
			{Alert: analysis.Alert{Level: "warn", Title: "Cadence sous la cible"}, Count: 2},
			{Alert: analysis.Alert{Level: "danger", Title: "Douleur pendant l'effort"}, Count: 1},
		},
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".md")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run go test -update): %v", path, err)
	}
	if got != string(want) {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := 0; i < len(gl) || i < len(wl); i++ {
			var g, w string
			if i < len(gl) {
				g = gl[i]
			}
			if i < len(wl) {
				w = wl[i]
			}
			if g != w {
				t.Fatalf("%s differs at line %d:\n got: %q\nwant: %q", name, i+1, g, w)
			}
		}
	}
	checkCommon(t, name, got)
}

func checkCommon(t *testing.T, name, got string) {
	t.Helper()
	if !strings.HasPrefix(got, Directive+"\n\n") {
		t.Errorf("%s: directive must come first", name)
	}
	if strings.Contains(got, "SFP") {
		t.Errorf("%s: contains SFP", name)
	}
	if strings.Contains(got, "\n\n\n") || strings.Contains(got, "NaN") || strings.Contains(got, "<nil>") {
		t.Errorf("%s: malformed output", name)
	}
}

func TestActivityGoldenAllChips(t *testing.T) {
	golden(t, "activity_all", Activity(fullActivity(), kneeProfile(), nil))
}

func TestActivityGoldenPainPlan(t *testing.T) {
	inc, err := ParseInclude("pain, plan")
	if err != nil {
		t.Fatal(err)
	}
	got := Activity(fullActivity(), kneeProfile(), inc)
	golden(t, "activity_pain_plan", got)
	for _, absent := range []string{"### BIOMÉCANIQUE", "### DÉTAIL DES INTERVALLES", "### CHARGE, STAMINA", "### RÉCUPÉRATION", "### HISTORIQUE"} {
		if strings.Contains(got, absent) {
			t.Errorf("section %q should be omitted", absent)
		}
	}
	for _, present := range []string{"### CONTEXTE & OBJECTIFS DE L'ATHLÈTE", "### RAPPORT SÉANCE - 02/11/2026 18:30", "**Ressenti & Douleur :**", "### ALERTES AUTOMATIQUES"} {
		if !strings.Contains(got, present) {
			t.Errorf("missing %q", present)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(got), "récupération tissulaire réduite.") {
		t.Error("alerts must be last")
	}
}

func TestActivityHistoryWithoutPainColumns(t *testing.T) {
	inc, _ := ParseInclude("history")
	got := Activity(fullActivity(), kneeProfile(), inc)
	if !strings.Contains(got, "| Date | Dist. | Durée | Allure | Cadence | GCT | D- | Load | Chaussures |\n") {
		t.Errorf("history header without pain columns expected:\n%s", got)
	}
	if strings.Contains(got, "Gêne") || strings.Contains(got, "Ressenti") || strings.Contains(got, "CONTEXTE") {
		t.Error("pain/plan content must be omitted")
	}
	if strings.Contains(got, "| 30/10 |") {
		t.Error("history table lists running sessions only")
	}
}

func TestActivityStravaOnlyNulls(t *testing.T) {
	got := Activity(stravaOnly(), athlete.DefaultProfile("Europe/Paris"), nil)
	golden(t, "activity_strava_only", got)
	if !strings.Contains(got, "Performance") {
		t.Error("none preset must print Performance mode")
	}
}

func TestDayGolden(t *testing.T) {
	golden(t, "day_all", Day(dayView(), kneeProfile(), nil))
	inc, _ := ParseInclude("plan")
	got := Day(dayView(), kneeProfile(), inc)
	if strings.Contains(got, "TENDANCE 7 JOURS") || strings.Contains(got, "RÉCUPÉRATION") || strings.Contains(got, "HISTORIQUE") {
		t.Error("health/history sections must be omitted")
	}
	if !strings.Contains(got, "### BILAN SANTÉ & CHARGE QUOTIDIENNE - 02/11/2026") {
		t.Error("BILAN is always present")
	}
	empty := Day(&view.DayView{Date: "2026-11-02"}, athlete.DefaultProfile("UTC"), nil)
	checkCommon(t, "day_empty", empty)
}

func TestMultiGolden(t *testing.T) {
	golden(t, "multi_all", Multi(multiExport(), kneeProfile(), nil))
	inc, _ := ParseInclude("history")
	got := Multi(multiExport(), kneeProfile(), inc)
	checkCommon(t, "multi_history", got)
	if !strings.Contains(got, "| Date | Type | Dist. | Durée | Allure | GAP | FC moy | D- | Chaussures |\n") {
		t.Errorf("reduced session table expected:\n%s", got)
	}
	if !strings.Contains(got, "| Semaine | Séances | Volume | Durée |\n") {
		t.Errorf("reduced weekly table expected:\n%s", got)
	}
	if strings.Contains(got, "PRÉVU vs RÉALISÉ") || strings.Contains(got, "CONTEXTE") {
		t.Error("plan sections must be omitted")
	}
	if !strings.Contains(got, "### DEMANDE") {
		t.Error("DEMANDE always present")
	}
}

func TestParseInclude(t *testing.T) {
	for _, in := range []string{"", " ", ",", " , "} {
		inc, err := ParseInclude(in)
		if err != nil || inc.String() != strings.Join(Chips, ",") {
			t.Errorf("%q → %v %v", in, inc, err)
		}
	}
	inc, err := ParseInclude(" Pain ,splits")
	if err != nil || !inc.Has("pain") || !inc.Has("splits") || inc.Has("plan") || inc.String() != "pain,splits" {
		t.Errorf("subset: %v %v", inc, err)
	}
	if _, err := ParseInclude("pain,weather"); err == nil {
		t.Error("unknown chip must fail")
	}
	if inc, err := ParseIncludeList(nil); err != nil || len(inc) != len(Chips) {
		t.Error("nil list → all")
	}
	var none Include
	if !none.Has("health") {
		t.Error("nil Include enables everything")
	}
	if EstimateTokens("abcde") != 2 || EstimateTokens("é") != 1 {
		t.Error("token estimate")
	}
}
