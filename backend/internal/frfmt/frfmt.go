// Package frfmt formats numbers, durations, paces and dates the French way
// (decimal comma, space as thousands separator, "N/D" when absent).
package frfmt

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const ND = "N/D"

func Valid(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func P(p *float64) float64 {
	if p == nil {
		return math.NaN()
	}
	return *p
}

func round(v float64, n int) float64 {
	p := math.Pow(10, float64(n))
	return math.Round(v*p) / p
}

func Dec(v float64, n int) string {
	if !Valid(v) {
		return ND
	}
	r := round(v, n)
	if r == 0 {
		r = 0 // avoid "-0"
	}
	return strings.Replace(strconv.FormatFloat(r, 'f', n, 64), ".", ",", 1)
}

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

func PaceShort(secPerKm float64) string {
	if !Valid(secPerKm) || secPerKm <= 0 {
		return ND
	}
	t := int64(math.Round(secPerKm))
	return fmt.Sprintf("%d:%02d", t/60, t%60)
}

func Pace(secPerKm float64) string {
	if !Valid(secPerKm) || secPerKm <= 0 {
		return ND
	}
	return PaceShort(secPerKm) + "/km"
}

func SleepHM(s float64) string {
	if !Valid(s) || s < 0 {
		return ND
	}
	t := int64(math.Round(s / 60))
	return fmt.Sprintf("%dh%02d", t/60, t%60)
}

func Km(m float64, n int) string {
	if !Valid(m) {
		return ND
	}
	return Dec(m/1000, n) + " km"
}

func DateTime(local string) string {
	local = strings.Replace(strings.TrimSpace(local), " ", "T", 1)
	if len(local) < 16 {
		return Date(local)
	}
	return Date(local[:10]) + " " + local[11:16]
}

func Date(d string) string {
	if len(d) < 10 {
		return ND
	}
	return d[8:10] + "/" + d[5:7] + "/" + d[0:4]
}

func DayMonth(d string) string {
	if len(d) < 10 {
		return ND
	}
	return d[8:10] + "/" + d[5:7]
}

func Or(v float64, f func(float64) string) string {
	if !Valid(v) {
		return ND
	}
	return f(v)
}

func Unit(v float64, unit string) string {
	if !Valid(v) {
		return ND
	}
	return Int(v) + " " + unit
}

func UnitDec(v float64, n int, unit string) string {
	if !Valid(v) {
		return ND
	}
	return Dec(v, n) + " " + unit
}

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
