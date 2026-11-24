// steps.go documents the structured workout steps in the input schema of the write
// tools (issue #139).
//
// The tools take the very same step model as the REST API, the Structure (steps)
// editor and the .FIT export (workout.Step), but that type is recursive
// (Step.Steps): jsonschema-go refuses cycles, so inferring the schema would panic at
// AddTool. The step definition is therefore written once, by hand, into $defs/step
// and referenced from the `steps` property. Everything else — property names, types,
// required fields, descriptions — still comes from the Go structs and their
// jsonschema tags, so there is no second copy of the argument list to keep in sync.
//
// The schema is English like every other MCP schema (tool descriptions and tool
// errors follow the profile language, the schemas do not). It enforces the shape
// (types, the required step `type`, the workout package's type and target-kind
// enums) and documents the rest in prose; ranges, "duration or distance but not
// both" and the nesting depth stay the domain's job, so their failures come back
// in-band, localized, and next to the session they concern.
package mcp

import (
	"reflect"

	"stride/internal/workout"

	"github.com/google/jsonschema-go/jsonschema"
)

// enumOf turns a domain enum into a JSON Schema enum, so the schema can never drift
// from the values the domain accepts.
func enumOf(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

// opt is an optional property: null or the given type, with its description. It is the
// shape the inference produces for a pointer field, so the hand-written definitions
// match the generated ones.
func opt(typ, desc string) *jsonschema.Schema {
	return &jsonschema.Schema{Types: []string{"null", typ}, Description: desc}
}

// noMore is the JSON Schema `false`: no property is allowed here. jsonschema-go
// marshals &Schema{Not: &Schema{}} as "false" (its own representation of it).
func noMore() *jsonschema.Schema { return &jsonschema.Schema{Not: &jsonschema.Schema{}} }

// targetSchema is one workout.Step.Target: the kind plus its bounds.
func targetSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:     "object",
		Required: []string{"kind"},
		Properties: map[string]*jsonschema.Schema{
			"kind": {Type: "string", Enum: enumOf(workout.TargetKinds),
				Description: "what the bounds measure: pace in s/km (the fastest value is the minimum), hr in bpm, cadence in spm, power in W"},
			"min": opt("number", "lower bound of the target, or its only bound when max is absent"),
			"max": opt("number", "upper bound of the target (an inverted pair is swapped by the server)"),
		},
		PropertyOrder:        []string{"kind", "min", "max"},
		AdditionalProperties: noMore(),
	}
}

// stepSchema is the $defs/step definition: one step, or one repeat block of steps.
// Nested repeats reference it again, which is exactly the recursion the inference
// cannot express.
func stepSchema() *jsonschema.Schema {
	tgt := targetSchema()
	return &jsonschema.Schema{
		Type:     "object",
		Required: []string{"type"},
		Properties: map[string]*jsonschema.Schema{
			"type": {Type: "string", Enum: enumOf(workout.StepTypes),
				Description: "warmup, run, interval, recovery, rest, cooldown, or repeat (a block: set repeat and steps)"},
			"durationS": opt("number", "duration in seconds, 1 s to 6 h; give either durationS or distanceM, never both"),
			"distanceM": opt("number", "distance in metres, 1 m to 100 km; give either durationS or distanceM, never both"),
			"repeat":    opt("integer", "number of repetitions, 1 to 50; only for type repeat, and required then"),
			"steps": {Types: []string{"null", "array"}, Items: &jsonschema.Schema{Ref: "#/$defs/step"},
				Description: "the repeated steps, only for type repeat (nesting stops after two levels of repeat)"},
			"target": {Types: []string{"null", "object"}, Description: "target of the step; omit it for a free-effort step",
				Properties: tgt.Properties, Required: tgt.Required, PropertyOrder: tgt.PropertyOrder,
				AdditionalProperties: noMore()},
			"note": opt("string", "free note shown next to the step, 200 characters max (for example 'marche')"),
		},
		PropertyOrder:        []string{"type", "durationS", "distanceM", "repeat", "steps", "target", "note"},
		AdditionalProperties: noMore(),
	}
}

// planSchema returns the input schema of a write tool: the schema inferred from its
// argument struct, with the `steps` property pointed at the recursive step
// definition. typ must be planCreateIn or planUpdateIn.
func planSchema(typ reflect.Type) *jsonschema.Schema {
	s, err := jsonschema.ForType(typ, &jsonschema.ForOptions{
		// The recursive Step is never inferred: the real definition is plugged in
		// below, this placeholder only stops the inference from seeing the cycle.
		TypeSchemas: map[reflect.Type]*jsonschema.Schema{reflect.TypeFor[workout.Step](): {Type: "object"}},
	})
	if err != nil {
		panic("mcp: schéma des arguments d'écriture : " + err.Error())
	}
	steps, ok := s.Properties["steps"]
	if !ok {
		// A renamed field would silently strip steps from tools/list.
		panic("mcp: schéma des arguments d'écriture : champ « steps » absent de " + typ.String())
	}
	s.Defs = map[string]*jsonschema.Schema{"step": stepSchema()}
	steps.Items = &jsonschema.Schema{Ref: "#/$defs/step"}
	return s
}
