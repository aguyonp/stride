package domain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"stride/internal/analysis"
	"stride/internal/apimeter"
	"stride/internal/athlete"
	"stride/internal/garmin"
	"stride/internal/queue"
	"stride/internal/report"
	"stride/internal/view"
	"stride/internal/workout"
)

const (
	garminCapTTL    = 24 * time.Hour
	maxBulkSessions = 120
	maxBulkPhases   = 12
	maxCalendarDays = 371
	unsupportedMsg  = "L'API personnelle Garmin ne permet pas d'envoyer des séances (404) : téléchargez le fichier .FIT de la séance et copiez-le en USB dans GARMIN/NewFiles sur la montre (elle apparaît dans Entraînement › Mes séances)."
)

// PushUnsupportedError answers 409 CONFLICT with the session (garmin.status = "unsupported").
type PushUnsupportedError struct {
	Session athlete.PlannedSession
}

func (e *PushUnsupportedError) Error() string { return unsupportedMsg }

type PlanGenerateInput struct {
	Mode      string `json:"mode"`
	Horizon   string `json:"horizon"`
	StartDate string `json:"startDate"`
	GoalID    *int64 `json:"goalId"`
	Note      string `json:"note"`
}

type PlanPromptResult struct {
	Prompt         string `json:"prompt"`
	TokensEstimate int    `json:"tokensEstimate"`
}

type PlanDraftsResult struct {
	Drafts []athlete.PlannedSession `json:"drafts"`
	Phases []athlete.PlanPhase      `json:"phases"`
	Errors []string                 `json:"errors"`
	Raw    string                   `json:"raw,omitempty"`
}

const (
	goalAllSessionsDays = 21 // up to this many days before the goal, every session is detailed
	planHorizonMaxDays  = 366
)

// planGoal returns the requested goal, or the nearest active goal dated on or after from.
func (s *Services) planGoal(ctx context.Context, uid int64, id *int64, from string) (athlete.Goal, error) {
	r := s.repo()
	if id != nil {
		g, err := r.Goal(ctx, uid, *id)
		if err != nil {
			return g, repoErr(err)
		}
		if g.TargetDate == nil {
			return g, badRequest("Cet objectif n'a pas de date cible : renseignez-la pour planifier jusqu'à l'objectif.")
		}
		return g, nil
	}
	goals, err := r.Goals(ctx, uid, true)
	if err != nil {
		return athlete.Goal{}, err
	}
	var best *athlete.Goal
	for i := range goals {
		g := &goals[i]
		if g.TargetDate != nil && *g.TargetDate > from && (best == nil || *g.TargetDate < *best.TargetDate) {
			best = g
		}
	}
	if best == nil {
		return athlete.Goal{}, badRequest("Aucun objectif actif avec une date cible à venir : ajoutez-en un ou choisissez un autre horizon.")
	}
	return *best, nil
}

// planWindow resolves the period, the detailed sub-period and the goal of a generation request.
func (s *Services) planWindow(ctx context.Context, uid int64, in *PlanGenerateInput) (report.PlanRequest, error) {
	req := report.PlanRequest{Horizon: in.Horizon, StartDate: in.StartDate, Note: in.Note}
	switch in.Horizon {
	case "session":
		req.EndDate = in.StartDate
	case "week":
		req.EndDate = analysis.DateAdd(in.StartDate, 6)
	case "2weeks":
		req.EndDate = analysis.DateAdd(in.StartDate, 13)
	case "goal":
		g, err := s.planGoal(ctx, uid, in.GoalID, in.StartDate)
		if err != nil {
			return req, err
		}
		if *g.TargetDate <= in.StartDate {
			return req, badRequest("La date de l'objectif doit être postérieure à la date de début.")
		}
		if daysBetween(in.StartDate, *g.TargetDate) > planHorizonMaxDays {
			return req, badRequest("Objectif trop lointain (%d jours max).", planHorizonMaxDays)
		}
		req.Goal, req.EndDate = &g, *g.TargetDate
	default:
		return req, badRequest("horizon invalide (session, week, 2weeks, goal).")
	}
	req.DetailFrom, req.DetailTo = req.StartDate, req.EndDate
	if req.Goal != nil && daysBetween(req.StartDate, req.EndDate) > goalAllSessionsDays {
		req.WithPhases = true
		existing, err := s.repo().Plan(ctx, uid, req.StartDate, req.EndDate)
		if err != nil {
			return req, err
		}
		// asking again extends the detail after the sessions already planned
		if n := len(existing); n > 0 && existing[n-1].Date < req.EndDate {
			req.DetailFrom = analysis.DateAdd(existing[n-1].Date, 1)
		}
		weeks := 4
		if daysBetween(req.DetailFrom, req.EndDate) < 56 {
			weeks = 2
		}
		req.DetailTo = analysis.DateAdd(req.DetailFrom, 7*weeks-1)
		if req.DetailTo > req.EndDate {
			req.DetailTo = req.EndDate
		}
	}
	return req, nil
}

func (s *Services) preparePlanGenerate(ctx context.Context, uid int64, in *PlanGenerateInput) (string, string, error) {
	if in.Mode != "prompt" && in.Mode != "llm" {
		return "", "", badRequest("mode invalide (prompt, llm).")
	}
	if in.Horizon == "" {
		in.Horizon = "week"
	}
	if len([]rune(in.Note)) > 1000 {
		return "", "", badRequest("Précision trop longue (1000 caractères max).")
	}
	if in.Mode == "llm" {
		c, err := s.llmConfig(ctx, uid)
		if err != nil {
			return "", "", err
		}
		if c == nil {
			return "", "", ErrLLMNotConfigured
		}
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return "", "", err
	}
	today := s.today(p)
	if in.StartDate == "" {
		in.StartDate = today
	}
	if !athlete.ValidDate(in.StartDate) {
		return "", "", badRequest("startDate attendue au format AAAA-MM-JJ.")
	}
	req, err := s.planWindow(ctx, uid, in)
	if err != nil {
		return "", "", err
	}
	dv, err := s.DayView(ctx, uid, today)
	if err != nil {
		return "", "", err
	}
	if dv.Health, err = s.trackedZones(ctx, uid); err != nil {
		return "", "", err
	}
	if dv.Records, err = s.records(ctx, uid); err != nil {
		return "", "", err
	}
	r := s.repo()
	if req.Existing, err = r.Plan(ctx, uid, req.StartDate, req.EndDate); err != nil {
		return "", "", err
	}
	if req.Events, err = r.Events(ctx, uid, req.StartDate, req.EndDate); err != nil {
		return "", "", err
	}
	planID, err := s.defaultPlanID(ctx, uid)
	if err != nil {
		return "", "", err
	}
	if req.Phases, err = r.Phases(ctx, uid, planID, req.StartDate, req.EndDate); err != nil {
		return "", "", err
	}
	if req.LoadRatio, err = s.loadRatio(ctx, uid, p, today); err != nil {
		return "", "", err
	}
	return report.Plan(dv, p, req), req.StartDate + ".." + req.EndDate, nil
}

// GeneratePlan builds the plan prompt (prompt mode) or asks the athlete's LLM for drafts on the LLM
// worker pool (*PlanDraftsResult, or the *queue.JobStatus when async or still running).
func (s *Services) GeneratePlan(ctx context.Context, uid int64, in PlanGenerateInput, async bool) (any, error) {
	prompt, ref, err := s.preparePlanGenerate(ctx, uid, &in)
	if err != nil {
		return nil, err
	}
	if in.Mode == "prompt" {
		return &PlanPromptResult{Prompt: prompt, TokensEstimate: report.EstimateTokens(prompt)}, nil
	}
	return awaitJob[PlanDraftsResult](ctx, s, queue.EnqueueRequest{
		UserID:  uid,
		Kind:    queue.KindLLMPlanGenerate,
		Queue:   queue.QueueCritical,
		Label:   "Génération du plan IA",
		Payload: llmPlanGenerateJobPayload{Ref: ref, Prompt: prompt, Async: async},
	}, async)
}

func (s *Services) loadRatio(ctx context.Context, uid int64, p athlete.Profile, today string) (*analysis.LoadRatio, error) {
	rows, err := s.activitiesBetween(ctx, uid, analysis.DateAdd(today, -27), today)
	if err != nil {
		return nil, err
	}
	loads, _ := loadsOf(rows)
	return analysis.ComputeLoadRatio(loads, today, "", p.Thresholds.LoadRatioMax), nil
}

// ImportPlan parses an AI answer (or any stride-plan JSON) into drafts, without saving.
func (s *Services) ImportPlan(text string) (*PlanDraftsResult, error) {
	if strings.TrimSpace(text) == "" {
		return nil, badRequest("Texte vide : collez la réponse de l'IA.")
	}
	drafts, phases, errs := athlete.ParsePlan(text, "import")
	return &PlanDraftsResult{Drafts: drafts, Phases: phases, Errors: errs}, nil
}

// decodeSessions validates planned-session inputs (drafts with id 0); errors are joined.
func decodeSessions(sessions []json.RawMessage) ([]athlete.PlannedSession, error) {
	list := make([]athlete.PlannedSession, 0, len(sessions))
	var errs []string
	for i, raw := range sessions {
		ps := athlete.PlannedSession{Sport: "run"}
		if err := athlete.Patch(&ps, raw, athlete.PlanFields, athlete.PlanReadOnly); err != nil {
			errs = append(errs, fmt.Sprintf("Séance %d : %v", i+1, err))
			continue
		}
		if err := ps.Normalize(); err != nil {
			errs = append(errs, fmt.Sprintf("Séance %d : %s", i+1, strings.ReplaceAll(err.Error(), "\n", " ; ")))
			continue
		}
		list = append(list, ps)
	}
	if len(errs) > 0 {
		return nil, &BadRequestError{Msg: "Données invalides : " + strings.Join(errs, " | ")}
	}
	return list, nil
}

func decodePhases(phases []json.RawMessage) ([]athlete.PlanPhase, error) {
	list := make([]athlete.PlanPhase, 0, len(phases))
	var errs []string
	for i, raw := range phases {
		var ph athlete.PlanPhase
		if err := athlete.Patch(&ph, raw, []string{"name", "startDate", "endDate", "focus", "weeklyVolumeKm", "note"}, []string{"id"}); err != nil {
			errs = append(errs, fmt.Sprintf("Phase %d : %v", i+1, err))
			continue
		}
		if err := ph.Normalize(); err != nil {
			errs = append(errs, fmt.Sprintf("Phase %d : %s", i+1, strings.ReplaceAll(err.Error(), "\n", " ; ")))
			continue
		}
		list = append(list, ph)
	}
	if len(errs) > 0 {
		return nil, &BadRequestError{Msg: "Données invalides : " + strings.Join(errs, " | ")}
	}
	return list, nil
}

// BulkCreatePlanned validates every session and phase first, then creates them all in one
// transaction (none on error); phases replace the stored phases overlapping their dates.
func (s *Services) BulkCreatePlanned(ctx context.Context, uid int64, sessions, phases []json.RawMessage) ([]athlete.PlannedSession, error) {
	if len(sessions)+len(phases) == 0 || len(sessions) > maxBulkSessions || len(phases) > maxBulkPhases {
		return nil, badRequest("Entre 1 et %d séances (et %d phases au plus) attendues.", maxBulkSessions, maxBulkPhases)
	}
	return s.replacePlan(ctx, uid, nil, sessions, phases)
}

// replacePlan deletes removeIds, creates sessions and replaces overlapping phases atomically.
func (s *Services) replacePlan(ctx context.Context, uid int64, removeIds []int64, sessions, phases []json.RawMessage) ([]athlete.PlannedSession, error) {
	list, err := decodeSessions(sessions)
	if err != nil {
		return nil, err
	}
	phs, err := decodePhases(phases)
	if err != nil {
		return nil, err
	}
	err = s.deps.Store.Tx(ctx, func(tx *sql.Tx) error {
		r := athlete.Repo{DB: tx, Now: s.Now}
		for _, id := range removeIds {
			if err := r.DeletePlanned(ctx, uid, id); err != nil {
				return repoErr(err)
			}
		}
		for i := range list {
			if err := r.CreatePlanned(ctx, uid, &list[i]); err != nil {
				return validation(err)
			}
		}
		planID, err := activePlanID(ctx, tx, uid)
		if err != nil {
			return err
		}
		return r.ReplacePhases(ctx, uid, planID, phs)
	})
	if err != nil {
		return nil, err
	}
	out := make([]athlete.PlannedSession, 0, len(list))
	for _, ps := range list {
		got, err := s.plannedWithMatch(ctx, uid, ps.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, got)
	}
	return out, nil
}

func capKey(uid int64) string { return userKey(uid, "cap", "garmin_workouts") }

func (s *Services) markUnsupported(ctx context.Context, uid, id int64) error {
	msg := unsupportedMsg
	if err := s.repo().SetGarminPush(ctx, uid, id, "unsupported", nil, &msg); err != nil {
		return repoErr(err)
	}
	ps, err := s.plannedWithMatch(ctx, uid, id)
	if err != nil {
		return err
	}
	return &PushUnsupportedError{Session: ps}
}

// PushGarmin tries to create the workout in Garmin Connect and schedule it on the session date.
// The Personal API refusal (404/403) is cached 24 h per user.
func (s *Services) PushGarmin(ctx context.Context, uid, id int64) (athlete.PlannedSession, error) {
	r := s.repo()
	ps, err := r.Planned(ctx, uid, id)
	if err != nil {
		return ps, repoErr(err)
	}
	if len(ps.Steps) == 0 {
		return ps, badRequest("Cette séance n'a pas d'étapes structurées à envoyer.")
	}
	if ps.Sport != "run" && ps.Sport != "bike" {
		return ps, badRequest("Seules les séances de course ou de vélo peuvent être envoyées.")
	}
	g, err := s.garminClient(ctx, uid)
	if err != nil {
		return ps, err
	}
	if _, ok := s.cacheGet(ctx, capKey(uid)); ok {
		return ps, s.markUnsupported(ctx, uid, id)
	}
	title := ps.Title
	if title == "" {
		title = "Séance Stride " + ps.Date
	}
	payload, _ := json.Marshal(workout.GarminWorkout(title, ps.Instructions, ps.Sport, ps.Steps))
	ctx = apimeter.WithUser(ctx, uid)
	body, err := g.CreateWorkout(ctx, payload)
	if st := garmin.Status(err); st == http.StatusNotFound || st == http.StatusForbidden {
		s.cacheSet(ctx, capKey(uid), []byte("unsupported"), garminCapTTL)
		s.log.Info("⌚ Envoi de séance refusé par l'API personnelle Garmin (capacité mise en cache 24 h)", "user", uid, "status", st)
		return ps, s.markUnsupported(ctx, uid, id)
	}
	if err != nil {
		msg := "Échec de l'envoi : " + errText(providerErr(pGarmin, err))
		_ = r.SetGarminPush(ctx, uid, id, "failed", nil, &msg)
		return ps, providerErr(pGarmin, err)
	}
	var created struct {
		WorkoutID int64 `json:"workoutId"`
	}
	if json.Unmarshal(body, &created) != nil || created.WorkoutID == 0 {
		msg := "Réponse Garmin inattendue (identifiant de séance absent)."
		_ = r.SetGarminPush(ctx, uid, id, "failed", nil, &msg)
		return ps, &ProviderError{Provider: pGarmin, Status: http.StatusOK, Detail: "identifiant de séance absent", Err: errors.New("no workoutId")}
	}
	var note *string
	if _, err := g.ScheduleWorkout(ctx, created.WorkoutID, ps.Date); err != nil {
		msg := "Séance créée dans Garmin Connect, mais sa planification au " + ps.Date + " a échoué : planifiez-la depuis Garmin Connect."
		note = &msg
	}
	if err := r.SetGarminPush(ctx, uid, id, "pushed", &created.WorkoutID, note); err != nil {
		return ps, repoErr(err)
	}
	s.log.Info("⌚ Séance envoyée vers Garmin Connect", "user", uid)
	return s.plannedWithMatch(ctx, uid, id)
}

// WorkoutFIT encodes the planned session as a FIT workout file.
func (s *Services) WorkoutFIT(ctx context.Context, uid, id int64) ([]byte, string, error) {
	ps, err := s.repo().Planned(ctx, uid, id)
	if err != nil {
		return nil, "", repoErr(err)
	}
	if len(ps.Steps) == 0 {
		return nil, "", badRequest("Cette séance n'a pas d'étapes structurées.")
	}
	name := ps.Title
	if name == "" {
		name = "Stride " + ps.Date
	}
	b, err := workout.EncodeFIT(name, ps.Sport, ps.Steps, s.now())
	if err != nil {
		return nil, "", err
	}
	return b, fmt.Sprintf("stride-%s-%d.fit", ps.Date, ps.ID), nil
}

// Calendar returns planned and done sessions and calendar events per day (≤ 371 days).
func (s *Services) Calendar(ctx context.Context, uid int64, from, to string) (*view.Calendar, error) {
	if !athlete.ValidDate(from) || !athlete.ValidDate(to) || to < from {
		return nil, badRequest("from et to attendus au format AAAA-MM-JJ (from ≤ to).")
	}
	if daysBetween(from, to) >= maxCalendarDays {
		return nil, badRequest("Période limitée à %d jours.", maxCalendarDays)
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	pm, err := s.matchPlan(ctx, uid, p, from, to)
	if err != nil {
		return nil, err
	}
	acts, err := s.activitiesBetween(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	sums, err := s.summaries(ctx, uid, acts)
	if err != nil {
		return nil, err
	}
	events, err := s.repo().Events(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	out := &view.Calendar{Days: []view.CalendarDay{}}
	for _, d := range analysis.DateRange(from, to) {
		day := view.CalendarDay{WeekDay: view.WeekDay{Date: d, Planned: []athlete.PlannedSession{}, Done: []view.ActivitySummary{}},
			Events: []athlete.CalendarEvent{}}
		for _, ev := range events {
			if ev.Covers(d) {
				day.Events = append(day.Events, ev)
			}
		}
		for _, ps := range pm.planned {
			if ps.Date == d {
				day.Planned = append(day.Planned, ps)
			}
		}
		for i, a := range acts {
			if a.LocalDate == d {
				sm := sums[i]
				if st, ok := pm.statuses[a.ID]; ok {
					sm.PlanStatus = &st
				}
				day.Done = append(day.Done, sm)
			}
		}
		out.Days = append(out.Days, day)
	}
	return out, nil
}
