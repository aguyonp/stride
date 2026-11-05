package domainapi_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"stride/internal/apimeter"
	"stride/internal/auth"
	"stride/internal/cache"
	"stride/internal/domain"
	"stride/internal/domainapi"
	"stride/internal/env"
	"stride/internal/provider/strava"
	"stride/internal/provider/weather"
	"stride/internal/secret"
	"stride/internal/store"
)

const goodKey = "cle-synthetique-de-test-abcd"

var paris, _ = time.LoadLocation("Europe/Paris")

type fixture struct {
	now      time.Time
	runStart time.Time // yesterday 18:00 local
	calls    atomic.Int32
}

func fakeJWT() string {
	p := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, time.Now().Add(24*time.Hour).Unix())))
	return "e30." + p + ".sig"
}

func (f *fixture) day(n int) string { return f.now.In(paris).AddDate(0, 0, n).Format("2006-01-02") }

func (f *fixture) garmin() http.Handler {
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, s string) { _, _ = io.WriteString(w, s) }
	mux.HandleFunc("POST /personalapitoken-service/personal-api/personal-api-key/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "ApiKey "+goodKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		write(w, fakeJWT())
	})
	gmt := func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05") }
	loc := func(t time.Time) string { return t.In(paris).Format("2006-01-02 15:04:05") }
	run, bike, old := f.runStart, f.runStart.Add(-10*time.Hour), f.runStart.AddDate(0, 0, -10)
	mux.HandleFunc("GET /activitylist-service/activities/search/activities", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		write(w, fmt.Sprintf(`[
{"activityId":1001,"activityName":"Footing","startTimeLocal":%q,"startTimeGMT":%q,"activityType":{"typeKey":"running"},"distance":5400,"duration":1800,"elapsedDuration":1800,"averageSpeed":3.0,"avgGradeAdjustedSpeed":3.05,"averageHR":140,"averageRunningCadenceInStepsPerMinute":172,"avgGroundContactTime":250,"elevationGain":20,"elevationLoss":35,"activityTrainingLoad":80,"hrTimeInZone_1":60,"hrTimeInZone_2":900,"hrTimeInZone_3":700,"hrTimeInZone_4":100,"hrTimeInZone_5":40},
{"activityId":1002,"activityName":"Vélotaf","startTimeLocal":%q,"startTimeGMT":%q,"activityType":{"typeKey":"cycling"},"distance":8000,"duration":1500,"averageHR":120,"activityTrainingLoad":20},
{"activityId":1003,"activityName":"Footing","startTimeLocal":%q,"startTimeGMT":%q,"activityType":{"typeKey":"running"},"distance":6000,"duration":2100,"averageSpeed":2.86,"activityTrainingLoad":70}]`,
			loc(run), gmt(run), loc(bike), gmt(bike), loc(old), gmt(old)))
	})
	mux.HandleFunc("GET /activity-service/activity/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if r.PathValue("id") != "1001" {
			write(w, `{"activityTypeDTO":{"typeKey":"cycling"},"summaryDTO":{}}`)
			return
		}
		write(w, fmt.Sprintf(`{"activityId":1001,"activityName":"Footing","activityTypeDTO":{"typeKey":"running"},"summaryDTO":{"startTimeLocal":%q,"startTimeGMT":%q,
"startLatitude":45.1234,"startLongitude":5.6789,"distance":5400,"duration":1800,"elapsedDuration":1800,"averageSpeed":3.0,"averageHR":140,"averageRunCadence":172,
"groundContactTime":250,"verticalOscillation":8.9,"strideLength":104,"trainingEffect":3.1,"activityTrainingLoad":80,"impactLoad":1234,"avgGradeAdjustedSpeed":3.05,
"beginPotentialStamina":100,"endPotentialStamina":81,"minAvailableStamina":64,"elevationGain":20,"elevationLoss":35}}`, loc(run), gmt(run)))
	})
	mux.HandleFunc("GET /activity-service/activity/{id}/details", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		var rows []string
		ms := run.UnixMilli()
		alt, dist := 300.0, 0.0
		for i := 0; i < 1800; i++ {
			cad, gct := 174, 245
			if i >= 1200 {
				gct = 262
			}
			if i >= 900 && i < 1200 {
				alt -= 0.18
				cad = 168
			}
			dist += 3
			rows = append(rows, fmt.Sprintf(`{"metrics":[%d,%d,%d,3.0,3.05,%.2f,%.1f,%d,9.0,%d,%d]}`, ms+int64(i)*1000, 135+i/120, cad, alt, dist, gct, 100-i/90, 90-i/60))
		}
		write(w, `{"metricDescriptors":[{"metricsIndex":0,"key":"directTimestamp"},{"metricsIndex":1,"key":"directHeartRate"},{"metricsIndex":2,"key":"directDoubleCadence"},
{"metricsIndex":3,"key":"directSpeed"},{"metricsIndex":4,"key":"directGradeAdjustedSpeed"},{"metricsIndex":5,"key":"directElevation"},{"metricsIndex":6,"key":"sumDistance"},
{"metricsIndex":7,"key":"directGroundContactTime"},{"metricsIndex":8,"key":"directVerticalOscillation"},{"metricsIndex":9,"key":"directPotentialStamina"},{"metricsIndex":10,"key":"directAvailableStamina"}],
"activityDetailMetrics":[`+strings.Join(rows, ",")+`]}`)
	})
	mux.HandleFunc("GET /download-service/files/activity/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("GET /sleep-service/sleep/dailySleepSummariesByDate", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		var items []string
		for _, d := range []string{f.day(-1), f.day(0)} {
			items = append(items, fmt.Sprintf(`{"calendarDate":%q,"sleepTimeSeconds":26000,"deepSleepSeconds":4800,"sleepScores":{"overall":{"value":78,"qualifierKey":"GOOD"}}}`, d))
		}
		write(w, "["+strings.Join(items, ",")+"]")
	})
	mux.HandleFunc("GET /hrv-service/hrv/daily/{a}/{b}", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		write(w, fmt.Sprintf(`{"hrvSummaries":[{"calendarDate":%q,"lastNightAvg":48,"weeklyAvg":50,"status":"BALANCED","baseline":{"balancedLow":42,"balancedUpper":58}}]}`, f.day(-1)))
	})
	mux.HandleFunc("GET /usersummary-service/v2/dailysummaries", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		write(w, fmt.Sprintf(`{"data":[{"calendarDate":%q,"movement":{"steps":{"value":12000},"floorsAscended":{"value":18},"floorsDescended":{"value":16}},"stress":{"avgLevel":33},"heartRate":{"restingValue":47}}]}`, f.day(-2)))
	})
	mux.HandleFunc("GET /usersummary-service/usersummary/daily", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		write(w, fmt.Sprintf(`{"calendarDate":%q,"totalSteps":9000,"dailyStepGoal":8000,"floorsAscended":%d,"floorsDescended":3,"averageStressLevel":28,"restingHeartRate":48,"lastSevenDaysAvgRestingHeartRate":47,"bodyBatteryAtWakeTime":82,"bodyBatteryChargedValue":60,"bodyBatteryDrainedValue":40}`, r.URL.Query().Get("calendarDate"), map[bool]int{true: 18, false: 4}[r.URL.Query().Get("calendarDate") == f.day(-2)]))
	})
	mux.HandleFunc("GET /biometric-service/heartRateZones/v2", func(w http.ResponseWriter, r *http.Request) {
		write(w, `[{"sport":"RUNNING","zone1Floor":100,"zone2Floor":120,"zone3Floor":140,"zone4Floor":160,"zone5Floor":175}]`)
	})
	return mux
}

type fakeStrava struct {
	acts     map[int64]*strava.Activity
	subs     atomic.Int32
	deauthed atomic.Int32
}

func (f *fakeStrava) AuthorizeURL(state, redirect string) string {
	return "https://strava.example/authorize?state=" + state + "&redirect_uri=" + redirect
}
func (f *fakeStrava) Exchange(context.Context, string) (strava.Tokens, error) {
	return strava.Tokens{AccessToken: "at-synthetique", RefreshToken: "rt-synthetique", ExpiresAt: time.Now().Add(6 * time.Hour),
		Scope: "read,activity:read_all,profile:read_all", AthleteID: 4242, AthleteName: "Athlète Test"}, nil
}
func (f *fakeStrava) Refresh(context.Context, string) (strava.Tokens, error) {
	return f.Exchange(context.Background(), "")
}
func (f *fakeStrava) Deauthorize(context.Context, string) error { f.deauthed.Add(1); return nil }
func (f *fakeStrava) ListActivities(context.Context, string, time.Time, time.Time, int, int) ([]strava.Activity, error) {
	var out []strava.Activity
	for _, a := range f.acts {
		out = append(out, *a)
	}
	return out, nil
}
func (f *fakeStrava) GetActivity(_ context.Context, _ string, id int64) (*strava.Activity, error) {
	if a, ok := f.acts[id]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeStrava) GetStreams(context.Context, string, int64) (*strava.Streams, error) {
	st := &strava.Streams{}
	alt, dist := 200.0, 0.0
	for i := 0; i < 1500; i++ {
		dist += 2.9
		if i > 700 && i < 1000 {
			alt -= 0.15
		}
		st.TimeS = append(st.TimeS, float64(i))
		st.DistanceM = append(st.DistanceM, dist)
		st.HeartRate = append(st.HeartRate, 140)
		st.CadenceRaw = append(st.CadenceRaw, 86)
		st.AltitudeM = append(st.AltitudeM, alt)
		st.VelocityMS = append(st.VelocityMS, 2.9)
		st.Moving = append(st.Moving, true)
	}
	return st, nil
}
func (f *fakeStrava) GetGear(context.Context, string, string) (*strava.Gear, error) { return nil, nil }
func (f *fakeStrava) ListAthleteGear(context.Context, string) ([]strava.Gear, error) {
	return []strava.Gear{{ID: "g1", Name: "Chaussure A", BrandName: "Marque", ModelName: "Modèle", Kind: "shoe"}}, nil
}
func (f *fakeStrava) EnsureSubscription(context.Context, string, string) (int64, error) {
	f.subs.Add(1)
	return 1, nil
}

type fakeWeather struct{}

func (fakeWeather) At(_ context.Context, lat, lon float64, _ time.Time) (*weather.Conditions, error) {
	if lat != 45.12 || lon != 5.68 {
		return nil, fmt.Errorf("coordinates not rounded: %v %v", lat, lon)
	}
	t, h, wk, wd := 12.0, 80.0, 15.0, 225.0
	return &weather.Conditions{Source: "open-meteo", TemperatureC: &t, HumidityPct: &h, WindKph: &wk, WindDirDeg: &wd, Description: "Nuageux"}, nil
}

type env2 struct {
	t   *testing.T
	srv *httptest.Server
	svc *domain.Services
	st  *store.Store
	fx  *fixture
	fs  *fakeStrava
}

func setup(t *testing.T) *env2 {
	t.Helper()
	ctx := context.Background()
	fx := &fixture{now: time.Now()}
	y := fx.now.In(paris).AddDate(0, 0, -1)
	fx.runStart = time.Date(y.Year(), y.Month(), y.Day(), 18, 0, 0, 0, paris)
	gsrv := httptest.NewServer(fx.garmin())
	t.Cleanup(gsrv.Close)
	st, err := store.Open(ctx, t.TempDir()+"/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	sealer, _ := secret.New(key, nil)
	for i, name := range []string{"admin", "alice", "bob"} {
		role := "user"
		if i == 0 {
			role = "admin"
		}
		if _, err := st.DB.Exec(`INSERT INTO users (id, display_name, role, created_at) VALUES (?, ?, ?, 1)`, i+1, name, role); err != nil {
			t.Fatal(err)
		}
	}
	fs := &fakeStrava{acts: map[int64]*strava.Activity{}}
	cfg := env.Config{GarminBaseURL: gsrv.URL, Timezone: "Europe/Paris", PublicURL: "http://localhost:8080", WeatherProvider: "open-meteo",
		StravaClientID: "1", StravaClientSecret: "x", StravaWebhookToken: "verif", AdminGarminKey: goodKey}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := domain.New(domain.Deps{Env: cfg, Store: st, Cache: cache.NewMemory(), Sealer: sealer, Meter: apimeter.Nop(),
		Strava: fs, Weather: fakeWeather{}, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	requireUser := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := strconv.ParseInt(r.Header.Get("X-Test-User"), 10, 64)
			if id == 0 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			role := auth.RoleUser
			if id == 1 {
				role = auth.RoleAdmin
			}
			ctx := auth.WithUser(apimeter.WithUser(r.Context(), id), auth.User{ID: id, Role: role})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	requireAdmin := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u, _ := auth.UserFrom(r.Context()); !u.IsAdmin() {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	domainapi.Register(mux, domainapi.Deps{Services: svc, RequireUser: requireUser, RequireAdmin: requireAdmin, Log: log})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Cleanup(svc.Wait)
	return &env2{t: t, srv: srv, svc: svc, st: st, fx: fx, fs: fs}
}

func (e *env2) do(user int, method, path string, body any, out any) (int, string) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		if s, ok := body.(string); ok {
			rd = strings.NewReader(s)
		} else {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	if user > 0 {
		req.Header.Set("X-Test-User", strconv.Itoa(user))
	}
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := cl.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			e.t.Fatalf("%s %s: decode: %v (%s)", method, path, err, raw)
		}
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return resp.StatusCode, resp.Header.Get("Location")
	}
	return resp.StatusCode, string(raw)
}

type m = map[string]any

func TestDomainEndToEnd(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	// first admin gets the env key; secrets are encrypted at rest
	if err := e.svc.OnUserCreated(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	e.svc.Wait()
	var integ m
	if code, _ := e.do(1, "GET", "/api/integrations", nil, &integ); code != 200 {
		t.Fatalf("integrations: %d", code)
	}
	g := integ["garmin"].(m)
	if g["connected"] != true || g["keyHint"] != "••••abcd" || g["tokenExpiresAt"] == nil {
		t.Fatalf("garmin integration: %+v", g)
	}
	var secretBlob, tokenBlob []byte
	_ = e.st.DB.QueryRow(`SELECT secret_enc, token_enc FROM provider_connections WHERE user_id = 1`).Scan(&secretBlob, &tokenBlob)
	if bytes.Contains(secretBlob, []byte(goodKey)) || bytes.Contains(tokenBlob, []byte("e30.")) || len(secretBlob) == 0 {
		t.Fatal("credentials stored in clear")
	}
	if code, body := e.do(2, "PUT", "/api/integrations/garmin", m{"apiKey": "mauvaise-cle-synthetique"}, nil); code != 400 || !strings.Contains(body, "PROVIDER_UNAUTHORIZED") {
		t.Fatalf("bad key: %d %s", code, body)
	}

	// a recent sync debounces lazy syncs but is not reported as a rate-limit cooldown
	var sync m
	if code, _ := e.do(1, "POST", "/api/sync", nil, &sync); code != 200 || sync["cooldownUntil"] != nil || sync["running"] != false {
		t.Fatalf("lazy sync right after a sync: %d %+v", code, sync)
	}

	// listing (synced by OnUserCreated) and isolation
	var list struct {
		Items []struct {
			ID         int64    `json:"id"`
			Sport      string   `json:"sport"`
			GapSec     *float64 `json:"gapSecPerKm"`
			Sources    []string `json:"sources"`
			PlanStatus *string  `json:"planStatus"`
		} `json:"items"`
		HasMore   bool `json:"hasMore"`
		Freshness m    `json:"freshness"`
	}
	if code, body := e.do(1, "GET", "/api/activities?limit=2", nil, &list); code != 200 || len(list.Items) != 2 || !list.HasMore {
		t.Fatalf("list: %d %s", code, body)
	}
	runID := list.Items[0].ID
	if list.Items[0].Sport != "run" || list.Items[0].GapSec == nil || list.Freshness["source"] == nil {
		t.Errorf("list item: %+v %+v", list.Items[0], list.Freshness)
	}
	if code, _ := e.do(2, "GET", fmt.Sprintf("/api/activities/%d", runID), nil, nil); code != 404 {
		t.Errorf("other user's activity must be 404, got %d", code)
	}
	if code, _ := e.do(2, "PUT", fmt.Sprintf("/api/activities/%d/notes", runID), m{"painDuring": 2}, nil); code != 404 {
		t.Errorf("other user's notes must be 404, got %d", code)
	}
	if code, _ := e.do(0, "GET", "/api/activities", nil, nil); code != 401 {
		t.Errorf("anonymous must be 401, got %d", code)
	}

	// athlete data
	if code, body := e.do(1, "PUT", "/api/profile", m{"focusArea": "knee_pfps", "injuredSide": "left"}, nil); code != 200 {
		t.Fatalf("profile: %d %s", code, body)
	}
	var shoe m
	if code, body := e.do(1, "POST", "/api/shoes", m{"name": "Route", "dropMm": 8, "type": "soft"}, &shoe); code != 201 {
		t.Fatalf("shoe: %d %s", code, body)
	}
	if code, _ := e.do(1, "PUT", fmt.Sprintf("/api/activities/%d/notes", runID), m{"painDuring": 3.3}, nil); code != 400 {
		t.Errorf("pain 3.3 must be rejected, got %d", code)
	}
	var notes m
	if code, body := e.do(1, "PUT", fmt.Sprintf("/api/activities/%d/notes", runID),
		m{"painDuring": 3.5, "painNextMorning": 4, "rpe": 6, "shoeId": shoe["id"], "tags": []string{"metronome"}, "note": "RAS"}, &notes); code != 200 {
		t.Fatalf("notes: %d %s", code, body)
	}
	if notes["shoe"] == nil || notes["updatedAt"] == nil {
		t.Errorf("notes: %+v", notes)
	}
	var goal m
	e.do(1, "POST", "/api/goals", m{"title": "Courir 45 min sans gêne", "kind": "macro", "targetDate": e.fx.day(20)}, &goal)
	var ps m
	if code, body := e.do(1, "POST", "/api/plan", m{"date": e.fx.day(-1), "sport": "run", "title": "Footing", "targetDurationS": 1800,
		"targetPaceMinSecPerKm": 320, "targetPaceMaxSecPerKm": 340, "targetCadenceSpm": 170}, &ps); code != 201 {
		t.Fatalf("plan: %d %s", code, body)
	}
	if mt, _ := ps["match"].(m); mt == nil || mt["status"] != "done" {
		t.Errorf("plan match: %+v", ps["match"])
	}
	e.do(1, "POST", "/api/plan", m{"date": e.fx.day(2), "sport": "run", "title": "Fractionné", "targetDurationS": 2400}, nil)
	if code, _ := e.do(1, "PUT", "/api/days/today/checkin", m{"trainingReadiness": 72, "readinessStatus": "Prêt"}, nil); code != 200 {
		t.Errorf("checkin: %d", code)
	}
	if code, _ := e.do(1, "PUT", "/api/days/"+e.fx.day(-1)+"/checkin", m{"trainingReadiness": 65, "readinessStatus": "Modéré"}, nil); code != 200 {
		t.Errorf("checkin: %d", code)
	}

	// activity view
	var v m
	if code, body := e.do(1, "GET", fmt.Sprintf("/api/activities/%d", runID), nil, &v); code != 200 {
		t.Fatalf("view: %d %s", code, body)
	}
	act := v["activity"].(m)
	bio := v["biomechanics"].(m)
	load := v["load"].(m)
	cl := v["crossLoad"].(m)
	checks := map[string]any{
		"sourceLabel": v["sourceLabel"], "gap": act["gapSecPerKm"], "stamina": act["stamina"], "impact": load["impactLoad"],
		"loadRatio": load["loadRatio"], "fatigueDrift": bio["fatigueDrift"], "slope": bio["slope"], "weather": v["weather"],
		"recovery": v["recovery"], "plan": v["plan"], "performance": v["performance"], "history": v["history"],
	}
	for k, val := range checks {
		if val == nil {
			t.Errorf("view.%s is null", k)
		}
	}
	if bike := cl["bike"].(m); bike["rides"].(float64) != 1 || bike["distanceKm"].(float64) != 8 {
		t.Errorf("cross load bike: %+v", bike)
	}
	if fl := cl["floors"].(m); fl["prevDayUp"] == nil {
		t.Errorf("floors J-1 missing: %+v", fl)
	}
	if w := v["weather"].(m); w["windDirLabel"] != "SO" || w["temperatureC"].(float64) != 12 {
		t.Errorf("weather: %+v", w)
	}
	if rd := v["recovery"].(m)["readiness"].(m); rd["source"] != "manual" || rd["score"].(float64) != 65 {
		t.Errorf("readiness: %+v", rd)
	}
	codes := map[string]bool{}
	for _, a := range v["alerts"].([]any) {
		codes[a.(m)["code"].(string)] = true
	}
	for _, c := range []string{"PAIN_DURING", "PAIN_NEXT_MORNING", "DESCENT_HIGH", "FLOORS_PREV_DAY"} {
		if !codes[c] {
			t.Errorf("alert %s expected, got %v", c, codes)
		}
	}
	if len(v["upcoming"].([]any)) != 1 || len(v["goals"].([]any)) != 1 {
		t.Errorf("upcoming/goals: %v %v", v["upcoming"], v["goals"])
	}
	calls := e.fx.calls.Load()
	var v2 m
	e.do(1, "GET", fmt.Sprintf("/api/activities/%d", runID), nil, &v2)
	if fr := v2["freshness"].(m); fr["source"] == "live" || e.fx.calls.Load() != calls {
		t.Errorf("second view must not call Garmin: %+v (+%d calls)", fr, e.fx.calls.Load()-calls)
	}

	// reports
	code, md := e.do(1, "GET", fmt.Sprintf("/api/activities/%d/report", runID), nil, nil)
	if code != 200 || !strings.HasPrefix(md, "> **DIRECTIVE SYSTÈME POUR L'IA :**") || !strings.Contains(md, "### BIOMÉCANIQUE & DÉRIVE DE FATIGUE") ||
		!strings.Contains(md, "Suivi Pathologie (Genou / Fémoro-patellaire)") || strings.Contains(md, "SFP") {
		t.Errorf("activity report: %d\n%s", code, md)
	}
	if _, md := e.do(1, "GET", fmt.Sprintf("/api/activities/%d/report?include=pain", runID), nil, nil); strings.Contains(md, "BIOMÉCANIQUE") || !strings.Contains(md, "ALERTES AUTOMATIQUES") {
		t.Errorf("include=pain report:\n%s", md)
	}
	if code, _ := e.do(1, "GET", fmt.Sprintf("/api/activities/%d/report?include=foo", runID), nil, nil); code != 400 {
		t.Errorf("bad chip: %d", code)
	}
	var day m
	if code, body := e.do(1, "GET", "/api/days/today", nil, &day); code != 200 || day["checkin"] == nil {
		t.Fatalf("day: %d %s", code, body)
	}
	if code, md := e.do(1, "GET", "/api/days/"+e.fx.day(-1)+"/report", nil, nil); code != 200 || !strings.Contains(md, "BILAN SANTÉ & CHARGE QUOTIDIENNE") {
		t.Errorf("day report: %d %s", code, md)
	}
	var multi m
	if code, body := e.do(1, "POST", "/api/exports/multi", m{"weeks": 2}, &multi); code != 200 || multi["sessions"].(float64) < 1 ||
		!strings.Contains(multi["markdown"].(string), "TENDANCES HEBDOMADAIRES") {
		t.Errorf("multi: %d %s", code, body)
	}
	var week m
	if code, _ := e.do(1, "GET", "/api/plan/week?date="+e.fx.day(-1), nil, &week); code != 200 || len(week["days"].([]any)) != 7 {
		t.Errorf("week: %d %+v", code, week)
	}

	// Strava: webhook validation, OAuth, fusion with the Garmin run
	if code, body := e.do(0, "GET", "/api/webhooks/strava?hub.mode=subscribe&hub.verify_token=verif&hub.challenge=abc", nil, nil); code != 200 || !strings.Contains(body, `"hub.challenge":"abc"`) {
		t.Errorf("webhook verify: %d %s", code, body)
	}
	if code, _ := e.do(0, "GET", "/api/webhooks/strava?hub.mode=subscribe&hub.verify_token=bad&hub.challenge=abc", nil, nil); code != 403 {
		t.Errorf("webhook bad token: %d", code)
	}
	code, loc := e.do(1, "GET", "/api/integrations/strava/connect?return=/settings", nil, nil)
	if code != 302 || !strings.Contains(loc, "state=") {
		t.Fatalf("connect: %d %s", code, loc)
	}
	state := loc[strings.Index(loc, "state=")+6 : strings.Index(loc, "&")]
	if code, loc := e.do(2, "GET", "/api/integrations/strava/callback?state="+state+"&code=c&scope=read,activity:read_all", nil, nil); code != 302 || !strings.Contains(loc, "strava=error") {
		t.Errorf("state bound to another user must fail: %s", loc)
	}
	code, loc = e.do(1, "GET", "/api/integrations/strava/connect?return=/settings", nil, nil)
	state = loc[strings.Index(loc, "state=")+6 : strings.Index(loc, "&")]
	if code, loc := e.do(1, "GET", "/api/integrations/strava/callback?state="+state+"&code=c&scope=read,activity:read_all", nil, nil); code != 302 || loc != "/settings?strava=connected" {
		t.Fatalf("callback: %d %s", code, loc)
	}
	e.svc.Wait()
	sp := 86.0
	e.fs.acts[777] = &strava.Activity{ID: 777, Name: "Footing du soir", Description: "Bonnes sensations", SportType: "Run",
		StartDate: e.fx.runStart.Add(45 * time.Second).UTC(), StartDateLocal: e.fx.runStart.Format("2006-01-02T15:04:05Z"),
		DistanceM: 5400, MovingTimeS: 1790, AverageCadenceRaw: &sp, SufferScore: &sp, PerceivedExertion: &sp, GearID: "g1"}
	if code, _ := e.do(0, "POST", "/api/webhooks/strava", m{"object_type": "activity", "object_id": 777, "aspect_type": "create", "owner_id": 4242}, nil); code != 200 {
		t.Fatalf("webhook event: %d", code)
	}
	e.svc.Wait()
	var fused m
	e.do(1, "GET", fmt.Sprintf("/api/activities/%d", runID), nil, &fused)
	if fused["sourceLabel"] != "Fusion" || fused["activity"].(m)["strava"] == nil || fused["load"].(m)["stravaRelativeEffort"] == nil {
		t.Errorf("fusion: %v %+v", fused["sourceLabel"], fused["activity"].(m)["strava"])
	}
	var shoes []m
	if code, _ := e.do(1, "POST", "/api/shoes/import-strava", nil, &shoes); code != 200 || len(shoes) != 2 {
		t.Errorf("import shoes: %d %+v", code, shoes)
	}

	// Strava-only user: degraded mode
	if _, err := e.svc.Integrations(ctx, 3); err != nil {
		t.Fatal(err)
	}
	code, loc = e.do(3, "GET", "/api/integrations/strava/connect", nil, nil)
	state = loc[strings.Index(loc, "state=")+6 : strings.Index(loc, "&")]
	e.fs.acts = map[int64]*strava.Activity{888: {ID: 888, Name: "Sortie", SportType: "Run", StartDate: e.fx.runStart.UTC().Add(-48 * time.Hour),
		StartDateLocal: e.fx.runStart.Add(-48 * time.Hour).Format("2006-01-02T15:04:05Z"), DistanceM: 4350, MovingTimeS: 1500, AverageCadenceRaw: &sp}}
	_, _ = e.st.DB.Exec(`UPDATE provider_connections SET external_id = NULL WHERE user_id = 1 AND provider = 'strava'`)
	if code, loc := e.do(3, "GET", "/api/integrations/strava/callback?state="+state+"&code=c&scope=read,activity:read_all", nil, nil); code != 302 || !strings.Contains(loc, "connected") {
		t.Fatalf("strava-only callback: %s", loc)
	}
	e.svc.Wait()
	var l3 struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	e.do(3, "GET", "/api/activities", nil, &l3)
	if len(l3.Items) != 1 {
		t.Fatalf("strava-only list: %+v", l3)
	}
	var sv m
	if code, body := e.do(3, "GET", fmt.Sprintf("/api/activities/%d", l3.Items[0].ID), nil, &sv); code != 200 {
		t.Fatalf("strava-only view: %d %s", code, body)
	}
	sb := sv["biomechanics"].(m)
	if sv["sourceLabel"] != "Strava" || sb["groundContactTimeMs"] != nil || sb["cadenceAvgSpm"].(float64) != 172 || sv["recovery"] != nil {
		t.Errorf("degraded view: %v gct=%v cad=%v", sv["sourceLabel"], sb["groundContactTimeMs"], sb["cadenceAvgSpm"])
	}
	if ws := fmt.Sprint(sv["warnings"]); !strings.Contains(ws, "Garmin non connecté") {
		t.Errorf("warnings: %s", ws)
	}

	// backup round trip, admin routes
	code, raw := e.do(1, "GET", "/api/backup", nil, nil)
	if code != 200 || strings.Contains(raw, `"id"`) || strings.Contains(raw, goodKey) {
		t.Fatalf("backup: %d", code)
	}
	var restored struct {
		Imported map[string]int `json:"imported"`
	}
	if code, body := e.do(1, "POST", "/api/backup/restore?mode=replace", raw, &restored); code != 200 || restored.Imported["notes"] != 1 ||
		restored.Imported["goals"] != 1 || restored.Imported["plan"] != 2 || restored.Imported["checkins"] != 2 {
		t.Errorf("restore: %d %s %+v", code, body, restored)
	}
	var notes2 m
	e.do(1, "PUT", fmt.Sprintf("/api/activities/%d/notes", runID), m{}, &notes2)
	if notes2["painDuring"].(float64) != 3.5 || notes2["shoe"] == nil {
		t.Errorf("notes after restore: %+v", notes2)
	}
	if code, _ := e.do(2, "GET", "/api/admin/connections", nil, nil); code != 403 {
		t.Errorf("admin route for a user: %d", code)
	}
	var conns []m
	if code, _ := e.do(1, "GET", "/api/admin/connections", nil, &conns); code != 200 || len(conns) < 2 {
		t.Errorf("admin connections: %d %+v", code, conns)
	}
	if code, _ := e.do(1, "POST", "/api/admin/users/99/resync", nil, nil); code != 404 {
		t.Errorf("resync unknown user: %d", code)
	}
	if code, _ := e.do(1, "POST", "/api/admin/users/2/resync", nil, nil); code != 202 {
		t.Errorf("resync: %d", code)
	}
	ids, err := e.svc.ConnectedUserIDs(ctx)
	if err != nil || len(ids) != 2 {
		t.Errorf("connected users: %v %v", ids, err)
	}
	if e.fs.subs.Load() != 1 {
		t.Errorf("webhook subscription ensured %d times", e.fs.subs.Load())
	}
	if code, _ := e.do(1, "DELETE", "/api/integrations/strava", nil, nil); code != 200 || e.fs.deauthed.Load() != 1 {
		t.Errorf("strava disconnect: %d", code)
	}
}
