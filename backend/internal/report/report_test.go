package report

import (
	"strings"
	"testing"

	"patella/internal/analysis"
	"patella/internal/config"
)

func fp(v float64) *float64 { return &v }
func sp(s string) *string   { return &s }

// sample reproduit l'exemple du §4 du contrat.
func sample(mode analysis.Mode) *analysis.ActivityAnalysis {
	return &analysis.ActivityAnalysis{
		Mode: mode,
		Activity: analysis.ActivityDetail{
			ActivitySummary: analysis.ActivitySummary{
				ID: 1, Type: "running", TypeLabel: "Course", IsRunning: true,
				StartLocal: "2026-11-02T18:30:12", Date: "2026-11-02",
				DistanceM: fp(7020), DurationS: fp(2530), AvgPaceSecPerKm: fp(360), AvgPower: fp(285),
				TrainingLoad: fp(85),
			},
			ImpactLoad: fp(92), AerobicTE: fp(3.1), AnaerobicTE: fp(0.5),
		},
		Biomechanics: analysis.Biomechanics{
			GctBalance:            analysis.GctBalance{Available: true, LeftPct: fp(49.2), RightPct: fp(50.8), GapPct: fp(1.6)},
			GroundContactTimeMs:   fp(248),
			CadenceAvgSpm:         fp(178),
			CadenceMaxSpm:         fp(186),
			CadenceInTargetPct:    fp(72),
			VerticalOscillationCm: fp(8.1),
			VerticalRatioPct:      fp(7.6),
			StrideLengthM:         fp(1.05),
		},
		Load: analysis.Load{
			ElevGainM: fp(32), ElevLossM: fp(35),
			Drift:        &analysis.Drift{Method: "pw:hr", DecouplingPct: 3.2, LastThirdHrRiseBpm: 4, Verdict: "stable"},
			TrainingLoad: fp(85), AerobicTE: fp(3.1), AnaerobicTE: fp(0.5),
		},
		Segments: analysis.Segments{Primary: "blocks", Blocks: []analysis.Segment{
			{Index: 1, Kind: "run", Label: "Bloc 1", DurationS: 600, DistanceM: 1750, PaceSecPerKm: fp(343), AvgHR: fp(134), AvgPower: fp(290), AvgCadenceSpm: fp(179)},
			{Index: 2, Kind: "walk", Label: "Pause 1", DurationS: 60, DistanceM: 80, AvgHR: fp(118)},
		}},
		Recovery: &analysis.RecoveryContext{
			Date: "2026-11-02",
			HRV: &analysis.HRVContext{LastNightAvgMs: fp(45), WeeklyAvgMs: fp(48), BaselineLowMs: fp(40), BaselineHighMs: fp(55),
				Status: sp("BALANCED"), StatusLabel: sp("Équilibré")},
			Sleep:       &analysis.SleepContext{Score: fp(78), DeepS: fp(4920), DurationS: fp(25920)},
			RestingHR:   &analysis.RestingHR{Bpm: fp(48), Avg7d: fp(46), Delta: fp(2)},
			BodyBattery: &analysis.BodyBattery{AtWake: fp(82)},
			Trend: []analysis.TrendDay{
				{Date: "2026-10-28", HrvMs: fp(46), HrvStatus: sp("BALANCED"), SleepScore: fp(81), DeepS: fp(5460), RestingHR: fp(47), Steps: fp(9120), FloorsUp: fp(8)},
			},
		},
		Alerts: []analysis.Alert{{Level: "warn", Code: "CADENCE_LOW", Title: "Cadence basse", Message: "168 spm < 170, risque d'overstriding."}},
	}
}

const recoveryBlock = `### RÉCUPÉRATION & PHYSIOLOGIE (NUIT DU 01/11 AU 02/11)
- **VFC nocturne :** 45 ms (Statut : Équilibré vs baseline 40-55 ms | moy. 7 j : 48 ms)
- **Score Sommeil :** 78/100 (Dont profond : 1h22) | Durée : 7h12
- **FC repos nocturne :** 48 bpm (moy. 7 j : 46 bpm) | **Body Battery réveil :** 82/100`

const splitsBlock = `### DÉTAIL DES INTERVALLES / SPLITS
| Bloc | Durée | Distance | Allure | FC moy | Watts | Cadence |
|---|---|---|---|---|---|---|
| Bloc 1 | 10:00 | 1,75 km | 5:43/km | 134 bpm | 290 W | 179 spm |
| Pause 1 | 1:00 | 0,08 km | Marche | 118 bpm | — | — |`

func TestActivitySFPGolden(t *testing.T) {
	want := `### RAPPORT SÉANCE GARMIN - 02/11/2026 18:30
- **Mode :** Rééducation SFP
- **Séance :** 7,02 km en 42:10 | Allure moy: 6:00/km | Puissance: 285 W
- **Dénivelé :** +32 m / -35 m

### BIOMÉCANIQUE ROTULE & IMPACTS
- **Équilibre contact sol (G/D) :** 49,2 % G / 50,8 % D (écart 1,6 pt)
- **Temps de contact sol :** 248 ms
- **Cadence moyenne :** 178 spm (Max : 186 spm) | Dans la cible 175-182 : 72 %
- **Oscillation verticale :** 8,1 cm | **Ratio vertical :** 7,6 %
- **Longueur de foulée moyenne :** 1,05 m

` + splitsBlock + `

### CHARGE & FATIGUE
- **Dérive cardiaque (Pw:Hr) :** +3,2 % (stable) | FC dernier tiers : +4 bpm
- **Training Load :** 85 | **TE aérobie :** 3,1 | **Impact load :** 92

` + recoveryBlock + `

### ALERTES
- ⚠️ Cadence basse : 168 spm < 170, risque d'overstriding.
`
	got := Reporter{}.Activity(sample(analysis.ModeSFP), config.DefaultProfile())
	assertGolden(t, got, want)
}

func TestActivitySFPMissingBalance(t *testing.T) {
	a := sample(analysis.ModeSFP)
	a.Biomechanics.GctBalance = analysis.GctBalance{Available: false}
	a.Recovery = nil
	a.Load.Drift = nil
	got := Reporter{}.Activity(a, config.DefaultProfile())
	for _, s := range []string{
		"- **Équilibre contact sol (G/D) :** N/D (non mesuré par le capteur)",
		"- **Dérive cardiaque :** N/D",
		"- **VFC nocturne :** N/D",
		"- **Score Sommeil :** N/D",
		"- **FC repos nocturne :** N/D (moy. 7 j : N/D) | **Body Battery réveil :** N/D",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("ligne attendue absente : %q\n%s", s, got)
		}
	}
	if strings.Contains(got, "\n\n\n") {
		t.Error("ligne vide superflue")
	}
}

func TestActivityPerformanceGolden(t *testing.T) {
	a := sample(analysis.ModePerformance)
	a.Biomechanics.GctBalance = analysis.GctBalance{Available: false}
	a.Performance = &analysis.Performance{
		AvgPower: fp(285), NormPower: fp(298),
		TimeInZonesS: []float64{130, 1120, 725, 390, 45},
		TargetPaces: []analysis.TargetPace{
			{Label: "Allure 10 km", RangeLabel: "3:35–3:40", Seconds: 260, DistanceM: 1200},
			{Label: "Allure marathon", RangeLabel: "3:50–4:00", Seconds: 0, DistanceM: 0},
		},
		ACWR: &analysis.ACWR{Acute7d: 410, Chronic28d: 365, Ratio: fp(1.12), Verdict: "optimal"},
	}
	want := `### RAPPORT SÉANCE GARMIN - 02/11/2026 18:30
- **Mode :** Performance
- **Séance :** 7,02 km en 42:10 | Allure moy: 6:00/km | Puissance: 285 W
- **Dénivelé :** +32 m / -35 m

### PERFORMANCE
- **Puissance :** moy 285 W | normalisée 298 W
- **Zones FC :** Z1 2:10 | Z2 18:40 | Z3 12:05 | Z4 6:30 | Z5 0:45 (Z4+Z5 : 7:15)
- **Allure 10 km (3:35-3:40) :** 4:20 sur 1,20 km | **Allure marathon (3:50-4:00) :** 0:00
- **Dérive cardiaque (Pw:Hr) :** +3,2 % (stable)
- **Charge aiguë/chronique :** 410 / 365 (ACWR 1,12 – optimal)
- **Training Load séance :** 85 | **TE :** 3,1 aéro / 0,5 anaéro

### BIOMÉCANIQUE (RÉSUMÉ)
- Cadence 178 spm | GCT 248 ms | OV 8,1 cm | RV 7,6 % | Foulée 1,05 m | G/D N/D

` + splitsBlock + `

` + recoveryBlock + `

### ALERTES
- ⚠️ Cadence basse : 168 spm < 170, risque d'overstriding.
`
	got := Reporter{}.Activity(a, config.DefaultProfile())
	assertGolden(t, got, want)
}

func TestActivityRecoveryWithHistory(t *testing.T) {
	a := sample(analysis.ModeRecovery)
	a.HistoryDays = 14
	a.History = []analysis.HistoryRow{
		{Date: "2026-10-28", Type: "running", TypeLabel: "Course", DistanceM: fp(7020), DurationS: fp(2530), PaceSecPerKm: fp(360), AvgHR: fp(134), AvgCadenceSpm: fp(178), ElevLossM: fp(35), TrainingLoad: fp(85)},
		{Date: "2026-10-29", Type: "cycling", TypeLabel: "Vélo", DistanceM: fp(6100), DurationS: fp(1330), PaceSecPerKm: fp(218), AvgHR: fp(110)},
	}
	want := `### RAPPORT SÉANCE GARMIN - 02/11/2026 18:30
- **Mode :** Santé & Récupération
- **Séance :** 7,02 km en 42:10 | Allure moy: 6:00/km | Puissance: 285 W
- **Dénivelé :** +32 m / -35 m
- **Biomécanique :** Cadence 178 spm | GCT 248 ms | OV 8,1 cm | RV 7,6 % | Foulée 1,05 m | G/D 49,2/50,8

` + recoveryBlock + `

### TENDANCE 7 JOURS
| Jour | VFC | Statut | Sommeil | Profond | FC repos | Pas | Étages |
|---|---|---|---|---|---|---|---|
| 28/10 | 46 ms | Équilibré | 81 | 1h31 | 47 | 9 120 | 8 |

### HISTORIQUE 14 JOURS
| Date | Type | Distance | Durée | Allure | FC | Cadence | D- | Load |
|---|---|---|---|---|---|---|---|---|
| 28/10 | Course | 7,02 km | 42:10 | 6:00/km | 134 | 178 | 35 m | 85 |
| 29/10 | Vélo | 6,10 km | 22:10 | — | 110 | — | N/D | N/D |

### ALERTES
- ⚠️ Cadence basse : 168 spm < 170, risque d'overstriding.
`
	got := Reporter{}.Activity(a, config.DefaultProfile())
	assertGolden(t, got, want)
}

func TestDayGolden(t *testing.T) {
	a := sample(analysis.ModeSFP)
	rc := *a.Recovery
	rc.Date = "2026-11-03"
	d := &analysis.DayAnalysis{
		Mode: analysis.ModeRecovery, Date: "2026-11-03",
		Recovery: rc,
		Day: analysis.DayInfo{Steps: fp(8432), StepGoal: fp(8000), FloorsUp: fp(12), FloorsDown: fp(10),
			IntensityModerateMin: fp(25), IntensityVigorousMin: fp(5), StressAvg: fp(32), BodyBatteryCharged: fp(45), BodyBatteryDrained: fp(60)},
		Activities: []analysis.ActivitySummary{
			{Type: "cycling", TypeLabel: "Vélo", DurationS: fp(1330), DistanceM: fp(6100)},
			{Type: "walking", TypeLabel: "Marche", DurationS: fp(2100), DistanceM: fp(2800)},
		},
		Alerts: []analysis.Alert{{Level: "ok", Code: "ALL_GOOD", Title: "RAS", Message: "Aucun signal d'alerte sur cette journée."}},
	}
	want := `### BILAN RÉCUPÉRATION GARMIN - 03/11/2026
- **Mode :** Santé & Récupération (hors séance)
- **Activité du jour :** 8 432 pas (objectif 8 000) | Étages : 12 ↑ / 10 ↓ | Intensité : 25 min mod. / 5 min vig.
- **Activités enregistrées :** Vélo 22:10 (6,1 km) ; Marche 35:00 (2,8 km)
- **Stress moyen :** 32 | **Body Battery :** +45 / -60

### RÉCUPÉRATION & PHYSIOLOGIE (NUIT DU 02/11 AU 03/11)
- **VFC nocturne :** 45 ms (Statut : Équilibré vs baseline 40-55 ms | moy. 7 j : 48 ms)
- **Score Sommeil :** 78/100 (Dont profond : 1h22) | Durée : 7h12
- **FC repos nocturne :** 48 bpm (moy. 7 j : 46 bpm) | **Body Battery réveil :** 82/100

### TENDANCE 7 JOURS
| Jour | VFC | Statut | Sommeil | Profond | FC repos | Pas | Étages |
|---|---|---|---|---|---|---|---|
| 28/10 | 46 ms | Équilibré | 81 | 1h31 | 47 | 9 120 | 8 |

### ALERTES
- ✅ RAS : Aucun signal d'alerte sur cette journée.
`
	got := Reporter{}.Day(d, config.DefaultProfile())
	assertGolden(t, got, want)
}

func TestLapsHeaderAndPauses(t *testing.T) {
	a := sample(analysis.ModeSFP)
	a.Segments = analysis.Segments{Primary: "laps", Laps: []analysis.Segment{
		{Index: 1, Kind: "lap", Label: "Km 1", DurationS: 331, DistanceM: 1000, PaceSecPerKm: fp(331), AvgHR: fp(119)},
	}, Blocks: []analysis.Segment{{Kind: "stand", Label: "Pause 1"}}}
	got := Reporter{}.Activity(a, config.DefaultProfile())
	if !strings.Contains(got, "| Lap | Durée |") || !strings.Contains(got, "| Km 1 | 5:31 | 1,00 km | 5:31/km | 119 bpm | N/D | N/D |") {
		t.Errorf("tableau laps inattendu :\n%s", got)
	}
}

func assertGolden(t *testing.T, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(gl) || i < len(wl); i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			t.Fatalf("ligne %d différente :\n obtenu : %q\n attendu: %q\n--- rapport complet ---\n%s", i+1, g, w, got)
		}
	}
}
