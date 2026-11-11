package domain

import (
	"context"
	"slices"

	"stride/internal/athlete"
)

// Portal planning and session detail (§15.1): a link with the planning permission prescribes planned
// sessions and calendar events (source « share », prescribedBy snapshot) and may only edit or delete
// what it created; a link with activityDetails reads one session of its period in detail.

const (
	portalPastDays     = 7   // a link edits from today −7 d…
	portalFutureDays   = 120 // …to today +120 d
	portalCalendarDays = 42  // the public calendar shows today −7 d → +42 d
)

// portalReadOnly: the PlannedSessionInput « source » is accepted but ignored (forced to share).
var portalPlanReadOnly = append(slices.Clone(athlete.PlanReadOnly), "source")

func (s *Services) publicCalendar(ctx context.Context, uid int64, today string) (*PublicCalendar, error) {
	from, to := addDays(today, -portalPastDays), addDays(today, portalCalendarDays)
	planned, err := s.Plan(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	events, err := s.repo().Events(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	return &PublicCalendar{Planned: planned, Events: events}, nil
}

// portalWindow refuses dates outside today −7 d → +120 d (owner's timezone).
func (s *Services) portalWindow(ctx context.Context, uid int64, dates ...string) error {
	p, err := s.profile(ctx, uid)
	if err != nil {
		return err
	}
	today := s.today(p)
	from, to := addDays(today, -portalPastDays), addDays(today, portalFutureDays)
	for _, d := range dates {
		if d < from || d > to {
			return badRequest("Date hors de la fenêtre du lien (du %s au %s).", from, to)
		}
	}
	return nil
}

// linkPrescriber snapshots the link label and the author of its latest note.
func (s *Services) linkPrescriber(ctx context.Context, l shareLinkRow) *athlete.Prescriber {
	id := l.id
	p := &athlete.Prescriber{LinkID: &id, Label: l.label}
	notes, err := s.queryNotes(ctx, `WHERE n.user_id = ? AND n.share_link_id = ? AND n.author_name IS NOT NULL ORDER BY n.updated_at DESC LIMIT 1`, l.uid, l.id)
	if err == nil && len(notes) == 1 {
		p.AuthorName = notes[0].AuthorName
	}
	return p
}

// ownedBy checks that a row was prescribed by link l: another link or the athlete is ErrShareForbidden.
func ownedBy(link *int64, err error, l shareLinkRow) error {
	if err != nil {
		return repoErr(err)
	}
	if link == nil || *link != l.id {
		return ErrShareForbidden
	}
	return nil
}

// CreateSharePlanned prescribes a planned session through a link (planning permission).
func (s *Services) CreateSharePlanned(ctx context.Context, token string, body []byte) (athlete.PlannedSession, error) {
	ps := athlete.PlannedSession{Sport: "run"}
	l, err := s.portalLink(ctx, token, "planning")
	if err != nil {
		return ps, err
	}
	if err := patch(&ps, body, athlete.PlanFields, portalPlanReadOnly); err != nil {
		return ps, err
	}
	ps.Source = "share"
	if err := ps.Normalize(); err != nil {
		return ps, validation(err)
	}
	if err := s.portalWindow(ctx, l.uid, ps.Date); err != nil {
		return ps, err
	}
	ps.PrescribedBy = s.linkPrescriber(ctx, l)
	if err := s.repo().CreatePlanned(ctx, l.uid, &ps); err != nil {
		return ps, err
	}
	s.afterPortalWrite(ctx, l, "plan")
	return s.plannedWithMatch(ctx, l.uid, ps.ID)
}

// PatchSharePlanned edits a session the link prescribed.
func (s *Services) PatchSharePlanned(ctx context.Context, token string, id int64, body []byte) (athlete.PlannedSession, error) {
	var ps athlete.PlannedSession
	l, err := s.portalLink(ctx, token, "planning")
	if err != nil {
		return ps, err
	}
	link, lerr := s.repo().PlannedLink(ctx, l.uid, id)
	if err := ownedBy(link, lerr, l); err != nil {
		return ps, err
	}
	if ps, err = s.repo().Planned(ctx, l.uid, id); err != nil {
		return ps, repoErr(err)
	}
	before := ps.Date
	if err := patch(&ps, body, athlete.PlanFields, portalPlanReadOnly); err != nil {
		return ps, err
	}
	ps.ID, ps.Source = id, "share"
	if err := ps.Normalize(); err != nil {
		return ps, validation(err)
	}
	if err := s.portalWindow(ctx, l.uid, before, ps.Date); err != nil {
		return ps, err
	}
	if err := s.repo().UpdatePlanned(ctx, l.uid, &ps); err != nil {
		return ps, repoErr(err)
	}
	s.afterPortalWrite(ctx, l, "plan")
	return s.plannedWithMatch(ctx, l.uid, id)
}

// DeleteSharePlanned removes a session the link prescribed.
func (s *Services) DeleteSharePlanned(ctx context.Context, token string, id int64) error {
	l, err := s.portalLink(ctx, token, "planning")
	if err != nil {
		return err
	}
	link, lerr := s.repo().PlannedLink(ctx, l.uid, id)
	if err := ownedBy(link, lerr, l); err != nil {
		return err
	}
	if err := repoErr(s.repo().DeletePlanned(ctx, l.uid, id)); err != nil {
		return err
	}
	s.afterPortalWrite(ctx, l, "plan")
	return nil
}

// CreateShareEvent adds a calendar event through a link (planning permission).
func (s *Services) CreateShareEvent(ctx context.Context, token string, body []byte) (athlete.CalendarEvent, error) {
	var e athlete.CalendarEvent
	l, err := s.portalLink(ctx, token, "planning")
	if err != nil {
		return e, err
	}
	if err := patch(&e, body, athlete.EventFields, athlete.EventReadOnly); err != nil {
		return e, err
	}
	if err := normalizeEvent(&e); err != nil {
		return e, err
	}
	if err := s.portalWindow(ctx, l.uid, e.StartDate, e.EndDate); err != nil {
		return e, err
	}
	e.PrescribedBy = s.linkPrescriber(ctx, l)
	if err := s.repo().CreateEvent(ctx, l.uid, &e); err != nil {
		return e, err
	}
	s.afterPortalWrite(ctx, l, "event")
	return e, nil
}

// PatchShareEvent edits an event the link created.
func (s *Services) PatchShareEvent(ctx context.Context, token string, id int64, body []byte) (athlete.CalendarEvent, error) {
	var e athlete.CalendarEvent
	l, err := s.portalLink(ctx, token, "planning")
	if err != nil {
		return e, err
	}
	link, lerr := s.repo().EventLink(ctx, l.uid, id)
	if err := ownedBy(link, lerr, l); err != nil {
		return e, err
	}
	if e, err = s.repo().Event(ctx, l.uid, id); err != nil {
		return e, repoErr(err)
	}
	before := e.StartDate
	if err := patch(&e, body, athlete.EventFields, athlete.EventReadOnly); err != nil {
		return e, err
	}
	e.ID = id
	if err := normalizeEvent(&e); err != nil {
		return e, err
	}
	if err := s.portalWindow(ctx, l.uid, before, e.StartDate, e.EndDate); err != nil {
		return e, err
	}
	if err := repoErr(s.repo().UpdateEvent(ctx, l.uid, &e)); err != nil {
		return e, err
	}
	s.afterPortalWrite(ctx, l, "event")
	return s.repo().Event(ctx, l.uid, id)
}

// DeleteShareEvent removes an event the link created.
func (s *Services) DeleteShareEvent(ctx context.Context, token string, id int64) error {
	l, err := s.portalLink(ctx, token, "planning")
	if err != nil {
		return err
	}
	link, lerr := s.repo().EventLink(ctx, l.uid, id)
	if err := ownedBy(link, lerr, l); err != nil {
		return err
	}
	if err := repoErr(s.repo().DeleteEvent(ctx, l.uid, id)); err != nil {
		return err
	}
	s.afterPortalWrite(ctx, l, "event")
	return nil
}

// ---------- session detail ----------

type PublicWeather struct {
	TemperatureC *float64 `json:"temperatureC"`
	ApparentC    *float64 `json:"apparentC"`
	HumidityPct  *float64 `json:"humidityPct"`
	WindKph      *float64 `json:"windKph"`
	Description  string   `json:"description"`
}

type PublicLap struct {
	Index         int      `json:"index"`
	Label         string   `json:"label"`
	DurationS     float64  `json:"durationS"`
	DistanceM     float64  `json:"distanceM"`
	PaceSecPerKm  *float64 `json:"paceSecPerKm"`
	AvgHR         *float64 `json:"avgHR"`
	AvgCadenceSpm *float64 `json:"avgCadenceSpm"`
	GctMs         *float64 `json:"gctMs"`
	ElevGainM     *float64 `json:"elevGainM"`
	ElevLossM     *float64 `json:"elevLossM"`
}

type PublicActivityShoe struct {
	Name       string   `json:"name"`
	DropMm     *float64 `json:"dropMm"`
	DistanceKm float64  `json:"distanceKm"`
}

// PublicActivityDetail is one session seen through a link with activityDetails: measured metrics,
// laps, shoe, pains and this link's remark. Never GPS, polyline or (anonymized) place names.
type PublicActivityDetail struct {
	ID                    int64               `json:"id"`
	Date                  string              `json:"date"`
	StartLocal            string              `json:"startLocal"`
	Name                  string              `json:"name"`
	Sport                 string              `json:"sport"`
	TypeLabel             string              `json:"typeLabel"`
	DistanceM             *float64            `json:"distanceM"`
	DurationS             *float64            `json:"durationS"`
	AvgPaceSecPerKm       *float64            `json:"avgPaceSecPerKm"`
	GapSecPerKm           *float64            `json:"gapSecPerKm"`
	AvgHR                 *float64            `json:"avgHR"`
	MaxHR                 *float64            `json:"maxHR"`
	EfSpeed               *float64            `json:"efSpeed"`
	AvgCadenceSpm         *float64            `json:"avgCadenceSpm"`
	GctMs                 *float64            `json:"gctMs"`
	VerticalOscillationCm *float64            `json:"verticalOscillationCm"`
	BalanceLeftPct        *float64            `json:"balanceLeftPct"`
	BalanceRightPct       *float64            `json:"balanceRightPct"`
	ElevGainM             *float64            `json:"elevGainM"`
	ElevLossM             *float64            `json:"elevLossM"`
	ImpactLoad            *float64            `json:"impactLoad"`
	TrainingLoad          *float64            `json:"trainingLoad"`
	Weather               *PublicWeather      `json:"weather"`
	Laps                  []PublicLap         `json:"laps"`
	Shoe                  *PublicActivityShoe `json:"shoe"`
	PainDuring            *float64            `json:"painDuring"`
	PainNextMorning       *float64            `json:"painNextMorning"`
	Remark                *athlete.ShareNote  `json:"remark"`
}

// PublicActivity returns one session of the link period in detail (activityDetails permission).
func (s *Services) PublicActivity(ctx context.Context, token string, aid int64) (*PublicActivityDetail, error) {
	l, err := s.portalLink(ctx, token, "activityDetails")
	if err != nil {
		return nil, err
	}
	if _, err := s.portalActivity(ctx, l, aid); err != nil {
		return nil, err
	}
	v, err := s.ActivityView(ctx, l.uid, aid)
	if err != nil {
		return nil, err
	}
	a, bm := v.Activity, v.Biomechanics
	d := &PublicActivityDetail{ID: a.ID, Date: a.Date, StartLocal: a.StartLocal, Name: a.Name, Sport: a.Sport, TypeLabel: a.TypeLabel,
		DistanceM: a.DistanceM, DurationS: a.DurationS, AvgPaceSecPerKm: a.AvgPaceSecPerKm, GapSecPerKm: a.GapSecPerKm, AvgHR: a.AvgHR,
		MaxHR: a.MaxHR, AvgCadenceSpm: a.AvgCadenceSpm, GctMs: bm.GroundContactTimeMs, VerticalOscillationCm: bm.VerticalOscillationCm,
		BalanceLeftPct: bm.GctBalance.LeftPct, BalanceRightPct: bm.GctBalance.RightPct, ElevGainM: a.ElevGainM, ElevLossM: a.ElevLossM,
		ImpactLoad: a.ImpactLoad, TrainingLoad: a.TrainingLoad, Laps: []PublicLap{},
		PainDuring: v.Notes.PainDuring, PainNextMorning: v.Notes.PainNextMorning}
	if l.scope.Anonymize {
		d.Name = a.TypeLabel
	}
	if v.Efficiency != nil && v.Efficiency.Eligible {
		d.EfSpeed = v.Efficiency.EfSpeed
	}
	if w := v.Weather; w != nil {
		d.Weather = &PublicWeather{TemperatureC: w.TemperatureC, ApparentC: w.ApparentC, HumidityPct: w.HumidityPct, WindKph: w.WindKph,
			Description: w.Description}
	}
	for _, sg := range v.Segments.PrimaryList() {
		d.Laps = append(d.Laps, PublicLap{Index: sg.Index, Label: sg.Label, DurationS: sg.DurationS, DistanceM: sg.DistanceM,
			PaceSecPerKm: sg.PaceSecPerKm, AvgHR: sg.AvgHR, AvgCadenceSpm: sg.AvgCadenceSpm, GctMs: sg.GctMs, ElevGainM: sg.ElevGainM,
			ElevLossM: sg.ElevLossM})
	}
	if sh := v.Notes.Shoe; sh != nil {
		d.Shoe = &PublicActivityShoe{Name: sh.Name, DropMm: sh.DropMm, DistanceKm: sh.DistanceKm}
	}
	_, remarks, err := s.linkNotes(ctx, l.uid, l.id)
	if err != nil {
		return nil, err
	}
	if n, ok := remarks[aid]; ok {
		d.Remark = &n
	}
	return d, nil
}
