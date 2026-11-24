// records_test.go pins the semantics of Patch, the JSON overlay shared by every write
// route (REST and MCP): a provided key replaces its value, an absent key keeps it.
package athlete

import (
	"errors"
	"testing"
)

func num(v float64) *float64 { return &v }

// TestPatchReplacesSteps pins the regression found with issue #139: a steps array
// replaces the previous one entirely. Decoding the merged document in place made
// encoding/json reuse the slice elements, so a block replaced by a plain step kept
// the sub-steps of the old block (and the session was then rejected as invalid).
func TestPatchReplacesSteps(t *testing.T) {
	ps := PlannedSession{Date: "2026-11-24", Sport: "run", Instructions: "zone 2"}
	first := `{"steps":[
		{"type":"warmup","durationS":600},
		{"type":"repeat","repeat":3,"steps":[{"type":"interval","durationS":600},{"type":"recovery","durationS":90}]},
		{"type":"cooldown","durationS":600}]}`
	if err := Patch(&ps, []byte(first), PlanFields, PlanReadOnly); err != nil {
		t.Fatalf("first patch: %v", err)
	}
	if err := ps.Normalize(); err != nil {
		t.Fatalf("first normalize: %v", err)
	}
	if len(ps.Steps) != 3 || ps.Steps[1].Repeat == nil || *ps.Steps[1].Repeat != 3 {
		t.Fatalf("first patch: %+v", ps.Steps)
	}

	// A shorter structure of plain steps: no sub-step may survive.
	second := `{"title":"2 × 15'","steps":[{"type":"run","durationS":900},{"type":"recovery","distanceM":200}]}`
	if err := Patch(&ps, []byte(second), PlanFields, PlanReadOnly); err != nil {
		t.Fatalf("second patch: %v", err)
	}
	if err := ps.Normalize(); err != nil {
		t.Fatalf("second normalize (stale sub-steps?): %v", err)
	}
	if len(ps.Steps) != 2 || len(ps.Steps[1].Steps) != 0 || ps.Steps[1].Repeat != nil {
		t.Fatalf("steps: %+v", ps.Steps)
	}
	if ps.Steps[1].DistanceM == nil || *ps.Steps[1].DistanceM != 200 || ps.Steps[1].DurationS != nil {
		t.Fatalf("second step: %+v", ps.Steps[1])
	}
	// The keys the body omits are kept.
	if ps.Title != "2 × 15'" || ps.Instructions != "zone 2" || ps.Date != "2026-11-24" || ps.Sport != "run" {
		t.Fatalf("kept fields: %+v", ps)
	}
}

// TestPatchClearsWithNull keeps the documented null semantics: an explicit null clears
// a pointer field, while an absent key leaves it alone.
func TestPatchClearsWithNull(t *testing.T) {
	ps := PlannedSession{Date: "2026-11-24", Sport: "run", TargetDurationS: num(1800), Title: "Footing"}
	if err := Patch(&ps, []byte(`{"targetDurationS":null}`), PlanFields, PlanReadOnly); err != nil {
		t.Fatalf("patch: %v", err)
	}
	if ps.TargetDurationS != nil || ps.Title != "Footing" {
		t.Fatalf("null: %+v", ps)
	}
}

// TestPatchRejectsUnknownField: a key outside the allowed and read-only sets is an error,
// not a silent drop.
func TestPatchRejectsUnknownField(t *testing.T) {
	ps := PlannedSession{Date: "2026-11-24", Sport: "run"}
	err := Patch(&ps, []byte(`{"steps":[],"bogus":1}`), PlanFields, PlanReadOnly)
	if !errors.Is(err, ErrUnknownField) {
		t.Fatalf("err: %v", err)
	}
	// A read-only key is ignored without failing (a push state never comes from a client).
	if err := Patch(&ps, []byte(`{"id":42,"match":null,"garmin":{"status":"pushed"}}`), PlanFields, PlanReadOnly); err != nil {
		t.Fatalf("read-only: %v", err)
	}
	if ps.ID != 0 || ps.Match != nil || ps.Garmin.Status != "" {
		t.Fatalf("read-only applied: %+v", ps)
	}
}
