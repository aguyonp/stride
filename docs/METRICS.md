# Stride — Prometheus metrics

Stride serves `GET /metrics` (Prometheus text format, isolated registry in `backend/internal/observability`) on a dedicated listener enabled by `STRIDE_METRICS_ADDR` (e.g. `:9090`); it is never exposed on the public listener and compose does not publish it.

Every metric uses bounded, low-cardinality labels (HTTP routes are normalized via Go `http.Request.Pattern`, e.g. `GET /api/activities/{id}`, falling back to `unmatched` on unknown paths) so time-series cardinality stays deterministic regardless of traffic or user count.

---

## 1. Scraping & Deployment

```yaml
scrape_configs:
  - job_name: stride
    scrape_interval: 15s
    metrics_path: /metrics
    static_configs:
      - targets: ["stride:9090"] # STRIDE_METRICS_ADDR=:9090, same Docker network as Prometheus
```

---

## 2. Application Metrics Reference

### 2.1 HTTP & Real-Time SSE Layer

| Metric | Type | Labels | Buckets / Unit | Description |
| --- | --- | --- | --- | --- |
| `stride_http_requests_total` | Counter | `method`, `route`, `status` | requests | Total completed HTTP requests partitioned by HTTP method, normalized mux pattern (`GET /api/activities/{id}`, `unmatched`), and numeric status code (`200`, `401`, `500`, …). |
| `stride_http_request_duration_seconds` | Histogram | `method`, `route`, `status` | `0.002, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30` (s) | End-to-end HTTP handler latency in seconds (excludes long-lived `/api/events/stream` hold duration). |
| `stride_http_requests_in_flight` | Gauge | — | requests | Number of HTTP requests currently executing across the server. |
| `stride_sse_clients_connected` | Gauge | — | connections | Active authenticated Server-Sent Events (`/api/events/stream`) client streams. |

### 2.2 Valkey / In-Memory Cache Layer

| Metric | Type | Labels | Buckets / Unit | Description |
| --- | --- | --- | --- | --- |
| `stride_cache_operations_total` | Counter | `backend`, `operation`, `result` | operations | Cache operations executed. `backend` ∈ `valkey\|memory`; `operation` ∈ `get\|set\|del\|del_prefix`; `result` ∈ `hit\|miss\|ok\|error`. |
| `stride_cache_operation_duration_seconds` | Histogram | `backend`, `operation` | `0.0001, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 1` (s) | Round-trip latency of cache operations against Valkey or the in-memory store. |

### 2.3 In-process jobs

| Metric | Type | Labels | Buckets / Unit | Description |
| --- | --- | --- | --- | --- |
| `stride_worker_jobs_queued` | Gauge | `queue` | jobs | Jobs waiting for a worker or a retry, by priority (`queue` ∈ `critical\|default\|low`; workers are pooled per group: sync, llm, webhook, backup). |
| `stride_worker_jobs_active` | Gauge | `queue`, `job_type` | jobs | Jobs currently executing in worker goroutines (`job_type` ∈ `sync.provider\|sync.strava_webhook\|sync.resync\|sync.history\|import.activity\|llm.analyze\|llm.plan_generate\|llm.plan_adapt\|llm.morph_refine\|webhook.deliver\|backup.run\|backup.upload`). |
| `stride_worker_jobs_processed_total` | Counter | `queue`, `job_type`, `status` | jobs | Total worker executions by outcome (`status` ∈ `completed\|retried\|failed`). |
| `stride_worker_job_duration_seconds` | Histogram | `queue`, `job_type` | `0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120` (s) | Execution duration of a single worker attempt in seconds. |

### 2.4 Sports Telemetry & Activity Ingestion

| Metric | Type | Labels | Unit | Description |
| --- | --- | --- | --- | --- |
| `stride_telemetry_points_ingested_total` | Counter | `format` | points | Individual GPS / biometric time-series samples parsed and ingested (`format` ∈ `fit\|gpx\|tcx` for rescue imports, `strava_stream`). |
| `stride_activities_synced_total` | Counter | `source` | activities | Activities persisted or fused into SQLite (`source` ∈ `garmin\|strava\|fusion\|import`). |

### 2.5 Outbound Third-Party APIs

| Metric | Type | Labels | Buckets / Unit | Description |
| --- | --- | --- | --- | --- |
| `stride_external_api_requests_total` | Counter | `provider`, `status_class` | requests | Outbound HTTP requests to external providers (`provider` ∈ `garmin\|strava\|open-meteo\|llm\|webhook`; `status_class` ∈ `2xx\|4xx\|429\|5xx\|network_error`). |
| `stride_external_api_duration_seconds` | Histogram | `provider` | `0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120` (s) | End-to-end outbound call duration by external provider. |

### 2.6 Backups (4.2.0)

| Metric | Type | Labels | Unit | Description |
| --- | --- | --- | --- | --- |
| `stride_backup_last_success_timestamp_seconds` | Gauge | `target` | Unix seconds | Time of the last successful backup per `target` ∈ `local\|s3` (local snapshot written, S3 upload acknowledged). Restored from the sidecars at boot; absent until a first success. |
| `stride_backup_last_size_bytes` | Gauge | — | bytes | Size of the last successful local snapshot (gzip). |
| `stride_backup_total` | Counter | `target`, `status` | backups | Backup attempts per `target` ∈ `local\|s3` and `status` ∈ `success\|error` (each S3 retry counts). |

### 2.7 Go Runtime & OS Process Collectors

Stride also registers standard Prometheus collectors on the same registry:
- `go_goroutines`, `go_memstats_alloc_bytes`, `go_gc_duration_seconds`, `go_threads`
- `process_cpu_seconds_total`, `process_resident_memory_bytes`, `process_open_fds`, `process_max_fds`

---

## 3. Essential PromQL Queries

### HTTP Latency Percentiles (P50 / P95 / P99 by Route)

```promql
# P50 latency per route (5m window)
histogram_quantile(0.50, sum by (le, route) (rate(stride_http_request_duration_seconds_bucket[5m])))

# P95 latency per route (5m window)
histogram_quantile(0.95, sum by (le, route) (rate(stride_http_request_duration_seconds_bucket[5m])))

# P99 latency across all API endpoints (5m window)
histogram_quantile(0.99, sum by (le) (rate(stride_http_request_duration_seconds_bucket{route=~".* /api/.*"}[5m])))
```

### Valkey Cache Hit Ratio & P95 Latency

```promql
# Cache hit ratio (0.0 -> 1.0)
sum(rate(stride_cache_operations_total{operation="get", result="hit"}[5m]))
/
clamp_min(sum(rate(stride_cache_operations_total{operation="get", result=~"hit|miss"}[5m])), 1e-9)

# Valkey P95 operation latency
histogram_quantile(0.95, sum by (le, operation) (rate(stride_cache_operation_duration_seconds_bucket{backend="valkey"}[5m])))
```

### Async Worker Throughput, Retries & Queue Backlog

```promql
# Pending jobs by priority queue
sum by (queue) (stride_worker_jobs_queued)

# Worker terminal failure ratio over 15m
sum(rate(stride_worker_jobs_processed_total{status="failed"}[15m]))
/
clamp_min(sum(rate(stride_worker_jobs_processed_total[15m])), 1e-9)
```

### Telemetry Point Ingestion Rate (points/sec)

```promql
sum by (format) (rate(stride_telemetry_points_ingested_total[5m]))
```

### Third-Party API Error & Rate-Limit (`429`) Ratios

```promql
# Upstream failure + rate-limit rate per provider
sum by (provider, status_class) (rate(stride_external_api_requests_total{status_class=~"429|5xx|network_error"}[5m]))
```

---

## 4. Recommended Alert Thresholds

| Alert Name | Condition (PromQL) | For | Severity | Operational Meaning |
| --- | --- | --- | --- | --- |
| `StrideHighHttp5xxRate` | `sum(rate(stride_http_requests_total{status=~"5.."}[5m])) / clamp_min(sum(rate(stride_http_requests_total[5m])), 1e-9) > 0.02` | `5m` | `critical` | More than 2 % of HTTP requests are failing with server errors; check SQLite WAL lock contention or disk space. |
| `StrideSlowApiP95` | `histogram_quantile(0.95, sum by (le) (rate(stride_http_request_duration_seconds_bucket{route=~".* /api/.*"}[5m]))) > 0.75` | `10m` | `warning` | P95 API latency exceeds 750 ms despite heavy work being offloaded to the async worker. |
| `StrideCacheBackendErrors` | `sum(rate(stride_cache_operations_total{result="error"}[5m])) > 0` | `2m` | `critical` | Valkey connection errors detected; verify `valkey` container health and memory limits. |
| `StrideCriticalQueueBacklog` | `stride_worker_jobs_queued{queue="critical"} > 10` | `3m` | `warning` | Interactive `.FIT` imports or LLM streaming jobs are queuing behind active tasks. |
| `StrideWorkerJobFailures` | `increase(stride_worker_jobs_processed_total{status="failed"}[15m]) >= 3` | `0m` | `warning` | Background jobs exhausted exponential backoff retries and entered terminal `failed` state. |
| `StrideProviderRateLimited` | `increase(stride_external_api_requests_total{status_class="429"}[15m]) >= 1` | `0m` | `info` | Strava, Garmin, or LLM provider returned HTTP 429; Stride enters automatic quota cooldown. |
| `StrideProviderUpstreamDown` | `sum by (provider) (rate(stride_external_api_requests_total{status_class=~"5xx|network_error"}[10m])) / clamp_min(sum by (provider) (rate(stride_external_api_requests_total[10m])), 1e-9) > 0.25` | `10m` | `warning` | External provider error rate exceeds 25 % over 10 minutes. |
| `StrideBackupStale` | `time() - stride_backup_last_success_timestamp_seconds{target="local"} > 26 * 3600` | `30m` | `critical` | No local snapshot for more than 26 hours: the daily backup (03:00) failed or the data volume is full. |
| `StrideBackupS3Stale` | `time() - stride_backup_last_success_timestamp_seconds{target="s3"} > 26 * 3600` | `1h` | `warning` | Auto-upload is on but no snapshot reached S3 for more than 26 hours (credentials, endpoint or egress). |
| `StrideBackupErrors` | `increase(stride_backup_total{status="error"}[1h]) >= 3` | `0m` | `warning` | Repeated snapshot or upload failures (the upload job retries 10 s, 1 min, 6 min). |
