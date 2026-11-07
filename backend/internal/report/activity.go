package report

import (
	"fmt"
	"strings"

	"stride/internal/analysis"
	"stride/internal/athlete"
	f "stride/internal/frfmt"
	"stride/internal/view"
)

// Activity renders the single-session prompt (§6.2).
func Activity(v *view.ActivityView, prof athlete.Profile, inc Include) string {
	b := newBuilder()
	writeCoachNotes(b, v.CoachNotes)
	writeReadinessPriority(b, v.Readiness)
	if inc.Has(ChipPain) {
		writeBodyStress(b, v.BodyStress, v.BodyCorrelations)
	}
	if inc.Has(ChipPlan) {
		writeContext(b, prof, v.Health, v.Goals, plannedTodayText(v), v.Upcoming, v.Records)
	}
	writeSession(b, v, inc.Has(ChipPain))
	if inc.Has(ChipBiomech) {
		writeBiomech(b, v, prof)
	}
	if inc.Has(ChipSplits) {
		writeSplits(b, v.Segments)
	}
	if inc.Has(ChipLoad) {
		writeLoad(b, v)
	}
	if inc.Has(ChipHealth) {
		writeRecovery(b, v.Recovery)
	}
	if inc.Has(ChipHistory) {
		writeHistory(b, v.History, inc.Has(ChipPain))
	}
	writeAlerts(b, "ALERTES AUTOMATIQUES", v.Alerts)
	return b.String()
}

func plannedTodayText(v *view.ActivityView) string {
	s := ""
	if v.PlannedToday != nil {
		s = describePlanned(*v.PlannedToday, true)
	}
	if v.Plan != nil && strings.TrimSpace(v.Plan.Summary) != "" {
		if s == "" {
			return v.Plan.Summary
		}
		s += " → " + v.Plan.Summary
	}
	if s == "" {
		return "Aucune"
	}
	return s
}

func weatherText(w *view.Weather) string {
	if w == nil {
		return f.ND
	}
	temp := f.ND
	if w.TemperatureC != nil {
		temp = f.Int(*w.TemperatureC) + "°C"
	}
	hum := f.ND
	if w.HumidityPct != nil {
		hum = f.Int(*w.HumidityPct) + "%"
	}
	wind := f.ND
	if w.WindKph != nil {
		wind = f.Int(*w.WindKph) + " km/h"
		if w.WindDirLabel != nil && *w.WindDirLabel != "" {
			wind += " (" + *w.WindDirLabel + ")"
		}
	}
	return fmt.Sprintf("%s, Hum: %s, Vent: %s", temp, hum, wind)
}

func notesText(v *view.ActivityView) string {
	var parts []string
	rpe := v.Notes.RPE
	if rpe == nil && v.Activity.Strava != nil {
		rpe = v.Activity.Strava.RPE
	}
	if rpe != nil {
		parts = append(parts, "RPE "+f.Num(*rpe, 1)+"/10")
	}
	if n := compact(v.Notes.Note, 280); n != "" {
		parts = append(parts, n)
	}
	if st := v.Activity.Strava; st != nil {
		if d := compact(st.Description, 280); d != "" {
			parts = append(parts, "Strava : "+d)
		}
		if pn := compact(st.PrivateNote, 280); pn != "" {
			parts = append(parts, "Note privée : "+pn)
		}
	}
	if len(parts) == 0 {
		return f.ND
	}
	return strings.Join(parts, " – ")
}

func shoeText(v *view.ActivityView) string {
	if s := v.Notes.Shoe; s != nil {
		name := strings.TrimSpace(s.Model)
		if name != "" && strings.TrimSpace(s.Brand) != "" {
			name = strings.TrimSpace(s.Brand) + " " + name
		}
		if name == "" {
			name = s.Name
		}
		drop := f.ND
		if s.DropMm != nil {
			drop = f.Num(*s.DropMm, 1) + " mm"
		}
		return fmt.Sprintf("%s (Drop: %s, Type: %s)", name, drop, athlete.ShoeTypeLabel(s.Type))
	}
	if st := v.Activity.Strava; st != nil && st.GearName != nil && *st.GearName != "" {
		return *st.GearName + " (Drop: N/D, Type: N/D)"
	}
	return f.ND
}

func writeSession(b *builder, v *view.ActivityView, withPain bool) {
	a := v.Activity
	b.section("RAPPORT SÉANCE - " + f.DateTime(a.StartLocal))
	src := v.SourceLabel
	if src == "" {
		src = f.ND
	}
	b.line("- **Source :** %s | **Météo :** %s", src, weatherText(v.Weather))
	speed := ""
	if a.IsRunning {
		speed = fmt.Sprintf("Allure: %s (GAP: %s)", pace(a.AvgPaceSecPerKm), pace(a.GapSecPerKm))
	} else if a.DistanceM != nil && a.DurationS != nil && *a.DurationS > 0 {
		speed = "Vitesse: " + f.Dec(*a.DistanceM / *a.DurationS * 3.6, 1) + " km/h"
	} else {
		speed = "Vitesse: " + f.ND
	}
	b.line("- **Réalisé :** %s en %s | %s | Puissance: %s", km(a.DistanceM), f.Duration(p(a.DurationS)), speed, unit(a.AvgPower, "W"))
	gain, loss := v.Load.ElevGainM, v.Load.ElevLossM
	if gain == nil {
		gain = a.ElevGainM
	}
	if loss == nil {
		loss = a.ElevLossM
	}
	b.line("- **Dénivelé :** %s / %s | **RPE / Notes :** %s", signedM(gain, "+"), signedM(loss, "-"), notesText(v))
	b.raw("- **Chaussures :** " + shoeText(v))
	if withPain {
		tags := "Aucun"
		if len(v.Notes.Tags) > 0 {
			labels := make([]string, 0, len(v.Notes.Tags))
			for _, t := range v.Notes.Tags {
				labels = append(labels, athlete.TagLabel(t))
			}
			tags = strings.Join(labels, ", ")
		}
		b.line("- **Ressenti & Douleur :** Effort: %s | À froid: %s | Lendemain: %s | Contexte: %s",
			pain(v.Notes.PainDuring), pain(v.Notes.PainCold), pain(v.Notes.PainNextMorning), tags)
	}
}

func writeBiomech(b *builder, v *view.ActivityView, prof athlete.Profile) {
	bm := v.Biomechanics
	fd := bm.FatigueDrift
	if fd == nil {
		fd = &analysis.FatigueDrift{}
	}
	sl := bm.Slope
	var flat, down *analysis.SlopeBucket
	if sl != nil {
		flat, down = sl.Flat, sl.Downhill
	}
	bucketCad := func(bk *analysis.SlopeBucket) string {
		if bk == nil {
			return f.ND
		}
		return unit(bk.CadenceSpm, "spm")
	}
	th := prof.Thresholds
	b.section("BIOMÉCANIQUE & DÉRIVE DE FATIGUE")
	b.line("- **Cadence moy :** %s (Max: %s) | %% dans la cible (%s-%s): %s | **Dérive Cadence :** %s",
		unit(bm.CadenceAvgSpm, "spm"), intOr(bm.CadenceMaxSpm), f.Num(th.CadenceTargetMin, 1), f.Num(th.CadenceTargetMax, 1),
		unit(bm.CadenceInTargetPct, "%"), signedUnit(fd.CadenceDeltaSpm, 0, "spm"))
	b.line("- **Cadence selon pente :** Plat: %s | Descente (<-1.5%%): %s", bucketCad(flat), bucketCad(down))
	gctDrift := f.ND
	if fd.GctDeltaMs != nil {
		gctDrift = signedUnit(fd.GctDeltaMs, 0, "ms")
		if fd.GctDeltaPct != nil {
			gctDrift += " (" + signedUnit(fd.GctDeltaPct, 1, "%") + ")"
		}
	}
	b.line("- **Temps de contact sol (GCT) :** %s | **Dérive GCT (Début vs Fin) :** %s", unit(bm.GroundContactTimeMs, "ms"), gctDrift)
	b.line("- **Oscillation verticale :** %s (Ratio: %s) | **Dérive OV :** %s",
		f.UnitDec(p(bm.VerticalOscillationCm), 1, "cm"), f.UnitDec(p(bm.VerticalRatioPct), 1, "%"), signedUnit(fd.VoDeltaCm, 1, "cm"))
	downStride := f.ND
	if down != nil {
		downStride = f.UnitDec(p(down.StrideM), 2, "m")
	}
	b.line("- **Longueur de foulée :** Moy: %s (En descente: %s)", f.UnitDec(p(bm.StrideLengthM), 2, "m"), downStride)
	g := bm.GctBalance
	bal := f.ND
	if g.Available && g.LeftPct != nil && g.RightPct != nil {
		bal = f.Dec(*g.LeftPct, 1) + " % / " + f.Dec(*g.RightPct, 1) + " %"
	} else if g.Reason != nil && *g.Reason != "" {
		bal = f.ND + " (" + *g.Reason + ")"
	}
	b.raw("- **Équilibre G/D :** " + bal)
}

func writeSplits(b *builder, sg analysis.Segments) {
	b.section("DÉTAIL DES INTERVALLES / SPLITS")
	rows := sg.PrimaryList()
	if len(rows) == 0 {
		b.raw("Aucun split disponible.")
		return
	}
	head := "Bloc"
	if sg.Primary != "blocks" {
		head = "Lap"
	}
	b.raw("| " + head + " | Durée | Dist. | Allure | GAP | FC moy | Watts | Cadence | GCT | D+/D- |")
	b.raw("|---|---|---|---|---|---|---|---|---|---|")
	for _, s := range rows {
		pc, gap, watts, cad, gct := pace(s.PaceSecPerKm), pace(s.GapSecPerKm), unit(s.AvgPower, "W"), unit(s.AvgCadenceSpm, "spm"), unit(s.GctMs, "ms")
		switch s.Kind {
		case "walk":
			pc, gap, watts, cad, gct = "Marche", dash, dash, dash, dash
		case "stand":
			pc, gap, watts, cad, gct = "Arrêt", dash, dash, dash, dash
		}
		b.raw("| " + strings.Join([]string{s.Label, f.Duration(s.DurationS), f.Km(s.DistanceM, 2), pc, gap,
			unit(s.AvgHR, "bpm"), watts, cad, gct, elevPair(s.ElevGainM, s.ElevLossM)}, " | ") + " |")
	}
}

func writeLoad(b *builder, v *view.ActivityView) {
	l := v.Load
	b.section("CHARGE, STAMINA & IMPACT QUOTIDIEN (48H)")
	drift := "- **Dérive cardiaque :** " + f.ND
	if d := l.Drift; d != nil {
		drift = fmt.Sprintf("- **Dérive cardiaque (%s) :** %s %%", analysis.DriftMethodLabel(d.Method), f.Signed(d.DecouplingPct, 1))
	}
	stamina := f.ND
	if st := l.Stamina; st != nil {
		stamina = fmt.Sprintf("Début %s -> Fin %s (Min: %s)", unit(st.StartPct, "%"), unit(st.EndPct, "%"), unit(st.MinPct, "%"))
	}
	b.raw(drift + " | **Stamina Fenix 8 :** " + stamina)
	impact := l.ImpactLoad
	if impact == nil {
		impact = v.Activity.ImpactLoad
	}
	re := l.StravaRelativeEffort
	if re == nil && v.Activity.Strava != nil {
		re = v.Activity.Strava.RelativeEffort
	}
	b.line("- **Charge séance :** Garmin Load %s | Impact Load: %s | TE Aérobie: %s | Strava Relative Effort: %s",
		intOr(l.TrainingLoad), intOr(impact), decOr(l.AerobicTE, 1), intOr(re))
	if lr := l.LoadRatio; lr != nil {
		b.line("- **Ratio de charge global :** %s (Aiguë: %s / Chronique: %s)", decOr(lr.Ratio, 2), f.Int(lr.Acute), f.Int(lr.Chronic))
	} else {
		b.raw("- **Ratio de charge global :** " + f.ND)
	}
	b.raw(efficiencyText(v.Efficiency))
	b.raw(impactText(v.Impact))
	cl := v.CrossLoad
	b.line("- **Charge croisée (48h) :** Vélo: %s | Escaliers (J-1/J): %s | Pas (J-1): %s | Stress J-1: %s",
		bikeText(cl.Bike), floorsText(cl.Floors.PrevDayUp, cl.Floors.DayUp), intOr(cl.Steps.PrevDay), outOf100(cl.StressPrevDay))
}

func bikeText(bk view.BikeLoad) string {
	if bk.Rides == 0 {
		return "aucune sortie"
	}
	return fmt.Sprintf("%s km / %s min (Load: %s)", f.Dec(bk.DistanceKm, 1), f.Int(bk.DurationS/60), intOr(bk.TrainingLoad))
}

func floorsText(prev, day *float64) string {
	if prev == nil && day == nil {
		return f.ND
	}
	return intOr(prev) + " / " + intOr(day) + " étages"
}

func efficiencyText(ef *analysis.Efficiency) string {
	if ef == nil || ef.EfSpeed == nil {
		reason := ""
		if ef != nil && ef.Reason != nil {
			reason = " (" + *ef.Reason + ")"
		}
		return "- **Efficience aérobie (EF) :** " + f.ND + reason
	}
	trend := f.ND
	if ef.TrendChangePct != nil {
		trend = f.Signed(*ef.TrendChangePct, 0) + " %"
	}
	return fmt.Sprintf("- **Efficience aérobie (EF) :** %s m/min/bpm | Pw:HR %s W/bpm (tendance 8 sem. : %s)",
		decOr(ef.EfSpeed, 2), decOr(ef.EfPower, 2), trend)
}

func impactText(im *analysis.Impact) string {
	if im == nil || im.Steps == nil {
		return "- **Charge d'impact cumulée :** " + f.ND
	}
	return fmt.Sprintf("- **Charge d'impact cumulée :** %s foulées | Indice articulaire %s (OV ×%s · D- ×%s) | 7 j : %s",
		intOr(im.Steps), decOr(im.JointIndex, 1), decOr(im.VoFactor, 2), decOr(im.DescentFactor, 2), decOr(im.JointIndex7d, 1))
}
