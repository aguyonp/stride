// steps_test.go pins the input schema of the write tools (issue #139): the step
// definition it documents must stay the exact step model of the domain, and the rest
// of the schema must stay the exact argument list of the handlers.
package mcp

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"stride/internal/workout"

	"github.com/google/jsonschema-go/jsonschema"
)

// schemaJSON marshals a schema into the generic map tools/list sends to clients.
func schemaJSON(t *testing.T, s *jsonschema.Schema) map[string]any {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	return m
}

// props returns the property names of an object schema, sorted.
func props(t *testing.T, m any) []string {
	t.Helper()
	obj, ok := m.(map[string]any)
	if !ok {
		t.Fatalf("not an object schema: %#v", m)
	}
	p, ok := obj["properties"].(map[string]any)
	if !ok {
		t.Fatalf("no properties: %#v", obj)
	}
	out := make([]string, 0, len(p))
	for k := range p {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// fieldNames returns the JSON names of the fields of a struct type, sorted.
func fieldNames(t *testing.T, typ reflect.Type) []string {
	t.Helper()
	out := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" {
			t.Fatalf("%s: field %s has no JSON name", typ, typ.Field(i).Name)
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func equal(a, b []string) bool { return reflect.DeepEqual(a, b) }

// enumStrings converts the JSON form of an "enum" keyword into a []string.
func enumStrings(t *testing.T, m any) []string {
	t.Helper()
	obj, ok := m.(map[string]any)
	if !ok {
		t.Fatalf("not a schema: %#v", m)
	}
	raw, ok := obj["enum"].([]any)
	if !ok {
		t.Fatalf("no enum: %#v", obj)
	}
	out := make([]string, len(raw))
	for i, v := range raw {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("non-string enum value: %#v", v)
		}
		out[i] = s
	}
	return out
}

// TestPlanSchemaDocumentsSteps covers the third acceptance criterion: tools/list
// documents the fields expected in `steps`, down to the nested repeat blocks.
func TestPlanSchemaDocumentsSteps(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeFor[planCreateIn](), reflect.TypeFor[planUpdateIn]()} {
		root := schemaJSON(t, planSchema(typ))

		steps, ok := root["properties"].(map[string]any)["steps"].(map[string]any)
		if !ok {
			t.Fatalf("%s: no steps property", typ)
		}
		items, _ := steps["items"].(map[string]any)
		if items["$ref"] != "#/$defs/step" {
			t.Fatalf("%s: steps.items = %#v, want a $defs/step reference", typ, steps["items"])
		}
		if d, _ := steps["description"].(string); !strings.Contains(d, "repeat") {
			t.Fatalf("%s: steps description does not document repeat blocks: %q", typ, d)
		}
		// The separation is stated on both sides: the effort blocks live in steps,
		// instructions keeps the contextual guidance.
		instr, _ := root["properties"].(map[string]any)["instructions"].(map[string]any)
		if d, _ := instr["description"].(string); !strings.Contains(d, "steps") {
			t.Fatalf("%s: instructions description does not point at steps: %q", typ, d)
		}

		defs, ok := root["$defs"].(map[string]any)
		if !ok {
			t.Fatalf("%s: no $defs", typ)
		}
		step, ok := defs["step"].(map[string]any)
		if !ok {
			t.Fatalf("%s: no $defs/step", typ)
		}

		// The documented fields are exactly the step model the domain validates and
		// the Structure editor and the .FIT export read: no field may be missing,
		// none may be invented.
		if got, want := props(t, step), fieldNames(t, reflect.TypeFor[workout.Step]()); !equal(got, want) {
			t.Fatalf("%s: $defs/step properties = %v, want %v", typ, got, want)
		}
		if got := enumStrings(t, step["properties"].(map[string]any)["type"]); !equal(got, workout.StepTypes) {
			t.Fatalf("%s: step type enum = %v, want %v", typ, got, workout.StepTypes)
		}
		if req, _ := step["required"].([]any); len(req) != 1 || req[0] != "type" {
			t.Fatalf("%s: $defs/step required = %#v, want [type]", typ, step["required"])
		}

		// A repeat nested in a repeat is what makes the definition recursive.
		inner, _ := step["properties"].(map[string]any)["steps"].(map[string]any)
		if inner["items"].(map[string]any)["$ref"] != "#/$defs/step" {
			t.Fatalf("%s: nested steps are not recursive: %#v", typ, inner["items"])
		}

		// The target mirrors workout.Target (kind plus its two bounds).
		tgt, _ := step["properties"].(map[string]any)["target"].(map[string]any)
		if got, want := props(t, tgt), fieldNames(t, reflect.TypeFor[workout.Target]()); !equal(got, want) {
			t.Fatalf("%s: target properties = %v, want %v", typ, got, want)
		}
		if got := enumStrings(t, tgt["properties"].(map[string]any)["kind"]); !equal(got, workout.TargetKinds) {
			t.Fatalf("%s: target kind enum = %v, want %v", typ, got, workout.TargetKinds)
		}
	}
}

// TestPlanSchemaMatchesHandlers pins the rest of the schema: every writable field of
// the handlers is documented, and nothing else is (additionalProperties is false, so
// an undocumented field would be unreachable for a client).
func TestPlanSchemaMatchesHandlers(t *testing.T) {
	// planCreateIn repeats date and sport to make them required; its own fields are
	// the full writable set.
	fields := fieldNames(t, reflect.TypeFor[planFields]())
	if got := props(t, schemaJSON(t, planSchema(reflect.TypeFor[planCreateIn]()))); !equal(got, fields) {
		t.Fatalf("plan_create properties = %v, want %v", got, fields)
	}
	update := append(fieldNames(t, reflect.TypeFor[planFields]()), "id")
	sort.Strings(update)
	if got := props(t, schemaJSON(t, planSchema(reflect.TypeFor[planUpdateIn]()))); !equal(got, update) {
		t.Fatalf("plan_update properties = %v, want %v", got, update)
	}
}

// TestPlanSchemaValidation covers the boundary kept on purpose: the schema guards the
// shape (types, required step type, the domain's enums), while semantic rules —
// duration or distance, ranges, nesting depth — stay in the domain so their errors
// come back in-band and localized.
func TestPlanSchemaValidation(t *testing.T) {
	s, err := planSchema(reflect.TypeFor[planCreateIn]()).Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	check := func(raw string, valid bool) {
		t.Helper()
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			t.Fatalf("bad test payload %q: %v", raw, err)
		}
		err := s.Validate(v)
		if valid && err != nil {
			t.Errorf("rejected: %s: %v", raw, err)
		}
		if !valid && err == nil {
			t.Errorf("accepted: %s", raw)
		}
	}

	// A full interval session: warm-up, three blocks of effort + walk, cool-down.
	check(`{"date":"2026-11-24","sport":"run","title":"3 × 10'","instructions":"Chaussures tempo",
		"steps":[{"type":"warmup","durationS":600,"target":{"kind":"pace","min":330,"max":360}},
		{"type":"repeat","repeat":3,"steps":[
			{"type":"interval","durationS":600,"target":{"kind":"pace","min":305,"max":315}},
			{"type":"recovery","durationS":90,"note":"marche"}]},
		{"type":"cooldown","distanceM":1500}]}`, true)
	// Nulls on optional fields are what a JSON client naturally sends.
	check(`{"date":"2026-11-24","sport":"run","steps":[{"type":"run","durationS":null,"note":null,"target":null}]}`, true)
	// A nested repeat is expressible: the definition is recursive.
	check(`{"date":"2026-11-24","sport":"run","steps":[{"type":"repeat","repeat":2,"steps":[{"type":"repeat","repeat":4,"steps":[{"type":"run","durationS":60}]}]}]}`, true)
	// Both a duration and a distance is the domain's call (localized in-band error).
	check(`{"date":"2026-11-24","sport":"run","steps":[{"type":"run","durationS":600,"distanceM":1000}]}`, true)

	// Shape errors are caught before the handler runs.
	check(`{"date":"2026-11-24","steps":[{"type":"sprint"}]}`, false)
	check(`{"date":"2026-11-24","steps":[{"durationS":600}]}`, false)
	check(`{"date":"2026-11-24","steps":[{"type":"run","pace":300}]}`, false)
	check(`{"date":"2026-11-24","steps":[{"type":"run","target":{"min":300,"max":320}}]}`, false)
	check(`{"date":"2026-11-24","steps":[{"type":"run","target":{"kind":"speed"}}]}`, false)
	check(`{"date":"2026-11-24","steps":"600s"}`, false)
	check(`{"sport":"run","steps":[{"type":"run"}]}`, false)
}
