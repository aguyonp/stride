// Package analysis provides biomechanical, physiological, and anatomical stress analysis.
package analysis

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"stride/internal/frfmt"
)

// BodyZone identifies one of the 6 canonical anatomical zones monitored by Stride.
type BodyZone string

const (
	ZoneKneePFPS         BodyZone = "knee_pfps"
	ZoneAchilles         BodyZone = "achilles"
	ZonePlantarFascia    BodyZone = "plantar_fascia"
	ZoneShinSplints      BodyZone = "shin_splints"
	ZoneIliotibialBand   BodyZone = "iliotibial_band"
	ZoneHamstringsCalves BodyZone = "hamstrings_calves"
)

// Canonical list of all 6 body zones.
var AllBodyZones = []BodyZone{
	ZoneKneePFPS,
	ZoneShinSplints,
	ZoneIliotibialBand,
	ZoneAchilles,
	ZonePlantarFascia,
	ZoneHamstringsCalves,
}

// BodySide defines anatomical laterality.
type BodySide string

const (
	SideLeft  BodySide = "left"
	SideRight BodySide = "right"
	SideBoth  BodySide = "both"
)

// BodyZoneLabel returns the UI presentation label for a zone.
func BodyZoneLabel(z BodyZone) string {
	switch z {
	case ZoneKneePFPS:
		return "Rotule / Genou (SFP)"
	case ZoneShinSplints:
		return "Périoste tibial (Périostite)"
	case ZoneIliotibialBand:
		return "Bandelette ilio-tibiale (TFL)"
	case ZoneAchilles:
		return "Tendon d’Achille"
	case ZonePlantarFascia:
		return "Aponévrose plantaire"
	case ZoneHamstringsCalves:
		return "Ischio-jambiers / Mollets"
	default:
		return string(z)
	}
}

// BodyZoneReportLabel returns the label for AI markdown prompts (strictly without "SFP").
func BodyZoneReportLabel(z BodyZone) string {
	if z == ZoneKneePFPS {
		return "Rotule / Genou (fémoro-patellaire)"
	}
	return BodyZoneLabel(z)
}

// BodyStressEntry represents a recorded pain or tightness observation.
type BodyStressEntry struct {
	ID               int64    `json:"id"`
	Date             string   `json:"date"`
	ActivityID       *int64   `json:"activityId"`
	ActivityName     *string  `json:"activityName"`
	Zone             BodyZone `json:"zone"`
	ZoneLabel        string   `json:"zoneLabel"`
	Side             BodySide `json:"side"`
	PainScore        float64  `json:"painScore"`
	ColdNextDayScore *float64 `json:"coldNextDayScore"`
	Note             string   `json:"note"`
	CreatedAt        string   `json:"createdAt"`
	UpdatedAt        string   `json:"updatedAt"`
}

// BodyStressInput is the input payload for creating or patching a pain observation.
type BodyStressInput struct {
	Date             string    `json:"date"`
	ActivityID       *int64    `json:"activityId,omitempty"`
	Zone             BodyZone  `json:"zone"`
	Side             *BodySide `json:"side,omitempty"`
	PainScore        float64   `json:"painScore"`
	ColdNextDayScore *float64  `json:"coldNextDayScore,omitempty"`
	Note             *string   `json:"note,omitempty"`
}

// BodyZoneSummary summarizes 30d/90d pain trajectory for a canonical anatomical zone.
type BodyZoneSummary struct {
	Zone                   BodyZone  `json:"zone"`
	ZoneLabel              string    `json:"zoneLabel"`
	LatestScore            *float64  `json:"latestScore"`
	LatestColdNextDayScore *float64  `json:"latestColdNextDayScore"`
	LatestSide             *BodySide `json:"latestSide"`
	LatestDate             *string   `json:"latestDate"`
	MaxScore30d            *float64  `json:"maxScore30d"`
	EntriesCount90d        int       `json:"entriesCount90d"`
	Trend                  string    `json:"trend"` // improving | stable | worsening | none
}

// BodyShoeHighlight identifies a shoe involved in pain or recovery episodes.
type BodyShoeHighlight struct {
	ShoeID       int64    `json:"shoeId"`
	ShoeName     string   `json:"shoeName"`
	DropMm       *float64 `json:"dropMm"`
	DistanceKm   float64  `json:"distanceKm"`
	PainEpisodes int      `json:"painEpisodes"`
}

// BodyCorrelationFactor compares a factor between the runs linked to a zone's pain entries
// (symptomatic) and the other runs of the window (baseline). Means, delta and summary are null
// when either sample is too small (InsufficientData).
type BodyCorrelationFactor struct {
	Zone             BodyZone           `json:"zone"`
	ZoneLabel        string             `json:"zoneLabel"`
	Factor           string             `json:"factor"` // elev_loss | shoe_wear | shoe_drop | gct
	FactorLabel      string             `json:"factorLabel"`
	Unit             string             `json:"unit"`
	SymptomaticMean  *float64           `json:"symptomaticMean"`
	BaselineMean     *float64           `json:"baselineMean"`
	DeltaPct         *float64           `json:"deltaPct"`
	SymptomaticRuns  int                `json:"symptomaticRuns"`
	BaselineRuns     int                `json:"baselineRuns"`
	Confidence       string             `json:"confidence"` // insufficient | low | moderate | high
	InsufficientData bool               `json:"insufficientData"`
	ShoeHighlight    *BodyShoeHighlight `json:"shoeHighlight"`
	Summary          *string            `json:"summary"`
}

// BodyMapOverview encapsulates the complete body stress map response.
type BodyMapOverview struct {
	Zones        []BodyZoneSummary       `json:"zones"`
	Entries      []BodyStressEntry       `json:"entries"`
	Correlations []BodyCorrelationFactor `json:"correlations"`
	WindowDays   int                     `json:"windowDays"`
}

// ShoeBioSummary carries the minimal shoe data needed for correlation computation.
type ShoeBioSummary struct {
	ID         int64
	Name       string
	DropMm     *float64
	DistanceKm float64 // current total, all activities included
}

// ActivityBioSummary carries the minimal activity data needed for correlation computation.
type ActivityBioSummary struct {
	ID         int64
	Date       string
	IsRunning  bool
	DistanceKm *float64
	ElevLossM  *float64
	GctMs      *float64
	ShoeID     *int64
}

// Sample sizes (runs with a value) for a correlation: below the minimum it is not computed.
const (
	corrMinSymptomatic, corrMinBaseline           = 2, 3
	corrModerateSymptomatic, corrModerateBaseline = 3, 5
	corrHighSymptomatic, corrHighBaseline         = 5, 10
)

func corrConfidence(sym, base int) string {
	switch {
	case sym < corrMinSymptomatic || base < corrMinBaseline:
		return "insufficient"
	case sym < corrModerateSymptomatic || base < corrModerateBaseline:
		return "low"
	case sym < corrHighSymptomatic || base < corrHighBaseline:
		return "moderate"
	}
	return "high"
}

type corrFactor struct {
	key, label, unit string
	digits           int
	value            func(ActivityBioSummary) *float64
}

// ComputeCorrelations compares, for each zone with pain entries, the runs linked to an entry
// (same activity or same day) with the other runs of acts. Only measured values are used.
func ComputeCorrelations(activeZones []BodyZoneSummary, entries []BodyStressEntry, shoes []ShoeBioSummary, acts []ActivityBioSummary) []BodyCorrelationFactor {
	shoeByID := make(map[int64]ShoeBioSummary, len(shoes))
	for _, sh := range shoes {
		shoeByID[sh.ID] = sh
	}
	wear := shoeWearAtActivity(shoes, acts)
	drop := func(a ActivityBioSummary) *float64 {
		if a.ShoeID == nil {
			return nil
		}
		return shoeByID[*a.ShoeID].DropMm
	}
	positive := func(get func(ActivityBioSummary) *float64) func(ActivityBioSummary) *float64 {
		return func(a ActivityBioSummary) *float64 {
			if v := get(a); v != nil && *v > 0 {
				return v
			}
			return nil
		}
	}
	elevLoss := corrFactor{"elev_loss", "Dénivelé négatif (D−)", "m", 0, func(a ActivityBioSummary) *float64 {
		if a.ElevLossM != nil && *a.ElevLossM >= 0 {
			return a.ElevLossM
		}
		return nil
	}}
	shoeWear := corrFactor{"shoe_wear", "Usure chaussure", "km", 0, positive(func(a ActivityBioSummary) *float64 { return wear[a.ID] })}
	shoeDrop := corrFactor{"shoe_drop", "Drop chaussure", "mm", 1, positive(drop)}
	gct := corrFactor{"gct", "Temps de contact au sol (GCT)", "ms", 0, positive(func(a ActivityBioSummary) *float64 { return a.GctMs })}

	var runs []ActivityBioSummary
	for _, a := range acts {
		if a.IsRunning {
			runs = append(runs, a)
		}
	}
	corrs := []BodyCorrelationFactor{}
	for _, z := range activeZones {
		if z.EntriesCount90d <= 0 {
			continue
		}
		days, ids := map[string]bool{}, map[int64]bool{}
		for _, e := range entries {
			if e.Zone == z.Zone {
				days[e.Date] = true
				if e.ActivityID != nil {
					ids[*e.ActivityID] = true
				}
			}
		}
		var sym, base []ActivityBioSummary
		for _, a := range runs {
			if ids[a.ID] || days[a.Date] {
				sym = append(sym, a)
			} else {
				base = append(base, a)
			}
		}
		var factors []corrFactor
		switch z.Zone {
		case ZoneKneePFPS:
			factors = []corrFactor{elevLoss, shoeWear}
		case ZoneAchilles, ZonePlantarFascia:
			factors = []corrFactor{shoeDrop}
		default:
			factors = []corrFactor{gct}
		}
		for _, f := range factors {
			c := compareFactor(z, f, sym, base)
			if f.key == "shoe_wear" || f.key == "shoe_drop" {
				c.ShoeHighlight = mostWornShoe(sym, shoeByID)
			}
			corrs = append(corrs, c)
		}
	}
	return corrs
}

func compareFactor(z BodyZoneSummary, f corrFactor, sym, base []ActivityBioSummary) BodyCorrelationFactor {
	sMean, sN := meanOf(sym, f.value)
	bMean, bN := meanOf(base, f.value)
	c := BodyCorrelationFactor{Zone: z.Zone, ZoneLabel: z.ZoneLabel, Factor: f.key, FactorLabel: f.label, Unit: f.unit,
		SymptomaticRuns: sN, BaselineRuns: bN, Confidence: corrConfidence(sN, bN)}
	if c.Confidence == "insufficient" {
		c.InsufficientData = true
		return c
	}
	c.SymptomaticMean, c.BaselineMean = Num(sMean, f.digits), Num(bMean, f.digits)
	if bMean > 0 {
		c.DeltaPct = Num((sMean-bMean)/bMean*100, 1)
	}
	summary := fmt.Sprintf("%s : %s en moyenne sur les %s liées à la gêne, contre %s sur les %s.",
		f.label, frfmt.UnitDec(sMean, f.digits, f.unit), plural(sN, "sortie"), frfmt.UnitDec(bMean, f.digits, f.unit), plural(bN, "autre sortie"))
	if c.DeltaPct != nil {
		summary = strings.TrimSuffix(summary, ".") + fmt.Sprintf(" (%s %%).", frfmt.Signed(*c.DeltaPct, 0))
	}
	c.Summary = &summary
	return c
}

func plural(n int, word string) string {
	if n > 1 {
		word = strings.ReplaceAll(word, " ", "s ") + "s"
	}
	return fmt.Sprintf("%d %s", n, word)
}

func meanOf(acts []ActivityBioSummary, value func(ActivityBioSummary) *float64) (float64, int) {
	sum, n := 0.0, 0
	for _, a := range acts {
		if v := value(a); v != nil {
			sum += *v
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	return sum / float64(n), n
}

// shoeWearAtActivity returns each activity's shoe mileage at that activity: the shoe's current
// total minus the distance of its later activities (acts must reach today).
func shoeWearAtActivity(shoes []ShoeBioSummary, acts []ActivityBioSummary) map[int64]*float64 {
	total := make(map[int64]float64, len(shoes))
	for _, sh := range shoes {
		total[sh.ID] = sh.DistanceKm
	}
	sorted := append([]ActivityBioSummary(nil), acts...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Date != sorted[j].Date {
			return sorted[i].Date > sorted[j].Date
		}
		return sorted[i].ID > sorted[j].ID
	})
	out := map[int64]*float64{}
	later := map[int64]float64{}
	for _, a := range sorted {
		if a.ShoeID == nil {
			continue
		}
		t, ok := total[*a.ShoeID]
		if !ok {
			continue
		}
		w := math.Max(0, t-later[*a.ShoeID])
		out[a.ID] = &w
		if a.DistanceKm != nil {
			later[*a.ShoeID] += *a.DistanceKm
		}
	}
	return out
}

// mostWornShoe returns the shoe used on most symptomatic runs (nil when none is known).
func mostWornShoe(sym []ActivityBioSummary, shoes map[int64]ShoeBioSummary) *BodyShoeHighlight {
	counts := map[int64]int{}
	var best int64
	for _, a := range sym {
		if a.ShoeID == nil {
			continue
		}
		if _, ok := shoes[*a.ShoeID]; !ok {
			continue
		}
		counts[*a.ShoeID]++
		if c := counts[*a.ShoeID]; c > counts[best] || (c == counts[best] && *a.ShoeID < best) {
			best = *a.ShoeID
		}
	}
	if counts[best] == 0 {
		return nil
	}
	sh := shoes[best]
	return &BodyShoeHighlight{ShoeID: sh.ID, ShoeName: sh.Name, DropMm: sh.DropMm, DistanceKm: sh.DistanceKm, PainEpisodes: counts[best]}
}
