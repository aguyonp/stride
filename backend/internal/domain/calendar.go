package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"stride/internal/analysis"
	"stride/internal/athlete"
	"stride/internal/frfmt"
	"stride/internal/queue"
	"stride/internal/report"
	"stride/internal/workout"
)

const (
	maxEventDays   = 366
	moveSearchDays = 14 // a session displaced by an event is moved at most this far after it
	adaptLLMMin    = 13 // LLM adaptation covers at least two weeks…
	adaptLLMMax    = 41 // …and at most six
)

// ---------- events ----------

func (s *Services) Events(ctx context.Context, uid int64, from, to string) ([]athlete.CalendarEvent, error) {
	if (from != "" && !athlete.ValidDate(from)) || (to != "" && !athlete.ValidDate(to)) {
		return nil, badRequest("Dates from/to attendues au format AAAA-MM-JJ.")
	}
	return s.repo().Events(ctx, uid, from, to)
}

func normalizeEvent(e *athlete.CalendarEvent) error {
	if err := e.Normalize(); err != nil {
		return validation(err)
	}
	if daysBetween(e.StartDate, e.EndDate) >= maxEventDays {
		return badRequest("Événement limité à %d jours.", maxEventDays)
	}
	return nil
}

func (s *Services) CreateEvent(ctx context.Context, uid int64, body []byte) (athlete.CalendarEvent, error) {
	var e athlete.CalendarEvent
	if err := patch(&e, body, athlete.EventFields, athlete.EventReadOnly); err != nil {
		return e, err
	}
	if err := normalizeEvent(&e); err != nil {
		return e, err
	}
	return e, s.repo().CreateEvent(ctx, uid, &e)
}

func (s *Services) PatchEvent(ctx context.Context, uid, id int64, body []byte) (athlete.CalendarEvent, error) {
	e, err := s.repo().Event(ctx, uid, id)
	if err != nil {
		return e, repoErr(err)
	}
	if err := patch(&e, body, athlete.EventFields, athlete.EventReadOnly); err != nil {
		return e, err
	}
	e.ID = id
	if err := normalizeEvent(&e); err != nil {
		return e, err
	}
	return e, repoErr(s.repo().UpdateEvent(ctx, uid, &e))
}

func (s *Services) DeleteEvent(ctx context.Context, uid, id int64) error {
	return repoErr(s.repo().DeleteEvent(ctx, uid, id))
}

// Phases serves GET /api/plan/phases: the phases of the plan /api/cycles shows by default.
func (s *Services) Phases(ctx context.Context, uid int64) ([]athlete.PlanPhase, error) {
	planID, err := s.defaultPlanID(ctx, uid)
	if err != nil {
		return nil, err
	}
	return s.repo().Phases(ctx, uid, planID, "", "")
}

// ---------- shift ----------

type ShiftInput struct {
	Days   *int    `json:"days"`
	ToDate *string `json:"toDate"`
}

// ShiftPlanned moves one planned session by n days or to a date.
func (s *Services) ShiftPlanned(ctx context.Context, uid, id int64, in ShiftInput) (athlete.PlannedSession, error) {
	ps, err := s.repo().Planned(ctx, uid, id)
	if err != nil {
		return ps, repoErr(err)
	}
	switch {
	case (in.Days == nil) == (in.ToDate == nil):
		return ps, badRequest("Indiquez soit days, soit toDate.")
	case in.Days != nil:
		if *in.Days == 0 || *in.Days < -365 || *in.Days > 365 {
			return ps, badRequest("days doit être compris entre -365 et 365 (hors 0).")
		}
		ps.Date = analysis.DateAdd(ps.Date, *in.Days)
	default:
		if !athlete.ValidDate(*in.ToDate) {
			return ps, badRequest("toDate attendue au format AAAA-MM-JJ.")
		}
		ps.Date = *in.ToDate
	}
	if err := s.repo().UpdatePlanned(ctx, uid, &ps); err != nil {
		return ps, validation(repoErr(err))
	}
	return s.plannedWithMatch(ctx, uid, id)
}

// ---------- adaptation ----------

type AdaptInput struct {
	From    string `json:"from"`
	Mode    string `json:"mode"`
	EventID *int64 `json:"eventId"`
}

// AdaptResult is a preview: removeIds are replaced by sessions (drafts, id 0) once applied.
type AdaptResult struct {
	Sessions  []athlete.PlannedSession `json:"sessions"`
	RemoveIDs []int64                  `json:"removeIds"`
	Summary   string                   `json:"summary"`
}

func (s *Services) prepareAdaptPlan(ctx context.Context, uid int64, in *AdaptInput) (*AdaptResult, *llmPlanAdaptJobPayload, error) {
	if in.Mode != "rules" && in.Mode != "llm" {
		return nil, nil, badRequest("mode invalide (rules, llm).")
	}
	if in.Mode == "llm" {
		c, err := s.llmConfig(ctx, uid)
		if err != nil {
			return nil, nil, err
		}
		if c == nil {
			return nil, nil, ErrLLMNotConfigured
		}
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, nil, err
	}
	today := s.today(p)
	if in.From == "" {
		in.From = today
	}
	if !athlete.ValidDate(in.From) {
		return nil, nil, badRequest("from attendue au format AAAA-MM-JJ.")
	}
	r := s.repo()
	var events []athlete.CalendarEvent
	if in.EventID != nil {
		ev, err := r.Event(ctx, uid, *in.EventID)
		if err != nil {
			return nil, nil, repoErr(err)
		}
		events = []athlete.CalendarEvent{ev}
	} else if events, err = r.Events(ctx, uid, in.From, ""); err != nil {
		return nil, nil, err
	}
	if in.Mode == "rules" {
		plan, err := r.Plan(ctx, uid, weekStart(in.From), "")
		if err != nil {
			return nil, nil, err
		}
		return adaptRules(plan, events, in.From), nil, nil
	}
	to := analysis.DateAdd(in.From, adaptLLMMin)
	for _, ev := range events {
		if end := analysis.DateAdd(ev.EndDate, 7); end > to {
			to = end
		}
	}
	if limit := analysis.DateAdd(in.From, adaptLLMMax); to > limit {
		to = limit
	}
	req := report.AdaptRequest{From: in.From, To: to, Events: events}
	if req.Remaining, err = r.Plan(ctx, uid, in.From, to); err != nil {
		return nil, nil, err
	}
	if req.LoadRatio, err = s.loadRatio(ctx, uid, p, today); err != nil {
		return nil, nil, err
	}
	dv, err := s.DayView(ctx, uid, today)
	if err != nil {
		return nil, nil, err
	}
	if dv.Records, err = s.records(ctx, uid); err != nil {
		return nil, nil, err
	}
	if req.CoachNotes, err = s.coachNotes(ctx, uid, today, nil, addDays(today, -coachNotesDays)); err != nil {
		return nil, nil, err
	}
	remIDs := make([]int64, 0, len(req.Remaining))
	for _, ps := range req.Remaining {
		remIDs = append(remIDs, ps.ID)
	}
	return nil, &llmPlanAdaptJobPayload{
		From:         in.From,
		To:           to,
		Prompt:       report.Localize(report.Adapt(dv, p, req), s.lang(ctx, p)),
		RemainingIDs: remIDs,
	}, nil
}

// AdaptPlan previews the plan adapted to the calendar events, by rules or by the LLM worker pool
// (*AdaptResult, or the *queue.JobStatus when async or still running); nothing is saved.
func (s *Services) AdaptPlan(ctx context.Context, uid int64, in AdaptInput, async bool) (any, error) {
	rulesRes, payload, err := s.prepareAdaptPlan(ctx, uid, &in)
	if err != nil {
		return nil, err
	}
	if rulesRes != nil {
		return rulesRes, nil
	}
	payload.Async = async
	return awaitJob[AdaptResult](ctx, s, queue.EnqueueRequest{
		UserID:  uid,
		Kind:    queue.KindLLMPlanAdapt,
		Queue:   queue.QueueCritical,
		Label:   "Adaptation du plan IA",
		Payload: *payload,
	}, async)
}

func plural(n int, word string) string {
	if n > 1 {
		parts := strings.Fields(word)
		for i := range parts {
			parts[i] += "s"
		}
		word = strings.Join(parts, " ")
	}
	return fmt.Sprintf("%d %s", n, word)
}

// sessionVolume is the planned duration (s) of a session, estimated from distance at 6:00/km when needed.
func sessionVolume(ps *athlete.PlannedSession) float64 {
	switch {
	case ps == nil || ps.Sport == "rest":
		return 0
	case ps.TargetDurationS != nil:
		return *ps.TargetDurationS
	case len(ps.Steps) > 0:
		return workout.TotalDurationS(ps.Steps, 360)
	case ps.TargetDistanceM != nil:
		return *ps.TargetDistanceM * 0.36
	}
	return 0
}

func scaleSteps(steps []workout.Step, k float64) []workout.Step {
	out := make([]workout.Step, len(steps))
	for i, st := range steps {
		if st.DurationS != nil {
			st.DurationS = f64(math.Round(*st.DurationS * k))
		}
		if st.DistanceM != nil {
			st.DistanceM = f64(math.Round(*st.DistanceM * k))
		}
		st.Steps = scaleSteps(st.Steps, k)
		out[i] = st
	}
	return out
}

// soften removes intensity (intervals become easy running) and cuts 20/30/40 % by severity.
func soften(ps athlete.PlannedSession, ev athlete.CalendarEvent) athlete.PlannedSession {
	k := 1 - (0.1 + 0.1*float64(ev.Severity))
	if ps.TargetDurationS != nil {
		ps.TargetDurationS = f64(math.Round(*ps.TargetDurationS * k))
	} else if d := workout.TotalDurationS(ps.Steps, 360); d > 0 {
		ps.TargetDurationS = f64(math.Round(d * k))
	}
	if ps.TargetDistanceM != nil {
		ps.TargetDistanceM = f64(math.Round(*ps.TargetDistanceM * k))
	}
	note := fmt.Sprintf("Séance allégée après « %s » (−%d %%)", athlete.EventLabel(ev.Kind), int((1-k)*100+0.5))
	if workout.Structured(ps.Steps) {
		ps.Steps = []workout.Step{}
		ps.TargetPaceMinSecPerKm, ps.TargetPaceMaxSecPerKm = nil, nil
		if ps.Sport == "run" {
			ps.Title = "Footing facile"
		}
		note += " : intensité retirée, endurance facile"
	} else {
		ps.Steps = scaleSteps(ps.Steps, k)
	}
	ps.Instructions = strings.TrimSpace(note + ". " + ps.Instructions)
	if len([]rune(ps.Instructions)) > 2000 {
		ps.Instructions = string([]rune(ps.Instructions)[:2000])
	}
	return ps
}

func draft(ps athlete.PlannedSession) athlete.PlannedSession {
	ps.ID, ps.Match, ps.Garmin = 0, nil, athlete.GarminPush{Status: "none"}
	return ps
}

// adaptRules applies §12.5 to the plan (sessions from the week of from): no session during an
// event (moved to the next free day, dropped when the event lasts more than 3 days), lighter
// sessions for 3/5/7 days after illness, pain or forced rest, and no week receiving moved sessions
// above max(its planned volume, 110 % of the following week's planned volume).
func adaptRules(plan []athlete.PlannedSession, events []athlete.CalendarEvent, from string) *AdaptResult {
	out := &AdaptResult{Sessions: []athlete.PlannedSession{}, RemoveIDs: []int64{}}
	sort.Slice(events, func(i, j int) bool { return events[i].StartDate < events[j].StartDate })
	blocked := func(d string) bool {
		for _, ev := range events {
			if ev.Covers(d) {
				return true
			}
		}
		return false
	}
	busy := map[string]bool{}
	origWeek := map[string]float64{}
	for i := range plan {
		ps := &plan[i]
		origWeek[weekStart(ps.Date)] += sessionVolume(ps)
		if ps.Sport != "rest" && ps.Date >= from {
			busy[ps.Date] = true
		}
	}
	changed := map[int64]*athlete.PlannedSession{} // nil = dropped
	movedIn := map[string][]int64{}                // week → moved session ids, in order
	moved, dropped, softened := 0, 0, 0
	for _, ev := range events {
		long := daysBetween(ev.StartDate, ev.EndDate)+1 > 3
		for i := range plan {
			ps := plan[i]
			if _, done := changed[ps.ID]; done || ps.Date < from || ps.Sport == "rest" || !ev.Covers(ps.Date) {
				continue
			}
			if long {
				changed[ps.ID] = nil
				dropped++
				continue
			}
			d := analysis.DateAdd(ev.EndDate, 1)
			for n := 0; n < moveSearchDays && (busy[d] || blocked(d)); n++ {
				d = analysis.DateAdd(d, 1)
			}
			if busy[d] || blocked(d) {
				changed[ps.ID] = nil
				dropped++
				continue
			}
			busy[d] = true
			ps.Date = d
			changed[ps.ID] = &ps
			movedIn[weekStart(d)] = append(movedIn[weekStart(d)], ps.ID)
			moved++
		}
	}
	isSoft := map[int64]bool{}
	for _, ev := range events {
		if ev.Kind != "illness" && ev.Kind != "pain" && ev.Kind != "forced_rest" {
			continue
		}
		until := analysis.DateAdd(ev.EndDate, 1+2*ev.Severity)
		for i := range plan {
			cur := &plan[i]
			if c, ok := changed[cur.ID]; ok {
				cur = c
			}
			if cur == nil || isSoft[cur.ID] || cur.Date < from || cur.Sport == "rest" || cur.Date <= ev.EndDate || cur.Date > until {
				continue
			}
			sp := soften(*cur, ev)
			changed[cur.ID], isSoft[cur.ID] = &sp, true
			softened++
		}
	}
	weekVol := func(w string) float64 {
		v := 0.0
		for i := range plan {
			cur := &plan[i]
			if c, ok := changed[cur.ID]; ok {
				cur = c
			}
			if cur != nil && weekStart(cur.Date) == w {
				v += sessionVolume(cur)
			}
		}
		return v
	}
	for w, ids := range movedIn {
		limit := 1.1 * origWeek[w]
		if next := origWeek[analysis.DateAdd(w, 7)]; next > 0 {
			limit = max(origWeek[w], 1.1*next)
		}
		if limit <= 0 {
			continue
		}
		for n := len(ids) - 1; n >= 0 && weekVol(w) > limit; n-- {
			if changed[ids[n]] != nil {
				changed[ids[n]] = nil
				moved--
				dropped++
			}
		}
	}
	for i := range plan {
		id := plan[i].ID
		c, ok := changed[id]
		if !ok {
			continue
		}
		out.RemoveIDs = append(out.RemoveIDs, id)
		if c != nil {
			out.Sessions = append(out.Sessions, draft(*c))
		}
	}
	sort.SliceStable(out.Sessions, func(i, j int) bool { return out.Sessions[i].Date < out.Sessions[j].Date })
	out.Summary = adaptSummary(events, moved, dropped, softened)
	return out
}

func adaptSummary(events []athlete.CalendarEvent, moved, dropped, softened int) string {
	if len(events) == 0 {
		return "Aucun imprévu ne touche le plan à venir : rien à adapter."
	}
	var evs []string
	recovery := 0
	for _, ev := range events {
		r := athlete.EventLabel(ev.Kind) + fmt.Sprintf(" (sévérité %d/3) ", ev.Severity)
		if ev.StartDate == ev.EndDate {
			r += "le " + frfmt.Date(ev.StartDate)
		} else {
			r += "du " + frfmt.Date(ev.StartDate) + " au " + frfmt.Date(ev.EndDate)
		}
		evs = append(evs, r)
		if ev.Kind == "illness" || ev.Kind == "pain" || ev.Kind == "forced_rest" {
			recovery = max(recovery, 1+2*ev.Severity)
		}
	}
	var parts []string
	if moved > 0 {
		parts = append(parts, plural(moved, "séance")+" décalée"+map[bool]string{true: "s"}[moved > 1])
	}
	if dropped > 0 {
		parts = append(parts, fmt.Sprintf("%d supprimée%s", dropped, map[bool]string{true: "s"}[dropped > 1]))
	}
	if softened > 0 {
		parts = append(parts, fmt.Sprintf("%d allégée%s", softened, map[bool]string{true: "s"}[softened > 1]))
	}
	if len(parts) == 0 {
		parts = []string{"aucune séance concernée"}
	}
	out := strings.Join(evs, ", ") + " : " + strings.Join(parts, ", ") + "."
	if recovery > 0 {
		out += fmt.Sprintf(" Reprise progressive sur %d jours, sans semaine au-delà de +10 %% du volume prévu.", recovery)
	}
	return out
}

// ReplacePlan applies an adaptation preview atomically (POST /api/plan/replace).
func (s *Services) ReplacePlan(ctx context.Context, uid int64, removeIDs []int64, sessions []json.RawMessage) ([]athlete.PlannedSession, error) {
	if len(removeIDs) == 0 && len(sessions) == 0 {
		return nil, badRequest("Rien à appliquer : removeIds et sessions sont vides.")
	}
	if len(sessions) > maxBulkSessions || len(removeIDs) > 4*maxBulkSessions {
		return nil, badRequest("Au plus %d séances à créer.", maxBulkSessions)
	}
	return s.replacePlan(ctx, uid, removeIDs, sessions, nil)
}
