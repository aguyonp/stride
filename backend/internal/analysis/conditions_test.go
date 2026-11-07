package analysis

import (
	"strings"
	"testing"

	"stride/internal/athlete"
)

func TestConditionCatalog(t *testing.T) {
	cat := Conditions()
	if len(cat.Zones) != len(AllBodyZones) {
		t.Fatalf("zones: %d", len(cat.Zones))
	}
	seen := map[string]BodyZone{}
	for i, z := range cat.Zones {
		if z.Zone != AllBodyZones[i] || z.ZoneLabel == "" || z.ZoneLabel == string(z.Zone) {
			t.Fatalf("zone %d: %+v", i, z)
		}
		if len(z.Conditions) < 2 || z.Conditions[len(z.Conditions)-1].Key != ConditionOther {
			t.Fatalf("%s: « Autre » must close the list: %+v", z.Zone, z.Conditions)
		}
		for _, c := range z.Conditions[:len(z.Conditions)-1] {
			if prev, dup := seen[c.Key]; dup || c.Key == "" || c.Label == "" || c.Key == ConditionOther {
				t.Fatalf("%s: bad or duplicated key %q (also in %s)", z.Zone, c.Key, prev)
			}
			seen[c.Key] = z.Zone
		}
	}
	if seen["ankle_fracture"] != ZoneAnkle || seen["ankle_sprain"] != ZoneAnkle || seen["metatarsal_stress_fracture"] != ZoneForefoot ||
		seen["pubalgia"] != ZoneHip || seen["low_back_pain"] != ZoneLowerBack || seen["plantar_fasciitis"] != ZonePlantarFascia {
		t.Fatalf("expected conditions missing: %v", seen)
	}
	if !ValidCondition(ZoneAnkle, "ankle_fracture") || ValidCondition(ZoneKneePFPS, "ankle_fracture") || ValidCondition(ZoneAnkle, "elbow") || !ValidCondition(ZoneHip, ConditionOther) {
		t.Fatal("ValidCondition")
	}
	for key, want := range map[string]string{"patellofemoral_pain": athlete.FocusKnee, "achilles_tendinopathy": athlete.FocusAchilles,
		"medial_tibial_stress": athlete.FocusShin, "ankle_fracture": "", "meniscus_tear": ""} {
		if got := ConditionPreset(seen[key], key); got != want {
			t.Fatalf("preset %s: %q", key, got)
		}
	}
}

func TestCustomLabelAndPromptText(t *testing.T) {
	for in, ok := range map[string]bool{"  Plica synoviale  ": true, "": false, "   ": false, strings.Repeat("é", 80): true,
		strings.Repeat("é", 81): false, "ligne\nsuivante": false, "<b>gras</b>": true} {
		if _, got := NormalizeCustomLabel(in); got != ok {
			t.Fatalf("%q: %v", in, got)
		}
	}
	if s, _ := NormalizeCustomLabel("  Plica  "); s != "Plica" {
		t.Fatalf("trim: %q", s)
	}
	since := "2026-10-12"
	fracture := HealthZone{Zone: ZoneAnkle, Side: SideLeft, Since: &since, Condition: "ankle_fracture"}
	if got := HealthZoneText(fracture); got != "Fracture de cheville (malléole) — Cheville, gauche, depuis le 12/10/2026" {
		t.Fatalf("text: %q", got)
	}
	custom := HealthZone{Zone: ZoneKneePFPS, Side: SideBoth, Condition: ConditionOther, CustomLabel: "Plica synoviale"}
	if got := HealthZonesText([]HealthZone{fracture, custom, {Zone: ZoneHip, Side: SideRight}}); got !=
		"Fracture de cheville (malléole) — Cheville, gauche, depuis le 12/10/2026 ; Plica synoviale — Rotule / Genou (fémoro-patellaire), bilatéral ; Hanche / Bassin, droite" {
		t.Fatalf("joined: %q", got)
	}
	if ConditionLabelOf(HealthZone{Zone: ZoneKneePFPS, Condition: "ankle_fracture"}) != "" {
		t.Fatal("a condition of another zone has no label")
	}
}
