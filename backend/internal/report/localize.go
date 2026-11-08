package report

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	f "stride/internal/frfmt"
)

// Prompts and reports are written in French (golden-tested); Localize renders the English version
// (§15.2) by mapping the fixed fragments of every French format string to their English
// counterpart (enPairs: same verbs, same order) and rewriting French number formatting with the
// frfmt English formatter. Fenced blocks (JSON contracts, quoted third-party text) are left as is.

type enChunk struct {
	fr, en string
	re     *regexp.Regexp // format pattern, or letter-guarded match for short labels; nil: plain replace
}

var (
	verb     = regexp.MustCompile(`%%|%[sdvq]|%\.\d+f|%g`)
	enOnce   sync.Once
	enFormat []enChunk // format strings with verbs, matched as a whole (verbs become groups)
	enLabels []enChunk // fixed labels and section titles
)

func hasLetter(s string) bool { return strings.IndexFunc(s, unicode.IsLetter) >= 0 }

func runes(s string) int { return utf8.RuneCountInString(s) }

// formatPattern turns "Allure: %s (GAP: %s)" into a regexp with one group per verb and the English
// template into its expansion ("Pace: ${1} (GAP: ${2})"); a trailing verb runs to the end of line.
func formatPattern(fr, en string) (*regexp.Regexp, string) {
	var pat strings.Builder
	parts, verbs := verb.Split(fr, -1), verb.FindAllString(fr, -1)
	for i, part := range parts {
		pat.WriteString(regexp.QuoteMeta(part))
		if i < len(verbs) {
			switch {
			case verbs[i] == "%%":
				pat.WriteString("%")
			case i == len(verbs)-1 && parts[i+1] == "":
				pat.WriteString("(.*)")
			default:
				pat.WriteString("(.+?)")
			}
		}
	}
	n := 0
	tmpl := verb.ReplaceAllStringFunc(strings.ReplaceAll(en, "$", "$$"), func(v string) string {
		if v == "%%" {
			return "%"
		}
		n++
		return "${" + strconv.Itoa(n) + "}"
	})
	return regexp.MustCompile(pat.String()), tmpl
}

func buildChunks() {
	seen := map[string]bool{}
	for _, p := range enPairs {
		fr, en := p[0], p[1]
		if fr == en || seen[fr] || !hasLetter(fr) {
			continue
		}
		seen[fr] = true
		if verb.MatchString(fr) {
			re, tmpl := formatPattern(fr, en)
			enFormat = append(enFormat, enChunk{fr: fr, en: tmpl, re: re})
			continue
		}
		c := enChunk{fr: fr, en: en}
		if runes(fr) < 12 {
			pre, post := "", ""
			if r, _ := utf8.DecodeRuneInString(fr); unicode.IsLetter(r) {
				pre = `(^|[^\p{L}\p{N}])`
			}
			if r, _ := utf8.DecodeLastRuneInString(fr); unicode.IsLetter(r) {
				post = `($|[^\p{L}\p{N}])`
			}
			if pre != "" || post != "" {
				c.re = regexp.MustCompile(pre + regexp.QuoteMeta(fr) + post)
			}
		}
		enLabels = append(enLabels, c)
	}
	longest := func(cs []enChunk) func(i, j int) bool {
		return func(i, j int) bool { return runes(cs[i].fr) > runes(cs[j].fr) }
	}
	sort.SliceStable(enFormat, longest(enFormat))
	sort.SliceStable(enLabels, longest(enLabels))
}

// literal is the fixed text of a format string before its first verb (cheap pre-filter).
func literal(fr string) string { return verb.Split(fr, 2)[0] }

func translateLine(s string) string {
	for _, c := range enFormat {
		if lit := literal(c.fr); lit != "" && !strings.Contains(s, lit) {
			continue
		}
		s = c.re.ReplaceAllString(s, c.en)
	}
	for _, c := range enLabels {
		if !strings.Contains(s, c.fr) {
			continue
		}
		if c.re == nil {
			s = strings.ReplaceAll(s, c.fr, c.en)
			continue
		}
		s = c.re.ReplaceAllStringFunc(s, func(m string) string { return strings.Replace(m, c.fr, c.en, 1) })
	}
	return f.English(frenchTypography.Replace(s))
}

// frenchTypography: spaces before « : ; » and guillemets are French conventions.
var frenchTypography = strings.NewReplacer(" : ", ": ", " ; ", "; ", "« ", "“", " »", "”")

// Localize returns md in language l: unchanged in French, translated in English.
func Localize(md string, l f.Lang) string {
	if l != f.EN {
		return md
	}
	enOnce.Do(buildChunks)
	lines := strings.Split(md, "\n")
	fenced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if !fenced {
			lines[i] = translateLine(line)
		}
	}
	return strings.Join(lines, "\n")
}

// DirectiveFor is the §6.1 objectivity directive in language l.
func DirectiveFor(l f.Lang) string { return Localize(Directive, l) }
