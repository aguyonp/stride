package analysis

import (
	"math"
	"sort"

	"stride/internal/fitdata"
)

// SlopeThresholdPct separates downhill (< -1.5 %), flat and uphill (> +1.5 %).
const SlopeThresholdPct = 1.5

// minettiCost is the energy cost of running (J/kg/m) at grade i (fraction), Minetti et al. 2002.
func minettiCost(i float64) float64 {
	i = math.Max(-0.45, math.Min(0.45, i))
	return 155.4*math.Pow(i, 5) - 30.4*math.Pow(i, 4) - 43.3*math.Pow(i, 3) + 46.3*i*i + 19.5*i + 3.6
}

// ModelGAPSpeed converts a speed on a grade (%) to the equivalent flat speed.
func ModelGAPSpeed(speed, gradePct float64) float64 {
	if !valid(speed) || !valid(gradePct) || speed <= 0 {
		return nan
	}
	return speed * minettiCost(gradePct/100) / minettiCost(0)
}

// computeGrades fills s.grade (smoothed over >= 10 s and >= 20 m) and s.gap (Garmin GAP when
// recorded, otherwise the grade cost model).
func (s *session) computeGrades() {
	recs := s.act.Records
	n := len(recs)
	s.grade = make([]float64, n)
	s.gap = make([]float64, n)
	for i := range recs {
		s.grade[i] = nan
		if valid(recs[i].GradePct) {
			s.grade[i] = math.Max(-45, math.Min(45, recs[i].GradePct))
		}
	}
	if !s.act.Has(func(r fitdata.Record) float64 { return r.GradePct }) {
		for i := range recs {
			s.grade[i] = smoothedGrade(recs, i)
		}
	}
	for i, r := range recs {
		s.gap[i] = nan
		if valid(r.GAPSpeedMS) && r.GAPSpeedMS > 0 {
			s.gap[i] = r.GAPSpeedMS
			s.gapRecorded = true
			continue
		}
		if g := ModelGAPSpeed(r.SpeedMS, s.grade[i]); valid(g) {
			s.gap[i] = g
			s.gapModeled = true
		}
	}
}

func smoothedGrade(recs []fitdata.Record, i int) float64 {
	if !valid(recs[i].AltM) || !valid(recs[i].DistM) {
		return nan
	}
	lo, hi := i, i
	for steps := 0; steps < 120; steps++ {
		if recs[hi].T-recs[lo].T >= 10 && math.Abs(recs[hi].DistM-recs[lo].DistM) >= 20 {
			break
		}
		moved := false
		if lo > 0 && valid(recs[lo-1].AltM) && valid(recs[lo-1].DistM) && recs[i].T-recs[lo-1].T <= 60 {
			lo--
			moved = true
		}
		if hi+1 < len(recs) && valid(recs[hi+1].AltM) && valid(recs[hi+1].DistM) && recs[hi+1].T-recs[i].T <= 60 {
			hi++
			moved = true
		}
		if !moved {
			break
		}
	}
	dd := recs[hi].DistM - recs[lo].DistM
	if dd < 20 || recs[hi].T-recs[lo].T < 10 {
		return nan
	}
	g := (recs[hi].AltM - recs[lo].AltM) / dd * 100
	return math.Max(-45, math.Min(45, g))
}

type bucketAcc struct {
	t, d       float64
	cad, step  wacc
	steps      []float64
	idx        []int
	strideMaxV float64
}

func (s *session) slope() *Slope {
	if s == nil || !s.isRunning {
		return nil
	}
	accs := map[string]*bucketAcc{"down": {}, "flat": {}, "up": {}}
	any := false
	for i, r := range s.act.Records {
		g := s.grade[i]
		if !s.run[i] || !valid(g) {
			continue
		}
		any = true
		key := "flat"
		if g < -SlopeThresholdPct {
			key = "down"
		} else if g > SlopeThresholdPct {
			key = "up"
		}
		a := accs[key]
		w := s.w[i]
		a.t += w
		if valid(r.SpeedMS) {
			a.d += r.SpeedMS * w
		}
		a.cad.add(r.CadenceSpm, w)
		st := stepLength(r)
		a.step.add(st, w)
		if valid(st) && st > 0.3 && st < 2.5 {
			a.steps = append(a.steps, st)
		}
		a.idx = append(a.idx, i)
	}
	if !any {
		return nil
	}
	out := &Slope{ThresholdPct: SlopeThresholdPct}
	build := func(a *bucketAcc) *SlopeBucket {
		if a.t < 20 {
			return nil
		}
		b := &SlopeBucket{DurationS: roundTo(a.t, 0), DistanceM: roundTo(a.d, 0),
			CadenceSpm: num(a.cad.mean(), 0), StrideM: num(a.step.mean(), 2), StrideMaxM: num(s.strideMax(a), 2)}
		if a.d >= 10 {
			b.PaceSecPerKm = num(a.t/a.d*1000, 0)
		}
		return b
	}
	out.Flat, out.Downhill, out.Uphill = build(accs["flat"]), build(accs["down"]), build(accs["up"])
	if out.Flat != nil && out.Downhill != nil && out.Flat.CadenceSpm != nil && out.Downhill.CadenceSpm != nil {
		out.DownhillCadenceDropSpm = num(*out.Flat.CadenceSpm-*out.Downhill.CadenceSpm, 0)
	}
	return out
}

// strideMax is the max of a rolling 10 s mean of step length inside one bucket, after dropping
// values above 1.5 × the bucket median (GPS / sensor spikes).
func (s *session) strideMax(a *bucketAcc) float64 {
	if len(a.steps) == 0 {
		return nan
	}
	sorted := append([]float64(nil), a.steps...)
	sort.Float64s(sorted)
	limit := 1.5 * sorted[len(sorted)/2]
	recs := s.act.Records
	best := nan
	start := 0
	var sum, cnt float64
	type v struct{ t, x float64 }
	var win []v
	for _, i := range a.idx {
		x := stepLength(recs[i])
		if !valid(x) || x <= 0.3 || x > limit {
			continue
		}
		t := recs[i].T
		if len(win) > 0 && t-win[len(win)-1].t > 3 {
			win, sum, cnt, start = win[:0], 0, 0, 0
		}
		win = append(win, v{t, x})
		sum += x
		cnt++
		for start < len(win) && t-win[start].t > 10 {
			sum -= win[start].x
			cnt--
			start++
		}
		if t-win[start].t >= 8 && cnt > 0 {
			if m := sum / cnt; !valid(best) || m > best {
				best = m
			}
		}
	}
	return best
}
