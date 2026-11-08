package frfmt

import (
	"context"
	"testing"
)

func TestLangResolution(t *testing.T) {
	en, fr := "en", "fr"
	cases := []struct {
		profile *string
		accept  string
		want    Lang
	}{
		{&fr, "en-GB,en;q=0.9", FR},
		{&en, "fr-FR", EN},
		{nil, "fr-FR,fr;q=0.9,en;q=0.8", FR},
		{nil, "de-DE, en-US;q=0.7", EN},
		{nil, "de-DE", EN},
		{nil, "", EN},
		{nil, "fr;q=0, en", EN},
	}
	for _, c := range cases {
		if got := Resolve(c.profile, FromAcceptLanguage(c.accept)); got != c.want {
			t.Errorf("Resolve(%v, %q) = %s, want %s", c.profile, c.accept, got, c.want)
		}
	}
	if got := LangFrom(WithLang(context.Background(), FR)); got != FR {
		t.Errorf("LangFrom = %q", got)
	}
}

func TestEnglishFormatter(t *testing.T) {
	in := Int(12345) + " · " + Dec(7.025, 2) + " km · 64 % · " + ND + " · 05/11 · 1:02:10"
	if got, want := English(in), "12,345 · 7.03 km · 64% · N/A · 05/11 · 1:02:10"; got != want {
		t.Errorf("English(%q) = %q, want %q", in, got, want)
	}
	if got := Format(FR, Dec(1.5, 1)); got != "1,5" {
		t.Errorf("Format(FR) = %q", got)
	}
}
