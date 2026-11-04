// Package report génère les rapports texte markdown compacts (§4 du contrat) :
// chiffres FR à virgule, "N/D" si absent, pas d'emoji hors section ALERTES.
package report

import (
	"fmt"
	"strings"
	"time"

	"stride/internal/analysis"
	"stride/internal/config"
	f "stride/internal/frfmt"
)

// Reporter implémente analysis.Reporter.
type Reporter struct{}

var _ analysis.Reporter = Reporter{}

const dash = "—"

func p(v *float64) float64 { return f.P(v) }

// builder : sections séparées par une seule ligne vide.
type builder struct {
	sections []string
	cur      []string
}

func (b *builder) section(title string) {
	b.flush()
	b.cur = append(b.cur, "### "+title)
}

func (b *builder) line(format string, args ...any) {
	b.cur = append(b.cur, fmt.Sprintf(format, args...))
}

// raw ajoute une ligne telle quelle (pas d'interprétation des %).
func (b *builder) raw(s string) { b.cur = append(b.cur, s) }

func (b *builder) flush() {
	if len(b.cur) > 0 {
		b.sections = append(b.sections, strings.Join(b.cur, "\n"))
		b.cur = nil
	}
}

func (b *builder) String() string {
	b.flush()
	return strings.Join(b.sections, "\n\n") + "\n"
}

// ---------- helpers de formatage ----------

func unitInt(v *float64, unit string) string { return f.Unit(p(v), unit) }

func signedM(v float64, sign string) string {
	if !f.Valid(v) {
		return f.ND
	}
	return sign + f.Int(v) + " m"
}

func driftVerdictFR(v string) string {
	switch v {
	case "moderate":
		return "modérée"
	case "high":
		return "élevée"
	}
	return "stable"
}

func acwrVerdictFR(v string) string {
	switch v {
	case "low":
		return "faible"
	case "high":
		return "élevé"
	case "danger":
		return "danger"
	}
	return "optimal"
}

func alertEmoji(level string) string {
	switch level {
	case "danger":
		return "🛑"
	case "warn":
		return "⚠️"
	case "ok":
		return "✅"
	}
	return "ℹ️"
}

// ---------- sections communes ----------

func modeLabel(m analysis.Mode) string {
	switch m {
	case analysis.ModePerformance:
		return "Performance"
	case analysis.ModeRecovery:
		return "Santé & Récupération"
	}
	return "Rééducation SFP"
}

func writeHeader(b *builder, a *analysis.ActivityAnalysis) {
	act := a.Activity
	b.section("RAPPORT SÉANCE GARMIN - " + f.DateTime(act.StartLocal))
	b.line("- **Mode :** %s", modeLabel(a.Mode))
	speed := ""
	if act.IsRunning || act.DistanceM == nil || act.DurationS == nil || p(act.DurationS) <= 0 {
		speed = "Allure moy: " + f.Pace(p(act.AvgPaceSecPerKm))
	} else {
		speed = "Vitesse moy: " + f.Dec(p(act.DistanceM)/p(act.DurationS)*3.6, 1) + " km/h"
	}
	b.line("- **Séance :** %s en %s | %s | Puissance: %s",
		f.Km(p(act.DistanceM), 2), f.Duration(p(act.DurationS)), speed, unitInt(act.AvgPower, "W"))
	b.line("- **Dénivelé :** %s / %s", signedM(p(a.Load.ElevGainM), "+"), signedM(p(a.Load.ElevLossM), "-"))
}

func gctBalanceText(g analysis.GctBalance) string {
	if !g.Available || g.LeftPct == nil || g.RightPct == nil {
		return f.ND + " (non mesuré par le capteur)"
	}
	return fmt.Sprintf("%s %% G / %s %% D (écart %s pt)", f.Dec(*g.LeftPct, 1), f.Dec(*g.RightPct, 1), f.Dec(p(g.GapPct), 1))
}

func gctBalanceShort(g analysis.GctBalance) string {
	if !g.Available || g.LeftPct == nil || g.RightPct == nil {
		return f.ND
	}
	return f.Dec(*g.LeftPct, 1) + "/" + f.Dec(*g.RightPct, 1)
}

func biomechOneLine(bm analysis.Biomechanics) string {
	return fmt.Sprintf("Cadence %s | GCT %s | OV %s | RV %s | Foulée %s | G/D %s",
		unitInt(bm.CadenceAvgSpm, "spm"), unitInt(bm.GroundContactTimeMs, "ms"),
		f.UnitDec(p(bm.VerticalOscillationCm), 1, "cm"), f.UnitDec(p(bm.VerticalRatioPct), 1, "%"),
		f.UnitDec(p(bm.StrideLengthM), 2, "m"), gctBalanceShort(bm.GctBalance))
}

func writeBiomechSFP(b *builder, a *analysis.ActivityAnalysis, prof config.Profile) {
	bm := a.Biomechanics
	b.section("BIOMÉCANIQUE ROTULE & IMPACTS")
	b.line("- **Équilibre contact sol (G/D) :** %s", gctBalanceText(bm.GctBalance))
	b.line("- **Temps de contact sol :** %s", unitInt(bm.GroundContactTimeMs, "ms"))
	b.line("- **Cadence moyenne :** %s (Max : %s) | Dans la cible %s-%s : %s",
		unitInt(bm.CadenceAvgSpm, "spm"), unitInt(bm.CadenceMaxSpm, "spm"),
		f.Int(prof.CadenceTargetMin), f.Int(prof.CadenceTargetMax), unitInt(bm.CadenceInTargetPct, "%"))
	b.line("- **Oscillation verticale :** %s | **Ratio vertical :** %s",
		f.UnitDec(p(bm.VerticalOscillationCm), 1, "cm"), f.UnitDec(p(bm.VerticalRatioPct), 1, "%"))
	b.line("- **Longueur de foulée moyenne :** %s", f.UnitDec(p(bm.StrideLengthM), 2, "m"))
}

func writeSegments(b *builder, sg analysis.Segments) {
	b.section("DÉTAIL DES INTERVALLES / SPLITS")
	rows := sg.Blocks
	head := "Bloc"
	if sg.Primary != "blocks" {
		rows = sg.Laps
		head = "Lap"
	}
	if len(rows) == 0 {
		b.raw("Aucun split disponible.")
		return
	}
	b.line("| %s | Durée | Distance | Allure | FC moy | Watts | Cadence |", head)
	b.raw("|---|---|---|---|---|---|---|")
	for _, s := range rows {
		pace, watts, cad := f.Pace(p(s.PaceSecPerKm)), unitInt(s.AvgPower, "W"), unitInt(s.AvgCadenceSpm, "spm")
		switch s.Kind {
		case "walk":
			pace, watts, cad = "Marche", dash, dash
		case "stand":
			pace, watts, cad = "Arrêt", dash, dash
		}
		b.line("| %s | %s | %s | %s | %s | %s | %s |", s.Label, f.Duration(s.DurationS), f.Km(s.DistanceM, 2), pace,
			unitInt(s.AvgHR, "bpm"), watts, cad)
	}
}

func driftLine(d *analysis.Drift, withThird bool) string {
	if d == nil {
		return "- **Dérive cardiaque :** " + f.ND
	}
	s := fmt.Sprintf("- **Dérive cardiaque (%s) :** %s %% (%s)", analysis.DriftMethodLabel(d.Method), f.Signed(d.DecouplingPct, 1), driftVerdictFR(d.Verdict))
	if withThird {
		s += fmt.Sprintf(" | FC dernier tiers : %s bpm", f.Signed(d.LastThirdHrRiseBpm, 0))
	}
	return s
}

func writeLoadSFP(b *builder, a *analysis.ActivityAnalysis) {
	b.section("CHARGE & FATIGUE")
	b.raw(driftLine(a.Load.Drift, true))
	b.line("- **Training Load :** %s | **TE aérobie :** %s | **Impact load :** %s",
		f.Or(p(a.Load.TrainingLoad), f.Int), f.Or(p(a.Load.AerobicTE), func(v float64) string { return f.Dec(v, 1) }),
		f.Or(p(a.Activity.ImpactLoad), f.Int))
}

func writeRecovery(b *builder, date string, rc *analysis.RecoveryContext) {
	b.section(fmt.Sprintf("RÉCUPÉRATION & PHYSIOLOGIE (NUIT DU %s AU %s)", f.DayMonth(prevDay(date)), f.DayMonth(date)))
	if rc == nil {
		rc = &analysis.RecoveryContext{}
	}
	// VFC
	if h := rc.HRV; h != nil {
		status := f.ND
		if h.StatusLabel != nil {
			status = *h.StatusLabel
		}
		base := f.ND
		if h.BaselineLowMs != nil && h.BaselineHighMs != nil {
			base = f.Int(*h.BaselineLowMs) + "-" + f.Int(*h.BaselineHighMs) + " ms"
		}
		b.line("- **VFC nocturne :** %s (Statut : %s vs baseline %s | moy. 7 j : %s)",
			unitInt(h.LastNightAvgMs, "ms"), status, base, unitInt(h.WeeklyAvgMs, "ms"))
	} else {
		b.raw("- **VFC nocturne :** " + f.ND)
	}
	// Sommeil
	if sl := rc.Sleep; sl != nil {
		score := f.ND
		if sl.Score != nil {
			score = f.Int(*sl.Score) + "/100"
		}
		b.line("- **Score Sommeil :** %s (Dont profond : %s) | Durée : %s", score, f.SleepHM(p(sl.DeepS)), f.SleepHM(p(sl.DurationS)))
	} else {
		b.raw("- **Score Sommeil :** " + f.ND)
	}
	// FC repos + Body Battery
	rhr, avg := f.ND, f.ND
	if r := rc.RestingHR; r != nil {
		rhr, avg = unitInt(r.Bpm, "bpm"), unitInt(r.Avg7d, "bpm")
	}
	bb := f.ND
	if rc.BodyBattery != nil && rc.BodyBattery.AtWake != nil {
		bb = f.Int(*rc.BodyBattery.AtWake) + "/100"
	}
	b.line("- **FC repos nocturne :** %s (moy. 7 j : %s) | **Body Battery réveil :** %s", rhr, avg, bb)
}

func writeTrend(b *builder, rc *analysis.RecoveryContext) {
	b.section("TENDANCE 7 JOURS")
	b.raw("| Jour | VFC | Statut | Sommeil | Profond | FC repos | Pas | Étages |")
	b.raw("|---|---|---|---|---|---|---|---|")
	if rc == nil {
		return
	}
	for _, t := range rc.Trend {
		status := f.ND
		if t.HrvStatus != nil {
			status = analysis.HRVStatusLabel(*t.HrvStatus)
		}
		b.line("| %s | %s | %s | %s | %s | %s | %s | %s |", f.DayMonth(t.Date), unitInt(t.HrvMs, "ms"), status,
			f.Or(p(t.SleepScore), f.Int), f.SleepHM(p(t.DeepS)), f.Or(p(t.RestingHR), f.Int), f.Or(p(t.Steps), f.Int), f.Or(p(t.FloorsUp), f.Int))
	}
}

func writeHistory(b *builder, days int, rows []analysis.HistoryRow) {
	b.section(fmt.Sprintf("HISTORIQUE %d JOURS", days))
	if len(rows) == 0 {
		b.raw("Aucune activité sur la période.")
		return
	}
	b.raw("| Date | Type | Distance | Durée | Allure | FC | Cadence | D- | Load |")
	b.raw("|---|---|---|---|---|---|---|---|---|")
	for _, r := range rows {
		label := r.TypeLabel
		if label == "" {
			label = analysis.TypeLabel(r.Type)
		}
		pace, cad := dash, dash
		if analysis.IsRunningType(r.Type) {
			pace, cad = f.Pace(p(r.PaceSecPerKm)), f.Or(p(r.AvgCadenceSpm), f.Int)
		}
		b.line("| %s | %s | %s | %s | %s | %s | %s | %s | %s |", f.DayMonth(r.Date), label, f.Km(p(r.DistanceM), 2),
			f.Duration(p(r.DurationS)), pace, f.Or(p(r.AvgHR), f.Int), cad, unitInt(r.ElevLossM, "m"), f.Or(p(r.TrainingLoad), f.Int))
	}
}

func writeAlerts(b *builder, alerts []analysis.Alert) {
	b.section("ALERTES")
	if len(alerts) == 0 {
		b.raw("- Aucune alerte.")
		return
	}
	for _, a := range alerts {
		b.line("- %s %s : %s", alertEmoji(a.Level), a.Title, a.Message)
	}
}

func prevDay(date string) string {
	if len(date) < 10 {
		return date
	}
	return analysisDateAdd(date, -1)
}

// ---------- rapports ----------

// Activity génère le rapport d'une séance selon le mode.
func (Reporter) Activity(a *analysis.ActivityAnalysis, prof config.Profile) string {
	var b builder
	writeHeader(&b, a)
	switch a.Mode {
	case analysis.ModePerformance:
		writePerformance(&b, a)
		b.section("BIOMÉCANIQUE (RÉSUMÉ)")
		b.raw("- " + biomechOneLine(a.Biomechanics))
		writeSegments(&b, a.Segments)
		writeRecovery(&b, a.Activity.Date, a.Recovery)
	case analysis.ModeRecovery:
		b.raw("- **Biomécanique :** " + biomechOneLine(a.Biomechanics))
		writeRecovery(&b, a.Activity.Date, a.Recovery)
		writeTrend(&b, a.Recovery)
	default:
		writeBiomechSFP(&b, a, prof)
		writeSegments(&b, a.Segments)
		writeLoadSFP(&b, a)
		writeRecovery(&b, a.Activity.Date, a.Recovery)
	}
	if a.History != nil && a.HistoryDays > 0 {
		writeHistory(&b, a.HistoryDays, a.History)
	}
	writeAlerts(&b, a.Alerts)
	return b.String()
}

func writePerformance(b *builder, a *analysis.ActivityAnalysis) {
	pf := a.Performance
	if pf == nil {
		pf = &analysis.Performance{}
	}
	b.section("PERFORMANCE")
	b.line("- **Puissance :** moy %s | normalisée %s", unitInt(pf.AvgPower, "W"), unitInt(pf.NormPower, "W"))
	if z := pf.TimeInZonesS; len(z) == 5 {
		b.line("- **Zones FC :** Z1 %s | Z2 %s | Z3 %s | Z4 %s | Z5 %s (Z4+Z5 : %s)",
			f.Duration(z[0]), f.Duration(z[1]), f.Duration(z[2]), f.Duration(z[3]), f.Duration(z[4]), f.Duration(z[3]+z[4]))
	} else {
		b.raw("- **Zones FC :** " + f.ND)
	}
	if len(pf.TargetPaces) > 0 {
		parts := make([]string, 0, len(pf.TargetPaces))
		for _, t := range pf.TargetPaces {
			val := f.Duration(t.Seconds)
			if t.Seconds > 0 {
				val += " sur " + f.Km(t.DistanceM, 2)
			}
			parts = append(parts, fmt.Sprintf("**%s (%s) :** %s", t.Label, strings.ReplaceAll(t.RangeLabel, "–", "-"), val))
		}
		b.raw("- " + strings.Join(parts, " | "))
	}
	b.raw(driftLine(a.Load.Drift, false))
	if ac := pf.ACWR; ac != nil {
		ratio := f.ND
		if ac.Ratio != nil {
			ratio = f.Dec(*ac.Ratio, 2)
		}
		b.line("- **Charge aiguë/chronique :** %s / %s (ACWR %s – %s)", f.Int(ac.Acute7d), f.Int(ac.Chronic28d), ratio, acwrVerdictFR(ac.Verdict))
	} else {
		b.raw("- **Charge aiguë/chronique :** " + f.ND)
	}
	b.line("- **Training Load séance :** %s | **TE :** %s aéro / %s anaéro", f.Or(p(a.Load.TrainingLoad), f.Int),
		f.Or(p(a.Load.AerobicTE), func(v float64) string { return f.Dec(v, 1) }),
		f.Or(p(a.Load.AnaerobicTE), func(v float64) string { return f.Dec(v, 1) }))
}

// Day génère le bilan « Santé & Récupération » d'une journée.
func (Reporter) Day(d *analysis.DayAnalysis, _ config.Profile) string {
	var b builder
	b.section("BILAN RÉCUPÉRATION GARMIN - " + f.Date(d.Date))
	b.raw("- **Mode :** Santé & Récupération (hors séance)")
	day := d.Day
	steps := f.ND
	if day.Steps != nil {
		steps = f.Int(*day.Steps) + " pas"
		if day.StepGoal != nil {
			steps += " (objectif " + f.Int(*day.StepGoal) + ")"
		}
	}
	b.line("- **Activité du jour :** %s | Étages : %s ↑ / %s ↓ | Intensité : %s mod. / %s vig.", steps,
		f.Or(p(day.FloorsUp), f.Int), f.Or(p(day.FloorsDown), f.Int),
		unitInt(day.IntensityModerateMin, "min"), unitInt(day.IntensityVigorousMin, "min"))
	if len(d.Activities) == 0 {
		b.raw("- **Activités enregistrées :** Aucune")
	} else {
		parts := make([]string, 0, len(d.Activities))
		for _, a := range d.Activities {
			s := a.TypeLabel + " " + f.Duration(p(a.DurationS))
			if a.DistanceM != nil && *a.DistanceM >= 50 {
				s += " (" + f.Km(*a.DistanceM, 1) + ")"
			}
			parts = append(parts, s)
		}
		b.raw("- **Activités enregistrées :** " + strings.Join(parts, " ; "))
	}
	charged, drained := f.ND, f.ND
	if day.BodyBatteryCharged != nil {
		charged = "+" + f.Int(*day.BodyBatteryCharged)
	}
	if day.BodyBatteryDrained != nil {
		drained = "-" + f.Int(*day.BodyBatteryDrained)
	}
	b.line("- **Stress moyen :** %s | **Body Battery :** %s / %s", f.Or(p(day.StressAvg), f.Int), charged, drained)
	rc := d.Recovery
	writeRecovery(&b, d.Date, &rc)
	writeTrend(&b, &rc)
	if d.History != nil && d.HistoryDays > 0 {
		writeHistory(&b, d.HistoryDays, d.History)
	}
	writeAlerts(&b, d.Alerts)
	return b.String()
}

func analysisDateAdd(d string, n int) string {
	t, err := time.Parse("2006-01-02", d[:10])
	if err != nil {
		return d
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}
