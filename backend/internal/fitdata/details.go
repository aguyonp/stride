package fitdata

import (
	"math"
	"time"

	"stride/internal/garmin"
)

// FromDetails construit une Activity (records uniquement, sans laps ni splits) depuis
// /activity/{id}/details — utilisé en repli quand le FIT est indisponible.
// Les valeurs de l'API sont déjà dans l'unité d'affichage (sonde 2026-11-03 :
// directDoubleCadence en pas/min, directSpeed en m/s, directVerticalOscillation en cm,
// directStrideLength en cm, directTimestamp en ms GMT). Latitude/longitude ignorées.
func FromDetails(d *garmin.ActivityDetails, start time.Time) *Activity {
	col := func(k string) int { return d.Column(k) }
	iTs, iElapsed := col("directTimestamp"), col("sumElapsedDuration")
	iHR, iDbl, iRun, iFrac := col("directHeartRate"), col("directDoubleCadence"), col("directRunCadence"), col("directFractionalCadence")
	iSpd, iPw, iEle, iDist := col("directSpeed"), col("directPower"), col("directElevation"), col("sumDistance")
	iGCT, iVO, iVR, iStride := col("directGroundContactTime"), col("directVerticalOscillation"), col("directVerticalRatio"), col("directStrideLength")

	get := func(row []*float64, i int) float64 {
		if i < 0 || i >= len(row) || row[i] == nil {
			return NaN
		}
		return *row[i]
	}
	out := &Activity{Source: "details", Start: start}
	out.Session = Session{TimerS: NaN, ElapsedS: NaN, DistM: NaN, AvgHR: NaN, MaxHR: NaN, AvgPower: NaN, NormPower: NaN,
		AvgCadSpm: NaN, MaxCadSpm: NaN, AvgGCTMs: NaN, AvgVOCm: NaN, AvgVRPct: NaN, AvgStepM: NaN, AvgGCTBal: NaN, AscentM: NaN, DescentM: NaN}
	var t0 float64 = NaN
	for _, m := range d.Metrics {
		row := m.Metrics
		t := NaN
		if ts := get(row, iTs); !math.IsNaN(ts) {
			if start.IsZero() {
				if math.IsNaN(t0) {
					t0 = ts
				}
				t = (ts - t0) / 1000
			} else {
				t = (ts - float64(start.UnixMilli())) / 1000
			}
		} else if e := get(row, iElapsed); !math.IsNaN(e) {
			t = e
		}
		if math.IsNaN(t) {
			continue
		}
		cad := get(row, iDbl)
		if math.IsNaN(cad) {
			if r := get(row, iRun); !math.IsNaN(r) {
				f := get(row, iFrac)
				if math.IsNaN(f) {
					f = 0
				}
				cad = 2*r + f
			}
		}
		stride := get(row, iStride)
		if !math.IsNaN(stride) {
			stride /= 100
		}
		out.Records = append(out.Records, Record{
			T:             t,
			HR:            get(row, iHR),
			CadenceSpm:    cad,
			SpeedMS:       get(row, iSpd),
			PowerW:        get(row, iPw),
			AltM:          get(row, iEle),
			DistM:         get(row, iDist),
			GCTMs:         get(row, iGCT),
			VOCm:          get(row, iVO),
			VRPct:         get(row, iVR),
			StepM:         stride,
			GCTBalLeftPct: NaN, // non exposé par /details
		})
	}
	return out
}
