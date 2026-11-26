# Stride — Architecture & Data Flows

Stride is a self-hosted, multi-user training platform: one statically linked Go binary serves the API and the embedded React PWA, with SQLite for durable data and Valkey for disposable data. The API contract is [SPEC.md](SPEC.md) plus `frontend/src/api/types.ts`.

![Architecture](architecture.svg)

## 1. Runtime topology (`compose.yml`)

| Component | Technology | Responsibility |
| --- | --- | --- |
| API + SPA | Go (`net/http`), embedded Vite bundle | passkey auth + one-time recovery codes, CSRF / CSP headers, REST JSON API, SSE `/api/events/stream`, public `/version`, `/share/:token` and per-user MCP `POST /mcp/{token}`, in-process jobs |
| Storage | SQLite (`modernc.org/sqlite`, WAL, `foreign_keys = ON`) | users, passkeys, sealed credentials, activity bundles, cycles, body map, share links and portal notes, job statuses |
| Backups | `internal/backup`, local volume + optional S3-compatible bucket | atomic snapshots (`<STRIDE_DATA_DIR>/backups`), SHA-256 sidecars, SigV4 uploads through the egress guard |
| Cache & Pub/Sub | Valkey 9 (`valkey-go`), optional | TTL cache (`stride:cache:*`, public share views), WebAuthn / OAuth ceremonies, SSE fan-out between instances |
| Metrics | Prometheus client | `GET /metrics` on `STRIDE_METRICS_ADDR` only, never on the public listener ([METRICS.md](METRICS.md)) |

Without `STRIDE_VALKEY_URL` an in-memory cache is used (single instance).

**SQLite pools (§15.3).** `store.Store.DB` is the write pool: one connection, `BEGIN IMMEDIATE`, so writes never race. `store.Store.R` is a `query_only` read pool (`max(4, GOMAXPROCS)` connections) used by plain reads (activity rows, search, share links and notes, public share lookups, exports, admin overview); WAL lets it read while the writer works. Both use `busy_timeout=5000`, `synchronous=NORMAL`, `cache_size=-64000`, `mmap_size=256 MiB`. A `Tx` callback uses its `*sql.Tx` only: a query on `DB` there waits forever for the connection the transaction holds — `internal/store/txguard_test.go` walks every `Tx` callback (and the receiver methods it calls) and fails on such an access.

## 2. Migrations (`internal/database`)

- `pressly/goose/v3`, embedded `.up.sql` / `.down.sql` pairs (`00001_platform` .. `00015_recovery_codes`), additive only; applied files are never edited. A table is rebuilt (rows copied as is) only when SQLite cannot alter it (CHECK change, column used by a foreign key in a down migration).
- Before pending migrations are applied to a non-empty database, boot takes a `pre_migration` snapshot (see §7); a failed snapshot stops the boot.
- 3.x databases: rows of the legacy `schema_migrations` table are bridged into `goose_db_version` before the newer migrations run.
- Boot asserts the expected tables and columns (fast) and logs orphan rows as warnings in the background; `stride doctor` runs the full `integrity_check` and `foreign_key_check`.

## 3. Jobs (`internal/queue`, in process)

| Group | Workers | Kinds |
| --- | --- | --- |
| `sync` | 2 | `sync.provider`, `sync.strava_webhook`, `sync.resync`, `sync.history`, `import.activity` |
| `llm` | 2 | `llm.analyze`, `llm.plan_generate`, `llm.plan_adapt`, `llm.morph_refine` |
| `webhook` | 4 | `webhook.deliver` (Discord / Telegram / JSON) |
| `backup` | 1 | `backup.run` (snapshot, then optional upload), `backup.upload` (4 attempts) |

- Inside a group: `critical` > `default` > `low`. Retries: 10 s, 1 min, 6 min, then 15 min. Webhooks retry only refused connections, 429 and 5xx.
- `async_jobs` keeps each state change (`queued` → `running` → `retrying` / `completed` / `failed`) and `processed` / `total`; errors are public French messages only. Unfinished jobs are failed at startup; shutdown drains running jobs within the grace period.
- Interactive endpoints wait at most 100 s, then answer 202 + `JobStatus` (or at once with `?async=1`).

## 4. Server-Sent Events (`internal/sse`)

One authenticated stream, outside gzip, write deadline cleared, `connected` event then a heartbeat every 20 s. Events: `job.updated`, `sync.updated`, `alert.triggered`, `llm.token` / `llm.done` (token streaming of the AI analyses; `llm.done` of kind `morph_refine` carries `morphPreview`), `share.updated` (`{kind, linkId}` after a portal write), `notification.created` / `notification.updated` (notification centre, §21: a new entry, then the new unread count), each delivered only to its user.

## 5. Domain engines

- **Cycles** (`/api/cycles`): plans → phases → weeks, planned vs actual from the plan matching and measured loads; imported phases join the active plan.
- **Health** (`/api/health/status`, `/api/bodymap`): the health status is the golden source of the profile focus; body entries are the single source of symptoms (notes project them); correlations use measured values only and report `insufficientData` below their sample thresholds.
- **History** (`POST /api/sync/history`): resumable backwards listing of the provider history (summaries only); `GET /api/search` covers the whole history.
- **Share** (`/api/share`, `/share/:token`): 256-bit tokens, SHA-256 stored, URL sealed (`share:<uid>:url`), per-link period / sections / anonymize, 5-minute cache by token hash, per-IP rate limit, tokens redacted in logs.
- **Physio / coach portal** (`/api/public/share/{token}/…`, §15.1): permissions `comments`, `planning`, `activityDetails` (off by default). Notes live in `share_notes` (one directive per link, one remark per link and session, upserts); prescribed sessions and events carry `share_link_id` + a label snapshot (`PlanSource` `share`) and stay editable by their link only. Unknown, revoked and expired tokens are the same 404; a permission that is off is `403 SHARE_FORBIDDEN`. A write drops the owner's cached public views and publishes `share.updated`.
- **Coach notes in the AI** (`report/coach.go`): every prompt (activity, day, multi, plan generation and adaptation, morph refinement) starts, right after the system directive, with « Consignes kiné / coach (prioritaires) »: active directives (link valid, `validUntil` not passed) and the remarks on the sessions of the prompt (plans: last 14 days). Third-party text is quoted in a fenced block, fences and headings neutralized, 600 characters per note, 12 notes at most.
- **Smart Workout Morphing** (`domain/morph.go`, §15.5): for today's or tomorrow's session, constraints come from active directive restrictions, pain ≥ 3/10 in the last 48 h (notes and body map), ACWR > 1.3, and the Open-Meteo forecast at the session slot (> 22 °C, wind > 30 km/h). Deterministic rules: intervals → easy continuous run (no-interval / easy-only directive, pain), volume −15/−20/−30 % (load, pain), paces slowed with heat, effort cues on wind, physio cadence in the steps. Refinement runs on the LLM pool; apply keeps the planned trace in `planned_sessions.morph_json` (revertible), the `.FIT` export encodes the current steps.
- **Notification centre** (`internal/domain/inbox.go`, §21): a per-user history fed by the analysis alerts (`emitAlert`) and the physio/coach portal writes, next to their toast, in the domain so it exists with no tab open. `inbox_entries` deduplicates on `(user_id, dedup_key)` — an edit updates the row and keeps its read state — stores an English title plus a stable `code` the interface localizes from, and is bounded by an admin retention (days, entries per user) purged on change and by the daily maintenance pass. `GET /api/inbox`, `POST /api/inbox/read` and `POST /api/inbox/read-all` are scoped to the caller; the header bell and its panel follow `notification.*` over SSE. Distinct from `/api/notifications`, which manages the external Discord / Telegram / webhook targets.
- **Garmin bridge** (`internal/garminbridge`, §23): the experimental workout push through Garmin Connect's *web* API, isolated behind five calls (`Login`, `LoginMFA`, `Status`, `Push`, `Disconnect`) and a `TokenStore`. No import of `domain`, `store` or `workout`: the domain builds the Garmin payload and picks a `WorkoutPublisher` (`personal`, the read-only API, or `web`, this gateway) depending on the switches and the stored tokens. Two switches (`STRIDE_GARMIN_BRIDGE` and `garminBridgeEnabled`), tokens sealed under `garmin_web:<uid>:token` in their own `provider_connections` row, a per-user lock, a breaker after repeated blocks, and the notification centre for expiry. The .FIT download stays the fallback, with the Garmin Connect Web import replacing the USB cable.
- **Release check** (`internal/updates`, §22): `GET /api/updates` (admin only) compares the newest stable GitHub release with `buildinfo.Version` through the egress client, with a SemVer comparison that ignores pre-releases and `dev` builds. One cached answer per instance (6 h on success, 1 h on failure) under a mutex; a failure never surfaces. When a release is newer, the route writes one `update` entry per admin and per version in the notification centre (dedup key `update:<version>`, read state preserved) so the banner disappears once read; the link is built from the constant repository and the validated tag. `updateCheckEnabled` turns the whole path off.
- **Per-user MCP** (`internal/mcp`, §16): `POST /mcp/{token}` over the official Go SDK (Streamable HTTP, stateless), one shared fr/en tool instance pair; the token is resolved from `mcp_access` (SHA-256) and the user is injected in the request context; the 13 tools call the same domain services as the REST API and are scoped to the resolved user. `plan_create` / `plan_update` take the structured workout as `steps` (SPEC §19.1): their input schema is built from the handler struct with the recursive `workout.Step` plugged in as a `$defs/step` reference (`steps.go`), because jsonschema-go cannot infer a cycle.

## 6. Security

Credentials, webhook URLs, bot tokens, share URLs and the S3 secret key are sealed with AES-256-GCM (`secret.Sealer`, AAD `<scope>:<uid>:<field>`, `backup:0:s3_secret_key`), never logged or returned. User ids come only from the session; any id received in a body or query must belong to the caller, otherwise 404.

Recovery codes (4.4, SPEC §17.1) are the last-resort way in for the passkey-only flow: a set of 10 one-time codes (10 random bytes each, base32, shown `XXXX-XXXX-XXXX-XXXX`), of which only the **SHA-256 of the normalized code** is stored (`recovery_codes`); the plaintext is displayed once and never recoverable. Each code is strictly single-use (atomic `UPDATE … WHERE used_at IS NULL`) and regeneration atomically voids the remaining set. The public login `POST /api/auth/login/recovery` is rate-limited per client IP (10 attempts/min → 429), answers 401 for unknown/used codes and 403 for a disabled account (code still consumed); the passkeys « last credential » 409 guard is intentionally unchanged.

Portal writes have no session: they require the CSRF header and the Stride origin (`Origin` / `Sec-Fetch-Site` against `STRIDE_PUBLIC_URL`), a 16 KB body without unknown fields, sanitized text (control and invisible characters dropped), and pass two limits (30 writes / 10 min per token, 60 per client IP). Logs and the audit trail keep only the token prefix (actions `share.public_*`, actor `share:<prefix>`).

MCP tokens are never stored in plaintext (SHA-256 in `mcp_access`; the plaintext is the one-time answer of the generate endpoint). The public route answers the same 404 for unknown, revoked or disabled tokens, rejects cross-origin requests (403, no CORS headers) and passes two in-memory limits (100 requests / 10 min per token, 200 per client IP). The uid of a tool call comes only from the resolved token (request context), never from the request body; logs and the audit trail keep only the token prefix (action `mcp.request`, actor `mcp:<prefix>`).

## 7. Backups (`internal/backup`)

- Snapshot: `VACUUM INTO` a temporary file through a dedicated connection (the writer stays free), gzip, then `stride-<UTC yyyymmddThhmmssZ>-<version>.sqlite.gz` (0600, directory 0700) and its JSON sidecar `{name, sizeBytes, sha256, createdAt, trigger, s3}`. The filesystem is the source of truth; names are matched strictly (no path, no traversal).
- Triggers: daily at 03:00 local (scheduler), before pending migrations at boot, manual from `/admin`. Local retention: 7 latest, 3 latest pre-migration.
- S3 (`backup/s3.go`): SigV4 client in the standard library (PUT, ListObjectsV2, DELETE, HEAD bucket; path-style or virtual-hosted), egress-guarded (private endpoints need `STRIDE_ALLOW_PRIVATE_EGRESS`). Uploads are `backup.upload` jobs (retries 10 s, 1 min, 6 min); after a success, remote snapshots older than `retentionDays` are deleted.
- Configuration in admin settings (seeded once from `STRIDE_S3_*`); metrics `stride_backup_*` ([METRICS.md](METRICS.md)).
