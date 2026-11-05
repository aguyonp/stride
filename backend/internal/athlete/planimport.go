package athlete

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"stride/internal/workout"
)

// PlanFormat is the exchange contract imposed on any AI (§11.3).
const PlanFormat = "stride-plan"

var fenceRe = regexp.MustCompile("(?s)```[a-zA-Z]*\\s*\\n(.*?)```")

// extractJSON returns the first fenced block holding JSON, else the outermost raw JSON value.
func extractJSON(text string) string {
	for _, m := range fenceRe.FindAllStringSubmatch(text, -1) {
		if s := strings.TrimSpace(m[1]); strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
			return s
		}
	}
	start := strings.IndexAny(text, "{[")
	if start < 0 {
		return ""
	}
	end := strings.LastIndexAny(text, "}]")
	if end <= start {
		return ""
	}
	return text[start : end+1]
}

var sportAliases = map[string]string{
	"run": "run", "running": "run", "course": "run", "footing": "run", "trail": "run", "cap": "run",
	"bike": "bike", "cycling": "bike", "vélo": "bike", "velo": "bike", "ride": "bike",
	"strength": "strength", "renforcement": "strength", "renfo": "strength", "musculation": "strength",
	"mobility": "mobility", "mobilité": "mobility", "mobilite": "mobility", "étirements": "mobility", "yoga": "mobility",
	"rest": "rest", "repos": "rest", "off": "rest",
	"other": "other", "autre": "other",
}

var stepAliases = map[string]string{
	"warmup": "warmup", "warm-up": "warmup", "warm_up": "warmup", "échauffement": "warmup", "echauffement": "warmup",
	"run": "run", "active": "run", "steady": "run", "footing": "run", "course": "run",
	"interval": "interval", "work": "interval", "effort": "interval", "fractionné": "interval", "fractionne": "interval",
	"recovery": "recovery", "recover": "recovery", "récupération": "recovery", "recuperation": "recovery", "récup": "recovery", "recup": "recovery",
	"rest": "rest", "repos": "rest", "pause": "rest",
	"cooldown": "cooldown", "cool-down": "cooldown", "cool_down": "cooldown", "retour au calme": "cooldown",
	"repeat": "repeat", "repetition": "repeat", "répétition": "repeat", "series": "repeat", "séries": "repeat", "block": "repeat",
}

var targetAliases = map[string]string{
	"none": "none", "open": "none", "pace": "pace", "allure": "pace", "speed": "pace",
	"hr": "hr", "heart_rate": "hr", "heartrate": "hr", "fc": "hr", "bpm": "hr",
	"cadence": "cadence", "spm": "cadence", "power": "power", "puissance": "power", "watts": "power",
}

var clockRe = regexp.MustCompile(`^(\d+):(\d{1,2})(?::(\d{1,2}))?$`)
var numUnitRe = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)\s*([a-zé/"']*)$`)

// number parses JSON numbers and tolerant strings ("5:30", "10 min", "1.5 km", "90s"); kind is
// "duration", "distance" or "" and drives unit conversion.
func number(v any, kind string) (*float64, bool) {
	switch x := v.(type) {
	case nil:
		return nil, true
	case float64:
		return &x, true
	case string:
		s := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(x, ",", ".")))
		s = strings.TrimSuffix(strings.TrimSuffix(s, "/km"), " ")
		if s == "" {
			return nil, true
		}
		if m := clockRe.FindStringSubmatch(s); m != nil {
			a, _ := strconv.ParseFloat(m[1], 64)
			b, _ := strconv.ParseFloat(m[2], 64)
			if m[3] != "" {
				c, _ := strconv.ParseFloat(m[3], 64)
				r := a*3600 + b*60 + c
				return &r, true
			}
			r := a*60 + b
			return &r, true
		}
		m := numUnitRe.FindStringSubmatch(s)
		if m == nil {
			return nil, false
		}
		f, _ := strconv.ParseFloat(m[1], 64)
		switch m[2] {
		case "min", "mn", "'", "minutes", "minute":
			f *= 60
		case "h", "heure", "heures":
			f *= 3600
		case "km":
			f *= 1000
		case "", "s", "sec", "secondes", "m", "bpm", "spm", "w", "ppm", "\"":
		default:
			return nil, false
		}
		return &f, true
	}
	return nil, false
}

func field(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

func strField(m map[string]any, keys ...string) string {
	v, _ := field(m, keys...)
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

type parser struct{ errs []string }

func (p *parser) num(m map[string]any, label, kind string, keys ...string) *float64 {
	v, ok := field(m, keys...)
	if !ok {
		return nil
	}
	f, ok := number(v, kind)
	if !ok {
		p.errs = append(p.errs, fmt.Sprintf("%s : valeur illisible pour « %s »", label, keys[0]))
	}
	return f
}

func (p *parser) target(v any, label string) *workout.Target {
	m, ok := v.(map[string]any)
	if !ok {
		if v != nil {
			p.errs = append(p.errs, label+" : cible illisible (objet {kind, min, max} attendu)")
		}
		return nil
	}
	kind := strings.ToLower(strField(m, "kind", "type"))
	if k, ok := targetAliases[kind]; ok {
		kind = k
	}
	t := &workout.Target{Kind: kind, Min: p.num(m, label, "", "min", "low", "from"), Max: p.num(m, label, "", "max", "high", "to")}
	if t.Min == nil && t.Max == nil {
		t.Min = p.num(m, label, "", "value")
		t.Max = t.Min
	}
	return t
}

func (p *parser) steps(v any, label string, depth int) []workout.Step {
	list, ok := v.([]any)
	if !ok {
		if v != nil {
			p.errs = append(p.errs, label+" : « steps » doit être une liste")
		}
		return nil
	}
	out := make([]workout.Step, 0, len(list))
	for i, it := range list {
		m, ok := it.(map[string]any)
		sl := fmt.Sprintf("%s étape %d", label, i+1)
		if !ok {
			p.errs = append(p.errs, sl+" : objet attendu")
			continue
		}
		typ := strings.ToLower(strField(m, "type", "kind", "stepType"))
		if a, ok := stepAliases[typ]; ok {
			typ = a
		}
		st := workout.Step{Type: typ, Note: strField(m, "note", "notes", "description")}
		st.DurationS = p.num(m, sl, "duration", "durationS", "duration", "time", "durationSec")
		st.DistanceM = p.num(m, sl, "distance", "distanceM", "distance")
		if km := p.num(m, sl, "", "distanceKm"); km != nil && st.DistanceM == nil {
			v := *km * 1000
			st.DistanceM = &v
		}
		if r := p.num(m, sl, "", "repeat", "repetitions", "reps", "times", "iterations"); r != nil {
			n := int(math.Round(*r))
			st.Repeat = &n
			if typ == "" {
				st.Type = "repeat"
			}
		}
		if sub, ok := m["steps"]; ok && depth < 3 {
			st.Steps = p.steps(sub, sl, depth+1)
			if st.Type == "" {
				st.Type = "repeat"
			}
		}
		if t, ok := field(m, "target"); ok {
			st.Target = p.target(t, sl)
		}
		out = append(out, st)
	}
	return out
}

func normDate(s string) string {
	if ValidDate(s) {
		return s
	}
	for _, layout := range []string{"02/01/2006", "2/1/2006", "2006/01/02", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return s
}

// ParsePlan reads a stride-plan answer (first ```json block or raw JSON) tolerantly. Only valid
// sessions are returned as drafts; errors are French one-liners.
func ParsePlan(text, source string) ([]PlannedSession, []string) {
	raw := extractJSON(text)
	if raw == "" {
		return []PlannedSession{}, []string{"Aucun JSON trouvé : un bloc ```json au format « stride-plan » est attendu."}
	}
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return []PlannedSession{}, []string{"JSON illisible : " + err.Error()}
	}
	var sessions []any
	var errs []string
	switch d := doc.(type) {
	case []any:
		sessions = d
	case map[string]any:
		if f, ok := d["format"].(string); ok && f != PlanFormat {
			errs = append(errs, fmt.Sprintf("Format « %s » inattendu (« %s » attendu) : lecture tolérante.", f, PlanFormat))
		}
		if s, ok := field(d, "sessions", "plan", "workouts"); ok {
			sessions, _ = s.([]any)
		} else if _, ok := d["date"]; ok {
			sessions = []any{d}
		}
	}
	if len(sessions) == 0 {
		return []PlannedSession{}, append(errs, "Aucune séance trouvée (liste « sessions » vide ou absente).")
	}
	drafts := []PlannedSession{}
	for i, it := range sessions {
		label := fmt.Sprintf("Séance %d", i+1)
		m, ok := it.(map[string]any)
		if !ok {
			errs = append(errs, label+" : objet attendu")
			continue
		}
		p := &parser{}
		sport := strings.ToLower(strField(m, "sport", "type"))
		if a, ok := sportAliases[sport]; ok {
			sport = a
		} else if sport == "" {
			sport = "run"
		}
		ps := PlannedSession{Date: normDate(strField(m, "date", "day")), Sport: sport, Title: strField(m, "title", "name", "titre"),
			Instructions: strField(m, "instructions", "description", "notes"), Source: source}
		ps.TargetDurationS = p.num(m, label, "duration", "targetDurationS", "durationS", "duration")
		ps.TargetDistanceM = p.num(m, label, "distance", "targetDistanceM", "distanceM", "distance")
		if km := p.num(m, label, "", "targetDistanceKm", "distanceKm"); km != nil && ps.TargetDistanceM == nil {
			v := *km * 1000
			ps.TargetDistanceM = &v
		}
		ps.TargetPaceMinSecPerKm = p.num(m, label, "", "targetPaceMinSecPerKm", "paceMin")
		ps.TargetPaceMaxSecPerKm = p.num(m, label, "", "targetPaceMaxSecPerKm", "paceMax")
		ps.TargetCadenceSpm = p.num(m, label, "", "targetCadenceSpm", "cadence", "cadenceSpm")
		if s, ok := field(m, "steps", "workout"); ok {
			ps.Steps = p.steps(s, label+",", 0)
		}
		if ps.TargetPaceMinSecPerKm != nil && ps.TargetPaceMaxSecPerKm != nil && *ps.TargetPaceMinSecPerKm > *ps.TargetPaceMaxSecPerKm {
			ps.TargetPaceMinSecPerKm, ps.TargetPaceMaxSecPerKm = ps.TargetPaceMaxSecPerKm, ps.TargetPaceMinSecPerKm
		}
		if ps.TargetDurationS == nil && len(ps.Steps) > 0 {
			if d := workout.TotalDurationS(workout.Normalize(ps.Steps), 0); d > 0 {
				ps.TargetDurationS = &d
			}
		}
		if ps.Steps == nil {
			ps.Steps = []workout.Step{}
		}
		if len(p.errs) > 0 {
			errs = append(errs, p.errs...)
			continue
		}
		if err := ps.Normalize(); err != nil {
			for _, line := range strings.Split(err.Error(), "\n") {
				errs = append(errs, label+" : "+line)
			}
			continue
		}
		drafts = append(drafts, ps)
	}
	if errs == nil {
		errs = []string{}
	}
	return drafts, errs
}
