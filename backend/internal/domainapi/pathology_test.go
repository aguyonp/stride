package domainapi_test

import (
	"fmt"
	"strings"
	"testing"
)

func TestHealthConditionsCatalogAndStatusRoundTrip(t *testing.T) {
	e := setup(t)
	var cat m
	if code, body := e.do(2, "GET", "/api/health/conditions", nil, &cat); code != 200 || len(cat["zones"].([]any)) != 10 {
		t.Fatalf("catalog: %d %s", code, body)
	}
	ankle := cat["zones"].([]any)[6].(m)
	if ankle["zone"] != "ankle" || ankle["zoneLabel"] != "Cheville" || !strings.Contains(fmt.Sprint(ankle["conditions"]), "Fracture de cheville (malléole)") {
		t.Fatalf("ankle: %v", ankle)
	}

	in := m{"mode": "tracking", "zones": []m{
		{"zone": "knee_pfps", "side": "both", "since": "2026-09-01", "condition": "other", "customLabel": "  Plica synoviale  "},
		{"zone": "ankle", "side": "left", "since": "2026-10-12", "condition": "ankle_fracture", "customLabel": "ignored", "conditionLabel": "forged"},
	}}
	var st m
	if code, body := e.do(2, "PUT", "/api/health/status", in, &st); code != 200 || st["focusArea"] != "other" {
		t.Fatalf("put: %d %s", code, body)
	}
	zones := st["zones"].([]any)
	knee, fracture := zones[0].(m), zones[1].(m)
	if knee["condition"] != "other" || knee["customLabel"] != "Plica synoviale" || knee["conditionLabel"] != "Plica synoviale" ||
		fracture["condition"] != "ankle_fracture" || fracture["customLabel"] != nil || fracture["conditionLabel"] != "Fracture de cheville (malléole)" {
		t.Fatalf("round trip: %v", zones)
	}
	var raw string
	_ = e.st.DB.QueryRow(`SELECT zones FROM health_status WHERE user_id = 2`).Scan(&raw)
	if strings.Contains(raw, "conditionLabel") || strings.Contains(raw, "forged") || strings.Contains(raw, "ignored") {
		t.Fatalf("derived or ignored fields stored: %s", raw)
	}
	var prof m
	if e.do(2, "GET", "/api/profile", nil, &prof); prof["focusArea"] != "other" || prof["focusLabel"] != "Fracture de cheville (malléole)" || prof["injuredSide"] != "left" {
		t.Fatalf("derived profile: %v", prof)
	}
	// a condition with a dedicated preset keeps it; « other » takes the custom label
	e.do(2, "PUT", "/api/health/status", m{"mode": "tracking", "zones": []m{{"zone": "shin_splints", "side": "right", "condition": "medial_tibial_stress"}}}, nil)
	if e.do(2, "GET", "/api/profile", nil, &prof); prof["focusArea"] != "shin_splints" {
		t.Fatalf("preset condition: %v", prof)
	}
	e.do(2, "PUT", "/api/health/status", m{"mode": "tracking", "zones": []m{{"zone": "hip", "side": "left", "condition": "other", "customLabel": "Conflit fémoro-acétabulaire"}}}, nil)
	if e.do(2, "GET", "/api/profile", nil, &prof); prof["focusArea"] != "other" || prof["focusLabel"] != "Conflit fémoro-acétabulaire" {
		t.Fatalf("custom condition: %v", prof)
	}

	for _, bad := range []m{
		{"zone": "ankle", "condition": "patellofemoral_pain"},
		{"zone": "ankle", "condition": "broken"},
		{"zone": "ankle", "condition": "other"},
		{"zone": "ankle", "condition": "other", "customLabel": "   "},
		{"zone": "ankle", "condition": "other", "customLabel": strings.Repeat("x", 81)},
		{"zone": "ankle", "condition": "other", "customLabel": "deux\nlignes"},
	} {
		bad["side"] = "left"
		if code, _ := e.do(2, "PUT", "/api/health/status", m{"mode": "tracking", "zones": []m{bad}}, nil); code != 400 {
			t.Fatalf("%v accepted: %d", bad, code)
		}
	}
}

func TestNewZonesInBodyMapAndPrompts(t *testing.T) {
	e := setup(t)
	bootGarmin(t, e)
	runID := e.id(`SELECT id FROM activities WHERE garmin_id = 1001`)
	if code, body := e.do(1, "PUT", "/api/health/status", m{"mode": "tracking", "zones": []m{
		{"zone": "ankle", "side": "left", "since": "2026-10-12", "condition": "ankle_fracture"},
	}}, nil); code != 200 {
		t.Fatalf("put: %d %s", code, body)
	}
	if code, body := e.do(1, "POST", "/api/bodymap/entries", m{"date": e.fx.day(-1), "activityId": runID, "zone": "ankle", "side": "left", "painScore": 3}, nil); code != 201 {
		t.Fatalf("ankle entry: %d %s", code, body)
	}
	var ov m
	if code, _ := e.do(1, "GET", "/api/bodymap?days=3650&zone=ankle", nil, &ov); code != 200 || len(ov["entries"].([]any)) != 1 || len(ov["zones"].([]any)) != 10 {
		t.Fatalf("bodymap: %v", ov)
	}
	// a single symptomatic run: the measured correlation stays « insufficient », never estimated
	for _, c := range ov["correlations"].([]any) {
		if c := c.(m); c["zone"] == "ankle" && (c["insufficientData"] != true || c["summary"] != nil || c["factor"] != "elev_loss") {
			t.Fatalf("ankle correlation: %v", c)
		}
	}
	want := "- **Suivi santé :** Fracture de cheville (malléole) — Cheville, gauche, depuis le 12/10/2026"
	if _, md := e.do(1, "GET", fmt.Sprintf("/api/activities/%d/report", runID), nil, nil); !strings.Contains(md, want) || !strings.Contains(md, "Suivi Pathologie (Fracture de cheville (malléole))") {
		t.Fatalf("activity prompt:\n%s", md)
	}
	if _, md := e.do(1, "GET", "/api/days/"+e.fx.day(-1)+"/report", nil, nil); !strings.Contains(md, want) {
		t.Fatalf("day prompt:\n%s", md)
	}
	var multi m
	if code, body := e.do(1, "POST", "/api/exports/multi", m{"weeks": 2}, &multi); code != 200 || !strings.Contains(multi["markdown"].(string), want) {
		t.Fatalf("multi prompt: %d %s", code, body)
	}
}
