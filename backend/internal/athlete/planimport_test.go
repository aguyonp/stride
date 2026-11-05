package athlete

import (
	"strings"
	"testing"
)

const contract = `{"format":"stride-plan","version":1,"sessions":[{"date":"2026-11-10","sport":"run","title":"Footing + 6×1'","instructions":"Cadence 172 spm, relâché","targetDurationS":2700,"targetCadenceSpm":172,"steps":[{"type":"warmup","durationS":600,"target":{"kind":"pace","min":330,"max":360}},{"type":"repeat","repeat":6,"steps":[{"type":"interval","durationS":60,"target":{"kind":"pace","min":250,"max":260}},{"type":"recovery","durationS":90}]},{"type":"cooldown","durationS":600}]}]}`

func TestParsePlanContract(t *testing.T) {
	text := "Voici le plan :\n```json\n" + contract + "\n```\nBon courage."
	drafts, _, errs := ParsePlan(text, "import")
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
	drafts, _, errs := ParsePlan(text, "llm")
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
	if _, _, errs := ParsePlan("pas de json ici", "import"); len(errs) != 1 {
		t.Fatal(errs)
	}
}

func TestParsePlanV2Phases(t *testing.T) {
	text := "Voici le plan :\n```json\n" + `{"format":"stride-plan","version":2,"goal":{"title":"10 km","date":"2027-02-15"},
		"phases":[{"name":"Développement","startDate":"2026-11-09","endDate":"2026-12-06","focus":"volume aérobie","weeklyVolumeKm":"35 km"},
		{"name":"Affûtage","start":"01/02/2027","end":"2027-02-15","weeklyVolumeKm":20},
		{"name":"","startDate":"2026-12-07","endDate":"2026-12-01"}],
		"sessions":[{"date":"2026-11-10","sport":"run","title":"Footing","targetDurationS":2700}]}` + "\n```"
	drafts, phases, errs := ParsePlan(text, "llm")
	if len(drafts) != 1 || len(phases) != 2 || len(errs) != 2 {
		t.Fatalf("drafts=%d phases=%+v errs=%v", len(drafts), phases, errs)
	}
	if phases[0].WeeklyVolumeKm == nil || *phases[0].WeeklyVolumeKm != 35 {
		t.Fatalf("volume: %v", phases[0].WeeklyVolumeKm)
	}
	if phases[1].StartDate != "2027-02-01" || phases[1].ID != 0 {
		t.Fatalf("phase dates: %+v", phases[1])
	}
	if _, ph, errs := ParsePlan(`{"format":"stride-plan","version":2,"phases":[{"name":"Base","startDate":"2026-11-09","endDate":"2026-11-30"}],"sessions":[]}`, "import"); len(ph) != 1 || len(errs) != 0 {
		t.Fatalf("phases only: %v %v", ph, errs)
	}
}
