package domain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"stride/internal/analysis"
	"stride/internal/athlete"
	"stride/internal/frfmt"
)

const (
	HealthPerformance = "performance"
	HealthTracking    = "tracking"
)

// HealthZone is a body zone under follow-up, optionally qualified by a catalog condition (§14.7).
type HealthZone = analysis.HealthZone

type HealthStatusInput struct {
	Mode  string       `json:"mode"`
	Zones []HealthZone `json:"zones"`
}

// HealthStatus is the golden source of the athlete's focus: the profile focusArea / injuredSide
// are derived from it.
type HealthStatus struct {
	Mode      string       `json:"mode"`
	Zones     []HealthZone `json:"zones"`
	FocusArea string       `json:"focusArea"`
	UpdatedAt *string      `json:"updatedAt"`
}

// HealthStatus returns the stored status, or the one implied by a pre-4.1 profile.
func (s *Services) HealthStatus(ctx context.Context, uid int64) (*HealthStatus, error) {
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := &HealthStatus{Mode: HealthPerformance, Zones: []HealthZone{}, FocusArea: p.FocusArea}
	var raw string
	var at int64
	err = s.db().QueryRowContext(ctx, `SELECT zones, updated_at FROM health_status WHERE user_id = ?`, uid).Scan(&raw, &at)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if z, ok := zoneOfFocus(p.FocusArea); ok {
			side := analysis.SideBoth
			if p.InjuredSide != nil {
				side = analysis.BodySide(*p.InjuredSide)
			}
			out.Zones = []HealthZone{{Zone: z, Side: side}}
		}
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal([]byte(raw), &out.Zones); err != nil {
			return nil, err
		}
		out.UpdatedAt = rfc3339(at)
	}
	for i := range out.Zones {
		out.Zones[i].ConditionLabel = analysis.ConditionLabelIn(out.Zones[i], s.lang(ctx, p))
	}
	if len(out.Zones) > 0 {
		out.Mode = HealthTracking
	}
	return out, nil
}

// PutHealthStatus stores the status and derives the profile focus from its most recent zone.
func (s *Services) PutHealthStatus(ctx context.Context, uid int64, in HealthStatusInput) (*HealthStatus, error) {
	zones, err := normalizeHealth(in)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(zones)
	if _, err := s.db().ExecContext(ctx, `INSERT INTO health_status (user_id, zones, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET zones = excluded.zones, updated_at = excluded.updated_at`, uid, string(raw), s.now().Unix()); err != nil {
		return nil, err
	}
	area, label, side := athlete.FocusNone, "", (*string)(nil)
	if z := focusZone(zones); z != nil {
		area, label = focusOfZone(*z)
		v := string(z.Side)
		side = &v
	}
	body, _ := json.Marshal(map[string]any{"focusArea": area, "focusLabel": label, "injuredSide": side})
	if _, err := s.saveProfile(ctx, uid, body); err != nil {
		return nil, err
	}
	return s.HealthStatus(ctx, uid)
}

func normalizeHealth(in HealthStatusInput) ([]HealthZone, error) {
	switch in.Mode {
	case HealthPerformance:
		if len(in.Zones) > 0 {
			return nil, badRequest("Le mode performance n'a aucune zone suivie.")
		}
		return []HealthZone{}, nil
	case HealthTracking:
	default:
		return nil, badRequest("mode doit valoir performance ou tracking.")
	}
	if len(in.Zones) == 0 || len(in.Zones) > len(analysis.AllBodyZones) {
		return nil, badRequest("Le suivi demande entre 1 et %d zones.", len(analysis.AllBodyZones))
	}
	seen := map[analysis.BodyZone]bool{}
	out := make([]HealthZone, 0, len(in.Zones))
	for _, z := range in.Zones {
		if !isValidZone(z.Zone) || seen[z.Zone] {
			return nil, badRequest("Zone anatomique inconnue ou en double.")
		}
		seen[z.Zone] = true
		switch z.Side {
		case "":
			z.Side = analysis.SideBoth
		case analysis.SideLeft, analysis.SideRight, analysis.SideBoth:
		default:
			return nil, badRequest("Côté anatomique invalide (left | right | both).")
		}
		if z.Since != nil && (*z.Since == "" || !isValidDate(*z.Since)) {
			return nil, badRequest("since attendu au format AAAA-MM-JJ.")
		}
		if z.Note = strings.TrimSpace(z.Note); len([]rune(z.Note)) > 300 {
			return nil, badRequest("Note trop longue (300 caractères max).")
		}
		if err := normalizeCondition(&z); err != nil {
			return nil, err
		}
		out = append(out, z)
	}
	return out, nil
}

// focusZone is the most recent zone by since (undated zones are the oldest; on ties the last wins).
func focusZone(zones []HealthZone) *HealthZone {
	var best *HealthZone
	for i := range zones {
		z := &zones[i]
		switch {
		case best == nil, z.Since == nil && best.Since == nil:
			best = z
		case z.Since != nil && (best.Since == nil || *z.Since >= *best.Since):
			best = z
		}
	}
	return best
}

// normalizeCondition validates the optional condition of a tracked zone: a key of the zone's
// catalog list, or « other » with a 1–80 character customLabel (ignored for any other key).
// The derived conditionLabel is never stored.
func normalizeCondition(z *HealthZone) error {
	z.Condition, z.ConditionLabel = strings.TrimSpace(z.Condition), ""
	switch {
	case z.Condition == "":
		z.CustomLabel = ""
	case !analysis.ValidCondition(z.Zone, z.Condition):
		return badRequest("Pathologie inconnue pour la zone %s.", analysis.BodyZoneLabel(z.Zone))
	case z.Condition == analysis.ConditionOther:
		label, ok := analysis.NormalizeCustomLabel(z.CustomLabel)
		if !ok {
			return badRequest("Précisez la pathologie (1 à %d caractères, texte simple).", analysis.CustomLabelMax)
		}
		z.CustomLabel = label
	default:
		z.CustomLabel = ""
	}
	return nil
}

// focusOfZone maps a tracked zone to the profile focus: a condition with a dedicated preset gives
// it, any other condition gives `other` labelled with the condition (or the custom label); without
// condition the 4.1 zone mapping applies.
func focusOfZone(z HealthZone) (area, label string) {
	if z.Condition != "" {
		if p := analysis.ConditionPreset(z.Zone, z.Condition); p != "" {
			return p, ""
		}
		return athlete.FocusOther, analysis.ConditionLabelOf(z)
	}
	switch z.Zone {
	case analysis.ZoneKneePFPS:
		return athlete.FocusKnee, ""
	case analysis.ZoneAchilles:
		return athlete.FocusAchilles, ""
	case analysis.ZoneShinSplints:
		return athlete.FocusShin, ""
	}
	return athlete.FocusOther, analysis.BodyZoneLabel(z.Zone)
}

func zoneOfFocus(area string) (analysis.BodyZone, bool) {
	switch area {
	case athlete.FocusKnee:
		return analysis.ZoneKneePFPS, true
	case athlete.FocusAchilles:
		return analysis.ZoneAchilles, true
	case athlete.FocusShin:
		return analysis.ZoneShinSplints, true
	}
	return "", false
}

// healthFromProfileBody turns a legacy profile focus change into a health status update.
func (s *Services) healthFromProfileBody(ctx context.Context, uid int64, keys map[string]json.RawMessage) error {
	cur, err := s.HealthStatus(ctx, uid)
	if err != nil {
		return err
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return err
	}
	area, side := p.FocusArea, p.InjuredSide
	if raw, ok := keys["focusArea"]; ok && json.Unmarshal(raw, &area) != nil {
		return badRequest("focusArea invalide.")
	}
	if raw, ok := keys["injuredSide"]; ok && json.Unmarshal(raw, &side) != nil {
		return badRequest("injuredSide invalide.")
	}
	in := HealthStatusInput{Mode: HealthPerformance}
	if area != athlete.FocusNone {
		z, ok := zoneOfFocus(area)
		if !ok {
			return badRequest("Déclarez la zone suivie dans Santé & forme.")
		}
		hz := HealthZone{Zone: z, Side: analysis.SideBoth}
		for _, c := range cur.Zones {
			if c.Zone == z {
				hz = c
			}
		}
		if side != nil {
			hz.Side = analysis.BodySide(*side)
		}
		in = HealthStatusInput{Mode: HealthTracking, Zones: []HealthZone{hz}}
	}
	_, err = s.PutHealthStatus(ctx, uid, in)
	return err
}

// HealthConditions is the pathology catalog (GET /api/health/conditions, §14.7).
// HealthConditions is the pathology catalog in the effective language (§15.2).
func (s *Services) HealthConditions(ctx context.Context, uid int64) analysis.ConditionCatalog {
	l := frfmt.Resolve(nil, frfmt.LangFrom(ctx))
	if p, err := s.profile(ctx, uid); err == nil {
		l = s.lang(ctx, p)
	}
	return analysis.ConditionsIn(l)
}

// trackedZones returns the zones under follow-up (empty in performance mode).
func (s *Services) trackedZones(ctx context.Context, uid int64) ([]HealthZone, error) {
	st, err := s.HealthStatus(ctx, uid)
	if err != nil {
		return nil, err
	}
	return st.Zones, nil
}

// putActivityPains writes the pains entered for an activity as body entries of the tracked zones
// (one per zone: pain_score = during, cold_next_day_score = next morning). It returns false when
// no zone is tracked (the pains then stay on the notes).
func (s *Services) putActivityPains(ctx context.Context, uid int64, a *activityRow, during, nextMorning *float64) (bool, error) {
	zones, err := s.trackedZones(ctx, uid)
	if err != nil || len(zones) == 0 {
		return false, err
	}
	for _, z := range zones {
		if during == nil && nextMorning == nil {
			if _, err := s.db().ExecContext(ctx, `DELETE FROM body_stress_entries WHERE user_id = ? AND activity_id = ? AND zone = ?`, uid, a.ID, string(z.Zone)); err != nil {
				return false, err
			}
			continue
		}
		pain := 0.0
		if during != nil {
			pain = *during
		}
		if _, err := s.upsertActivityEntry(ctx, uid, analysis.BodyStressInput{Date: a.LocalDate, ActivityID: &a.ID, Zone: z.Zone,
			Side: &z.Side, PainScore: pain, ColdNextDayScore: nextMorning}); err != nil {
			return false, err
		}
	}
	return true, nil
}

// upsertActivityEntry keeps one entry per activity and zone, and a notes row so the notes project the pains.
func (s *Services) upsertActivityEntry(ctx context.Context, uid int64, in analysis.BodyStressInput) (int64, error) {
	now := s.now().Unix()
	side := analysis.SideBoth
	if in.Side != nil {
		side = *in.Side
	}
	var id int64
	err := s.db().QueryRowContext(ctx, `SELECT id FROM body_stress_entries WHERE user_id = ? AND activity_id = ? AND zone = ? ORDER BY id LIMIT 1`,
		uid, *in.ActivityID, string(in.Zone)).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, err := s.db().ExecContext(ctx, `INSERT INTO body_stress_entries (user_id, date, activity_id, zone, side, pain_score, cold_next_day_score, note, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uid, in.Date, *in.ActivityID, string(in.Zone), string(side), in.PainScore, in.ColdNextDayScore,
			trimmed(in.Note, ""), now, now)
		if err != nil {
			return 0, err
		}
		if id, err = res.LastInsertId(); err != nil {
			return 0, err
		}
	case err != nil:
		return 0, err
	default:
		if _, err := s.db().ExecContext(ctx, `UPDATE body_stress_entries SET date = ?, side = ?, pain_score = ?, cold_next_day_score = ?,
			note = COALESCE(?, note), updated_at = ? WHERE id = ? AND user_id = ?`, in.Date, string(side), in.PainScore, in.ColdNextDayScore,
			in.Note, now, id, uid); err != nil {
			return 0, err
		}
	}
	_, err = s.db().ExecContext(ctx, `INSERT INTO activity_notes (activity_id, user_id, updated_at) VALUES (?, ?, ?) ON CONFLICT (activity_id) DO NOTHING`,
		*in.ActivityID, uid, now)
	return id, err
}
