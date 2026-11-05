package analysis

import (
	"math"
	"time"
)

var nan = math.NaN()

func valid(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func roundTo(v float64, n int) float64 {
	p := math.Pow(10, float64(n))
	return math.Round(v*p) / p
}

// num returns a pointer rounded to n decimals, nil when invalid.
func num(v float64, n int) *float64 {
	if !valid(v) {
		return nil
	}
	r := roundTo(v, n)
	return &r
}

// Num is the exported form of num.
func Num(v float64, n int) *float64 { return num(v, n) }

func pf(p *float64) float64 {
	if p == nil {
		return nan
	}
	return *p
}

// PF converts a pointer to float (NaN when nil).
func PF(p *float64) float64 { return pf(p) }

func pr(p *float64, n int) *float64 {
	if p == nil {
		return nil
	}
	return num(*p, n)
}

// PF0 converts a pointer to float (0 when nil).
func PF0(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// PR rounds an optional value.
func PR(p *float64, n int) *float64 { return pr(p, n) }

func strp(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func spv(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func first(vs ...float64) float64 {
	for _, v := range vs {
		if valid(v) {
			return v
		}
	}
	return nan
}

// wacc accumulates a weighted mean.
type wacc struct{ sw, swx float64 }

func (a *wacc) add(v, w float64) {
	if valid(v) && w > 0 {
		a.sw += w
		a.swx += v * w
	}
}

func (a *wacc) mean() float64 {
	if a.sw <= 0 {
		return nan
	}
	return a.swx / a.sw
}

// DateAdd adds n days to a YYYY-MM-DD date.
func DateAdd(d string, n int) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

func dateAdd(d string, n int) string { return DateAdd(d, n) }

// DateRange returns the dates from..to inclusive.
func DateRange(from, to string) []string {
	var out []string
	for d := from; d <= to && len(out) < 800; d = DateAdd(d, 1) {
		out = append(out, d)
	}
	return out
}

func dateRange(from, to string) []string { return DateRange(from, to) }

// PaceFromSpeed converts m/s to s/km (NaN below 0.5 m/s).
func PaceFromSpeed(v float64) float64 {
	if !valid(v) || v < 0.5 {
		return nan
	}
	return 1000 / v
}

func paceFromSpeed(v float64) float64 { return PaceFromSpeed(v) }
