package analysis

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"stride/internal/config"
	"stride/internal/fitdata"
	"stride/internal/garmin"
)

type stubReporter struct{}

func (stubReporter) Activity(a *ActivityAnalysis, _ config.Profile) string {
	return "rapport " + string(a.Mode)
}
func (stubReporter) Day(d *DayAnalysis, _ config.Profile) string { return "jour " + d.Date }

// fakeGarminAPI sert des réponses synthétiques pour toutes les sources (aucune donnée réelle).
func fakeGarminAPI(t *testing.T) *httptest.Server {
	t.Helper()
	jwt := "e30." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, time.Now().Add(24*time.Hour).Unix()))) + ".x"
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, s string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, s)
	}
	mux.HandleFunc("POST /personalapitoken-service/personal-api/personal-api-key/token", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, jwt)
	})
	mux.HandleFunc("GET /activity-service/activity/77", func(w http.ResponseWriter, r *http.Request) {
		write(w, `{"activityId":77,"activityName":"Test","activityTypeDTO":{"typeKey":"running"},"summaryDTO":{"startTimeLocal":"2026-11-02T18:30:00.0","startTimeGMT":"2026-11-02T17:30:00.0","distance":1800,"duration":600,"elapsedDuration":600,"averageSpeed":3.0,"averageHR":135,"averageRunCadence":178,"averagePower":290,"elevationGain":3,"elevationLoss":45,"activityTrainingLoad":60,"trainingEffect":3.1}}`)
	})
	mux.HandleFunc("GET /download-service/files/activity/77", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // force le repli /details (après retries)
	})
	mux.HandleFunc("GET /activity-service/activity/77/details", func(w http.ResponseWriter, r *http.Request) {
		var rows []string
		start := time.Date(2026, 11, 2, 17, 30, 0, 0, time.UTC).UnixMilli()
		for i := 0; i < 700; i++ {
			hr := 130 + i*10/700
			rows = append(rows, fmt.Sprintf(`{"metrics":[%d,%d,178,3.0,290,%d]}`, start+int64(i)*1000, hr, i*3))
		}
		write(w, `{"metricDescriptors":[{"metricsIndex":0,"key":"directTimestamp"},{"metricsIndex":1,"key":"directHeartRate"},{"metricsIndex":2,"key":"directDoubleCadence"},{"metricsIndex":3,"key":"directSpeed"},{"metricsIndex":4,"key":"directPower"},{"metricsIndex":5,"key":"sumDistance"}],"activityDetailMetrics":[`+strings.Join(rows, ",")+`]}`)
	})
	mux.HandleFunc("GET /sleep-service/sleep/dailySleep", func(w http.ResponseWriter, r *http.Request) {
		write(w, `{"calendarDate":"`+r.URL.Query().Get("date")+`","sleepTimeSeconds":25000,"deepSleepSeconds":3000,"sleepEndTimestampGMT":1762069500000,"sleepScores":{"overall":{"value":55,"qualifierKey":"POOR"}}}`)
	})
	mux.HandleFunc("GET /sleep-service/sleep/dailySleepSummariesByDate", func(w http.ResponseWriter, r *http.Request) { write(w, `[]`) })
	mux.HandleFunc("GET /hrv-service/hrv/{date}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /hrv-service/hrv/daily/{a}/{b}", func(w http.ResponseWriter, r *http.Request) {
		write(w, `{"hrvSummaries":[{"calendarDate":"2026-11-01","lastNightAvg":50,"status":"BALANCED"}]}`)
	})
	mux.HandleFunc("GET /wellness-service/wellness/bodyBattery/reports/daily", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // source en échec → warning non bloquant
	})
	mux.HandleFunc("GET /wellness-service/wellness/dailyHeartRate", func(w http.ResponseWriter, r *http.Request) {
		write(w, `{"restingHeartRate":52,"lastSevenDaysAvgRestingHeartRate":46}`)
	})
	mux.HandleFunc("GET /usersummary-service/v2/dailysummaries", func(w http.ResponseWriter, r *http.Request) {
		write(w, `{"data":[{"calendarDate":"2026-11-02","movement":{"steps":{"goal":8000,"value":9000},"floorsAscended":{"goal":10,"value":25}},"stress":{"avgLevel":30}}]}`)
	})
	mux.HandleFunc("GET /usersummary-service/stats/steps/daily/{a}/{b}", func(w http.ResponseWriter, r *http.Request) {
		write(w, `[{"calendarDate":"2026-11-02","totalSteps":9000,"stepGoal":8000}]`)
	})
	mux.HandleFunc("GET /activitylist-service/activities/search/activities", func(w http.ResponseWriter, r *http.Request) {
		write(w, `[{"activityId":77,"startTimeLocal":"2026-11-02 18:30:00","activityType":{"typeKey":"running"},"activityTrainingLoad":60,"hrTimeInZone_1":10,"hrTimeInZone_2":100,"hrTimeInZone_3":400,"hrTimeInZone_4":80,"hrTimeInZone_5":10,"fastestSplit_1000":320},
		           {"activityId":76,"startTimeLocal":"2026-10-30 08:00:00","activityType":{"typeKey":"cycling"},"activityTrainingLoad":40,"distance":12000,"duration":1800}]`)
	})
	mux.HandleFunc("GET /biometric-service/heartRateZones/v2", func(w http.ResponseWriter, r *http.Request) {
		write(w, `[{"sport":"DEFAULT","zone1Floor":95,"zone2Floor":114,"zone3Floor":133,"zone4Floor":152,"zone5Floor":171}]`)
	})
	return httptest.NewServer(mux)
}

func newTestService(t *testing.T, url string) *Service {
	t.Helper()
	dir := t.TempDir()
	st, err := config.NewStore(dir, config.Config{EnvAPIKey: "cle-synthetique", Timezone: "Europe/Paris"})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := garmin.New(garmin.Options{BaseURL: url, DataDir: dir, Key: func() string { k, _ := st.APIKey(); return k }, Logger: log, Location: st.Location})
	return &Service{G: g, FIT: fitdata.NewStore(g, log), Settings: st, Reporter: stubReporter{}, Log: log}
}

func TestAnalyzeActivityPipeline(t *testing.T) {
	srv := fakeGarminAPI(t)
	defer srv.Close()
	svc := newTestService(t, srv.URL)
	a, err := svc.AnalyzeActivity(context.Background(), 77, ModePerformance, 7)
	if err != nil {
		t.Fatal(err)
	}
	has := func(xs []string, s string) bool {
		for _, x := range xs {
			if strings.Contains(x, s) {
				return true
			}
		}
		return false
	}
	if !has(a.DataSources, "details") || has(a.DataSources, "fit") || !has(a.Warnings, "Fichier FIT") || !has(a.Warnings, "Body Battery indisponible (HTTP 404)") {
		t.Errorf("sources=%v warnings=%v", a.DataSources, a.Warnings)
	}
	if has(a.Warnings, "VFC") {
		t.Errorf("un 204 ne doit pas produire d'avertissement : %v", a.Warnings)
	}
	if a.Activity.Date != "2026-11-02" || pf(a.Biomechanics.CadenceAvgSpm) != 178 || a.Load.Drift == nil || len(a.Series.TS) == 0 {
		t.Errorf("analyse incomplète : %+v", a.Biomechanics)
	}
	if len(a.Segments.Laps) == 0 || a.Segments.Laps[0].Label != "Km 1" {
		t.Errorf("laps synthétiques attendus : %+v", a.Segments.Laps)
	}
	if a.Performance == nil || len(a.Performance.BestEfforts) != 1 || a.Performance.ACWR == nil || len(a.Load.HrZonesS) != 5 || len(a.Performance.HrZoneFloorsBpm) != 5 {
		t.Errorf("performance : %+v", a.Performance)
	}
	if a.Recovery == nil || a.Recovery.Sleep == nil || pf(a.Recovery.RestingHR.Delta) != 6 || len(a.Recovery.Trend) != 7 {
		t.Errorf("récupération : %+v", a.Recovery)
	}
	if len(a.History) != 1 || a.History[0].TypeLabel != "Vélo" {
		t.Errorf("historique : %+v", a.History)
	}
	codes := map[string]bool{}
	for _, al := range a.Alerts {
		codes[al.Code] = true
	}
	for _, c := range []string{"DESCENT_HIGH", "RHR_HIGH", "SLEEP_LOW", "GCT_BALANCE_NA"} {
		if !codes[c] {
			t.Errorf("alerte %s attendue (%v)", c, codes)
		}
	}
	if a.Report != "rapport performance" || a.ReportTokensEstimate == 0 {
		t.Errorf("rapport : %q", a.Report)
	}
}

func TestAnalyzeDayPipeline(t *testing.T) {
	srv := fakeGarminAPI(t)
	defer srv.Close()
	svc := newTestService(t, srv.URL)
	d, err := svc.AnalyzeDay(context.Background(), "2026-11-02", 7)
	if err != nil {
		t.Fatal(err)
	}
	if pf(d.Day.Steps) != 9000 || pf(d.Day.FloorsUp) != 25 || len(d.Activities) != 1 || d.Activities[0].ID != 77 {
		t.Errorf("jour : %+v %+v", d.Day, d.Activities)
	}
	if len(d.History) != 1 || d.History[0].Date != "2026-10-30" {
		t.Errorf("historique jour : %+v", d.History)
	}
	found := false
	for _, a := range d.Alerts {
		if a.Code == "FLOORS_HIGH" {
			found = true
		}
	}
	if !found {
		t.Errorf("FLOORS_HIGH attendu : %+v", d.Alerts)
	}
	// clé absente → erreur NO_API_KEY propagée
	_ = svc.Settings.SetAPIKey("")
	svc2 := newTestService(t, srv.URL)
	_ = svc2.Settings.SetAPIKey("")
	svc2.G = garmin.New(garmin.Options{BaseURL: srv.URL, DataDir: t.TempDir(), Key: func() string { return "" }, Logger: svc2.Log})
	if _, err := svc2.AnalyzeDay(context.Background(), "2026-11-02", 0); err == nil || !strings.Contains(err.Error(), "clé") {
		t.Errorf("erreur clé attendue, obtenu %v", err)
	}
}
