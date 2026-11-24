// steps_e2e_test.go covers issue #139 end to end, through the public MCP route and
// the official SDK client: plan_create/plan_update accept a structured workout
// (`steps`, with repeat blocks), the structure is validated and persisted, comes back
// in the write answer and in the read views, and the .FIT export consumes it.
package mcp_test

import (
	"context"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// textOf concatenates the text content of a tool result (the human-readable answer).
func textOf(res *mcpsdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// stepsOf returns the `steps` array of a session payload.
func stepsOf(t *testing.T, m map[string]any) []any {
	t.Helper()
	s, ok := m["steps"].([]any)
	if !ok {
		t.Fatalf("steps is not an array: %#v", m["steps"])
	}
	return s
}

// obj returns list[i] as an object.
func obj(t *testing.T, list []any, i int) map[string]any {
	t.Helper()
	if i >= len(list) {
		t.Fatalf("index %d out of range (%d steps)", i, len(list))
	}
	m, ok := list[i].(map[string]any)
	if !ok {
		t.Fatalf("step %d is not an object: %#v", i, list[i])
	}
	return m
}

// intervalSession is the session of the issue: 3 × 10 min at 5:05–5:15/km with
// 1 min 30 of walking between blocks, plus warm-up and cool-down.
func intervalSession(date string) map[string]any {
	return map[string]any{
		"date":                  date,
		"sport":                 "run",
		"title":                 "3 × 10' au seuil",
		"targetDurationS":       2190,
		"targetPaceMinSecPerKm": 305,
		"targetPaceMaxSecPerKm": 315,
		"instructions":          "Chaussures tempo, échauffement et renfo avant le départ",
		"steps": []any{
			map[string]any{"type": "warmup", "durationS": 600, "target": map[string]any{"kind": "pace", "min": 330, "max": 360}},
			map[string]any{"type": "repeat", "repeat": 3, "steps": []any{
				map[string]any{"type": "interval", "durationS": 600, "target": map[string]any{"kind": "pace", "min": 305, "max": 315}},
				map[string]any{"type": "recovery", "durationS": 90, "note": "marche"},
			}},
			map[string]any{"type": "cooldown", "durationS": 600},
		},
	}
}

// TestMCPPlanCreateWithSteps covers the acceptance criteria of issue #139: a
// plan_create with steps persists the structure, returns it, and the .FIT export
// carries the blocks.
func TestMCPPlanCreateWithSteps(t *testing.T) {
	h := newHarness(t)
	today := time.Now().In(paris).Format("2006-01-02")
	ctx := context.Background()

	token := h.genToken(2)
	cs := h.connect(t, token, "")
	defer cs.Close()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: "plan_create", Arguments: intervalSession(today)})
	if err != nil {
		t.Fatalf("plan_create: %v", err)
	}
	if res.IsError {
		t.Fatalf("plan_create: IsError: %v", res.Content)
	}
	created := mustMap(t, res.StructuredContent)
	id, _ := created["id"].(float64)
	if id <= 0 {
		t.Fatalf("plan_create: no id: %v", created)
	}

	// The answer carries the structure, blocks and targets included.
	steps := stepsOf(t, created)
	if len(steps) != 3 {
		t.Fatalf("steps: %d steps, want 3: %#v", len(steps), steps)
	}
	if warm := obj(t, steps, 0); warm["type"] != "warmup" || warm["durationS"] != float64(600) {
		t.Fatalf("warm-up: %#v", warm)
	}
	block := obj(t, steps, 1)
	if block["type"] != "repeat" || block["repeat"] != float64(3) {
		t.Fatalf("repeat block: %#v", block)
	}
	inner, _ := block["steps"].([]any)
	if len(inner) != 2 {
		t.Fatalf("repeat block steps: %#v", block["steps"])
	}
	effort := obj(t, inner, 0)
	target, _ := effort["target"].(map[string]any)
	if effort["type"] != "interval" || effort["durationS"] != float64(600) || target["kind"] != "pace" ||
		target["min"] != float64(305) || target["max"] != float64(315) {
		t.Fatalf("effort step: %#v", effort)
	}
	if walk := obj(t, inner, 1); walk["type"] != "recovery" || walk["durationS"] != float64(90) || walk["note"] != "marche" {
		t.Fatalf("walk step: %#v", walk)
	}
	// instructions keeps the contextual guidance only: the effort blocks are not
	// duplicated there.
	if instr, _ := created["instructions"].(string); strings.Contains(instr, "repeat") || !strings.Contains(instr, "Chaussures") {
		t.Fatalf("instructions: %q", instr)
	}

	// Persisted with the session, not just echoed: the row holds the structure …
	var stored string
	if err := h.st.DB.QueryRow(`SELECT steps FROM planned_sessions WHERE user_id = 2 AND id = ?`, int64(id)).Scan(&stored); err != nil {
		t.Fatalf("stored steps: %v", err)
	}
	if !strings.Contains(stored, `"repeat":3`) || !strings.Contains(stored, `"kind":"pace"`) {
		t.Fatalf("stored steps: %s", stored)
	}

	// … and a second client (a new session, so fresh reads) sees it in day and planning.
	cs2 := h.connect(t, token, "")
	defer cs2.Close()
	day, err := cs2.CallTool(ctx, &mcpsdk.CallToolParams{Name: "day", Arguments: map[string]any{"date": today}})
	if err != nil || day.IsError {
		t.Fatalf("day: %v %v", err, day)
	}
	planned, _ := mustMap(t, day.StructuredContent)["planned"].([]any)
	if len(planned) != 1 {
		t.Fatalf("day planned: %#v", planned)
	}
	daySteps := stepsOf(t, obj(t, planned, 0))
	if len(daySteps) != 3 || obj(t, daySteps, 1)["repeat"] != float64(3) {
		t.Fatalf("day steps: %#v", daySteps)
	}
	week, err := cs2.CallTool(ctx, &mcpsdk.CallToolParams{Name: "planning", Arguments: struct{}{}})
	if err != nil || week.IsError {
		t.Fatalf("planning: %v %v", err, week)
	}
	found := false
	for _, d := range mustMap(t, week.StructuredContent)["days"].([]any) {
		dm, _ := d.(map[string]any)
		if dm["date"] != today {
			continue
		}
		for _, p := range dm["planned"].([]any) {
			pm, _ := p.(map[string]any)
			if int64(pm["id"].(float64)) == int64(id) {
				found = true
				if got := stepsOf(t, pm); len(got) != 3 {
					t.Fatalf("planning steps: %#v", got)
				}
			}
		}
	}
	if !found {
		t.Fatalf("planning: session %d not in today's plan", int64(id))
	}

	// The .FIT export encodes the steps (14-byte header, ".FIT" magic at offset 8).
	fit, name, err := h.dom.WorkoutFIT(ctx, 2, int64(id))
	if err != nil {
		t.Fatalf("FIT: %v", err)
	}
	if len(fit) < 14 || string(fit[8:12]) != ".FIT" {
		t.Fatalf("FIT: not a FIT file (%d bytes)", len(fit))
	}
	if !strings.Contains(name, today) {
		t.Fatalf("FIT name: %q", name)
	}
}

// TestMCPPlanUpdateSteps covers plan_update: steps replaces the structure, an
// omitted steps keeps it (backward compatibility), an empty array clears it.
func TestMCPPlanUpdateSteps(t *testing.T) {
	h := newHarness(t)
	today := time.Now().In(paris).Format("2006-01-02")
	ctx := context.Background()
	cs := h.connect(t, h.genToken(2), "")
	defer cs.Close()

	call := func(name string, args any) map[string]any {
		t.Helper()
		res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s: IsError: %s", name, textOf(res))
		}
		return mustMap(t, res.StructuredContent)
	}

	created := call("plan_create", intervalSession(today))
	id := int64(created["id"].(float64))

	// A new structure replaces the previous one.
	updated := call("plan_update", map[string]any{"id": id, "title": "2 × 15'", "steps": []any{
		map[string]any{"type": "run", "durationS": 900, "target": map[string]any{"kind": "pace", "min": 300, "max": 310}},
		map[string]any{"type": "recovery", "distanceM": 200},
		map[string]any{"type": "run", "durationS": 900, "target": map[string]any{"kind": "pace", "min": 300, "max": 310}},
	}})
	if updated["title"] != "2 × 15'" || updated["instructions"] != created["instructions"] {
		t.Fatalf("plan_update: %v", updated)
	}
	steps := stepsOf(t, updated)
	if len(steps) != 3 || obj(t, steps, 1)["distanceM"] != float64(200) {
		t.Fatalf("replaced steps: %#v", steps)
	}

	// Omitting steps leaves the structure alone (only the provided fields apply).
	kept := call("plan_update", map[string]any{"id": id, "instructions": "sortie tôt"})
	if got := stepsOf(t, kept); len(got) != 3 || obj(t, got, 1)["type"] != "recovery" {
		t.Fatalf("omit steps: %#v", got)
	}
	if kept["instructions"] != "sortie tôt" || kept["title"] != "2 × 15'" {
		t.Fatalf("omit steps: %v", kept)
	}

	// An empty array clears it, and the session stays usable by the .FIT guard.
	if got := stepsOf(t, call("plan_update", map[string]any{"id": id, "steps": []any{}})); len(got) != 0 {
		t.Fatalf("clear steps: %#v", got)
	}
	if _, _, err := h.dom.WorkoutFIT(ctx, 2, id); err == nil {
		t.Fatal("FIT without steps: expected an error")
	}
}

// TestMCPPlanStepsRejected covers the server-side validation of the structure: the
// semantic rules stay in the domain, so their errors come back in-band, in the
// caller's language, naming the faulty step.
func TestMCPPlanStepsRejected(t *testing.T) {
	h := newHarness(t)
	today := time.Now().In(paris).Format("2006-01-02")
	ctx := context.Background()
	cs := h.connect(t, h.genToken(2), "fr")
	defer cs.Close()

	rejected := func(name string, args any, want string) {
		t.Helper()
		res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !res.IsError {
			t.Fatalf("%s: accepted %v", name, res.StructuredContent)
		}
		if msg := textOf(res); !strings.Contains(msg, want) {
			t.Fatalf("%s: message %q, want it to mention %q", name, msg, want)
		}
	}
	rejected("plan_create", map[string]any{"date": today, "sport": "run", "steps": []any{
		map[string]any{"type": "run", "durationS": 600, "distanceM": 1000},
	}}, "durée ou une distance")
	// Nesting is capped: the domain backstops the recursive schema.
	rejected("plan_create", map[string]any{"date": today, "sport": "run", "steps": []any{
		map[string]any{"type": "repeat", "repeat": 2, "steps": []any{
			map[string]any{"type": "repeat", "repeat": 2, "steps": []any{
				map[string]any{"type": "repeat", "repeat": 2, "steps": []any{
					map[string]any{"type": "run", "durationS": 60},
				}},
			}},
		}},
	}}, "trop profondes")
	// Ranges too.
	rejected("plan_create", map[string]any{"date": today, "sport": "run", "steps": []any{
		map[string]any{"type": "run", "durationS": 90000},
	}}, "hors limites")
}

// TestMCPPlanWithoutSteps pins the backward compatibility asked by the issue: a
// call without steps behaves exactly as before (an empty structure, no error).
func TestMCPPlanWithoutSteps(t *testing.T) {
	h := newHarness(t)
	today := time.Now().In(paris).Format("2006-01-02")
	ctx := context.Background()
	cs := h.connect(t, h.genToken(2), "")
	defer cs.Close()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: "plan_create",
		Arguments: map[string]any{"date": today, "sport": "run", "title": "Footing", "instructions": "zone 2"}})
	if err != nil || res.IsError {
		t.Fatalf("plan_create: %v %v", err, res)
	}
	created := mustMap(t, res.StructuredContent)
	if got := stepsOf(t, created); len(got) != 0 {
		t.Fatalf("steps: %#v, want an empty array", got)
	}
	if created["instructions"] != "zone 2" {
		t.Fatalf("instructions: %v", created["instructions"])
	}
}

// TestMCPStepsSchemaListsTheStepModel checks what a client actually receives from
// tools/list: `steps` is documented as an array of the recursive step definition.
func TestMCPStepsSchemaListsTheStepModel(t *testing.T) {
	h := newHarness(t)
	cs := h.connect(t, h.genToken(2), "")
	defer cs.Close()

	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, tl := range tools.Tools {
		if tl.Name != "plan_create" && tl.Name != "plan_update" {
			continue
		}
		checked++
		root, ok := tl.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("%s: input schema is %T", tl.Name, tl.InputSchema)
		}
		steps, ok := root["properties"].(map[string]any)["steps"].(map[string]any)
		if !ok {
			t.Fatalf("%s: no steps property in %v", tl.Name, root["properties"])
		}
		if items, _ := steps["items"].(map[string]any); items["$ref"] != "#/$defs/step" {
			t.Fatalf("%s: steps.items = %#v", tl.Name, steps["items"])
		}
		defs, _ := root["$defs"].(map[string]any)
		step, _ := defs["step"].(map[string]any)
		if step == nil {
			t.Fatalf("%s: no $defs/step", tl.Name)
		}
		for _, key := range []string{"type", "durationS", "distanceM", "repeat", "steps", "target", "note"} {
			if _, ok := step["properties"].(map[string]any)[key]; !ok {
				t.Fatalf("%s: $defs/step lacks %q: %v", tl.Name, key, step["properties"])
			}
		}
		if !strings.Contains(tl.Description, "steps") {
			t.Fatalf("%s: description does not mention steps: %q", tl.Name, tl.Description)
		}
	}
	if checked != 2 {
		t.Fatalf("checked %d tools, want plan_create and plan_update", checked)
	}
}
