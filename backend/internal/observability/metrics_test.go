package observability_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"stride/internal/observability"
)

func TestIsolatedRegistriesAndAllMetricsExposed(t *testing.T) {
	// Creating two independent Metrics instances must never panic on duplicate registration.
	m1 := observability.New()
	m2 := observability.New()
	if m1.Registry == m2.Registry {
		t.Fatal("expected distinct registries")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/activities/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	h := m1.HTTPMiddleware(mux)

	req := httptest.NewRequest("GET", "/api/activities/42", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	req404 := httptest.NewRequest("GET", "/unknown-path", nil)
	rec404 := httptest.NewRecorder()
	h.ServeHTTP(rec404, req404)

	m1.ObserveCache("memory", "get", "hit", time.Millisecond)
	m1.AddSSEClient(1)
	m1.SetQueueDepth("critical", 2)
	m1.AddWorkerActive("critical", "llm.analyze", 1)
	m1.ObserveWorkerJob("critical", "llm.analyze", "completed", 120*time.Millisecond)
	m1.AddTelemetryPoints("fit", 1800)
	m1.IncActivitySynced("garmin")
	m1.ObserveExternalAPI("garmin", 200, nil, 50*time.Millisecond)
	m1.ObserveExternalAPI("strava", 429, errors.New("rate limit"), 30*time.Millisecond)
	m1.ObserveExternalAPI("open-meteo", 0, errors.New("dial error"), 10*time.Millisecond)

	mrec := httptest.NewRecorder()
	m1.Handler().ServeHTTP(mrec, httptest.NewRequest("GET", "/metrics", nil))
	if mrec.Code != http.StatusOK {
		t.Fatalf("metrics status: %d", mrec.Code)
	}
	body := mrec.Body.String()

	required := []string{
		`stride_http_requests_total{method="GET",route="/api/activities/{id}",status="200"} 1`,
		`stride_http_requests_total{method="GET",route="unmatched",status="404"} 1`,
		`stride_http_request_duration_seconds_bucket{method="GET",route="/api/activities/{id}",status="200",le="0.002"}`,
		`stride_http_requests_in_flight 0`,
		`stride_sse_clients_connected 1`,
		`stride_cache_operations_total{backend="memory",operation="get",result="hit"} 1`,
		`stride_cache_operation_duration_seconds_count{backend="memory",operation="get"} 1`,
		`stride_worker_jobs_queued{queue="critical"} 2`,
		`stride_worker_jobs_active{job_type="llm.analyze",queue="critical"} 1`,
		`stride_worker_jobs_processed_total{job_type="llm.analyze",queue="critical",status="completed"} 1`,
		`stride_worker_job_duration_seconds_count{job_type="llm.analyze",queue="critical"} 1`,
		`stride_telemetry_points_ingested_total{format="fit"} 1800`,
		`stride_activities_synced_total{source="garmin"} 1`,
		`stride_external_api_requests_total{provider="garmin",status_class="2xx"} 1`,
		`stride_external_api_requests_total{provider="strava",status_class="429"} 1`,
		`stride_external_api_requests_total{provider="open-meteo",status_class="network_error"} 1`,
		`stride_external_api_duration_seconds_count{provider="garmin"} 1`,
	}
	for _, want := range required {
		if !strings.Contains(body, want) {
			t.Errorf("missing metric line %q in:\n%s", want, body)
		}
	}
}
