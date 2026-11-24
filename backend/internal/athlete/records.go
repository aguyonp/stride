package athlete

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"stride/internal/workout"
)

type Goal struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	Kind        string   `json:"kind"`
	TargetValue *float64 `json:"targetValue"`
	TargetUnit  *string  `json:"targetUnit"`
	TargetDate  *string  `json:"targetDate"`
	Status      string   `json:"status"`
	Note        string   `json:"note"`
}

type PlanMatch struct {
	PlannedID         int64    `json:"plannedId"`
	ActivityID        *int64   `json:"activityId"`
	Status            string   `json:"status"` // done | partial | missed | upcoming | extra
	DurationDeltaPct  *float64 `json:"durationDeltaPct"`
	DistanceDeltaPct  *float64 `json:"distanceDeltaPct"`
	PaceDeltaSecPerKm *float64 `json:"paceDeltaSecPerKm"`
	CadenceDeltaSpm   *float64 `json:"cadenceDeltaSpm"`
	Summary           string   `json:"summary"`
}

type PlannedSession struct {
	ID                    int64          `json:"id"`
	Date                  string         `json:"date"`
	Sport                 string         `json:"sport"`
	Title                 string         `json:"title"`
	TargetDurationS       *float64       `json:"targetDurationS"`
	TargetDistanceM       *float64       `json:"targetDistanceM"`
	TargetPaceMinSecPerKm *float64       `json:"targetPaceMinSecPerKm"`
	TargetPaceMaxSecPerKm *float64       `json:"targetPaceMaxSecPerKm"`
	TargetCadenceSpm      *float64       `json:"targetCadenceSpm"`
	Instructions          string         `json:"instructions"`
	Steps                 []workout.Step `json:"steps"`
	Source                string         `json:"source"` // manual | llm | import | share
	Garmin                GarminPush     `json:"garmin"`
	Match                 *PlanMatch     `json:"match"`
	// PrescribedBy names the share link that created the session (physio/coach portal).
	PrescribedBy *Prescriber `json:"prescribedBy"`
	// Morph keeps the planned trace of a session adapted by Smart Workout Morphing.
	Morph *PlanMorph `json:"morph"`
}

// GarminPush is the state of the Garmin Connect workout push of a planned session.
type GarminPush struct {
	Status    string  `json:"status"` // none | pushed | failed | unsupported
	WorkoutID *int64  `json:"workoutId"`
	PushedAt  *string `json:"pushedAt"`
	Error     *string `json:"error"`
}

var PlanSources = []string{"manual", "llm", "import", "share"}

type Shoe struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Brand        string   `json:"brand"`
	Model        string   `json:"model"`
	DropMm       *float64 `json:"dropMm"`
	Type         string   `json:"type"`
	StravaGearID *string  `json:"stravaGearId"`
	Retired      bool     `json:"retired"`
	// DistanceKm is the pair's real total: Strava's all-time gear distance when known (linked pair),
	// else AnalyzedKm. Never a partial sum presented as the total.
	DistanceKm float64 `json:"distanceKm"`
	// AnalyzedKm sums the activities Stride holds for this pair (notes shoe, else matching Strava gear).
	AnalyzedKm float64 `json:"analyzedKm"`
	// DistanceSource tells where DistanceKm comes from: strava | stride.
	DistanceSource string `json:"distanceSource"`
	// StravaDistanceKm is the total distance reported by Strava for this gear (all-time).
	StravaDistanceKm *float64 `json:"stravaDistanceKm"`
	// StravaPrimary marks the athlete's default pair on Strava.
	StravaPrimary bool `json:"stravaPrimary"`
	// StravaState is the gear state seen at the last Strava sync: active | retired | deleted (nil: never synced).
	StravaState    *string `json:"stravaState"`
	StravaSyncedAt *string `json:"stravaSyncedAt"`
	Note           string  `json:"note"`
}

type ActivityNotes struct {
	PainDuring      *float64 `json:"painDuring"`
	PainCold        *float64 `json:"painCold"`
	PainNextMorning *float64 `json:"painNextMorning"`
	RPE             *float64 `json:"rpe"`
	ShoeID          *int64   `json:"shoeId"`
	Shoe            *Shoe    `json:"shoe"`
	// ShoeAuto is true when the shoe was assigned from the Strava gear, not by the athlete.
	ShoeAuto  bool     `json:"shoeAuto"`
	Tags      []string `json:"tags"`
	Note      string   `json:"note"`
	UpdatedAt *string  `json:"updatedAt"`
}

func (n ActivityNotes) IsEmpty() bool {
	return n.PainDuring == nil && n.PainCold == nil && n.PainNextMorning == nil && n.RPE == nil &&
		(n.ShoeID == nil || n.ShoeAuto) && len(n.Tags) == 0 && strings.TrimSpace(n.Note) == ""
}

type DailyCheckin struct {
	Date              string   `json:"date"`
	TrainingReadiness *float64 `json:"trainingReadiness"`
	ReadinessStatus   *string  `json:"readinessStatus"`
	Note              string   `json:"note"`
	UpdatedAt         string   `json:"updatedAt"`
}

var (
	GoalKinds    = []string{"macro", "race", "cadence", "weekly_volume", "other"}
	GoalStatuses = []string{"active", "done", "archived"}
	PlanSports   = []string{"run", "bike", "strength", "mobility", "rest", "other"}
	ShoeTypes    = []string{"soft", "tempo", "carbon"}
)

func oneOf(v string, set []string) bool {
	for _, s := range set {
		if v == s {
			return true
		}
	}
	return false
}

func ValidDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func maxLen(s string, n int, name string) error {
	if len([]rune(s)) > n {
		return fmt.Errorf("%s trop long (%d caractères max)", name, n)
	}
	return nil
}

func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	s := strings.TrimSpace(*p)
	if s == "" {
		return nil
	}
	return &s
}

func (g *Goal) Normalize() error {
	g.Title = strings.TrimSpace(g.Title)
	g.Note = strings.TrimSpace(g.Note)
	g.TargetUnit = trimPtr(g.TargetUnit)
	g.TargetDate = trimPtr(g.TargetDate)
	if g.Kind == "" {
		g.Kind = "other"
	}
	if g.Status == "" {
		g.Status = "active"
	}
	var errs []error
	if g.Title == "" {
		errs = append(errs, errors.New("titre requis"))
	}
	errs = append(errs, maxLen(g.Title, 120, "titre"), maxLen(g.Note, 2000, "note"))
	if g.TargetUnit != nil {
		errs = append(errs, maxLen(*g.TargetUnit, 20, "unité"))
	}
	if !oneOf(g.Kind, GoalKinds) {
		errs = append(errs, fmt.Errorf("kind invalide (%s)", strings.Join(GoalKinds, ", ")))
	}
	if !oneOf(g.Status, GoalStatuses) {
		errs = append(errs, fmt.Errorf("status invalide (%s)", strings.Join(GoalStatuses, ", ")))
	}
	if g.TargetDate != nil && !ValidDate(*g.TargetDate) {
		errs = append(errs, errors.New("targetDate attendue au format AAAA-MM-JJ"))
	}
	if g.TargetValue != nil && (math.IsNaN(*g.TargetValue) || math.IsInf(*g.TargetValue, 0)) {
		errs = append(errs, errors.New("targetValue invalide"))
	}
	return errors.Join(errs...)
}

func positive(p *float64, name string, hi float64) error {
	if p == nil {
		return nil
	}
	if !between(*p, 0, hi) {
		return fmt.Errorf("%s hors limites [0, %g]", name, hi)
	}
	return nil
}

func (ps *PlannedSession) Normalize() error {
	ps.Title = strings.TrimSpace(ps.Title)
	ps.Instructions = strings.TrimSpace(ps.Instructions)
	ps.Match = nil
	ps.Steps = workout.Normalize(ps.Steps)
	if ps.Source == "" {
		ps.Source = "manual"
	}
	if ps.Garmin.Status == "" {
		ps.Garmin.Status = "none"
	}
	var errs []error
	if !oneOf(ps.Source, PlanSources) {
		errs = append(errs, fmt.Errorf("source invalide (%s)", strings.Join(PlanSources, ", ")))
	}
	for _, e := range workout.Validate(ps.Steps, "") {
		errs = append(errs, errors.New(e))
	}
	if !ValidDate(ps.Date) {
		errs = append(errs, errors.New("date attendue au format AAAA-MM-JJ"))
	}
	if !oneOf(ps.Sport, PlanSports) {
		errs = append(errs, fmt.Errorf("sport invalide (%s)", strings.Join(PlanSports, ", ")))
	}
	errs = append(errs, maxLen(ps.Title, 120, "titre"), maxLen(ps.Instructions, 2000, "instructions"),
		positive(ps.TargetDurationS, "targetDurationS", 86400), positive(ps.TargetDistanceM, "targetDistanceM", 500000))
	for _, p := range []*float64{ps.TargetPaceMinSecPerKm, ps.TargetPaceMaxSecPerKm} {
		if p != nil && !between(*p, 120, 1800) {
			errs = append(errs, errors.New("allure cible hors limites [2:00, 30:00]/km"))
		}
	}
	if ps.TargetPaceMinSecPerKm != nil && ps.TargetPaceMaxSecPerKm != nil && *ps.TargetPaceMinSecPerKm > *ps.TargetPaceMaxSecPerKm {
		errs = append(errs, errors.New("targetPaceMinSecPerKm doit être ≤ targetPaceMaxSecPerKm (allure la plus rapide en premier)"))
	}
	if ps.TargetCadenceSpm != nil && !between(*ps.TargetCadenceSpm, 100, 250) {
		errs = append(errs, errors.New("targetCadenceSpm hors limites [100, 250]"))
	}
	return errors.Join(errs...)
}

func (s *Shoe) Normalize() error {
	s.Name, s.Brand, s.Model, s.Note = strings.TrimSpace(s.Name), strings.TrimSpace(s.Brand), strings.TrimSpace(s.Model), strings.TrimSpace(s.Note)
	s.StravaGearID = trimPtr(s.StravaGearID)
	if s.Type == "" {
		s.Type = "soft"
	}
	var errs []error
	if s.Name == "" {
		errs = append(errs, errors.New("nom requis"))
	}
	errs = append(errs, maxLen(s.Name, 80, "nom"), maxLen(s.Brand, 60, "marque"), maxLen(s.Model, 80, "modèle"), maxLen(s.Note, 1000, "note"))
	if !oneOf(s.Type, ShoeTypes) {
		errs = append(errs, fmt.Errorf("type invalide (%s)", strings.Join(ShoeTypes, ", ")))
	}
	if s.DropMm != nil && !between(*s.DropMm, 0, 20) {
		errs = append(errs, errors.New("dropMm hors limites [0, 20]"))
	}
	return errors.Join(errs...)
}

func halfStep(p *float64, name string, lo, hi float64) error {
	if p == nil {
		return nil
	}
	v := *p
	if !between(v, lo, hi) || math.Abs(v*2-math.Round(v*2)) > 1e-9 {
		return fmt.Errorf("%s doit être entre %g et %g par pas de 0,5", name, lo, hi)
	}
	return nil
}

func (n *ActivityNotes) Normalize() error {
	n.Note = strings.TrimSpace(n.Note)
	tags := make([]string, 0, len(n.Tags))
	seen := map[string]bool{}
	var errs []error
	for _, t := range n.Tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		if len([]rune(t)) > 40 {
			errs = append(errs, errors.New("tag trop long (40 caractères max)"))
			continue
		}
		seen[t] = true
		tags = append(tags, t)
	}
	n.Tags = tags
	if len(tags) > 20 {
		errs = append(errs, errors.New("20 tags maximum"))
	}
	errs = append(errs, halfStep(n.PainDuring, "painDuring", 0, 10), halfStep(n.PainCold, "painCold", 0, 10),
		halfStep(n.PainNextMorning, "painNextMorning", 0, 10), halfStep(n.RPE, "rpe", 1, 10), maxLen(n.Note, 4000, "note"))
	return errors.Join(errs...)
}

func (c *DailyCheckin) Normalize() error {
	c.Note = strings.TrimSpace(c.Note)
	c.ReadinessStatus = trimPtr(c.ReadinessStatus)
	var errs []error
	if c.TrainingReadiness != nil && !between(*c.TrainingReadiness, 0, 100) {
		errs = append(errs, errors.New("trainingReadiness doit être entre 0 et 100"))
	}
	if c.ReadinessStatus != nil {
		errs = append(errs, maxLen(*c.ReadinessStatus, 40, "readinessStatus"))
	}
	errs = append(errs, maxLen(c.Note, 2000, "note"))
	return errors.Join(errs...)
}

// ErrUnknownField is returned by Patch for keys outside the allowed set.
var ErrUnknownField = errors.New("champ inconnu")

// Patch overlays the JSON object body onto dst (a pointer to a struct). Keys in readOnly are
// silently ignored; keys missing from allowed are rejected. A JSON null clears pointer fields.
//
// The merged document is decoded into a fresh value: unmarshalling in place would merge the
// provided values with the current ones (encoding/json reuses slice elements and only
// overwrites the fields present), so a replaced `steps` array would inherit the sub-steps of
// the previous blocks, and a nested struct would keep whatever the body omits.
func Patch(dst any, body []byte, allowed, readOnly []string) error {
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(body, &patch); err != nil || patch == nil {
		return errors.New("corps JSON invalide (objet attendu)")
	}
	cur, err := json.Marshal(dst)
	if err != nil {
		return err
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(cur, &merged); err != nil {
		return err
	}
	for k, v := range patch {
		switch {
		case oneOf(k, readOnly):
		case oneOf(k, allowed):
			merged[k] = v
		default:
			return fmt.Errorf("%w : %s", ErrUnknownField, k)
		}
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return errors.New("destination invalide (pointeur vers une structure attendu)")
	}
	fresh := reflect.New(rv.Elem().Type())
	if err := json.Unmarshal(out, fresh.Interface()); err != nil {
		return fmt.Errorf("valeur invalide : %v", err)
	}
	rv.Elem().Set(fresh.Elem())
	return nil
}

var (
	GoalFields      = []string{"title", "kind", "targetValue", "targetUnit", "targetDate", "status", "note"}
	PlanFields      = []string{"date", "sport", "title", "targetDurationS", "targetDistanceM", "targetPaceMinSecPerKm", "targetPaceMaxSecPerKm", "targetCadenceSpm", "instructions", "steps", "source"}
	ShoeFields      = []string{"name", "brand", "model", "dropMm", "type", "stravaGearId", "retired", "note"}
	NotesFields     = []string{"painDuring", "painCold", "painNextMorning", "rpe", "shoeId", "tags", "note"}
	CheckinFields   = []string{"trainingReadiness", "readinessStatus", "note"}
	GoalReadOnly    = []string{"id"}
	PlanReadOnly    = []string{"id", "match", "garmin", "prescribedBy", "morph"}
	ShoeReadOnly    = []string{"id", "distanceKm", "analyzedKm", "distanceSource", "stravaDistanceKm", "stravaPrimary", "stravaState", "stravaSyncedAt"}
	NotesReadOnly   = []string{"shoe", "shoeAuto", "updatedAt"}
	CheckinReadOnly = []string{"date", "updatedAt"}
)
