package domain

import (
	"context"
	"encoding/json"
	"errors"

	"stride/internal/athlete"
)

func validation(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, athlete.ErrConflict) {
		return &ConflictError{Msg: err.Error()}
	}
	return &BadRequestError{Msg: "Données invalides : " + err.Error()}
}

func repoErr(err error) error {
	if errors.Is(err, athlete.ErrNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, athlete.ErrConflict) {
		return &ConflictError{Msg: err.Error()}
	}
	return err
}

func patch(dst any, body []byte, allowed, readOnly []string) error {
	if err := athlete.Patch(dst, body, allowed, readOnly); err != nil {
		return &BadRequestError{Msg: err.Error()}
	}
	return nil
}

// ---------- profile ----------

func (s *Services) Profile(ctx context.Context, uid int64) (athlete.Profile, error) { return s.profile(ctx, uid) }

func (s *Services) Presets() []athlete.Preset { return athlete.Presets() }

// PutProfile applies the body onto the current profile; switching focus area without thresholds
// resets them to the new preset.
func (s *Services) PutProfile(ctx context.Context, uid int64, body []byte) (athlete.Profile, error) {
	cur, err := s.profile(ctx, uid)
	if err != nil {
		return cur, err
	}
	var keys map[string]json.RawMessage
	_ = json.Unmarshal(body, &keys)
	next := cur
	if err := patch(&next, body, []string{"focusArea", "focusLabel", "injuredSide", "timezone", "thresholds", "paceTargets"}, nil); err != nil {
		return cur, err
	}
	if _, hasTh := keys["thresholds"]; !hasTh && next.FocusArea != cur.FocusArea {
		next.Thresholds = athlete.PresetFor(next.FocusArea).Thresholds
	}
	if _, hasLabel := keys["focusLabel"]; !hasLabel && next.FocusArea != cur.FocusArea {
		next.FocusLabel = ""
	}
	if err := next.Normalize(); err != nil {
		return cur, validation(err)
	}
	if err := s.repo().PutProfile(ctx, uid, next); err != nil {
		return cur, err
	}
	return s.profile(ctx, uid)
}

// ---------- goals ----------

func (s *Services) Goals(ctx context.Context, uid int64) ([]athlete.Goal, error) {
	return s.repo().Goals(ctx, uid, false)
}

func (s *Services) CreateGoal(ctx context.Context, uid int64, body []byte) (athlete.Goal, error) {
	g := athlete.Goal{Kind: "other", Status: "active"}
	if err := patch(&g, body, athlete.GoalFields, athlete.GoalReadOnly); err != nil {
		return g, err
	}
	if err := g.Normalize(); err != nil {
		return g, validation(err)
	}
	return g, s.repo().CreateGoal(ctx, uid, &g)
}

func (s *Services) PatchGoal(ctx context.Context, uid, id int64, body []byte) (athlete.Goal, error) {
	g, err := s.repo().Goal(ctx, uid, id)
	if err != nil {
		return g, repoErr(err)
	}
	if err := patch(&g, body, athlete.GoalFields, athlete.GoalReadOnly); err != nil {
		return g, err
	}
	g.ID = id
	if err := g.Normalize(); err != nil {
		return g, validation(err)
	}
	return g, repoErr(s.repo().UpdateGoal(ctx, uid, &g))
}

func (s *Services) DeleteGoal(ctx context.Context, uid, id int64) error {
	return repoErr(s.repo().DeleteGoal(ctx, uid, id))
}

// ---------- plan ----------

func (s *Services) CreatePlanned(ctx context.Context, uid int64, body []byte) (athlete.PlannedSession, error) {
	ps := athlete.PlannedSession{Sport: "run"}
	if err := patch(&ps, body, athlete.PlanFields, athlete.PlanReadOnly); err != nil {
		return ps, err
	}
	if err := ps.Normalize(); err != nil {
		return ps, validation(err)
	}
	if err := s.repo().CreatePlanned(ctx, uid, &ps); err != nil {
		return ps, err
	}
	return s.plannedWithMatch(ctx, uid, ps.ID)
}

func (s *Services) PatchPlanned(ctx context.Context, uid, id int64, body []byte) (athlete.PlannedSession, error) {
	ps, err := s.repo().Planned(ctx, uid, id)
	if err != nil {
		return ps, repoErr(err)
	}
	if err := patch(&ps, body, athlete.PlanFields, athlete.PlanReadOnly); err != nil {
		return ps, err
	}
	ps.ID = id
	if err := ps.Normalize(); err != nil {
		return ps, validation(err)
	}
	if err := s.repo().UpdatePlanned(ctx, uid, &ps); err != nil {
		return ps, repoErr(err)
	}
	return s.plannedWithMatch(ctx, uid, id)
}

func (s *Services) DeletePlanned(ctx context.Context, uid, id int64) error {
	return repoErr(s.repo().DeletePlanned(ctx, uid, id))
}

// ---------- shoes ----------

func (s *Services) Shoes(ctx context.Context, uid int64) ([]athlete.Shoe, error) { return s.repo().Shoes(ctx, uid) }

func (s *Services) CreateShoe(ctx context.Context, uid int64, body []byte) (athlete.Shoe, error) {
	sh := athlete.Shoe{Type: "soft"}
	if err := patch(&sh, body, athlete.ShoeFields, athlete.ShoeReadOnly); err != nil {
		return sh, err
	}
	if err := sh.Normalize(); err != nil {
		return sh, validation(err)
	}
	if err := s.repo().CreateShoe(ctx, uid, &sh); err != nil {
		return sh, repoErr(err)
	}
	return s.repo().Shoe(ctx, uid, sh.ID)
}

func (s *Services) PatchShoe(ctx context.Context, uid, id int64, body []byte) (athlete.Shoe, error) {
	sh, err := s.repo().Shoe(ctx, uid, id)
	if err != nil {
		return sh, repoErr(err)
	}
	if err := patch(&sh, body, athlete.ShoeFields, athlete.ShoeReadOnly); err != nil {
		return sh, err
	}
	sh.ID = id
	if err := sh.Normalize(); err != nil {
		return sh, validation(err)
	}
	if err := s.repo().UpdateShoe(ctx, uid, &sh); err != nil {
		return sh, repoErr(err)
	}
	return s.repo().Shoe(ctx, uid, id)
}

func (s *Services) DeleteShoe(ctx context.Context, uid, id int64) error {
	return repoErr(s.repo().DeleteShoe(ctx, uid, id))
}

// ---------- notes & check-ins ----------

// PutNotes merges the partial body into the notes of an activity owned by uid.
func (s *Services) PutNotes(ctx context.Context, uid, aid int64, body []byte) (athlete.ActivityNotes, error) {
	if _, err := s.activity(ctx, uid, aid); err != nil {
		return athlete.ActivityNotes{}, err
	}
	n, err := s.repo().NotesFor(ctx, uid, aid)
	if err != nil {
		return n, err
	}
	if err := patch(&n, body, athlete.NotesFields, athlete.NotesReadOnly); err != nil {
		return n, err
	}
	if err := n.Normalize(); err != nil {
		return n, validation(err)
	}
	if err := s.repo().PutNotes(ctx, uid, aid, &n); err != nil {
		return n, validation(err)
	}
	return s.repo().NotesFor(ctx, uid, aid)
}

func (s *Services) PutCheckin(ctx context.Context, uid int64, date string, body []byte) (athlete.DailyCheckin, error) {
	cur, err := s.repo().Checkin(ctx, uid, date)
	if err != nil {
		return athlete.DailyCheckin{}, err
	}
	c := athlete.DailyCheckin{Date: date}
	if cur != nil {
		c = *cur
	}
	if err := patch(&c, body, athlete.CheckinFields, athlete.CheckinReadOnly); err != nil {
		return c, err
	}
	c.Date = date
	if err := c.Normalize(); err != nil {
		return c, validation(err)
	}
	if err := s.repo().PutCheckin(ctx, uid, &c); err != nil {
		return c, err
	}
	return c, nil
}
