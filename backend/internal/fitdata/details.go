package fitdata

import (
	"math"
	"sort"
	"time"

	"stride/internal/garmin"
)

// detailCols maps the /details metric descriptors. Values are already in display units
// (probe 2026-11-03): directDoubleCadence spm, directSpeed m/s, directVerticalOscillation cm,
// directStrideLength cm, directTimestamp ms GMT. Latitude/longitude are ignored.
type detailCols struct {
	ts, elapsed, hr, dbl, run, frac, spd, pw, ele, dist, gct, vo, vr, stride, gap, stam, avail int
}

func columns(d *garmin.ActivityDetails) detailCols {
	c := d.Column
	return detailCols{
		ts: c("directTimestamp"), elapsed: c("sumElapsedDuration"), hr: c("directHeartRate"),
		dbl: c("directDoubleCadence"), run: c("directRunCadence"), frac: c("directFractionalCadence"),
		spd: c("directSpeed"), pw: c("directPower"), ele: c("directElevation"), dist: c("sumDistance"),
		gct: c("directGroundContactTime"), vo: c("directVerticalOscillation"), vr: c("directVerticalRatio"),
		stride: c("directStrideLength"), gap: c("directGradeAdjustedSpeed"), stam: c("directPotentialStamina"),
		avail: c("directAvailableStamina"),
	}
}

func cell(row []*float64, i int) float64 {
	if i < 0 || i >= len(row) || row[i] == nil {
		return NaN
	}
	return *row[i]
}

func detailTime(row []*float64, c detailCols, start time.Time, t0 *float64) float64 {
	if ts := cell(row, c.ts); !math.IsNaN(ts) {
		if start.IsZero() {
			if math.IsNaN(*t0) {
				*t0 = ts
			}
			return (ts - *t0) / 1000
		}
		return (ts - float64(start.UnixMilli())) / 1000
	}
	return cell(row, c.elapsed)
}

// FromDetails builds records (no laps nor splits) from /activity/{id}/details; used when the FIT
// file is unavailable.
func FromDetails(d *garmin.ActivityDetails, start time.Time) *Activity {
	c := columns(d)
	out := &Activity{Source: "details", Start: start, Session: EmptySession()}
	t0 := NaN
	for _, m := range d.Metrics {
		row := m.Metrics
		t := detailTime(row, c, start, &t0)
		if math.IsNaN(t) {
			continue
		}
		cad := cell(row, c.dbl)
		if math.IsNaN(cad) {
			if r := cell(row, c.run); !math.IsNaN(r) {
				f := cell(row, c.frac)
				if math.IsNaN(f) {
					f = 0
				}
				cad = 2*r + f
			}
		}
		stride := cell(row, c.stride)
		if !math.IsNaN(stride) {
			stride /= 100
		}
		rec := emptyRecord(t)
		rec.HR, rec.CadenceSpm, rec.SpeedMS, rec.PowerW = cell(row, c.hr), cad, cell(row, c.spd), cell(row, c.pw)
		rec.AltM, rec.DistM, rec.GCTMs, rec.VOCm = cell(row, c.ele), cell(row, c.dist), cell(row, c.gct), cell(row, c.vo)
		rec.VRPct, rec.StepM = cell(row, c.vr), stride
		rec.GAPSpeedMS, rec.StaminaPct, rec.AvailStaminaPct = cell(row, c.gap), cell(row, c.stam), cell(row, c.avail)
		out.Records = append(out.Records, rec)
	}
	return out
}

// MergeDetails copies grade-adjusted speed and stamina from the /details series into the FIT records
// (nearest sample within 3 s).
func MergeDetails(a *Activity, d *garmin.ActivityDetails) {
	if a == nil || d == nil || len(a.Records) == 0 {
		return
	}
	c := columns(d)
	if c.gap < 0 && c.stam < 0 && c.avail < 0 {
		return
	}
	type pt struct{ t, gap, stam, avail float64 }
	var pts []pt
	t0 := NaN
	for _, m := range d.Metrics {
		t := detailTime(m.Metrics, c, a.Start, &t0)
		if math.IsNaN(t) {
			continue
		}
		pts = append(pts, pt{t, cell(m.Metrics, c.gap), cell(m.Metrics, c.stam), cell(m.Metrics, c.avail)})
	}
	if len(pts) == 0 {
		return
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].t < pts[j].t })
	for i := range a.Records {
		r := &a.Records[i]
		k := sort.Search(len(pts), func(j int) bool { return pts[j].t >= r.T })
		best := -1
		for _, j := range []int{k - 1, k} {
			if j >= 0 && j < len(pts) && math.Abs(pts[j].t-r.T) <= 3 && (best < 0 || math.Abs(pts[j].t-r.T) < math.Abs(pts[best].t-r.T)) {
				best = j
			}
		}
		if best < 0 {
			continue
		}
		if math.IsNaN(r.GAPSpeedMS) {
			r.GAPSpeedMS = pts[best].gap
		}
		if math.IsNaN(r.StaminaPct) {
			r.StaminaPct = pts[best].stam
		}
		if math.IsNaN(r.AvailStaminaPct) {
			r.AvailStaminaPct = pts[best].avail
		}
	}
}
