package analysis

import (
	"stride/internal/fitdata"
)

const maxSeriesPoints = 400

// buildSeries downsample les records en ≤ 400 points (moyenne par paquet).
func buildSeries(s *session) Series {
	out := Series{
		TS: []float64{}, HR: []*float64{}, CadenceSpm: []*float64{}, PowerW: []*float64{},
		PaceSecPerKm: []*float64{}, ElevationM: []*float64{}, GctMs: []*float64{}, VoCm: []*float64{},
		GctBalanceLeftPct: []*float64{},
	}
	if s == nil || len(s.act.Records) == 0 {
		return out
	}
	recs := s.act.Records
	n := len(recs)
	step := (n + maxSeriesPoints - 1) / maxSeriesPoints
	if step < 1 {
		step = 1
	}
	avg := func(chunk []fitdata.Record, get func(fitdata.Record) float64) float64 {
		var a wacc
		for _, r := range chunk {
			a.add(get(r), 1)
		}
		return a.mean()
	}
	for i := 0; i < n; i += step {
		j := i + step
		if j > n {
			j = n
		}
		c := recs[i:j]
		out.TS = append(out.TS, roundTo(c[0].T, 0))
		out.HR = append(out.HR, num(avg(c, func(r fitdata.Record) float64 { return r.HR }), 0))
		out.CadenceSpm = append(out.CadenceSpm, num(avg(c, func(r fitdata.Record) float64 {
			if r.CadenceSpm <= 0 {
				return nan
			}
			return r.CadenceSpm
		}), 0))
		out.PowerW = append(out.PowerW, num(avg(c, func(r fitdata.Record) float64 { return r.PowerW }), 0))
		out.PaceSecPerKm = append(out.PaceSecPerKm, num(paceFromSpeed(avg(c, func(r fitdata.Record) float64 { return r.SpeedMS })), 0))
		out.ElevationM = append(out.ElevationM, num(avg(c, func(r fitdata.Record) float64 { return r.AltM }), 1))
		out.GctMs = append(out.GctMs, num(avg(c, func(r fitdata.Record) float64 { return r.GCTMs }), 0))
		out.VoCm = append(out.VoCm, num(avg(c, func(r fitdata.Record) float64 { return r.VOCm }), 1))
		out.GctBalanceLeftPct = append(out.GctBalanceLeftPct, num(avg(c, func(r fitdata.Record) float64 { return r.GCTBalLeftPct }), 1))
	}
	return out
}
