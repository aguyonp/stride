// tools.go registers the v1 MCP tools on a language instance: reads (planning, day,
// activities, activity, health, cycles, profile) and writes (plan_create/update/delete,
// event_create/update/delete). The caller is always resolved from the request context —
// never from the arguments — so a token can only act on its owner's data.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"stride/internal/athlete"
	"stride/internal/auth"
	"stride/internal/buildinfo"
	"stride/internal/domain"
	"stride/internal/frfmt"
	"stride/internal/view"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// tools couples the domain services with the language of the instance they are attached to.
type tools struct {
	svc *domain.Services
	fr  bool
}

func newTools(svc *domain.Services, lang frfmt.Lang) *tools {
	return &tools{svc: svc, fr: lang == frfmt.FR}
}

// l returns the French or English variant of a pair of strings.
func (t *tools) l(fr, en string) string {
	if t.fr {
		return fr
	}
	return en
}

// uid extracts the owner of the request from the context (injected by ServeMCP from the token).
func uid(ctx context.Context) (int64, error) {
	u, ok := auth.UserFrom(ctx)
	if !ok {
		return 0, fmt.Errorf("contexte sans utilisateur")
	}
	return u.ID, nil
}

// build creates the MCP server for this language and registers every tool on it.
func (t *tools) build() *mcpsdk.Server {
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "stride", Version: buildinfo.Version},
		&mcpsdk.ServerOptions{
			Instructions: t.l(
				`Serveur Stride d'un athlète : lis son planning et y écris.
Les dates sont au format AAAA-MM-JJ, les durées en secondes, les distances en mètres.
plan_update et event_update n'appliquent que les champs fournis.
Tu ne peux agir que sur les données du propriétaire du jeton.`,
				`Stride server for one athlete: read their plan and write to it.
Dates are YYYY-MM-DD, durations in seconds, distances in metres.
plan_update and event_update apply only the provided fields.
You can only act on the data of the token owner.`),
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "planning",
		Description: t.l(`Le planning de la semaine (lundi à dimanche) : séances planifiées, séances effectuées et totaux de charge. Sans paramètre : la semaine en cours ; avec date : la semaine contenant cette date.`,
			`The week plan (Monday to Sunday): planned sessions, completed sessions and load totals. Without argument: the current week; with date: the week containing that date.`)}, t.planning)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "day",
		Description: t.l(`Le détail d'un jour : récupération, charge du jour, activités réalisées, séance planifiée, objectifs et alertes. Sans paramètre : aujourd'hui.`,
			`The detail of one day: recovery, daily load, completed activities, planned session, goals and alerts. Without argument: today.`)}, t.day)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "activities",
		Description: t.l(`Les activités de l'athlète (course, vélo, …), les plus récentes d'abord, avec pagination.`,
			`The athlete's activities (running, cycling, …), newest first, with pagination.`)}, t.activities)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "activity",
		Description: t.l(`Le détail complet d'une activité : métriques, biomécanique, fatigue, segments et équipement.`,
			`The full detail of one activity: metrics, biomechanics, fatigue, segments and gear.`)}, t.activity)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "health",
		Description: t.l(`L'état de santé actuel : mode d'entraînement (performance, récupération, …), zones suivies et focus.`,
			`The current health status: training mode (performance, recovery, …), tracked zones and focus area.`)}, t.health)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "cycles",
		Description: t.l(`Les cycles d'entraînement : plans, phases, semaines et volumes effectifs.`,
			`The training cycles: plans, phases, weeks and effective volumes.`)}, t.cycles)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "profile",
		Description: t.l(`Le profil de l'athlète : zone de focus, seuils d'alerte, allures cibles et langue.`,
			`The athlete's profile: focus area, alert thresholds, target paces and language.`)}, t.profile)

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "plan_create",
		Description: t.l(`Planifie une séance (course, vélo, renforcement, mobilité, repos) sur une date. Retourne la séance créée.`,
			`Plan a session (run, bike, strength, mobility, rest) on a date. Returns the created session.`)}, t.planCreate)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "plan_update",
		Description: t.l(`Modifie une séance planifiée : seuls les champs fournis sont appliqués (les autres sont conservés). Retourne la séance mise à jour.`,
			`Update a planned session: only the provided fields are applied (the others are kept). Returns the updated session.`)}, t.planUpdate)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "plan_delete",
		Description: t.l(`Supprime une séance planifiée.`,
			`Delete a planned session.`)}, t.planDelete)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "event_create",
		Description: t.l(`Enregistre un événement qui module le plan : fatigue, douleur, maladie, indisponibilité ou repos forcé. Retourne l'événement créé.`,
			`Record an event that modulates the plan: fatigue, pain, illness, unavailability or forced rest. Returns the created event.`)}, t.eventCreate)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "event_update",
		Description: t.l(`Modifie un événement : seuls les champs fournis sont appliqués (les autres sont conservés). Retourne l'événement mis à jour.`,
			`Update an event: only the provided fields are applied (the others are kept). Returns the updated event.`)}, t.eventUpdate)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "event_delete",
		Description: t.l(`Supprime un événement.`,
			`Delete an event.`)}, t.eventDelete)
	return srv
}

// ---------- read tools ----------
//
// The read views embed planned sessions, whose steps are recursive (workout.Step.Steps):
// jsonschema-go cannot infer their schema and AddTool would panic. Those tools therefore use
// Out = any: the SDK omits the output schema and still returns the full JSON in
// structuredContent (with a text fallback), which is all an LLM client needs.

// planningIn is the optional date of the week to show.
type planningIn struct {
	Date *string `json:"date,omitempty" jsonschema:"date in YYYY-MM-DD, in the requested week (default: today's week)"`
}

// planning returns the week plan containing the given date (current week by default).
func (t *tools) planning(ctx context.Context, _ *mcpsdk.CallToolRequest, in planningIn) (*mcpsdk.CallToolResult, any, error) {
	id, err := uid(ctx)
	if err != nil {
		return nil, nil, err
	}
	date := ""
	if in.Date != nil {
		date = *in.Date
	}
	wp, err := t.svc.WeekPlan(ctx, id, date)
	if err != nil {
		return nil, nil, err
	}
	return nil, wp, nil
}

// dayIn is the optional date of the day to show.
type dayIn struct {
	Date *string `json:"date,omitempty" jsonschema:"date in YYYY-MM-DD (default: today)"`
}

// day returns the detail view of one day.
func (t *tools) day(ctx context.Context, _ *mcpsdk.CallToolRequest, in dayIn) (*mcpsdk.CallToolResult, any, error) {
	id, err := uid(ctx)
	if err != nil {
		return nil, nil, err
	}
	date := ""
	if in.Date != nil {
		date = *in.Date
	}
	dv, err := t.svc.DayView(ctx, id, date)
	if err != nil {
		return nil, nil, err
	}
	return nil, dv, nil
}

// activitiesIn filters and paginates the activity list.
type activitiesIn struct {
	Limit *int    `json:"limit,omitempty" jsonschema:"number of activities to return (default 30, max 100)"`
	From  *string `json:"from,omitempty" jsonschema:"local start date, YYYY-MM-DD, inclusive"`
	To    *string `json:"to,omitempty" jsonschema:"local end date, YYYY-MM-DD, inclusive"`
	Sport *string `json:"sport,omitempty" jsonschema:"sport filter: run, bike, …"`
}

// activities lists the athlete's activities, newest first.
func (t *tools) activities(ctx context.Context, _ *mcpsdk.CallToolRequest, in activitiesIn) (*mcpsdk.CallToolResult, view.ActivityList, error) {
	id, err := uid(ctx)
	if err != nil {
		return nil, view.ActivityList{}, err
	}
	limit := 0
	from, to, sport := "", "", ""
	if in.Limit != nil {
		limit = *in.Limit
	}
	if in.From != nil {
		from = *in.From
	}
	if in.To != nil {
		to = *in.To
	}
	if in.Sport != nil {
		sport = *in.Sport
	}
	list, err := t.svc.ListActivities(ctx, id, limit, "", from, to, sport)
	if err != nil {
		return nil, view.ActivityList{}, err
	}
	return nil, *list, nil
}

// activityIn carries the activity to detail.
type activityIn struct {
	ID int64 `json:"id" jsonschema:"internal id of the activity"`
}

// activity returns the full detail of one activity.
func (t *tools) activity(ctx context.Context, _ *mcpsdk.CallToolRequest, in activityIn) (*mcpsdk.CallToolResult, any, error) {
	id, err := uid(ctx)
	if err != nil {
		return nil, nil, err
	}
	av, err := t.svc.ActivityView(ctx, id, in.ID)
	if err != nil {
		return nil, nil, err
	}
	return nil, av, nil
}

// emptyIn is the input of the tools that take no argument.
type emptyIn struct{}

// health returns the current health status.
func (t *tools) health(ctx context.Context, _ *mcpsdk.CallToolRequest, _ emptyIn) (*mcpsdk.CallToolResult, domain.HealthStatus, error) {
	id, err := uid(ctx)
	if err != nil {
		return nil, domain.HealthStatus{}, err
	}
	hs, err := t.svc.HealthStatus(ctx, id)
	if err != nil {
		return nil, domain.HealthStatus{}, err
	}
	return nil, *hs, nil
}

// cyclesIn optionally selects one plan.
type cyclesIn struct {
	PlanID *int64 `json:"planId,omitempty" jsonschema:"id of a specific plan to expand (default: the active plan)"`
}

// cycles returns the training cycles overview.
func (t *tools) cycles(ctx context.Context, _ *mcpsdk.CallToolRequest, in cyclesIn) (*mcpsdk.CallToolResult, any, error) {
	id, err := uid(ctx)
	if err != nil {
		return nil, nil, err
	}
	ov, err := t.svc.Cycles(ctx, id, in.PlanID)
	if err != nil {
		return nil, nil, err
	}
	return nil, ov, nil
}

// profile returns the athlete profile.
func (t *tools) profile(ctx context.Context, _ *mcpsdk.CallToolRequest, _ emptyIn) (*mcpsdk.CallToolResult, athlete.Profile, error) {
	id, err := uid(ctx)
	if err != nil {
		return nil, athlete.Profile{}, err
	}
	p, err := t.svc.Profile(ctx, id)
	if err != nil {
		return nil, athlete.Profile{}, err
	}
	return nil, p, nil
}

// ---------- write tools ----------

// planFields are the writable fields of a planned session (domain validation applies).
type planFields struct {
	Date                  *string  `json:"date,omitempty" jsonschema:"date of the session, YYYY-MM-DD"`
	Sport                 *string  `json:"sport,omitempty" jsonschema:"sport: run, bike, strength, mobility, rest, other"`
	Title                 *string  `json:"title,omitempty" jsonschema:"short title of the session"`
	TargetDurationS       *float64 `json:"targetDurationS,omitempty" jsonschema:"target duration in seconds"`
	TargetDistanceM       *float64 `json:"targetDistanceM,omitempty" jsonschema:"target distance in metres"`
	TargetPaceMinSecPerKm *float64 `json:"targetPaceMinSecPerKm,omitempty" jsonschema:"target min pace in s/km"`
	TargetPaceMaxSecPerKm *float64 `json:"targetPaceMaxSecPerKm,omitempty" jsonschema:"target max pace in s/km"`
	TargetCadenceSpm      *float64 `json:"targetCadenceSpm,omitempty" jsonschema:"target cadence in steps per minute"`
	Instructions          *string  `json:"instructions,omitempty" jsonschema:"coach instructions for the session"`
}

// planCreateIn is a planned session to create (date and sport required).
type planCreateIn struct {
	Date  string `json:"date" jsonschema:"date of the session, YYYY-MM-DD"`
	Sport string `json:"sport" jsonschema:"sport: run, bike, strength, mobility, rest, other"`
	planFields
}

// planUpdateIn is a planned session to update (id required, other fields optional).
type planUpdateIn struct {
	ID int64 `json:"id" jsonschema:"internal id of the planned session"`
	planFields
}

// planDeleteIn carries the planned session to delete.
type planDeleteIn struct {
	ID int64 `json:"id" jsonschema:"internal id of the planned session"`
}

// eventCreateIn is an event to create (kind and startDate required, the domain fills the
// defaults for endDate and severity).
type eventCreateIn struct {
	Kind      string  `json:"kind" jsonschema:"kind: fatigue, pain, illness, unavailable, forced_rest"`
	StartDate string  `json:"startDate" jsonschema:"start date, YYYY-MM-DD"`
	EndDate   *string `json:"endDate,omitempty" jsonschema:"end date, YYYY-MM-DD (default: startDate)"`
	Severity  *int    `json:"severity,omitempty" jsonschema:"severity 1 to 3 (default 2)"`
	Note      *string `json:"note,omitempty" jsonschema:"free note, up to 1000 characters"`
}

// eventUpdateIn is an event to update (id required, other fields optional).
type eventUpdateIn struct {
	ID        int64   `json:"id" jsonschema:"internal id of the event"`
	Kind      *string `json:"kind,omitempty" jsonschema:"kind: fatigue, pain, illness, unavailable, forced_rest"`
	StartDate *string `json:"startDate,omitempty" jsonschema:"start date, YYYY-MM-DD"`
	EndDate   *string `json:"endDate,omitempty" jsonschema:"end date, YYYY-MM-DD"`
	Severity  *int    `json:"severity,omitempty" jsonschema:"severity 1 to 3"`
	Note      *string `json:"note,omitempty" jsonschema:"free note, up to 1000 characters"`
}

// eventUpdateBody is the eventUpdateIn without the id (the domain identifies the event by id).
type eventUpdateBody struct {
	Kind      *string `json:"kind,omitempty"`
	StartDate *string `json:"startDate,omitempty"`
	EndDate   *string `json:"endDate,omitempty"`
	Severity  *int    `json:"severity,omitempty"`
	Note      *string `json:"note,omitempty"`
}

func (in eventUpdateIn) body() eventUpdateBody {
	return eventUpdateBody{Kind: in.Kind, StartDate: in.StartDate, EndDate: in.EndDate,
		Severity: in.Severity, Note: in.Note}
}

// eventDeleteIn carries the event to delete.
type eventDeleteIn struct {
	ID int64 `json:"id" jsonschema:"internal id of the event"`
}

// deletedOut is the confirmation returned by the delete tools.
type deletedOut struct {
	Deleted bool  `json:"deleted"`
	ID      int64 `json:"id"`
}

func (t *tools) planCreate(ctx context.Context, _ *mcpsdk.CallToolRequest, in planCreateIn) (*mcpsdk.CallToolResult, any, error) {
	id, err := uid(ctx)
	if err != nil {
		return nil, nil, err
	}
	f := in.planFields
	f.Date = &in.Date
	f.Sport = &in.Sport
	body, err := json.Marshal(f)
	if err != nil {
		return nil, nil, err
	}
	ps, err := t.svc.CreatePlanned(ctx, id, body)
	if err != nil {
		return nil, nil, err
	}
	return nil, ps, nil
}

func (t *tools) planUpdate(ctx context.Context, _ *mcpsdk.CallToolRequest, in planUpdateIn) (*mcpsdk.CallToolResult, any, error) {
	id, err := uid(ctx)
	if err != nil {
		return nil, nil, err
	}
	body, err := json.Marshal(in.planFields)
	if err != nil {
		return nil, nil, err
	}
	ps, err := t.svc.PatchPlanned(ctx, id, in.ID, body)
	if err != nil {
		return nil, nil, err
	}
	return nil, ps, nil
}

func (t *tools) planDelete(ctx context.Context, _ *mcpsdk.CallToolRequest, in planDeleteIn) (*mcpsdk.CallToolResult, deletedOut, error) {
	id, err := uid(ctx)
	if err != nil {
		return nil, deletedOut{}, err
	}
	if err := t.svc.DeletePlanned(ctx, id, in.ID); err != nil {
		return nil, deletedOut{}, err
	}
	return nil, deletedOut{Deleted: true, ID: in.ID}, nil
}

func (t *tools) eventCreate(ctx context.Context, _ *mcpsdk.CallToolRequest, in eventCreateIn) (*mcpsdk.CallToolResult, athlete.CalendarEvent, error) {
	id, err := uid(ctx)
	if err != nil {
		return nil, athlete.CalendarEvent{}, err
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, athlete.CalendarEvent{}, err
	}
	e, err := t.svc.CreateEvent(ctx, id, body)
	if err != nil {
		return nil, athlete.CalendarEvent{}, err
	}
	return nil, e, nil
}

func (t *tools) eventUpdate(ctx context.Context, _ *mcpsdk.CallToolRequest, in eventUpdateIn) (*mcpsdk.CallToolResult, athlete.CalendarEvent, error) {
	id, err := uid(ctx)
	if err != nil {
		return nil, athlete.CalendarEvent{}, err
	}
	body, err := json.Marshal(in.body())
	if err != nil {
		return nil, athlete.CalendarEvent{}, err
	}
	e, err := t.svc.PatchEvent(ctx, id, in.ID, body)
	if err != nil {
		return nil, athlete.CalendarEvent{}, err
	}
	return nil, e, nil
}

func (t *tools) eventDelete(ctx context.Context, _ *mcpsdk.CallToolRequest, in eventDeleteIn) (*mcpsdk.CallToolResult, deletedOut, error) {
	id, err := uid(ctx)
	if err != nil {
		return nil, deletedOut{}, err
	}
	if err := t.svc.DeleteEvent(ctx, id, in.ID); err != nil {
		return nil, deletedOut{}, err
	}
	return nil, deletedOut{Deleted: true, ID: in.ID}, nil
}
