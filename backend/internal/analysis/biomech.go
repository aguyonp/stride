package analysis

import (
	"math"
	"sort"

	"stride/internal/athlete"
	"stride/internal/fitdata"
	"stride/internal/garmin"
)

const (
	runMinSpeedMS    = 1.8
	runMinCadenceSpm = 130
	maxRecordWeightS = 5.0
	gctNAReason      = "Non mesuré : nécessite HRM-Pro / HRM 600 / Running Dynamics Pod"
	// NoGarminReason is the reason given for Garmin-only metrics of Strava-only users.
	NoGarminReason = "Garmin non connecté"
)

// session holds the per-record masks derived from a decoded activity.
type session struct {
	act       *fitdata.Activity
	isRunning bool
	w         []float64 // time weight of each record (s)
	run       []bool    // running record (walk/stand excluded)
	pauses    [][2]float64
	grade     []float64 // smoothed grade (%)
	gap       []float64 // grade-adjusted speed (m/s)

	gapRecorded, gapModeled bool
}

func newSession(a *fitdata.Activity, isRunning bool) *session {
	s := &session{act: a, isRunning: isRunning}
	n := len(a.Records)
	s.w = make([]float64, n)
	for i := 0; i < n; i++ {
		w := 1.0
		if i+1 < n {
			w = a.Records[i+1].T - a.Records[i].T
		}
		if w < 0 {
			w = 0
		}
		if w > maxRecordWeightS {
			w = 1 // recording gap (auto-pause): do not overweight
		}
		s.w[i] = w
	}
	for _, sp := range a.Splits {
		if (sp.Type == "rwd_walk" || sp.Type == "rwd_stand") && valid(sp.StartT) && valid(sp.EndT) {
			s.pauses = append(s.pauses, [2]float64{sp.StartT, sp.EndT})
		}
	}
	s.run = make([]bool, n)
	for i, r := range a.Records {
		s.run[i] = s.isRunRecord(r)
	}
	s.computeGrades()
	return s
}

func (s *session) inPause(t float64) bool {
	for _, p := range s.pauses {
		if t >= p[0] && t <= p[1] {
			return true
		}
	}
	return false
}

func (s *session) isRunRecord(r fitdata.Record) bool {
	if s.inPause(r.T) {
		return false
	}
	cad, spd := r.CadenceSpm, r.SpeedMS
	if !s.isRunning {
		if valid(spd) {
			return spd >= 0.5
		}
		return valid(r.HR)
	}
	if !valid(cad) && !valid(spd) {
		return false
	}
	if valid(cad) && cad < runMinCadenceSpm {
		return false
	}
	if valid(spd) && spd < runMinSpeedMS {
		return false
	}
	return true
}

func (s *session) runMean(get func(fitdata.Record) float64) float64 {
	var acc wacc
	for i, r := range s.act.Records {
		if s.run[i] {
			acc.add(get(r), s.w[i])
		}
	}
	return acc.mean()
}

func (s *session) runTime() float64 {
	t := 0.0
	for i := range s.act.Records {
		if s.run[i] {
			t += s.w[i]
		}
	}
	return t
}

// stepLength returns the measured step length, or speed / cadence when the sensor gives none.
func stepLength(r fitdata.Record) float64 {
	if valid(r.StepM) && r.StepM > 0 {
		return r.StepM
	}
	if valid(r.SpeedMS) && valid(r.CadenceSpm) && r.CadenceSpm > 0 && r.SpeedMS > 0 {
		return r.SpeedMS * 60 / r.CadenceSpm
	}
	return nan
}

func computeBiomech(s *session, det *garmin.ActivityDetail, th athlete.Thresholds, isRun, garminConnected bool) Biomechanics {
	b := Biomechanics{GctBalance: GctBalance{Available: false, Reason: strp(gctNAReason)}}
	if !garminConnected {
		b.GctBalance.Reason = strp(NoGarminReason)
	}
	if !isRun {
		b.GctBalance.Reason = strp("Non applicable : activité hors course à pied")
		return b
	}
	var sm *garmin.ActivitySumDTO
	if det != nil {
		sm = &det.Summary
	}
	api := func(f func(*garmin.ActivitySumDTO) *float64) float64 {
		if sm == nil {
			return nan
		}
		return pf(f(sm))
	}
	cad, cadMax, inTarget, below := nan, nan, nan, nan
	gct, vo, vr, stride, bal := nan, nan, nan, nan, nan
	ss := fitdata.EmptySession()
	if s != nil && len(s.act.Records) > 0 {
		ss = s.act.Session
		cad = s.runMean(func(r fitdata.Record) float64 { return r.CadenceSpm })
		cadMax = s.smoothedRunMax(func(r fitdata.Record) float64 { return r.CadenceSpm }, 5)
		inTarget, below = s.cadenceShares(th.CadenceTargetMin, th.CadenceTargetMax)
		gct = s.runMean(func(r fitdata.Record) float64 { return r.GCTMs })
		vo = s.runMean(func(r fitdata.Record) float64 { return r.VOCm })
		vr = s.runMean(func(r fitdata.Record) float64 { return r.VRPct })
		stride = s.runMean(func(r fitdata.Record) float64 { return r.StepM })
		bal = s.runMean(func(r fitdata.Record) float64 { return r.GCTBalLeftPct })
	} else if s != nil {
		ss = s.act.Session
	}
	cad = first(cad, api(func(m *garmin.ActivitySumDTO) *float64 { return m.AverageRunCadence }), ss.AvgCadSpm)
	cadMax = first(cadMax, api(func(m *garmin.ActivitySumDTO) *float64 { return m.MaxRunCadence }), ss.MaxCadSpm)
	gct = first(gct, api(func(m *garmin.ActivitySumDTO) *float64 { return m.GroundContactTime }), ss.AvgGCTMs)
	vo = first(vo, api(func(m *garmin.ActivitySumDTO) *float64 { return m.VerticalOscillation }), ss.AvgVOCm)
	vr = first(vr, api(func(m *garmin.ActivitySumDTO) *float64 { return m.VerticalRatio }), ss.AvgVRPct)
	strideAPI := api(func(m *garmin.ActivitySumDTO) *float64 { return m.StrideLength })
	if valid(strideAPI) {
		strideAPI /= 100
	}
	stride = first(stride, strideAPI, ss.AvgStepM)
	if !valid(stride) && s != nil {
		stride = s.runMean(stepLength)
	}
	bal = first(bal, ss.AvgGCTBal)

	b.CadenceAvgSpm = num(cad, 0)
	b.CadenceMaxSpm = num(cadMax, 0)
	b.CadenceInTargetPct = num(inTarget, 0)
	b.CadenceBelow170Pct = num(below, 0)
	b.GroundContactTimeMs = num(gct, 0)
	b.VerticalOscillationCm = num(vo, 1)
	b.VerticalRatioPct = num(vr, 1)
	b.StrideLengthM = num(stride, 2)
	if valid(bal) && bal > 0 && bal < 100 {
		b.GctBalance = gctBalance(bal, th)
	}
	return b
}

func gctBalance(left float64, th athlete.Thresholds) GctBalance {
	l := roundTo(left, 1)
	r := roundTo(100-left, 1)
	gap := roundTo(math.Abs(l-r), 1)
	g := GctBalance{Available: true, LeftPct: &l, RightPct: &r, GapPct: &gap}
	switch {
	case l < r:
		g.WeakerSide = strp("left")
	case r < l:
		g.WeakerSide = strp("right")
	}
	lvl := "ok"
	if gap > 2*th.GctBalanceGapPct {
		lvl = "danger"
	} else if gap > th.GctBalanceGapPct {
		lvl = "warn"
	}
	g.Alert = &lvl
	return g
}

// cadenceShares returns the share of running time inside [min, max] and below 170 spm.
func (s *session) cadenceShares(min, max float64) (float64, float64) {
	tot, in, below := 0.0, 0.0, 0.0
	for i, r := range s.act.Records {
		if !s.run[i] || !valid(r.CadenceSpm) {
			continue
		}
		w := s.w[i]
		tot += w
		c := math.Round(r.CadenceSpm)
		if c >= min && c <= max {
			in += w
		}
		if c < 170 {
			below += w
		}
	}
	if tot <= 0 {
		return nan, nan
	}
	return in / tot * 100, below / tot * 100
}

// smoothedRunMax is the max of a k-record moving average over running records (ignores spikes).
func (s *session) smoothedRunMax(get func(fitdata.Record) float64, k int) float64 {
	var vals []float64
	best := nan
	for i, r := range s.act.Records {
		v := get(r)
		if !s.run[i] || !valid(v) {
			vals = vals[:0]
			continue
		}
		vals = append(vals, v)
		if len(vals) > k {
			vals = vals[1:]
		}
		if len(vals) == k {
			m := 0.0
			for _, x := range vals {
				m += x
			}
			m /= float64(k)
			if !valid(best) || m > best {
				best = m
			}
		}
	}
	return best
}

// computeDrift is the cardiac decoupling over running records (>= 10 min of running).
func computeDrift(s *session) *Drift {
	if s == nil {
		return nil
	}
	type pt struct{ t, w, hr, x float64 }
	nPow, nRun := 0, 0
	for i, r := range s.act.Records {
		if s.run[i] && valid(r.HR) {
			nRun++
			if valid(r.PowerW) && r.PowerW > 0 {
				nPow++
			}
		}
	}
	if nRun == 0 {
		return nil
	}
	method := "pace:hr"
	if float64(nPow) >= 0.8*float64(nRun) {
		method = "pw:hr"
	}
	if !s.isRunning && method != "pw:hr" {
		return nil
	}
	var pts []pt
	tot := 0.0
	for i, r := range s.act.Records {
		if !s.run[i] || !valid(r.HR) || r.HR <= 0 {
			continue
		}
		x := r.SpeedMS
		if method == "pw:hr" {
			x = r.PowerW
		}
		if !valid(x) || x <= 0 {
			continue
		}
		pts = append(pts, pt{t: tot, w: s.w[i], hr: r.HR, x: x})
		tot += s.w[i]
	}
	if tot < 600 || len(pts) < 60 {
		return nil
	}
	var h1, h2, x1, x2, t2, t3 wacc
	for _, p := range pts {
		if p.t < tot/2 {
			h1.add(p.hr, p.w)
			x1.add(p.x, p.w)
		} else {
			h2.add(p.hr, p.w)
			x2.add(p.x, p.w)
		}
		switch {
		case p.t >= 2*tot/3:
			t3.add(p.hr, p.w)
		case p.t >= tot/3:
			t2.add(p.hr, p.w)
		}
	}
	ef1 := x1.mean() / h1.mean()
	ef2 := x2.mean() / h2.mean()
	if !valid(ef1) || !valid(ef2) || ef1 == 0 {
		return nil
	}
	dec := (ef1 - ef2) / ef1 * 100
	verdict := "stable"
	if dec > 10 {
		verdict = "high"
	} else if dec >= 5 {
		verdict = "moderate"
	}
	rise := t3.mean() - t2.mean()
	if !valid(rise) {
		rise = 0
	}
	return &Drift{
		Method:             method,
		DecouplingPct:      roundTo(dec, 1),
		FirstHalfHR:        roundTo(h1.mean(), 0),
		SecondHalfHR:       roundTo(h2.mean(), 0),
		LastThirdHrRiseBpm: roundTo(rise, 0),
		Verdict:            verdict,
	}
}

// targetPaceTime returns time and distance spent in a pace range (running records, speed smoothed over 5 records).
func targetPaceTime(s *session, minSec, maxSec float64) (float64, float64) {
	if s == nil {
		return 0, 0
	}
	recs := s.act.Records
	secs, dist := 0.0, 0.0
	const k = 5
	for i := range recs {
		if !s.run[i] {
			continue
		}
		var acc wacc
		lo, hi := i-k/2, i+k/2
		if lo < 0 {
			lo = 0
		}
		if hi >= len(recs) {
			hi = len(recs) - 1
		}
		for j := lo; j <= hi; j++ {
			acc.add(recs[j].SpeedMS, 1)
		}
		v := acc.mean()
		pace := paceFromSpeed(v)
		if valid(pace) && pace >= minSec && pace <= maxSec {
			secs += s.w[i]
			dist += v * s.w[i]
		}
	}
	return secs, dist
}

// zonesFromRecords computes time in Z1..Z5 from zone floors.
func zonesFromRecords(s *session, floors []float64) []float64 {
	if s == nil || len(floors) != 5 || !sort.Float64sAreSorted(floors) {
		return nil
	}
	out := make([]float64, 5)
	found := false
	for i, r := range s.act.Records {
		if !valid(r.HR) {
			continue
		}
		z := -1
		for k := 4; k >= 0; k-- {
			if r.HR >= floors[k] {
				z = k
				break
			}
		}
		if z >= 0 {
			out[z] += s.w[i]
			found = true
		}
	}
	if !found {
		return nil
	}
	for i := range out {
		out[i] = roundTo(out[i], 0)
	}
	return out
}
