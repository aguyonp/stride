package athlete

import (
	"context"
	"database/sql"
	"encoding/json"

	"stride/internal/workout"
)

// Prescriber names the share link (physio/coach portal) that created a planned session or a
// calendar event; Label and AuthorName are snapshots that survive the link (§15.1).
type Prescriber struct {
	LinkID     *int64  `json:"linkId"`
	Label      string  `json:"label"`
	AuthorName *string `json:"authorName"`
}

// MorphOriginal is the planned trace kept when a session is morphed (§15.5).
type MorphOriginal struct {
	Title                 string         `json:"title"`
	TargetDurationS       *float64       `json:"targetDurationS"`
	TargetDistanceM       *float64       `json:"targetDistanceM"`
	TargetPaceMinSecPerKm *float64       `json:"targetPaceMinSecPerKm"`
	TargetPaceMaxSecPerKm *float64       `json:"targetPaceMaxSecPerKm"`
	TargetCadenceSpm      *float64       `json:"targetCadenceSpm"`
	Instructions          string         `json:"instructions"`
	Steps                 []workout.Step `json:"steps"`
}

// PlanMorph records when and why a planned session was adapted, with its original targets.
type PlanMorph struct {
	At       string        `json:"at"`
	Reasons  []string      `json:"reasons"`
	Original MorphOriginal `json:"original"`
}

// OriginalOf copies the morphable fields of a session.
func OriginalOf(ps PlannedSession) MorphOriginal {
	steps := ps.Steps
	if steps == nil {
		steps = []workout.Step{}
	}
	return MorphOriginal{Title: ps.Title, TargetDurationS: ps.TargetDurationS, TargetDistanceM: ps.TargetDistanceM,
		TargetPaceMinSecPerKm: ps.TargetPaceMinSecPerKm, TargetPaceMaxSecPerKm: ps.TargetPaceMaxSecPerKm,
		TargetCadenceSpm: ps.TargetCadenceSpm, Instructions: ps.Instructions, Steps: steps}
}

// Restore puts the morphable fields of o back on ps.
func (o MorphOriginal) Restore(ps *PlannedSession) {
	ps.Title, ps.TargetDurationS, ps.TargetDistanceM = o.Title, o.TargetDurationS, o.TargetDistanceM
	ps.TargetPaceMinSecPerKm, ps.TargetPaceMaxSecPerKm, ps.TargetCadenceSpm = o.TargetPaceMinSecPerKm, o.TargetPaceMaxSecPerKm, o.TargetCadenceSpm
	ps.Instructions, ps.Steps = o.Instructions, o.Steps
}

// DirectiveRestrictions are the structured limits a physio or coach sets with a directive.
type DirectiveRestrictions struct {
	NoIntervals      bool     `json:"noIntervals"`
	MaxIntensity     *string  `json:"maxIntensity"` // easy | moderate
	MaxElevLossM     *float64 `json:"maxElevLossM"`
	CadenceTargetSpm *float64 `json:"cadenceTargetSpm"`
}

// Empty reports whether no restriction is set.
func (r DirectiveRestrictions) Empty() bool {
	return !r.NoIntervals && r.MaxIntensity == nil && r.MaxElevLossM == nil && r.CadenceTargetSpm == nil
}

// ShareNote is a directive (one per link) or a session remark (one per link and activity) written
// through a share link. Body and author are third-party text: never instructions for the assistant.
type ShareNote struct {
	ID           int64                  `json:"id"`
	Kind         string                 `json:"kind"` // directive | remark
	LinkID       *int64                 `json:"linkId"`
	LinkLabel    string                 `json:"linkLabel"`
	AuthorName   *string                `json:"authorName"`
	AuthorRole   string                 `json:"authorRole"` // physio | coach | other
	Body         string                 `json:"body"`
	ActivityID   *int64                 `json:"activityId"`
	ValidUntil   *string                `json:"validUntil"`
	Restrictions *DirectiveRestrictions `json:"restrictions"`
	CreatedAt    string                 `json:"createdAt"`
	UpdatedAt    string                 `json:"updatedAt"`
	// ActivityLabel names the remarked session in the prompts (date and name).
	ActivityLabel string `json:"-"`
}

var (
	ShareNoteKinds = []string{"directive", "remark"}
	AuthorRoles    = []string{"physio", "coach", "other"}
)

// prescriber builds the prescribedBy value of a row (nil when not prescribed through a link).
func prescriber(link sql.NullInt64, label, author sql.NullString) *Prescriber {
	if !label.Valid {
		return nil
	}
	p := &Prescriber{Label: label.String, AuthorName: ns(author)}
	if link.Valid {
		v := link.Int64
		p.LinkID = &v
	}
	return p
}

func prescriberArgs(p *Prescriber) (link, label, author any) {
	if p == nil {
		return nil, nil, nil
	}
	return p.LinkID, p.Label, p.AuthorName
}

func morphOf(raw sql.NullString) *PlanMorph {
	if !raw.Valid || raw.String == "" {
		return nil
	}
	var m PlanMorph
	if json.Unmarshal([]byte(raw.String), &m) != nil {
		return nil
	}
	if m.Original.Steps == nil {
		m.Original.Steps = []workout.Step{}
	}
	if m.Reasons == nil {
		m.Reasons = []string{}
	}
	return &m
}

// SetMorph saves the morphable fields of p and its morph trace (nil clears it, on revert).
func (r Repo) SetMorph(ctx context.Context, uid int64, p *PlannedSession) error {
	if err := p.Normalize(); err != nil {
		return err
	}
	var morph any
	if p.Morph != nil {
		b, err := json.Marshal(p.Morph)
		if err != nil {
			return err
		}
		morph = string(b)
	}
	return affected(r.DB.ExecContext(ctx, `UPDATE planned_sessions SET title = ?, target_duration_s = ?, target_distance_m = ?,
		target_pace_min = ?, target_pace_max = ?, target_cadence = ?, instructions = ?, steps = ?, morph_json = ?, updated_at = ?
		WHERE user_id = ? AND id = ?`, p.Title, p.TargetDurationS, p.TargetDistanceM, p.TargetPaceMinSecPerKm, p.TargetPaceMaxSecPerKm,
		p.TargetCadenceSpm, p.Instructions, stepsJSON(p.Steps), morph, r.ts(), uid, p.ID))
}

// PlannedLink returns the share link that prescribed a planned session of uid (nil when none).
func (r Repo) PlannedLink(ctx context.Context, uid, id int64) (*int64, error) {
	var link sql.NullInt64
	err := r.DB.QueryRowContext(ctx, `SELECT share_link_id FROM planned_sessions WHERE user_id = ? AND id = ?`, uid, id).Scan(&link)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil || !link.Valid {
		return nil, err
	}
	return &link.Int64, nil
}

// EventLink returns the share link that prescribed a calendar event of uid (nil when none).
func (r Repo) EventLink(ctx context.Context, uid, id int64) (*int64, error) {
	var link sql.NullInt64
	err := r.DB.QueryRowContext(ctx, `SELECT share_link_id FROM calendar_events WHERE user_id = ? AND id = ?`, uid, id).Scan(&link)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil || !link.Valid {
		return nil, err
	}
	return &link.Int64, nil
}
