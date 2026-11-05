package domain

import (
	"strings"
	"testing"

	"stride/internal/athlete"
	"stride/internal/workout"
)

func fp(v float64) *float64 { return &v }

func TestAdaptRulesWeekCap(t *testing.T) {
	// 2026-11-02 is a Monday. Unavailable Thu 05–Sat 07: the Thursday session moves to Sunday 08
	// (Friday and Saturday are covered); week 1 keeps its volume, so the cap does not apply.
	plan := []athlete.PlannedSession{
		{ID: 1, Date: "2026-11-03", Sport: "run", TargetDurationS: fp(3600)},
		{ID: 2, Date: "2026-11-05", Sport: "run", TargetDurationS: fp(3600)},
		{ID: 3, Date: "2026-11-10", Sport: "run", TargetDurationS: fp(1800)},
	}
	ev := athlete.CalendarEvent{Kind: "unavailable", StartDate: "2026-11-05", EndDate: "2026-11-07", Severity: 2}
	res := adaptRules(plan, []athlete.CalendarEvent{ev}, "2026-11-02")
	if len(res.Sessions) != 1 || res.Sessions[0].Date != "2026-11-08" || res.Sessions[0].ID != 0 || len(res.RemoveIDs) != 1 || res.RemoveIDs[0] != 2 {
		t.Fatalf("move within the week: %+v", res)
	}
	// Moving into week 2 (no session there in week 1's place): week 2 would reach 3600+1800 s,
	// above max(1800, 110 % of week 3 = 1980): the moved session is dropped.
	plan = []athlete.PlannedSession{
		{ID: 1, Date: "2026-11-07", Sport: "run", TargetDurationS: fp(3600)},
		{ID: 2, Date: "2026-11-08", Sport: "run", TargetDurationS: fp(3600)},
		{ID: 3, Date: "2026-11-10", Sport: "run", TargetDurationS: fp(1800)},
		{ID: 4, Date: "2026-11-17", Sport: "run", TargetDurationS: fp(1800)},
	}
	ev = athlete.CalendarEvent{Kind: "fatigue", StartDate: "2026-11-07", EndDate: "2026-11-07", Severity: 1}
	res = adaptRules(plan, []athlete.CalendarEvent{ev}, "2026-11-02")
	if len(res.Sessions) != 0 || len(res.RemoveIDs) != 1 || !strings.Contains(res.Summary, "1 supprimée") {
		t.Fatalf("week cap: %+v", res)
	}
	// A long illness (> 3 days) drops its sessions and softens the next 7 days at severity 3.
	steps := []workout.Step{{Type: "repeat", Repeat: new(int), Steps: []workout.Step{{Type: "interval", DurationS: fp(60)}}}}
	plan = []athlete.PlannedSession{
		{ID: 1, Date: "2026-11-03", Sport: "run", TargetDurationS: fp(3600)},
		{ID: 2, Date: "2026-11-09", Sport: "run", TargetDurationS: fp(3000), Steps: steps, Title: "VMA"},
		{ID: 3, Date: "2026-11-13", Sport: "strength", TargetDurationS: fp(1800)},
		{ID: 4, Date: "2026-11-14", Sport: "run", TargetDurationS: fp(1800)},
	}
	ev = athlete.CalendarEvent{Kind: "illness", StartDate: "2026-11-02", EndDate: "2026-11-06", Severity: 3}
	res = adaptRules(plan, []athlete.CalendarEvent{ev}, "2026-11-02")
	if len(res.RemoveIDs) != 3 || len(res.Sessions) != 2 {
		t.Fatalf("long illness: %+v", res)
	}
	if s := res.Sessions[0]; s.Title != "Footing facile" || *s.TargetDurationS != 1800 || len(s.Steps) != 0 {
		t.Fatalf("softened intervals: %+v", s)
	}
	if s := res.Sessions[1]; *s.TargetDurationS != 1080 || s.Sport != "strength" {
		t.Fatalf("softened strength: %+v", s)
	}
	if !strings.Contains(res.Summary, "Reprise progressive sur 7 jours") {
		t.Fatalf("summary: %s", res.Summary)
	}
	if res := adaptRules(plan, nil, "2026-11-02"); len(res.RemoveIDs) != 0 || !strings.Contains(res.Summary, "rien à adapter") {
		t.Fatalf("no event: %+v", res)
	}
}
