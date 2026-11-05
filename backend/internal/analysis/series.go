package analysis

const maxSeriesPoints = 400

// buildSeries downsamples the records to <= 400 points (bucket means).
func buildSeries(s *session) Series {
	out := Series{
		TS: []float64{}, HR: []*float64{}, CadenceSpm: []*float64{}, PowerW: []*float64{},
		PaceSecPerKm: []*float64{}, ElevationM: []*float64{}, GctMs: []*float64{}, VoCm: []*float64{},
		GctBalanceLeftPct: []*float64{}, GapSecPerKm: []*float64{}, StaminaPct: []*float64{},
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
	for i := 0; i < n; i += step {
		j := i + step
		if j > n {
			j = n
		}
		var hr, cad, pw, spd, alt, gct, vo, bal, gap, stam wacc
		for k := i; k < j; k++ {
			r := recs[k]
			hr.add(r.HR, 1)
			if r.CadenceSpm > 0 {
				cad.add(r.CadenceSpm, 1)
			}
			pw.add(r.PowerW, 1)
			spd.add(r.SpeedMS, 1)
			alt.add(r.AltM, 1)
			gct.add(r.GCTMs, 1)
			vo.add(r.VOCm, 1)
			bal.add(r.GCTBalLeftPct, 1)
			if s.isRunning && valid(r.SpeedMS) && r.SpeedMS >= 0.5 {
				gap.add(s.gap[k], 1)
			}
			stam.add(r.StaminaPct, 1)
		}
		out.TS = append(out.TS, roundTo(recs[i].T, 0))
		out.HR = append(out.HR, num(hr.mean(), 0))
		out.CadenceSpm = append(out.CadenceSpm, num(cad.mean(), 0))
		out.PowerW = append(out.PowerW, num(pw.mean(), 0))
		out.PaceSecPerKm = append(out.PaceSecPerKm, num(paceFromSpeed(spd.mean()), 0))
		out.ElevationM = append(out.ElevationM, num(alt.mean(), 1))
		out.GctMs = append(out.GctMs, num(gct.mean(), 0))
		out.VoCm = append(out.VoCm, num(vo.mean(), 1))
		out.GctBalanceLeftPct = append(out.GctBalanceLeftPct, num(bal.mean(), 1))
		out.GapSecPerKm = append(out.GapSecPerKm, num(paceFromSpeed(gap.mean()), 0))
		out.StaminaPct = append(out.StaminaPct, num(stam.mean(), 0))
	}
	return out
}
