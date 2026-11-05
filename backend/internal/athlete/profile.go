// Package athlete holds the manual athlete data (profile & focus presets, goals, plan, shoes,
// activity notes, daily check-ins): types, validation and SQLite queries scoped by user.
package athlete

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	FocusNone     = "none"
	FocusKnee     = "knee_pfps"
	FocusAchilles = "achilles"
	FocusShin     = "shin_splints"
	FocusOther    = "other"
)

type Thresholds struct {
	CadenceMinSpm            float64 `json:"cadenceMinSpm"`
	CadenceTargetMin         float64 `json:"cadenceTargetMin"`
	CadenceTargetMax         float64 `json:"cadenceTargetMax"`
	GctBalanceGapPct         float64 `json:"gctBalanceGapPct"`
	VerticalOscillationMaxCm float64 `json:"verticalOscillationMaxCm"`
	GctDriftMaxPct           float64 `json:"gctDriftMaxPct"`
	DescentMaxM              float64 `json:"descentMaxM"`
	FloorsPrevDayMax         float64 `json:"floorsPrevDayMax"`
	LoadRatioMax             float64 `json:"loadRatioMax"`
	RhrDeltaMaxBpm           float64 `json:"rhrDeltaMaxBpm"`
	PlanDeviationPct         float64 `json:"planDeviationPct"`
	DownhillCadenceDropSpm   float64 `json:"downhillCadenceDropSpm"`
}

type PaceTarget struct {
	Label       string  `json:"label"`
	MinSecPerKm float64 `json:"minSecPerKm"`
	MaxSecPerKm float64 `json:"maxSecPerKm"`
}

type Profile struct {
	FocusArea   string       `json:"focusArea"`
	FocusLabel  string       `json:"focusLabel"`
	InjuredSide *string      `json:"injuredSide"`
	Timezone    string       `json:"timezone"`
	Thresholds  Thresholds   `json:"thresholds"`
	PaceTargets []PaceTarget `json:"paceTargets"`
}

type PainLabels struct {
	During      string `json:"during"`
	Cold        string `json:"cold"`
	NextMorning string `json:"nextMorning"`
}

type Preset struct {
	FocusArea   string     `json:"focusArea"`
	Label       string     `json:"label"`
	Thresholds  Thresholds `json:"thresholds"`
	PainLabels  PainLabels `json:"painLabels"`
	PainEnabled bool       `json:"painEnabled"`
	Emphasis    []string   `json:"emphasis"`
	// InjuryAlerts enables the injury-specific alerts (descent, floors, downhill cadence).
	InjuryAlerts bool `json:"-"`
}

func baseThresholds() Thresholds {
	return Thresholds{
		CadenceMinSpm: 168, CadenceTargetMin: 170, CadenceTargetMax: 180,
		GctBalanceGapPct: 1.5, VerticalOscillationMaxCm: 9.5, GctDriftMaxPct: 5,
		DescentMaxM: 30, FloorsPrevDayMax: 15, LoadRatioMax: 1.3, RhrDeltaMaxBpm: 5,
		PlanDeviationPct: 20, DownhillCadenceDropSpm: 5,
	}
}

var genericPain = PainLabels{During: "Gêne pendant l'effort", Cold: "Gêne à froid (après)", NextMorning: "Gêne au réveil (lendemain)"}

func Presets() []Preset {
	none := baseThresholds()
	achilles := baseThresholds()
	achilles.CadenceMinSpm, achilles.GctDriftMaxPct, achilles.DescentMaxM, achilles.FloorsPrevDayMax = 170, 4, 60, 25
	shin := baseThresholds()
	shin.CadenceMinSpm, shin.LoadRatioMax, shin.VerticalOscillationMaxCm, shin.DescentMaxM, shin.FloorsPrevDayMax = 170, 1.2, 9, 50, 25
	return []Preset{
		{FocusArea: FocusNone, Label: "100 % Performance – Aucune blessure", Thresholds: none, PainLabels: genericPain,
			PainEnabled: false, Emphasis: []string{"load", "pace", "cadence"}},
		{FocusArea: FocusKnee, Label: "Genou / Fémoro-patellaire", Thresholds: baseThresholds(), InjuryAlerts: true, PainEnabled: true,
			PainLabels: PainLabels{During: "Douleur genou pendant l'effort", Cold: "Douleur genou à froid (après)", NextMorning: "Douleur genou au réveil (lendemain)"},
			Emphasis:   []string{"cadence", "downhill", "vertical_oscillation", "floors", "descent"}},
		{FocusArea: FocusAchilles, Label: "Tendon d'Achille", Thresholds: achilles, InjuryAlerts: true, PainEnabled: true,
			PainLabels: PainLabels{During: "Douleur tendon pendant l'effort", Cold: "Raideur / douleur à froid (après)", NextMorning: "Raideur matinale (lendemain)"},
			Emphasis:   []string{"cadence", "gct_drift", "uphill", "ascent", "calf_load"}},
		{FocusArea: FocusShin, Label: "Périostite tibiale", Thresholds: shin, InjuryAlerts: true, PainEnabled: true,
			PainLabels: PainLabels{During: "Douleur tibia pendant l'effort", Cold: "Douleur tibia à froid (après)", NextMorning: "Douleur tibia au réveil (lendemain)"},
			Emphasis:   []string{"cadence", "impact_load", "volume", "load", "surface"}},
		{FocusArea: FocusOther, Label: "Autre", Thresholds: baseThresholds(), InjuryAlerts: true, PainEnabled: true,
			PainLabels: genericPain, Emphasis: []string{"cadence", "load"}},
	}
}

func PresetFor(area string) Preset {
	for _, p := range Presets() {
		if p.FocusArea == area {
			return p
		}
	}
	return Presets()[0]
}

func DefaultProfile(tz string) Profile {
	if tz == "" {
		tz = "Europe/Paris"
	}
	p := PresetFor(FocusNone)
	return Profile{FocusArea: p.FocusArea, FocusLabel: p.Label, Timezone: tz, Thresholds: p.Thresholds, PaceTargets: []PaceTarget{}}
}

func (p Profile) Preset() Preset { return PresetFor(p.FocusArea) }

// ModeLabel is the « Profil / Mode actif » line of the prompts.
func (p Profile) ModeLabel() string {
	if p.FocusArea == FocusNone || p.FocusArea == "" {
		return "Performance"
	}
	label := strings.TrimSpace(p.FocusLabel)
	if label == "" {
		label = PresetFor(p.FocusArea).Label
	}
	return "Suivi Pathologie (" + label + ")"
}

func (p Profile) Location() *time.Location {
	if loc, err := time.LoadLocation(p.Timezone); err == nil {
		return loc
	}
	return time.UTC
}

// Hash identifies the inputs that change computed analyses (thresholds, targets, focus).
func (p Profile) Hash() string {
	b, _ := json.Marshal(struct {
		F  string
		I  *string
		T  Thresholds
		PT []PaceTarget
	}{p.FocusArea, p.InjuredSide, p.Thresholds, p.PaceTargets})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:6])
}

func between(v, lo, hi float64) bool { return !math.IsNaN(v) && v >= lo && v <= hi }

// Normalize validates the profile and fills defaults (label from preset, empty slices).
func (p *Profile) Normalize() error {
	var errs []error
	found := false
	for _, pr := range Presets() {
		if pr.FocusArea == p.FocusArea {
			found = true
		}
	}
	if !found {
		errs = append(errs, fmt.Errorf("focusArea invalide (%s)", strings.Join([]string{FocusNone, FocusKnee, FocusAchilles, FocusShin, FocusOther}, ", ")))
	}
	p.FocusLabel = strings.TrimSpace(p.FocusLabel)
	if p.FocusLabel == "" {
		p.FocusLabel = PresetFor(p.FocusArea).Label
	}
	if len([]rune(p.FocusLabel)) > 80 {
		errs = append(errs, errors.New("focusLabel trop long (80 caractères max)"))
	}
	if p.InjuredSide != nil {
		s := strings.ToLower(strings.TrimSpace(*p.InjuredSide))
		switch s {
		case "left", "right", "both":
			p.InjuredSide = &s
		case "":
			p.InjuredSide = nil
		default:
			errs = append(errs, errors.New("injuredSide doit valoir left, right, both ou null"))
		}
	}
	p.Timezone = strings.TrimSpace(p.Timezone)
	if p.Timezone == "" {
		p.Timezone = "Europe/Paris"
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		errs = append(errs, fmt.Errorf("fuseau horaire inconnu : %q", p.Timezone))
	}
	t := p.Thresholds
	checks := []struct {
		v      float64
		name   string
		lo, hi float64
	}{
		{t.CadenceMinSpm, "cadenceMinSpm", 100, 250}, {t.CadenceTargetMin, "cadenceTargetMin", 100, 250},
		{t.CadenceTargetMax, "cadenceTargetMax", 100, 250}, {t.GctBalanceGapPct, "gctBalanceGapPct", 0.1, 20},
		{t.VerticalOscillationMaxCm, "verticalOscillationMaxCm", 3, 20}, {t.GctDriftMaxPct, "gctDriftMaxPct", 0.5, 50},
		{t.DescentMaxM, "descentMaxM", 1, 10000}, {t.FloorsPrevDayMax, "floorsPrevDayMax", 1, 500},
		{t.LoadRatioMax, "loadRatioMax", 0.5, 3}, {t.RhrDeltaMaxBpm, "rhrDeltaMaxBpm", 1, 40},
		{t.PlanDeviationPct, "planDeviationPct", 1, 200}, {t.DownhillCadenceDropSpm, "downhillCadenceDropSpm", 1, 50},
	}
	for _, c := range checks {
		if !between(c.v, c.lo, c.hi) {
			errs = append(errs, fmt.Errorf("thresholds.%s hors limites [%g, %g]", c.name, c.lo, c.hi))
		}
	}
	if t.CadenceTargetMin > t.CadenceTargetMax {
		errs = append(errs, errors.New("cadenceTargetMin doit être ≤ cadenceTargetMax"))
	}
	if p.PaceTargets == nil {
		p.PaceTargets = []PaceTarget{}
	}
	if len(p.PaceTargets) > 10 {
		errs = append(errs, errors.New("10 allures cibles maximum"))
	}
	for i := range p.PaceTargets {
		pt := &p.PaceTargets[i]
		pt.Label = strings.TrimSpace(pt.Label)
		if pt.Label == "" || len([]rune(pt.Label)) > 40 {
			errs = append(errs, fmt.Errorf("paceTargets[%d].label requis (40 caractères max)", i))
		}
		if !between(pt.MinSecPerKm, 120, 1200) || !between(pt.MaxSecPerKm, 120, 1200) || pt.MinSecPerKm > pt.MaxSecPerKm {
			errs = append(errs, fmt.Errorf("paceTargets[%d] : bornes d'allure invalides (s/km, min ≤ max)", i))
		}
	}
	return errors.Join(errs...)
}

// overrides returns the thresholds differing from the preset (persisted form).
func overrides(t, preset Thresholds) map[string]float64 {
	var a, b map[string]float64
	ja, _ := json.Marshal(t)
	jb, _ := json.Marshal(preset)
	_ = json.Unmarshal(ja, &a)
	_ = json.Unmarshal(jb, &b)
	out := map[string]float64{}
	for k, v := range a {
		if b[k] != v {
			out[k] = v
		}
	}
	return out
}

func applyOverrides(preset Thresholds, ov map[string]float64) Thresholds {
	if len(ov) == 0 {
		return preset
	}
	var m map[string]float64
	jb, _ := json.Marshal(preset)
	_ = json.Unmarshal(jb, &m)
	for k, v := range ov {
		if _, ok := m[k]; ok {
			m[k] = v
		}
	}
	jm, _ := json.Marshal(m)
	out := preset
	_ = json.Unmarshal(jm, &out)
	return out
}

var tagLabels = map[string]string{
	"metronome":     "Métronome activé",
	"warmup_iso":    "Échauffement / Activation isométrique pré-course",
	"squatting_48h": "Travaux / Positions accroupies (48h)",
	"desk_sitting":  "Position assise bureau prolongée",
}

// TagLabel returns the French label of a context tag (the key itself for custom tags).
func TagLabel(tag string) string {
	if l, ok := tagLabels[tag]; ok {
		return l
	}
	return tag
}

func ShoeTypeLabel(t string) string {
	switch t {
	case "tempo":
		return "Tempo"
	case "carbon":
		return "Carbone"
	}
	return "Souple"
}
