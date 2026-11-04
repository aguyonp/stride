// Package frfmt formate nombres, durées, allures et dates à la française
// (virgule décimale, séparateur de milliers = espace, "N/D" si absent).
package frfmt

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ND est la valeur affichée quand une donnée est absente.
const ND = "N/D"

// Valid indique si v est une valeur exploitable (non NaN / Inf).
func Valid(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// P convertit un pointeur en float (NaN si nil).
func P(p *float64) float64 {
	if p == nil {
		return math.NaN()
	}
	return *p
}

// round arrondit à n décimales (demi vers l'extérieur).
func round(v float64, n int) float64 {
	p := math.Pow(10, float64(n))
	return math.Round(v*p) / p
}

// Dec formate avec n décimales et une virgule ("8,1"). Pas de séparateur de milliers.
func Dec(v float64, n int) string {
	if !Valid(v) {
		return ND
	}
	r := round(v, n)
	if r == 0 {
		r = 0 // évite "-0"
	}
	return strings.Replace(strconv.FormatFloat(r, 'f', n, 64), ".", ",", 1)
}

// Int formate un entier arrondi avec séparateur de milliers ("8 432").
func Int(v float64) string {
	if !Valid(v) {
		return ND
	}
	n := int64(math.Round(v))
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// Signed formate avec signe explicite ("+3,2", "-1,0", "+4").
func Signed(v float64, n int) string {
	if !Valid(v) {
		return ND
	}
	r := round(v, n)
	if r >= 0 {
		return "+" + Dec(math.Abs(r), n)
	}
	return "-" + Dec(math.Abs(r), n)
}

// Duration formate des secondes en "42:10" ou "1:02:10".
func Duration(s float64) string {
	if !Valid(s) || s < 0 {
		return ND
	}
	t := int64(math.Round(s))
	h, m, sec := t/3600, (t%3600)/60, t%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}

// PaceShort formate une allure en s/km : "5:43".
func PaceShort(secPerKm float64) string {
	if !Valid(secPerKm) || secPerKm <= 0 {
		return ND
	}
	t := int64(math.Round(secPerKm))
	return fmt.Sprintf("%d:%02d", t/60, t%60)
}

// Pace formate une allure en s/km : "5:43/km".
func Pace(secPerKm float64) string {
	if !Valid(secPerKm) || secPerKm <= 0 {
		return ND
	}
	return PaceShort(secPerKm) + "/km"
}

// SleepHM formate une durée de sommeil : "1h22", "7h05".
func SleepHM(s float64) string {
	if !Valid(s) || s < 0 {
		return ND
	}
	t := int64(math.Round(s / 60))
	return fmt.Sprintf("%dh%02d", t/60, t%60)
}

// Km formate des mètres en kilomètres avec n décimales : "7,02 km".
func Km(m float64, n int) string {
	if !Valid(m) {
		return ND
	}
	return Dec(m/1000, n) + " km"
}

// DateTime convertit "2026-11-02T18:30:12" (ou avec espace) en "02/11/2026 18:30".
func DateTime(local string) string {
	local = strings.Replace(strings.TrimSpace(local), " ", "T", 1)
	if len(local) < 16 {
		return Date(local)
	}
	return Date(local[:10]) + " " + local[11:16]
}

// Date convertit "2026-11-02" en "02/11/2026".
func Date(d string) string {
	if len(d) < 10 {
		return ND
	}
	return d[8:10] + "/" + d[5:7] + "/" + d[0:4]
}

// DayMonth convertit "2026-11-02" en "02/11".
func DayMonth(d string) string {
	if len(d) < 10 {
		return ND
	}
	return d[8:10] + "/" + d[5:7]
}

// Or retourne s si v est valide, sinon N/D.
func Or(v float64, f func(float64) string) string {
	if !Valid(v) {
		return ND
	}
	return f(v)
}

// Unit formate "v unité" (entier) ou N/D.
func Unit(v float64, unit string) string {
	if !Valid(v) {
		return ND
	}
	return Int(v) + " " + unit
}

// UnitDec formate "v,d unité" ou N/D.
func UnitDec(v float64, n int, unit string) string {
	if !Valid(v) {
		return ND
	}
	return Dec(v, n) + " " + unit
}

// Num formate avec au plus maxDec décimales, sans zéros inutiles ("9", "1,5").
func Num(v float64, maxDec int) string {
	if !Valid(v) {
		return ND
	}
	s := Dec(v, maxDec)
	if strings.Contains(s, ",") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ",")
	}
	return s
}
