package domainapi_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"stride/internal/analysis"
	"stride/internal/domain"
)

func publicGet(t *testing.T, e *env2, token string, out any) (int, string) {
	t.Helper()
	return e.do(0, "GET", "/api/public/share/"+token, nil, out)
}

func TestBodyMapRejectsForeignActivities(t *testing.T) {
	e := setup(t)
	bootGarmin(t, e)
	foreign := e.id(`SELECT id FROM activities WHERE user_id = 1 AND garmin_id = 1001`)
	in := m{"date": e.fx.day(-1), "zone": "achilles", "painScore": 3, "activityId": foreign}
	if code, body := e.do(2, "POST", "/api/bodymap/entries", in, nil); code != http.StatusNotFound || strings.Contains(body, "Sortie") {
		t.Fatalf("foreign activity accepted on create: %d %s", code, body)
	}
	var own analysis.BodyStressEntry
	if code, body := e.do(1, "POST", "/api/bodymap/entries", in, &own); code != http.StatusCreated || own.ActivityName == nil {
		t.Fatalf("own activity: %d %s", code, body)
	}
	var mine analysis.BodyStressEntry
	e.do(2, "POST", "/api/bodymap/entries", m{"date": e.fx.day(-1), "zone": "achilles", "painScore": 2}, &mine)
	if code, _ := e.do(2, "PATCH", fmt.Sprintf("/api/bodymap/entries/%d", mine.ID), m{"activityId": foreign, "painScore": 2}, nil); code != http.StatusNotFound {
		t.Fatalf("foreign activity accepted on patch: %d", code)
	}
	for _, method := range []string{"PATCH", "DELETE"} {
		if code, _ := e.do(2, method, fmt.Sprintf("/api/bodymap/entries/%d", own.ID), m{"painScore": 1}, nil); code != http.StatusNotFound {
			t.Fatalf("%s of another user's entry: %d", method, code)
		}
	}
	// a legacy row pointing at a foreign activity must not leak its name
	if _, err := e.st.DB.Exec(`UPDATE body_stress_entries SET activity_id = ? WHERE id = ?`, foreign, mine.ID); err != nil {
		t.Fatal(err)
	}
	var ov analysis.BodyMapOverview
	if code, body := e.do(2, "GET", "/api/bodymap", nil, &ov); code != 200 || len(ov.Entries) != 1 || ov.Entries[0].ActivityName != nil {
		t.Fatalf("foreign activity name leaked: %d %s", code, body)
	}
	for _, q := range []string{"days=3", "days=400", "days=abc", "zone=elbow"} {
		if code, _ := e.do(2, "GET", "/api/bodymap?"+q, nil, nil); code != http.StatusBadRequest {
			t.Fatalf("%s: %d", q, code)
		}
	}
	if code, _ := e.do(2, "GET", "/api/bodymap?days=365", nil, nil); code != 200 {
		t.Fatal("days=365 refused")
	}
}

func TestBodyMapCorrelationsWithoutData(t *testing.T) {
	e := setup(t)
	e.do(2, "POST", "/api/bodymap/entries", m{"date": e.fx.day(-1), "zone": "knee_pfps", "painScore": 4}, nil)
	var ov analysis.BodyMapOverview
	if code, body := e.do(2, "GET", "/api/bodymap", nil, &ov); code != 200 || len(ov.Correlations) != 2 {
		t.Fatalf("bodymap: %d %s", code, body)
	}
	for _, c := range ov.Correlations {
		if !c.InsufficientData || c.SymptomaticMean != nil || c.BaselineMean != nil || c.DeltaPct != nil || c.Summary != nil ||
			c.BaselineRuns != 0 || c.Confidence != "insufficient" {
			t.Fatalf("fabricated correlation for a user without runs: %+v", c)
		}
	}
}

func TestPublicShareNewUserHasNoFabricatedMetrics(t *testing.T) {
	e := setup(t)
	var link domain.ShareLink
	if code, body := e.do(3, "POST", "/api/share", m{"label": "Kiné"}, &link); code != 201 {
		t.Fatalf("create: %d %s", code, body)
	}
	var v domain.PublicShareView
	if code, body := publicGet(t, e, strings.TrimPrefix(link.URL, "/share/"), &v); code != 200 {
		t.Fatalf("public view: %d %s", code, body)
	}
	ls := v.LoadSummary
	if v.Athlete.DisplayName != "bob" || ls.EFLatest != nil || ls.EFTrendPct != nil || ls.ImpactLoad7d != nil || ls.JointIndex7d != nil ||
		ls.ACWR != nil || ls.Verdict != nil || len(v.RecentSessions) != 0 || len(v.LoadSeries) != 42 {
		t.Fatalf("new user view: %+v %+v", v.Athlete, ls)
	}
	for _, p := range v.LoadSeries {
		if p.EFSpeed != nil || p.ImpactLoad7d != nil || p.JointIndex7d != nil || p.ACWR != nil {
			t.Fatalf("fabricated series point: %+v", p)
		}
	}
}

func TestPublicShareRealMetricsCacheAndRevocation(t *testing.T) {
	e := setup(t)
	bootGarmin(t, e)
	var link domain.ShareLink
	e.do(1, "POST", "/api/share", m{"label": "Coach", "expiresInDays": 0}, &link)
	token := strings.TrimPrefix(link.URL, "/share/")
	var list []domain.ShareLink
	if e.do(1, "GET", "/api/share", nil, &list); len(list) != 1 || list[0].URL != link.URL || link.ExpiresAt != nil {
		t.Fatalf("list must reopen the sealed URL: %+v", list)
	}
	var v domain.PublicShareView
	if code, body := publicGet(t, e, token, &v); code != 200 || len(v.RecentSessions) == 0 {
		t.Fatalf("public view: %d %s", code, body)
	}
	if v.Athlete.DisplayName != "admin" {
		t.Fatalf("display name: %q", v.Athlete.DisplayName)
	}
	// session metrics are the activity's own (Garmin impact load, persisted EF and joint index)
	for _, it := range v.RecentSessions {
		if it.ID != e.id(`SELECT id FROM activities WHERE garmin_id = 1001`) {
			continue
		}
		var av m
		e.do(1, "GET", fmt.Sprintf("/api/activities/%d", it.ID), nil, &av)
		ef, imp := av["efficiency"].(m)["efSpeed"], av["impact"].(m)["jointIndex"]
		if it.ImpactLoad == nil || *it.ImpactLoad != 1234 || it.EfSpeed == nil || *it.EfSpeed != ef || it.JointIndex == nil || *it.JointIndex != imp {
			t.Fatalf("session metrics: impact=%v ef=%v/%v joint=%v/%v", it.ImpactLoad, it.EfSpeed, ef, it.JointIndex, imp)
		}
	}
	if v.LoadSummary.ImpactLoad7d == nil || *v.LoadSummary.ImpactLoad7d < 1234 || v.LoadSummary.JointIndex7d == nil {
		t.Fatalf("7-day sums: %+v", v.LoadSummary)
	}
	// cached: an owner rename is not visible before the TTL
	if _, err := e.st.DB.Exec(`UPDATE users SET display_name = 'renamed' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if publicGet(t, e, token, &v); v.Athlete.DisplayName != "admin" {
		t.Fatal("public view not cached")
	}
	if _, err := e.st.DB.Exec(`UPDATE users SET status = 'disabled' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if code, _ := publicGet(t, e, token, nil); code != http.StatusNotFound {
		t.Fatalf("disabled owner's link: %d", code)
	}
	if _, err := e.st.DB.Exec(`UPDATE users SET status = 'active' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if code, _ := e.do(1, "DELETE", fmt.Sprintf("/api/share/%d", link.ID), nil, nil); code != 204 {
		t.Fatal("revoke")
	}
	if code, _ := publicGet(t, e, token, nil); code != http.StatusNotFound {
		t.Fatalf("revoked link served from cache: %d", code)
	}
	if code, _ := e.do(2, "DELETE", fmt.Sprintf("/api/share/%d", link.ID), nil, nil); code != http.StatusNotFound {
		t.Fatal("revocation of another user's link")
	}
}

func TestShareLinkValidationAndRateLimit(t *testing.T) {
	e := setup(t)
	for _, in := range []m{{"label": "Kiné", "expiresAt": "demain"}, {"label": "Kiné", "expiresInDays": -1}, {"label": "K"}} {
		if code, _ := e.do(2, "POST", "/api/share", in, nil); code != http.StatusBadRequest {
			t.Fatalf("%v accepted: %d", in, code)
		}
	}
	codes := map[int]int{}
	for range 35 {
		code, _ := publicGet(t, e, "st4_unknown", nil)
		codes[code]++
	}
	if codes[http.StatusNotFound] != 30 || codes[http.StatusTooManyRequests] != 5 {
		t.Fatalf("rate limit: %v", codes)
	}
}

func TestCyclesRejectForeignIDs(t *testing.T) {
	e := setup(t)
	var ov domain.TrainingCycleOverview
	if code, body := e.do(2, "POST", "/api/cycles/plans", m{"name": "Marathon", "startDate": "2026-01-05", "endDate": "2026-04-12"}, &ov); code != 201 {
		t.Fatalf("plan: %d %s", code, body)
	}
	planID, phaseID := ov.ActivePlan.ID, ov.Phases[0].ID
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/cycles/phases", m{"planId": planID, "name": "Intrus", "startDate": "2026-05-04", "endDate": "2026-05-10"}},
		{"PUT", "/api/cycles/weeks", m{"planId": planID, "weekStart": "2026-01-12", "note": "x"}},
		{"PUT", "/api/cycles/weeks", m{"phaseId": phaseID, "weekStart": "2026-01-12", "note": "x"}},
		{"GET", fmt.Sprintf("/api/cycles?planId=%d", planID), nil},
		{"PATCH", fmt.Sprintf("/api/cycles/plans/%d", planID), m{"name": "x"}},
		{"PATCH", fmt.Sprintf("/api/cycles/phases/%d", phaseID), m{"name": "x"}},
		{"DELETE", fmt.Sprintf("/api/cycles/phases/%d", phaseID), nil},
		{"DELETE", fmt.Sprintf("/api/cycles/plans/%d", planID), nil},
	} {
		if code, body := e.do(3, c.method, c.path, c.body, nil); code != http.StatusNotFound || strings.Contains(body, "Marathon") {
			t.Fatalf("%s %s by another user: %d %s", c.method, c.path, code, body)
		}
	}
	var n int
	if err := e.st.DB.QueryRow(`SELECT COUNT(*) FROM plan_phases WHERE plan_id = ? AND user_id <> 2`, planID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("foreign rows attached to the plan: %d (%v)", n, err)
	}
	var goal m
	if code, body := e.do(2, "POST", "/api/goals", m{"title": "10 km", "kind": "race"}, &goal); code != 201 {
		t.Fatalf("goal: %d %s", code, body)
	}
	if code, _ := e.do(3, "POST", "/api/cycles/plans", m{"name": "x", "startDate": "2026-01-05", "endDate": "2026-02-01", "goalId": goal["id"]}, nil); code != 404 {
		t.Fatalf("foreign goal accepted: %d", code)
	}
	// a phase of another plan of the same user is refused for a week
	var other domain.TrainingCycleOverview
	e.do(2, "POST", "/api/cycles/plans", m{"name": "Trail", "startDate": "2026-06-01", "endDate": "2026-08-30", "status": "archived"}, &other)
	if code, _ := e.do(2, "PUT", "/api/cycles/weeks", m{"planId": other.ActivePlan.ID, "phaseId": phaseID, "weekStart": "2026-06-01"}, nil); code != 400 {
		t.Fatalf("phase of another plan accepted: %d", code)
	}
}

// Review case: importing phases while a scaffolded plan is active used to delete the plan's
// overlapping phases and insert unattached ones, so /api/cycles and /api/plan/phases disagreed.
func TestImportedPhasesJoinTheActivePlan(t *testing.T) {
	e := setup(t)
	if _, err := e.st.DB.Exec(`INSERT INTO plan_phases (user_id, name, start_date, end_date, created_at, updated_at) VALUES (2, 'Ancienne', '2026-02-02', '2026-02-15', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	var ov domain.TrainingCycleOverview
	e.do(2, "POST", "/api/cycles/plans", m{"name": "Marathon", "startDate": "2026-01-05", "endDate": "2026-04-12"}, &ov)
	planID := ov.ActivePlan.ID
	phases := []m{
		{"name": "Bloc seuil", "startDate": "2026-02-02", "endDate": "2026-02-22", "focus": "Seuil", "weeklyVolumeKm": 60},
	}
	if code, body := e.do(2, "POST", "/api/plan/bulk", m{"sessions": []m{}, "phases": phases}, nil); code >= 300 {
		t.Fatalf("bulk: %d %s", code, body)
	}
	var cycles domain.TrainingCycleOverview
	e.do(2, "GET", "/api/cycles", nil, &cycles)
	var flat []m
	e.do(2, "GET", "/api/plan/phases", nil, &flat)
	if len(cycles.Phases) != len(flat) {
		t.Fatalf("cycles has %d phases, /api/plan/phases %d", len(cycles.Phases), len(flat))
	}
	imported := false
	for i, ph := range cycles.Phases {
		if ph.PlanID == nil || *ph.PlanID != planID || int64(flat[i]["id"].(float64)) != ph.ID {
			t.Fatalf("phase %d: %+v vs %v", i, ph, flat[i])
		}
		if ph.Name == "Bloc seuil" {
			imported = true
		}
		if ph.Name != "Bloc seuil" && ph.StartDate <= "2026-02-22" && ph.EndDate >= "2026-02-02" {
			t.Fatalf("overlapping plan phase kept: %+v", ph)
		}
		if i > 0 && ph.StartDate < cycles.Phases[i-1].StartDate {
			t.Fatalf("phases not ordered by date: %+v", cycles.Phases)
		}
	}
	var orphan int
	_ = e.st.DB.QueryRow(`SELECT COUNT(*) FROM plan_phases WHERE user_id = 2 AND plan_id IS NULL`).Scan(&orphan)
	if !imported || orphan != 1 {
		t.Fatalf("imported=%v, phases outside the plan touched (orphans=%d)", imported, orphan)
	}
}
