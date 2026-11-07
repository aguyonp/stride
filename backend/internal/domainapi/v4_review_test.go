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
