package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"stride/internal/apimeter"
	"stride/internal/athlete"
	"stride/internal/frfmt"
	"stride/internal/provider/weather"
	"stride/internal/queue"
	"stride/internal/report"
	"stride/internal/sse"
	"stride/internal/workout"
)

// Smart Workout Morphing (§15.5): today's or tomorrow's planned session is adapted to the day's
// constraints by deterministic rules (active physio/coach restrictions, recent pain, acute load,
// forecast heat and wind), optionally refined by the athlete's LLM, then applied with its original
// targets kept in `morph` (revertible). The FIT export always encodes the current steps.

const (
	morphPainMin     = 3.0
	morphACWRMax     = 1.3
	morphHeatC       = 22.0
	morphWindKph     = 30.0
	morphPainWindow  = 48 * time.Hour
	morphForecastTTL = time.Hour
	morphDefaultHour = 18
	morphNoteRunes   = 600
)

type MorphConstraint struct {
	Kind     string `json:"kind"` // directive | pain | load | heat | wind
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	Severity int    `json:"severity"` // 1 | 2 | 3
}

type MorphChange struct {
	Field     string `json:"field"`
	StepIndex *int   `json:"stepIndex"`
	Before    string `json:"before"`
	After     string `json:"after"`
	Reason    string `json:"reason"`
}

// MorphPreview is the before/after of an adaptation; Proposal is nil without constraint.
type MorphPreview struct {
	SessionID   int64                   `json:"sessionId"`
	Constraints []MorphConstraint       `json:"constraints"`
	Original    athlete.PlannedSession  `json:"original"`
	Proposal    *athlete.PlannedSession `json:"proposal"`
	Changes     []MorphChange           `json:"changes"`
}

type MorphRefineInput struct {
	Note string `json:"note"`
}

type MorphApplyInput struct {
	Proposal json.RawMessage `json:"proposal"`
}

type llmMorphRefineJobPayload struct {
	SessionID int64        `json:"sessionId"`
	Prompt    string       `json:"prompt"`
	Preview   MorphPreview `json:"preview"`
}

// morphFields are the only fields an applied proposal may change (date, sport, links stay).
var morphFields = []string{"title", "targetDurationS", "targetDistanceM", "targetPaceMinSecPerKm", "targetPaceMaxSecPerKm",
	"targetCadenceSpm", "instructions", "steps"}

var morphIgnored = []string{"id", "date", "sport", "source", "match", "garmin", "prescribedBy", "morph"}

// morphFacts are the measured inputs of the rules.
type morphFacts struct {
	directives []athlete.ShareNote
	painMax    *float64
	acwr       *float64
	tempC      *float64
	windKph    *float64
}

// ---------- French texts (kept together for the i18n pass) ----------

var morphRoleLabels = map[string]string{"physio": "kiné", "coach": "coach", "other": "intervenant"}

func morphDirectiveLabel(n athlete.ShareNote) string {
	return fmt.Sprintf("Consigne %s (%s)", morphRoleLabels[n.AuthorRole], n.LinkLabel)
}

func morphPainText(v float64) (string, string) {
	return "Douleur récente", fmt.Sprintf("Jusqu'à %s/10 pendant ou au lendemain d'une séance (48 dernières heures)", frfmt.Num(v, 1))
}

func morphLoadText(v float64) (string, string) {
	return "Charge aiguë élevée", fmt.Sprintf("Ratio aigu/chronique %s (> %s)", frfmt.Dec(v, 2), frfmt.Dec(morphACWRMax, 1))
}

func morphHeatText(v float64) (string, string) {
	return "Chaleur annoncée", fmt.Sprintf("%s °C prévus à l'heure de la séance", frfmt.Num(v, 0))
}

func morphWindText(v float64) (string, string) {
	return "Vent fort annoncé", fmt.Sprintf("%s km/h prévus à l'heure de la séance", frfmt.Num(v, 0))
}

const (
	morphReasonNoIntervals = "Consigne kiné / coach : pas de fractionné"
	morphReasonEasyOnly    = "Consigne kiné / coach : intensité facile uniquement"
	morphReasonPain        = "Douleur ≥ 3/10 sur les 48 dernières heures"
	morphReasonLoad        = "Charge aiguë élevée (ratio > 1,3)"
	morphReasonHeat        = "Chaleur : allure ralentie"
	morphReasonWind        = "Vent fort : courir à l'effort ou à la FC plutôt qu'à l'allure"
	morphReasonCadence     = "Consigne kiné / coach : cadence cible"
	morphReasonElev        = "Consigne kiné / coach : dénivelé négatif limité"
	morphReasonLLM         = "Affinage par l'IA"
	morphNoteEasy          = "Endurance facile, aisance respiratoire"
	morphNoteWind          = "Vent : allure indicative, courir à l'effort"
	morphTitleEasy         = "Footing facile"
	morphFree              = "libre"
	morphManualReason      = "Adaptation validée par l'athlète"
)

// ---------- constraints & rules ----------

func severity(v float64, thresholds ...float64) int {
	n := 1
	for _, t := range thresholds {
		if v >= t {
			n++
		}
	}
	return min(n, 3)
}

func morphConstraints(f morphFacts) []MorphConstraint {
	out := []MorphConstraint{}
	for _, d := range f.directives {
		r := d.Restrictions
		if r == nil || r.Empty() {
			continue
		}
		sev := 2
		if r.NoIntervals || (r.MaxIntensity != nil && *r.MaxIntensity == "easy") {
			sev = 3
		}
		out = append(out, MorphConstraint{Kind: "directive", Label: morphDirectiveLabel(d), Detail: report.RestrictionsText(r), Severity: sev})
	}
	if v := f.painMax; v != nil && *v >= morphPainMin {
		l, d := morphPainText(*v)
		out = append(out, MorphConstraint{Kind: "pain", Label: l, Detail: d, Severity: severity(*v, 4, 6)})
	}
	if v := f.acwr; v != nil && *v > morphACWRMax {
		l, d := morphLoadText(*v)
		out = append(out, MorphConstraint{Kind: "load", Label: l, Detail: d, Severity: severity(*v, 1.4, 1.5)})
	}
	if v := f.tempC; v != nil && *v > morphHeatC {
		l, d := morphHeatText(*v)
		out = append(out, MorphConstraint{Kind: "heat", Label: l, Detail: d, Severity: severity(*v, 26, 30)})
	}
	if v := f.windKph; v != nil && *v > morphWindKph {
		l, d := morphWindText(*v)
		out = append(out, MorphConstraint{Kind: "wind", Label: l, Detail: d, Severity: severity(*v, 40, 50)})
	}
	return out
}

// mergedRestrictions combines the restrictions of every active directive (the strictest wins).
func mergedRestrictions(ds []athlete.ShareNote) athlete.DirectiveRestrictions {
	var m athlete.DirectiveRestrictions
	for _, d := range ds {
		r := d.Restrictions
		if r == nil {
			continue
		}
		m.NoIntervals = m.NoIntervals || r.NoIntervals
		if r.MaxIntensity != nil && (m.MaxIntensity == nil || *r.MaxIntensity == "easy") {
			m.MaxIntensity = r.MaxIntensity
		}
		if r.MaxElevLossM != nil && (m.MaxElevLossM == nil || *r.MaxElevLossM < *m.MaxElevLossM) {
			m.MaxElevLossM = r.MaxElevLossM
		}
		if r.CadenceTargetSpm != nil && m.CadenceTargetSpm == nil {
			m.CadenceTargetSpm = r.CadenceTargetSpm
		}
	}
	return m
}

func cloneSession(ps athlete.PlannedSession) athlete.PlannedSession {
	b, _ := json.Marshal(ps)
	var out athlete.PlannedSession
	_ = json.Unmarshal(b, &out)
	if out.Steps == nil {
		out.Steps = []workout.Step{}
	}
	return out
}

func durText(v *float64) string {
	if v == nil {
		return morphFree
	}
	return frfmt.Duration(*v)
}

func distText(v *float64) string {
	if v == nil {
		return morphFree
	}
	return frfmt.Km(*v, 2) + " km"
}

func paceText(lo, hi *float64) string {
	switch {
	case lo != nil && hi != nil:
		return frfmt.Pace(*lo) + "–" + frfmt.Pace(*hi)
	case lo != nil:
		return frfmt.Pace(*lo)
	case hi != nil:
		return frfmt.Pace(*hi)
	}
	return morphFree
}

func cadText(v *float64) string {
	if v == nil {
		return morphFree
	}
	return frfmt.Int(*v) + " pas/min"
}

func stepsText(steps []workout.Step) string {
	if t := report.StepsText(steps); t != "" {
		return t
	}
	return morphFree
}

func noteWith(note, add string) string {
	if strings.Contains(note, add) {
		return note
	}
	n := strings.TrimSpace(strings.TrimSpace(note) + " · " + add)
	n = strings.TrimPrefix(n, "· ")
	if r := []rune(n); len(r) > 200 {
		n = string(r[:200])
	}
	return n
}

func roundTo(v, step float64) float64 { return math.Round(v/step) * step }

// heatSlowdown is the pace slowdown (s/km) for a forecast temperature: 5 s/km per 2 °C above 22 °C,
// at least 5, at most 30.
func heatSlowdown(tempC float64) float64 {
	return math.Max(5, math.Min(30, roundTo((tempC-morphHeatC)*2.5, 5)))
}

// pacedStep reports whether a step carries a pace target.
func pacedStep(st workout.Step) bool {
	if st.Target != nil && st.Target.Kind == "pace" {
		return true
	}
	for _, c := range st.Steps {
		if pacedStep(c) {
			return true
		}
	}
	return false
}

// mapSteps applies fn to every non-repeat step (repeats are walked).
func mapSteps(steps []workout.Step, fn func(*workout.Step)) {
	for i := range steps {
		if steps[i].Type == "repeat" {
			mapSteps(steps[i].Steps, fn)
			continue
		}
		fn(&steps[i])
	}
}

// morphRules applies the deterministic adaptation to base; nil without constraint.
func morphRules(base athlete.PlannedSession, f morphFacts, cs []MorphConstraint) (*athlete.PlannedSession, []MorphChange) {
	changes := []MorphChange{}
	if len(cs) == 0 {
		return nil, changes
	}
	p := cloneSession(base)
	p.Match, p.Morph = nil, nil
	sev := map[string]int{}
	for _, c := range cs {
		sev[c.Kind] = max(sev[c.Kind], c.Severity)
	}
	add := func(field string, idx *int, before, after, reason string) {
		if before != after {
			changes = append(changes, MorphChange{Field: field, StepIndex: idx, Before: before, After: after, Reason: reason})
		}
	}
	r := mergedRestrictions(f.directives)
	var reasons []string

	// 1. Intervals become easy continuous running (no-interval or easy-only directive, recent pain).
	easyReason := ""
	switch {
	case r.NoIntervals:
		easyReason = morphReasonNoIntervals
	case r.MaxIntensity != nil && *r.MaxIntensity == "easy":
		easyReason = morphReasonEasyOnly
	case sev["pain"] > 0:
		easyReason = morphReasonPain
	}
	if easyReason != "" && workout.Structured(p.Steps) {
		// The planned duration is the volume to keep; the steps total is the fallback.
		total := workout.TotalDurationS(p.Steps, 360)
		if p.TargetDurationS != nil {
			total = *p.TargetDurationS
		}
		total = math.Max(600, roundTo(total, 60))
		before := stepsText(p.Steps)
		p.Steps = []workout.Step{{Type: "run", DurationS: &total, Note: morphNoteEasy}}
		add("steps", nil, before, stepsText(p.Steps), easyReason)
		if p.TargetPaceMinSecPerKm != nil || p.TargetPaceMaxSecPerKm != nil {
			add("targetPace", nil, paceText(p.TargetPaceMinSecPerKm, p.TargetPaceMaxSecPerKm), morphFree, easyReason)
			p.TargetPaceMinSecPerKm, p.TargetPaceMaxSecPerKm = nil, nil
		}
		if p.Sport == "run" {
			add("title", nil, p.Title, morphTitleEasy, easyReason)
			p.Title = morphTitleEasy
		}
		reasons = append(reasons, easyReason)
	}

	// 2. Volume −15/−20/−30 % on acute load or pain (worst severity).
	if vs := max(sev["load"], sev["pain"]); vs > 0 {
		cut := []float64{0, 0.15, 0.20, 0.30}[vs]
		k := 1 - cut
		reason := morphReasonLoad
		if sev["pain"] >= sev["load"] {
			reason = morphReasonPain
		}
		reason += fmt.Sprintf(" (−%d %%)", int(cut*100+0.5))
		if p.TargetDurationS != nil {
			v := roundTo(*p.TargetDurationS*k, 60)
			add("targetDurationS", nil, durText(p.TargetDurationS), durText(&v), reason)
			p.TargetDurationS = &v
		}
		if p.TargetDistanceM != nil {
			v := roundTo(*p.TargetDistanceM*k, 100)
			add("targetDistanceM", nil, distText(p.TargetDistanceM), distText(&v), reason)
			p.TargetDistanceM = &v
		}
		if len(p.Steps) > 0 {
			before := stepsText(p.Steps)
			p.Steps = scaleSteps(p.Steps, k)
			add("steps", nil, before, stepsText(p.Steps), reason)
		}
		reasons = append(reasons, reason)
	}

	// 3. Heat slows the target paces.
	if sev["heat"] > 0 && f.tempC != nil {
		d := heatSlowdown(*f.tempC)
		reason := fmt.Sprintf("%s (+%s s/km)", morphReasonHeat, frfmt.Int(d))
		slow := func(v *float64) *float64 {
			if v == nil {
				return nil
			}
			return f64(math.Min(1800, *v+d))
		}
		if p.TargetPaceMinSecPerKm != nil || p.TargetPaceMaxSecPerKm != nil {
			before := paceText(p.TargetPaceMinSecPerKm, p.TargetPaceMaxSecPerKm)
			p.TargetPaceMinSecPerKm, p.TargetPaceMaxSecPerKm = slow(p.TargetPaceMinSecPerKm), slow(p.TargetPaceMaxSecPerKm)
			add("targetPace", nil, before, paceText(p.TargetPaceMinSecPerKm, p.TargetPaceMaxSecPerKm), reason)
		}
		for i := range p.Steps {
			if !pacedStep(p.Steps[i]) {
				continue
			}
			before := stepsText(p.Steps[i : i+1])
			mapSteps(p.Steps[i:i+1], func(st *workout.Step) {
				if st.Target != nil && st.Target.Kind == "pace" {
					st.Target.Min, st.Target.Max = slow(st.Target.Min), slow(st.Target.Max)
				}
			})
			idx := i
			add("steps", &idx, before, stepsText(p.Steps[i:i+1]), reason)
		}
		reasons = append(reasons, reason)
	}

	// 4. Wind: pace targets become effort / heart-rate cues.
	if sev["wind"] > 0 {
		for i := range p.Steps {
			if !pacedStep(p.Steps[i]) {
				continue
			}
			before := p.Steps[i].Note
			mapSteps(p.Steps[i:i+1], func(st *workout.Step) {
				if st.Target != nil && st.Target.Kind == "pace" {
					st.Note = noteWith(st.Note, morphNoteWind)
				}
			})
			if p.Steps[i].Type == "repeat" {
				p.Steps[i].Note = noteWith(p.Steps[i].Note, morphNoteWind)
			}
			idx := i
			add("steps.note", &idx, before, p.Steps[i].Note, morphReasonWind)
		}
		reasons = append(reasons, morphReasonWind)
	}

	// 5. The physio/coach cadence goes into the session and its running steps.
	if c := r.CadenceTargetSpm; c != nil {
		add("targetCadenceSpm", nil, cadText(p.TargetCadenceSpm), cadText(c), morphReasonCadence)
		p.TargetCadenceSpm = f64(*c)
		cue := fmt.Sprintf("Cadence cible %s pas/min", frfmt.Int(*c))
		for i := range p.Steps {
			before := stepsText(p.Steps[i:i+1]) + " " + p.Steps[i].Note
			mapSteps(p.Steps[i:i+1], func(st *workout.Step) {
				if st.Type == "rest" || st.Type == "recovery" {
					return
				}
				if st.Target == nil {
					st.Target = &workout.Target{Kind: "cadence", Min: f64(*c - 3), Max: f64(*c + 3)}
				} else if st.Target.Kind != "cadence" {
					st.Note = noteWith(st.Note, cue)
				}
			})
			idx := i
			add("steps", &idx, before, stepsText(p.Steps[i:i+1])+" "+p.Steps[i].Note, morphReasonCadence)
		}
		reasons = append(reasons, morphReasonCadence)
	}

	// 6. Limited descent: a route cue.
	if r.MaxElevLossM != nil {
		reasons = append(reasons, fmt.Sprintf("%s (D− ≤ %s m, parcours plat)", morphReasonElev, frfmt.Int(*r.MaxElevLossM)))
	}

	if len(reasons) > 0 {
		head := "Séance adaptée : " + strings.Join(reasons, " ; ") + "."
		instr := strings.TrimSpace(head + " " + base.Instructions)
		if rr := []rune(instr); len(rr) > 2000 {
			instr = string(rr[:2000])
		}
		add("instructions", nil, base.Instructions, instr, strings.Join(reasons, " ; "))
		p.Instructions = instr
	}
	return &p, changes
}

// diffSessions lists the morphable fields that differ (LLM refinement).
func diffSessions(a, b athlete.PlannedSession, reason string) []MorphChange {
	out := []MorphChange{}
	add := func(field, before, after string) {
		if before != after {
			out = append(out, MorphChange{Field: field, Before: before, After: after, Reason: reason})
		}
	}
	add("title", a.Title, b.Title)
	add("targetDurationS", durText(a.TargetDurationS), durText(b.TargetDurationS))
	add("targetDistanceM", distText(a.TargetDistanceM), distText(b.TargetDistanceM))
	add("targetPace", paceText(a.TargetPaceMinSecPerKm, a.TargetPaceMaxSecPerKm), paceText(b.TargetPaceMinSecPerKm, b.TargetPaceMaxSecPerKm))
	add("targetCadenceSpm", cadText(a.TargetCadenceSpm), cadText(b.TargetCadenceSpm))
	add("steps", stepsText(a.Steps), stepsText(b.Steps))
	add("instructions", a.Instructions, b.Instructions)
	return out
}

// ---------- facts ----------

// morphBase is the session as planned: a morphed session gets its original targets back.
// The result shares no pointer with ps (json.Unmarshal writes through existing pointers).
func morphBase(ps athlete.PlannedSession) athlete.PlannedSession {
	base := cloneSession(ps)
	if ps.Morph != nil {
		ps.Morph.Original.Restore(&base)
	}
	return cloneSession(base)
}

func (s *Services) morphFacts(ctx context.Context, uid int64, p athlete.Profile, today, date string) (morphFacts, error) {
	var f morphFacts
	var err error
	if f.directives, err = s.activeDirectives(ctx, uid, today); err != nil {
		return f, err
	}
	rows, err := s.activitiesBetween(ctx, uid, addDays(today, -3), today)
	if err != nil {
		return f, err
	}
	since := s.now().Add(-morphPainWindow).Unix()
	var ids []int64
	for _, a := range rows {
		if a.StartUTC >= since {
			ids = append(ids, a.ID)
		}
	}
	pain := func(v *float64) {
		if v != nil && (f.painMax == nil || *v > *f.painMax) {
			f.painMax = f64(*v)
		}
	}
	if len(ids) > 0 {
		notes, err := s.repo().Notes(ctx, uid, ids)
		if err != nil {
			return f, err
		}
		for _, n := range notes {
			pain(n.PainDuring)
			pain(n.PainNextMorning)
		}
	}
	var bodyPain, bodyCold *float64
	if err := s.rdb().QueryRowContext(ctx, `SELECT MAX(pain_score), MAX(cold_next_day_score) FROM body_stress_entries WHERE user_id = ? AND date >= ?`,
		uid, addDays(today, -2)).Scan(&bodyPain, &bodyCold); err != nil {
		return f, err
	}
	pain(bodyPain)
	pain(bodyCold)
	lr, err := s.loadRatio(ctx, uid, p, today)
	if err != nil {
		return f, err
	}
	if lr != nil {
		f.acwr = lr.Ratio
	}
	if c := s.morphForecast(ctx, uid, p, date); c != nil {
		f.tempC, f.windKph = c.TemperatureC, c.WindKph
		if c.ApparentC != nil && (f.tempC == nil || *c.ApparentC > *f.tempC) {
			f.tempC = c.ApparentC
		}
	}
	return f, nil
}

// morphForecast reads the forecast at the session slot (the hour of the latest run, 18:00 by
// default) near the latest run start; best effort, cached one hour, nil when unavailable.
func (s *Services) morphForecast(ctx context.Context, uid int64, p athlete.Profile, date string) *weather.Conditions {
	fc, ok := s.deps.Weather.(weather.Forecaster)
	if !ok || s.deps.Weather == nil {
		return nil
	}
	key := userKey(uid, "morph-wx", date)
	if raw, ok := s.cacheGet(ctx, key); ok {
		var c weather.Conditions
		if json.Unmarshal(raw, &c) == nil {
			return &c
		}
	}
	runs, err := s.queryActivities(ctx, `WHERE user_id = ? AND sport = 'run' ORDER BY start_utc DESC LIMIT 5`, uid)
	if err != nil {
		return nil
	}
	for _, a := range runs {
		lat, lon, ok := s.startPosition(ctx, uid, a)
		if !ok {
			continue
		}
		hour := morphDefaultHour
		if len(a.StartLocal) >= 13 {
			if h, err := strconv.Atoi(a.StartLocal[11:13]); err == nil {
				hour = h
			}
		}
		d, err := time.ParseInLocation("2006-01-02", date, s.location(p))
		if err != nil {
			return nil
		}
		wctx, cancel := context.WithTimeout(apimeter.WithUser(ctx, uid), 8*time.Second)
		defer cancel()
		c, err := fc.Forecast(wctx, round2(lat), round2(lon), d.Add(time.Duration(hour)*time.Hour))
		if err != nil {
			s.log.Warn("🌦️ Prévision météo indisponible pour l'adaptation de séance", "user", uid, "err", errText(providerErr("open-meteo", err)))
			return nil
		}
		if raw, err := json.Marshal(c); err == nil {
			s.cacheSet(ctx, key, raw, morphForecastTTL)
		}
		return c
	}
	return nil
}

// ---------- endpoints ----------

// MorphPreview computes the constraints and the rule-based adaptation of today's or tomorrow's session.
func (s *Services) MorphPreview(ctx context.Context, uid, id int64) (*MorphPreview, error) {
	ps, err := s.plannedWithMatch(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	today := s.today(p)
	if ps.Date < today || ps.Date > addDays(today, 1) {
		return nil, badRequest("Adaptation possible pour la séance du jour ou du lendemain uniquement.")
	}
	base := morphBase(ps)
	f, err := s.morphFacts(ctx, uid, p, today, ps.Date)
	if err != nil {
		return nil, err
	}
	cs := morphConstraints(f)
	prop, changes := morphRules(base, f, cs)
	return &MorphPreview{SessionID: id, Constraints: cs, Original: base, Proposal: prop, Changes: changes}, nil
}

func constraintLines(cs []MorphConstraint) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Label+" : "+c.Detail)
	}
	return out
}

// ApplyMorph saves a proposal (rule-based or refined) on the session, keeping the planned trace.
func (s *Services) ApplyMorph(ctx context.Context, uid, id int64, in MorphApplyInput) (athlete.PlannedSession, error) {
	if len(in.Proposal) == 0 || string(in.Proposal) == "null" {
		return athlete.PlannedSession{}, badRequest("proposal requis.")
	}
	ps, err := s.repo().Planned(ctx, uid, id)
	if err != nil {
		return ps, repoErr(err)
	}
	original := athlete.OriginalOf(cloneSession(ps))
	if ps.Morph != nil {
		original = athlete.OriginalOf(morphBase(ps))
	}
	next := morphBase(ps)
	if err := patch(&next, in.Proposal, morphFields, morphIgnored); err != nil {
		return ps, err
	}
	next.ID = id
	if err := next.Normalize(); err != nil {
		return ps, validation(err)
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return ps, err
	}
	f, err := s.morphFacts(ctx, uid, p, s.today(p), ps.Date)
	if err != nil {
		return ps, err
	}
	reasons := constraintLines(morphConstraints(f))
	if len(reasons) == 0 {
		reasons = []string{morphManualReason}
	}
	next.Morph = &athlete.PlanMorph{At: s.now().UTC().Format(time.RFC3339), Reasons: reasons, Original: original}
	if err := s.repo().SetMorph(ctx, uid, &next); err != nil {
		return ps, validation(repoErr(err))
	}
	return s.plannedWithMatch(ctx, uid, id)
}

// RevertMorph restores the planned targets of a morphed session.
func (s *Services) RevertMorph(ctx context.Context, uid, id int64) (athlete.PlannedSession, error) {
	ps, err := s.repo().Planned(ctx, uid, id)
	if err != nil {
		return ps, repoErr(err)
	}
	if ps.Morph == nil {
		return ps, &ConflictError{Msg: "Cette séance n'a pas été adaptée."}
	}
	ps.Morph.Original.Restore(&ps)
	ps.Morph = nil
	if err := s.repo().SetMorph(ctx, uid, &ps); err != nil {
		return ps, validation(repoErr(err))
	}
	return s.plannedWithMatch(ctx, uid, id)
}

// RefineMorph asks the athlete's LLM to refine the adaptation (LLM queue, tokens over SSE; llm.done
// carries the refined morphPreview). It always answers the job status.
func (s *Services) RefineMorph(ctx context.Context, uid, id int64, in MorphRefineInput) (*queue.JobStatus, error) {
	if len([]rune(strings.TrimSpace(in.Note))) > morphNoteRunes {
		return nil, badRequest("Précision trop longue (%d caractères max).", morphNoteRunes)
	}
	c, err := s.llmConfig(ctx, uid)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrLLMNotConfigured
	}
	prev, err := s.MorphPreview(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	p, err := s.profile(ctx, uid)
	if err != nil {
		return nil, err
	}
	today := s.today(p)
	dv, err := s.DayView(ctx, uid, today)
	if err != nil {
		return nil, err
	}
	if dv.Records, err = s.records(ctx, uid); err != nil {
		return nil, err
	}
	if dv.Health, err = s.trackedZones(ctx, uid); err != nil {
		return nil, err
	}
	req := report.MorphRequest{Original: prev.Original, Proposal: prev.Proposal, Constraints: constraintLines(prev.Constraints), Note: in.Note}
	if req.LoadRatio, err = s.loadRatio(ctx, uid, p, today); err != nil {
		return nil, err
	}
	if req.CoachNotes, err = s.coachNotes(ctx, uid, today, nil, addDays(today, -coachNotesDays)); err != nil {
		return nil, err
	}
	return s.queue.Enqueue(ctx, queue.EnqueueRequest{UserID: uid, Kind: queue.KindLLMMorphRefine, Queue: queue.QueueCritical,
		Label:   "Affinage IA de la séance adaptée",
		Payload: llmMorphRefineJobPayload{SessionID: id, Prompt: report.Localize(report.Morph(dv, p, req), s.lang(ctx, p)), Preview: *prev}})
}

func (s *Services) runMorphRefine(ctx context.Context, jc *queue.JobContext) (any, error) {
	var p llmMorphRefineJobPayload
	if err := json.Unmarshal(jc.Payload, &p); err != nil {
		return nil, jobErr(err, false)
	}
	ref := strconv.FormatInt(p.SessionID, 10)
	text, err := s.runLLMJob(ctx, jc, "Affinage IA de la séance adaptée…", p.Prompt, true, "morph_refine", ref)
	if err != nil {
		return nil, err
	}
	out := morphFromLLM(text, p.Preview)
	s.emitLLMDone(jc.UserID, sse.StreamLLMDoneEvent{JobID: jc.ID, Kind: "morph_refine", Ref: ref, MorphPreview: out})
	return out, nil
}

// morphFromLLM keeps the first parsed session of the answer as the proposal (same id, date and
// sport); an unusable answer leaves the rule-based preview as is.
func morphFromLLM(text string, prev MorphPreview) *MorphPreview {
	drafts, _, _ := athlete.ParsePlan(text, "llm")
	out := prev
	if len(drafts) == 0 {
		return &out
	}
	d := drafts[0]
	prop := cloneSession(prev.Original)
	prop.Match, prop.Morph = nil, nil
	prop.Title, prop.TargetDurationS, prop.TargetDistanceM = d.Title, d.TargetDurationS, d.TargetDistanceM
	prop.TargetPaceMinSecPerKm, prop.TargetPaceMaxSecPerKm, prop.TargetCadenceSpm = d.TargetPaceMinSecPerKm, d.TargetPaceMaxSecPerKm, d.TargetCadenceSpm
	prop.Instructions, prop.Steps = d.Instructions, d.Steps
	if prop.Normalize() != nil {
		return &out
	}
	out.Proposal = &prop
	out.Changes = diffSessions(prev.Original, prop, morphReasonLLM)
	return &out
}
