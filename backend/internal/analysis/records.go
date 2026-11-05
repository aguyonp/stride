package analysis

import (
	"math"

	"stride/internal/fitdata"
)

// RecordsVersion is bumped whenever the best-effort algorithm changes (persisted per activity).
const RecordsVersion = 1

const (
	glitchSpeedMS  = 12.0  // faster sample-to-sample speeds are GPS jumps: windows never span them
	recordMinPaceS = 150.0 // s/km; faster windows are discarded as implausible
)

// BestEfforts returns, for each distance (key → metres), the fastest wall-clock time (s) over a
// rolling window of the cumulative distance, interpolating the window start between samples.
func BestEfforts(act *fitdata.Activity, distances map[string]float64) map[string]float64 {
	out := map[string]float64{}
	if act == nil {
		return out
	}
	type pt struct{ t, d float64 }
	var pts []pt
	var segs []int // index of the first point of each segment (split at GPS jumps)
	for _, r := range act.Records {
		if math.IsNaN(r.DistM) || math.IsNaN(r.T) {
			continue
		}
		if n := len(pts); n > 0 {
			last := pts[n-1]
			if r.T <= last.t || r.DistM < last.d {
				continue
			}
			if (r.DistM-last.d)/(r.T-last.t) > glitchSpeedMS {
				segs = append(segs, n)
			}
		} else {
			segs = append(segs, 0)
		}
		pts = append(pts, pt{r.T, r.DistM})
	}
	segs = append(segs, len(pts))
	for key, D := range distances {
		best := math.Inf(1)
		for s := 0; s+1 < len(segs); s++ {
			lo, hi := segs[s], segs[s+1]
			i := lo
			for j := lo; j < hi; j++ {
				if pts[j].d-pts[lo].d < D {
					continue
				}
				for i+1 < j && pts[j].d-pts[i+1].d >= D {
					i++
				}
				start := pts[i].t
				if dd := pts[i+1].d - pts[i].d; dd > 0 {
					frac := math.Min(1, (pts[j].d-D-pts[i].d)/dd)
					start += frac * (pts[i+1].t - pts[i].t)
				}
				best = math.Min(best, pts[j].t-start)
			}
		}
		if !math.IsInf(best, 1) && best >= D/1000*recordMinPaceS {
			out[key] = math.Round(best)
		}
	}
	return out
}

// WholeEffort estimates a best effort from an activity without series whose distance is within
// 3 % above D (e.g. a 10.1 km race for the 10 km).
func WholeEffort(distanceM, durationS, D float64) (float64, bool) {
	if distanceM < D || distanceM > D*1.03 || durationS <= 0 {
		return 0, false
	}
	t := math.Round(durationS * D / distanceM)
	return t, t >= D/1000*recordMinPaceS
}
