package analysis

import (
	"strings"
	"testing"
)

func ip(v int64) *int64 { return &v }

func knee(n int) []BodyZoneSummary {
	return []BodyZoneSummary{{Zone: ZoneKneePFPS, ZoneLabel: BodyZoneLabel(ZoneKneePFPS), EntriesCount90d: n}}
}

func TestComputeCorrelationsEmpty(t *testing.T) {
	if got := ComputeCorrelations(nil, nil, nil, nil); len(got) != 0 || got == nil {
		t.Fatalf("no zone: %+v", got)
	}
	got := ComputeCorrelations(knee(1), []BodyStressEntry{{Zone: ZoneKneePFPS, Date: "2026-05-01"}}, nil, nil)
	if len(got) != 2 {
		t.Fatalf("knee factors: %+v", got)
	}
	for _, c := range got {
		if !c.InsufficientData || c.Confidence != "insufficient" || c.SymptomaticMean != nil || c.BaselineMean != nil ||
			c.DeltaPct != nil || c.Summary != nil || c.ShoeHighlight != nil || c.SymptomaticRuns != 0 || c.BaselineRuns != 0 {
			t.Fatalf("no run must give an empty factor: %+v", c)
		}
	}
}

func TestComputeCorrelationsInsufficient(t *testing.T) {
	entries := []BodyStressEntry{{Zone: ZoneShinSplints, Date: "2026-05-01"}}
	acts := []ActivityBioSummary{
		{ID: 1, Date: "2026-05-01", IsRunning: true, GctMs: fp(260)},
		{ID: 2, Date: "2026-04-28", IsRunning: true, GctMs: fp(240)},
		{ID: 3, Date: "2026-04-25", IsRunning: true, GctMs: fp(242)},
		{ID: 4, Date: "2026-04-22", IsRunning: true, GctMs: fp(244)},
	}
	got := ComputeCorrelations([]BodyZoneSummary{{Zone: ZoneShinSplints, EntriesCount90d: 1}}, entries, nil, acts)
	if len(got) != 1 {
		t.Fatalf("factors: %+v", got)
	}
	c := got[0]
	if c.Factor != "gct" || !c.InsufficientData || c.SymptomaticRuns != 1 || c.BaselineRuns != 3 || c.SymptomaticMean != nil || c.Summary != nil {
		t.Fatalf("one symptomatic run is not enough: %+v", c)
	}
}

func TestComputeCorrelationsRealData(t *testing.T) {
	entries := []BodyStressEntry{
		{Zone: ZoneKneePFPS, Date: "2026-05-10", ActivityID: ip(1)},
		{Zone: ZoneKneePFPS, Date: "2026-05-03"},
	}
	shoes := []ShoeBioSummary{{ID: 7, Name: "Vieille paire", DropMm: fp(8), DistanceKm: 600}, {ID: 8, Name: "Neuve", DropMm: fp(4), DistanceKm: 50}}
	acts := []ActivityBioSummary{
		{ID: 1, Date: "2026-05-10", IsRunning: true, ElevLossM: fp(300), DistanceKm: fp(20), ShoeID: ip(7)},
		{ID: 2, Date: "2026-05-03", IsRunning: true, ElevLossM: fp(200), DistanceKm: fp(10), ShoeID: ip(7)},
		{ID: 3, Date: "2026-05-02", IsRunning: true, ElevLossM: fp(50), DistanceKm: fp(10), ShoeID: ip(8)},
		{ID: 4, Date: "2026-05-01", IsRunning: true, ElevLossM: fp(0), DistanceKm: fp(10), ShoeID: ip(8)},
		{ID: 5, Date: "2026-04-30", IsRunning: true, ElevLossM: fp(100), DistanceKm: fp(10), ShoeID: ip(8)},
		{ID: 6, Date: "2026-04-29", IsRunning: false, ElevLossM: fp(900), DistanceKm: fp(5), ShoeID: ip(7)},
	}
	got := ComputeCorrelations(knee(2), entries, shoes, acts)
	if len(got) != 2 {
		t.Fatalf("factors: %+v", got)
	}
	elev, wear := got[0], got[1]
	if elev.Factor != "elev_loss" || elev.InsufficientData || elev.SymptomaticRuns != 2 || elev.BaselineRuns != 3 || elev.Confidence != "low" ||
		*elev.SymptomaticMean != 250 || *elev.BaselineMean != 50 || *elev.DeltaPct != 400 {
		t.Fatalf("elev_loss: %+v", elev)
	}
	if elev.Summary == nil || !strings.Contains(*elev.Summary, "250 m") || !strings.Contains(*elev.Summary, "50 m") || !strings.Contains(*elev.Summary, "+400 %") {
		t.Fatalf("summary must quote the computed numbers: %v", elev.Summary)
	}
	// wear at the run: 600 km now, minus later runs on the same pair (run 1: 600, run 2: 580)
	if wear.Factor != "shoe_wear" || *wear.SymptomaticMean != 590 || wear.ShoeHighlight == nil || wear.ShoeHighlight.ShoeID != 7 ||
		wear.ShoeHighlight.PainEpisodes != 2 {
		t.Fatalf("shoe_wear: %+v %+v", wear, wear.ShoeHighlight)
	}
	// baseline: pair 8 at 50, 40 and 30 km
	if *wear.BaselineMean != 40 {
		t.Fatalf("shoe_wear baseline: %v", *wear.BaselineMean)
	}
}

func TestCorrConfidenceThresholds(t *testing.T) {
	for _, c := range []struct {
		sym, base int
		want      string
	}{{1, 20, "insufficient"}, {2, 2, "insufficient"}, {2, 3, "low"}, {3, 4, "low"}, {3, 5, "moderate"}, {5, 9, "moderate"}, {5, 10, "high"}} {
		if got := corrConfidence(c.sym, c.base); got != c.want {
			t.Fatalf("%d/%d: %s, want %s", c.sym, c.base, got, c.want)
		}
	}
}
