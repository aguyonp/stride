// Package report renders the Markdown prompts of docs/SPEC.md §6 (French formatting, N/D when
// absent, objectivity directive always first, sections toggled by chips).
package report

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"stride/internal/analysis"
	"stride/internal/athlete"
	f "stride/internal/frfmt"
	"stride/internal/view"
	"stride/internal/workout"
)

// Directive is §6.1, verbatim.
const Directive = "> **DIRECTIVE SYSTÈME POUR L'IA :** Agis comme un ingénieur biomécanicien et entraîneur de course à pied strictement objectif, analytique et rigoureux. Ne cherche JAMAIS à flatter l'utilisateur, à minimiser ses erreurs ou à lui répondre pour lui faire plaisir. Analyse les données froidement : si une métrique (cadence, dérive, charge, allure, douleur) indique un risque de blessure, une compensation ou une erreur d'entraînement, dis-le-lui directement et ajuste le plan vers le bas s'il le faut. Dis uniquement ce qui est physiologiquement et mécaniquement juste pour lui."

const (
	ChipPain    = "pain"
	ChipBiomech = "biomech"
	ChipHealth  = "health"
	ChipPlan    = "plan"
	ChipLoad    = "load"
	ChipSplits  = "splits"
	ChipHistory = "history"
)

var Chips = []string{ChipPain, ChipBiomech, ChipHealth, ChipPlan, ChipLoad, ChipSplits, ChipHistory}

// Include is the set of enabled chips; a nil Include enables everything.
type Include map[string]bool

func All() Include {
	inc := Include{}
	for _, c := range Chips {
		inc[c] = true
	}
	return inc
}

func (i Include) Has(chip string) bool {
	if i == nil {
		return true
	}
	return i[chip]
}

// String lists the enabled chips in canonical order (stable cache keys).
func (i Include) String() string {
	var out []string
	for _, c := range Chips {
		if i.Has(c) {
			out = append(out, c)
		}
	}
	return strings.Join(out, ",")
}

func ParseInclude(csv string) (Include, error) {
	return ParseIncludeList(strings.Split(csv, ","))
}

// ParseIncludeList: empty means every chip, "none" means no optional section.
func ParseIncludeList(list []string) (Include, error) {
	inc := Include{}
	none := false
	for _, raw := range list {
		c := strings.ToLower(strings.TrimSpace(raw))
		if c == "" {
			continue
		}
		if c == "none" {
			none = true
			continue
		}
		known := false
		for _, k := range Chips {
			if k == c {
				known = true
			}
		}
		if !known {
			return nil, fmt.Errorf("puce inconnue %q (attendu : %s)", c, strings.Join(Chips, ", "))
		}
		inc[c] = true
	}
	if len(inc) == 0 && !none {
		return All(), nil
	}
	return inc, nil
}

// EstimateTokens approximates the prompt size (runes / 4, rounded up).
func EstimateTokens(s string) int { return (utf8.RuneCountInString(s) + 3) / 4 }

// builder joins sections with exactly one blank line.
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

func newBuilder() *builder {
	b := &builder{}
	b.raw(Directive)
	return b
}

// ---------- value helpers ----------

const dash = "—"

func p(v *float64) float64 { return f.P(v) }

func intOr(v *float64) string { return f.Or(p(v), f.Int) }

func unit(v *float64, u string) string { return f.Unit(p(v), u) }

func decOr(v *float64, n int) string { return f.Dec(p(v), n) }

func signedUnit(v *float64, n int, u string) string {
	if v == nil || !f.Valid(*v) {
		return f.ND
	}
	return f.Signed(*v, n) + " " + u
}

func outOf100(v *float64) string {
	if v == nil || !f.Valid(*v) {
		return f.ND
	}
	return f.Int(*v) + "/100"
}

func pain(v *float64) string {
	if v == nil || !f.Valid(*v) {
		return f.ND
	}
	return f.Num(*v, 1) + "/10"
}

func pace(v *float64) string { return f.Pace(p(v)) }

func km(v *float64) string { return f.Km(p(v), 2) }

func str(s *string) string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return f.ND
	}
	return *s
}

// compact collapses whitespace and truncates to max runes.
func compact(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return strings.TrimSpace(string(r[:max-1])) + "…"
	}
	return s
}

func elevPair(gain, loss *float64) string {
	if gain == nil && loss == nil {
		return f.ND
	}
	return "+" + intOr(gain) + "/-" + intOr(loss) + " m"
}

func signedM(v *float64, sign string) string {
	if v == nil || !f.Valid(*v) {
		return f.ND
	}
	return sign + f.Int(*v) + " m"
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

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

var sportLabels = map[string]string{"run": "Course", "bike": "Vélo", "strength": "Renforcement", "mobility": "Mobilité", "rest": "Repos", "other": "Autre"}

var matchStatusLabels = map[string]string{"done": "Réalisée", "partial": "Partielle", "missed": "Manquée", "upcoming": "À venir", "extra": "Hors plan"}

func minutes(s float64) string {
	if int64(s)%60 == 0 {
		return f.Int(s/60) + " min"
	}
	return f.Duration(s)
}

func paceRange(minS, maxS *float64) string {
	switch {
	case minS != nil && maxS != nil && f.PaceShort(*minS) != f.PaceShort(*maxS):
		return f.PaceShort(*minS) + "-" + f.PaceShort(*maxS) + "/km"
	case minS != nil:
		return f.Pace(*minS)
	case maxS != nil:
		return f.Pace(*maxS)
	}
	return ""
}

// describePlanned renders "Titre (38 min, 7 km) @ 5:15-5:25/km | Cible cadence: 170 spm – consigne".
func describePlanned(ps athlete.PlannedSession, full bool) string {
	title := strings.TrimSpace(ps.Title)
	if title == "" {
		title = sportLabels[ps.Sport]
		if title == "" {
			title = "Séance"
		}
	}
	var targets []string
	if ps.TargetDurationS != nil && *ps.TargetDurationS > 0 {
		targets = append(targets, minutes(*ps.TargetDurationS))
	}
	if ps.TargetDistanceM != nil && *ps.TargetDistanceM > 0 {
		targets = append(targets, f.Num(*ps.TargetDistanceM/1000, 2)+" km")
	}
	s := title
	if len(targets) > 0 {
		s += " (" + strings.Join(targets, ", ") + ")"
	}
	if pr := paceRange(ps.TargetPaceMinSecPerKm, ps.TargetPaceMaxSecPerKm); pr != "" {
		s += " @ " + pr
	}
	if !full {
		return s
	}
	if ps.TargetCadenceSpm != nil {
		s += " | Cible cadence: " + f.Int(*ps.TargetCadenceSpm) + " spm"
	}
	if st := StepsText(ps.Steps); st != "" {
		s += " | Structure : " + st
	}
	if in := compact(ps.Instructions, 160); in != "" {
		s += " – " + in
	}
	return s
}

var stepLabels = map[string]string{"warmup": "échauffement", "run": "course", "interval": "effort", "recovery": "récup",
	"rest": "repos", "cooldown": "retour au calme"}

func targetText(t *workout.Target) string {
	if t == nil || (t.Min == nil && t.Max == nil) {
		return ""
	}
	switch t.Kind {
	case "pace":
		return " @ " + paceRange(t.Min, t.Max)
	case "hr":
		return " @ " + rangeText(t.Min, t.Max, " bpm")
	case "cadence":
		return " @ " + rangeText(t.Min, t.Max, " spm")
	case "power":
		return " @ " + rangeText(t.Min, t.Max, " W")
	}
	return ""
}

func rangeText(lo, hi *float64, u string) string {
	switch {
	case lo != nil && hi != nil && *lo != *hi:
		return f.Int(*lo) + "–" + f.Int(*hi) + u
	case lo != nil:
		return f.Int(*lo) + u
	}
	return f.Int(*hi) + u
}

// StepsText renders structured steps compactly, e.g. "10' échauffement, 6×(1' effort @ 4:10–4:20/km + 1'30 récup)".
func StepsText(steps []workout.Step) string {
	parts := make([]string, 0, len(steps))
	for _, st := range steps {
		if st.Type == "repeat" {
			n := 1
			if st.Repeat != nil {
				n = *st.Repeat
			}
			parts = append(parts, fmt.Sprintf("%d×(%s)", n, strings.ReplaceAll(StepsText(st.Steps), ", ", " + ")))
			continue
		}
		amount := "lap"
		switch {
		case st.DurationS != nil:
			amount = stepDuration(*st.DurationS)
		case st.DistanceM != nil:
			amount = f.Num(*st.DistanceM, 0) + " m"
			if *st.DistanceM >= 1000 {
				amount = f.Num(*st.DistanceM/1000, 2) + " km"
			}
		}
		parts = append(parts, amount+" "+stepLabels[st.Type]+targetText(st.Target))
	}
	return strings.Join(parts, ", ")
}

func stepDuration(s float64) string {
	t := int(s + 0.5)
	if t%60 == 0 {
		return fmt.Sprintf("%d'", t/60)
	}
	if t < 60 {
		return fmt.Sprintf("%d\"", t)
	}
	return fmt.Sprintf("%d'%02d", t/60, t%60)
}

// writeReadinessPriority adds the §11.4 priority section (orange / red only), right after the directive.
func writeReadinessPriority(b *builder, rb analysis.ReadinessBanner) {
	if rb.Level != "orange" && rb.Level != "red" {
		return
	}
	b.section("⚠️ SIGNAL DE PRÉPARATION PRIORITAIRE")
	b.line("- **Niveau :** %s %s", analysis.ReadinessEmoji(rb.Level), rb.Title)
	for _, r := range rb.Reasons {
		b.raw("- " + r)
	}
	b.raw("- **Consigne :** tiens compte de ce signal en priorité dans l'analyse et les recommandations.")
}

func upcomingText(ups []athlete.PlannedSession) string {
	if len(ups) == 0 {
		return "Aucune"
	}
	parts := make([]string, 0, len(ups))
	for _, u := range ups {
		parts = append(parts, f.DayMonth(u.Date)+" "+describePlanned(u, false))
	}
	return strings.Join(parts, " ; ")
}

func goalText(g athlete.Goal) string {
	s := compact(g.Title, 160)
	if g.TargetValue != nil {
		s += " – cible " + f.Num(*g.TargetValue, 2)
		if g.TargetUnit != nil && *g.TargetUnit != "" {
			s += " " + *g.TargetUnit
		}
	}
	if g.TargetDate != nil && *g.TargetDate != "" {
		s += " – échéance " + f.Date(*g.TargetDate)
	}
	if n := compact(g.Note, 160); n != "" {
		s += " (" + n + ")"
	}
	return s
}

// splitGoals returns the macro goal (first active macro, else first active goal) and the others.
func splitGoals(goals []athlete.Goal) (*athlete.Goal, []athlete.Goal) {
	var active []athlete.Goal
	for _, g := range goals {
		if g.Status == "" || g.Status == "active" {
			active = append(active, g)
		}
	}
	if len(active) == 0 {
		return nil, nil
	}
	idx := 0
	for i, g := range active {
		if g.Kind == "macro" {
			idx = i
			break
		}
	}
	macro := active[idx]
	others := append(append([]athlete.Goal{}, active[:idx]...), active[idx+1:]...)
	return &macro, others
}

func macroText(goals []athlete.Goal) string {
	m, _ := splitGoals(goals)
	if m == nil {
		return f.ND
	}
	return goalText(*m)
}

// ---------- shared sections ----------

func writeRecovery(b *builder, rc *analysis.RecoveryContext) {
	b.section("RÉCUPÉRATION & PRÉPARATION (NUIT PRÉCÉDENTE)")
	readiness := f.ND + " (non exposé par l'API Garmin, saisie manuelle possible)"
	bbWake := f.ND
	if rc != nil {
		if r := rc.Readiness; r.Source != nil && *r.Source == "manual" {
			readiness = outOf100(r.Score)
			if r.Status != nil && *r.Status != "" {
				readiness += " (" + *r.Status + ")"
			}
		}
		if rc.BodyBattery != nil {
			bbWake = outOf100(rc.BodyBattery.AtWake)
		}
	}
	b.line("- **Training Readiness :** %s | **Body Battery réveil :** %s", readiness, bbWake)
	if rc != nil && rc.HRV != nil {
		h := rc.HRV
		status := f.ND
		if h.StatusLabel != nil {
			status = *h.StatusLabel
		}
		base := f.ND
		if h.BaselineLowMs != nil && h.BaselineHighMs != nil {
			base = f.Int(*h.BaselineLowMs) + "-" + f.Int(*h.BaselineHighMs) + " ms"
		}
		b.line("- **VFC nocturne :** %s (Statut: %s | Baseline: %s | Moy 7j: %s)", unit(h.LastNightAvgMs, "ms"), status, base, unit(h.WeeklyAvgMs, "ms"))
	} else {
		b.raw("- **VFC nocturne :** " + f.ND)
	}
	sleep := f.ND
	rhr := f.ND
	if rc != nil {
		if sl := rc.Sleep; sl != nil {
			sleep = fmt.Sprintf("Score %s (Durée: %s dont Profond: %s)", outOf100(sl.Score), f.SleepHM(p(sl.DurationS)), f.SleepHM(p(sl.DeepS)))
		}
		if rc.RestingHR != nil {
			rhr = unit(rc.RestingHR.Bpm, "bpm")
		}
	}
	b.line("- **Sommeil :** %s | **FC repos :** %s", sleep, rhr)
}

func writeHistory(b *builder, rows []view.HistoryRow, withPain bool) {
	b.section("HISTORIQUE 14 JOURS (COURSE & RESSENTI)")
	var runs []view.HistoryRow
	for _, r := range rows {
		if r.IsRunning {
			runs = append(runs, r)
		}
	}
	if len(runs) == 0 {
		b.raw("Aucune course sur la période.")
		return
	}
	if withPain {
		b.raw("| Date | Dist. | Durée | Allure | Cadence | GCT | D- | Load | Chaussures | Gêne Effort | Gêne Froid |")
		b.raw("|---|---|---|---|---|---|---|---|---|---|---|")
	} else {
		b.raw("| Date | Dist. | Durée | Allure | Cadence | GCT | D- | Load | Chaussures |")
		b.raw("|---|---|---|---|---|---|---|---|---|")
	}
	for _, r := range runs {
		cells := []string{f.DayMonth(r.Date), km(r.DistanceM), f.Duration(p(r.DurationS)), pace(r.PaceSecPerKm),
			unit(r.AvgCadenceSpm, "spm"), unit(r.GctMs, "ms"), unit(r.ElevLossM, "m"), intOr(r.TrainingLoad), str(r.ShoeName)}
		if withPain {
			cells = append(cells, pain(r.PainDuring), pain(r.PainCold))
		}
		b.raw("| " + strings.Join(cells, " | ") + " |")
	}
}

func writeAlerts(b *builder, title string, alerts []analysis.Alert) {
	b.section(title)
	if len(alerts) == 0 {
		b.raw("- Aucune alerte.")
		return
	}
	for _, a := range alerts {
		b.line("- %s %s : %s", alertEmoji(a.Level), a.Title, a.Message)
	}
}

func writeContext(b *builder, prof athlete.Profile, goals []athlete.Goal, today string, upcoming []athlete.PlannedSession) {
	b.section("CONTEXTE & OBJECTIFS DE L'ATHLÈTE")
	b.raw("- **Profil / Mode actif :** " + prof.ModeLabel())
	b.raw("- **Objectif Macro :** " + macroText(goals))
	b.raw("- **Séance prévue aujourd'hui :** " + today)
	b.raw("- **Prochaines séances prévues :** " + upcomingText(upcoming))
}

// sortedAlertCounts orders aggregated alerts by severity then frequency.
func sortedAlertCounts(in []view.AlertCount) []view.AlertCount {
	order := map[string]int{"danger": 0, "warn": 1, "info": 2, "ok": 3}
	out := append([]view.AlertCount(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		if order[out[i].Alert.Level] != order[out[j].Alert.Level] {
			return order[out[i].Alert.Level] < order[out[j].Alert.Level]
		}
		return out[i].Count > out[j].Count
	})
	return out
}
