package athlete

import (
	"strings"
	"testing"
)

const contract = `{"format":"stride-plan","version":1,"sessions":[{"date":"2026-11-10","sport":"run","title":"Footing + 6×1'","instructions":"Cadence 172 spm, relâché","targetDurationS":2700,"targetCadenceSpm":172,"steps":[{"type":"warmup","durationS":600,"target":{"kind":"pace","min":330,"max":360}},{"type":"repeat","repeat":6,"steps":[{"type":"interval","durationS":60,"target":{"kind":"pace","min":250,"max":260}},{"type":"recovery","durationS":90}]},{"type":"cooldown","durationS":600}]}]}`

func TestParsePlanContract(t *testing.T) {
	text := "Voici le plan :\n```json\n" + contract + "\n```\nBon courage."
	drafts, errs := ParsePlan(text, "import")
	if len(errs) != 0 || len(drafts) != 1 {
		t.Fatalf("drafts=%d errs=%v", len(drafts), errs)
	}
	d := drafts[0]
	if d.Date != "2026-11-10" || d.Source != "import" || len(d.Steps) != 3 || *d.Steps[1].Repeat != 6 || len(d.Steps[1].Steps) != 2 ||
		*d.TargetCadenceSpm != 172 || d.Steps[0].Target.Kind != "pace" {
		t.Fatalf("draft: %+v", d)
	}
}

func TestParsePlanTolerant(t *testing.T) {
	text := `Plan: [{"date":"11/11/2026","sport":"Course","titre":"Seuil","steps":[{"type":"Échauffement","duration":"15 min"},
	{"repeat":"3","steps":[{"type":"work","distance":"1 km","target":{"type":"allure","min":"4:10","max":"4:00"}},{"type":"récup","duration":"2:00"}]},
	{"type":"cooldown","duration":"10'"}]},{"date":"2026-13-01","sport":"run","steps":[{"type":"sprint"}]}]`
	drafts, errs := ParsePlan(text, "llm")
	if len(drafts) != 1 || len(errs) != 2 {
		t.Fatalf("drafts=%d errs=%q", len(drafts), errs)
	}
	d := drafts[0]
	if d.Date != "2026-11-11" || d.Sport != "run" || d.Title != "Seuil" || *d.Steps[0].DurationS != 900 || d.Steps[1].Type != "repeat" ||
		*d.Steps[1].Steps[0].DistanceM != 1000 || *d.Steps[1].Steps[0].Target.Min != 240 || *d.Steps[2].DurationS != 600 || d.TargetDurationS == nil {
		t.Fatalf("draft: %+v %+v", d, d.Steps[1].Steps[0])
	}
	if !strings.HasPrefix(errs[0], "Séance 2 : ") {
		t.Fatalf("errs: %q", errs)
	}
	if _, errs := ParsePlan("pas de json ici", "import"); len(errs) != 1 {
		t.Fatal(errs)
	}
}
