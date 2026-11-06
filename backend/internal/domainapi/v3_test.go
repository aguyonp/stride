package domainapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"stride/internal/domain"
	"stride/internal/provider/strava"
)

func bootGarmin(t *testing.T, e *env2) {
	t.Helper()
	if err := e.svc.OnUserCreated(context.Background(), 1, true); err != nil {
		t.Fatal(err)
	}
	e.svc.Wait()
	if err := e.svc.MorningSync(context.Background(), 1); err != nil {
		t.Fatalf("sync: %v", err)
	}
	e.svc.Wait()
}

func connectStrava(t *testing.T, e *env2, uid int) {
	t.Helper()
	_, loc := e.do(uid, "GET", "/api/integrations/strava/connect", nil, nil)
	state := loc[strings.Index(loc, "state=")+6 : strings.Index(loc, "&")]
	if code, loc := e.do(uid, "GET", "/api/integrations/strava/callback?state="+state+"&code=c&scope=read,activity:read_all,profile:read_all", nil, nil); code != 302 || !strings.Contains(loc, "connected") {
		t.Fatalf("strava callback: %d %s", code, loc)
	}
	e.svc.Wait()
}

func (e *env2) id(q string, args ...any) int64 {
	e.t.Helper()
	var id int64
	if err := e.st.DB.QueryRow(q, args...).Scan(&id); err != nil {
		e.t.Fatalf("%s: %v", q, err)
	}
	return id
}

func gpxFile(start time.Time, n int) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><gpx xmlns="http://www.topografix.com/GPX/1/1" xmlns:gpxtpx="http://www.garmin.com/xmlschemas/TrackPointExtension/v1"><trk><name>Sortie montre HS</name><type>running</type><trkseg>`)
	for i := 0; i < n; i++ {
		ele := 300.0
		if i > 900 && i < 1200 {
			ele -= float64(i-900) * 0.2
		} else if i >= 1200 {
			ele = 240
		}
		fmt.Fprintf(&b, `<trkpt lat="%.6f" lon="5.700000"><ele>%.1f</ele><time>%s</time><extensions><gpxtpx:TrackPointExtension><gpxtpx:hr>%d</gpxtpx:hr><gpxtpx:cad>86</gpxtpx:cad></gpxtpx:TrackPointExtension></extensions></trkpt>`,
			45.1+float64(i)*0.000027, ele, start.Add(time.Duration(i)*time.Second).UTC().Format(time.RFC3339), 140+i/600)
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	return []byte(b.String())
}

func tcxFile(start time.Time, n int) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><TrainingCenterDatabase xmlns="http://www.garmin.com/xmlschemas/TrainingCenterDatabase/v2"><Activities><Activity Sport="Running"><Lap><Track>`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `<Trackpoint><Time>%s</Time><Position><LatitudeDegrees>%.6f</LatitudeDegrees><LongitudeDegrees>5.6789</LongitudeDegrees></Position><AltitudeMeters>310</AltitudeMeters><DistanceMeters>%d</DistanceMeters><HeartRateBpm><Value>142</Value></HeartRateBpm><Cadence>87</Cadence></Trackpoint>`,
			start.Add(time.Duration(i)*time.Second).UTC().Format(time.RFC3339), 45.1234+float64(i)*0.000027, i*3)
	}
	b.WriteString(`</Track></Lap></Activity></Activities></TrainingCenterDatabase>`)
	return []byte(b.String())
}

func (e *env2) upload(user int, name string, data []byte, out any) (int, string) {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/imports", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Test-User", fmt.Sprint(user))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 {
		_ = json.Unmarshal(raw, out)
	}
	return resp.StatusCode, string(raw)
}

func TestV3ReconciliationAndMetrics(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	bootGarmin(t, e)
	runID := e.id(`SELECT id FROM activities WHERE user_id = 1 AND garmin_id = 1001`)
	// V2 left a Strava duplicate 200 s away (outside its 120 s window) holding the athlete's notes.
	start := e.fx.runStart.Add(200 * time.Second)
	res, err := e.st.DB.Exec(`INSERT INTO activities (user_id, strava_id, start_utc, start_local, local_date, type, sport, name, distance_m, strava_gear, created_at, updated_at)
		VALUES (1, 9001, ?, ?, ?, 'running', 'run', 'Sortie custom', 5300, 'g1', 1, 1)`, start.Unix(), start.In(paris).Format("2006-01-02T15:04:05"), start.In(paris).Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	dupID, _ := res.LastInsertId()
	if _, err := e.st.DB.Exec(`INSERT INTO activity_notes (activity_id, user_id, pain_during, updated_at) VALUES (?, 1, 3, 1)`, dupID); err != nil {
		t.Fatal(err)
	}
	// too far in distance: never merged
	far := e.fx.runStart.Add(-100 * time.Second)
	if _, err := e.st.DB.Exec(`INSERT INTO activities (user_id, strava_id, start_utc, start_local, local_date, type, sport, name, distance_m, created_at, updated_at)
		VALUES (1, 9002, ?, '2026-01-01T00:00:00', '2026-01-01', 'running', 'run', 'Autre', 9000, 1, 1)`, far.Unix()); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DedupOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var n, notes int
	_ = e.st.DB.QueryRow(`SELECT COUNT(*) FROM activities WHERE user_id = 1 AND (garmin_id = 1001 OR strava_id = 9001)`).Scan(&n)
	_ = e.st.DB.QueryRow(`SELECT COUNT(*) FROM activity_notes WHERE activity_id = ?`, dupID).Scan(&notes)
	if n != 1 || notes != 1 || e.id(`SELECT id FROM activities WHERE garmin_id = 1001 AND strava_id = 9001`) != dupID {
		t.Fatalf("dedup: rows=%d notes=%d", n, notes)
	}
	if e.id(`SELECT COUNT(*) FROM activities WHERE strava_id = 9002 AND garmin_id IS NULL`) != 1 {
		t.Fatal("distance rule ignored")
	}
	if e.id(`SELECT COUNT(*) FROM app_settings WHERE key = 'v3_dedup_done'`) != 1 {
		t.Fatal("dedup marker not set")
	}
	if code, _ := e.do(1, "GET", fmt.Sprintf("/api/activities/%d", runID), nil, nil); code != 404 {
		t.Fatalf("dropped row still served: %d", code)
	}
	var v m
	if code, body := e.do(1, "GET", fmt.Sprintf("/api/activities/%d", dupID), nil, &v); code != 200 {
		t.Fatalf("fused view: %d %s", code, body)
	}
	act := v["activity"].(m)
	if v["sourceLabel"] != "Fusion" || act["name"] != "Sortie custom" || fmt.Sprint(act["sources"]) != "[garmin strava]" || act["garminGear"] != nil ||
		act["garminGearReason"] == nil {
		t.Fatalf("fusion: %v %v %v %v", v["sourceLabel"], act["name"], act["sources"], act["garminGearReason"])
	}
	ef, imp, rb := v["efficiency"].(m), v["impact"].(m), v["readiness"].(m)
	if ef["eligible"] != true || ef["efSpeed"].(float64) < 1 || imp["steps"].(float64) < 4000 || imp["jointIndex7d"] == nil || rb["level"] == nil || rb["title"] == "" {
		t.Fatalf("metrics: %v %v %v", ef, imp, rb)
	}
	var day m
	e.do(1, "GET", "/api/days/"+e.fx.day(-1), nil, &day)
	if day["readiness"].(m)["level"] == nil {
		t.Fatalf("day readiness: %v", day["readiness"])
	}
	var rep string
	_, rep = e.do(1, "GET", fmt.Sprintf("/api/activities/%d/report", dupID), nil, nil)
	if !strings.Contains(rep, "Efficience aérobie (EF) :** ") || !strings.Contains(rep, "Charge d'impact cumulée :** ") {
		t.Fatalf("report additions missing: %s", rep)
	}
	var trend m
	if code, body := e.do(1, "GET", "/api/trends/efficiency?weeks=12", nil, &trend); code != 200 || len(trend["points"].([]any)) != 1 ||
		len(trend["rollingEfSpeed"].([]any)) != 1 {
		t.Fatalf("trend: %d %s", code, body)
	}
	if code, _ := e.do(1, "GET", "/api/trends/efficiency?weeks=99", nil, nil); code != 400 {
		t.Fatal("weeks bound")
	}
	if code, _ := e.do(2, "GET", fmt.Sprintf("/api/activities/%d/track", dupID), nil, nil); code != 404 {
		t.Fatal("track of another user")
	}
	var tr m
	if code, _ := e.do(1, "GET", fmt.Sprintf("/api/activities/%d/track", dupID), nil, &tr); code != 200 || len(tr["points"].([]any)) != 0 || tr["bounds"] != nil {
		t.Fatalf("track without GPS: %v", tr)
	}
	var cal m
	from, to := e.fx.day(-3), e.fx.day(1)
	if code, _ := e.do(1, "GET", "/api/calendar?from="+from+"&to="+to, nil, &cal); code != 200 || len(cal["days"].([]any)) != 5 {
		t.Fatalf("calendar: %v", cal)
	}
	if code, _ := e.do(1, "GET", "/api/calendar?from=2026-01-01&to=2027-03-15", nil, nil); code != 400 {
		t.Fatal("calendar range")
	}
}

func TestV3StravaFirstThenGarmin(t *testing.T) {
	e := setup(t)
	sp := 86.0
	start := e.fx.runStart.Add(-250 * time.Second)
	e.fs.acts[555] = &strava.Activity{ID: 555, Name: "Titre Strava", SportType: "Run", StartDate: start.UTC(),
		StartDateLocal: start.In(paris).Format("2006-01-02T15:04:05Z"), DistanceM: 5350, MovingTimeS: 1800, AverageCadenceRaw: &sp, GearID: "g1"}
	if _, err := e.st.DB.Exec(`INSERT INTO shoes (user_id, name, brand, model, created_at, updated_at) VALUES (1, 'Mes runnings', 'Marque', 'Modèle', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	connectStrava(t, e, 1)
	if err := e.svc.MorningSync(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	e.svc.Wait()
	sid := e.id(`SELECT id FROM activities WHERE strava_id = 555`)
	var shoes []m
	e.do(1, "GET", "/api/shoes", nil, &shoes)
	if len(shoes) != 1 || shoes[0]["stravaGearId"] != "g1" {
		t.Fatalf("shoe dedup by brand/model: %v", shoes)
	}
	var notes m
	var v m
	e.do(1, "GET", fmt.Sprintf("/api/activities/%d", sid), nil, &v)
	notes = v["notes"].(m)
	if notes["shoeAuto"] != true || notes["shoeId"] == nil || v["activity"].(m)["hasNotes"] == true {
		t.Fatalf("auto shoe: %v", notes)
	}
	// Garmin arrives later: fused into the Strava row (250 s apart), Strava title kept
	bootGarmin(t, e)
	if got := e.id(`SELECT id FROM activities WHERE garmin_id = 1001`); got != sid {
		t.Fatalf("garmin not fused into strava row: %d vs %d", got, sid)
	}
	var name string
	_ = e.st.DB.QueryRow(`SELECT name FROM activities WHERE id = ?`, sid).Scan(&name)
	if name != "Titre Strava" {
		t.Fatalf("title precedence: %q", name)
	}
	// merge two shoes: notes move, source deleted
	var b m
	e.do(1, "POST", "/api/shoes", m{"name": "Doublon"}, &b)
	bid := int64(b["id"].(float64))
	aid := int64(shoes[0]["id"].(float64))
	e.do(1, "PUT", fmt.Sprintf("/api/activities/%d/notes", sid), m{"shoeId": bid}, nil)
	if code, _ := e.do(2, "POST", fmt.Sprintf("/api/shoes/%d/merge", bid), m{"intoId": aid}, nil); code != 404 {
		t.Fatal("merge of another user's shoe")
	}
	var merged m
	if code, body := e.do(1, "POST", fmt.Sprintf("/api/shoes/%d/merge", bid), m{"intoId": aid}, &merged); code != 200 || merged["id"].(float64) != float64(aid) {
		t.Fatalf("merge: %d %s", code, body)
	}
	e.do(1, "GET", "/api/shoes", nil, &shoes)
	var shoeID int64
	_ = e.st.DB.QueryRow(`SELECT shoe_id FROM activity_notes WHERE activity_id = ?`, sid).Scan(&shoeID)
	if len(shoes) != 1 || shoeID != aid {
		t.Fatalf("after merge: shoes=%d note shoe=%d", len(shoes), shoeID)
	}
}

func TestV3PlanWorkouts(t *testing.T) {
	var mode atomic.Value
	mode.Store("ok")
	var creates atomic.Int32
	var lastWorkout atomic.Value
	e := setupWith(t, func(mux *http.ServeMux) {
		mux.HandleFunc("POST /workout-service/workout", func(w http.ResponseWriter, r *http.Request) {
			creates.Add(1)
			raw, _ := io.ReadAll(r.Body)
			lastWorkout.Store(string(raw))
			if mode.Load() == "404" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = io.WriteString(w, `{"workoutId":555}`)
		})
		mux.HandleFunc("POST /workout-service/schedule/{id}", func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			if r.PathValue("id") != "555" || !strings.Contains(string(raw), `"date":"2026-11-10"`) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, `{}`)
		})
	}, nil)
	bootGarmin(t, e)
	text := "Voici :\n```json\n" + `{"format":"stride-plan","version":1,"sessions":[{"date":"2026-11-10","sport":"run","title":"Footing + 6×1'","targetDurationS":2700,"targetCadenceSpm":172,"steps":[{"type":"warmup","durationS":600,"target":{"kind":"pace","min":330,"max":360}},{"type":"repeat","repeat":6,"steps":[{"type":"interval","durationS":60,"target":{"kind":"pace","min":250,"max":260}},{"type":"recovery","durationS":90}]},{"type":"cooldown","durationS":600}]},{"date":"2026-11-11","sport":"run","steps":[{"type":"sprint"}]}]}` + "\n```"
	var imp struct {
		Drafts []m      `json:"drafts"`
		Errors []string `json:"errors"`
	}
	if code, body := e.do(1, "POST", "/api/plan/import", m{"text": text}, &imp); code != 200 || len(imp.Drafts) != 1 || len(imp.Errors) != 1 {
		t.Fatalf("import: %d %s", code, body)
	}
	var created []m
	if code, body := e.do(1, "POST", "/api/plan/bulk", m{"sessions": imp.Drafts}, &created); code != 201 || len(created) != 1 {
		t.Fatalf("bulk: %d %s", code, body)
	}
	ps := created[0]
	pid := int64(ps["id"].(float64))
	if ps["source"] != "import" || len(ps["steps"].([]any)) != 3 || ps["garmin"].(m)["status"] != "none" {
		t.Fatalf("created: %v", ps)
	}
	if code, _ := e.do(1, "POST", "/api/plan/bulk", m{"sessions": []m{{"date": "bad"}}}, nil); code != 400 {
		t.Fatal("bulk validation")
	}
	code, body := e.do(1, "GET", fmt.Sprintf("/api/plan/%d/workout.fit", pid), nil, nil)
	if code != 200 || len(body) < 14 || body[8:12] != ".FIT" {
		t.Fatalf("fit: %d", code)
	}
	if code, _ := e.do(2, "GET", fmt.Sprintf("/api/plan/%d/workout.fit", pid), nil, nil); code != 404 {
		t.Fatal("fit of another user")
	}
	var pushed m
	if code, body := e.do(1, "POST", fmt.Sprintf("/api/plan/%d/push-garmin", pid), nil, &pushed); code != 200 || pushed["garmin"].(m)["status"] != "pushed" ||
		pushed["garmin"].(m)["workoutId"].(float64) != 555 {
		t.Fatalf("push: %d %s", code, body)
	}
	wk := lastWorkout.Load().(string)
	for _, want := range []string{`"sportTypeKey":"running"`, `"RepeatGroupDTO"`, `"ExecutableStepDTO"`, `"pace.zone"`} {
		if !strings.Contains(wk, want) {
			t.Errorf("workout payload lacks %s", want)
		}
	}
	// the Personal API refuses: unsupported, cached 24 h
	mode.Store("404")
	var second []m
	e.do(1, "POST", "/api/plan/bulk", m{"sessions": imp.Drafts}, &second)
	pid2 := int64(second[0]["id"].(float64))
	for i := 0; i < 2; i++ {
		code, body := e.do(1, "POST", fmt.Sprintf("/api/plan/%d/push-garmin", pid2), nil, nil)
		if code != 409 || !strings.Contains(body, `"status":"unsupported"`) || !strings.Contains(body, `"code":"CONFLICT"`) {
			t.Fatalf("unsupported %d: %d %s", i, code, body)
		}
	}
	if creates.Load() != 2 {
		t.Fatalf("capability not cached: %d creates", creates.Load())
	}
	var mine []m
	e.do(2, "POST", "/api/plan/bulk", m{"sessions": imp.Drafts}, &mine)
	if code, _ := e.do(2, "POST", fmt.Sprintf("/api/plan/%d/push-garmin", int64(mine[0]["id"].(float64))), nil, nil); code != 412 {
		t.Fatal("push without Garmin must be 412")
	}
	var gen m
	if code, body := e.do(1, "POST", "/api/plan/generate", m{"mode": "prompt", "horizon": "week", "startDate": "2026-11-09", "note": "Pas de côtes"}, &gen); code != 200 ||
		!strings.HasPrefix(gen["prompt"].(string), "> **DIRECTIVE") || !strings.Contains(gen["prompt"].(string), `"format":"stride-plan"`) ||
		!strings.Contains(gen["prompt"].(string), "Pas de côtes") || gen["tokensEstimate"].(float64) <= 0 {
		t.Fatalf("generate prompt: %d %s", code, body)
	}
	if code, _ := e.do(1, "POST", "/api/plan/generate", m{"mode": "llm", "horizon": "session", "startDate": "2026-11-09"}, nil); code != 412 {
		t.Fatal("llm mode without connector must be 412")
	}
}

type llmFake struct {
	mu   sync.Mutex
	hdr  http.Header
	body m
	path string
}

func (f *llmFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.hdr, f.path, f.body = r.Header.Clone(), r.URL.Path, m{}
		_ = json.NewDecoder(r.Body).Decode(&f.body)
		if strings.Contains(r.Header.Get("x-api-key")+r.Header.Get("x-goog-api-key")+r.Header.Get("Authorization"), "refused") {
			w.WriteHeader(401)
			_, _ = io.WriteString(w, `{"error":{"message":"invalid api key"}}`)
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1beta/models/"):
			_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"## Analyse Gemini"}]}}]}`)
		case r.URL.Path == "/v1/messages":
			_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"## Analyse Claude"}]}`)
		default:
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"## Analyse OpenAI"}}]}`)
		}
	})
}

func TestV3LLM(t *testing.T) {
	fake := &llmFake{}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	e := setupWith(t, nil, func(d *domain.Deps) {
		d.Env.AllowPrivateEgress = true
		d.BaseURLs = map[string]string{"gemini": srv.URL, "anthropic": srv.URL, "openai": srv.URL}
	})
	bootGarmin(t, e)
	runID := e.id(`SELECT id FROM activities WHERE garmin_id = 1001`)
	if code, _ := e.do(1, "POST", "/api/llm/analyze", m{"kind": "activity", "activityId": runID}, nil); code != 412 {
		t.Fatal("analyze without connector")
	}
	var cfg m
	if code, body := e.do(1, "PUT", "/api/llm", m{"provider": "gemini", "model": "gemini-2.5-flash", "apiKey": "k-gemini-synthetique-1234"}, &cfg); code != 200 ||
		cfg["configured"] != true || cfg["keyHint"] != "••••1234" {
		t.Fatalf("put: %d %s", code, body)
	}
	var enc []byte
	_ = e.st.DB.QueryRow(`SELECT key_enc FROM llm_configs WHERE user_id = 1`).Scan(&enc)
	if bytes.Contains(enc, []byte("synthetique")) || len(enc) == 0 {
		t.Fatal("LLM key stored in clear")
	}
	var test m
	if e.do(1, "POST", "/api/llm/test", nil, &test); test["ok"] != true {
		t.Fatalf("test: %v", test)
	}
	var an m
	if code, body := e.do(1, "POST", "/api/llm/analyze", m{"kind": "activity", "activityId": runID, "include": "biomech,load", "question": "Ma cadence ?"}, &an); code != 200 ||
		an["markdown"] != "## Analyse Gemini" || an["ref"] != fmt.Sprint(runID) {
		t.Fatalf("analyze gemini: %d %s", code, body)
	}
	sys := fake.body["systemInstruction"].(m)["parts"].([]any)[0].(m)["text"].(string)
	user := fake.body["contents"].([]any)[0].(m)["parts"].([]any)[0].(m)["text"].(string)
	if fake.path != "/v1beta/models/gemini-2.5-flash:generateContent" || fake.hdr.Get("x-goog-api-key") != "k-gemini-synthetique-1234" ||
		!strings.HasPrefix(sys, "**DIRECTIVE") || !strings.HasPrefix(user, "> **DIRECTIVE") || !strings.Contains(user, "QUESTION DE L'ATHLÈTE") ||
		fake.body["generationConfig"].(m)["maxOutputTokens"].(float64) != 4096 {
		t.Fatalf("gemini request: %s %v", fake.path, fake.hdr)
	}
	e.do(1, "PUT", "/api/llm", m{"provider": "anthropic", "model": "claude-sonnet-4-5", "apiKey": "k-anthropic-synthetique"}, nil)
	if code, body := e.do(1, "POST", "/api/llm/analyze", m{"kind": "day", "date": e.fx.day(-1)}, &an); code != 200 || an["markdown"] != "## Analyse Claude" {
		t.Fatalf("analyze anthropic: %d %s", code, body)
	}
	if fake.path != "/v1/messages" || fake.hdr.Get("x-api-key") != "k-anthropic-synthetique" || fake.hdr.Get("anthropic-version") != "2023-06-01" ||
		fake.body["model"] != "claude-sonnet-4-5" || fake.body["max_tokens"].(float64) != 4096 || !strings.HasPrefix(fake.body["system"].(string), "**DIRECTIVE") ||
		fake.body["messages"].([]any)[0].(m)["role"] != "user" {
		t.Fatalf("anthropic request: %v", fake.body)
	}
	var job m
	if code, body := e.do(1, "POST", "/api/llm/analyze?async=1", m{"kind": "day", "date": e.fx.day(-1)}, &job); code != 202 ||
		job["kind"] != "llm.analyze" || job["state"] != "queued" {
		t.Fatalf("async analyze: %d %s", code, body)
	}
	e.svc.Wait()
	if code, body := e.do(1, "GET", fmt.Sprintf("/api/jobs/%v", job["id"]), nil, &job); code != 200 || job["state"] != "completed" ||
		job["result"].(m)["markdown"] != "## Analyse Claude" {
		t.Fatalf("async analyze job: %d %s", code, body)
	}
	if code, _ := e.do(2, "GET", fmt.Sprintf("/api/jobs/%v", job["id"]), nil, nil); code != 404 {
		t.Fatal("job visible to another user")
	}
	e.do(1, "PUT", "/api/llm", m{"provider": "openai", "model": "gpt-4.1", "apiKey": "k-openai-synthetique"}, nil)
	if code, body := e.do(1, "POST", "/api/llm/analyze", m{"kind": "multi", "weeks": 2, "include": []string{"load"}}, &an); code != 200 || an["markdown"] != "## Analyse OpenAI" {
		t.Fatalf("analyze openai: %d %s", code, body)
	}
	msgs := fake.body["messages"].([]any)
	if fake.path != "/v1/chat/completions" || fake.hdr.Get("Authorization") != "Bearer k-openai-synthetique" || fake.body["max_completion_tokens"] == nil ||
		msgs[0].(m)["role"] != "system" || msgs[1].(m)["role"] != "user" {
		t.Fatalf("openai request: %v", fake.body)
	}
	// compatible server (Ollama) without key, reached through the egress guard (private allowed here)
	if code, body := e.do(1, "PUT", "/api/llm", m{"provider": "openai_compat", "model": "llama3.1:8b", "baseUrl": srv.URL + "/v1", "apiKey": ""}, &cfg); code != 200 ||
		cfg["keyHint"] != nil || cfg["baseUrl"] != srv.URL {
		t.Fatalf("compat: %d %s", code, body)
	}
	if code, _ := e.do(1, "POST", "/api/llm/analyze", m{"kind": "activity", "activityId": runID}, &an); code != 200 || fake.hdr.Get("Authorization") != "" ||
		fake.body["max_tokens"] == nil {
		t.Fatalf("compat request: %v %v", fake.hdr, fake.body)
	}
	var list []m
	if e.do(1, "GET", fmt.Sprintf("/api/llm/analyses?kind=activity&ref=%d", runID), nil, &list); len(list) != 2 {
		t.Fatalf("analyses: %d", len(list))
	}
	if e.do(2, "GET", "/api/llm/analyses", nil, &list); len(list) != 0 {
		t.Fatal("analyses leak across users")
	}
	e.do(1, "PUT", "/api/llm", m{"provider": "openai", "model": "gpt-4.1", "apiKey": "k-refused-synthetique"}, nil)
	if e.do(1, "POST", "/api/llm/test", nil, &test); test["ok"] != false || !strings.Contains(test["message"].(string), "invalid api key") {
		t.Fatalf("refused key: %v", test)
	}
	if code, _ := e.do(1, "PUT", "/api/llm", m{"provider": "gemini", "model": "gemini-2.5-flash", "apiKey": ""}, nil); code != 400 {
		t.Fatal("gemini without key accepted")
	}
	if code, _ := e.do(1, "DELETE", "/api/llm", nil, nil); code != 204 {
		t.Fatal("delete")
	}
	if e.do(1, "GET", "/api/llm", nil, &cfg); cfg["configured"] != false {
		t.Fatal("still configured")
	}
	// default guard: private base URLs are refused
	strict := setup(t)
	if code, _ := strict.do(1, "PUT", "/api/llm", m{"provider": "openai_compat", "model": "llama3", "baseUrl": "http://127.0.0.1:11434"}, nil); code != 400 {
		t.Fatal("private base URL accepted")
	}
	if code, _ := strict.do(1, "PUT", "/api/llm", m{"provider": "openai_compat", "model": "llama3", "baseUrl": "file:///etc/passwd"}, nil); code != 400 {
		t.Fatal("non-http base URL accepted")
	}
}

type hookFake struct {
	mu    sync.Mutex
	calls map[string][]string
}

func (f *hookFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		key := r.URL.Path
		if strings.HasPrefix(key, "/bot") {
			key = "telegram"
		}
		f.calls[key] = append(f.calls[key], string(raw))
		f.mu.Unlock()
		if key == "/gone" {
			w.WriteHeader(410)
			return
		}
		w.WriteHeader(204)
	})
}

func TestV3NotificationsAndImport(t *testing.T) {
	hooks := &hookFake{calls: map[string][]string{}}
	srv := httptest.NewServer(hooks.handler())
	t.Cleanup(srv.Close)
	e := setupWith(t, nil, func(d *domain.Deps) {
		d.Env.AllowPrivateEgress = true
		d.BaseURLs = map[string]string{"telegram": srv.URL}
		d.JobBackoff = time.Millisecond
	})
	bootGarmin(t, e)
	var dc, tg, js m
	if code, body := e.do(1, "POST", "/api/notifications", m{"kind": "discord", "label": "Mon Discord", "enabled": true, "url": srv.URL + "/discord/secret-path"}, &dc); code != 201 ||
		strings.Contains(body, "secret-path") || dc["hint"] == "" {
		t.Fatalf("discord: %d %s", code, body)
	}
	if code, body := e.do(1, "POST", "/api/notifications", m{"kind": "telegram", "enabled": true, "botToken": "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ", "chatId": "-100123"}, &tg); code != 201 ||
		strings.Contains(body, "ABCDEFGHIJ") {
		t.Fatalf("telegram: %d %s", code, body)
	}
	e.do(1, "POST", "/api/notifications", m{"kind": "json", "enabled": true, "url": srv.URL + "/json"}, &js)
	if code, _ := e.do(1, "POST", "/api/notifications", m{"kind": "telegram", "botToken": "nope", "chatId": "1"}, nil); code != 400 {
		t.Fatal("invalid telegram token accepted")
	}
	if code, _ := e.do(1, "POST", "/api/notifications", m{"kind": "discord"}, nil); code != 400 {
		t.Fatal("discord without url accepted")
	}
	var sealed []byte
	_ = e.st.DB.QueryRow(`SELECT secret_enc FROM notification_targets WHERE kind = 'discord'`).Scan(&sealed)
	if bytes.Contains(sealed, []byte("secret-path")) {
		t.Fatal("webhook URL stored in clear")
	}
	var res m
	if code, _ := e.do(1, "POST", fmt.Sprintf("/api/notifications/%v/test", tg["id"]), nil, &res); code != 200 || res["ok"] != true {
		t.Fatalf("test telegram: %v", res)
	}
	if code, _ := e.do(2, "POST", fmt.Sprintf("/api/notifications/%v/test", tg["id"]), nil, nil); code != 404 {
		t.Fatal("test of another user's target")
	}
	var dead, gone m
	e.do(1, "POST", "/api/notifications", m{"kind": "json", "label": "Morte", "enabled": true, "url": "http://127.0.0.1:1/hook/s3cr3t-tok"}, &dead)
	e.do(1, "POST", "/api/notifications", m{"kind": "json", "label": "Disparue", "enabled": true, "url": srv.URL + "/gone"}, &gone)
	// dispatch: recent complete activities, once
	for i := 0; i < 2; i++ {
		if err := e.svc.MorningSync(context.Background(), 1); err != nil {
			t.Fatal(err)
		}
		e.svc.Wait()
	}
	hooks.mu.Lock()
	jsonCalls, discord := hooks.calls["/json"], hooks.calls["/discord/secret-path"]
	tgCalls := hooks.calls["telegram"]
	hooks.mu.Unlock()
	if len(jsonCalls) != 2 || len(discord) != 2 || len(tgCalls) != 3 {
		t.Fatalf("dispatch counts: json=%d discord=%d telegram=%d", len(jsonCalls), len(discord), len(tgCalls))
	}
	all := strings.Join(jsonCalls, "\n")
	if !strings.Contains(all, `"event":"activity.synced"`) || !strings.Contains(all, `/?s=`) || !strings.Contains(all, `"efficiency"`) ||
		!strings.Contains(strings.Join(discord, ""), "km") || !strings.Contains(strings.Join(tgCalls, ""), `"parse_mode":"Markdown"`) {
		t.Fatalf("payloads: %s", all)
	}
	// refused connections are retried, 4xx are not, and no job error exposes the target URL
	attempts := func(label string) (n int, state string) {
		_ = e.st.DB.QueryRow(`SELECT attempts, state FROM async_jobs WHERE kind = 'webhook.deliver' AND label = ?`, "Notification "+label).Scan(&n, &state)
		return n, state
	}
	if n, state := attempts(fmt.Sprint(dead["label"])); n != 3 || state != "failed" {
		t.Fatalf("dead target: %d attempts, %s", n, state)
	}
	if n, state := attempts(fmt.Sprint(gone["label"])); n != 1 || state != "failed" {
		t.Fatalf("410 target: %d attempts, %s", n, state)
	}
	if _, body := e.do(1, "GET", "/api/jobs?limit=100", nil, nil); strings.Contains(body, "s3cr3t") || strings.Contains(body, "127.0.0.1:1/") ||
		!strings.Contains(body, "Échec de l'envoi") {
		t.Fatalf("job errors: %s", body)
	}
	var list []m
	e.do(1, "PATCH", fmt.Sprintf("/api/notifications/%v", dc["id"]), m{"enabled": false}, nil)
	e.do(1, "GET", "/api/notifications", nil, &list)
	if len(list) != 5 || list[0]["enabled"] != false || list[1]["lastStatus"] != "ok" || list[3]["lastStatus"] != "error" ||
		strings.Contains(fmt.Sprint(list[3]["lastError"]), "s3cr3t") {
		t.Fatalf("list: %v", list)
	}
	if code, _ := e.do(1, "DELETE", fmt.Sprintf("/api/notifications/%v", js["id"]), nil, nil); code != 204 {
		t.Fatal("delete")
	}

	// rescue import: GPX for a user without providers, then a TCX merged into the Garmin run
	var imp m
	if code, body := e.upload(2, "sortie.gpx", gpxFile(e.fx.runStart.Add(-48*time.Hour), 1800), &imp); code != 201 || imp["merged"] != false {
		t.Fatalf("gpx import: %d %s", code, body)
	}
	gid := int64(imp["activityId"].(float64))
	var v m
	if code, body := e.do(2, "GET", fmt.Sprintf("/api/activities/%d", gid), nil, &v); code != 200 || v["sourceLabel"] != "Import" ||
		v["efficiency"].(m)["eligible"] != true || v["impact"].(m)["descentFactor"] == nil {
		t.Fatalf("imported view: %d %s", code, body)
	}
	var tr struct {
		Points [][]*float64 `json:"points"`
		Bounds m            `json:"bounds"`
	}
	if code, _ := e.do(2, "GET", fmt.Sprintf("/api/activities/%d/track", gid), nil, &tr); code != 200 || len(tr.Points) == 0 || len(tr.Points) > 1500 ||
		tr.Bounds == nil || *tr.Points[0][3] != 172 || len(tr.Points[0]) != 5 {
		t.Fatalf("track: %d points", len(tr.Points))
	}
	if code, body := e.upload(2, "sortie.gpx", gpxFile(e.fx.runStart.Add(-48*time.Hour), 1800), &imp); code != 200 || imp["merged"] != true ||
		int64(imp["activityId"].(float64)) != gid {
		t.Fatalf("re-import: %d %s", code, body)
	}
	runID := e.id(`SELECT id FROM activities WHERE garmin_id = 1001`)
	if code, body := e.upload(1, "montre.tcx", tcxFile(e.fx.runStart.Add(30*time.Second), 1800), &imp); code != 200 || imp["merged"] != true ||
		int64(imp["activityId"].(float64)) != runID {
		t.Fatalf("tcx merge: %d %s", code, body)
	}
	e.do(1, "GET", fmt.Sprintf("/api/activities/%d", runID), nil, &v)
	if fmt.Sprint(v["activity"].(m)["sources"]) != "[garmin import]" {
		t.Fatalf("sources after merge: %v", v["activity"].(m)["sources"])
	}
	if code, _ := e.do(1, "GET", fmt.Sprintf("/api/activities/%d/track", runID), nil, &tr); code != 200 || len(tr.Points) == 0 {
		t.Fatal("track from merged TCX")
	}
	if code, _ := e.upload(1, "notes.txt", []byte("bonjour"), nil); code != 400 {
		t.Fatal("unknown format accepted")
	}
}
