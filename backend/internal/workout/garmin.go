package workout

import "math"

type obj = map[string]any

var garminStepTypes = map[string]obj{
	"warmup":   {"stepTypeId": 1, "stepTypeKey": "warmup"},
	"cooldown": {"stepTypeId": 2, "stepTypeKey": "cooldown"},
	"interval": {"stepTypeId": 3, "stepTypeKey": "interval"},
	"run":      {"stepTypeId": 3, "stepTypeKey": "interval"},
	"recovery": {"stepTypeId": 4, "stepTypeKey": "recovery"},
	"rest":     {"stepTypeId": 5, "stepTypeKey": "rest"},
	"repeat":   {"stepTypeId": 6, "stepTypeKey": "repeat"},
}

func garminSport(sport string) obj {
	if sport == "bike" {
		return obj{"sportTypeId": 2, "sportTypeKey": "cycling"}
	}
	return obj{"sportTypeId": 1, "sportTypeKey": "running"}
}

func speedOf(paceSecPerKm *float64) any {
	if paceSecPerKm == nil || *paceSecPerKm <= 0 {
		return nil
	}
	return math.Round(1000 / *paceSecPerKm * 10000) / 10000
}

func valueOr(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func garminTarget(t *Target) obj {
	out := obj{"targetType": obj{"workoutTargetTypeId": 1, "workoutTargetTypeKey": "no.target"}}
	if t == nil {
		return out
	}
	lo, hi := t.Min, t.Max
	if lo == nil {
		lo = hi
	}
	if hi == nil {
		hi = lo
	}
	switch t.Kind {
	case "pace":
		// Garmin pace zones are speeds in m/s: slower bound first.
		out["targetType"] = obj{"workoutTargetTypeId": 6, "workoutTargetTypeKey": "pace.zone"}
		out["targetValueOne"], out["targetValueTwo"] = speedOf(hi), speedOf(lo)
	case "hr":
		out["targetType"] = obj{"workoutTargetTypeId": 4, "workoutTargetTypeKey": "heart.rate.zone"}
		out["targetValueOne"], out["targetValueTwo"] = valueOr(lo), valueOr(hi)
	case "cadence":
		out["targetType"] = obj{"workoutTargetTypeId": 3, "workoutTargetTypeKey": "cadence"}
		out["targetValueOne"], out["targetValueTwo"] = valueOr(lo), valueOr(hi)
	case "power":
		out["targetType"] = obj{"workoutTargetTypeId": 2, "workoutTargetTypeKey": "power.zone"}
		out["targetValueOne"], out["targetValueTwo"] = valueOr(lo), valueOr(hi)
	}
	return out
}

func garminSteps(steps []Step, order *int) []obj {
	out := make([]obj, 0, len(steps))
	for _, st := range steps {
		*order++
		if st.Type == "repeat" {
			n := 1
			if st.Repeat != nil {
				n = *st.Repeat
			}
			g := obj{"type": "RepeatGroupDTO", "stepOrder": *order, "stepType": garminStepTypes["repeat"], "numberOfIterations": n,
				"endCondition": obj{"conditionTypeId": 7, "conditionTypeKey": "iterations"}, "endConditionValue": n, "smartRepeat": false}
			g["workoutSteps"] = garminSteps(st.Steps, order)
			out = append(out, g)
			continue
		}
		g := obj{"type": "ExecutableStepDTO", "stepOrder": *order, "stepType": garminStepTypes[st.Type]}
		switch {
		case st.DurationS != nil:
			g["endCondition"], g["endConditionValue"] = obj{"conditionTypeId": 2, "conditionTypeKey": "time"}, *st.DurationS
		case st.DistanceM != nil:
			g["endCondition"], g["endConditionValue"] = obj{"conditionTypeId": 3, "conditionTypeKey": "distance"}, *st.DistanceM
		default:
			g["endCondition"] = obj{"conditionTypeId": 1, "conditionTypeKey": "lap.button"}
		}
		for k, v := range garminTarget(st.Target) {
			g[k] = v
		}
		if st.Note != "" {
			g["description"] = st.Note
		}
		out = append(out, g)
	}
	return out
}

// GarminWorkout builds the Garmin Connect workout-service payload.
func GarminWorkout(name, description, sport string, steps []Step) obj {
	order := 0
	sp := garminSport(sport)
	return obj{
		"workoutName": name, "description": description, "sportType": sp,
		"workoutSegments": []obj{{"segmentOrder": 1, "sportType": sp, "workoutSteps": garminSteps(steps, &order)}},
	}
}
