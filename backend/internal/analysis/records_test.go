package analysis

import (
	"math"
	"testing"

	"stride/internal/fitdata"
)

func track(speed func(t int) float64, n int) *fitdata.Activity {
	act := &fitdata.Activity{}
	d := 0.0
	for t := 0; t < n; t++ {
		if t > 0 {
			d += speed(t)
		}
		act.Records = append(act.Records, fitdata.Record{T: float64(t), DistM: d, HR: math.NaN()})
	}
	return act
}

func TestBestEfforts(t *testing.T) {
	dists := map[string]float64{"1k": 1000, "5k": 5000, "10k": 10000}
	// 3 m/s with a 400 s block at 4 m/s: the fastest km is inside the block (250 s).
	act := track(func(t int) float64 {
		if t > 1000 && t <= 1400 {
			return 4
		}
		return 3
	}, 2400)
	got := BestEfforts(act, dists)
	if got["1k"] != 250 {
		t.Fatalf("1k = %v", got["1k"])
	}
	if _, ok := got["10k"]; ok {
		t.Fatalf("10k on a 7.6 km run: %v", got)
	}
	// 5 km: 1600 m at 4 m/s (400 s) + 3400 m at 3 m/s (1133.3 s) = 1533 s
	if got["5k"] != 1533 {
		t.Fatalf("5k = %v", got["5k"])
	}
	// A GPS jump of 3 km in one second never makes a record; NaN distances are skipped.
	glitch := track(func(t int) float64 {
		if t == 500 {
			return 3000
		}
		return 2.5
	}, 1200)
	glitch.Records[10].DistM = math.NaN()
	g := BestEfforts(glitch, dists)
	if g["1k"] != 400 {
		t.Fatalf("glitch 1k = %v", g["1k"])
	}
	if _, ok := g["5k"]; ok {
		t.Fatalf("glitch made a 5k: %v", g)
	}
	if len(BestEfforts(nil, dists)) != 0 {
		t.Fatal("nil activity")
	}
	if v, ok := WholeEffort(10100, 2727, 10000); !ok || v != 2700 {
		t.Fatalf("whole effort = %v %v", v, ok)
	}
	if _, ok := WholeEffort(12000, 3600, 10000); ok {
		t.Fatal("12 km is not a 10 km")
	}
}
