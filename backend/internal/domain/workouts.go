package domain

import (
	"context"
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
	"stride/internal/report"
	"stride/internal/view"
	"stride/internal/workout"
)

const (
	garminCapTTL    = 24 * time.Hour
	maxBulkSessions = 31
	maxCalendarDays = 62
	unsupportedMsg  = "L'API personnelle Garmin ne permet pas l'envoi de séances structurées : téléchargez le fichier FIT et copiez-le dans GARMIN/NewFiles sur la montre."
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
	Note      string `json:"note"`
}

type PlanPromptResult struct {
	Prompt         string `json:"prompt"`
	TokensEstimate int    `json:"tokensEstimate"`
}

type PlanDraftsResult struct {
	Drafts []athlete.PlannedSession `json:"drafts"`
	Errors []string                 `json:"errors"`
	Raw    string                   `json:"raw,omitempty"`
}

// GeneratePlan builds the plan prompt (prompt mode) or asks the athlete's LLM for drafts (llm mode, not saved).
func (s *Services) GeneratePlan(ctx context.Context, uid int64, in PlanGenerateInput) (any, error) {
	if in.Mode != "prompt" && in.Mode != "llm" {
		return nil, badRequest("mode invalide (prompt, llm).")
	}
	if in.Horizon == "" {
		in.Horizon = "week"
	}
	if in.Horizon != "session" && in.Horizon != "week" {
		return nil, badRequest("horizon invalide (session, week).")
	}
	if len([]rune(in.Note)) > 1000 {
		return nil, badRequest("Précision trop longue (1000 caractères max).")
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	today := s.today(p)
	if in.StartDate == "" {
		in.StartDate = today
	}
	if !athlete.ValidDate(in.StartDate) {
		return nil, badRequest("startDate attendue au format AAAA-MM-JJ.")
	}
	dv, err := s.DayView(ctx, uid, today)
	if err != nil {
		return nil, err
	}
	end := in.StartDate
	if in.Horizon == "week" {
		end = analysis.DateAdd(in.StartDate, 6)
	}
	existing, err := s.repo().Plan(ctx, uid, in.StartDate, end)
	if err != nil {
		return nil, err
	}
	rows, err := s.activitiesBetween(ctx, uid, analysis.DateAdd(today, -27), today)
	if err != nil {
		return nil, err
	}
	loads, _ := loadsOf(rows)
	prompt := report.Plan(dv, p, report.PlanRequest{Horizon: in.Horizon, StartDate: in.StartDate, Note: in.Note, Existing: existing,
		LoadRatio: analysis.ComputeLoadRatio(loads, today, "", p.Thresholds.LoadRatioMax)})
	if in.Mode == "prompt" {
		return &PlanPromptResult{Prompt: prompt, TokensEstimate: report.EstimateTokens(prompt)}, nil
	}
	c, err := s.llmConfig(ctx, uid)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrLLMNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, llmTimeout)
	defer cancel()
	text, err := s.callLLM(ctx, uid, c, llmRequest{system: systemDirective(), prompt: prompt, maxTokens: llmMaxTokens})
	if err != nil {
		return nil, err
	}
	drafts, errs := athlete.ParsePlan(text, "llm")
	return &PlanDraftsResult{Drafts: drafts, Errors: errs, Raw: text}, nil
}

// ImportPlan parses an AI answer (or any stride-plan JSON) into drafts, without saving.
func (s *Services) ImportPlan(text string) (*PlanDraftsResult, error) {
	if strings.TrimSpace(text) == "" {
		return nil, badRequest("Texte vide : collez la réponse de l'IA.")
	}
	drafts, errs := athlete.ParsePlan(text, "import")
	return &PlanDraftsResult{Drafts: drafts, Errors: errs}, nil
}

// BulkCreatePlanned validates every session first, then creates them all (none on error).
func (s *Services) BulkCreatePlanned(ctx context.Context, uid int64, sessions []json.RawMessage) ([]athlete.PlannedSession, error) {
	if len(sessions) == 0 || len(sessions) > maxBulkSessions {
		return nil, badRequest("Entre 1 et %d séances attendues.", maxBulkSessions)
	}
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
	out := make([]athlete.PlannedSession, 0, len(list))
	r := s.repo()
	for i := range list {
		if err := r.CreatePlanned(ctx, uid, &list[i]); err != nil {
			return nil, validation(err)
		}
	}
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

// Calendar returns planned and done sessions per day (≤ 62 days).
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
	out := &view.Calendar{Days: []view.WeekDay{}}
	for _, d := range analysis.DateRange(from, to) {
		day := view.WeekDay{Date: d, Planned: []athlete.PlannedSession{}, Done: []view.ActivitySummary{}}
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
