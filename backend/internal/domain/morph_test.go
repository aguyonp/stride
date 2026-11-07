package domain

import (
	"strings"
	"testing"

	"stride/internal/athlete"
	"stride/internal/workout"
)

func ip(v int) *int { return &v }

func strp(s string) *string { return &s }

// intervalSession: 15' warm-up, 6×(3' at 4:00–4:10 + 2' recovery), 10' cool-down, 60', 12 km.
func intervalSession() athlete.PlannedSession {
	return athlete.PlannedSession{ID: 7, Date: "2026-11-08", Sport: "run", Title: "6×3' VMA", TargetDurationS: fp(3600), TargetDistanceM: fp(12000),
		TargetPaceMinSecPerKm: fp(240), TargetPaceMaxSecPerKm: fp(250), Instructions: "Fractionné.",
		Steps: []workout.Step{
			{Type: "warmup", DurationS: fp(900)},
			{Type: "repeat", Repeat: ip(6), Steps: []workout.Step{
				{Type: "interval", DurationS: fp(180), Target: &workout.Target{Kind: "pace", Min: fp(240), Max: fp(250)}},
				{Type: "recovery", DurationS: fp(120)},
			}},
			{Type: "cooldown", DurationS: fp(600)},
		}}
}

func directive(r athlete.DirectiveRestrictions) athlete.ShareNote {
	return athlete.ShareNote{Kind: "directive", AuthorRole: "physio", LinkLabel: "Kiné", Restrictions: &r}
}

func fieldsOf(ch []MorphChange) string {
	var out []string
	for _, c := range ch {
		out = append(out, c.Field)
	}
	return strings.Join(out, ",")
}

func TestMorphNoConstraintNoProposal(t *testing.T) {
	f := morphFacts{painMax: fp(2.5), acwr: fp(1.2), tempC: fp(21), windKph: fp(25),
		directives: []athlete.ShareNote{{Kind: "directive", AuthorRole: "coach", Body: "Courage"}}}
	cs := morphConstraints(f)
	if len(cs) != 0 {
		t.Fatalf("constraints below thresholds: %+v", cs)
	}
	if p, ch := morphRules(intervalSession(), f, cs); p != nil || len(ch) != 0 {
		t.Fatalf("proposal without constraint: %+v %+v", p, ch)
	}
}

func TestMorphNoIntervalsDirective(t *testing.T) {
	f := morphFacts{directives: []athlete.ShareNote{directive(athlete.DirectiveRestrictions{NoIntervals: true})}}
	cs := morphConstraints(f)
	if len(cs) != 1 || cs[0].Kind != "directive" || cs[0].Severity != 3 || !strings.Contains(cs[0].Detail, "pas de fractionné") {
		t.Fatalf("constraints: %+v", cs)
	}
	base := intervalSession()
	p, ch := morphRules(base, f, cs)
	if p == nil || len(p.Steps) != 1 || p.Steps[0].Type != "run" || workout.Structured(p.Steps) || *p.Steps[0].DurationS != 3600 {
		t.Fatalf("intervals kept: %+v", p)
	}
	if p.TargetPaceMinSecPerKm != nil || p.Title != "Footing facile" || !strings.HasPrefix(p.Instructions, "Séance adaptée") {
		t.Fatalf("easy session: %+v", p)
	}
	if got := fieldsOf(ch); got != "steps,targetPace,title,instructions" {
		t.Fatalf("changes: %s", got)
	}
	if err := p.Normalize(); err != nil {
		t.Fatalf("proposal invalid: %v", err)
	}
	if len(base.Steps) != 3 {
		t.Fatal("rules mutated the original")
	}
}

func TestMorphPainCutsVolumeAndIntensity(t *testing.T) {
	f := morphFacts{painMax: fp(4.5)}
	cs := morphConstraints(f)
	if len(cs) != 1 || cs[0].Kind != "pain" || cs[0].Severity != 2 {
		t.Fatalf("constraints: %+v", cs)
	}
	p, ch := morphRules(intervalSession(), f, cs)
	if workout.Structured(p.Steps) {
		t.Fatal("intervals kept despite pain")
	}
	if *p.TargetDurationS != 2880 || *p.TargetDistanceM != 9600 || *p.Steps[0].DurationS != 2880 { // −20 %
		t.Fatalf("volume: %v %v %v", *p.TargetDurationS, *p.TargetDistanceM, *p.Steps[0].DurationS)
	}
	if !strings.Contains(ch[len(ch)-1].Reason, "Douleur") {
		t.Fatalf("reason: %+v", ch)
	}
}

func TestMorphLoadSeverityCuts(t *testing.T) {
	for _, c := range []struct {
		acwr float64
		want float64
	}{{1.35, 3060}, {1.45, 2880}, {1.6, 2520}} { // −15 / −20 / −30 % of 60'
		f := morphFacts{acwr: fp(c.acwr)}
		cs := morphConstraints(f)
		if len(cs) != 1 || cs[0].Kind != "load" {
			t.Fatalf("%v: %+v", c.acwr, cs)
		}
		ps := athlete.PlannedSession{Date: "2026-11-08", Sport: "run", Title: "Footing", TargetDurationS: fp(3600)}
		p, _ := morphRules(ps, f, cs)
		if *p.TargetDurationS != c.want {
			t.Fatalf("acwr %v: %v, want %v", c.acwr, *p.TargetDurationS, c.want)
		}
	}
}

func TestMorphHeatSlowsPaces(t *testing.T) {
	f := morphFacts{tempC: fp(28)}
	cs := morphConstraints(f)
	if len(cs) != 1 || cs[0].Kind != "heat" || cs[0].Severity != 2 {
		t.Fatalf("constraints: %+v", cs)
	}
	p, ch := morphRules(intervalSession(), f, cs)
	in := p.Steps[1].Steps[0].Target
	if *p.TargetPaceMinSecPerKm != 255 || *p.TargetPaceMaxSecPerKm != 265 || *in.Min != 255 || *in.Max != 265 { // +15 s/km at 28 °C
		t.Fatalf("paces: %v %v %v %v", *p.TargetPaceMinSecPerKm, *p.TargetPaceMaxSecPerKm, *in.Min, *in.Max)
	}
	var stepChange *MorphChange
	for i := range ch {
		if ch[i].Field == "steps" && ch[i].StepIndex != nil {
			stepChange = &ch[i]
		}
	}
	if stepChange == nil || *stepChange.StepIndex != 1 || !workout.Structured(p.Steps) {
		t.Fatalf("step change: %+v", ch)
	}
	if heatSlowdown(23) != 5 || heatSlowdown(40) != 30 {
		t.Fatal("slowdown bounds")
	}
}

func TestMorphWindAddsEffortCues(t *testing.T) {
	f := morphFacts{windKph: fp(45)}
	cs := morphConstraints(f)
	if len(cs) != 1 || cs[0].Kind != "wind" || cs[0].Severity != 2 {
		t.Fatalf("constraints: %+v", cs)
	}
	p, ch := morphRules(intervalSession(), f, cs)
	if !strings.Contains(p.Steps[1].Steps[0].Note, "effort") || !strings.Contains(p.Steps[1].Note, "effort") || p.Steps[0].Note != "" {
		t.Fatalf("cues: %+v", p.Steps)
	}
	if ch[0].Field != "steps.note" || ch[0].StepIndex == nil || *ch[0].StepIndex != 1 {
		t.Fatalf("changes: %+v", ch)
	}
	if *p.Steps[1].Steps[0].Target.Min != 240 {
		t.Fatal("wind changed the pace")
	}
}

func TestMorphCadenceDirective(t *testing.T) {
	f := morphFacts{directives: []athlete.ShareNote{directive(athlete.DirectiveRestrictions{CadenceTargetSpm: fp(172), MaxElevLossM: fp(50)})}}
	cs := morphConstraints(f)
	if len(cs) != 1 || cs[0].Severity != 2 {
		t.Fatalf("constraints: %+v", cs)
	}
	p, _ := morphRules(intervalSession(), f, cs)
	if p.TargetCadenceSpm == nil || *p.TargetCadenceSpm != 172 {
		t.Fatal("session cadence")
	}
	if w := p.Steps[0].Target; w == nil || w.Kind != "cadence" || *w.Min != 169 || *w.Max != 175 {
		t.Fatalf("warm-up cadence target: %+v", w)
	}
	iv := p.Steps[1].Steps[0]
	if iv.Target.Kind != "pace" || !strings.Contains(iv.Note, "172") || p.Steps[1].Steps[1].Target != nil {
		t.Fatalf("interval / recovery: %+v %+v", iv, p.Steps[1].Steps[1])
	}
	if !strings.Contains(p.Instructions, "D− ≤ 50 m") {
		t.Fatalf("descent cue: %s", p.Instructions)
	}
	if err := p.Normalize(); err != nil {
		t.Fatalf("proposal invalid: %v", err)
	}
}

func TestMorphStrictestRestrictionWins(t *testing.T) {
	m := mergedRestrictions([]athlete.ShareNote{
		directive(athlete.DirectiveRestrictions{MaxIntensity: strp("moderate"), MaxElevLossM: fp(200)}),
		directive(athlete.DirectiveRestrictions{MaxIntensity: strp("easy"), MaxElevLossM: fp(80)}),
	})
	if *m.MaxIntensity != "easy" || *m.MaxElevLossM != 80 {
		t.Fatalf("merged: %+v", m)
	}
	p, _ := morphRules(intervalSession(), morphFacts{directives: []athlete.ShareNote{directive(athlete.DirectiveRestrictions{MaxIntensity: strp("easy")})}},
		[]MorphConstraint{{Kind: "directive", Severity: 3}})
	if workout.Structured(p.Steps) {
		t.Fatal("easy-only directive kept the intervals")
	}
}

func TestMorphFromLLMKeepsIdentity(t *testing.T) {
	prev := MorphPreview{SessionID: 7, Original: intervalSession()}
	text := "```json\n" + `{"format":"stride-plan","version":1,"sessions":[{"date":"2030-01-01","sport":"bike","title":"Footing 40'","targetDurationS":2400,"instructions":"Facile"}]}` + "\n```"
	out := morphFromLLM(text, prev)
	if out.Proposal == nil || out.Proposal.Date != "2026-11-08" || out.Proposal.Sport != "run" || out.Proposal.ID != 7 || out.Proposal.Title != "Footing 40'" {
		t.Fatalf("proposal: %+v", out.Proposal)
	}
	if len(out.Changes) == 0 || out.Changes[0].Reason != morphReasonLLM {
		t.Fatalf("changes: %+v", out.Changes)
	}
	if bad := morphFromLLM("pas de JSON", prev); bad.Proposal != nil {
		t.Fatal("unusable answer produced a proposal")
	}
}
