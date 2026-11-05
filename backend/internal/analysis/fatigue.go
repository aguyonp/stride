package analysis

import "stride/internal/fitdata"

// fatigueDrift compares the first and last third of running time (walk/stand excluded):
// GCT (ms, %), vertical oscillation and cadence. The verdict uses the GCT % (< 3 stable,
// 3–5 moderate, > 5 high), or the cadence drop when GCT is not recorded.
func (s *session) fatigueDrift() *FatigueDrift {
	if s == nil || !s.isRunning {
		return nil
	}
	total := s.runTime()
	if total < 360 {
		return nil
	}
	var g1, g3, v1, v3, c1, c3 wacc
	t := 0.0
	for i, r := range s.act.Records {
		if !s.run[i] {
			continue
		}
		w := s.w[i]
		switch {
		case t < total/3:
			g1.add(r.GCTMs, w)
			v1.add(r.VOCm, w)
			c1.add(r.CadenceSpm, w)
		case t >= 2*total/3:
			g3.add(r.GCTMs, w)
			v3.add(r.VOCm, w)
			c3.add(r.CadenceSpm, w)
		}
		t += w
	}
	fd := &FatigueDrift{Verdict: "stable"}
	gs, ge := g1.mean(), g3.mean()
	vs, ve := v1.mean(), v3.mean()
	cs, ce := c1.mean(), c3.mean()
	if !valid(gs) && !valid(cs) {
		return nil
	}
	fd.GctStartMs, fd.GctEndMs = num(gs, 0), num(ge, 0)
	if valid(gs) && valid(ge) && gs > 0 {
		fd.GctDeltaMs = num(ge-gs, 0)
		pct := (ge - gs) / gs * 100
		fd.GctDeltaPct = num(pct, 1)
		switch {
		case pct > 5:
			fd.Verdict = "high"
		case pct >= 3:
			fd.Verdict = "moderate"
		}
	}
	fd.VoStartCm, fd.VoEndCm = num(vs, 1), num(ve, 1)
	if valid(vs) && valid(ve) {
		fd.VoDeltaCm = num(ve-vs, 1)
	}
	fd.CadenceStartSpm, fd.CadenceEndSpm = num(cs, 0), num(ce, 0)
	if valid(cs) && valid(ce) {
		d := ce - cs
		fd.CadenceDeltaSpm = num(d, 0)
		if fd.GctDeltaPct == nil {
			switch {
			case d < -4:
				fd.Verdict = "high"
			case d <= -2:
				fd.Verdict = "moderate"
			}
		}
	}
	return fd
}

// staminaFrom returns start/end/min from the summary, completed by the series.
func staminaFrom(begin, end, minAvail *float64, a *fitdata.Activity) *Stamina {
	st := &Stamina{StartPct: pr(begin, 0), EndPct: pr(end, 0), MinPct: pr(minAvail, 0)}
	if a != nil && (st.StartPct == nil || st.EndPct == nil || st.MinPct == nil) {
		firstV, lastV, minV := nan, nan, nan
		for _, r := range a.Records {
			if valid(r.StaminaPct) {
				if !valid(firstV) {
					firstV = r.StaminaPct
				}
				lastV = r.StaminaPct
			}
			if valid(r.AvailStaminaPct) && (!valid(minV) || r.AvailStaminaPct < minV) {
				minV = r.AvailStaminaPct
			}
		}
		if st.StartPct == nil {
			st.StartPct = num(firstV, 0)
		}
		if st.EndPct == nil {
			st.EndPct = num(lastV, 0)
		}
		if st.MinPct == nil {
			st.MinPct = num(minV, 0)
		}
	}
	if st.StartPct == nil && st.EndPct == nil && st.MinPct == nil {
		return nil
	}
	return st
}
