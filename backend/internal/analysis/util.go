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

// num retourne un pointeur arrondi à n décimales, nil si invalide.
func num(v float64, n int) *float64 {
	if !valid(v) {
		return nil
	}
	r := roundTo(v, n)
	return &r
}

// pf : pointeur → float (NaN si nil).
func pf(p *float64) float64 {
	if p == nil {
		return nan
	}
	return *p
}

// pr : arrondit un pointeur existant.
func pr(p *float64, n int) *float64 {
	if p == nil {
		return nil
	}
	return num(*p, n)
}

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

// wacc accumule une moyenne pondérée.
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

func maxValid(vs []float64) float64 {
	m := nan
	for _, v := range vs {
		if valid(v) && (!valid(m) || v > m) {
			m = v
		}
	}
	return m
}

// dateAdd ajoute n jours à une date "YYYY-MM-DD".
func dateAdd(d string, n int) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

// dateRange retourne les dates de from à to inclus (ordre chronologique).
func dateRange(from, to string) []string {
	var out []string
	for d := from; d <= to && len(out) < 400; d = dateAdd(d, 1) {
		out = append(out, d)
	}
	return out
}

// paceFromSpeed : m/s → s/km (NaN si < 0.5 m/s).
func paceFromSpeed(v float64) float64 {
	if !valid(v) || v < 0.5 {
		return nan
	}
	return 1000 / v
}
