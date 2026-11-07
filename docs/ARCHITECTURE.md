# Stride — Architecture & Data Flows

Stride is a self-hosted, multi-user training platform: one statically linked Go binary serves the API and the embedded React PWA, with SQLite for durable data and Valkey for disposable data. The API contract is [SPEC.md](SPEC.md) plus `frontend/src/api/types.ts`.

![Architecture](architecture.svg)

## 1. Runtime topology (`compose.yml`)

| Component | Technology | Responsibility |
| --- | --- | --- |
| API + SPA | Go (`net/http`), embedded Vite bundle | passkey auth, CSRF / CSP headers, REST JSON API, SSE `/api/events/stream`, public `/version` and `/share/:token`, in-process jobs |
| Storage | SQLite (`modernc.org/sqlite`, WAL, `foreign_keys = ON`) | users, passkeys, sealed credentials, activity bundles, cycles, body map, share links, job statuses |
| Cache & Pub/Sub | Valkey 9 (`valkey-go`), optional | TTL cache (`stride:cache:*`, public share views), WebAuthn / OAuth ceremonies, SSE fan-out between instances |
| Metrics | Prometheus client | `GET /metrics` on `STRIDE_METRICS_ADDR` only, never on the public listener ([METRICS.md](METRICS.md)) |

Without `STRIDE_VALKEY_URL` an in-memory cache is used (single instance).

## 2. Migrations (`internal/database`)

- `pressly/goose/v3`, embedded `.up.sql` / `.down.sql` pairs (`00001_platform` .. `00007_v41`), additive only; applied files are never edited.
- 3.x databases: rows of the legacy `schema_migrations` table are bridged into `goose_db_version` before the newer migrations run.
- Boot asserts the expected tables and columns (fast) and logs orphan rows as warnings in the background; `stride doctor` runs the full `integrity_check` and `foreign_key_check`.

## 3. Jobs (`internal/queue`, in process)

| Group | Workers | Kinds |
| --- | --- | --- |
| `sync` | 2 | `sync.provider`, `sync.strava_webhook`, `sync.resync`, `sync.history`, `import.activity` |
| `llm` | 2 | `llm.analyze`, `llm.plan_generate`, `llm.plan_adapt` |
| `webhook` | 4 | `webhook.deliver` (Discord / Telegram / JSON) |

- Inside a group: `critical` > `default` > `low`. Retries: 10 s, 1 min, 6 min, then 15 min. Webhooks retry only refused connections, 429 and 5xx.
- `async_jobs` keeps each state change (`queued` → `running` → `retrying` / `completed` / `failed`) and `processed` / `total`; errors are public French messages only. Unfinished jobs are failed at startup; shutdown drains running jobs within the grace period.
- Interactive endpoints wait at most 100 s, then answer 202 + `JobStatus` (or at once with `?async=1`).

## 4. Server-Sent Events (`internal/sse`)

One authenticated stream, outside gzip, write deadline cleared, `connected` event then a heartbeat every 20 s. Events: `job.updated`, `sync.updated`, `alert.triggered`, `llm.token` / `llm.done` (token streaming of the AI analyses), each delivered only to its user.

## 5. Domain engines

- **Cycles** (`/api/cycles`): plans → phases → weeks, planned vs actual from the plan matching and measured loads; imported phases join the active plan.
- **Health** (`/api/health/status`, `/api/bodymap`): the health status is the golden source of the profile focus; body entries are the single source of symptoms (notes project them); correlations use measured values only and report `insufficientData` below their sample thresholds.
- **History** (`POST /api/sync/history`): resumable backwards listing of the provider history (summaries only); `GET /api/search` covers the whole history.
- **Share** (`/api/share`, `/share/:token`): 256-bit tokens, SHA-256 stored, URL sealed (`share:<uid>:url`), per-link period / sections / anonymize, 5-minute cache by token hash, per-IP rate limit, tokens redacted in logs.

## 6. Security

Credentials, webhook URLs, bot tokens and share URLs are sealed with AES-256-GCM (`secret.Sealer`, AAD `<scope>:<uid>:<field>`), never logged or returned. User ids come only from the session; any id received in a body or query must belong to the caller, otherwise 404.
