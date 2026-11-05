package report

import (
	"fmt"
	"strings"

	"stride/internal/analysis"
	"stride/internal/athlete"
	f "stride/internal/frfmt"
	"stride/internal/view"
)

// PlanContract is the strict output contract imposed on any AI (§11.3).
const PlanContract = "```json\n" + `{"format":"stride-plan","version":1,"sessions":[{"date":"2026-11-10","sport":"run","title":"Footing + 6×1'","instructions":"Cadence 172 spm, relâché","targetDurationS":2700,"targetCadenceSpm":172,"steps":[{"type":"warmup","durationS":600,"target":{"kind":"pace","min":330,"max":360}},{"type":"repeat","repeat":6,"steps":[{"type":"interval","durationS":60,"target":{"kind":"pace","min":250,"max":260}},{"type":"recovery","durationS":90}]},{"type":"cooldown","durationS":600}]}]}` + "\n```"

type PlanRequest struct {
	Horizon   string // session | week
	StartDate string
	Note      string
	Existing  []athlete.PlannedSession
	LoadRatio *analysis.LoadRatio
}

// Plan builds the plan-generation prompt from the day view of today.
func Plan(v *view.DayView, prof athlete.Profile, req PlanRequest) string {
	b := newBuilder()
	writeReadinessPriority(b, v.Readiness)
	writeContext(b, prof, v.Goals, plannedList(v.Planned), v.Upcoming)
	b.section("ÉTAT DE PRÉPARATION & CHARGE")
	rb := v.Readiness
	b.line("- **Préparation :** %s %s", analysis.ReadinessEmoji(rb.Level), rb.Title)
	if len(rb.Reasons) > 0 {
		b.raw("- **Signaux :** " + strings.Join(rb.Reasons, " ; "))
	}
	if lr := req.LoadRatio; lr != nil {
		b.line("- **Ratio de charge global :** %s (Aiguë: %s / Chronique: %s)", decOr(lr.Ratio, 2), f.Int(lr.Acute), f.Int(lr.Chronic))
	} else {
		b.raw("- **Ratio de charge global :** " + f.ND)
	}
	b.line("- **VFC moy. 7 j :** %s | Sommeil moy. 7 j : %s", unit(rb.Hrv7dAvg, "ms"), outOf100(rb.Sleep7dAvg))
	writeHistory(b, v.History, true)
	b.section("PLAN DÉJÀ PRÉVU SUR LA PÉRIODE")
	if len(req.Existing) == 0 {
		b.raw("- Aucune séance prévue.")
	}
	for _, ps := range req.Existing {
		b.raw("- " + f.Date(ps.Date) + " : " + describePlanned(ps, true))
	}
	b.section("DEMANDE")
	if req.Horizon == "week" {
		b.line("Propose les séances de la semaine du %s au %s (une par jour au plus, repos compris si utile), cohérentes avec les objectifs, le focus, la charge et l'état de préparation ci-dessus. Ne duplique pas les séances déjà prévues.",
			f.Date(req.StartDate), f.Date(analysis.DateAdd(req.StartDate, 6)))
	} else {
		b.line("Propose la prochaine séance, le %s, cohérente avec les objectifs, le focus, la charge et l'état de préparation ci-dessus.", f.Date(req.StartDate))
	}
	if n := strings.TrimSpace(req.Note); n != "" {
		b.raw("**Précision de l'athlète :** " + compact(n, 600))
	}
	b.section("FORMAT DE SORTIE (OBLIGATOIRE)")
	b.raw("Réponds avec un **unique** bloc ```json respectant exactement ce contrat (aucun autre JSON) :")
	b.raw(PlanContract)
	b.raw(fmt.Sprintf("- `date` AAAA-MM-JJ ; `sport` parmi %s ; `title` ≤ 120 caractères ; `instructions` en français.", strings.Join(athlete.PlanSports, ", ")))
	b.raw("- `steps[].type` parmi warmup, run, interval, recovery, rest, cooldown, repeat ; une étape a `durationS` **ou** `distanceM` ; `repeat` (1–50) contient `steps`.")
	b.raw("- `target.kind` parmi none, pace, hr, cadence, power ; allure en s/km (`min` = la plus rapide), FC en bpm, cadence en pas/min, puissance en W.")
	b.raw("- Tu peux ajouter une courte justification en dehors du bloc JSON.")
	return b.String()
}

func plannedList(ps []athlete.PlannedSession) string {
	if len(ps) == 0 {
		return "Aucune"
	}
	parts := make([]string, 0, len(ps))
	for _, p := range ps {
		parts = append(parts, describePlanned(p, true))
	}
	return strings.Join(parts, " ; ")
}
