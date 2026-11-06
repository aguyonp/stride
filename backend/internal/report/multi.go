package report

import (
	"fmt"
	"strings"

	"stride/internal/athlete"
	f "stride/internal/frfmt"
	"stride/internal/view"
)

type column struct {
	head string
	cell func(i int) string
}

func writeTable(b *builder, cols []column, n int) {
	heads := make([]string, len(cols))
	seps := make([]string, len(cols))
	for i, c := range cols {
		heads[i], seps[i] = c.head, "---"
	}
	b.raw("| " + strings.Join(heads, " | ") + " |")
	b.raw("|" + strings.Join(seps, "|") + "|")
	for r := 0; r < n; r++ {
		cells := make([]string, len(cols))
		for i, c := range cols {
			cells[i] = c.cell(r)
		}
		b.raw("| " + strings.Join(cells, " | ") + " |")
	}
}

func painTriple(d, c, n *float64) string {
	if d == nil && c == nil && n == nil {
		return f.ND
	}
	num := func(v *float64) string { return f.Or(p(v), func(x float64) string { return f.Num(x, 1) }) }
	return num(d) + " / " + num(c) + " / " + num(n)
}

func numOr(v *float64, n int) string {
	return f.Or(p(v), func(x float64) string { return f.Num(x, n) })
}

// Multi renders the multi-session export (§6.4, no splits).
func Multi(m *view.MultiExport, prof athlete.Profile, inc Include) string {
	b := newBuilder()
	writeBodyStress(b, m.BodyStress)
	if inc.Has(ChipPlan) {
		b.section("CONTEXTE & OBJECTIFS DE L'ATHLÈTE")
		macro, others := splitGoals(m.Goals)
		macroS, othersS := f.ND, "Aucun"
		if macro != nil {
			macroS = goalText(*macro)
		}
		if len(others) > 0 {
			parts := make([]string, 0, len(others))
			for _, g := range others {
				parts = append(parts, goalText(g))
			}
			othersS = strings.Join(parts, " ; ")
		}
		b.line("- **Profil / Mode actif :** %s | **Objectif Macro :** %s | **Autres objectifs :** %s", prof.ModeLabel(), macroS, othersS)
		b.line("- **Période analysée :** %s → %s (%s, %s)", f.Date(m.From), f.Date(m.To),
			plural(m.Weeks, "semaine", "semaines"), plural(len(m.Sessions), "séance", "séances"))
		b.raw("- **Records personnels :** " + recordsText(m.Records))
		b.raw("- **Prochaines séances prévues :** " + upcomingText(m.Upcoming))
	}

	if inc.Has(ChipHistory) {
		b.section("RÉCAPITULATIF DES SÉANCES")
		if len(m.Sessions) == 0 {
			b.raw("Aucune séance sur la période.")
		} else {
			ss := m.Sessions
			s := func(i int) view.ActivitySummary { return ss[i].Summary }
			cols := []column{
				{"Date", func(i int) string { return f.DayMonth(s(i).Date) }},
				{"Type", func(i int) string { return s(i).TypeLabel }},
				{"Dist.", func(i int) string { return km(s(i).DistanceM) }},
				{"Durée", func(i int) string { return f.Duration(p(s(i).DurationS)) }},
				{"Allure", func(i int) string { return pace(s(i).AvgPaceSecPerKm) }},
				{"GAP", func(i int) string { return pace(s(i).GapSecPerKm) }},
				{"FC moy", func(i int) string { return intOr(s(i).AvgHR) }},
			}
			if inc.Has(ChipBiomech) {
				cols = append(cols,
					column{"Cadence", func(i int) string { return unit(s(i).AvgCadenceSpm, "spm") }},
					column{"GCT", func(i int) string { return unit(ss[i].GctMs, "ms") }},
					column{"Dérive GCT", func(i int) string { return signedUnit(ss[i].GctDriftPct, 1, "%") }})
			}
			cols = append(cols, column{"D-", func(i int) string { return unit(s(i).ElevLossM, "m") }})
			if inc.Has(ChipLoad) {
				cols = append(cols, column{"Load", func(i int) string { return intOr(s(i).TrainingLoad) }})
			}
			cols = append(cols, column{"Chaussures", func(i int) string { return str(ss[i].ShoeName) }})
			if inc.Has(ChipPain) {
				cols = append(cols, column{"Gêne E/F/L", func(i int) string {
					return painTriple(ss[i].PainDuring, ss[i].PainCold, ss[i].PainNextMorning)
				}})
			}
			writeTable(b, cols, len(ss))
			if m.Unanalyzed > 0 && inc.Has(ChipBiomech) {
				b.raw(fmt.Sprintf("_%s sans analyse détaillée disponible (GCT / dérive : N/D)._", plural(m.Unanalyzed, "séance", "séances")))
			}
		}
	}

	b.section("TENDANCES HEBDOMADAIRES")
	if len(m.Weekly) == 0 {
		b.raw("Aucune donnée sur la période.")
	} else {
		ws := m.Weekly
		cols := []column{
			{"Semaine", func(i int) string { return f.DayMonth(ws[i].WeekStart) }},
			{"Séances", func(i int) string { return fmt.Sprint(ws[i].Sessions) }},
			{"Volume", func(i int) string { return f.Km(ws[i].DistanceM, 1) }},
			{"Durée", func(i int) string { return f.SleepHM(ws[i].DurationS) }},
		}
		if inc.Has(ChipLoad) {
			cols = append(cols,
				column{"Load", func(i int) string { return intOr(ws[i].Load) }},
				column{"Ratio charge", func(i int) string { return decOr(ws[i].LoadRatio, 2) }},
				column{"EF moy", func(i int) string { return decOr(ws[i].EfAvg, 2) }},
				column{"Indice impacts", func(i int) string { return decOr(ws[i].JointIndex, 1) }})
		}
		if inc.Has(ChipBiomech) {
			cols = append(cols,
				column{"Cadence moy", func(i int) string { return unit(ws[i].CadenceAvg, "spm") }},
				column{"GCT moy", func(i int) string { return unit(ws[i].GctAvg, "ms") }})
		}
		if inc.Has(ChipHealth) {
			cols = append(cols,
				column{"VFC moy", func(i int) string { return unit(ws[i].HrvAvg, "ms") }},
				column{"Sommeil moy", func(i int) string { return intOr(ws[i].SleepScoreAvg) }})
		}
		if inc.Has(ChipPain) {
			cols = append(cols,
				column{"Gêne effort", func(i int) string { return numOr(ws[i].PainDuring, 1) }},
				column{"Gêne froid", func(i int) string { return numOr(ws[i].PainCold, 1) }},
				column{"Gêne lendemain", func(i int) string { return numOr(ws[i].PainNextMorning, 1) }})
		}
		writeTable(b, cols, len(ws))
	}

	if inc.Has(ChipPlan) {
		b.section("PRÉVU vs RÉALISÉ")
		vol := f.ND
		if m.Plan.VolumeDeltaPct != nil {
			vol = f.Signed(*m.Plan.VolumeDeltaPct, 0) + " %"
		}
		b.line("- Séances prévues : %d | réalisées : %d | partielles : %d | manquées : %d | écart volume : %s",
			m.Plan.Planned, m.Plan.Done, m.Plan.Partial, m.Plan.Missed, vol)
	}

	b.section("ALERTES & SIGNAUX RÉCURRENTS")
	if len(m.Alerts) == 0 {
		b.raw("- Aucune alerte récurrente.")
	}
	for _, a := range sortedAlertCounts(m.Alerts) {
		b.line("- %s %s (×%d)", alertEmoji(a.Alert.Level), a.Alert.Title, a.Count)
	}

	next := m.NextWeeks
	if next <= 0 {
		next = 4
	}
	b.section("DEMANDE")
	b.line("- À partir de ces données, construis ou réadapte un plan d'entraînement pour les %d prochaines semaines, cohérent avec les objectifs et la zone surveillée. Justifie chaque ajustement par les données ci-dessus.", next)
	return b.String()
}
