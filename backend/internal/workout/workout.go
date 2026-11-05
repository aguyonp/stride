// Package workout holds structured workout steps (§11.3): validation, Garmin Connect workout JSON
// and FIT workout file encoding.
package workout

import (
	"fmt"
	"math"
	"strings"
)

var StepTypes = []string{"warmup", "run", "interval", "recovery", "rest", "cooldown", "repeat"}
var TargetKinds = []string{"none", "pace", "hr", "cadence", "power"}

type Target struct {
	Kind string   `json:"kind"`
	Min  *float64 `json:"min,omitempty"`
	Max  *float64 `json:"max,omitempty"`
}

// Step is one workout step; pace in s/km, hr bpm, cadence spm, power W.
type Step struct {
	Type      string   `json:"type"`
	DurationS *float64 `json:"durationS,omitempty"`
	DistanceM *float64 `json:"distanceM,omitempty"`
	Repeat    *int     `json:"repeat,omitempty"`
	Steps     []Step   `json:"steps,omitempty"`
	Target    *Target  `json:"target,omitempty"`
	Note      string   `json:"note,omitempty"`
}

const (
	maxSteps = 60
	maxDepth = 2
)

func oneOf(v string, set []string) bool {
	for _, s := range set {
		if v == s {
			return true
		}
	}
	return false
}

func bad(v *float64, lo, hi float64) bool {
	return v != nil && (math.IsNaN(*v) || *v < lo || *v > hi)
}

// Validate returns French error messages (empty when valid). prefix labels the session.
func Validate(steps []Step, prefix string) []string {
	var errs []string
	count := 0
	var walk func(list []Step, path string, depth int)
	walk = func(list []Step, path string, depth int) {
		for i := range list {
			st := &list[i]
			count++
			label := fmt.Sprintf("%sétape %s%d", prefix, path, i+1)
			if !oneOf(st.Type, StepTypes) {
				errs = append(errs, fmt.Sprintf("%s : type inconnu « %s » (%s)", label, st.Type, strings.Join(StepTypes, ", ")))
				continue
			}
			if st.Type == "repeat" {
				if depth >= maxDepth {
					errs = append(errs, label+" : répétitions imbriquées trop profondes")
					continue
				}
				if st.Repeat == nil || *st.Repeat < 1 || *st.Repeat > 50 {
					errs = append(errs, label+" : « repeat » doit être compris entre 1 et 50")
				}
				if len(st.Steps) == 0 {
					errs = append(errs, label+" : une répétition doit contenir des étapes")
				}
				walk(st.Steps, fmt.Sprintf("%s%d.", path, i+1), depth+1)
				continue
			}
			if len(st.Steps) > 0 {
				errs = append(errs, label+" : seules les étapes « repeat » peuvent contenir des sous-étapes")
			}
			if st.DurationS != nil && st.DistanceM != nil {
				errs = append(errs, label+" : indiquer une durée ou une distance, pas les deux")
			}
			if bad(st.DurationS, 1, 6*3600) {
				errs = append(errs, label+" : durée hors limites [1 s, 6 h]")
			}
			if bad(st.DistanceM, 1, 100000) {
				errs = append(errs, label+" : distance hors limites [1 m, 100 km]")
			}
			if len([]rune(st.Note)) > 200 {
				errs = append(errs, label+" : note trop longue (200 caractères max)")
			}
			if t := st.Target; t != nil {
				lo, hi := 0.0, 0.0
				switch t.Kind {
				case "none":
					continue
				case "pace":
					lo, hi = 120, 1800
				case "hr":
					lo, hi = 40, 230
				case "cadence":
					lo, hi = 60, 250
				case "power":
					lo, hi = 1, 2500
				default:
					errs = append(errs, fmt.Sprintf("%s : cible inconnue « %s » (%s)", label, t.Kind, strings.Join(TargetKinds, ", ")))
					continue
				}
				if t.Min == nil && t.Max == nil {
					errs = append(errs, label+" : cible sans valeur (min/max)")
				}
				if bad(t.Min, lo, hi) || bad(t.Max, lo, hi) {
					errs = append(errs, fmt.Sprintf("%s : cible %s hors limites [%g, %g]", label, t.Kind, lo, hi))
				}
			}
		}
	}
	walk(steps, "", 0)
	if count > maxSteps {
		errs = append(errs, fmt.Sprintf("%strop d'étapes (%d max)", prefix, maxSteps))
	}
	return errs
}

// Normalize orders min/max (pace: min = fastest) and drops empty targets.
func Normalize(steps []Step) []Step {
	out := make([]Step, 0, len(steps))
	for _, st := range steps {
		st.Type = strings.ToLower(strings.TrimSpace(st.Type))
		st.Note = strings.TrimSpace(st.Note)
		if t := st.Target; t != nil {
			t.Kind = strings.ToLower(strings.TrimSpace(t.Kind))
			if t.Kind == "" || t.Kind == "none" {
				st.Target = nil
			} else if t.Min != nil && t.Max != nil && *t.Min > *t.Max {
				t.Min, t.Max = t.Max, t.Min
			}
		}
		if st.Type != "repeat" {
			st.Repeat = nil
		}
		if len(st.Steps) > 0 {
			st.Steps = Normalize(st.Steps)
		}
		out = append(out, st)
	}
	return out
}

// TotalDurationS estimates the duration of the steps (distance steps use paceSecPerKm when known).
func TotalDurationS(steps []Step, paceSecPerKm float64) float64 {
	total := 0.0
	for _, st := range steps {
		switch {
		case st.Type == "repeat" && st.Repeat != nil:
			total += float64(*st.Repeat) * TotalDurationS(st.Steps, paceSecPerKm)
		case st.DurationS != nil:
			total += *st.DurationS
		case st.DistanceM != nil && paceSecPerKm > 0:
			p := paceSecPerKm
			if st.Target != nil && st.Target.Kind == "pace" && st.Target.Min != nil {
				p = *st.Target.Min
			}
			total += *st.DistanceM / 1000 * p
		}
	}
	return total
}

// Structured reports whether the steps describe an interval session (repeat or interval steps).
func Structured(steps []Step) bool {
	for _, st := range steps {
		if st.Type == "repeat" || st.Type == "interval" {
			return true
		}
	}
	return false
}
