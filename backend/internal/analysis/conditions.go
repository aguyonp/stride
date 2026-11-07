package analysis

import (
	"strings"
	"unicode"

	"stride/internal/athlete"
	"stride/internal/frfmt"
)

// ConditionOther is the catalog key of a condition described by the athlete (customLabel).
const ConditionOther = "other"

// CustomLabelMax bounds the free-text condition label (runes, after trimming).
const CustomLabelMax = 80

// Condition is a pathology of the health catalog, attached to one body zone.
type Condition struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// preset is the dedicated focus preset of the condition ("" → athlete.FocusOther).
	preset string
}

// ZoneConditions lists the conditions offered for one zone (« Autre (préciser) » last).
type ZoneConditions struct {
	Zone       BodyZone    `json:"zone"`
	ZoneLabel  string      `json:"zoneLabel"`
	Conditions []Condition `json:"conditions"`
}

// ConditionCatalog is the GET /api/health/conditions payload.
type ConditionCatalog struct {
	Zones []ZoneConditions `json:"zones"`
}

var otherCondition = Condition{Key: ConditionOther, Label: "Autre (préciser)"}

// conditionsByZone is the medically sensible (not exhaustive) list of running conditions per zone.
var conditionsByZone = map[BodyZone][]Condition{
	ZoneKneePFPS: {
		{Key: "patellofemoral_pain", Label: "Syndrome fémoro-patellaire", preset: athlete.FocusKnee},
		{Key: "patellar_tendinopathy", Label: "Tendinopathie rotulienne"},
		{Key: "pes_anserine_tendinopathy", Label: "Tendinopathie de la patte d’oie"},
		{Key: "meniscus_tear", Label: "Lésion méniscale"},
		{Key: "knee_ligament_sprain", Label: "Entorse du genou (ligament)"},
	},
	ZoneShinSplints: {
		{Key: "medial_tibial_stress", Label: "Périostite tibiale", preset: athlete.FocusShin},
		{Key: "tibial_stress_fracture", Label: "Fracture de fatigue du tibia"},
		{Key: "compartment_syndrome", Label: "Syndrome des loges"},
	},
	ZoneIliotibialBand: {
		{Key: "iliotibial_band_syndrome", Label: "Syndrome de la bandelette ilio-tibiale"},
	},
	ZoneAchilles: {
		{Key: "achilles_tendinopathy", Label: "Tendinopathie d’Achille", preset: athlete.FocusAchilles},
		{Key: "retrocalcaneal_bursitis", Label: "Bursite rétro-calcanéenne"},
		{Key: "achilles_partial_tear", Label: "Rupture partielle du tendon d’Achille"},
	},
	ZonePlantarFascia: {
		{Key: "plantar_fasciitis", Label: "Aponévrosite plantaire"},
		{Key: "calcaneal_stress_fracture", Label: "Fracture de fatigue du calcanéum"},
	},
	ZoneHamstringsCalves: {
		{Key: "hamstring_strain", Label: "Lésion musculaire des ischio-jambiers"},
		{Key: "calf_strain", Label: "Lésion musculaire du mollet"},
		{Key: "proximal_hamstring_tendinopathy", Label: "Tendinopathie proximale des ischio-jambiers"},
	},
	ZoneAnkle: {
		{Key: "ankle_sprain", Label: "Entorse de cheville"},
		{Key: "ankle_fracture", Label: "Fracture de cheville (malléole)"},
		{Key: "peroneal_tendinopathy", Label: "Tendinopathie des fibulaires"},
		{Key: "tibialis_posterior_tendinopathy", Label: "Tendinopathie du tibial postérieur"},
		{Key: "chronic_ankle_instability", Label: "Instabilité chronique de cheville"},
	},
	ZoneForefoot: {
		{Key: "metatarsal_stress_fracture", Label: "Fracture de fatigue métatarsienne"},
		{Key: "metatarsalgia", Label: "Métatarsalgie"},
		{Key: "morton_neuroma", Label: "Névrome de Morton"},
	},
	ZoneHip: {
		{Key: "gluteal_tendinopathy", Label: "Tendinopathie fessière (moyen fessier)"},
		{Key: "hip_flexor_tendinopathy", Label: "Tendinopathie du psoas"},
		{Key: "pubalgia", Label: "Pubalgie"},
		{Key: "femoral_neck_stress_fracture", Label: "Fracture de fatigue du col du fémur"},
	},
	ZoneLowerBack: {
		{Key: "low_back_pain", Label: "Lombalgie"},
		{Key: "sciatica", Label: "Sciatique"},
		{Key: "sacroiliac_pain", Label: "Douleur sacro-iliaque"},
	},
}

// Conditions returns the catalog, zones in canonical order, « Autre (préciser) » closing each list.
func Conditions() ConditionCatalog {
	out := ConditionCatalog{Zones: make([]ZoneConditions, 0, len(AllBodyZones))}
	for _, z := range AllBodyZones {
		list := append(append([]Condition{}, conditionsByZone[z]...), otherCondition)
		out.Zones = append(out.Zones, ZoneConditions{Zone: z, ZoneLabel: BodyZoneLabel(z), Conditions: list})
	}
	return out
}

func findCondition(zone BodyZone, key string) (Condition, bool) {
	if key == ConditionOther {
		return otherCondition, true
	}
	for _, c := range conditionsByZone[zone] {
		if c.Key == key {
			return c, true
		}
	}
	return Condition{}, false
}

// ValidCondition reports whether key belongs to zone's list ("other" is valid for every zone).
func ValidCondition(zone BodyZone, key string) bool {
	_, ok := findCondition(zone, key)
	return ok
}

// ConditionPreset is the dedicated focus preset of a condition ("" when it has none).
func ConditionPreset(zone BodyZone, key string) string {
	c, _ := findCondition(zone, key)
	return c.preset
}

// NormalizeCustomLabel trims a free-text condition label and reports whether it is acceptable:
// 1 to CustomLabelMax runes, printable text only.
func NormalizeCustomLabel(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len([]rune(s)) > CustomLabelMax {
		return s, false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return s, false
		}
	}
	return s, true
}

// HealthZone is a body zone under follow-up, optionally qualified by a catalog condition
// (customLabel describes the « other » condition). ConditionLabel is derived (read only).
type HealthZone struct {
	Zone           BodyZone `json:"zone"`
	Side           BodySide `json:"side"`
	Since          *string  `json:"since"`
	Note           string   `json:"note"`
	Condition      string   `json:"condition,omitempty"`
	CustomLabel    string   `json:"customLabel,omitempty"`
	ConditionLabel string   `json:"conditionLabel,omitempty"`
}

// ConditionLabelOf is the display label of z's condition: the catalog label, the custom label for
// « other », "" when no (known) condition is set.
func ConditionLabelOf(z HealthZone) string {
	if z.Condition == ConditionOther {
		return z.CustomLabel
	}
	if c, ok := findCondition(z.Zone, z.Condition); ok {
		return c.Label
	}
	return ""
}

var healthSideFR = map[BodySide]string{SideLeft: "gauche", SideRight: "droite", SideBoth: "bilatéral"}

// HealthZoneText is the prompt line of a tracked zone, e.g.
// « Fracture de cheville (malléole) — Cheville, gauche, depuis le 12/10/2026 ».
func HealthZoneText(z HealthZone) string {
	parts := []string{BodyZoneReportLabel(z.Zone)}
	if s := healthSideFR[z.Side]; s != "" {
		parts = append(parts, s)
	}
	if z.Since != nil && *z.Since != "" {
		parts = append(parts, "depuis le "+frfmt.Date(*z.Since))
	}
	out := strings.Join(parts, ", ")
	if label := ConditionLabelOf(z); label != "" {
		out = label + " — " + out
	}
	return out
}

// HealthZonesText joins the tracked zones for the « Suivi santé » prompt line ("" when none).
func HealthZonesText(zones []HealthZone) string {
	parts := make([]string, 0, len(zones))
	for _, z := range zones {
		parts = append(parts, HealthZoneText(z))
	}
	return strings.Join(parts, " ; ")
}
