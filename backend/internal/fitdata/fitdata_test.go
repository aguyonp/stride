package fitdata

import (
	"bytes"
	"math"
	"testing"
	"time"

	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"stride/internal/garmin"
)

// buildFIT encode un FIT d'activité synthétique (aucune position GPS).
func buildFIT(t *testing.T, withBalance bool) []byte {
	t.Helper()
	start := time.Date(2026, 11, 2, 17, 30, 0, 0, time.UTC)
	act := filedef.NewActivity()
	act.FileId.Type = typedef.FileActivity
	act.FileId.Manufacturer = typedef.ManufacturerDevelopment
	act.FileId.TimeCreated = start

	for i := 0; i < 120; i++ {
		r := mesgdef.NewRecord(nil)
		r.Timestamp = start.Add(time.Duration(i) * time.Second)
		r.HeartRate = uint8(130 + i/10)
		r.Cadence = 89               // strides/min
		r.FractionalCadence = 64     // 0,5 (scale 128)
		r.EnhancedSpeed = 3000       // 3,0 m/s (scale 1000)
		r.Distance = uint32(i * 300) // 3 m/s (scale 100)
		r.Power = 290
		r.EnhancedAltitude = uint32((100 + 500) * 5) // 100 m (scale 5, offset 500)
		r.VerticalOscillation = 810                  // 81,0 mm (scale 10)
		r.StanceTime = 2480                          // 248,0 ms (scale 10)
		r.VerticalRatio = 760                        // 7,60 % (scale 100)
		r.StepLength = 10500                         // 1050,0 mm (scale 10)
		if withBalance {
			r.StanceTimeBalance = 4920 // 49,20 % gauche (scale 100)
		}
		act.Records = append(act.Records, r)
	}
	ses := mesgdef.NewSession(nil)
	ses.Timestamp = start.Add(120 * time.Second)
	ses.StartTime = start
	ses.Sport = typedef.SportRunning
	ses.TotalTimerTime = 120000
	ses.TotalElapsedTime = 120000
	ses.TotalDistance = 36000
	ses.AvgCadence = 89
	ses.AvgFractionalCadence = 64
	ses.TotalAscent = 3
	ses.TotalDescent = 4
	act.Sessions = append(act.Sessions, ses)

	lap := mesgdef.NewLap(nil)
	lap.Timestamp = start.Add(120 * time.Second)
	lap.StartTime = start
	lap.TotalTimerTime = 120000
	lap.TotalElapsedTime = 120000
	lap.TotalDistance = 36000
	lap.AvgHeartRate = 135
	lap.AvgCadence = 89
	lap.AvgFractionalCadence = 64
	lap.Intensity = typedef.IntensityInterval
	lap.LapTrigger = typedef.LapTriggerDistance
	act.Laps = append(act.Laps, lap)

	sp := mesgdef.NewSplit(nil)
	sp.SplitType = typedef.SplitTypeRwdRun
	sp.StartTime = start
	sp.EndTime = start.Add(100 * time.Second)
	sp.TotalTimerTime = 100000
	sp.TotalDistance = 30000
	act.Splits = append(act.Splits, sp)
	sw := mesgdef.NewSplit(nil)
	sw.SplitType = typedef.SplitTypeRwdWalk
	sw.StartTime = start.Add(100 * time.Second)
	sw.EndTime = start.Add(120 * time.Second)
	sw.TotalTimerTime = 20000
	sw.TotalElapsedTime = 20000
	sw.TotalDistance = 2400
	act.Splits = append(act.Splits, sw)

	tz := mesgdef.NewTimeInZone(nil)
	tz.Timestamp = start.Add(120 * time.Second)
	tz.ReferenceMesg = typedef.MesgNumSession
	tz.ReferenceIndex = 0
	tz.TimeInHrZone = []uint32{5000, 10000, 20000, 40000, 30000, 10000, 5000}
	tz.HrZoneHighBoundary = []uint8{90, 108, 126, 144, 162, 180}
	act.TimeInZones = append(act.TimeInZones, tz)

	fit := act.ToFIT(nil)
	var buf bytes.Buffer
	if err := encoder.New(&buf).Encode(&fit); err != nil {
		t.Fatalf("encodage FIT : %v", err)
	}
	return buf.Bytes()
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestDecodeSyntheticFIT(t *testing.T) {
	a, err := Decode(buildFIT(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if a.Source != "fit" || a.Sport != "running" || len(a.Records) != 120 || len(a.Laps) != 1 || len(a.Splits) != 2 {
		t.Fatalf("activité : sport=%s records=%d laps=%d splits=%d", a.Sport, len(a.Records), len(a.Laps), len(a.Splits))
	}
	r := a.Records[10]
	checks := []struct {
		name      string
		got, want float64
	}{
		{"T", r.T, 10},
		{"cadence spm = 2×(89+0,5)", r.CadenceSpm, 179},
		{"vitesse", r.SpeedMS, 3},
		{"distance", r.DistM, 30},
		{"altitude", r.AltM, 100},
		{"OV cm", r.VOCm, 8.1},
		{"GCT ms", r.GCTMs, 248},
		{"ratio vertical", r.VRPct, 7.6},
		{"foulée m", r.StepM, 1.05},
		{"équilibre G", r.GCTBalLeftPct, 49.2},
		{"puissance", r.PowerW, 290},
		{"session cadence", a.Session.AvgCadSpm, 179},
		{"session D-", a.Session.DescentM, 4},
		{"lap cadence", a.Laps[0].AvgCadSpm, 179},
		{"split walk start", a.Splits[1].StartT, 100},
		{"split walk dist", a.Splits[1].DistM, 24},
	}
	for _, c := range checks {
		if !near(c.got, c.want) {
			t.Errorf("%s = %v, attendu %v", c.name, c.got, c.want)
		}
	}
	if a.Laps[0].Trigger != "distance" || a.Laps[0].Intensity != "interval" || a.Splits[0].Type != "rwd_run" || a.Splits[1].Type != "rwd_walk" {
		t.Errorf("énumérations : %+v %+v", a.Laps[0], a.Splits)
	}
	// Z1..Z5 : [sous Z1, Z1..Z5, au-dessus] → Z5 inclut au-dessus
	wantZ := []float64{10, 20, 40, 30, 15}
	for i, v := range wantZ {
		if !near(a.HRZonesS[i], v) {
			t.Errorf("zone %d = %v, attendu %v", i+1, a.HRZonesS[i], v)
		}
	}
	if !a.HasGCTBalance() {
		t.Error("équilibre GCT attendu")
	}
}

func TestDecodeWithoutBalance(t *testing.T) {
	a, err := Decode(buildFIT(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if a.HasGCTBalance() || !math.IsNaN(a.Records[0].GCTBalLeftPct) {
		t.Error("équilibre GCT absent attendu (NaN)")
	}
}

func TestDecodeGarbage(t *testing.T) {
	if _, err := Decode([]byte("pas un fit")); err == nil {
		t.Error("erreur attendue")
	}
}

func TestFromDetails(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	d := &garmin.ActivityDetails{MetricDescriptors: []garmin.MetricDescriptor{
		{MetricsIndex: 0, Key: "directTimestamp"}, {MetricsIndex: 1, Key: "directHeartRate"},
		{MetricsIndex: 2, Key: "directDoubleCadence"}, {MetricsIndex: 3, Key: "directSpeed"},
		{MetricsIndex: 4, Key: "directStrideLength"}, {MetricsIndex: 5, Key: "directLatitude"},
	}}
	start := time.Date(2026, 11, 2, 17, 30, 0, 0, time.UTC)
	ms := float64(start.UnixMilli())
	for i := 0; i < 3; i++ {
		d.Metrics = append(d.Metrics, struct {
			Metrics []*float64 `json:"metrics"`
		}{Metrics: []*float64{f(ms + float64(i)*1000), f(140), f(178), f(3.1), f(105), f(48.8)}})
	}
	a := FromDetails(d, start)
	if a.Source != "details" || len(a.Records) != 3 || a.Records[2].T != 2 || a.Records[0].CadenceSpm != 178 || !near(a.Records[0].StepM, 1.05) {
		t.Fatalf("details → records : %+v", a.Records)
	}
}
