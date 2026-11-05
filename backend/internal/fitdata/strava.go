package fitdata

import (
	"stride/internal/provider/strava"
)

// FromStrava builds records from Strava streams. Strava run cadence is one-leg rpm (×2 → spm);
// running dynamics (GCT, OV, stride, balance) do not exist there and stay NaN.
func FromStrava(act *strava.Activity, st *strava.Streams, isRunning bool) *Activity {
	out := &Activity{Source: "strava", Session: EmptySession()}
	if act != nil {
		out.Start = act.StartDate
		out.Sport = act.SportType
		out.Session.ElapsedS = act.ElapsedTimeS
		out.Session.TimerS = act.MovingTimeS
		out.Session.DistM = act.DistanceM
		out.Session.AscentM = act.ElevationGainM
		if act.AverageHeartrate != nil {
			out.Session.AvgHR = *act.AverageHeartrate
		}
		if act.MaxHeartrate != nil {
			out.Session.MaxHR = *act.MaxHeartrate
		}
		if act.AverageWatts != nil {
			out.Session.AvgPower = *act.AverageWatts
		}
		if act.WeightedAverageWatts != nil {
			out.Session.NormPower = *act.WeightedAverageWatts
		}
		if act.AverageCadenceRaw != nil {
			out.Session.AvgCadSpm = *act.AverageCadenceRaw
			if isRunning {
				out.Session.AvgCadSpm *= 2
			}
		}
		if len(act.StartLatLng) == 2 {
			out.Session.StartLat, out.Session.StartLon = degrees(act.StartLatLng[0]), degrees(act.StartLatLng[1])
		}
	}
	if st == nil {
		return out
	}
	at := func(s []float64, i int) float64 {
		if i < len(s) {
			return s[i]
		}
		return NaN
	}
	n := len(st.TimeS)
	out.Records = make([]Record, 0, n)
	for i := 0; i < n; i++ {
		r := emptyRecord(st.TimeS[i])
		r.HR = at(st.HeartRate, i)
		if r.HR == 0 {
			r.HR = NaN
		}
		r.CadenceSpm = at(st.CadenceRaw, i)
		if isRunning && r.CadenceSpm == r.CadenceSpm {
			r.CadenceSpm *= 2
		}
		r.PowerW = at(st.Watts, i)
		r.AltM = at(st.AltitudeM, i)
		r.DistM = at(st.DistanceM, i)
		r.SpeedMS = at(st.VelocityMS, i)
		r.GradePct = at(st.GradePct, i)
		if i < len(st.Moving) && !st.Moving[i] && r.SpeedMS == r.SpeedMS && r.SpeedMS > 0.5 {
			r.SpeedMS = 0
		}
		out.Records = append(out.Records, r)
	}
	return out
}
