package workout

import (
	"bytes"
	"errors"
	"math"
	"time"

	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
)

var fitIntensity = map[string]typedef.Intensity{
	"warmup": typedef.IntensityWarmup, "cooldown": typedef.IntensityCooldown, "interval": typedef.IntensityInterval,
	"run": typedef.IntensityActive, "recovery": typedef.IntensityRecovery, "rest": typedef.IntensityRest,
}

func u32(v float64) uint32 {
	if v < 0 || math.IsNaN(v) {
		return 0
	}
	return uint32(math.Round(v))
}

func fitTarget(ws *mesgdef.WorkoutStep, t *Target) {
	ws.TargetType = typedef.WktStepTargetOpen
	if t == nil {
		return
	}
	lo, hi := t.Min, t.Max
	if lo == nil {
		lo = hi
	}
	if hi == nil {
		hi = lo
	}
	if lo == nil {
		return
	}
	ws.TargetValue = 0
	switch t.Kind {
	case "pace":
		// custom speed range, m/s × 1000; the slowest pace is the low speed
		ws.TargetType = typedef.WktStepTargetSpeed
		ws.CustomTargetValueLow, ws.CustomTargetValueHigh = u32(1000 / *hi * 1000), u32(1000 / *lo * 1000)
	case "hr":
		// values above 100 are absolute bpm (+100)
		ws.TargetType = typedef.WktStepTargetHeartRate
		ws.CustomTargetValueLow, ws.CustomTargetValueHigh = u32(*lo+100), u32(*hi+100)
	case "cadence":
		ws.TargetType = typedef.WktStepTargetCadence
		ws.CustomTargetValueLow, ws.CustomTargetValueHigh = u32(*lo), u32(*hi)
	case "power":
		// values above 1000 are absolute watts (+1000)
		ws.TargetType = typedef.WktStepTargetPower
		ws.CustomTargetValueLow, ws.CustomTargetValueHigh = u32(*lo+1000), u32(*hi+1000)
	}
}

func fitSteps(steps []Step, out *[]*mesgdef.WorkoutStep) {
	for _, st := range steps {
		if st.Type == "repeat" {
			from := len(*out)
			fitSteps(st.Steps, out)
			n := 1
			if st.Repeat != nil {
				n = *st.Repeat
			}
			ws := mesgdef.NewWorkoutStep(nil)
			ws.MessageIndex = typedef.MessageIndex(len(*out))
			ws.DurationType = typedef.WktStepDurationRepeatUntilStepsCmplt
			ws.DurationValue = uint32(from)
			ws.TargetType = typedef.WktStepTargetOpen
			ws.TargetValue = uint32(n)
			*out = append(*out, ws)
			continue
		}
		ws := mesgdef.NewWorkoutStep(nil)
		ws.MessageIndex = typedef.MessageIndex(len(*out))
		ws.Intensity = fitIntensity[st.Type]
		switch {
		case st.DurationS != nil:
			ws.DurationType, ws.DurationValue = typedef.WktStepDurationTime, u32(*st.DurationS*1000)
		case st.DistanceM != nil:
			ws.DurationType, ws.DurationValue = typedef.WktStepDurationDistance, u32(*st.DistanceM*100)
		default:
			ws.DurationType = typedef.WktStepDurationOpen
		}
		fitTarget(ws, st.Target)
		if st.Note != "" {
			ws.Notes = truncate(st.Note, 50)
		}
		*out = append(*out, ws)
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// EncodeFIT writes a FIT workout file (file_id type workout, workout + workout_step messages).
func EncodeFIT(name, sport string, steps []Step, created time.Time) ([]byte, error) {
	if len(steps) == 0 {
		return nil, errors.New("aucune étape structurée")
	}
	f := filedef.NewWorkout()
	f.FileId = *mesgdef.NewFileId(nil)
	f.FileId.Type = typedef.FileWorkout
	f.FileId.Manufacturer = typedef.ManufacturerDevelopment
	f.FileId.Product = 1
	f.FileId.SerialNumber = uint32(created.Unix())
	f.FileId.TimeCreated = created
	fitSteps(steps, &f.WorkoutSteps)
	w := mesgdef.NewWorkout(nil)
	w.WktName = truncate(name, 40)
	w.Sport = typedef.SportRunning
	if sport == "bike" {
		w.Sport = typedef.SportCycling
	}
	w.NumValidSteps = uint16(len(f.WorkoutSteps))
	f.Workout = w
	fit := f.ToFIT(nil)
	var buf bytes.Buffer
	if err := encoder.New(&buf).Encode(&fit); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
