package report

import (
	"fmt"
	"strings"

	"stride/internal/analysis"
	"stride/internal/athlete"
	f "stride/internal/frfmt"
	"stride/internal/view"
)

// Day renders the day / health prompt (§6.3).
func Day(v *view.DayView, prof athlete.Profile, inc Include) string {
	b := newBuilder()
	writeReadinessPriority(b, v.Readiness)
	if inc.Has(ChipPlan) {
		today := "Aucune"
		if len(v.Planned) > 0 {
			parts := make([]string, 0, len(v.Planned))
			for _, ps := range v.Planned {
				parts = append(parts, describePlanned(ps, true))
			}
			today = strings.Join(parts, " ; ")
		}
		writeContext(b, prof, v.Goals, today, v.Upcoming)
	}
	writeDayBalance(b, v)
	if inc.Has(ChipHealth) {
		writeRecovery(b, v.Recovery)
		writeTrend(b, v.Recovery, inc.Has(ChipPain))
	}
	if inc.Has(ChipHistory) {
		writeHistory(b, v.History, inc.Has(ChipPain))
	}
	writeAlerts(b, "ALERTES AUTOMATIQUES", v.Alerts)
	return b.String()
}

func writeDayBalance(b *builder, v *view.DayView) {
	d := v.Day
	b.section("BILAN SANTÉ & CHARGE QUOTIDIENNE - " + f.Date(v.Date))
	steps := f.ND
	if d.Steps != nil {
		steps = f.Int(*d.Steps)
		if d.StepGoal != nil {
			steps += " (objectif " + f.Int(*d.StepGoal) + ")"
		}
	}
	b.line("- **Pas :** %s | **Étages :** %s ↑ / %s ↓ | **Intensité :** %s mod. / %s vig.", steps,
		intOr(d.FloorsUp), intOr(d.FloorsDown), unit(d.IntensityModerateMin, "min"), unit(d.IntensityVigorousMin, "min"))
	charged, drained := f.ND, f.ND
	if d.BodyBatteryCharged != nil {
		charged = "+" + f.Int(*d.BodyBatteryCharged)
	}
	if d.BodyBatteryDrained != nil {
		drained = "-" + f.Int(*d.BodyBatteryDrained)
	}
	b.line("- **Stress moyen :** %s | **Body Battery :** %s / %s | **Calories actives :** %s", outOf100(d.StressAvg), charged, drained, unit(d.ActiveCalories, "kcal"))

	bike := view.BikeLoad{}
	found := false
	for _, bd := range v.CrossLoad.BikeByDay {
		if bd.Date == v.Date {
			bike, found = bd.BikeLoad, true
		}
	}
	var others []string
	for _, a := range v.Activities {
		if a.Sport == "bike" {
			if !found {
				bike.Rides++
				bike.DistanceKm += p(a.DistanceM) / 1000
				bike.DurationS += p(a.DurationS)
			}
			continue
		}
		s := a.TypeLabel + " " + f.Duration(p(a.DurationS))
		if a.DistanceM != nil && *a.DistanceM >= 50 {
			s += " (" + f.Km(*a.DistanceM, 1) + ")"
		}
		others = append(others, s)
	}
	bikeLine := "Aucune sortie"
	if bike.Rides > 0 {
		bikeLine = fmt.Sprintf("%s, %s", plural(bike.Rides, "sortie", "sorties"), bikeText(bike))
	}
	otherLine := "Aucune"
	if len(others) > 0 {
		otherLine = strings.Join(others, " ; ")
	}
	b.line("- **Vélo :** %s | **Autres activités :** %s", bikeLine, otherLine)

	planned := "Aucune"
	if len(v.Planned) > 0 {
		parts := make([]string, 0, len(v.Planned))
		for _, ps := range v.Planned {
			s := describePlanned(ps, false)
			if m := ps.Match; m != nil {
				if l, ok := matchStatusLabels[m.Status]; ok {
					s += " – " + l
				}
				if sum := strings.TrimSpace(m.Summary); sum != "" {
					s += " (" + sum + ")"
				}
			}
			parts = append(parts, s)
		}
		planned = strings.Join(parts, " ; ")
	}
	b.raw("- **Séance prévue :** " + planned)
}

func writeTrend(b *builder, rc *analysis.RecoveryContext, withPain bool) {
	b.section("TENDANCE 7 JOURS")
	if rc == nil || len(rc.Trend) == 0 {
		b.raw("Aucune donnée disponible.")
		return
	}
	if withPain {
		b.raw("| Jour | VFC | Statut | Sommeil | Profond | FC repos | Pas | Étages | Load | Gêne lendemain |")
		b.raw("|---|---|---|---|---|---|---|---|---|---|")
	} else {
		b.raw("| Jour | VFC | Statut | Sommeil | Profond | FC repos | Pas | Étages | Load |")
		b.raw("|---|---|---|---|---|---|---|---|---|")
	}
	for _, t := range rc.Trend {
		status := f.ND
		if t.HrvStatus != nil {
			status = analysis.HRVStatusLabel(*t.HrvStatus)
		}
		cells := []string{f.DayMonth(t.Date), unit(t.HrvMs, "ms"), status, intOr(t.SleepScore), f.SleepHM(p(t.DeepS)),
			intOr(t.RestingHR), intOr(t.Steps), intOr(t.FloorsUp), intOr(t.TrainingLoad)}
		if withPain {
			cells = append(cells, pain(t.PainNextMorning))
		}
		b.raw("| " + strings.Join(cells, " | ") + " |")
	}
}
