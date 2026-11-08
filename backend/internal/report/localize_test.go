package report

import (
	"os"
	"regexp"
	"strings"
	"testing"

	f "stride/internal/frfmt"
)

func TestEnglishPairsKeepVerbs(t *testing.T) {
	for _, p := range enPairs {
		if a, b := verb.FindAllString(p[0], -1), verb.FindAllString(p[1], -1); strings.Join(a, " ") != strings.Join(b, " ") {
			t.Errorf("verbs differ:\n fr %q %v\n en %q %v", p[0], a, p[1], b)
		}
	}
}

func TestLocalizeKeepsFrenchByteIdentical(t *testing.T) {
	for _, name := range []string{"activity_all.md", "day_all.md", "multi_all.md"} {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if Localize(string(b), f.FR) != string(b) {
			t.Errorf("%s: French output changed", name)
		}
	}
}

// Structural French words that must not survive in an English report (user data excluded).
var frenchLeft = regexp.MustCompile(`(?i)\b(séance|séances|allure|durée|sommeil|récupération|préparation|dérive|chaussures|foulée|gêne|aucune?|prévue?s?|semaine)\b|N/D`)

func TestLocalizeEnglishReports(t *testing.T) {
	for _, name := range []string{"activity_all.md", "activity_pain_plan.md", "activity_strava_only.md", "day_all.md", "multi_all.md"} {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		en := Localize(string(b), f.EN)
		if !strings.HasPrefix(en, "> **SYSTEM DIRECTIVE FOR THE AI:**") {
			t.Errorf("%s: directive not translated: %.80q", name, en)
		}
		for _, line := range strings.Split(en, "\n") {
			if strings.Contains(line, "Note privée") || strings.Contains(line, "Strava : ") {
				continue
			}
			if m := frenchLeft.FindString(line); m != "" && !strings.Contains(line, "Courir 45 min") && !strings.Contains(line, "Séance non réalisée") {
				t.Errorf("%s: French %q left in %q", name, m, line)
			}
		}
	}
	got := Localize("- **Réalisé :** 7,02 km en 42:10 | Allure: 6:00/km (GAP: 5:52/km) | Puissance: 285 W\n- **Pas :** 12 345 | **Étages :** 3 ↑ / 2 ↓ | **Intensité :** 10 mod. / N/D vig.", f.EN)
	want := "- **Done:** 7.02 km in 42:10 | Pace: 6:00/km (GAP: 5:52/km) | Power: 285 W\n- **Steps:** 12,345 | **Floors:** 3 ↑ / 2 ↓ | **Intensity:** 10 mod. / N/A vig."
	if got != want {
		t.Errorf("Localize =\n%s\nwant\n%s", got, want)
	}
	if !strings.Contains(DirectiveFor(f.EN), "Answer in English.") || DirectiveFor(f.FR) != Directive {
		t.Error("DirectiveFor")
	}
}
