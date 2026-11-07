package domain

import (
	"testing"

	"stride/internal/athlete"
)

func TestScaffoldPhasesLongPlan(t *testing.T) {
	// 2026-01-07 is a Wednesday: the plan starts on Monday 2026-01-05 and spans 14 weeks.
	phases := scaffoldPhases("2026-01-07", "2026-04-12", 80, nil)
	if len(phases) != 4 || phases[0].start != "2026-01-05" || phases[3].end != "2026-04-12" || phases[3].phaseType != PhaseTaper {
		t.Fatalf("phases: %+v", phases)
	}
	weeks, next := 0, "2026-01-05"
	for _, ph := range phases {
		if ph.start != next || ph.load != nil || len(ph.weeks) == 0 {
			t.Fatalf("phase %s not contiguous or with an invented load: %+v", ph.name, ph)
		}
		for _, w := range ph.weeks {
			weeks++
			if w.number != weeks || w.start != addDays("2026-01-05", (weeks-1)*7) || w.load != nil {
				t.Fatalf("week %d: %+v", weeks, w)
			}
			if want := ph.phaseType != PhaseTaper && w.number%4 == 0; w.assim != want {
				t.Fatalf("week %d assimilation = %v", w.number, w.assim)
			}
			if w.assim && w.volumeKm != float64(int(ph.volumeKm*0.74*10+0.5))/10 {
				t.Fatalf("assimilation volume: %v of %v", w.volumeKm, ph.volumeKm)
			}
		}
		next = addDays(ph.end, 1)
	}
	if weeks != 14 || phases[2].volumeKm != 80 || phases[0].volumeKm != 62 {
		t.Fatalf("weeks=%d volumes=%v/%v", weeks, phases[0].volumeKm, phases[2].volumeKm)
	}
}

func TestScaffoldPhasesShortPlansAndLoad(t *testing.T) {
	phases := scaffoldPhases("2026-03-02", "2026-03-22", 40, fp(300))
	if len(phases) != 2 || len(phases[0].weeks)+len(phases[1].weeks) != 3 || phases[1].phaseType != PhaseTaper {
		t.Fatalf("3-week plan: %+v", phases)
	}
	if phases[0].load == nil || *phases[0].load != 285 || phases[0].weeks[0].load == nil {
		t.Fatalf("load targets follow the given peak: %+v", phases[0])
	}
	if one := scaffoldPhases("2026-03-02", "2026-03-04", 40, nil); len(one) != 1 || len(one[0].weeks) != 1 {
		t.Fatalf("1-week plan: %+v", one)
	}
}

func TestWeekActualsUseMatchingAndMeasuredLoad(t *testing.T) {
	act := func(id int64, sport, typ string, km, load *float64) *activityRow {
		a := &activityRow{ID: id, Sport: sport, Type: typ, DurationS: fp(1800), Load: load}
		if km != nil {
			m := *km * 1000
			a.DistanceM = &m
		}
		return a
	}
	done := int64(1)
	planned := []athlete.PlannedSession{
		{Sport: "run", TargetDistanceM: fp(10000), Match: &athlete.PlanMatch{ActivityID: &done}},
		{Sport: "run", TargetDistanceM: fp(8000), Match: &athlete.PlanMatch{}},
		{Sport: "rest"},
	}
	acts := []*activityRow{act(1, "run", "running", fp(10), fp(80)), act(2, "bike", "cycling", fp(20), nil), act(3, "run", "running", fp(5), nil)}
	got := weekActuals(planned, acts, nil, nil, nil)
	if got.SessionsPlanned != 2 || got.SessionsCompleted != 1 {
		t.Fatalf("sessions: %+v", got)
	}
	if got.ActualLoad != 80 || got.PlannedLoad != 0 || got.LoadCompletionPct != nil {
		t.Fatalf("load must be measured only: %+v", got)
	}
	if got.PlannedVolumeKm != 18 || got.ActualVolumeKm != 15 || got.VolumeCompletionPct == nil || *got.VolumeCompletionPct != 83.3 {
		t.Fatalf("volume: %+v", got)
	}
}
