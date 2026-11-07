// Package observability provides an isolated Prometheus registry and instrumentation
// helpers for HTTP, cache, worker queues, SSE, telemetry ingestion, and external APIs.
package observability

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HTTPDurationBuckets matches the Stride 4.0.0 metrics contract verbatim.
var HTTPDurationBuckets = []float64{0.002, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

// Metrics holds a dedicated Prometheus registry and all Stride 4.0.0 collectors.
// All helper methods are nil-safe so tests omitting Metrics do not panic.
type Metrics struct {
	Registry *prometheus.Registry

	HTTPRequestsTotal     *prometheus.CounterVec
	HTTPRequestDuration   *prometheus.HistogramVec
	HTTPRequestsInFlight  prometheus.Gauge
	SSEClientsConnected   prometheus.Gauge
	CacheOperationsTotal  *prometheus.CounterVec
	CacheOperationDur     *prometheus.HistogramVec
	WorkerJobsQueued      *prometheus.GaugeVec
	WorkerJobsActive      *prometheus.GaugeVec
	WorkerJobsProcessed   *prometheus.CounterVec
	WorkerJobDuration     *prometheus.HistogramVec
	TelemetryPointsTotal  *prometheus.CounterVec
	ActivitiesSyncedTotal *prometheus.CounterVec
	ExternalRequestsTotal *prometheus.CounterVec
	ExternalRequestDur    *prometheus.HistogramVec
	BackupLastSuccess     *prometheus.GaugeVec
	BackupLastSize        prometheus.Gauge
	BackupTotal           *prometheus.CounterVec
}

// New creates an isolated Prometheus registry with Go, Process, and all 14 stride_* collectors.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{
		Registry: reg,
		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "stride_http_requests_total",
			Help: "Total number of HTTP requests processed.",
		}, []string{"method", "route", "status"}),
		HTTPRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "stride_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: HTTPDurationBuckets,
		}, []string{"method", "route", "status"}),
		HTTPRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "stride_http_requests_in_flight",
			Help: "Current number of HTTP requests being served.",
		}),
		SSEClientsConnected: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "stride_sse_clients_connected",
			Help: "Current number of active Server-Sent Events stream connections.",
		}),
		CacheOperationsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "stride_cache_operations_total",
			Help: "Total cache operations by backend, operation, and result.",
		}, []string{"backend", "operation", "result"}),
		CacheOperationDur: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "stride_cache_operation_duration_seconds",
			Help:    "Cache operation duration in seconds.",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 1},
		}, []string{"backend", "operation"}),
		WorkerJobsQueued: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "stride_worker_jobs_queued",
			Help: "Current number of queued async jobs per priority queue.",
		}, []string{"queue"}),
		WorkerJobsActive: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "stride_worker_jobs_active",
			Help: "Current number of active async jobs per queue and job type.",
		}, []string{"queue", "job_type"}),
		WorkerJobsProcessed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "stride_worker_jobs_processed_total",
			Help: "Total async jobs processed by queue, job type, and terminal/retry status.",
		}, []string{"queue", "job_type", "status"}),
		WorkerJobDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "stride_worker_job_duration_seconds",
			Help:    "Async worker job execution duration in seconds.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
		}, []string{"queue", "job_type"}),
		TelemetryPointsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "stride_telemetry_points_ingested_total",
			Help: "Total telemetry data points ingested by source format.",
		}, []string{"format"}),
		ActivitiesSyncedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "stride_activities_synced_total",
			Help: "Total activities created or fused by source.",
		}, []string{"source"}),
		ExternalRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "stride_external_api_requests_total",
			Help: "Total outbound external API requests by provider and HTTP status class.",
		}, []string{"provider", "status_class"}),
		ExternalRequestDur: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "stride_external_api_duration_seconds",
			Help:    "Outbound external API request duration in seconds.",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
		}, []string{"provider"}),
		BackupLastSuccess: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "stride_backup_last_success_timestamp_seconds",
			Help: "Unix time of the last successful backup per target (local snapshot, S3 upload).",
		}, []string{"target"}),
		BackupLastSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "stride_backup_last_size_bytes",
			Help: "Size in bytes of the last successful local snapshot (gzip).",
		}),
		BackupTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "stride_backup_total",
			Help: "Backups attempted per target and outcome.",
		}, []string{"target", "status"}),
	}

	reg.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.HTTPRequestsInFlight,
		m.SSEClientsConnected,
		m.CacheOperationsTotal,
		m.CacheOperationDur,
		m.WorkerJobsQueued,
		m.WorkerJobsActive,
		m.WorkerJobsProcessed,
		m.WorkerJobDuration,
		m.TelemetryPointsTotal,
		m.ActivitiesSyncedTotal,
		m.ExternalRequestsTotal,
		m.ExternalRequestDur,
		m.BackupLastSuccess,
		m.BackupLastSize,
		m.BackupTotal,
	)

	// Pre-initialize baseline series so /metrics exposes core families immediately at boot.
	m.HTTPRequestsInFlight.Set(0)
	m.SSEClientsConnected.Set(0)
	for _, q := range []string{"critical", "default", "low"} {
		m.WorkerJobsQueued.WithLabelValues(q).Set(0)
	}
	for _, f := range []string{"fit", "gpx", "tcx", "strava_stream", "garmin_details"} {
		m.TelemetryPointsTotal.WithLabelValues(f).Add(0)
	}
	for _, src := range []string{"garmin", "strava", "fusion", "manual", "import"} {
		m.ActivitiesSyncedTotal.WithLabelValues(src).Add(0)
	}
	for _, target := range []string{"local", "s3"} {
		for _, status := range []string{"success", "error"} {
			m.BackupTotal.WithLabelValues(target, status).Add(0)
		}
	}

	return m
}

// Handler returns the Prometheus exposition HTTP handler for GET /metrics.
func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return promhttp.HandlerFor(prometheus.NewRegistry(), promhttp.HandlerOpts{})
	}
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

type metricsResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricsResponseWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *metricsResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *metricsResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *metricsResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func normalizeRoute(pattern, method string) string {
	p := strings.TrimSpace(pattern)
	if p == "" {
		return "unmatched"
	}
	if method != "" && strings.HasPrefix(p, method+" ") {
		p = strings.TrimSpace(strings.TrimPrefix(p, method+" "))
	}
	if p == "/api/" {
		return "unmatched"
	}
	return p
}

// HTTPMiddleware records stride_http_requests_total, stride_http_request_duration_seconds,
// and stride_http_requests_in_flight using Go 1.22+ ServeMux r.Pattern.
func (m *Metrics) HTTPMiddleware(next http.Handler) http.Handler {
	if m == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.HTTPRequestsInFlight.Inc()
		defer m.HTTPRequestsInFlight.Dec()

		start := time.Now()
		mw := &metricsResponseWriter{ResponseWriter: w}
		next.ServeHTTP(mw, r)
		status := mw.status
		if status == 0 {
			status = http.StatusOK
		}
		route := normalizeRoute(r.Pattern, r.Method)
		code := strconv.Itoa(status)
		m.HTTPRequestsTotal.WithLabelValues(r.Method, route, code).Inc()
		m.HTTPRequestDuration.WithLabelValues(r.Method, route, code).Observe(time.Since(start).Seconds())
	})
}

func (m *Metrics) ObserveCache(backend, operation, result string, dur time.Duration) {
	if m == nil {
		return
	}
	m.CacheOperationsTotal.WithLabelValues(backend, operation, result).Inc()
	m.CacheOperationDur.WithLabelValues(backend, operation).Observe(dur.Seconds())
}

func (m *Metrics) AddSSEClient(delta float64) {
	if m == nil {
		return
	}
	m.SSEClientsConnected.Add(delta)
}

func (m *Metrics) SetQueueDepth(queue string, depth float64) {
	if m == nil {
		return
	}
	m.WorkerJobsQueued.WithLabelValues(queue).Set(depth)
}

func (m *Metrics) AddQueueDepth(queue string, delta float64) {
	if m == nil {
		return
	}
	m.WorkerJobsQueued.WithLabelValues(queue).Add(delta)
}

func (m *Metrics) AddWorkerActive(queue, jobType string, delta float64) {
	if m == nil {
		return
	}
	m.WorkerJobsActive.WithLabelValues(queue, jobType).Add(delta)
}

func (m *Metrics) ObserveWorkerJob(queue, jobType, status string, dur time.Duration) {
	if m == nil {
		return
	}
	m.WorkerJobsProcessed.WithLabelValues(queue, jobType, status).Inc()
	m.WorkerJobDuration.WithLabelValues(queue, jobType).Observe(dur.Seconds())
}

func (m *Metrics) AddTelemetryPoints(format string, count int) {
	if m == nil || count <= 0 {
		return
	}
	m.TelemetryPointsTotal.WithLabelValues(format).Add(float64(count))
}

func (m *Metrics) IncActivitySynced(source string) {
	if m == nil {
		return
	}
	m.ActivitiesSyncedTotal.WithLabelValues(source).Inc()
}

// StatusClass classifies an outbound HTTP response into 2xx | 4xx | 429 | 5xx | network_error.
func StatusClass(status int, err error) string {
	switch {
	case status == http.StatusTooManyRequests:
		return "429"
	case status == 0 && err != nil:
		return "network_error"
	case status >= 200 && status < 400:
		return "2xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500:
		return "5xx"
	default:
		return "network_error"
	}
}

func (m *Metrics) ObserveExternalAPI(provider string, status int, err error, dur time.Duration) {
	if m == nil {
		return
	}
	m.ExternalRequestsTotal.WithLabelValues(provider, StatusClass(status, err)).Inc()
	m.ExternalRequestDur.WithLabelValues(provider).Observe(dur.Seconds())
}

// ObserveBackup counts a backup attempt; a success also records its time (and its size, local only).
func (m *Metrics) ObserveBackup(target, status string, size int64, at time.Time) {
	if m == nil {
		return
	}
	m.BackupTotal.WithLabelValues(target, status).Inc()
	if status != "success" {
		return
	}
	m.SetBackupLastSuccess(target, at)
	if target == "local" {
		m.SetBackupSize(size)
	}
}

// SetBackupLastSuccess records the last successful backup of a target (local | s3).
func (m *Metrics) SetBackupLastSuccess(target string, at time.Time) {
	if m == nil || at.IsZero() {
		return
	}
	m.BackupLastSuccess.WithLabelValues(target).Set(float64(at.Unix()))
}

func (m *Metrics) SetBackupSize(size int64) {
	if m == nil {
		return
	}
	m.BackupLastSize.Set(float64(size))
}
