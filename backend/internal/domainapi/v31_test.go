package domainapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"stride/internal/domain"
	"stride/internal/provider/strava"
)

func recordOf(t *testing.T, body m, key string) m {
	t.Helper()
	for _, r := range body["records"].([]any) {
		if r.(m)["key"] == key {
			return r.(m)
		}
	}
	t.Fatalf("record %s missing: %v", key, body)
	return nil
}

// A user without any provider gets every view, report and prompt from manual activities.
func TestV31StandaloneManualActivitiesAndRecords(t *testing.T) {
	e := setup(t)
	bootGarmin(t, e) // user 1 only: user 3 has no integration at all
	const u = 3
	y := e.fx.day(-1)
	var run m
	if code, body := e.do(u, "POST", "/api/activities", m{"date": y, "startTime": "07:30", "sport": "run", "title": "Course du club",
		"distanceM": 10050, "durationS": 2700, "avgHR": 152, "maxHR": 171, "avgCadenceSpm": 174, "elevGainM": 40, "elevLossM": 38}, &run); code != 201 {
		t.Fatalf("create: %d %s", code, body)
	}
	if fmt.Sprint(run["sources"]) != "[manual]" || run["kudos"] != nil || run["name"] != "Course du club" || run["avgPaceSecPerKm"] == nil {
		t.Fatalf("manual summary: %v", run)
	}
	runID := int64(run["id"].(float64))
	if code, _ := e.do(u, "POST", "/api/activities", m{"date": e.fx.day(0), "sport": "bike", "durationS": 1800}, nil); code != 201 {
		t.Fatal("create bike")
	}
	for _, bad := range []m{{"date": e.fx.day(2), "sport": "run", "durationS": 600}, {"date": y, "sport": "ski", "durationS": 600},
		{"date": y, "sport": "run"}, {"date": y, "sport": "run", "durationS": 600, "avgHR": 160, "maxHR": 150}, {"date": y, "sport": "run", "durationS": 600, "bogus": 1}} {
		if code, _ := e.do(u, "POST", "/api/activities", bad, nil); code != 400 {
			t.Fatalf("invalid manual activity accepted: %v", bad)
		}
	}
	var view m
	if code, body := e.do(u, "GET", fmt.Sprintf("/api/activities/%d", runID), nil, &view); code != 200 || view["sourceLabel"] != "Manuel" ||
		view["weather"] != nil || view["weatherReason"] == nil {
		t.Fatalf("manual view: %d %.300s", code, body)
	}
	paths := []string{"/api/activities", fmt.Sprintf("/api/activities/%d/report", runID), "/api/days/" + y, "/api/days/" + y + "/report",
		"/api/plan/week", "/api/calendar?from=" + e.fx.day(-7) + "&to=" + e.fx.day(7), "/api/records", "/api/trends/efficiency",
		"/api/sync/status", "/api/integrations", "/api/goals", "/api/plan", "/api/shoes", "/api/calendar/events", "/api/plan/phases"}
	for _, p := range paths {
		if code, body := e.do(u, "GET", p, nil, nil); code != 200 {
			t.Fatalf("GET %s without provider: %d %.200s", p, code, body)
		}
	}
	_, report := e.do(u, "GET", fmt.Sprintf("/api/activities/%d/report", runID), nil, nil)
	if !strings.Contains(report, "**Records personnels :** 10 km 44:47") || !strings.Contains(report, "Plus longue sortie 10,1 km") {
		t.Fatalf("records missing from the prompt: %.1500s", report)
	}
	for _, c := range []struct {
		method, path string
		body         any
	}{{"POST", "/api/exports/multi", m{"weeks": 2}}, {"POST", "/api/plan/generate", m{"mode": "prompt", "horizon": "2weeks"}}, {"POST", "/api/sync", nil}} {
		if code, body := e.do(u, c.method, c.path, c.body, nil); code != 200 {
			t.Fatalf("%s %s without provider: %d %.200s", c.method, c.path, code, body)
		}
	}
	var recs m
	e.do(u, "GET", "/api/records", nil, &recs)
	if r := recordOf(t, recs, "10k"); r["timeS"] != 2687.0 || r["source"] != "computed" || r["activityId"] != float64(runID) || r["paceSecPerKm"] != 269.0 {
		t.Fatalf("10k: %v", r)
	}
	if r := recordOf(t, recs, "5k"); r["timeS"] != nil || r["distanceM"] != 5000.0 {
		t.Fatalf("5k without series: %v", r)
	}
	var put m
	if code, body := e.do(u, "PUT", "/api/records/5k", m{"timeS": 1290, "date": "2026-10-12"}, &put); code != 200 || put["source"] != "manual" || put["paceSecPerKm"] != 258.0 {
		t.Fatalf("put record: %d %s", code, body)
	}
	if code, _ := e.do(u, "PUT", "/api/records/longest", m{"timeS": 7200}, nil); code != 400 {
		t.Fatal("longest without distance")
	}
	if code, _ := e.do(u, "PUT", "/api/records/3k", m{"timeS": 700}, nil); code != 404 {
		t.Fatal("unknown record key")
	}
	e.do(u, "GET", "/api/records", nil, &recs)
	if r := recordOf(t, recs, "5k"); r["source"] != "manual" || r["timeS"] != 1290.0 {
		t.Fatalf("manual wins: %v", r)
	}
	if code, _ := e.do(u, "DELETE", "/api/records/5k", nil, nil); code != 204 {
		t.Fatal("delete record")
	}
	// PATCH / DELETE: manual activities only, own activities only
	var patched m
	if code, body := e.do(u, "PATCH", fmt.Sprintf("/api/activities/%d", runID), m{"distanceM": 5100, "durationS": 1400}, &patched); code != 200 ||
		patched["distanceM"] != 5100.0 || patched["name"] != "Course du club" {
		t.Fatalf("patch: %d %s", code, body)
	}
	e.do(u, "GET", "/api/records", nil, &recs)
	if r := recordOf(t, recs, "5k"); r["timeS"] != 1373.0 || r["source"] != "computed" {
		t.Fatalf("records not recomputed after patch: %v", r)
	}
	garminRun := e.id(`SELECT id FROM activities WHERE user_id = 1 AND garmin_id = 1001`)
	if code, _ := e.do(u, "PATCH", fmt.Sprintf("/api/activities/%d", garminRun), m{"durationS": 600}, nil); code != 404 {
		t.Fatal("patch of another user's activity")
	}
	if code, _ := e.do(1, "PATCH", fmt.Sprintf("/api/activities/%d", garminRun), m{"durationS": 600}, nil); code != 409 {
		t.Fatal("patch of a provider activity")
	}
	if code, _ := e.do(1, "DELETE", fmt.Sprintf("/api/activities/%d", garminRun), nil, nil); code != 409 {
		t.Fatal("delete of a provider activity")
	}
	if code, _ := e.do(u, "DELETE", fmt.Sprintf("/api/activities/%d", runID), nil, nil); code != 204 {
		t.Fatal("delete manual")
	}
	if code, _ := e.do(u, "GET", fmt.Sprintf("/api/activities/%d", runID), nil, nil); code != 404 {
		t.Fatal("deleted activity still served")
	}
}

// Credentials and data are strictly per user: user 2 never sees nor uses user 1's Garmin key,
// Strava tokens, LLM key or activities, even with a copied encrypted row.
func TestV31CredentialIsolation(t *testing.T) {
	var llmCalls atomic.Int32
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmCalls.Add(1)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	t.Cleanup(llm.Close)
	e := setupWith(t, nil, func(d *domain.Deps) {
		d.Env.AllowPrivateEgress = true
		d.BaseURLs = map[string]string{"openai": llm.URL}
	})
	bootGarmin(t, e)
	connectStrava(t, e, 1)
	if code, body := e.do(1, "PUT", "/api/llm", m{"provider": "openai", "model": "gpt-test", "apiKey": "k-openai-synthetique-9876"}, nil); code != 200 {
		t.Fatalf("llm: %d %s", code, body)
	}
	runID := e.id(`SELECT id FROM activities WHERE user_id = 1 AND garmin_id = 1001`)
	var in m
	e.do(2, "GET", "/api/integrations", nil, &in)
	g, s := in["garmin"].(m), in["strava"].(m)
	if g["connected"] != false || g["keyHint"] != nil || s["connected"] != false || s["athleteName"] != nil || len(s["scopes"].([]any)) != 0 {
		t.Fatalf("user 2 integrations: %v", in)
	}
	var cfg m
	if e.do(2, "GET", "/api/llm", nil, &cfg); cfg["configured"] != false || cfg["keyHint"] != nil {
		t.Fatalf("user 2 llm: %v", cfg)
	}
	var list m
	if e.do(2, "GET", "/api/activities", nil, &list); len(list["items"].([]any)) != 0 {
		t.Fatalf("user 2 sees activities: %v", list)
	}
	for _, c := range []struct {
		method, path string
		body         any
		want         int
	}{
		{"GET", fmt.Sprintf("/api/activities/%d", runID), nil, 404},
		{"POST", "/api/llm/analyze", m{"kind": "activity", "activityId": runID}, 412},
		{"POST", "/api/llm/test", nil, 412},
		{"GET", "/api/integrations/strava/stats", nil, 412},
		{"POST", "/api/integrations/garmin/test", nil, 412},
		{"POST", "/api/shoes/import-strava", nil, 412},
	} {
		if code, body := e.do(2, c.method, c.path, c.body, nil); code != c.want {
			t.Fatalf("user 2 %s %s: %d (want %d) %.200s", c.method, c.path, code, c.want, body)
		}
	}
	// A Garmin row copied from user 1 cannot be decrypted as user 2 (AAD binds the user id).
	if _, err := e.st.DB.Exec(`INSERT INTO provider_connections (user_id, provider, status, secret_enc, token_enc, token_expires_at, created_at, updated_at)
		SELECT 2, provider, status, secret_enc, token_enc, token_expires_at, created_at, updated_at FROM provider_connections WHERE user_id = 1 AND provider = 'garmin'`); err != nil {
		t.Fatal(err)
	}
	before := e.fx.calls.Load()
	if code, _ := e.do(2, "POST", "/api/integrations/garmin/test", nil, nil); code == 200 {
		t.Fatal("user 2 used user 1's Garmin key")
	}
	if code, _ := e.do(2, "POST", "/api/sync", nil, nil); code != 200 {
		t.Fatal("sync")
	}
	e.svc.Wait()
	if e.fx.calls.Load() != before || e.id(`SELECT COUNT(*) FROM activities WHERE user_id = 2`) != 0 {
		t.Fatal("Garmin called on behalf of user 2 with user 1's key")
	}
	// Same for a copied LLM configuration.
	if _, err := e.st.DB.Exec(`INSERT INTO llm_configs (user_id, provider, model, base_url, key_enc, key_hint, updated_at)
		SELECT 2, provider, model, base_url, key_enc, key_hint, updated_at FROM llm_configs WHERE user_id = 1`); err != nil {
		t.Fatal(err)
	}
	calls := llmCalls.Load()
	if code, _ := e.do(2, "POST", "/api/llm/test", nil, &cfg); code == 200 && cfg["ok"] == true {
		t.Fatal("user 2 used user 1's LLM key")
	}
	if llmCalls.Load() != calls {
		t.Fatal("LLM called on behalf of user 2 with user 1's key")
	}
	var stats m
	if code, body := e.do(1, "GET", "/api/integrations/strava/stats", nil, &stats); code != 200 || stats["recentRun"].(m)["count"] != 3.0 {
		t.Fatalf("user 1 stats: %d %s", code, body)
	}
}

// The env Strava tokens seed the first admin once; a read-only token is detected and listed as
// missing scopes; a rejected token is refreshed.
func TestV31StravaSeed(t *testing.T) {
	e := setupWith(t, nil, func(d *domain.Deps) {
		d.Env.AdminStravaAccessToken, d.Env.AdminStravaRefreshToken = "seed-readonly", "seed-refresh"
	})
	bootGarmin(t, e)
	var in m
	e.do(1, "GET", "/api/integrations", nil, &in)
	s := in["strava"].(m)
	if s["connected"] != true || fmt.Sprint(s["scopes"]) != "[read]" || fmt.Sprint(s["missingScopes"]) != "[activity:read_all profile:read_all]" {
		t.Fatalf("seeded strava: %v", s)
	}
	var st m
	if code, body := e.do(1, "GET", "/api/integrations/strava/stats", nil, &st); code != 200 || st["recentRun"].(m)["distanceM"] != 25000.0 {
		t.Fatalf("stats: %d %s", code, body)
	}
	var sync m
	e.do(1, "GET", "/api/sync/status", nil, &sync)
	if p := sync["providers"].(m)["strava"]; p != nil && p.(m)["error"] != nil {
		t.Fatalf("read-only token must not fail the sync: %v", p)
	}
	// disconnecting is respected at the next startup
	if code, _ := e.do(1, "DELETE", "/api/integrations/strava", nil, nil); code != 200 {
		t.Fatal("disconnect")
	}
	e.svc.SeedFirstAdmin(context.Background())
	e.svc.Wait()
	if e.id(`SELECT COUNT(*) FROM provider_connections WHERE provider = 'strava'`) != 0 {
		t.Fatal("re-seeded after a deliberate disconnection")
	}

	// startup seed of an existing admin, with an expired access token
	e2 := setupWith(t, nil, func(d *domain.Deps) {
		d.Env.AdminStravaAccessToken, d.Env.AdminStravaRefreshToken = "seed-expired", "seed-refresh"
	})
	e2.svc.SeedFirstAdmin(context.Background())
	e2.svc.Wait()
	var in2 m
	e2.do(1, "GET", "/api/integrations", nil, &in2)
	if s := in2["strava"].(m); s["connected"] != true || !strings.Contains(fmt.Sprint(s["scopes"]), "activity:read") {
		t.Fatalf("startup seed: %v", s)
	}
	if e2.id(`SELECT COUNT(*) FROM provider_connections WHERE user_id <> 1`) != 0 {
		t.Fatal("seed applied to another user")
	}
}

// Kudos come from Strava (null for Garmin-only), Strava best_efforts compete with computed windows.
func TestV31KudosAndStravaBestEfforts(t *testing.T) {
	e := setup(t)
	bootGarmin(t, e)
	kudos := 5
	e.fs.acts[7001] = &strava.Activity{ID: 7001, Name: "Footing", SportType: "Run", StartDate: e.fx.runStart.Add(30 * 1e9).UTC(),
		StartDateLocal: e.fx.runStart.Add(30 * 1e9).In(paris).Format("2006-01-02T15:04:05"), DistanceM: 5380, MovingTimeS: 1800, ElapsedTimeS: 1800,
		KudosCount: &kudos, BestEfforts: []strava.Effort{{Name: "1k", DistanceM: 1000, ElapsedS: 301}, {Name: "5k", DistanceM: 5000, ElapsedS: 1720}}}
	connectStrava(t, e, 1)
	runID := e.id(`SELECT id FROM activities WHERE user_id = 1 AND garmin_id = 1001`)
	if e.id(`SELECT COUNT(*) FROM activities WHERE strava_id = 7001 AND id = ?`, runID) != 1 {
		t.Fatal("strava activity not fused")
	}
	var list m
	e.do(1, "GET", "/api/activities", nil, &list)
	for _, it := range list["items"].([]any) {
		it := it.(m)
		want := any(nil)
		if it["id"] == float64(runID) {
			want = 5.0
		}
		if it["kudos"] != want {
			t.Fatalf("kudos of %v: %v", it["name"], it["kudos"])
		}
	}
	if code, _ := e.do(1, "GET", fmt.Sprintf("/api/activities/%d", runID), nil, nil); code != 200 {
		t.Fatal("view")
	}
	var recs m
	e.do(1, "GET", "/api/records", nil, &recs)
	if r := recordOf(t, recs, "1k"); r["source"] != "strava" || r["timeS"] != 301.0 {
		t.Fatalf("1k: %v", r)
	}
	if r := recordOf(t, recs, "5k"); r["source"] != "computed" || r["timeS"] != 1667.0 {
		t.Fatalf("5k: %v", r)
	}
	if r := recordOf(t, recs, "longest"); r["distanceM"] != 6000.0 {
		t.Fatalf("longest: %v", r)
	}
}

func dayN(n int) string { return time.Now().In(paris).AddDate(0, 0, n).Format("2006-01-02") }

// Calendar events, shift, rule/LLM adaptation, atomic replace, long-term generation with phases.
func TestV31EventsAdaptAndLongTerm(t *testing.T) {
	plan := fmt.Sprintf("Plan :\n```json\n"+`{"format":"stride-plan","version":2,"goal":{"title":"10 km","date":%q},
		"phases":[{"name":"Développement","startDate":%q,"endDate":%q,"focus":"volume","weeklyVolumeKm":30},{"name":"Affûtage","startDate":%q,"endDate":%q,"weeklyVolumeKm":20}],
		"sessions":[{"date":%q,"sport":"run","title":"Footing","targetDurationS":2400},{"date":%q,"sport":"run","title":"Seuil","targetDurationS":3000}]}`+"\n```",
		dayN(60), dayN(0), dayN(45), dayN(46), dayN(60), dayN(1), dayN(4))
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(m{"choices": []m{{"message": m{"content": plan}}}})
		_, _ = w.Write(b)
	}))
	t.Cleanup(llm.Close)
	e := setupWith(t, nil, func(d *domain.Deps) {
		d.Env.AllowPrivateEgress = true
		d.BaseURLs = map[string]string{"openai": llm.URL}
	})
	const u = 2
	d := e.fx.day
	intervals := []m{{"type": "warmup", "durationS": 600}, {"type": "repeat", "repeat": 6, "steps": []m{{"type": "interval", "durationS": 60}, {"type": "recovery", "durationS": 90}}}, {"type": "cooldown", "durationS": 600}}
	sessions := []m{
		{"date": d(1), "sport": "run", "title": "Footing", "targetDurationS": 2700},
		{"date": d(2), "sport": "run", "title": "6×1'", "targetDurationS": 3000, "steps": intervals},
		{"date": d(4), "sport": "run", "title": "Footing", "targetDurationS": 2400},
		{"date": d(5), "sport": "run", "title": "Fractionné", "targetDurationS": 3600, "steps": intervals, "targetPaceMinSecPerKm": 250, "targetPaceMaxSecPerKm": 260},
		{"date": d(6), "sport": "rest", "title": "Repos"},
		{"date": d(20), "sport": "run", "title": "Sortie longue", "targetDurationS": 5400},
	}
	for n := 7; n <= 13; n++ { // following weeks heavy enough: the +10 % cap never bites, whatever the weekday
		sessions = append(sessions, m{"date": d(n), "sport": "bike", "title": "Vélo", "targetDurationS": 5400})
	}
	var created []m
	if code, body := e.do(u, "POST", "/api/plan/bulk", m{"sessions": sessions}, &created); code != 201 || len(created) != 13 {
		t.Fatalf("bulk: %d %s", code, body)
	}
	var ev m
	if code, body := e.do(u, "POST", "/api/calendar/events", m{"kind": "illness", "startDate": d(2), "endDate": d(3), "severity": 2, "note": "Grippe"}, &ev); code != 201 || ev["id"] == nil {
		t.Fatalf("event: %d %s", code, body)
	}
	evID := int64(ev["id"].(float64))
	for _, bad := range []m{{"kind": "holiday", "startDate": d(1)}, {"kind": "pain", "startDate": d(3), "endDate": d(1)}, {"kind": "pain", "startDate": d(1), "severity": 4}} {
		if code, _ := e.do(u, "POST", "/api/calendar/events", bad, nil); code != 400 {
			t.Fatalf("invalid event accepted: %v", bad)
		}
	}
	if code, _ := e.do(1, "PATCH", fmt.Sprintf("/api/calendar/events/%d", evID), m{"severity": 1}, nil); code != 404 {
		t.Fatal("event of another user")
	}
	var evs []m
	if e.do(u, "GET", "/api/calendar/events?from="+d(0)+"&to="+d(10), nil, &evs); len(evs) != 1 || evs[0]["note"] != "Grippe" {
		t.Fatalf("events: %v", evs)
	}
	var cal m
	e.do(u, "GET", "/api/calendar?from="+d(0)+"&to="+d(4), nil, &cal)
	days := cal["days"].([]any)
	if len(days[2].(m)["events"].([]any)) != 1 || len(days[0].(m)["events"].([]any)) != 0 || len(days[4].(m)["events"].([]any)) != 0 {
		t.Fatalf("calendar events: %v", days)
	}

	var prev m
	if code, body := e.do(u, "POST", "/api/plan/adapt", m{"from": d(0), "mode": "rules"}, &prev); code != 200 {
		t.Fatalf("adapt: %d %s", code, body)
	}
	byDate := map[string]m{}
	for _, s := range prev["sessions"].([]any) {
		byDate[s.(m)["date"].(string)] = s.(m)
	}
	moved := byDate[d(6)]
	if moved == nil || moved["title"] != "Footing facile" || moved["id"] != 0.0 || len(moved["steps"].([]any)) != 0 {
		t.Fatalf("session of the illness not moved to the next free day and softened: %v", prev)
	}
	if f := byDate[d(5)]; f == nil || f["targetDurationS"] != 2520.0 || f["targetPaceMinSecPerKm"] != nil || !strings.Contains(f["instructions"].(string), "Maladie") {
		t.Fatalf("intervals after illness: %v", f)
	}
	if byDate[d(1)] != nil || byDate[d(20)] != nil || len(prev["removeIds"].([]any)) != len(prev["sessions"].([]any)) {
		t.Fatalf("untouched sessions in the preview: %v", prev)
	}
	if !strings.Contains(prev["summary"].(string), "Maladie") || !strings.Contains(prev["summary"].(string), "1 séance décalée") {
		t.Fatalf("summary: %v", prev["summary"])
	}
	var before []m
	e.do(u, "GET", "/api/plan", nil, &before)
	if len(before) != 13 {
		t.Fatal("adapt preview saved something")
	}
	var applied []m
	if code, body := e.do(u, "POST", "/api/plan/replace", m{"removeIds": prev["removeIds"], "sessions": prev["sessions"]}, &applied); code != 200 || len(applied) != len(prev["sessions"].([]any)) {
		t.Fatalf("replace: %d %s", code, body)
	}
	var after []m
	e.do(u, "GET", "/api/plan", nil, &after)
	for _, ps := range after {
		if ps["date"] == d(2) {
			t.Fatalf("session left during the illness: %v", after)
		}
	}
	if code, _ := e.do(u, "POST", "/api/plan/replace", m{"removeIds": []int64{int64(created[0]["id"].(float64)), 999999}, "sessions": []m{}}, nil); code != 404 {
		t.Fatal("replace with an unknown id must fail")
	}
	var still []m
	if e.do(u, "GET", "/api/plan", nil, &still); len(still) != len(after) {
		t.Fatal("failed replace not atomic")
	}

	firstID := int64(created[0]["id"].(float64))
	var shifted m
	if code, body := e.do(u, "POST", fmt.Sprintf("/api/plan/%d/shift", firstID), m{"days": 2}, &shifted); code != 200 || shifted["date"] != d(3) {
		t.Fatalf("shift: %d %s", code, body)
	}
	if code, _ := e.do(u, "POST", fmt.Sprintf("/api/plan/%d/shift", firstID), m{"toDate": d(1)}, &shifted); code != 200 || shifted["date"] != d(1) {
		t.Fatal("shift toDate")
	}
	for _, bad := range []m{{}, {"days": 1, "toDate": d(2)}, {"days": 0}, {"toDate": "demain"}} {
		if code, _ := e.do(u, "POST", fmt.Sprintf("/api/plan/%d/shift", firstID), bad, nil); code != 400 {
			t.Fatalf("invalid shift accepted: %v", bad)
		}
	}

	if code, _ := e.do(u, "POST", "/api/plan/generate", m{"mode": "prompt", "horizon": "goal"}, nil); code != 400 {
		t.Fatal("goal horizon without a dated goal")
	}
	if code, _ := e.do(u, "POST", "/api/plan/generate", m{"mode": "prompt", "horizon": "month"}, nil); code != 400 {
		t.Fatal("unknown horizon")
	}
	if code, _ := e.do(u, "POST", "/api/goals", m{"title": "10 km", "kind": "race", "targetDate": d(60)}, nil); code != 201 {
		t.Fatal("goal")
	}
	var pr m
	if code, body := e.do(u, "POST", "/api/plan/generate", m{"mode": "prompt", "horizon": "goal"}, &pr); code != 200 {
		t.Fatalf("goal prompt: %d %s", code, body)
	}
	prompt := pr["prompt"].(string)
	for _, want := range []string{`"version":2`, "en phases successives", "ÉVÉNEMENTS DU CALENDRIER", "Maladie du", "Records personnels", "objectif « 10 km »"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("goal prompt lacks %q:\n%s", want, prompt)
		}
	}
	if e.do(u, "POST", "/api/plan/generate", m{"mode": "prompt", "horizon": "2weeks"}, &pr); !strings.Contains(pr["prompt"].(string), "deux semaines") ||
		strings.Contains(pr["prompt"].(string), `"version":2`) {
		t.Fatal("2weeks prompt")
	}
	if code, _ := e.do(u, "PUT", "/api/llm", m{"provider": "openai", "model": "gpt-test", "apiKey": "k-openai-synthetique-1111"}, nil); code != 200 {
		t.Fatal("llm config")
	}
	var gen m
	if code, body := e.do(u, "POST", "/api/plan/generate", m{"mode": "llm", "horizon": "goal"}, &gen); code != 200 ||
		len(gen["phases"].([]any)) != 2 || len(gen["drafts"].([]any)) != 2 || gen["phases"].([]any)[0].(m)["id"] != 0.0 {
		t.Fatalf("goal llm: %d %s", code, body)
	}
	var bulk []m
	if code, body := e.do(u, "POST", "/api/plan/bulk", m{"sessions": gen["drafts"], "phases": gen["phases"]}, &bulk); code != 201 || len(bulk) != 2 {
		t.Fatalf("bulk with phases: %d %s", code, body)
	}
	if code, _ := e.do(u, "POST", "/api/plan/bulk", m{"sessions": []m{}, "phases": []m{{"name": "Base", "startDate": d(10), "endDate": d(20)}}}, nil); code != 201 {
		t.Fatal("phases only")
	}
	var phases []m
	if e.do(u, "GET", "/api/plan/phases", nil, &phases); len(phases) != 2 || phases[0]["name"] != "Base" || phases[1]["name"] != "Affûtage" {
		t.Fatalf("overlapping phase not replaced: %v", phases)
	}
	if e.do(1, "GET", "/api/plan/phases", nil, &phases); len(phases) != 0 {
		t.Fatal("phases of another user")
	}
	var llmPrev m
	if code, body := e.do(u, "POST", "/api/plan/adapt", m{"from": d(0), "mode": "llm", "eventId": evID}, &llmPrev); code != 200 ||
		len(llmPrev["sessions"].([]any)) != 2 || len(llmPrev["removeIds"].([]any)) == 0 {
		t.Fatalf("adapt llm: %d %s", code, body)
	}
	if code, _ := e.do(u, "DELETE", fmt.Sprintf("/api/calendar/events/%d", evID), nil, nil); code != 204 {
		t.Fatal("delete event")
	}
	if e.do(u, "POST", "/api/plan/adapt", m{"from": d(0), "mode": "rules"}, &prev); len(prev["removeIds"].([]any)) != 0 {
		t.Fatalf("adapt without events: %v", prev)
	}
}

// Missing weather is backfilled when a final bundle is viewed again; no-GPS activities get a reason.
func TestV31WeatherBackfill(t *testing.T) {
	e := setup(t)
	bootGarmin(t, e)
	runID := e.id(`SELECT id FROM activities WHERE user_id = 1 AND garmin_id = 1001`)
	bikeID := e.id(`SELECT id FROM activities WHERE user_id = 1 AND garmin_id = 1002`)
	var v m
	e.do(1, "GET", fmt.Sprintf("/api/activities/%d", runID), nil, &v)
	if e.id(`SELECT final FROM activities WHERE id = ?`, runID) != 1 {
		t.Fatal("bundle not final")
	}
	if _, err := e.st.DB.Exec(`DELETE FROM activity_blobs WHERE activity_id = ? AND kind = 'weather'`, runID); err != nil {
		t.Fatal(err)
	}
	if e.do(1, "GET", fmt.Sprintf("/api/activities/%d", runID), nil, &v); v["weather"] == nil || v["weatherReason"] != nil {
		t.Fatalf("weather not backfilled: %v %v", v["weather"], v["weatherReason"])
	}
	if e.do(1, "GET", fmt.Sprintf("/api/activities/%d", bikeID), nil, &v); v["weather"] != nil || !strings.Contains(fmt.Sprint(v["weatherReason"]), "GPS") {
		t.Fatalf("no-GPS weather: %v %v", v["weather"], v["weatherReason"])
	}
}
