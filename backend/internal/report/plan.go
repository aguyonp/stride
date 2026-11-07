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

// PlanContractV2 adds the phases of a long-term plan up to the goal (§12.6).
const PlanContractV2 = "```json\n" + `{"format":"stride-plan","version":2,"goal":{"title":"10 km","date":"2027-02-15"},"phases":[{"name":"Développement","startDate":"2026-11-09","endDate":"2026-12-06","focus":"volume aérobie, cadence","weeklyVolumeKm":35,"note":""}],"sessions":[{"date":"2026-11-10","sport":"run","title":"Footing + 6×1'","instructions":"Cadence 172 spm, relâché","targetDurationS":2700,"steps":[{"type":"warmup","durationS":600},{"type":"repeat","repeat":6,"steps":[{"type":"interval","durationS":60,"target":{"kind":"pace","min":250,"max":260}},{"type":"recovery","durationS":90}]},{"type":"cooldown","durationS":600}]}]}` + "\n```"

type PlanRequest struct {
	Horizon    string // session | week | 2weeks | goal
	StartDate  string
	EndDate    string
	DetailFrom string // sessions are detailed on [DetailFrom, DetailTo]
	DetailTo   string
	Goal       *athlete.Goal
	WithPhases bool
	Note       string
	Existing   []athlete.PlannedSession
	Events     []athlete.CalendarEvent
	Phases     []athlete.PlanPhase
	LoadRatio  *analysis.LoadRatio
}

// Plan builds the plan-generation prompt from the day view of today.
func Plan(v *view.DayView, prof athlete.Profile, req PlanRequest) string {
	b := newBuilder()
	writePlanState(b, v, prof, req.LoadRatio)
	b.section("PLAN DÉJÀ PRÉVU SUR LA PÉRIODE")
	if len(req.Existing) == 0 {
		b.raw("- Aucune séance prévue.")
	}
	for _, ps := range req.Existing {
		b.raw("- " + f.Date(ps.Date) + " : " + describePlanned(ps, true))
	}
	writePhases(b, req.Phases)
	writeEvents(b, req.Events)
	b.section("DEMANDE")
	from, to := f.Date(req.DetailFrom), f.Date(req.DetailTo)
	switch {
	case req.WithPhases:
		b.line("Découpe la préparation du %s au %s (objectif « %s ») en phases successives et contiguës dans `phases` (développement, assimilation, spécifique, affûtage : nom, dates, focus, volume hebdomadaire en km), puis détaille uniquement les séances du %s au %s dans `sessions` (une par jour au plus, repos compris si utile). Ne duplique pas les séances déjà prévues.",
			f.Date(req.StartDate), f.Date(req.EndDate), compact(req.Goal.Title, 120), from, to)
	case req.Goal != nil:
		b.line("Propose toutes les séances du %s au %s, jour de l'objectif « %s » inclus (une par jour au plus, repos compris si utile, affûtage avant l'échéance), cohérentes avec le focus, la charge et l'état de préparation ci-dessus. Ne duplique pas les séances déjà prévues.",
			from, to, compact(req.Goal.Title, 120))
	case req.Horizon == "week":
		b.line("Propose les séances de la semaine du %s au %s (une par jour au plus, repos compris si utile), cohérentes avec les objectifs, le focus, la charge et l'état de préparation ci-dessus. Ne duplique pas les séances déjà prévues.", from, to)
	case req.Horizon == "2weeks":
		b.line("Propose les séances des deux semaines du %s au %s (une par jour au plus, repos compris si utile, progression d'une semaine à l'autre), cohérentes avec les objectifs, le focus, la charge et l'état de préparation ci-dessus. Ne duplique pas les séances déjà prévues.", from, to)
	default:
		b.line("Propose la prochaine séance, le %s, cohérente avec les objectifs, le focus, la charge et l'état de préparation ci-dessus.", from)
	}
	if len(req.Events) > 0 {
		b.raw("Aucune séance pendant un événement du calendrier ; reprise progressive après une maladie, une douleur ou un repos forcé.")
	}
	if n := strings.TrimSpace(req.Note); n != "" {
		b.raw("**Précision de l'athlète :** " + compact(n, 600))
	}
	writeContract(b, req.WithPhases)
	return b.String()
}

func writePlanState(b *builder, v *view.DayView, prof athlete.Profile, lr *analysis.LoadRatio) {
	writeReadinessPriority(b, v.Readiness)
	writeContext(b, prof, v.Health, v.Goals, plannedList(v.Planned), v.Upcoming, v.Records)
	b.section("ÉTAT DE PRÉPARATION & CHARGE")
	rb := v.Readiness
	b.line("- **Préparation :** %s %s", analysis.ReadinessEmoji(rb.Level), rb.Title)
	if len(rb.Reasons) > 0 {
		b.raw("- **Signaux :** " + strings.Join(rb.Reasons, " ; "))
	}
	if lr != nil {
		b.line("- **Ratio de charge global :** %s (Aiguë: %s / Chronique: %s)", decOr(lr.Ratio, 2), f.Int(lr.Acute), f.Int(lr.Chronic))
	} else {
		b.raw("- **Ratio de charge global :** " + f.ND)
	}
	b.line("- **VFC moy. 7 j :** %s | Sommeil moy. 7 j : %s", unit(rb.Hrv7dAvg, "ms"), outOf100(rb.Sleep7dAvg))
	writeHistory(b, v.History, true)
}

func writePhases(b *builder, phases []athlete.PlanPhase) {
	if len(phases) == 0 {
		return
	}
	b.section("PHASES DÉJÀ DÉFINIES")
	for _, ph := range phases {
		s := fmt.Sprintf("- %s du %s au %s", ph.Name, f.Date(ph.StartDate), f.Date(ph.EndDate))
		if ph.Focus != "" {
			s += " : " + compact(ph.Focus, 200)
		}
		if ph.WeeklyVolumeKm != nil {
			s += " (" + f.Num(*ph.WeeklyVolumeKm, 1) + " km/sem.)"
		}
		b.raw(s)
	}
}

func writeEvents(b *builder, events []athlete.CalendarEvent) {
	if len(events) == 0 {
		return
	}
	b.section("ÉVÉNEMENTS DU CALENDRIER")
	for _, ev := range events {
		s := fmt.Sprintf("- %s du %s au %s (sévérité %d/3)", athlete.EventLabel(ev.Kind), f.Date(ev.StartDate), f.Date(ev.EndDate), ev.Severity)
		if ev.Note != "" {
			s += " : " + compact(ev.Note, 200)
		}
		b.raw(s)
	}
}

func writeContract(b *builder, withPhases bool) {
	b.section("FORMAT DE SORTIE (OBLIGATOIRE)")
	b.raw("Réponds avec un **unique** bloc ```json respectant exactement ce contrat (aucun autre JSON) :")
	if withPhases {
		b.raw(PlanContractV2)
		b.raw("- `phases[]` : `name`, `startDate`/`endDate` AAAA-MM-JJ (contiguës jusqu'à l'objectif), `focus`, `weeklyVolumeKm`, `note`.")
	} else {
		b.raw(PlanContract)
	}
	b.raw(fmt.Sprintf("- `date` AAAA-MM-JJ ; `sport` parmi %s ; `title` ≤ 120 caractères ; `instructions` en français.", strings.Join(athlete.PlanSports, ", ")))
	b.raw("- `steps[].type` parmi warmup, run, interval, recovery, rest, cooldown, repeat ; une étape a `durationS` **ou** `distanceM` ; `repeat` (1–50) contient `steps`.")
	b.raw("- `target.kind` parmi none, pace, hr, cadence, power ; allure en s/km (`min` = la plus rapide), FC en bpm, cadence en pas/min, puissance en W.")
	b.raw("- Tu peux ajouter une courte justification en dehors du bloc JSON.")
}

type AdaptRequest struct {
	From      string
	To        string
	Remaining []athlete.PlannedSession
	Events    []athlete.CalendarEvent
	LoadRatio *analysis.LoadRatio
}

// Adapt builds the plan-adaptation prompt (LLM mode of POST /api/plan/adapt).
func Adapt(v *view.DayView, prof athlete.Profile, req AdaptRequest) string {
	b := newBuilder()
	writePlanState(b, v, prof, req.LoadRatio)
	writeEvents(b, req.Events)
	b.section("PLAN RESTANT À ADAPTER")
	if len(req.Remaining) == 0 {
		b.raw("- Aucune séance prévue.")
	}
	for _, ps := range req.Remaining {
		b.raw("- " + f.Date(ps.Date) + " : " + describePlanned(ps, true))
	}
	b.section("DEMANDE")
	b.line("Adapte le plan du %s au %s aux événements ci-dessus : aucune séance pendant un événement (déplace-la vers un jour libre ou supprime-la si l'événement dure plus de 3 jours), séances allégées pendant 3 à 7 jours après une maladie, une douleur ou un repos forcé (fractionné remplacé par de l'endurance facile, durée réduite de 20 à 40 %% selon la sévérité), et aucune semaine ne dépasse de plus de 10 %% le volume prévu. Renvoie **toutes** les séances de la période (elles remplacent le plan restant).",
		f.Date(req.From), f.Date(req.To))
	writeContract(b, false)
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
