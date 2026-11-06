// Package openapi generates and serves the OpenAPI 3.0.3 specification for Stride 4.0.0.
package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	once    sync.Once
	rawJSON []byte
	rawYAML []byte
)

// SpecJSON returns the generated OpenAPI 3.0.3 specification in JSON format.
func SpecJSON() []byte {
	initSpec()
	return rawJSON
}

// SpecYAML returns the generated OpenAPI 3.0.3 specification in YAML format.
func SpecYAML() []byte {
	initSpec()
	return rawYAML
}

// WriteFiles writes openapi.json and openapi.yaml to the specified directory.
func WriteFiles(dir string) error {
	initSpec()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("création du dossier %s : %w", dir, err)
	}

	jsonPath := filepath.Join(dir, "openapi.json")
	if err := os.WriteFile(jsonPath, rawJSON, 0o644); err != nil {
		return fmt.Errorf("écriture de %s : %w", jsonPath, err)
	}

	yamlPath := filepath.Join(dir, "openapi.yaml")
	if err := os.WriteFile(yamlPath, rawYAML, 0o644); err != nil {
		return fmt.Errorf("écriture de %s : %w", yamlPath, err)
	}

	return nil
}

func initSpec() {
	once.Do(func() {
		spec := buildSpec()
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		if err := enc.Encode(spec); err == nil {
			rawJSON = bytes.TrimSpace(buf.Bytes())
		}

		if y, err := yaml.Marshal(spec); err == nil {
			rawYAML = bytes.TrimSpace(y)
		}
	})
}

func buildSpec() map[string]any {
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "Stride API",
			"version":     "4.0.0",
			"description": "Stride 4.0.0 athletic analytics, cycle-based planning, anatomical body stress map, and physio share API.",
		},
		"paths": buildPaths(),
		"components": map[string]any{
			"schemas": buildSchemas(),
		},
	}
}

func buildPaths() map[string]any {
	return map[string]any{
		"/version": map[string]any{
			"get": map[string]any{
				"summary":     "Public build and runtime version identity",
				"description": "Returns version metadata without requiring authentication.",
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Build information",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/BuildInfo"},
							},
						},
					},
				},
			},
		},
		"/api/version": map[string]any{
			"get": map[string]any{
				"summary":     "Authenticated version identity",
				"description": "Returns version metadata for authenticated users.",
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Build information",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/BuildInfo"},
							},
						},
					},
				},
			},
		},
		"/api/jobs": map[string]any{
			"get": map[string]any{
				"summary": "List recent async worker jobs",
				"parameters": []map[string]any{
					{
						"name":        "limit",
						"in":          "query",
						"required":    false,
						"description": "Maximum number of jobs to return",
						"schema":      map[string]any{"type": "integer", "default": 20},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "List of jobs",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/JobListResponse"},
							},
						},
					},
				},
			},
		},
		"/api/jobs/{id}": map[string]any{
			"get": map[string]any{
				"summary": "Retrieve async job status",
				"parameters": []map[string]any{
					{
						"name":        "id",
						"in":          "path",
						"required":    true,
						"description": "Unique job ID",
						"schema":      map[string]any{"type": "string"},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Job status",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/JobStatus"},
							},
						},
					},
					"404": map[string]any{
						"description": "Job not found or owned by another user",
					},
				},
			},
		},
		"/api/sync": map[string]any{
			"post": map[string]any{
				"summary": "Trigger asynchronous or synchronous provider synchronization",
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Provider synchronization status",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/SyncStatus"},
							},
						},
					},
				},
			},
		},
		"/api/imports": map[string]any{
			"post": map[string]any{
				"summary": "Upload and parse an activity file (FIT, GPX, TCX)",
				"parameters": []map[string]any{
					{
						"name":        "async",
						"in":          "query",
						"required":    false,
						"description": "1 to enqueue processing via worker queue",
						"schema":      map[string]any{"type": "integer"},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Synchronous import result",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/ImportResult"},
							},
						},
					},
					"201": map[string]any{
						"description": "Synchronous import created",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/ImportResult"},
							},
						},
					},
					"202": map[string]any{
						"description": "Job enqueued asynchronously",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/JobStatus"},
							},
						},
					},
				},
			},
		},
		"/api/llm/analyze": map[string]any{
			"post": map[string]any{
				"summary": "Execute or stream LLM Coach analysis",
				"parameters": []map[string]any{
					{
						"name":        "async",
						"in":          "query",
						"required":    false,
						"description": "1 to stream tokens over SSE",
						"schema":      map[string]any{"type": "integer"},
					},
				},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/LLMAnalyzeRequest"},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Full analysis generated",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/LLMAnalysis"},
							},
						},
					},
					"202": map[string]any{
						"description": "Job enqueued asynchronously",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/JobStatus"},
							},
						},
					},
				},
			},
		},
		"/api/plan/generate": map[string]any{
			"post": map[string]any{
				"summary": "Generate training plan draft sessions",
				"parameters": []map[string]any{
					{
						"name":     "async",
						"in":       "query",
						"required": false,
						"schema":   map[string]any{"type": "integer"},
					},
				},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/PlanGenerateRequest"},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Plan prompt or structured drafts",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"oneOf": []any{
										map[string]any{"$ref": "#/components/schemas/PlanPromptResponse"},
										map[string]any{"$ref": "#/components/schemas/PlanDraftsResponse"},
									},
								},
							},
						},
					},
					"202": map[string]any{
						"description": "Generation enqueued",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/JobStatus"},
							},
						},
					},
				},
			},
		},
		"/api/plan/adapt": map[string]any{
			"post": map[string]any{
				"summary": "Adapt remaining planned sessions following events",
				"parameters": []map[string]any{
					{
						"name":     "async",
						"in":       "query",
						"required": false,
						"schema":   map[string]any{"type": "integer"},
					},
				},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/PlanAdaptRequest"},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Plan adapt preview",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/PlanAdaptPreview"},
							},
						},
					},
					"202": map[string]any{
						"description": "Job enqueued",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/JobStatus"},
							},
						},
					},
				},
			},
		},
		"/api/cycles": map[string]any{
			"get": map[string]any{
				"summary": "Retrieve macrocycle, mesocycles, and microcycles overview",
				"parameters": []map[string]any{
					{
						"name":        "planId",
						"in":          "query",
						"required":    false,
						"description": "Specific plan ID to retrieve, defaults to active plan",
						"schema":      map[string]any{"type": "integer"},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Training cycle overview",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/TrainingCycleOverview"},
							},
						},
					},
				},
			},
		},
		"/api/cycles/plans": map[string]any{
			"post": map[string]any{
				"summary": "Create a macrocycle plan with optional auto-scaffolding",
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/TrainingPlanInput"},
						},
					},
				},
				"responses": map[string]any{
					"201": map[string]any{
						"description": "Created cycle overview",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/TrainingCycleOverview"},
							},
						},
					},
				},
			},
		},
		"/api/cycles/plans/{id}": map[string]any{
			"patch": map[string]any{
				"summary": "Modify an existing macrocycle plan",
				"parameters": []map[string]any{
					{
						"name":     "id",
						"in":       "path",
						"required": true,
						"schema":   map[string]any{"type": "integer"},
					},
				},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/TrainingPlanInput"},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Updated training plan",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/TrainingPlan"},
							},
						},
					},
				},
			},
			"delete": map[string]any{
				"summary": "Delete a macrocycle plan and all subordinate phases and weeks",
				"parameters": []map[string]any{
					{
						"name":     "id",
						"in":       "path",
						"required": true,
						"schema":   map[string]any{"type": "integer"},
					},
				},
				"responses": map[string]any{
					"204": map[string]any{"description": "Plan deleted"},
				},
			},
		},
		"/api/cycles/phases": map[string]any{
			"post": map[string]any{
				"summary": "Create a mesocycle training phase",
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/TrainingPhaseInput"},
						},
					},
				},
				"responses": map[string]any{
					"201": map[string]any{
						"description": "Created training phase",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/TrainingPhase"},
							},
						},
					},
				},
			},
		},
		"/api/cycles/phases/{id}": map[string]any{
			"patch": map[string]any{
				"summary": "Modify an existing mesocycle phase",
				"parameters": []map[string]any{
					{
						"name":     "id",
						"in":       "path",
						"required": true,
						"schema":   map[string]any{"type": "integer"},
					},
				},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/TrainingPhaseInput"},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Updated phase",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/TrainingPhase"},
							},
						},
					},
				},
			},
			"delete": map[string]any{
				"summary": "Delete a mesocycle phase",
				"parameters": []map[string]any{
					{
						"name":     "id",
						"in":       "path",
						"required": true,
						"schema":   map[string]any{"type": "integer"},
					},
				},
				"responses": map[string]any{
					"204": map[string]any{"description": "Phase deleted"},
				},
			},
		},
		"/api/cycles/weeks": map[string]any{
			"put": map[string]any{
				"summary": "Upsert a microcycle weekly target or assimilation flag",
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/TrainingWeekInput"},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Upserted training week",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/TrainingWeek"},
							},
						},
					},
				},
			},
		},
		"/api/bodymap": map[string]any{
			"get": map[string]any{
				"summary": "Retrieve body stress map overview, zones, and correlations",
				"parameters": []map[string]any{
					{
						"name":        "days",
						"in":          "query",
						"required":    false,
						"description": "Time window in days (default: 90)",
						"schema":      map[string]any{"type": "integer", "default": 90},
					},
					{
						"name":        "zone",
						"in":          "query",
						"required":    false,
						"description": "Optional zone filter",
						"schema":      map[string]any{"type": "string"},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Body stress map overview",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/BodyMapOverview"},
							},
						},
					},
				},
			},
		},
		"/api/bodymap/entries": map[string]any{
			"post": map[string]any{
				"summary": "Record a new anatomical pain or tension score",
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/BodyStressInput"},
						},
					},
				},
				"responses": map[string]any{
					"201": map[string]any{
						"description": "Created body stress entry",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/BodyStressEntry"},
							},
						},
					},
				},
			},
		},
		"/api/bodymap/entries/{id}": map[string]any{
			"patch": map[string]any{
				"summary": "Update an existing body stress entry",
				"parameters": []map[string]any{
					{
						"name":     "id",
						"in":       "path",
						"required": true,
						"schema":   map[string]any{"type": "integer"},
					},
				},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/BodyStressInput"},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Updated body stress entry",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/BodyStressEntry"},
							},
						},
					},
				},
			},
			"delete": map[string]any{
				"summary": "Delete a body stress entry",
				"parameters": []map[string]any{
					{
						"name":     "id",
						"in":       "path",
						"required": true,
						"schema":   map[string]any{"type": "integer"},
					},
				},
				"responses": map[string]any{
					"204": map[string]any{"description": "Entry deleted"},
				},
			},
		},
		"/api/share": map[string]any{
			"get": map[string]any{
				"summary": "List all read-only physio and coach share links",
				"responses": map[string]any{
					"200": map[string]any{
						"description": "List of share links",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":  "array",
									"items": map[string]any{"$ref": "#/components/schemas/ShareLink"},
								},
							},
						},
					},
				},
			},
			"post": map[string]any{
				"summary": "Generate an encrypted read-only share link",
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/ShareLinkInput"},
						},
					},
				},
				"responses": map[string]any{
					"201": map[string]any{
						"description": "Generated share link",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/ShareLink"},
							},
						},
					},
				},
			},
		},
		"/api/share/{id}": map[string]any{
			"delete": map[string]any{
				"summary": "Instantly revoke a share link",
				"parameters": []map[string]any{
					{
						"name":     "id",
						"in":       "path",
						"required": true,
						"schema":   map[string]any{"type": "integer"},
					},
				},
				"responses": map[string]any{
					"204": map[string]any{"description": "Link revoked"},
				},
			},
		},
		"/api/public/share/{token}": map[string]any{
			"get": map[string]any{
				"summary":     "Public clinical share view (read-only, no auth required)",
				"description": "Exposes sanitized clinical load, cycles, and body map without credentials or raw GPS tracks.",
				"parameters": []map[string]any{
					{
						"name":        "token",
						"in":          "path",
						"required":    true,
						"description": "Cryptographically secure share token",
						"schema":      map[string]any{"type": "string"},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Sanitized public share view",
						"headers": map[string]any{
							"Cache-Control": map[string]any{
								"schema":      map[string]any{"type": "string"},
								"description": "private, no-store",
							},
						},
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/PublicShareView"},
							},
						},
					},
					"404": map[string]any{"description": "Link not found or revoked"},
					"410": map[string]any{"description": "Link expired"},
				},
			},
		},
	}
}

func buildSchemas() map[string]any {
	return map[string]any{
		"BuildInfo": map[string]any{
			"type":     "object",
			"required": []string{"version", "commit", "builtAt", "go", "label"},
			"properties": map[string]any{
				"version": map[string]any{"type": "string"},
				"commit":  map[string]any{"type": "string"},
				"builtAt": map[string]any{"type": "string"},
				"go":      map[string]any{"type": "string"},
				"label":   map[string]any{"type": "string"},
			},
		},
		"JobStatus": map[string]any{
			"type":     "object",
			"required": []string{"id", "userId", "kind", "queue", "state", "label", "attempts", "maxAttempts", "createdAt", "updatedAt"},
			"properties": map[string]any{
				"id":          map[string]any{"type": "string"},
				"userId":      map[string]any{"type": "integer"},
				"kind":        map[string]any{"type": "string", "enum": []string{"sync.provider", "sync.strava_webhook", "sync.resync", "import.activity", "llm.analyze", "llm.plan_generate", "llm.plan_adapt", "webhook.deliver"}},
				"queue":       map[string]any{"type": "string", "enum": []string{"critical", "default", "low"}},
				"state":       map[string]any{"type": "string", "enum": []string{"queued", "running", "retrying", "completed", "failed"}},
				"label":       map[string]any{"type": "string"},
				"progressPct": map[string]any{"type": "number", "nullable": true},
				"processed":   map[string]any{"type": "integer", "nullable": true},
				"total":       map[string]any{"type": "integer", "nullable": true},
				"attempts":    map[string]any{"type": "integer"},
				"maxAttempts": map[string]any{"type": "integer"},
				"error":       map[string]any{"type": "string", "nullable": true},
				"result":      map[string]any{"type": "object", "nullable": true},
				"createdAt":   map[string]any{"type": "string"},
				"updatedAt":   map[string]any{"type": "string"},
			},
		},
		"JobListResponse": map[string]any{
			"type":     "object",
			"required": []string{"items"},
			"properties": map[string]any{
				"items": map[string]any{
					"type":  "array",
					"items": map[string]any{"$ref": "#/components/schemas/JobStatus"},
				},
			},
		},
		"SyncStatus": map[string]any{
			"type":     "object",
			"required": []string{"running"},
			"properties": map[string]any{
				"running":       map[string]any{"type": "boolean"},
				"jobId":         map[string]any{"type": "string", "nullable": true},
				"lastSyncAt":    map[string]any{"type": "string", "nullable": true},
				"lastLiveAt":    map[string]any{"type": "string", "nullable": true},
				"cooldownUntil": map[string]any{"type": "string", "nullable": true},
			},
		},
		"ImportResult": map[string]any{
			"type":     "object",
			"required": []string{"activityId", "merged"},
			"properties": map[string]any{
				"activityId": map[string]any{"type": "integer"},
				"merged":     map[string]any{"type": "boolean"},
			},
		},
		"LLMAnalysis": map[string]any{
			"type":     "object",
			"required": []string{"id", "kind", "ref", "provider", "model", "markdown", "promptTokensEstimate", "durationMs", "createdAt"},
			"properties": map[string]any{
				"id":                   map[string]any{"type": "integer"},
				"kind":                 map[string]any{"type": "string"},
				"ref":                  map[string]any{"type": "string"},
				"provider":             map[string]any{"type": "string"},
				"model":                map[string]any{"type": "string"},
				"markdown":             map[string]any{"type": "string"},
				"promptTokensEstimate": map[string]any{"type": "integer"},
				"durationMs":           map[string]any{"type": "integer"},
				"createdAt":            map[string]any{"type": "string"},
			},
		},
		"LLMAnalyzeRequest": map[string]any{
			"type":     "object",
			"required": []string{"kind"},
			"properties": map[string]any{
				"kind":       map[string]any{"type": "string"},
				"activityId": map[string]any{"type": "integer"},
				"date":       map[string]any{"type": "string"},
				"question":   map[string]any{"type": "string"},
			},
		},
		"PlanGenerateRequest": map[string]any{
			"type":     "object",
			"required": []string{"horizon", "startDate", "endDate"},
			"properties": map[string]any{
				"horizon":    map[string]any{"type": "string"},
				"startDate":  map[string]any{"type": "string"},
				"endDate":    map[string]any{"type": "string"},
				"goalId":     map[string]any{"type": "integer"},
				"mode":       map[string]any{"type": "string"},
				"withPhases": map[string]any{"type": "boolean"},
			},
		},
		"PlanPromptResponse": map[string]any{
			"type":     "object",
			"required": []string{"prompt", "tokensEstimate"},
			"properties": map[string]any{
				"prompt":         map[string]any{"type": "string"},
				"tokensEstimate": map[string]any{"type": "integer"},
			},
		},
		"PlanDraftsResponse": map[string]any{
			"type":     "object",
			"required": []string{"sessions"},
			"properties": map[string]any{
				"sessions": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			},
		},
		"PlanAdaptRequest": map[string]any{
			"type":     "object",
			"required": []string{"from", "mode"},
			"properties": map[string]any{
				"from":    map[string]any{"type": "string"},
				"mode":    map[string]any{"type": "string"},
				"eventId": map[string]any{"type": "integer"},
			},
		},
		"PlanAdaptPreview": map[string]any{
			"type":     "object",
			"required": []string{"removeIds", "sessions"},
			"properties": map[string]any{
				"removeIds": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
				"sessions":  map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			},
		},
		"TrainingCycleOverview": map[string]any{
			"type":     "object",
			"required": []string{"plans", "phases", "standaloneWeeks", "totals"},
			"properties": map[string]any{
				"plans":           map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/TrainingPlan"}},
				"activePlan":      map[string]any{"$ref": "#/components/schemas/TrainingPlan", "nullable": true},
				"phases":          map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/TrainingPhase"}},
				"standaloneWeeks": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/TrainingWeek"}},
				"totals":          map[string]any{"type": "object"},
			},
		},
		"TrainingPlan": map[string]any{
			"type":     "object",
			"required": []string{"id", "name", "sport", "startDate", "endDate", "targetEventLabel", "status", "note", "createdAt", "updatedAt"},
			"properties": map[string]any{
				"id":               map[string]any{"type": "integer"},
				"goalId":           map[string]any{"type": "integer", "nullable": true},
				"name":             map[string]any{"type": "string"},
				"sport":            map[string]any{"type": "string", "enum": []string{"run", "bike", "other"}},
				"startDate":        map[string]any{"type": "string"},
				"endDate":          map[string]any{"type": "string"},
				"targetEventLabel": map[string]any{"type": "string"},
				"status":           map[string]any{"type": "string", "enum": []string{"active", "completed", "archived"}},
				"note":             map[string]any{"type": "string"},
				"createdAt":        map[string]any{"type": "string"},
				"updatedAt":        map[string]any{"type": "string"},
			},
		},
		"TrainingPlanInput": map[string]any{
			"type":     "object",
			"required": []string{"name", "startDate", "endDate"},
			"properties": map[string]any{
				"name":               map[string]any{"type": "string"},
				"goalId":             map[string]any{"type": "integer", "nullable": true},
				"sport":              map[string]any{"type": "string", "enum": []string{"run", "bike", "other"}},
				"startDate":          map[string]any{"type": "string"},
				"endDate":            map[string]any{"type": "string"},
				"targetEventLabel":   map[string]any{"type": "string"},
				"status":             map[string]any{"type": "string", "enum": []string{"active", "completed", "archived"}},
				"note":               map[string]any{"type": "string"},
				"autoScaffold":       map[string]any{"type": "boolean"},
				"peakWeeklyVolumeKm": map[string]any{"type": "number", "nullable": true},
				"peakWeeklyLoad":     map[string]any{"type": "number", "nullable": true},
			},
		},
		"TrainingPhase": map[string]any{
			"type":     "object",
			"required": []string{"id", "name", "phaseType", "orderIndex", "startDate", "endDate", "focus", "note", "actuals", "weeks"},
			"properties": map[string]any{
				"id":                    map[string]any{"type": "integer"},
				"planId":                map[string]any{"type": "integer", "nullable": true},
				"name":                  map[string]any{"type": "string"},
				"phaseType":             map[string]any{"type": "string", "enum": []string{"foundation", "build", "threshold", "specific", "assimilation", "taper", "recovery"}},
				"orderIndex":            map[string]any{"type": "integer"},
				"startDate":             map[string]any{"type": "string"},
				"endDate":               map[string]any{"type": "string"},
				"focus":                 map[string]any{"type": "string"},
				"weeklyVolumeKm":        map[string]any{"type": "number", "nullable": true},
				"targetWeeklyLoad":      map[string]any{"type": "number", "nullable": true},
				"targetWeeklyDurationS": map[string]any{"type": "number", "nullable": true},
				"note":                  map[string]any{"type": "string"},
				"actuals":               map[string]any{"type": "object"},
				"weeks":                 map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/TrainingWeek"}},
			},
		},
		"TrainingPhaseInput": map[string]any{
			"type":     "object",
			"required": []string{"name", "startDate", "endDate"},
			"properties": map[string]any{
				"planId":                map[string]any{"type": "integer", "nullable": true},
				"name":                  map[string]any{"type": "string"},
				"phaseType":             map[string]any{"type": "string", "enum": []string{"foundation", "build", "threshold", "specific", "assimilation", "taper", "recovery"}},
				"orderIndex":            map[string]any{"type": "integer"},
				"startDate":             map[string]any{"type": "string"},
				"endDate":               map[string]any{"type": "string"},
				"focus":                 map[string]any{"type": "string"},
				"weeklyVolumeKm":        map[string]any{"type": "number", "nullable": true},
				"targetWeeklyLoad":      map[string]any{"type": "number", "nullable": true},
				"targetWeeklyDurationS": map[string]any{"type": "number", "nullable": true},
				"note":                  map[string]any{"type": "string"},
			},
		},
		"TrainingWeek": map[string]any{
			"type":     "object",
			"required": []string{"id", "weekStart", "weekEnd", "weekNumber", "isAssimilation", "note", "actuals", "workouts"},
			"properties": map[string]any{
				"id":              map[string]any{"type": "integer"},
				"planId":          map[string]any{"type": "integer", "nullable": true},
				"phaseId":         map[string]any{"type": "integer", "nullable": true},
				"weekStart":       map[string]any{"type": "string"},
				"weekEnd":         map[string]any{"type": "string"},
				"weekNumber":      map[string]any{"type": "integer"},
				"isAssimilation":  map[string]any{"type": "boolean"},
				"targetVolumeKm":  map[string]any{"type": "number", "nullable": true},
				"targetLoad":      map[string]any{"type": "number", "nullable": true},
				"targetDurationS": map[string]any{"type": "number", "nullable": true},
				"note":            map[string]any{"type": "string"},
				"actuals":         map[string]any{"type": "object"},
				"workouts":        map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			},
		},
		"TrainingWeekInput": map[string]any{
			"type":     "object",
			"required": []string{"weekStart"},
			"properties": map[string]any{
				"planId":          map[string]any{"type": "integer", "nullable": true},
				"phaseId":         map[string]any{"type": "integer", "nullable": true},
				"weekStart":       map[string]any{"type": "string"},
				"isAssimilation":  map[string]any{"type": "boolean"},
				"targetVolumeKm":  map[string]any{"type": "number", "nullable": true},
				"targetLoad":      map[string]any{"type": "number", "nullable": true},
				"targetDurationS": map[string]any{"type": "number", "nullable": true},
				"note":            map[string]any{"type": "string"},
			},
		},
		"BodyMapOverview": map[string]any{
			"type":     "object",
			"required": []string{"zones", "entries", "correlations", "windowDays"},
			"properties": map[string]any{
				"zones":        map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
				"entries":      map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/BodyStressEntry"}},
				"correlations": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
				"windowDays":   map[string]any{"type": "integer"},
			},
		},
		"BodyStressEntry": map[string]any{
			"type":     "object",
			"required": []string{"id", "date", "zone", "zoneLabel", "side", "painScore", "note", "createdAt", "updatedAt"},
			"properties": map[string]any{
				"id":               map[string]any{"type": "integer"},
				"date":             map[string]any{"type": "string"},
				"activityId":       map[string]any{"type": "integer", "nullable": true},
				"activityName":     map[string]any{"type": "string", "nullable": true},
				"zone":             map[string]any{"type": "string", "enum": []string{"knee_pfps", "achilles", "plantar_fascia", "shin_splints", "iliotibial_band", "hamstrings_calves"}},
				"zoneLabel":        map[string]any{"type": "string"},
				"side":             map[string]any{"type": "string", "enum": []string{"left", "right", "both"}},
				"painScore":        map[string]any{"type": "number"},
				"coldNextDayScore": map[string]any{"type": "number", "nullable": true},
				"note":             map[string]any{"type": "string"},
				"createdAt":        map[string]any{"type": "string"},
				"updatedAt":        map[string]any{"type": "string"},
			},
		},
		"BodyStressInput": map[string]any{
			"type":     "object",
			"required": []string{"date", "zone", "painScore"},
			"properties": map[string]any{
				"date":             map[string]any{"type": "string"},
				"activityId":       map[string]any{"type": "integer", "nullable": true},
				"zone":             map[string]any{"type": "string", "enum": []string{"knee_pfps", "achilles", "plantar_fascia", "shin_splints", "iliotibial_band", "hamstrings_calves"}},
				"side":             map[string]any{"type": "string", "enum": []string{"left", "right", "both"}},
				"painScore":        map[string]any{"type": "number"},
				"coldNextDayScore": map[string]any{"type": "number", "nullable": true},
				"note":             map[string]any{"type": "string"},
			},
		},
		"ShareLink": map[string]any{
			"type":     "object",
			"required": []string{"id", "label", "tokenPrefix", "url", "revoked", "accessCount", "createdAt"},
			"properties": map[string]any{
				"id":             map[string]any{"type": "integer"},
				"label":          map[string]any{"type": "string"},
				"tokenPrefix":    map[string]any{"type": "string"},
				"url":            map[string]any{"type": "string"},
				"expiresAt":      map[string]any{"type": "string", "nullable": true},
				"revoked":        map[string]any{"type": "boolean"},
				"revokedAt":      map[string]any{"type": "string", "nullable": true},
				"accessCount":    map[string]any{"type": "integer"},
				"lastAccessedAt": map[string]any{"type": "string", "nullable": true},
				"createdAt":      map[string]any{"type": "string"},
			},
		},
		"ShareLinkInput": map[string]any{
			"type":     "object",
			"required": []string{"label"},
			"properties": map[string]any{
				"label":         map[string]any{"type": "string"},
				"expiresInDays": map[string]any{"type": "integer", "nullable": true},
				"expiresAt":     map[string]any{"type": "string", "nullable": true},
			},
		},
		"PublicShareView": map[string]any{
			"type":     "object",
			"required": []string{"generatedAt", "shareLabel", "athlete", "readiness", "loadSummary", "loadSeries", "cycles", "recentSessions", "bodyMap", "shoes"},
			"properties": map[string]any{
				"generatedAt":    map[string]any{"type": "string"},
				"shareLabel":     map[string]any{"type": "string"},
				"expiresAt":      map[string]any{"type": "string", "nullable": true},
				"athlete":        map[string]any{"type": "object"},
				"readiness":      map[string]any{"type": "object"},
				"loadSummary":    map[string]any{"type": "object"},
				"loadSeries":     map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
				"cycles":         map[string]any{"$ref": "#/components/schemas/TrainingCycleOverview"},
				"recentSessions": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
				"bodyMap":        map[string]any{"$ref": "#/components/schemas/BodyMapOverview"},
				"shoes":          map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			},
		},
	}
}
