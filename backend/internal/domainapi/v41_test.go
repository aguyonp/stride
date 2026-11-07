package domainapi_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHealthStatusDrivesTheProfile(t *testing.T) {
	e := setup(t)
	var st m
	if code, body := e.do(2, "GET", "/api/health/status", nil, &st); code != 200 || st["mode"] != "performance" || len(st["zones"].([]any)) != 0 ||
		st["focusArea"] != "none" || st["updatedAt"] != nil {
		t.Fatalf("default: %d %s", code, body)
	}
	// legacy profile focus becomes a health status
	e.do(2, "PUT", "/api/profile", m{"focusArea": "knee_pfps", "injuredSide": "left"}, nil)
	if e.do(2, "GET", "/api/health/status", nil, &st); st["mode"] != "tracking" || st["focusArea"] != "knee_pfps" ||
		st["zones"].([]any)[0].(m)["side"] != "left" {
		t.Fatalf("from profile: %v", st)
	}
	in := m{"mode": "tracking", "zones": []m{
		{"zone": "achilles", "side": "right", "since": "2026-10-01", "note": "raideur"},
		{"zone": "iliotibial_band", "side": "left", "since": "2026-11-01", "note": ""},
	}}
	if code, body := e.do(2, "PUT", "/api/health/status", in, &st); code != 200 || st["focusArea"] != "other" || st["updatedAt"] == nil {
		t.Fatalf("put: %d %s", code, body)
	}
	var prof m
	if e.do(2, "GET", "/api/profile", nil, &prof); prof["focusArea"] != "other" || prof["focusLabel"] != "Bandelette ilio-tibiale (TFL)" || prof["injuredSide"] != "left" {
		t.Fatalf("derived profile: %v", prof)
	}
	for _, bad := range []m{
		{"mode": "tracking", "zones": []m{}},
		{"mode": "tracking", "zones": []m{{"zone": "achilles"}, {"zone": "achilles"}}},
		{"mode": "performance", "zones": []m{{"zone": "achilles"}}},
		{"mode": "tracking", "zones": []m{{"zone": "elbow"}}},
		{"mode": "tracking", "zones": []m{{"zone": "achilles", "since": "hier"}}},
	} {
		if code, _ := e.do(2, "PUT", "/api/health/status", bad, nil); code != 400 {
			t.Fatalf("%v accepted: %d", bad, code)
		}
	}
	if e.do(3, "GET", "/api/health/status", nil, &st); st["mode"] != "performance" {
		t.Fatalf("status leaked across users: %v", st)
	}
	if code, _ := e.do(2, "PUT", "/api/health/status", m{"mode": "performance", "zones": []m{}}, &st); code != 200 || st["focusArea"] != "none" {
		t.Fatalf("back to performance: %v", st)
	}
}

func TestActivityPainsLiveInTheBodyMap(t *testing.T) {
	e := setup(t)
	bootGarmin(t, e)
	runID := e.id(`SELECT id FROM activities WHERE garmin_id = 1001`)
	e.do(1, "PUT", "/api/health/status", m{"mode": "tracking", "zones": []m{{"zone": "achilles", "side": "right"}}}, nil)
	var notes m
	if code, body := e.do(1, "PUT", fmt.Sprintf("/api/activities/%d/notes", runID), m{"painDuring": 4, "painNextMorning": 2}, &notes); code != 200 ||
		notes["painDuring"] != 4.0 || notes["painNextMorning"] != 2.0 {
		t.Fatalf("notes: %d %s", code, body)
	}
	var stored, entries int
	_ = e.st.DB.QueryRow(`SELECT COUNT(*) FROM activity_notes WHERE activity_id = ? AND (pain_during IS NOT NULL OR pain_next_morning IS NOT NULL)`, runID).Scan(&stored)
	_ = e.st.DB.QueryRow(`SELECT COUNT(*) FROM body_stress_entries WHERE activity_id = ? AND zone = 'achilles' AND pain_score = 4 AND cold_next_day_score = 2`, runID).Scan(&entries)
	if stored != 0 || entries != 1 {
		t.Fatalf("pains must live in the body map only (notes=%d entries=%d)", stored, entries)
	}
	// Ressenti form: one entry per activity and zone, the notes project the max
	for _, pain := range []float64{5, 6} {
		if code, body := e.do(1, "POST", "/api/bodymap/entries", m{"date": e.fx.day(-1), "activityId": runID, "zone": "knee_pfps", "painScore": pain}, nil); code != 201 {
			t.Fatalf("ressenti: %d %s", code, body)
		}
	}
	_ = e.st.DB.QueryRow(`SELECT COUNT(*) FROM body_stress_entries WHERE activity_id = ?`, runID).Scan(&entries)
	if e.do(1, "PUT", fmt.Sprintf("/api/activities/%d/notes", runID), m{}, &notes); entries != 2 || notes["painDuring"] != 6.0 {
		t.Fatalf("projection: entries=%d notes=%v", entries, notes)
	}
	var ov m
	if code, _ := e.do(1, "GET", "/api/bodymap?days=3650", nil, &ov); code != 200 || len(ov["entries"].([]any)) != 2 {
		t.Fatalf("bodymap: %v", ov["entries"])
	}
}

func TestHistorySyncJob(t *testing.T) {
	e := setup(t)
	bootGarmin(t, e)
	from := e.fx.day(-400)
	var job m
	if code, body := e.do(1, "POST", "/api/sync/history", m{"from": from}, &job); code != http.StatusAccepted || job["kind"] != "sync.history" || job["queue"] != "low" {
		t.Fatalf("enqueue: %d %s", code, body)
	}
	e.svc.Wait()
	if code, body := e.do(1, "GET", fmt.Sprintf("/api/jobs/%v", job["id"]), nil, &job); code != 200 || job["state"] != "completed" ||
		job["processed"] == nil || job["total"] == nil {
		t.Fatalf("job: %d %s", code, body)
	}
	var hf string
	_ = e.st.DB.QueryRow(`SELECT history_from FROM provider_connections WHERE user_id = 1 AND provider = 'garmin'`).Scan(&hf)
	if hf != from {
		t.Fatalf("history_from = %s, want %s", hf, from)
	}
	if code, _ := e.do(2, "GET", fmt.Sprintf("/api/jobs/%v", job["id"]), nil, nil); code != 404 {
		t.Fatal("job visible to another user")
	}
	if code, _ := e.do(2, "POST", "/api/sync/history", m{"from": nil}, nil); code != http.StatusPreconditionFailed {
		t.Fatalf("user without provider: %d", code)
	}
	if code, _ := e.do(1, "POST", "/api/sync/history", m{"from": "14/04"}, nil); code != 400 {
		t.Fatalf("invalid from: %d", code)
	}
}

func TestSearchNamesAndFrenchDates(t *testing.T) {
	e := setup(t)
	bootGarmin(t, e)
	day, _ := time.Parse("2006-01-02", e.fx.day(-1))
	months := []string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}
	month := months[day.Month()-1]
	for _, q := range []string{"footing", "FOOT", e.fx.day(-1), fmt.Sprintf("%d %s", day.Day(), month), fmt.Sprintf("%s %d", month, day.Year()),
		day.Format("02/01/06"), day.Format("2/1/2006")} {
		var res struct {
			Query string `json:"query"`
			Items []m    `json:"items"`
		}
		if code, body := e.do(1, "GET", "/api/search?q="+strings.ReplaceAll(q, " ", "%20"), nil, &res); code != 200 || len(res.Items) == 0 {
			t.Fatalf("%q: %d %s", q, code, body)
		}
		if e.do(2, "GET", "/api/search?q="+strings.ReplaceAll(q, " ", "%20"), nil, &res); len(res.Items) != 0 {
			t.Fatalf("%q leaked across users", q)
		}
	}
	if code, _ := e.do(1, "GET", "/api/search?q=", nil, nil); code != 400 {
		t.Fatal("empty query accepted")
	}
}

func TestShareScopesFilterThePublicView(t *testing.T) {
	e := setup(t)
	bootGarmin(t, e)
	e.do(1, "POST", "/api/shoes", m{"name": "Route", "brand": "Asics", "model": "Novablast", "dropMm": 8, "type": "soft"}, nil)
	var link m
	if code, body := e.do(1, "POST", "/api/share", m{"label": "Kiné", "periodWeeks": 4, "sections": []string{"load", "activities"}, "anonymize": true}, &link); code != 201 ||
		link["periodWeeks"] != 4.0 || link["anonymize"] != true || fmt.Sprint(link["sections"]) != "[activities load]" || link["expiresAt"] != nil {
		t.Fatalf("create: %d %s", code, body)
	}
	token := strings.TrimPrefix(link["url"].(string), "/share/")
	_, body := publicGet(t, e, token, nil)
	for _, absent := range []string{`"readiness"`, `"cycles"`, `"bodyMap"`, `"shoes"`, `"records"`, `"focusArea"`, `"painDuring"`, `"admin"`} {
		if strings.Contains(body, absent) {
			t.Fatalf("%s must be absent: %s", absent, body)
		}
	}
	var v m
	publicGet(t, e, token, &v)
	if v["athlete"].(m)["displayName"] != "Athlète" || len(v["loadSeries"].([]any)) != 28 || v["periodWeeks"] != 4.0 || v["recentSessions"] == nil {
		t.Fatalf("scoped view: %v", v["athlete"])
	}
	id := int64(link["id"].(float64))
	if code, body := e.do(1, "PATCH", fmt.Sprintf("/api/share/%d", id), m{"sections": []string{"notes", "shoes", "health", "activities"}, "expiresInDays": 7}, &link); code != 200 ||
		fmt.Sprint(link["sections"]) != "[activities health shoes notes]" || link["expiresAt"] == nil || link["periodWeeks"] != 4.0 {
		t.Fatalf("patch: %d %s", code, body)
	}
	v = m{}
	publicGet(t, e, token, &v)
	shoes, _ := v["shoes"].([]any)
	if len(shoes) != 1 || shoes[0].(m)["id"] != nil || shoes[0].(m)["brand"] != "Asics" || v["bodyMap"] == nil || v["loadSeries"] != nil {
		t.Fatalf("patched view not refreshed: %v", v)
	}
	for _, bad := range []m{{"periodWeeks": 5}, {"sections": []string{}}, {"sections": []string{"secrets"}}, {"expiresInDays": 3}, {"token": "x"}} {
		if code, _ := e.do(1, "PATCH", fmt.Sprintf("/api/share/%d", id), bad, nil); code != 400 {
			t.Fatalf("%v accepted: %d", bad, code)
		}
	}
	if code, _ := e.do(2, "PATCH", fmt.Sprintf("/api/share/%d", id), m{"label": "Volé"}, nil); code != 404 {
		t.Fatal("patch of another user's link")
	}
	if e.do(1, "PATCH", fmt.Sprintf("/api/share/%d", id), m{"expiresInDays": nil}, &link); link["expiresAt"] != nil {
		t.Fatalf("expiry not removed: %v", link)
	}
	// a 4.0 link (empty scope) keeps every section
	if _, err := e.st.DB.Exec(`UPDATE share_links SET scope_json = '{}' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	e.do(1, "PATCH", fmt.Sprintf("/api/share/%d", id), m{"label": "Kiné 2"}, &link)
	if fmt.Sprint(link["sections"]) != "[activities health recovery load records plan shoes notes]" || link["periodWeeks"] != 12.0 {
		t.Fatalf("legacy scope: %v", link)
	}
}
