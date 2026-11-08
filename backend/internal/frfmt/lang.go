package frfmt

import (
	"context"
	"regexp"
	"strings"
)

// Lang is the output language of prompts, reports and labels (§15.2): fr or en.
type Lang string

const (
	FR Lang = "fr"
	EN Lang = "en"
)

// ParseLang accepts "fr"/"en" and any regional variant ("fr-FR", "en_GB").
func ParseLang(s string) (Lang, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "fr" || strings.HasPrefix(s, "fr-") || strings.HasPrefix(s, "fr_"):
		return FR, true
	case s == "en" || strings.HasPrefix(s, "en-") || strings.HasPrefix(s, "en_"):
		return EN, true
	}
	return "", false
}

// FromAcceptLanguage returns the first fr/en tag of an Accept-Language header (by order, q
// weights above zero), "" when none is supported.
func FromAcceptLanguage(h string) Lang {
	for _, part := range strings.Split(h, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if q := strings.TrimSpace(params); q == "q=0" || q == "q=0.0" || q == "q=0.000" {
			continue
		}
		if l, ok := ParseLang(tag); ok {
			return l
		}
	}
	return ""
}

// Resolve applies §15.2: profile.language ?? Accept-Language ?? en.
func Resolve(profile *string, accept Lang) Lang {
	if profile != nil {
		if l, ok := ParseLang(*profile); ok {
			return l
		}
	}
	if accept != "" {
		return accept
	}
	return EN
}

type langKey struct{}

// WithLang stores the request's Accept-Language choice (may be "") in ctx.
func WithLang(ctx context.Context, l Lang) context.Context {
	return context.WithValue(ctx, langKey{}, l)
}

// LangFrom returns the Accept-Language choice stored by WithLang, "" when absent.
func LangFrom(ctx context.Context) Lang {
	l, _ := ctx.Value(langKey{}).(Lang)
	return l
}

var (
	frDecimal   = regexp.MustCompile(`(\d),(\d)`)
	frThousands = regexp.MustCompile(`(\d) (\d{3})\b`)
	frPercent   = regexp.MustCompile(`(\d) %`)
)

// English is the English formatter: it rewrites numbers formatted by this package in French
// ("1 234", "7,02", "64 %", "N/D") into English ("1,234", "7.02", "64%", "N/A").
func English(s string) string {
	for {
		next := frThousands.ReplaceAllString(s, "$1 $2")
		if next == s {
			break
		}
		s = next
	}
	s = frDecimal.ReplaceAllString(s, "$1.$2")
	s = strings.ReplaceAll(s, " ", ",")
	s = frPercent.ReplaceAllString(s, "$1%")
	return strings.ReplaceAll(s, ND, "N/A")
}

// Format returns v formatted by f in lang l: identity in French, English() otherwise.
func Format(l Lang, v string) string {
	if l == EN {
		return English(v)
	}
	return v
}
