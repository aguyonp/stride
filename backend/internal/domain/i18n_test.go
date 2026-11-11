package domain

import (
	"context"
	"strings"
	"testing"

	"stride/internal/analysis"
	"stride/internal/athlete"
	"stride/internal/frfmt"
	"stride/internal/report"
)

func TestEffectiveLanguage(t *testing.T) {
	s := &Services{}
	ctx := frfmt.WithLang(context.Background(), frfmt.FR)
	if got := s.lang(ctx, athlete.Profile{Language: strp("en")}); got != frfmt.EN {
		t.Errorf("profile language must win, got %s", got)
	}
	if got := s.lang(ctx, athlete.Profile{}); got != frfmt.FR {
		t.Errorf("Accept-Language fallback, got %s", got)
	}
	if got := s.lang(context.Background(), athlete.Profile{}); got != frfmt.EN {
		t.Errorf("default must be en, got %s", got)
	}
}

func TestPromptLanguageCarriedIntoJobs(t *testing.T) {
	fr := report.Directive + "\n\n### SÉANCE PRÉVUE"
	en := report.Localize(fr, frfmt.EN)
	if promptLang(fr) != frfmt.FR || promptLang(en) != frfmt.EN {
		t.Fatal("promptLang")
	}
	if !strings.HasPrefix(systemDirective(en), "**SYSTEM DIRECTIVE FOR THE AI:**") || !strings.HasPrefix(systemDirective(fr), "**DIRECTIVE SYSTÈME") {
		t.Errorf("systemDirective: %.60q / %.60q", systemDirective(en), systemDirective(fr))
	}
}

func TestPublicShareLabelsFollowAcceptLanguage(t *testing.T) {
	area, label := athlete.FocusKnee, athlete.PresetFor(athlete.FocusKnee).Label
	summary := "Dénivelé négatif (D−) : 172 m en moyenne sur les 4 sorties liées à la gêne, contre 52 m sur les 16 sorties."
	mk := func() *PublicShareView {
		return &PublicShareView{
			Athlete:   PublicAthlete{FocusArea: &area, FocusLabel: &label},
			Readiness: &analysis.ReadinessBanner{Title: "Vigilance : signaux de fatigue", Reasons: []string{"Ratio de charge élevé (1,42 > 1,3)"}},
			BodyMap: &analysis.BodyMapOverview{Zones: []analysis.BodyZoneSummary{{Zone: "knee_pfps", ZoneLabel: "Rotule / Genou (SFP)"}},
				Correlations: []analysis.BodyCorrelationFactor{{Zone: "knee_pfps", FactorLabel: "Dénivelé négatif (D−)", Summary: &summary}}},
		}
	}
	if fr := localizePublicShare(mk(), frfmt.FR); *fr.Athlete.FocusLabel != label || fr.Readiness.Title != "Vigilance : signaux de fatigue" {
		t.Error("French view must stay untouched")
	}
	en := localizePublicShare(mk(), frfmt.EN)
	if *en.Athlete.FocusLabel != "Knee / patellofemoral pain" || en.Readiness.Title != "Caution: signs of fatigue" || en.Readiness.Reasons[0] != "High load ratio (1.42 > 1.3)" {
		t.Errorf("English labels: %q · %q · %q", *en.Athlete.FocusLabel, en.Readiness.Title, en.Readiness.Reasons[0])
	}
	if en.BodyMap.Zones[0].ZoneLabel != "Kneecap / knee (PFPS)" || en.BodyMap.Correlations[0].FactorLabel != "Elevation loss" ||
		!strings.HasPrefix(*en.BodyMap.Correlations[0].Summary, "Elevation loss: 172 m on average over the") {
		t.Errorf("English body map: %+v / %q", en.BodyMap.Zones[0], *en.BodyMap.Correlations[0].Summary)
	}
}

func TestConditionCatalogLanguages(t *testing.T) {
	fr, en := analysis.ConditionsIn(frfmt.FR), analysis.ConditionsIn(frfmt.EN)
	if len(fr.Zones) != len(en.Zones) {
		t.Fatal("zone count")
	}
	for i, z := range en.Zones {
		if len(z.Conditions) != len(fr.Zones[i].Conditions) {
			t.Fatalf("%s: condition count", z.Zone)
		}
		for j, c := range z.Conditions {
			if c.Key != fr.Zones[i].Conditions[j].Key || c.Label == fr.Zones[i].Conditions[j].Label && c.Key != "other" && !strings.EqualFold(c.Label, "Sciatique") {
				if c.Label == fr.Zones[i].Conditions[j].Label {
					t.Errorf("%s/%s not translated: %q", z.Zone, c.Key, c.Label)
				}
			}
		}
	}
	if fr.Zones[0].Conditions[0].Label == en.Zones[0].Conditions[0].Label {
		t.Error("labels must differ between languages")
	}
}

func TestProfileLanguageNormalize(t *testing.T) {
	p := athlete.DefaultProfile("Europe/Paris")
	p.Language = strp(" EN ")
	if err := p.Normalize(); err != nil || *p.Language != "en" {
		t.Fatalf("normalize: %v %v", err, p.Language)
	}
	p.Language = strp("de")
	if err := p.Normalize(); err == nil {
		t.Error("de must be rejected")
	}
	p.Language = strp("")
	if err := p.Normalize(); err != nil || p.Language != nil {
		t.Error("empty language means null")
	}
}
