// Package fitdata converts FIT files, Garmin detail series and Strava streams into one compact
// record model without GPS tracks. Absent values are NaN.
package fitdata

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
)

var NaN = math.NaN()

type Activity struct {
	Source        string // fit | details | strava
	Start         time.Time
	Sport         string
	SubSport      string
	Session       Session
	Records       []Record
	Laps          []Lap
	Splits        []Split
	HRZonesS      []float64
	PowerZonesS   []float64
	HRZoneHighBpm []float64
}

// Record is one sample; T is wall-clock seconds since Start.
type Record struct {
	T               float64
	HR              float64
	CadenceSpm      float64
	SpeedMS         float64
	PowerW          float64
	AltM            float64
	DistM           float64
	GCTMs           float64
	VOCm            float64
	VRPct           float64
	StepM           float64
	GCTBalLeftPct   float64
	GAPSpeedMS      float64
	StaminaPct      float64 // potential stamina
	AvailStaminaPct float64
	GradePct        float64
}

func emptyRecord(t float64) Record {
	return Record{T: t, HR: NaN, CadenceSpm: NaN, SpeedMS: NaN, PowerW: NaN, AltM: NaN, DistM: NaN, GCTMs: NaN, VOCm: NaN,
		VRPct: NaN, StepM: NaN, GCTBalLeftPct: NaN, GAPSpeedMS: NaN, StaminaPct: NaN, AvailStaminaPct: NaN, GradePct: NaN}
}

type Lap struct {
	StartT, EndT      float64
	TimerS, ElapsedS  float64
	DistM             float64
	AvgSpeed          float64
	AvgHR, MaxHR      float64
	AvgPower          float64
	AvgCadSpm         float64
	AvgGCTMs          float64
	AvgVOCm           float64
	AvgGCTBal         float64
	AscentM, DescentM float64
	Intensity         string // active|rest|warmup|cooldown|recovery|interval|other
	Trigger           string // manual|time|distance|session_end…
	HasWorkoutStep    bool
}

// Split is a FIT split (rwd_run, rwd_walk, rwd_stand, interval_*…).
type Split struct {
	Type              string
	StartT, EndT      float64
	TimerS, ElapsedS  float64
	DistM             float64
	AvgSpeed          float64
	AscentM, DescentM float64
}

type Session struct {
	TimerS, ElapsedS, DistM     float64
	AvgHR, MaxHR                float64
	AvgPower, NormPower         float64
	AvgCadSpm, MaxCadSpm        float64
	AvgGCTMs, AvgVOCm, AvgVRPct float64
	AvgStepM, AvgGCTBal         float64
	AscentM, DescentM           float64
	StartLat, StartLon          float64
}

func EmptySession() Session {
	return Session{TimerS: NaN, ElapsedS: NaN, DistM: NaN, AvgHR: NaN, MaxHR: NaN, AvgPower: NaN, NormPower: NaN,
		AvgCadSpm: NaN, MaxCadSpm: NaN, AvgGCTMs: NaN, AvgVOCm: NaN, AvgVRPct: NaN, AvgStepM: NaN, AvgGCTBal: NaN,
		AscentM: NaN, DescentM: NaN, StartLat: NaN, StartLon: NaN}
}

func (a *Activity) HasGCTBalance() bool {
	for i := range a.Records {
		if !math.IsNaN(a.Records[i].GCTBalLeftPct) {
			return true
		}
	}
	return !math.IsNaN(a.Session.AvgGCTBal)
}

// Has reports whether at least one record has a valid value for get.
func (a *Activity) Has(get func(Record) float64) bool {
	for i := range a.Records {
		if v := get(a.Records[i]); !math.IsNaN(v) && !math.IsInf(v, 0) {
			return true
		}
	}
	return false
}

// Decode decodes the first activity found in a .fit file.
func Decode(b []byte) (*Activity, error) {
	dec := decoder.New(bytes.NewReader(b))
	for dec.Next() {
		f, err := dec.Decode()
		if err != nil {
			return nil, fmt.Errorf("fit decode: %w", err)
		}
		act := filedef.NewActivity(f.Messages...)
		if len(act.Records) == 0 && len(act.Sessions) == 0 {
			continue
		}
		return convert(act), nil
	}
	return nil, errors.New("fit: no activity in file")
}

func u8(v uint8) float64 {
	if v == math.MaxUint8 {
		return NaN
	}
	return float64(v)
}

func u16(v uint16) float64 {
	if v == math.MaxUint16 {
		return NaN
	}
	return float64(v)
}

func u32s(v uint32, scale float64) float64 {
	if v == math.MaxUint32 {
		return NaN
	}
	return float64(v) / scale
}

func u16s(v uint16, scale float64) float64 {
	if v == math.MaxUint16 {
		return NaN
	}
	return float64(v) / scale
}

func firstValid(vs ...float64) float64 {
	for _, v := range vs {
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			return v
		}
	}
	return NaN
}

func degrees(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 180 {
		return NaN
	}
	return v
}

// cadenceSpm converts FIT running cadence (strides/min + fraction) to steps/min.
func cadenceSpm(c uint8, frac float64) float64 {
	if c == math.MaxUint8 {
		return NaN
	}
	if math.IsNaN(frac) {
		frac = 0
	}
	return 2 * (float64(c) + frac)
}

func offset(t, start time.Time) float64 {
	if t.IsZero() || start.IsZero() {
		return NaN
	}
	return t.Sub(start).Seconds()
}

func convert(act *filedef.Activity) *Activity {
	out := &Activity{Source: "fit"}
	var s *mesgdef.Session
	if len(act.Sessions) > 0 {
		s = act.Sessions[0]
		out.Start = s.StartTime
		out.Sport = s.Sport.String()
		out.SubSport = s.SubSport.String()
	}
	if out.Start.IsZero() && len(act.Records) > 0 {
		out.Start = act.Records[0].Timestamp
	}
	out.Session = EmptySession()
	if s != nil {
		out.Session = Session{
			TimerS:    u32s(s.TotalTimerTime, 1000),
			ElapsedS:  u32s(s.TotalElapsedTime, 1000),
			DistM:     u32s(s.TotalDistance, 100),
			AvgHR:     u8(s.AvgHeartRate),
			MaxHR:     u8(s.MaxHeartRate),
			AvgPower:  u16(s.AvgPower),
			NormPower: u16(s.NormalizedPower),
			AvgCadSpm: cadenceSpm(s.AvgCadence, s.AvgFractionalCadenceScaled()),
			MaxCadSpm: cadenceSpm(s.MaxCadence, s.MaxFractionalCadenceScaled()),
			AvgGCTMs:  s.AvgStanceTimeScaled(),
			AvgVOCm:   s.AvgVerticalOscillationScaled() / 10,
			AvgVRPct:  s.AvgVerticalRatioScaled(),
			AvgStepM:  s.AvgStepLengthScaled() / 1000,
			AvgGCTBal: s.AvgStanceTimeBalanceScaled(),
			AscentM:   u16(s.TotalAscent),
			DescentM:  u16(s.TotalDescent),
			StartLat:  degrees(s.StartPositionLatDegrees()),
			StartLon:  degrees(s.StartPositionLongDegrees()),
		}
	}

	out.Records = make([]Record, 0, len(act.Records))
	for _, r := range act.Records {
		if r.Timestamp.IsZero() {
			continue
		}
		rec := emptyRecord(offset(r.Timestamp, out.Start))
		rec.HR = u8(r.HeartRate)
		rec.CadenceSpm = cadenceSpm(r.Cadence, r.FractionalCadenceScaled())
		rec.SpeedMS = firstValid(r.EnhancedSpeedScaled(), r.SpeedScaled())
		rec.PowerW = u16(r.Power)
		rec.AltM = firstValid(r.EnhancedAltitudeScaled(), r.AltitudeScaled())
		rec.DistM = r.DistanceScaled()
		rec.GCTMs = r.StanceTimeScaled()
		rec.VOCm = r.VerticalOscillationScaled() / 10
		rec.VRPct = r.VerticalRatioScaled()
		rec.StepM = r.StepLengthScaled() / 1000
		rec.GCTBalLeftPct = r.StanceTimeBalanceScaled()
		if rec.HR == 0 {
			rec.HR = NaN
		}
		out.Records = append(out.Records, rec)
	}

	for _, l := range act.Laps {
		lap := Lap{
			StartT:         offset(l.StartTime, out.Start),
			EndT:           offset(l.Timestamp, out.Start),
			TimerS:         u32s(l.TotalTimerTime, 1000),
			ElapsedS:       u32s(l.TotalElapsedTime, 1000),
			DistM:          u32s(l.TotalDistance, 100),
			AvgSpeed:       firstValid(u32s(l.EnhancedAvgSpeed, 1000), u16s(l.AvgSpeed, 1000)),
			AvgHR:          u8(l.AvgHeartRate),
			MaxHR:          u8(l.MaxHeartRate),
			AvgPower:       u16(l.AvgPower),
			AvgCadSpm:      cadenceSpm(l.AvgCadence, l.AvgFractionalCadenceScaled()),
			AvgGCTMs:       l.AvgStanceTimeScaled(),
			AvgVOCm:        l.AvgVerticalOscillationScaled() / 10,
			AvgGCTBal:      l.AvgStanceTimeBalanceScaled(),
			AscentM:        u16(l.TotalAscent),
			DescentM:       u16(l.TotalDescent),
			Intensity:      intensityName(l.Intensity),
			Trigger:        l.LapTrigger.String(),
			HasWorkoutStep: l.WktStepIndex != typedef.MessageIndexInvalid,
		}
		if math.IsNaN(lap.EndT) && !math.IsNaN(lap.StartT) && !math.IsNaN(lap.ElapsedS) {
			lap.EndT = lap.StartT + lap.ElapsedS
		}
		out.Laps = append(out.Laps, lap)
	}

	for _, sp := range act.Splits {
		out.Splits = append(out.Splits, Split{
			Type:     sp.SplitType.String(),
			StartT:   offset(sp.StartTime, out.Start),
			EndT:     offset(sp.EndTime, out.Start),
			TimerS:   u32s(sp.TotalTimerTime, 1000),
			ElapsedS: u32s(sp.TotalElapsedTime, 1000),
			DistM:    u32s(sp.TotalDistance, 100),
			AvgSpeed: u32s(sp.AvgSpeed, 1000),
			AscentM:  u16(sp.TotalAscent),
			DescentM: u16(sp.TotalDescent),
		})
	}

	for _, z := range act.TimeInZones {
		if z.ReferenceMesg != typedef.MesgNumSession {
			continue
		}
		if hz := zoneTimes(z.TimeInHrZone); hz != nil && out.HRZonesS == nil {
			out.HRZonesS = hz
			for _, b := range z.HrZoneHighBoundary {
				if b != math.MaxUint8 {
					out.HRZoneHighBpm = append(out.HRZoneHighBpm, float64(b))
				}
			}
		}
		if pz := zoneTimes(z.TimeInPowerZone); pz != nil && out.PowerZonesS == nil {
			out.PowerZonesS = pz
		}
	}
	return out
}

// zoneTimes converts FIT time_in_*_zone (ms) to Z1..Z5 seconds. Garmin writes N+2 buckets
// [below Z1, Z1..Z5, above]; "above" is added to Z5.
func zoneTimes(arr []uint32) []float64 {
	if len(arr) < 5 {
		return nil
	}
	vals := make([]float64, len(arr))
	found := false
	for i, v := range arr {
		if v == math.MaxUint32 {
			continue
		}
		vals[i] = float64(v) / 1000
		if vals[i] > 0 {
			found = true
		}
	}
	if !found {
		return nil
	}
	if len(vals) == 5 {
		return vals
	}
	out := append([]float64{}, vals[1:6]...)
	for _, v := range vals[6:] {
		out[4] += v
	}
	return out
}

func intensityName(i typedef.Intensity) string {
	switch i {
	case typedef.IntensityActive:
		return "active"
	case typedef.IntensityRest:
		return "rest"
	case typedef.IntensityWarmup:
		return "warmup"
	case typedef.IntensityCooldown:
		return "cooldown"
	case typedef.IntensityRecovery:
		return "recovery"
	case typedef.IntensityInterval:
		return "interval"
	case typedef.IntensityInvalid:
		return ""
	default:
		return strings.ToLower(i.String())
	}
}
