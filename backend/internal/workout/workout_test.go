package workout

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/typedef"
)

func fp(v float64) *float64 { return &v }
func ip(v int) *int         { return &v }

func sample() []Step {
	return []Step{
		{Type: "warmup", DurationS: fp(600), Target: &Target{Kind: "pace", Min: fp(330), Max: fp(360)}},
		{Type: "repeat", Repeat: ip(6), Steps: []Step{
			{Type: "interval", DurationS: fp(60), Target: &Target{Kind: "pace", Min: fp(250), Max: fp(260)}},
			{Type: "recovery", DurationS: fp(90)},
		}},
		{Type: "cooldown", DistanceM: fp(1000), Target: &Target{Kind: "hr", Min: fp(120), Max: fp(140)}},
	}
}

func TestValidate(t *testing.T) {
	if errs := Validate(sample(), ""); len(errs) != 0 {
		t.Fatal(errs)
	}
	bad := []Step{{Type: "sprint"}, {Type: "repeat", Repeat: ip(0)}, {Type: "run", DurationS: fp(1), DistanceM: fp(2)},
		{Type: "run", Target: &Target{Kind: "pace", Min: fp(20)}}}
	errs := Validate(bad, "Séance 1, ")
	if len(errs) != 5 || !strings.Contains(errs[0], "type inconnu « sprint »") || !strings.HasPrefix(errs[0], "Séance 1, étape 1") {
		t.Fatalf("%d %q", len(errs), errs)
	}
	if d := TotalDurationS(sample(), 300); d != 600+6*150+300 {
		t.Fatalf("duration %v", d)
	}
}

func TestGarminWorkout(t *testing.T) {
	raw, _ := json.Marshal(GarminWorkout("Footing", "", "run", sample()))
	s := string(raw)
	for _, want := range []string{`"sportTypeKey":"running"`, `"type":"RepeatGroupDTO"`, `"numberOfIterations":6`,
		`"conditionTypeKey":"time"`, `"conditionTypeKey":"distance"`, `"workoutTargetTypeKey":"pace.zone"`,
		`"workoutTargetTypeKey":"heart.rate.zone"`, `"workoutTargetTypeKey":"no.target"`, `"stepTypeKey":"cooldown"`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
}

func TestEncodeFIT(t *testing.T) {
	b, err := EncodeFIT("Footing + 6×1'", "run", sample(), time.Unix(1_800_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	dec := decoder.New(bytes.NewReader(b))
	f, err := dec.Decode()
	if err != nil {
		t.Fatal(err)
	}
	w := filedef.NewWorkout(f.Messages...)
	if w.FileId.Type != typedef.FileWorkout || w.Workout == nil || w.Workout.NumValidSteps != 5 || len(w.WorkoutSteps) != 5 {
		t.Fatalf("workout: %+v steps=%d", w.Workout, len(w.WorkoutSteps))
	}
	rep := w.WorkoutSteps[3]
	if rep.DurationType != typedef.WktStepDurationRepeatUntilStepsCmplt || rep.DurationValue != 1 || rep.TargetValue != 6 {
		t.Fatalf("repeat step: %+v", rep)
	}
	if st := w.WorkoutSteps[1]; st.DurationValue != 60000 || st.TargetType != typedef.WktStepTargetSpeed || st.CustomTargetValueHigh != 4000 {
		t.Fatalf("interval step: %+v", st)
	}
	if st := w.WorkoutSteps[4]; st.DurationType != typedef.WktStepDurationDistance || st.DurationValue != 100000 || st.CustomTargetValueLow != 220 {
		t.Fatalf("cooldown step: %+v", st)
	}
}
