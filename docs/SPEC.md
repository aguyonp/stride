# Stride — Specification

Stride extracts sports telemetry (Garmin Connect Personal API, Strava), computes biomechanics / load / recovery insights, tracks plan, goals and pain, and generates compact Markdown prompts for an AI coach. Multi-user service.

**Non-negotiables**
- No product metric may be dropped. When a source cannot provide a value, return `null` and a human reason; never fabricate.
- Every generated prompt (single session, day/health, multi-session) starts with the objectivity directive (§6.1), verbatim.
- Strict per-user isolation: every domain query filters on `user_id` taken from the authenticated context, never from the request.
- Secrets (Garmin personal API keys, Garmin JWTs, Strava access/refresh tokens) are encrypted at rest with `secret.Sealer` (AES-256-GCM, `STRIDE_MASTER_KEY`), AAD = `"<provider>:<userID>:<field>"`. Never logged, never returned (only hints like `••••a1b2` are allowed in API responses).
- Code comments: very few, English only. UI text: French. Logs: emoji + structured fields.

Verified Garmin Personal API endpoints, payload shapes and FIT conversions: [GARMIN_API.md](GARMIN_API.md).

---

## 1. Verified provider facts (2026-11-04 probe)

Garmin Personal API (`https://personal.connectapi.garmin.com`), in addition to [GARMIN_API.md](GARMIN_API.md):
- **404 (not exposed):** training readiness (`/metrics-service/metrics/trainingreadiness/*`), training status / acute-chronic load (`/metrics-service/metrics/trainingstatus/*`), max metrics, gear (`/gear-service/*`), weather (`/weather-service/*`, `/activity-service/activity/{id}/weather`), devices.
- **200:** `/usersummary-service/usersummary/daily?calendarDate=YYYY-MM-DD` → flat object incl. `totalSteps, dailyStepGoal, floorsAscended, floorsDescended, averageStressLevel, maxStressLevel, restingHeartRate, lastSevenDaysAvgRestingHeartRate, bodyBatteryAtWakeTime, bodyBatteryHighestValue, bodyBatteryLowestValue, bodyBatteryChargedValue, bodyBatteryDrainedValue, moderateIntensityMinutes, vigorousIntensityMinutes, activeKilocalories, sleepingSeconds`.
- Activity `summaryDTO` exposes Stamina: `beginPotentialStamina`, `endPotentialStamina`, `minAvailableStamina`; GAP: `avgGradeAdjustedSpeed`; `impactLoad`. Details time series expose `directGradeAdjustedSpeed`, `directAvailableStamina`, `directPotentialStamina`, `directElevation`, `directVerticalSpeed`.
- FIT files contain **no** `weather_conditions` messages and an invalid `avg_temperature`.

Consequences:
- **Training Readiness**: not available from Garmin → manual daily check-in (score /100 + status) shown with `source: "manual"`; otherwise `null` + reason "Non exposé par l'API personnelle Garmin".
- **Acute / chronic load**: computed from `activityTrainingLoad` of all activities (acute = sum over last 7 days, chronic = sum over last 28 days / 4, ratio = acute / chronic, optimal 0.8–1.3), flagged `source: "computed"`.
- **Weather**: Open-Meteo (no key) from activity start lat/lon rounded to 2 decimals + start time; Strava/Garmin do not provide it.
- **Shoes**: Strava gear when connected, otherwise the local shoe list.

Strava API v3: OAuth2 (`https://www.strava.com/oauth/authorize`, `https://www.strava.com/oauth/token`, scopes `read,activity:read_all,profile:read_all`), access token 6 h + refresh token. `DetailedActivity` gives `name`, `description`, `private_note`, `perceived_exertion`, `suffer_score` (Relative Effort), `gear_id`/`gear`, `start_latlng`. Streams endpoint with `keys=time,distance,heartrate,cadence,watts,altitude,velocity_smooth,grade_smooth,moving,latlng,temp&key_by_type=true`. Run cadence is one-leg rpm → spm = ×2. Rate limits are app-wide (read: ~100 req/15 min, ~1000/day; trust `X-RateLimit-Limit` / `X-RateLimit-Usage` headers). Webhooks: one subscription per app; GET validation echoes `hub.challenge` when `hub.verify_token` matches; POST events must be acked < 2 s (process async).

---

## 2. Architecture

```
compose.yml
 ├─ stride   Go binary (API + embedded React SPA), non-root distroless, /data volume (SQLite WAL)
 └─ valkey   in-memory cache, no persistence, maxmemory + allkeys-lru, every key has a TTL
```

- SQLite (`modernc.org/sqlite`, pure Go, WAL, `foreign_keys=1`) holds users, credentials, sessions, invitations, provider connections, **immutable activity bundles**, final wellness days, and all manual athlete data. Migrations are embedded and applied at startup.
- Valkey holds only disposable data with TTL: provider list responses, today's wellness, computed analyses, public share views, WebAuthn ceremonies, OAuth states, sync locks; it also relays SSE events between instances (Pub/Sub). Jobs run in process. `POST /api/admin/cache/flush` deletes `stride:cache:*` only (sessions live in SQLite, so nobody is logged out).
- Without `STRIDE_VALKEY_URL` (local dev), the in-memory cache is used.

### 2.1 Backend packages

`cmd/stride` (serve, healthcheck, version, doctor) · `env` · `store` (SQLite) · `database` (goose migrations, schema check) · `queue` (in-process jobs) · `sse` · `observability` (Prometheus) · `cache` · `secret` (AES-GCM) · `auth` (passkeys, sessions) · `admin` · `scheduler` · `apimeter` · `mcp` (per-user MCP server, §16) · `ratelimit` (in-memory limits) · `httpapi` / `httpx` / `web` (router, middlewares, embedded SPA) · `domain` / `domainapi` (sync, views, athlete data, HTTP) · `garmin`, `provider/strava`, `provider/weather` · `fitdata` · `analysis` · `athlete` · `view` · `report` · `frfmt` · `logx`.

---

## 3. HTTP API

JSON camelCase. Errors: `{"error": "message FR lisible", "code": "<httpx.Code*>"}`. Dates `YYYY-MM-DD`, timestamps RFC3339. Every authenticated route returns 401 `UNAUTHORIZED` without a valid session; admin routes 403 `FORBIDDEN` for non-admins. Session = HttpOnly cookie `stride_session` (SameSite=Lax, Secure when `STRIDE_PUBLIC_URL` is https). State-changing requests must carry header `X-Stride-CSRF: 1` (simple CSRF guard; the SPA always sends it).

### 3.1 Platform

| Method & path | Body → Response |
|---|---|
| `GET /healthz` | liveness → `{"status":"ok"}` (moved from `/health`, which is the SPA « Santé & forme » route and would otherwise be masked on refresh) |
| `GET /health/ready` | DB + cache ping → 200 `{"status":"ready","checks":{"db":"ok","cache":"ok"}}` or 503 with failing check message |
| `GET /api/auth/state` | → `AuthState` |
| `POST /api/auth/register/begin` | `{displayName, inviteCode?}` → `{publicKey: PublicKeyCredentialCreationOptionsJSON}` (resident key required, user verification preferred) |
| `POST /api/auth/register/finish` | RegistrationResponseJSON (as produced by `@simplewebauthn/browser` `startRegistration`) → `Me` + session cookie |
| `POST /api/auth/login/begin` | `{}` → `{publicKey: PublicKeyCredentialRequestOptionsJSON}` (discoverable credentials, empty allowCredentials) |
| `POST /api/auth/login/finish` | AuthenticationResponseJSON → `Me` + session cookie |
| `POST /api/auth/login/recovery` | `{code}` → `Me` + session cookie (4.4, public; unknown/used code 401, disabled 403, 10 attempts/min/IP 429) |
| `POST /api/auth/logout` | → 204 |
| `GET /api/me` · `PATCH /api/me` | `{displayName?, onboardingCompleted?}` → `Me` |
| `DELETE /api/me` | delete own account (cascade) → 204 |
| `GET /api/me/passkeys` | → `Passkey[]` |
| `POST /api/me/passkeys/begin` · `/finish` | add a passkey while logged in |
| `PATCH /api/me/passkeys/{id}` · `DELETE /api/me/passkeys/{id}` | rename `{name}` · delete (refuse last one: 409) |
| `GET /api/me/recovery` | → `RecoveryStatus` (4.4: how many one-time codes remain) |
| `POST /api/me/recovery` | → `{ codes: string[] }` (4.4: fresh set of 10, previous ones voided; shown once) |
| `GET /api/mcp` | → `McpStatus` (§16) |
| `PUT /api/mcp` | `{enabled: boolean}` → 204 (disabling keeps the token) |
| `POST /api/mcp/token` | → `{token, prefix}` (plaintext shown once; replacing revokes the previous token) |
| `DELETE /api/mcp/token` | → 204 |
| `GET /api/admin/users` | → `AdminUser[]` |
| `PATCH /api/admin/users/{id}` | `{status?: "active"\|"disabled", role?: "user"\|"admin"}` (cannot change self) → `AdminUser`; disabling revokes sessions |
| `DELETE /api/admin/users/{id}` | → 204 (not self) |
| `GET /api/admin/invitations` · `POST` · `DELETE /{id}` | create `{label?, maxUses? (1), expiresInHours? (168)}` → `{invitation: Invitation, code, link}` (code shown once; link = `${PUBLIC_URL}/register?invite=CODE`) |
| `GET /api/admin/settings` · `PUT` | `{registrationMode: "open"\|"invite"}` |
| `GET /api/admin/meter` | → `apimeter.Snapshot` |
| `POST /api/admin/cache/flush` | → `{removed: number}` |

```ts
interface AuthState { authenticated: boolean; user: Me | null; registrationMode: "open" | "invite"; firstUser: boolean; rpId: string }
interface Me { id: number; displayName: string; role: "user" | "admin"; createdAt: string; onboardingCompleted: boolean }
interface Passkey { id: string; name: string; createdAt: string; lastUsedAt: string | null; backedUp: boolean }
interface RecoveryStatus { remaining: number }
interface McpStatus { enabled: boolean; hasToken: boolean; tokenPrefix: string; createdAt: string | null; lastUsedAt: string | null; quota: McpQuota (4.5.0, §16.4) }
interface McpQuota { tokenLimit: number; tokenWindow: number; ipLimit: number; ipWindow: number } // windows in minutes
interface AdminUser { id: number; displayName: string; role: "user" | "admin"; status: "active" | "disabled"; createdAt: string; lastLoginAt: string | null; passkeys: number }
interface Invitation { id: number; label: string; codePrefix: string; createdAt: string; expiresAt: string | null; maxUses: number; uses: number; revoked: boolean }
```
Rules: the very first registered user needs no invitation and becomes `admin`. Afterwards `registrationMode` (DB setting, initial value from `STRIDE_REGISTRATION`, default `invite`) applies. Disabled users cannot log in.

#### 3.1.1 Implementation notes
- WebAuthn `begin` endpoints set an HttpOnly cookie `stride_webauthn` (path `/api/`, 5 min) bound to the ceremony stored in the cache; `finish` must be called from the same browser. `register/finish`, `login/finish` and `passkeys/finish` bodies are the raw `@simplewebauthn/browser` JSON. Verification failures → 400 `BAD_REQUEST`; unknown passkey at login → 401; disabled account → 403 `FORBIDDEN`; expired/replayed ceremony → 400.
- Missing/invalid invitation → 403 `INVITE_REQUIRED`. Invitation codes look like `ABCD-EFGH-…` (24 base32 chars), case and dashes are ignored; `codePrefix` = first 4 chars. `DELETE /api/admin/invitations/{id}` revokes (soft, `revoked: true`) → 204. `expiresInHours: 0` = no expiry; `maxUses` 1–1000.
- `POST /api/me/passkeys/begin` accepts an optional `{name}`; `/finish` and `PATCH /{id}` return a `Passkey`. `Passkey.id` is the base64url credential id. Default names come from the user agent (« Chrome · macOS »).
- **Recovery codes (4.4)**: one-time login codes for the passkey-only flow — losing every device with unbacked passkeys must not mean losing the account. `POST /api/me/recovery` generates a set of 10 codes: 10 random bytes each, base32 (RFC 4648) without padding, 16 uppercase chars displayed `XXXX-XXXX-XXXX-XXXX`; on input, case and dashes are normalized. Only the **SHA-256** of the normalized code is stored (`recovery_codes`, migration `00015`, `ON DELETE CASCADE` on `users`); the plaintext is returned once and never recoverable. Each code is strictly single-use (atomic `UPDATE … SET used_at = ? WHERE code_hash = ? AND used_at IS NULL`, affected-rows guard → 401 `UNAUTHORIZED` for unknown/used/malformed codes). `POST` regenerates atomically (old set voided, new set stored in one transaction). `POST /api/auth/login/recovery` is public, session-issued like a passkey login, disabled account → 403 (the code is still consumed), rate-limited per client IP (10 attempts/min, 429 `RATE_LIMITED`). The passkeys « last credential » 409 guard is untouched by design.
- `PATCH` / `DELETE /api/admin/users/{id}` on oneself → 409 `CONFLICT`. `DELETE /api/me` by the only active admin while other users exist → 409.
- `register/finish` → 200 `Me`; `/healthz` also returns `version`; `/health/ready` 503 body `{"status":"unavailable","checks":{…}}`.
- CSRF: any non-GET `/api/*` call without `X-Stride-CSRF: 1` → 403 `FORBIDDEN` (except `POST /api/webhooks/strava`).
- CSP is `default-src 'self'` (inline styles allowed, **no inline scripts**, no external hosts): the SPA must not rely on inline `<script>`.
- `RequireUser` injects both `auth.User` and `apimeter.WithUser(ctx, id)`.
- Scheduler: users = `domain.ConnectedUserIDs` (active users with a connected provider); the `LastSyncAt` hook makes lazy syncs count for the 3 h skip rule.
- API meter: when a provider has a per-user limit (Garmin), its global limit counts **users** fetching concurrently (Garmin: 4 users × 6 calls each); Strava / Open-Meteo global limits count calls. 404 is not counted as an error. Errors and 429s are persisted to `api_events` (7 days) and reloaded at startup.
- Strava client: `StartDateLocal` drops Strava's trailing `Z`; `Timezone` is the IANA name; quota uses the tighter of `X-RateLimit-*` / `X-ReadRateLimit-*`; on 429 without `Retry-After` the cooldown lasts until the next quarter hour (or UTC midnight when the daily quota is exhausted); GETs retried twice on 5xx/network errors; `strava.APIError` + `strava.IsNotFound` added; token endpoint 400 → `*ErrUnauthorized`.
- Weather: forecast API for start times < 30 days old, archive API otherwise; `weather.Describe(code)`, `weather.WindDirLabel(deg)` (FR 8 points) and `weather.Round2` are available to the domain; `ErrNoData` / `ErrInvalidCoordinate`.

### 3.2 Domain

| Method & path | Body → Response |
|---|---|
| `GET /api/integrations` | → `Integrations` |
| `PUT /api/integrations/garmin` | `{apiKey}` → validates by token exchange (400 `PROVIDER_UNAUTHORIZED` if refused), stores encrypted → `Integrations` |
| `POST /api/integrations/garmin/test` · `DELETE /api/integrations/garmin` | |
| `GET /api/integrations/strava/connect?return=/path` | 302 → Strava authorize (state in cache 10 min, bound to user) |
| `GET /api/integrations/strava/callback` | 302 → `return` + `?strava=connected` or `?strava=error&reason=…` |
| `DELETE /api/integrations/strava` | deauthorize + delete tokens |
| `GET /api/webhooks/strava` · `POST` | public; validation + events (ack immediately, process async: fetch Strava activity, match/fuse with Garmin, persist) |
| `POST /api/sync` | lazy sync (non-blocking) → `SyncStatus` |
| `GET /api/sync/status` | → `SyncStatus` |
| `GET /api/activities?limit=30&before=<startLocal>&from=&to=&sport=` | → `{items: ActivitySummary[], hasMore, freshness: Freshness}` |
| `GET /api/activities/{id}` | `{id}` = Stride activity id → `ActivityView` |
| `GET /api/activities/{id}/report?include=…` | → `text/markdown; charset=utf-8` (§6) |
| `PUT /api/activities/{id}/notes` | `ActivityNotesInput` → `ActivityNotes` |
| `GET /api/days/{date}` | `date` or `today` → `DayView` |
| `GET /api/days/{date}/report?include=…` | → markdown (§6.3) |
| `PUT /api/days/{date}/checkin` | `{trainingReadiness?: number\|null, readinessStatus?: string\|null, note?: string}` → `DailyCheckin` |
| `GET /api/profile` · `PUT /api/profile` | `AthleteProfile` |
| `GET /api/profile/presets` | → `FocusPreset[]` |
| `GET /api/goals` · `POST` · `PATCH /{id}` · `DELETE /{id}` | `Goal` |
| `GET /api/plan?from=&to=` · `POST` · `PATCH /{id}` · `DELETE /{id}` | `PlannedSession` (with `match`) |
| `GET /api/plan/week?date=` | → `WeekPlan` (Monday-based week containing date) |
| `GET /api/shoes` · `POST` · `PATCH /{id}` · `DELETE /{id}` | `Shoe` |
| `POST /api/shoes/import-strava` | create/update shoes from Strava gear → `Shoe[]` |
| `POST /api/exports/multi` | `{activityIds?: number[], weeks?: number, include?: string[]}` → `{markdown, tokensEstimate, sessions}` |
| `GET /api/backup` | → JSON download (`Content-Disposition: attachment; filename=stride-backup-YYYY-MM-DD.json`) |
| `POST /api/backup/restore?mode=merge\|replace` | backup JSON → `{imported: {goals, plan, shoes, notes, checkins, profile}}` |
| `GET /api/admin/connections` | admin → `[{userId, provider, status, lastSyncAt, lastError, lastErrorAt}]` |
| `POST /api/admin/users/{id}/resync` | admin → 202 (drop cache + non-final rows, enqueue sync) |
| `GET /api/admin/users/{id}/backup` · `POST /api/admin/users/{id}/restore` | admin variants |

`include` = comma list among `pain,biomech,health,plan,load,splits,history`; default all. The directive is always present.

```ts
type Source = "garmin" | "strava";
interface Freshness { source: "db" | "cache" | "live"; updatedAt: string }   // live = a provider call happened for this response

interface Integrations {
  garmin: { connected: boolean; status: "ok" | "error" | "disconnected"; keyHint: string | null; tokenExpiresAt: string | null; lastSyncAt: string | null; lastError: string | null };
  strava: { available: boolean; connected: boolean; status: "ok" | "error" | "disconnected"; athleteName: string | null; lastSyncAt: string | null; lastError: string | null };
  weather: { enabled: boolean; provider: string };
}
interface SyncStatus { running: boolean; lastSyncAt: string | null; lastLiveAt: string | null; cooldownUntil: string | null; providers: { garmin: ProviderSync | null; strava: ProviderSync | null } }
interface ProviderSync { lastSyncAt: string | null; newActivities: number; error: string | null }

interface ActivitySummary {
  id: number; garminId: number | null; stravaId: number | null; sources: Source[];
  name: string; type: string; typeLabel: string; sport: "run" | "bike" | "walk" | "hike" | "swim" | "strength" | "other"; isRunning: boolean;
  startLocal: string; date: string;
  distanceM: number | null; durationS: number | null; avgPaceSecPerKm: number | null; gapSecPerKm: number | null;
  avgHR: number | null; avgPower: number | null; avgCadenceSpm: number | null;
  elevGainM: number | null; elevLossM: number | null; trainingLoad: number | null;
  painDuring: number | null; hasNotes: boolean; planStatus: "done" | "partial" | "extra" | null;
}
```

`ActivityView` embeds `Biomechanics`, `Segment`, `Series`, `RecoveryContext`, `Performance` and `Alert` (full definitions: `backend/internal/view`, `frontend/src/api/types.ts`), plus:
```ts
interface ActivityView {
  generatedAt: string; freshness: Freshness; sourceLabel: "Garmin" | "Strava" | "Fusion";
  activity: ActivityDetail & {
    gapSecPerKm: number | null;
    stamina: { startPct: number | null; endPct: number | null; minPct: number | null } | null;
    strava: { title: string; description: string; privateNote: string; rpe: number | null; relativeEffort: number | null; gearName: string | null } | null;
  };
  weather: (WeatherConditions & { windDirLabel: string | null }) | null;   // provider/weather.Conditions
  biomechanics: Biomechanics & {
    fatigueDrift: {            // running time only, pauses excluded, 1st third vs last third
      gctStartMs: number | null; gctEndMs: number | null; gctDeltaMs: number | null; gctDeltaPct: number | null;
      voStartCm: number | null; voEndCm: number | null; voDeltaCm: number | null;
      cadenceStartSpm: number | null; cadenceEndSpm: number | null; cadenceDeltaSpm: number | null;
      verdict: "stable" | "moderate" | "high";
    } | null;
    slope: {                   // grade from smoothed altitude/distance; downhill < -1.5 %, flat within ±1.5 %
      thresholdPct: number;
      flat: SlopeBucket | null; downhill: SlopeBucket | null; uphill: SlopeBucket | null;
      downhillCadenceDropSpm: number | null;   // flat − downhill, positive = drop
    } | null;
  };
  load: Load & {
    impactLoad: number | null;
    stamina: { startPct: number | null; endPct: number | null; minPct: number | null } | null;
    loadRatio: { acute: number; chronic: number; ratio: number | null; verdict: "low" | "optimal" | "high" | "danger"; source: "computed" } | null;
    stravaRelativeEffort: number | null;
  };
  segments: Segments;          // Segment gains gapSecPerKm: number | null
  series: Series & { gapSecPerKm: (number | null)[]; staminaPct: (number | null)[] };
  crossLoad: CrossLoad;
  recovery: (RecoveryContext & { readiness: Readiness }) | null;
  performance: Performance | null;     // always computed (no more modes)
  plan: PlanMatch | null;              // session planned that day vs this activity
  notes: ActivityNotes;                // empty values when nothing entered
  goals: Goal[];                       // active goals
  upcoming: PlannedSession[];          // next 3 planned sessions after this activity
  history: HistoryRow[];               // 14 days before (and including) this activity
  alerts: Alert[]; warnings: string[]; dataSources: string[];
}
interface SlopeBucket { cadenceSpm: number | null; strideM: number | null; strideMaxM: number | null; paceSecPerKm: number | null; durationS: number; distanceM: number }
interface Readiness { score: number | null; status: string | null; source: "manual" | null; reason: string | null }
interface CrossLoad {                  // window = J-1 (whole day) + J (until activity start)
  windowStart: string; windowEnd: string;
  bike: { rides: number; distanceKm: number; durationS: number; avgHR: number | null; trainingLoad: number | null };
  bikeByDay: { date: string; rides: number; distanceKm: number; durationS: number; avgHR: number | null; trainingLoad: number | null }[];
  floors: { prevDayUp: number | null; prevDayDown: number | null; dayUp: number | null; dayDown: number | null };
  steps: { prevDay: number | null; day: number | null };
  stressPrevDay: number | null;
  otherActivities: ActivitySummary[];
}
interface ActivityNotes { painDuring: number | null; painCold: number | null; painNextMorning: number | null; rpe: number | null; shoeId: number | null; shoe: Shoe | null; tags: string[]; note: string; updatedAt: string | null }
type ActivityNotesInput = Partial<Omit<ActivityNotes, "shoe" | "updatedAt">>;   // pains 0..10 step 0.5, rpe 1..10
interface HistoryRow { activityId: number; date: string; type: string; typeLabel: string; distanceM: number | null; durationS: number | null; paceSecPerKm: number | null; avgCadenceSpm: number | null; gctMs: number | null; elevLossM: number | null; trainingLoad: number | null; shoeName: string | null; painDuring: number | null; painCold: number | null; painNextMorning: number | null }

interface DayView {
  date: string; generatedAt: string; freshness: Freshness;
  recovery: (RecoveryContext & { readiness: Readiness }) | null;
  day: { steps: number | null; stepGoal: number | null; floorsUp: number | null; floorsDown: number | null; intensityModerateMin: number | null; intensityVigorousMin: number | null; activeCalories: number | null; stressAvg: number | null; bodyBatteryCharged: number | null; bodyBatteryDrained: number | null };
  crossLoad: CrossLoad; activities: ActivitySummary[]; planned: PlannedSession[];
  checkin: DailyCheckin | null; goals: Goal[]; upcoming: PlannedSession[];
  alerts: Alert[]; warnings: string[];
}
interface DailyCheckin { date: string; trainingReadiness: number | null; readinessStatus: string | null; note: string; updatedAt: string }

type FocusArea = "none" | "knee_pfps" | "achilles" | "shin_splints" | "other";
interface AthleteProfile {
  focusArea: FocusArea; focusLabel: string;            // label shown in prompts, e.g. "Genou / Fémoro-patellaire"
  injuredSide: "left" | "right" | "both" | null;
  timezone: string;
  thresholds: Thresholds;                              // defaults come from the focus preset, user overrides persisted
  paceTargets: { label: string; minSecPerKm: number; maxSecPerKm: number }[];   // e.g. 10 km 3:35–3:40, marathon
}
interface Thresholds {
  cadenceMinSpm: number; cadenceTargetMin: number; cadenceTargetMax: number;
  gctBalanceGapPct: number; verticalOscillationMaxCm: number; gctDriftMaxPct: number;
  descentMaxM: number; floorsPrevDayMax: number; loadRatioMax: number; rhrDeltaMaxBpm: number;
  planDeviationPct: number; downhillCadenceDropSpm: number;
}
interface FocusPreset { focusArea: FocusArea; label: string; thresholds: Thresholds; painLabels: { during: string; cold: string; nextMorning: string }; painEnabled: boolean; emphasis: string[] }
interface Goal { id: number; title: string; kind: "macro" | "race" | "cadence" | "weekly_volume" | "other"; targetValue: number | null; targetUnit: string | null; targetDate: string | null; status: "active" | "done" | "archived"; note: string }
interface PlannedSession { id: number; date: string; sport: "run" | "bike" | "strength" | "mobility" | "rest" | "other"; title: string; targetDurationS: number | null; targetDistanceM: number | null; targetPaceMinSecPerKm: number | null; targetPaceMaxSecPerKm: number | null; targetCadenceSpm: number | null; instructions: string; match: PlanMatch | null }
interface PlanMatch { plannedId: number; activityId: number | null; status: "done" | "partial" | "missed" | "upcoming" | "extra"; durationDeltaPct: number | null; distanceDeltaPct: number | null; paceDeltaSecPerKm: number | null; cadenceDeltaSpm: number | null; summary: string }
interface WeekPlan { weekStart: string; days: { date: string; planned: PlannedSession[]; done: ActivitySummary[] }[]; totals: { plannedDurationS: number; doneDurationS: number; plannedDistanceM: number; doneDistanceM: number; sessionsPlanned: number; sessionsDone: number } }
interface Shoe { id: number; name: string; brand: string; model: string; dropMm: number | null; type: "soft" | "tempo" | "carbon"; stravaGearId: string | null; retired: boolean; distanceKm: number; note: string }
```

Context tags (checkbox keys → FR labels): `metronome` → « Métronome activé », `warmup_iso` → « Échauffement / Activation isométrique pré-course », `squatting_48h` → « Travaux / Positions accroupies (48h) », `desk_sitting` → « Position assise bureau prolongée ».

Focus presets (labels FR): `none` « 100 % Performance – Aucune blessure » (pain form optional, injury alerts off except load), `knee_pfps` « Genou / Fémoro-patellaire » (cadence, downhill, OV, floors, descent emphasised), `achilles` « Tendon d'Achille » (cadence, GCT drift, uphill/D+, calf load), `shin_splints` « Périostite tibiale » (cadence, impact load, volume jumps, hard surfaces), `other` « Autre » (custom label).

Backup format: `{ "format": "stride-backup", "version": 1, "exportedAt", "profile", "goals", "plan", "shoes", "checkins", "activityNotes": [{ "activityRef": { "garminId", "stravaId", "startUtc" }, ...notes, "shoeRef": shoe name }] }` — never internal ids, never secrets.

---

## 4. Sync, caching, rate limits

- **Immutable storage**: an activity bundle (Garmin list item, detail, details series, FIT, Strava detail + streams, weather) is persisted gzip-compressed in SQLite once complete and never requested again. Activities finished < 2 h ago are not final (Garmin may still be computing).
- **Wellness**: day `D` is final (persisted) when `D < today − 1` in the user's timezone; otherwise cached in Valkey 10 min.
- **Lists**: provider activity lists cached 2 min; any unseen id triggers a bundle fetch.
- **Analyses**: cached in Valkey 1 h, key includes activity id, analysis version, profile hash and notes `updatedAt`.
- **Freshness**: every view reports `freshness` (`db` when all inputs came from SQLite, `cache` from Valkey, `live` if a provider call happened) and the UI shows it with the time.
- **Rate limiting**: every outbound call goes through `apimeter.Meter` (`Acquire` → call → `Observe`). 429 → provider cooldown (`Retry-After` or exponential 30 s → 15 min). 5xx/timeouts → 2 retries with backoff. Global concurrency per provider (Garmin 4, Strava 2, Open-Meteo 2) and per user (Garmin 6 for the parallel analysis fetch).
- **Scheduler** (platform): daily cycle at `STRIDE_SYNC_HOUR` (server TZ) iterating active users with a connected provider, sequentially, with a random 30–90 s jitter between users, calling `MorningSync`; skips users synced < 3 h ago. Plus **lazy sync**: the SPA calls `POST /api/sync` on load.
- **Strava webhook** on `create`/`update` of an activity: fetch it, match with a Garmin activity (same user, |start Δ| ≤ 120 s, compatible sport), fuse, persist; if Garmin has not got it yet, retry in 5 and 30 min.

---

## 5. Insights (domain)

Keep V1 analysis (biomechanics, blocks/laps, cardiac drift, recovery, performance, alerts) and add:
- **Fatigue drift**: running records only (exclude walk/stand blocks), split running time in thirds, compare first vs last: GCT ms and %, OV cm, cadence spm. Verdict on GCT %: < 3 stable, 3–5 moderate, > 5 high.
- **Slope**: smoothed grade (≥ 10 s / ≥ 20 m windows) from altitude & distance (or Strava `grade_smooth`); buckets downhill (< −1.5 %), flat (±1.5 %), uphill (> 1.5 %): cadence, average stride, max stride (rolling 10 s, spike-filtered), pace, duration, distance.
- **GAP**: Garmin `avgGradeAdjustedSpeed` / `directGradeAdjustedSpeed` per segment; Strava-only: compute with a standard grade cost model and say so in `warnings`.
- **Stamina**: start/end/min from `summaryDTO`; series from `directPotentialStamina`.
- **Cross-load 48 h**, **load ratio**, **readiness**, **weather**, **plan match**, **pain & shoes & tags** as typed above.
- **Alerts** (thresholds from profile, defaults below): cadence < 168 spm; downhill cadence drop > 5 spm; GCT drift > +5 %; OV > 9.5 cm; D− > 30 m; floors J-1 > 15; load ratio > 1.3; plan deviation > 20 % (duration/distance) or pace outside target; pain during ≥ 3 or next-morning > during; HRV low/unbalanced; RHR +5 bpm; sleep < 60; Body Battery < 50; G/D balance unavailable (info).
- Strava-only users: everything Garmin-specific returns `null` with reason « Garmin non connecté ».

---

## 6. Prompt templates (FR, exact)

Numbers use French formatting (V1 `frfmt`), absent → `N/D`, no empty sections: a section whose chip is off is omitted entirely; inside an enabled section, missing data shows `N/D` with a short reason when useful.

### 6.1 Directive (always first, verbatim)
```
> **DIRECTIVE SYSTÈME POUR L'IA :** Agis comme un ingénieur biomécanicien et entraîneur de course à pied strictement objectif, analytique et rigoureux. Ne cherche JAMAIS à flatter l'utilisateur, à minimiser ses erreurs ou à lui répondre pour lui faire plaisir. Analyse les données froidement : si une métrique (cadence, dérive, charge, allure, douleur) indique un risque de blessure, une compensation ou une erreur d'entraînement, dis-le-lui directement et ajuste le plan vers le bas s'il le faut. Dis uniquement ce qui est physiologiquement et mécaniquement juste pour lui.
```

### 6.2 Single session (`/api/activities/{id}/report`)
Chip → section: `plan` → CONTEXTE & OBJECTIFS; core (always) → RAPPORT SÉANCE; `biomech` → BIOMÉCANIQUE & DÉRIVE DE FATIGUE; `splits` → DÉTAIL DES INTERVALLES; `load` → CHARGE, STAMINA & IMPACT QUOTIDIEN; `health` → RÉCUPÉRATION & PRÉPARATION; `history` → HISTORIQUE 14 JOURS; ALERTES always. `pain` controls the « Ressenti & Douleur » line and the pain columns of the history table.

```text
> **DIRECTIVE SYSTÈME POUR L'IA :** … (6.1)

### CONTEXTE & OBJECTIFS DE L'ATHLÈTE
- **Profil / Mode actif :** [Ex: Suivi Pathologie (Genou / SFP) | Performance]
- **Objectif Macro :** [Ex: Courir 45 min continu sans gêne mi-novembre -> Reprise perf en décembre]
- **Séance prévue aujourd'hui :** [Ex: 38 min continu @ 5:20/km | Cible cadence: 170 spm]
- **Prochaines séances prévues :** [Liste des 2-3 prochaines séances planifiées dans l'agenda]

### RAPPORT SÉANCE - [Date & Heure]
- **Source :** [Garmin / Strava / Fusion] | **Météo :** [XX°C, Hum: XX%, Vent: XX km/h]
- **Réalisé :** [Distance km] en [Durée] | Allure: [X:XX/km] (GAP: [X:XX/km]) | Puissance: [XXX W]
- **Dénivelé :** +[XX] m / -[XX] m | **RPE / Notes :** [RPE et notes Strava/locales]
- **Chaussures :** [Modèle] (Drop: [X] mm, Type: [Souple/Tempo/Carbone])
- **Ressenti & Douleur :** Effort: [X/10] | À froid: [X/10] | Lendemain: [X/10] | Contexte: [Tags cochés]

### BIOMÉCANIQUE & DÉRIVE DE FATIGUE
- **Cadence moy :** [XXX spm] (Max: [XXX]) | % dans la cible (170-180): [XX %] | **Dérive Cadence :** [+/-X spm]
- **Cadence selon pente :** Plat: [XXX spm] | Descente (<-1.5%): [XXX spm]
- **Temps de contact sol (GCT) :** [XXX ms] | **Dérive GCT (Début vs Fin) :** [+/-XX ms] ([+/-X.X %])
- **Oscillation verticale :** [X.X cm] (Ratio: [X.X %]) | **Dérive OV :** [+/-X.X cm]
- **Longueur de foulée :** Moy: [X.XX m] (En descente: [X.XX m])
- **Équilibre G/D :** [XX.X % / XX.X % ou N/D]

### DÉTAIL DES INTERVALLES / SPLITS
| Bloc | Durée | Dist. | Allure | GAP | FC moy | Watts | Cadence | GCT | D+/D- |
|---|---|---|---|---|---|---|---|---|---|
| ... | ... | ... | ... | ... | ... | ... | ... | ... | ... |

### CHARGE, STAMINA & IMPACT QUOTIDIEN (48H)
- **Dérive cardiaque (Pw:Hr) :** [+/-X.X %] | **Stamina Fenix 8 :** Début [XX %] -> Fin [XX %] (Min: [XX %])
- **Charge séance :** Garmin Load [XX] | Impact Load: [XXXX] | TE Aérobie: [X.X] | Strava Relative Effort: [XX]
- **Ratio de charge global :** [X.XX] (Aiguë: [XXX] / Chronique: [XXX])
- **Charge croisée (48h) :** Vélo: [XX km / XX min] (Load: [XX]) | Escaliers (J-1/J): [XX / XX étages] | Pas (J-1): [XXXXX] | Stress J-1: [XX/100]

### RÉCUPÉRATION & PRÉPARATION (NUIT PRÉCÉDENTE)
- **Training Readiness :** [XX/100] ([Statut]) | **Body Battery réveil :** [XX/100]
- **VFC nocturne :** [XX ms] (Statut: [Équilibré/Bas] | Baseline: [XX-XX ms] | Moy 7j: [XX ms])
- **Sommeil :** Score [XX/100] (Durée: [XhXX] dont Profond: [XhXX]) | **FC repos :** [XX bpm]

### HISTORIQUE 14 JOURS (COURSE & RESSENTI)
| Date | Dist. | Durée | Allure | Cadence | GCT | D- | Load | Chaussures | Gêne Effort | Gêne Froid |
|---|---|---|---|---|---|---|---|---|---|---|
| ... | ... | ... | ... | ... | ... | ... | ... | ... | ... | ... |

### ALERTES AUTOMATIQUES
[Générer ici les alertes sur : Cadence < 168 spm, Cadence en descente qui chute, Dérive GCT > +5%, Oscillation > 9.5 cm, D- > 30m, Étages gravis J-1 > 15, Ratio de charge > 1.3, Écart important entre Prévu et Réalisé]
```
Implementation notes: « Profil / Mode actif » = `focusArea` label (e.g. « Suivi Pathologie (Genou / Fémoro-patellaire) » or « Performance »); the cadence target range comes from the profile (never hard-coded 170-180); « Training Readiness » shows the manual check-in or `N/D (non exposé par l'API Garmin, saisie manuelle possible)`; « Stamina Fenix 8 » label kept even if the watch differs (value `N/D` when absent); « Séance prévue aujourd'hui » adds the plan deviation summary when a match exists; alerts use ⚠️ / 🛑 / ℹ️ / ✅ prefixes like V1.

### 6.3 Day / health report (`/api/days/{date}/report`)
Directive, CONTEXTE & OBJECTIFS (`plan`), `### BILAN SANTÉ & CHARGE QUOTIDIENNE - [Date]` (steps vs goal, floors up/down, intensity minutes, stress, Body Battery charged/drained, bike/other activities of the day, planned session of the day and its status), RÉCUPÉRATION & PRÉPARATION (`health`), `### TENDANCE 7 JOURS` table (Jour | VFC | Statut | Sommeil | Profond | FC repos | Pas | Étages | Load | Gêne lendemain) (`health`), HISTORIQUE 14 JOURS (`history`), ALERTES AUTOMATIQUES.

### 6.4 Multi-session export (`POST /api/exports/multi`)
No splits. Structure:
```text
> **DIRECTIVE SYSTÈME POUR L'IA :** … (6.1)

### CONTEXTE & OBJECTIFS DE L'ATHLÈTE
- **Profil / Mode actif :** … | **Objectif Macro :** … | **Autres objectifs :** …
- **Période analysée :** [JJ/MM/AAAA] → [JJ/MM/AAAA] ([N] semaines, [N] séances)
- **Prochaines séances prévues :** …

### RÉCAPITULATIF DES SÉANCES
| Date | Type | Dist. | Durée | Allure | GAP | FC moy | Cadence | GCT | Dérive GCT | D- | Load | Chaussures | Gêne E/F/L |

### TENDANCES HEBDOMADAIRES
| Semaine | Séances | Volume | Durée | Load | Ratio charge | Cadence moy | GCT moy | VFC moy | Sommeil moy | Gêne effort | Gêne froid | Gêne lendemain |

### PRÉVU vs RÉALISÉ
- Séances prévues : [N] | réalisées : [N] | partielles : [N] | manquées : [N] | écart volume : [±X %]

### ALERTES & SIGNAUX RÉCURRENTS
- [alertes agrégées avec nombre d'occurrences]

### DEMANDE
- À partir de ces données, construis ou réadapte un plan d'entraînement pour les [N] prochaines semaines, cohérent avec les objectifs et la zone surveillée. Justifie chaque ajustement par les données ci-dessus.
```

---

## 7. Frontend (React) requirements

- **No more 3 tabs.** One unified, transparent dashboard: selected session (default = latest), today's status (plan vs done, readiness, recovery), week plan, goals, pain/shoes/tags quick entry. Smart chips above the prompt generator toggle `pain · biomech · health · plan · load · splits · history` (persisted), export copies the Markdown from `/report?include=…`.
- Header must be flawless at every width (320 → 2560 px), never overflow, sticky without visual artefacts.
- Auth: passkey-only (register / login / add passkey), invite links `/register?invite=CODE`, first-user flow. Onboarding wizard (profile & focus area → Garmin → Strava → done), each step skippable.
- Freshness pill (db / cache / live + time) and sync status; `POST /api/sync` on load.
- Planning & goals module (week view, plan vs done), pain form (0–10 step 0.5, labels from focus preset), shoes manager, context tags, daily readiness check-in.
- Multi-session export (select sessions or last N weeks).
- Settings: profile & thresholds, integrations, passkeys, recovery codes (4.4), backup export/restore JSON, account deletion.
- `/admin` (admin only): users (enable/disable/delete/role), invitations (create → copy link once, revoke), registration mode, provider connections, API meter (requests, 429s, cooldowns, recent errors), flush cache, resync user, user backup/restore.
- PWA: `manifest.json`, icons (any + maskable), iOS meta tags (`apple-mobile-web-app-capable`, status bar style, `apple-touch-icon`), Android `theme-color`, minimal service worker (network-first navigation, cache-first hashed assets).
- Glassmorphism design kept, mobile-first, accessible, no external network calls, French UI, rebranded « Stride » (new generic icon: no knee/kneecap).

---

## 8. Security rules

- Never read or print `.env` values, API keys, JWTs, OAuth tokens or cookies. Probes print structure only.
- No secrets or personal data (GPS, real names, real activity ids) in committed tests, fixtures, docs or logs.
- Outbound calls only to Garmin, Strava, Open-Meteo.

## 9. Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `STRIDE_MASTER_KEY` | **required** | 32-byte base64 AES key (`openssl rand -base64 32`) |
| `STRIDE_MASTER_KEY_PREVIOUS` | — | old key accepted for decryption during rotation |
| `STRIDE_PUBLIC_URL` | `http://localhost:8080` | external URL (WebAuthn origin, OAuth redirects, invite links) |
| `STRIDE_WEBAUTHN_RP_ID` / `STRIDE_WEBAUTHN_ORIGINS` | from public URL | override for reverse-proxy setups |
| `STRIDE_REGISTRATION` | `invite` | initial registration mode (`open`/`invite`) |
| `STRIDE_SESSION_TTL` | `720h` | session lifetime |
| `STRIDE_LISTEN_ADDR` | `:8080` | listen address |
| `STRIDE_METRICS_ADDR` | — (disabled) | separate listener for `GET /metrics` (e.g. `:9090`); never the public listener, not published by compose |
| `STRIDE_DATA_DIR` / `STRIDE_DB_PATH` | `./data` / `<data>/stride.db` | SQLite location |
| `STRIDE_VALKEY_URL` | — (memory) | `redis://valkey:6379/0` in compose |
| `STRIDE_ADMIN_GARMIN_KEY` (alias `GARMIN_API_TOKEN`) | — | auto-connect Garmin for the first admin |
| `STRIDE_STRAVA_CLIENT_ID` / `_SECRET` / `_WEBHOOK_TOKEN` | — | enable Strava |
| `STRIDE_STRAVA_ACCESS_TOKEN` / `_REFRESH_TOKEN` | — | one-time Strava seed for the first admin (§12.2) |
| `STRIDE_WEATHER` | `open-meteo` | `off` to disable |
| `STRIDE_SYNC_HOUR`, `STRIDE_SYNC_JITTER_MIN`, `STRIDE_SYNC_JITTER_MAX` | `7`, `30s`, `90s` | morning prefetch |
| `STRIDE_GARMIN_BASE_URL` | Garmin personal API | tests |
| `STRIDE_ALLOW_PRIVATE_EGRESS` | `false` | `true` lets user-supplied URLs (LLM `openai_compat` base URL, notification webhooks) reach loopback/private networks (e.g. a local Ollama) |
| `STRIDE_S3_ENDPOINT` / `_REGION` / `_BUCKET` / `_PREFIX` | — / `us-east-1` / — / — | one-time seed of the admin S3 backup target (§15.4) |
| `STRIDE_S3_ACCESS_KEY_ID` / `_SECRET_ACCESS_KEY` | — | same seed (secret sealed in DB, never returned) |
| `STRIDE_S3_PATH_STYLE` / `_AUTO_UPLOAD` / `_RETENTION_DAYS` | `false` / `false` / `30` | same seed |
| `TZ`, `LOG_LEVEL`, `LOG_FORMAT` | `Europe/Paris`, `info`, `pretty` | |
| `NO_COLOR` | — | set: logs without ANSI colors |
| `ENV_FILE` | `./.env` | path of the env file loaded at boot (existing process env wins) |
| compose only: `STRIDE_HOST_BIND`, `STRIDE_HOST_PORT`, `STRIDE_TAG`, `STRIDE_UID`, `STRIDE_GID` | `127.0.0.1`, `8080`, `latest`, `65532`, `65532` | published port, image tag (`latest` / `X.Y.Z` = releases, `prerelease` = main, `dev` = other branches), container user |

## 10. Domain implementation notes

- **Errors**: `404 NOT_FOUND` for any id of another user; `412 PROVIDER_NOT_CONNECTED` when Garmin/Strava is needed but not connected (or Strava not configured); provider refusing stored credentials → `502 PROVIDER_UNAUTHORIZED` (never 401, which means "session expired" to the SPA; `PUT /api/integrations/garmin` keeps `400 PROVIDER_UNAUTHORIZED`); provider cooldown/429 → `503 RATE_LIMITED` + `Retry-After`; provider 5xx/network → `502`/`504 PROVIDER_UNAVAILABLE`.
- **Responses**: `PUT|DELETE /api/integrations/garmin`, `POST …/garmin/test`, `DELETE /api/integrations/strava` return `Integrations`. `POST` goals/plan/shoes → `201` + object, `PATCH` → `200` + object, `DELETE` → `204`. `POST /api/admin/users/{id}/resync` → `202 {"queued":true}`. Strava callback errors: `reason` ∈ `state|denied|scope|exchange|already_linked|disabled|internal`.
- **Partial bodies**: `PUT /api/profile`, `PATCH`, notes and check-in bodies are merged onto the current value (`null` clears a nullable field, read-only keys such as `id`, `match`, `distanceKm`, `shoe`, `updatedAt` are ignored, unknown keys → 400). Changing `focusArea` without `thresholds` resets them to the new preset. `GET /api/days/{date}` and the check-in reject future dates.
- **Sync**: first sync lists 56 days (load ratio + trends); then each sync lists from `lastSync − 2 days`. Bundles (detail, details series, FIT, Strava detail + streams, weather) are prefetched for new activities of the last 7 days (max 6 per sync); older ones are fetched on first view or export (max 10 per export). Lazy sync is skipped for 5 min after a sync; `cooldownUntil` only reports provider rate limiting. Strava webhook retries (5 and 30 min) are in-memory; the next lazy sync catches up after a restart.
- **Wellness**: per-day kinds `sleep`, `hrv`, `daily_summary` (range endpoints, ≤ 28-day chunks) and `user_summary` (per day, J and J-1 only). A day without data is only persisted as final after 7 days (late watch syncs). Body Battery at wake, RHR 7 d and intensity minutes come from `usersummary/daily`; the V1 `bodyBattery/reports`, `dailyHeartRate` and `steps` calls are no longer needed. A day with step data but no floors counts as 0 floors.
- **Insights**: fatigue drift verdict falls back on the cadence drop (< −2 moderate, < −4 high spm) when GCT is not recorded; stride length falls back on speed ÷ cadence (warned). GAP uses Garmin `directGradeAdjustedSpeed` when recorded, the Minetti cost model otherwise (warned). Alert codes: V1 codes + `LOAD_RATIO` (was `ACWR`), `GCT_DRIFT`, `DOWNHILL_CADENCE_DROP`, `FLOORS_PREV_DAY`, `PLAN_DEVIATION`, `PAIN_DURING`, `PAIN_NEXT_MORNING`; descent/floors/downhill alerts only for injury presets (`none` keeps load, biomechanics and recovery alerts).
- **Plan**: matching per day and compatible sport (closest duration); `partial` when duration or distance is below target by more than `planDeviationPct`; `paceDeltaSecPerKm` is 0 inside the target range, otherwise the distance to the nearest bound. `ActivitySummary.planStatus = "extra"` only in weeks that have planned sessions (walks/hikes never count).
- **Shoes**: `distanceKm` sums activities whose notes point to the shoe, or (no note shoe) whose Strava gear matches `stravaGearId`.
- **History**: `ActivityView.history` lists every activity of the 14 days up to the session; the report table keeps running rows only.
- **Backup**: notes whose activity is not (yet) in the database are skipped and not counted.

---

## 11. V3 — Light UX, reconciliation, LLM connector, workouts

Rules: **no regression** of V2; migrations are **additive only** (new file `0003_v3.sql` and later; never edit `0001`/`0002`; SQLite `CHECK` changes require a safe table rebuild in the migration); a V2 database must upgrade automatically at startup.

### 11.1 Garmin + Strava reconciliation (zero duplicates)
- Same activity = same user, |Δstart| ≤ 300 s, compatible sport, distance within max(10 %, 300 m) when both known. Applies in **both orders** (Strava first then Garmin, or Garmin first) and to manual imports. A one-shot dedup pass at startup merges existing duplicate rows (keep the row holding notes; move notes, blobs and plan links).
- Field precedence on a fused activity: Garmin → physiology, running dynamics (GCT, VO, VR, balance, cadence), Stamina, Training Effect, impact load, FIT, laps; Strava → custom title, description, private note, RPE, Relative Effort, gear. `sources` = `["garmin","strava"]`, `sourceLabel` = `Fusion`. Strava's API exposes no GAP: GAP stays Garmin or computed (say so in `warnings`).
- Shoes: Garmin gear is **not exposed** by the Personal API (404) → `null` + reason; Strava shoes are imported automatically at each sync (name, brand, model, distance). Dedup: `stravaGearId` first, then normalized name/brand/model; `POST /api/shoes/{id}/merge {intoId}` moves notes and deletes the source. A fused activity whose Strava gear matches a shoe gets that shoe in its notes when the user has not chosen one (`notes.shoeAuto: true`).

### 11.2 External LLM connector (per user)
- Providers: `gemini` (`POST https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent`, header `x-goog-api-key`, `systemInstruction`), `anthropic` (`POST https://api.anthropic.com/v1/messages`, headers `x-api-key`, `anthropic-version: 2023-06-01`, `system`), `openai` (`POST https://api.openai.com/v1/chat/completions`, Bearer), `openai_compat` (Ollama or any OpenAI-compatible server: `{baseUrl}/v1/chat/completions`, Bearer optional). Plain `net/http`, no SDK. The objectivity directive (§6.1) is always the system message **and** stays at the top of the prompt.
- API key encrypted (AAD `llm:<uid>:key`), never returned (hint only). Calls go through `apimeter` (provider `llm`), timeout 120 s, max output ~4k tokens.
- **Egress guard** for user-supplied URLs (`openai_compat` base URL, notification webhooks): only `http(s)`, resolve then refuse loopback/private/link-local/metadata IPs at dial time unless `STRIDE_ALLOW_PRIVATE_EGRESS=true` (needed for a local Ollama). No redirects to private addresses.

| Method & path | Body → Response |
|---|---|
| `GET /api/llm` | → `LLMConfig` |
| `PUT /api/llm` | `{provider, model, baseUrl?, apiKey?}` (`apiKey` omitted = keep, `""` = remove) → `LLMConfig` |
| `DELETE /api/llm` · `POST /api/llm/test` | 204 · `{ok, latencyMs, message}` |
| `POST /api/llm/analyze` | `{kind: "activity"\|"day"\|"multi", activityId?, date?, activityIds?, weeks?, include?, question?}` → `LLMAnalysis` (412 `PROVIDER_NOT_CONNECTED` when not configured) |
| `GET /api/llm/analyses?kind=&ref=` | → `LLMAnalysis[]` (latest 20, stored) |

```ts
interface LLMConfig { configured: boolean; provider: "gemini" | "anthropic" | "openai" | "openai_compat" | null; model: string | null; baseUrl: string | null; keyHint: string | null; updatedAt: string | null }
interface LLMAnalysis { id: number; kind: string; ref: string; provider: string; model: string; markdown: string; promptTokensEstimate: number; durationMs: number; createdAt: string }
```

### 11.3 Plan generation, magic import, Garmin workout push
- Structured workout steps on planned sessions (`PlannedSession` gains `steps`, `source`, `garmin`):
```ts
type StepType = "warmup" | "run" | "interval" | "recovery" | "rest" | "cooldown" | "repeat";
interface WorkoutStep { type: StepType; durationS?: number; distanceM?: number; repeat?: number; steps?: WorkoutStep[]; target?: { kind: "none" | "pace" | "hr" | "cadence" | "power"; min?: number; max?: number }; note?: string }  // pace in s/km, hr bpm, cadence spm, power W
interface GarminPush { status: "none" | "pushed" | "failed" | "unsupported"; workoutId: number | null; pushedAt: string | null; error: string | null }
// PlannedSession += { steps: WorkoutStep[]; source: "manual" | "llm" | "import"; garmin: GarminPush }
```
- Exchange format imposed on any AI (prompt mode and LLM mode), parsed tolerantly (first ```json block or raw JSON):
```json
{"format":"stride-plan","version":1,"sessions":[{"date":"2026-11-10","sport":"run","title":"Footing + 6×1'","instructions":"Cadence 172 spm, relâché","targetDurationS":2700,"targetCadenceSpm":172,"steps":[{"type":"warmup","durationS":600,"target":{"kind":"pace","min":330,"max":360}},{"type":"repeat","repeat":6,"steps":[{"type":"interval","durationS":60,"target":{"kind":"pace","min":250,"max":260}},{"type":"recovery","durationS":90}]},{"type":"cooldown","durationS":600}]}]}
```

| Method & path | Body → Response |
|---|---|
| `POST /api/plan/generate` | `{mode: "prompt"\|"llm", horizon: "session"\|"week", startDate, note?}` → prompt mode `{prompt, tokensEstimate}` (directive + recent sessions, readiness, load ratio, goals, focus, existing plan + strict output contract above); llm mode `{drafts: PlannedSessionInput[], errors: string[], raw}` (not saved) |
| `POST /api/plan/import` | `{text}` → `{drafts: PlannedSessionInput[], errors: string[]}` (preview, not saved) |
| `POST /api/plan/bulk` | `{sessions: PlannedSessionInput[]}` → `PlannedSession[]` |
| `POST /api/plan/{id}/push-garmin` | tries Garmin Connect workout creation + scheduling; → `PlannedSession`; 412 `PROVIDER_NOT_CONNECTED`; when the Personal API refuses (404/403, capability cached 24 h) → 409 `CONFLICT` with `garmin.status = "unsupported"` |
| `GET /api/plan/{id}/workout.fit` | FIT workout file (`muktihari/fit` encoder), to copy into `GARMIN/NewFiles` on the watch |
| `GET /api/calendar?from=&to=` | → `{days: [{date, planned: PlannedSession[], done: ActivitySummary[]}]}` (≤ 371 days since 3.1.0, month view; days gain `events` §12.5) |

Reported facts: third-party coverage states the Garmin Training/Courses APIs are business-tier only; the push must degrade gracefully to the FIT download and the structure display.

### 11.4 Efficiency, cumulative impact, readiness
- **Aerobic efficiency** (run/bike, ≥ 20 min, steady effort: not interval-structured): `efSpeed` = speed (m/min) / avg HR, `efPower` = power (W) / avg HR, on moving running time (pauses excluded). `GET /api/trends/efficiency?weeks=12` → `{points: [{date, activityId, efSpeed, efPower, avgHR, paceSecPerKm}], rollingEfSpeed: (number|null)[], changePct: number|null}`.
- **Cumulative impact**: `steps` = Σ running cadence × time; `voFactor` = clamp(VO cm / 8, 0.75, 1.5); `descentFactor` = 1 + 0.01 × (D− m per km); `jointIndex` = steps × voFactor × descentFactor / 1000. Distinct from Garmin `impactLoad` (cardio/impact proxy) and from training load.
- **Readiness banner**: `level` red if (HRV 7-day avg < baseline low, or ≥ 3 of last 7 nights below) **and** load ratio > 1.3; orange if any of: HRV below baseline, load ratio > 1.3, 7-day sleep score avg < 60, RHR +5 bpm; else green; `reasons` FR one-liners. Present on `ActivityView` and `DayView`; when orange/red the prompts get a section right after the directive: `### ⚠️ SIGNAL DE PRÉPARATION PRIORITAIRE`.
```ts
// ActivityView += { efficiency: { eligible: boolean; reason: string | null; efSpeed: number | null; efPower: number | null; trendChangePct: number | null } | null; impact: { steps: number | null; stepsPerKm: number | null; voFactor: number | null; descentFactor: number | null; jointIndex: number | null; jointIndex7d: number | null } | null; readiness: ReadinessBanner }
// DayView += { readiness: ReadinessBanner }
interface ReadinessBanner { level: "green" | "orange" | "red" | "unknown"; title: string; reasons: string[]; hrv7dAvg: number | null; hrvBaselineLow: number | null; nightsBelowBaseline: number | null; sleep7dAvg: number | null; loadRatio: number | null }
```
- Report additions (CHARGE section): `- **Efficience aérobie (EF) :** [X,XX] m/min/bpm | Pw:HR [X,XX] W/bpm (tendance 8 sem. : [±X %])` and `- **Charge d'impact cumulée :** [N] foulées | Indice articulaire [X,X] (OV ×[X,XX] · D- ×[X,XX]) | 7 j : [X,X]`. Multi export weekly table gains `EF moy` and `Indice impacts`.

### 11.5 GPS track overlay
`GET /api/activities/{id}/track` → `{points: [lat, lon, tS, cadenceSpm|null, gradePct|null][], bounds: {minLat, maxLat, minLon, maxLon}}` (≤ 1500 points, own activities only, never cached publicly). Rendered client-side as a lightweight SVG (no tile server, no external call): colour by cadence (< `cadenceMinSpm` highlighted) or by slope (< −2 % highlighted), collapsible.

### 11.6 Personal notifications
| Method & path | Body → Response |
|---|---|
| `GET /api/notifications` · `POST` · `PATCH /{id}` · `DELETE /{id}` | `NotificationTarget` |
| `POST /api/notifications/{id}/test` | → `{ok, status, message}` |
```ts
interface NotificationTarget { id: number; kind: "discord" | "telegram" | "json"; label: string; enabled: boolean; hint: string; lastStatus: string | null; lastSentAt: string | null; lastError: string | null }
// create/patch input: { kind, label, enabled, url? (discord/json), botToken? + chatId? (telegram) } — secrets encrypted (AAD notify:<uid>:<id>), never returned
```
Sent once per newly synced (and fused) complete activity: FR compact summary (sport, distance, durée, allure, EF, impacts, top alerts) + link `${PUBLIC_URL}/?s=<id>`. Discord `{content}`, Telegram `sendMessage` (Markdown), JSON `{event: "activity.synced", activity, efficiency, impact, alerts, url}`. Egress guard applies.

### 11.7 Rescue import
`POST /api/imports` multipart field `file` (`.fit` / `.gpx` / `.tcx`, ≤ 25 MB) → `{activityId, merged: boolean}`; GPX/TCX converted to the internal series (HR, cadence, power, altitude, distance when present); deduplicated with the §11.1 rule (merging into an existing activity when it lacks a FIT). UI entry is discreet (Settings › Import de secours).

### 11.8 Light UX (frontend)
Progressive disclosure: the dashboard first screen shows only the readiness banner, a compact session hero (distance, durée, allure, 3 key signals), the top 3 alerts, the « Ressenti » quick entry and the two AI actions (« Copier le prompt », « Analyser avec mon IA »). Everything else (biomechanics, fatigue drift, slope, splits, charts, map, recovery details, load, cross-load, EF trend, history) lives in collapsible sections / drawers, closed by default except when an alert points to it. More whitespace, fewer borders, clear type scale; usable in 5 s on desktop and mobile PWA.

### 11.9 Backend implementation notes (V3, normative precisions)
- **Schema** (`0003_v3.sql`): `activities.import_format`, `activities.notified_at`; `activity_notes.shoe_auto`; `shoes.strava_distance_m`; `planned_sessions.steps/source/garmin_*`; `activity_blobs` rebuilt (data kept) to accept `import_fit|import_gpx|import_tcx`; new `llm_configs`, `llm_analyses`, `notification_targets`, `activity_metrics` (persisted EF/impact per activity, recomputed when the bundle or `MetricsVersion` changes). Rows present before V3 get `notified_at` set (never notified).
- **Additive JSON**: `ActivitySummary.sources` may contain `"import"` and `sourceLabel` may be `"Import"` (rescue import without provider); `ActivityDetail += { garminGear: null; garminGearReason: string }`; `Shoe += { stravaDistanceKm: number | null }` (all-time distance reported by Strava); `ActivityNotes += { shoeAuto: boolean }` (read-only; an auto shoe does not set `hasNotes`). The auto shoe applies to every activity carrying a Strava gear (fused or Strava-only) whose shoe was not chosen by the athlete; changing `shoeId` clears `shoeAuto`.
- **Dedup**: one-shot at startup (`app_settings.v3_dedup_done`); rows of the same provider never merge; kept row = the one holding notes, else the Garmin one; when both hold notes they are merged field by field (kept row wins); blobs move unless the kept row has the same kind; plan links are computed (no table), nothing to move.
- **Status codes**: `POST /api/plan/bulk` → 201 (all sessions validated first, none created on error); `POST /api/notifications` → 201; `POST /api/imports` → 201 when created, 200 when merged; `push-garmin` 409 body = `{error, code: "CONFLICT", session: PlannedSession}`; LLM routes answer 412 `PROVIDER_NOT_CONNECTED` when no connector. Plan drafts are `PlannedSession` objects with `id: 0` (`id`, `match`, `garmin` ignored on input).
- **Garmin push**: create then schedule; a scheduling failure keeps `status: "pushed"` with `error` explaining to schedule from Garmin Connect. FIT targets: pace as custom speed range, HR `bpm + 100`, power `W + 1000`, cadence in spm.
- **Efficiency**: moving samples = speed ≥ 1 m/s and recording gap ≤ 10 s (summary `moving_s`/distance/avg HR fallback without series); not steady when the FIT has ≥ 2 recovery laps or ≥ 3 active workout-step laps, when the 60 s speed (GAP when known) coefficient of variation > 0.25, or when the matched planned session has `interval`/`repeat` steps. `trendChangePct`/`changePct` = mean `efSpeed` of the last 4 weeks vs the first 4 weeks of the window (8 weeks ending at the activity; `weeks` for the endpoint, running only), ≥ 2 points per side else null; `rollingEfSpeed[i]` = mean over the 28 days ending at point i (null below 2 points).
- **Impact**: steps from the running cadence series over moving time (else avg cadence × moving time); `voFactor` null when VO is unknown (neutral 1 used in `jointIndex`); D− from the summary, the FIT session or the altitude series; `jointIndex7d` = Σ `jointIndex` of the running activities of the 7 days ending at the activity (inclusive).
- **Readiness**: `hrvBaselineLow` = latest Garmin `balancedLow` of the 7 days; a night is below when its HRV < that day's (else latest) low; orange "HRV below baseline" = 7-day avg or last night below; RHR +5 = resting HR vs 7-day avg; `unknown` when no HRV, sleep, RHR nor load ratio.
- **LLM**: `openai` sends `max_completion_tokens`, `openai_compat` `max_tokens`; `baseUrl` stored without trailing `/v1`; `include` accepts a CSV string or an array; `question` is appended as `### QUESTION DE L'ATHLÈTE`; 200 analyses kept per user.
- **Notifications**: window 48 h after the start; when Garmin and Strava are both connected, a single-source activity waits for the fusion until it is final (2 h after its end); rescue imports are not notified; failures never echo the target URL or token.
- **Track**: `{points: [], bounds: null}` when no GPS series (FIT, rescue import or Strava `latlng`) is stored; `Cache-Control: private, no-store`.

### 11.10 Version and audit logs
- `GET /api/version` (user) → `{version, commit, builtAt, go, label}` from `-ldflags -X stride/internal/buildinfo.*` (repo `VERSION` file; CI adds `-dev` off `main`); shown discreetly at the bottom of Settings with a copy button.
- `app_logs` (migration `0004_logs.sql`): categories `request` (API requests: method, path, status, latency; outbound provider calls via `apimeter`: provider, endpoint template, status, latency — error texts are never stored) and `user_action` (mapped from routed patterns: login, sync, prompt generation, LLM analysis, plan changes, settings…; `POST /api/actions {action: "prompt.copy"|"prompt.download"|"prompt.share"}` for browser-only events). Levels `info` / `error` (HTTP ≥ 400, rate limits, network errors). Asynchronous batched writes (bounded buffer), purge after 14 days and above 100 000 rows.
- `GET /api/admin/logs?category=all|request|user_action&level=all|error&userId=&before=&limit=` (admin) → `{items, nextBefore, dropped}`.

---

## 12. 3.1.0 — Live integrations, long-term planning, calendar events, records

### 12.1 Verified provider facts (live probe, 2026-11-06)
- **Garmin Personal API**: token + activities OK. **404**: personal records (`/personalrecord-service/*`), activity weather, gear (`/gear-service/*`), workouts list/create/schedule (`/workout-service/*` — POST create returns 404), calendar, training plans, likes/comments. No like/gear/weather field in activity JSON. ⇒ Garmin push stays `unsupported` (FIT download is the delivery path); Garmin shoes, likes, PRs and weather are `null` with a reason.
- **Strava**: the `.env` tokens only carry the `read` scope: `/athlete` and `/athletes/{id}/stats` work, `/athlete/activities` → 401. Activities, `kudos_count`, `best_efforts`, gear need `activity:read_all` + `profile:read_all` (user re-authorization). The API exposes no weather and no training plans.
- Weather therefore always comes from Open-Meteo (activity start lat/lon + time); indoor / no-GPS activities → `null` + reason.
- Implementation: `ActivityView += { weatherReason: string | null }` (indoor / no GPS, weather disabled, Open-Meteo unavailable). A final bundle without weather (Open-Meteo down or weather disabled at the time) is backfilled when the activity is viewed again (10 s budget, retried 30 min after a failure). Garmin push answers 409 `unsupported` with a French message pointing to the `.FIT` download (`GARMIN/NewFiles`).

### 12.2 Env seed & per-user credentials
- Credentials are per user (encrypted, AAD `<provider>:<uid>:<field>`): Garmin key, Strava OAuth tokens, LLM key. `.env` only provides the Strava app (`STRIDE_STRAVA_CLIENT_ID/SECRET`) and a **one-time seed for the first admin** when the DB has no credential yet: `STRIDE_ADMIN_GARMIN_KEY`, `STRIDE_STRAVA_ACCESS_TOKEN` + `STRIDE_STRAVA_REFRESH_TOKEN` (athlete id read from `/athlete`). Refreshed Strava tokens are stored in DB (never written back to `.env`).
- `Integrations.strava` gains `scopes: string[]`, `missingScopes: string[]`; when `activity:read_all` is missing the UI asks to reconnect (OAuth flow).
- Seed details: the env access token is checked with `GET /athlete` (athlete id + name); if Strava rejects it, the refresh token is used once. Scopes are detected and stored: `read` always, `profile:read_all` when `/athlete` answers the detailed representation, `activity:read` when listing activities works (a seeded token cannot prove `_all`). The seed runs at first-admin creation and at startup when the first admin has no Strava connection and no seed was done yet (`app_settings.seed_strava_admin`: a deliberate disconnection is kept). The unknown expiry of a seeded token is assumed to be 6 h; a token rejected by Strava is refreshed once and the call retried. Without `activity:read` the Strava sync is skipped silently (no error status).
- `scopes`/`missingScopes` are `[]` when disconnected; `missingScopes` = `read, activity:read_all, profile:read_all` minus the granted scopes.
- Addition: `GET /api/integrations/strava/stats` → `{ recentRun, ytdRun, allRun, recentRide, ytdRide }`, each `{ count, distanceM, movingS, elevGainM }` (`/athletes/{id}/stats`, works with `read`; 412 when not connected).

### 12.3 Personal records & kudos
```ts
type RecordKey = "1k" | "5k" | "10k" | "half" | "marathon" | "longest"
interface PersonalRecord { key: RecordKey; label: string; distanceM: number; timeS: number | null; paceSecPerKm: number | null; date: string | null; activityId: number | null; source: "computed" | "strava" | "manual" }
// ActivitySummary/ActivityDetail += { kudos: number | null }   (Strava kudos_count; Garmin likes not exposed)
```
| Method & path | Body → Response |
|---|---|
| `GET /api/records` | → `{ records: PersonalRecord[] }` (best of computed / Strava / manual; manual wins when set) |
| `PUT /api/records/{key}` | `{ timeS?, distanceM?, date? }` → `PersonalRecord` (manual) |
| `DELETE /api/records/{key}` | remove the manual value → 204 |
Computed = fastest rolling window of the distance over running records (FIT / details / Strava streams), recomputed with activity metrics. Records are injected in `CONTEXTE & OBJECTIFS` of every prompt: `- **Records personnels :** 5 km 22:41 (12/10/2026) · 10 km … · Plus longue sortie 21,4 km`.
- Algorithm: wall-clock time over the cumulative distance, window start interpolated between samples; samples whose time does not increase or distance decreases are skipped; sample-to-sample speeds > 12 m/s (GPS jumps) split the series; windows faster than 2:30/km are discarded; whole seconds. An activity without series counts as a whole when its distance is within [D, 1.03 × D] (time scaled to D). Strava `best_efforts` (`elapsed_time` of "1k", "5k", "10k", "Half-Marathon", "Marathon") compete with the computed values (the faster wins, `source` tells which). Per-activity bests are persisted in `activity_records` (version + `bundle_at`): only changed activities are recomputed (lazily and after each sync).
- `GET` always returns the 6 keys in order; unknown → `timeS: null`, `source: "computed"`. `longest` = longest running activity (`distanceM`, `timeS` = its duration). Prompt line: `Aucun` when nothing is known.
- `PUT`: distance records need `timeS` (pace 2:00–30:00/km; `distanceM`, if sent, must be the record distance); `longest` needs `distanceM` (100 m–500 km), `timeS` optional. Unknown key → 404. `DELETE` is idempotent (204).
- `kudos` is stored in `activities.kudos` from Strava list/detail `kudos_count`.

### 12.4 Manual activities (standalone mode)
`POST /api/activities` `{ date, startTime?, sport, title?, distanceM?, durationS, avgHR?, maxHR?, avgCadenceSpm?, elevGainM?, elevLossM? }` → `ActivitySummary` (`sources: ["manual"]`, `sourceLabel: "Manuel"`); `PATCH` / `DELETE /api/activities/{id}` allowed on manual activities only (409 otherwise). Notes/pain/shoes use the existing notes endpoint. Every view, report and prompt works with zero provider connected.
- `POST` → 201. `sport` ∈ `run | bike | walk | hike | swim | strength | other`; `startTime` `HH:MM` (profile timezone, default `12:00`); `date` ≤ today; `durationS` 60 s–48 h; HR 30–250 (`maxHR ≥ avgHR`); `title` defaults to the type label. `sourceLabel` (`"Manuel"`) is on `ActivityView`. A provider activity later matched by the §11.1 rule fuses into the manual one (provider data wins, it is no longer manual). `PATCH` re-triggers metrics and records.

### 12.5 Calendar events & adaptation
```ts
type EventKind = "fatigue" | "pain" | "illness" | "unavailable" | "forced_rest"
interface CalendarEvent { id: number; kind: EventKind; startDate: string; endDate: string; severity: 1 | 2 | 3; note: string }
// GET /api/calendar days += { events: CalendarEvent[] }
interface PlanPhase { id: number; name: string; startDate: string; endDate: string; focus: string; weeklyVolumeKm: number | null; note: string }
```
| Method & path | Body → Response |
|---|---|
| `GET /api/calendar/events?from=&to=` · `POST` · `PATCH /{id}` · `DELETE /{id}` | `CalendarEvent` |
| `POST /api/plan/{id}/shift` | `{ days?: number, toDate?: string }` → `PlannedSession` (move one session) |
| `POST /api/plan/adapt` | `{ from: string, mode: "rules" \| "llm", eventId?: number }` → `{ sessions: PlannedSessionInput[], removeIds: number[], summary: string }` (preview, nothing saved) |
| `POST /api/plan/replace` | `{ removeIds: number[], sessions: PlannedSessionInput[] }` → `PlannedSession[]` (apply a preview atomically) |
| `GET /api/plan/phases` | → `PlanPhase[]` |
Rules mode: during an event no session (moved to the next free day or dropped when the event lasts > 3 days); after `illness` / `pain` / `forced_rest` the next 3–7 days lose intensity (intervals → easy, −20–40 % duration by severity); no week may exceed the planned volume of the following week by more than 10 %. LLM mode sends the events, recent load, readiness and remaining plan, and expects the `stride-plan` JSON.
- Events: `POST` → 201, `DELETE` → 204; `GET` without `from`/`to` lists all; `endDate` defaults to `startDate`, `severity` to 2; ≤ 366 days. `GET /api/calendar` covers ≤ 371 days. `shift`: exactly one of `days` (±365, ≠ 0) or `toDate`.
- Rules, precisely: events = `eventId` or all events ending ≥ `from`; the next free day is the first day after the event without a non-rest session nor event, within 14 days (else dropped); lighter days = 1 + 2 × severity (3/5/7) after illness / pain / forced rest: duration and distance −(10 + 10 × severity) % (20/30/40 %), interval sessions become `Footing facile` without steps nor pace target, other steps are scaled. Volume rule as implemented: a week receiving moved sessions may not exceed max(its planned volume, 110 % of the next week's planned volume); otherwise moved sessions are dropped (latest first). Preview `sessions` are drafts (`id: 0`), `removeIds` their originals; nothing changes until `replace`.
- LLM mode: period [`from`, max(`from` + 13, last event end + 7)] capped at `from` + 41; `removeIds` = every session of the period (empty when the AI returned no valid session); parsing warnings are appended to `summary`. 412 without LLM.
- `replace` → 200 with the created sessions; an unknown id → 404 and nothing is applied (single transaction).

### 12.6 Long-term generation up to the goal
`POST /api/plan/generate` `horizon` ∈ `session | week | 2weeks | goal` (+ `goalId?`, default: nearest active goal with a `targetDate`). `goal`: ≤ 21 days → every session until the date; otherwise `phases` until the date (build, assimilation, specific, taper) + detailed sessions for the next 2–4 weeks; asking again later extends the detail. Exchange format `stride-plan` **version 2** (v1 still accepted):
```json
{"format":"stride-plan","version":2,"goal":{"title":"10 km","date":"2027-02-15"},"phases":[{"name":"Développement","startDate":"2026-11-09","endDate":"2026-12-06","focus":"volume aérobie, cadence","weeklyVolumeKm":35,"note":""}],"sessions":[…v1 sessions…]}
```
Responses gain `phases: PlanPhase[]` (drafts with `id: 0`); `POST /api/plan/bulk` accepts `{ sessions, phases? }` (phases replace those overlapping their dates).
- `week` = 7 days, `2weeks` = 14 days from `startDate`. `goal`: `goalId` without `targetDate`, no dated active goal, or a goal more than 366 days away → 400. With phases (> 21 days), the detailed window starts the day after the last session already planned before the goal (asking again extends the detail) and lasts 4 weeks (2 weeks when fewer than 8 weeks remain). Prompts list existing phases and calendar events of the period; contract v2 only when phases are requested. Tolerant phase parsing (`start`/`end`, `"35 km"`).
- `bulk`: ≤ 120 sessions and ≤ 12 phases, `sessions` may be empty when `phases` are given; one transaction. `GET /api/plan` without dates now covers up to today + 366 days.

### 12.7 Version (3.1.x)
`VERSION` was `3.1.1` prior to the `4.0.0` architectural upgrade below.

---

## 13. 4.0.0 — Goose migrations, in-process jobs, SSE, Prometheus, cycles, body map, share view, Cmd+K

### 13.1 Platform & observability
| Method & path | Auth | Response |
| --- | --- | --- |
| `GET /version` | Public | `BuildInfo` (`{version, commit, builtAt, go, label}`), same payload as `GET /api/version` |
| `GET /metrics` | metrics listener only (`STRIDE_METRICS_ADDR`) | Prometheus exposition, series in [METRICS.md](METRICS.md) |
| `GET /api/jobs` · `GET /api/jobs/{id}` | User | the user's jobs (`JobStatus`), another user's job is 404 |
| `GET /api/events/stream` | User | SSE: `connected`, `job.updated`, `sync.updated`, `alert.triggered`, `llm.token`, `llm.done`; 20 s heartbeat; Valkey Pub/Sub fans events out across instances |

SPEC.md and `frontend/src/api/types.ts` are the API contract (no generated OpenAPI document).

**Jobs (in-process).** One worker pool per kind group so slow LLM calls never starve sync: `sync` (sync, Strava webhooks, imports, history; 2 workers), `llm` (2), `webhook` (4); inside a group `critical` > `default` > `low`. Retries: 10 s, 1 min, 6 min, then 15 min. Webhooks are retried only on refused connections, 429 and 5xx, never after a timeout (the request may have been delivered). `async_jobs.error` and `job.updated` carry public French messages only, never raw error texts (they may hold webhook URLs or bot tokens). At startup, `queued` / `running` / `retrying` jobs left by the previous process are marked failed (« Tâche interrompue par un redémarrage du serveur. »); on shutdown queued jobs are dropped and running ones get the remaining grace period. `POST /api/llm/analyze`, `/api/plan/generate`, `/api/plan/adapt` and `/api/imports` wait at most 100 s (below the 150 s `WriteTimeout`) and answer **202 + `JobStatus`** past that or with `?async=1`; the result then arrives through `job.updated` / `llm.done`. The hourly maintenance purges finished jobs and revoked or expired share links after 14 days.

### 13.2 Versioned SQL migrations (`internal/database`)
- `pressly/goose/v3`, embedded `.up.sql` / `.down.sql` pairs (`00001_platform` .. `00007_v41`); applied files are never edited.
- Legacy 3.x `schema_migrations` rows are bridged into `goose_db_version` before the newer migrations run.
- Boot only asserts that the expected tables and columns exist; orphan rows (`PRAGMA foreign_key_check`) are logged as warnings in the background. `stride doctor` runs the full `integrity_check` + `foreign_key_check` (exit 1 on problems).

### 13.3 Training cycles (`/api/cycles`)
- `training_plans` (macro) → `plan_phases` (méso: `foundation | build | threshold | specific | assimilation | taper | recovery`) → `training_weeks` (micro, Monday-aligned overrides).
- `GET /api/cycles[?planId=]` (unknown or foreign plan → 404; default: the active plan, else the latest), `POST /api/cycles/plans` (`autoScaffold`; load targets only from `peakWeeklyLoad`), `PATCH`/`DELETE /api/cycles/plans/{id}`, `POST`/`PATCH`/`DELETE /api/cycles/phases[/{id}]`, `PUT /api/cycles/weeks`. `planId`, `phaseId`, `goalId` must belong to the user (404); a week's phase must belong to its plan (400).
- One phase model: imported / bulk / replaced plan phases (`/api/plan/bulk`, `/api/plan/replace`, LLM import) attach to the active plan (else to no plan) and replace only that plan's overlapping phases; `GET /api/plan/phases` returns the phases `/api/cycles` shows.
- `CycleActuals`: planned volume/duration from the week targets, else the planned sessions; planned load only from targets; actual load = measured training load; completed sessions = planned sessions matched to an activity.

### 13.4 Body map & correlations (`/api/bodymap`)
- 6 zones (`knee_pfps`, `achilles`, `plantar_fascia`, `shin_splints`, `iliotibial_band`, `hamstrings_calves`), `painScore` 0–10, optional `coldNextDayScore` (J+1). `GET /api/bodymap?days=7..3650&zone=`. `activityId` must belong to the user (404).
- Correlations compare runs linked to a zone's entries (same activity or day) with the other runs of 90 days, on measured values only: `elev_loss` (m), `shoe_wear` (km of the pair at the run), `shoe_drop` (mm), `gct` (ms). Below 2 symptomatic or 3 baseline runs with a value, `symptomaticMean`, `baselineMean`, `deltaPct`, `summary` are `null` and `insufficientData` is `true`; `confidence` is `insufficient | low (≥2/≥3) | moderate (≥3/≥5) | high (≥5/≥10)`; `summary` quotes the computed numbers; `shoeHighlight` is the pair most used on symptomatic runs.
- Prompts: with the `pain` chip, `### CARTOGRAPHIE CORPORELLE` lists the 30-day entries and becomes `… & CORRÉLATIONS` only when conclusive correlations exist (listed with their sample sizes).

### 13.5 Read-only share links (`/api/share`, `/share/:token`)
- `GET`/`POST /api/share`, `DELETE /api/share/{id}` (revocation), `PATCH` in 4.1 (§14.4). Tokens `st4_<base64url>` (256 bits); SQLite stores `SHA-256(token)` and the URL sealed with AAD `share:<uid>:url`. `expiresAt` must be RFC 3339 (400 otherwise).
- `GET /api/public/share/{token}` (`Cache-Control: private, no-store`): unknown, revoked or disabled-owner link → 404, expired → 410; 30 views per client IP and minute (429). The computed view is cached 5 min under the token hash and dropped on revoke or edit. Metrics come from stored data only (EF and joint index from `activity_metrics`, Garmin impact load, shoe from notes/gear, pains from notes); missing values are `null`. Request logs and audit entries keep only the 8-character token prefix.

### 13.6 Command palette (`Cmd+K` / `Ctrl+K`)
Keyboard navigation, session search (`GET /api/search`, §14.3) and direct actions.

### 13.7 Version
`VERSION` = `4.0.0` (no `v`, see release cycle).

## 14. 4.1.0 — Navigation, health golden source, full history, share options

`VERSION` = `4.1.0`. Migrations additive (`00007_*`), a 4.0 database upgrades losslessly.

### 14.1 Navigation (five menus)
| Menu | Route | Content |
| --- | --- | --- |
| Tableau de bord | `/` | essential only: readiness, session hero, top alerts, Ressenti, AI actions; details in collapsed sections |
| Planning | `/plan` | sessions, cycles, calendar, imprévus |
| Santé & forme | `/health` (old `/body` redirects) | health status (bonne santé / mode performance vs souci suivi), body map, symptoms history, correlations, physio share links |
| Coach IA | `/coach` (old `/export` redirects) | prompt export (copy/multi-session) and, when an LLM is configured, AI analysis and history |
| Réglages | `/settings` | account, integrations, LLM, notifications, shoes, backup, version |
Admin stays in the user menu. Search (Cmd/Ctrl+K) is an icon button in the header actions (and in the mobile menu) opening the palette; it never overlaps the main navigation at any width.

### 14.2 Health golden source
- `body_stress_entries` is the single source of truth for symptoms. Health status (`performance` = bonne santé, or `tracking` with one or more zones + side) is managed only in Santé & forme; the profile `focusArea` / `injuredSide` are derived from it (kept for prompts/presets compatibility) and removed from Réglages (replaced by a link).
- `GET /api/health/status` → `{ mode: "performance" | "tracking", zones: { zone, side, since, note }[], focusArea, updatedAt }`; `PUT /api/health/status` same body (without derived fields). Zone → preset: `knee_pfps`→`knee_pfps`, `achilles`→`achilles`, `shin_splints`→`shin_splints`, others→`other` (label = zone label); several zones → the most recent as `focusArea`.
- Declaring a symptom on an activity (Ressenti form) writes body entries linked to that activity (one per zone; `pain_score` = during, `cold_next_day_score` = next morning/J+1). `activity notes` pain fields (`painDuring`, `painCold`, `painNextMorning`) become a projection of those entries (max per activity) so reports, history and prompts keep working; writing pains through the notes endpoint maps to the active zone(s). No double source.
- Implementation: table `health_status` (zones JSON); without a row the status is derived from a 4.0 profile (`knee_pfps`, `achilles`, `shin_splints` → that zone, else performance). PUT → 400 for an unknown mode, `tracking` without zone, duplicated or unknown zone, invalid `since`. The profile focus follows the most recent zone (by `since`, undated zones last); unmapped zones give `other` labelled with the zone label. A focus sent to `PUT /api/profile` is converted into a health status. Body entries linked to an activity are unique per activity and zone (POST updates the existing one). Pains written through the notes endpoint without any tracked zone stay on the notes (legacy).

### 14.3 Full history sync
- `POST /api/sync/history` `{ from: "YYYY-MM-DD" | null }` (`null` = everything the provider returns) → `JobStatus` (in-process queue, low priority). Paginates Garmin activity list (and Strava when the scope allows) backwards, stores summaries only (bundles stay lazy on view/export), dedup/fusion rules unchanged, progress `processed/total` on the job, resumable (`history_from` updated per page), meter + backoff respected (429 → pause, resume).
- Implementation: job kind `sync.history`, Garmin listed in 180-day windows from the covered history down to `from` (`null`: down to 2008 or three empty windows in a row), `history_from` advanced per window; Strava pages of 100 summaries before the covered history; the job retries (10 s, 1 min, 6 min…) on 429, timeouts and 5xx and resumes; it waits while another sync of the user runs. Answer **202**; user without provider → 412.
- `GET /api/activities` keeps paging (`before`) and date filters; `GET /api/search?q=` (palette) matches activity names and dates over the whole stored history (e.g. « marathon », « 14 avril »). `GET /api/search?q=&limit=20` (max 50) → `{ query, items: [{ id, date, startLocal, name, sport, typeLabel, distanceM, durationS }] }`, newest first; dates: ISO prefixes, `14/04[/2019|/19]`, `14 avril`, `1er mai 2025`, `avril 2025`.

### 14.4 Share options
- `share_links` gains `scope_json` (default = everything, so existing links keep their behaviour). Create/patch body: `{ label, expiresInDays: 1 | 7 | 30 | 90 | null, periodWeeks: 4 | 8 | 12 | 26 | 52, sections: ("activities" | "health" | "recovery" | "load" | "records" | "plan" | "shoes" | "notes")[], anonymize: boolean }`; `PATCH /api/share/{id}` edits label/expiry/scope. The public payload only contains the allowed sections (others absent, not empty) and the period window; `anonymize` replaces the display name by « Athlète ».
- Shoes in the public view show `name`, `brand`, `model` (never an internal id).
- Implementation: `scope_json` `{periodWeeks, sections, anonymize}`; `{}` (4.0 links) = every section over 12 weeks. Create/PATCH accept any subset; `expiresInDays` restarts from now (`null` or `0` = never); `sections` are returned in the canonical order; unknown fields → 400. Payload keys per section: `recovery` → `readiness`, `load` → `loadSummary` + `loadSeries` (one point per day of the period), `plan` → `cycles`, `health` → `bodyMap` + athlete `focusArea` / `focusLabel` / `injuredSide`, `records` → `records`, `shoes` → `shoes`, `activities` → `recentSessions` (period, newest first, 200 max), `notes` → per-session `painDuring`, `painNextMorning`, `note`. `periodWeeks` and `sections` are always present.

### 14.5 Shoe mileage
- A shoe's total is the provider's real figure: `distanceKm` = Strava's all-time gear `distance` when the pair is linked (`stravaGearId`) and synced, else `analyzedKm` (sum of the activities Stride holds for the pair: notes shoe, else matching Strava gear). `distanceSource: "strava" | "stride"` says which; the UI labels it (« total Strava » with « dont X km analysés par Stride », « total Stride »). A partial local sum is never shown as the total. Reports, prompts, correlations and the public share use `distanceKm`.
- Gear sync (Strava sync + `POST /api/shoes/import-strava` on demand): `GET /athlete` (needs `profile:read_all`; a summary athlete means the scope is missing) lists the active shoes with `name`, `primary`, `distance`; `GET /gear/{id}` completes `brand_name`/`model_name` once and checks pairs Strava no longer lists. Label = Strava name, else « brand model », never an id. Calls go through the API meter; the first detail failure (quota, cooldown) stops further detail calls for the run.
- Retired pairs: Strava omits retired gear from `/athlete`. A linked pair absent from it is `GET /gear/{id}`-checked: `retired=true` → `stravaState: "retired"`, 404 → `"deleted"` (last known total kept); both set `retired`. The Strava retired flag is applied when the Strava state changes, so a local choice survives later syncs; a pair listed again becomes active. The periodic sync does not re-check pairs already retired/deleted (on-demand refresh does). Activities keep their shoe link. UI: active pairs (Strava primary first, then most worn) and a collapsed « Retirées (N) » group.
- Missing scope: the import answers 409 with a reconnect message; the shoes view shows a « Reconnecter Strava » hint when `integrations.strava.missingScopes` contains `profile:read_all`. Garmin has no gear endpoint: unchanged.
- Storage: migration `00008_shoe_gear` adds `shoes.strava_primary`, `strava_state` (`active` | `retired` | `deleted`), `strava_synced_at`; API fields `analyzedKm`, `distanceSource`, `stravaPrimary`, `stravaState`, `stravaSyncedAt` (read-only).

### 14.6 Admin control center
Administration (`/admin?tab=…`) is a control center: Vue d'ensemble · Utilisateurs · Tâches · Partages · Avis · Invitations · Fournisseurs · Journaux · Maintenance (scrollable tab row at every width; counts for live jobs, unread feedback, active links). Every route below is `RequireAdmin` (401 anonymous, 403 non-admin), CSRF-guarded like any non-GET `/api/*`, and state changes are audit-logged as `user_action` (actor = session user, target = path with the user, job, link or feedback id: `admin.user_sessions_revoke`, `admin.user_sync`, `admin.job_retry`, `admin.job_cancel`, `admin.share_revoke`, `admin.maintenance_purge|optimize|check`, `admin.feedback_update`). Responses never carry secrets, share URLs/tokens (8-char prefix only), job results (they may hold a private AI analysis) or ping error texts.

| Method & path | Response |
| --- | --- |
| `GET /api/admin/overview` | `AdminOverview`: build, uptime, registration mode, SQLite size / WAL / free bytes, cache backend + ping latency, job pools (`workers`, `running`, `queued`, `delayed` per `sync` / `llm` / `webhook`) + active + failed 24 h, SSE clients/users (this instance), error log count 24 h + dropped entries, `apimeter` providers (incl. `llm`), counts (users, active, admins, connected, LLM, activities, sessions, active share links, unread feedback), non-secret runtime config (allowlist: public URL, RP ID, secure cookies, session TTL, timezone, log level/format, cache backend, metrics on/off, Strava on/off, weather, private egress, sync hour). The UI polls it every 15 s. |
| `GET /api/admin/users` | `AdminUser` gains `lastSeenAt` (latest session activity), `sessions` (live), `activities`, `storageBytes` (activity + wellness blobs), `integrations` (`garmin` / `strava` / `llm`). Search, filters and sort are client-side. |
| `PATCH` / `DELETE /api/admin/users/{id}` | unchanged, plus: demoting, disabling or deleting the last active admin → 409 (store-level guard, `ErrLastAdmin`); self → 409. Disabling still revokes every session; a disabled account cannot authenticate (login 403). Deleting cascades (sessions, passkeys, domain rows, feedback). |
| `DELETE /api/admin/users/{id}/sessions` | `{revoked}`; self → 409 (use logout). |
| `POST /api/admin/users/{id}/sync` | 202 `AdminJob`: provider sync now (skips the lazy 5-min cooldown; a running sync is returned). Disabled user → 409, no provider → 412, unknown → 404. |
| `GET /api/admin/jobs?state=active\|failed\|all&limit=` (≤ 200) | `{items: AdminJob[], groups}`; `AdminJob` = `JobStatus` (`result` always null) + `userName`, `retryable`, `cancellable`. |
| `POST /api/admin/jobs/{id}/retry` | 202 `AdminJob`: replays a **failed** sync job (`sync.provider`, `sync.resync`, `sync.history`, `sync.strava_webhook`) for its owner with its stored payload; other kinds or states → 409. |
| `POST /api/admin/jobs/{id}/cancel` | queued / retry-scheduled job fails at once, a running one has its context cancelled and fails without retry (« Tâche annulée par un administrateur. »); finished → 409, unknown → 404. |
| `GET /api/admin/shares?state=active\|all` · `DELETE /api/admin/shares/{id}` | `AdminShareLink[]` (owner, label, token prefix, scope, expiry, views, `state`) · revoke (204) + drop the cached public view. |
| `POST /api/admin/maintenance/purge` | `{removed: {sessions, invitations, apiEvents, logs, jobs, shareLinks}}` (same purge as the hourly maintenance). |
| `POST /api/admin/maintenance/optimize` | body `{vacuum?: bool}`: `wal_checkpoint(TRUNCATE)` + `PRAGMA optimize` (+ `VACUUM`) → `{vacuum, before, after, durationMs}`. |
| `POST /api/admin/maintenance/check` | `stride doctor` checks (`integrity_check` + `foreign_key_check`) → `{ok, problems (≤ 50), total, durationMs}`. |

**User feedback.** « Donner un avis » (user menu at every width, command palette, Réglages footer) opens a dialog: category `bug | idea | other`, message (5–4000 chars), optional « J'accepte d'être recontacté par e-mail » with an e-mail field (accounts have no e-mail: the last consented address is remembered on the device only), and an auto-attached context shown to the user (`appVersion`, `route` = path only, `viewport`, `userAgent`). `POST /api/feedback` (user, 16 KB body, unknown fields → 400) → 201 `{id, createdAt}`; 5 per user per hour (429 `RATE_LIMITED`). Sanitized server-side: control/format characters removed, line endings normalized, context fields truncated, query strings and fragments dropped from `route`, share tokens cut to their prefix; `contactEmail` is validated and stored **only** when `contactOk` is true (DB CHECK). Messages and addresses are never logged. Admin: `GET /api/admin/feedback?status=new|read|done|all&category=bug|idea|other|all` → `{items: FeedbackItem[] (≤ 200, newest first), counts: {new, read, done, total}}`; `PATCH /api/admin/feedback/{id}` `{status?, adminNote? (≤ 2000)}` → `FeedbackItem`. Opening an unread entry marks it read; « Répondre par e-mail » is a `mailto:` link (no server-side mail).

Storage: migration `00009_admin` adds `async_jobs.payload_json` (written for the replayable sync kinds only, never for imports, LLM prompts or webhook bodies), index `async_jobs_state`, and table `feedback` (`user_id` → `users ON DELETE CASCADE`, `category`, `message`, `contact_ok`, `contact_email`, `context_json`, `status new|read|done`, `admin_note`, `created_at`, `updated_at`).

### 14.7 Pathologies
- Zones: four more canonical zones (appended, after the six 4.0 ones): `ankle` (Cheville), `forefoot` (Avant-pied / Métatarses), `hip` (Hanche / Bassin), `lower_back` (Bas du dos (lombaires)) — 10 zones. They are drawn on the body map (left/right: anterior view for ankle and hip, posterior for the lower back, plantar callout for the forefoot) and accepted everywhere a zone is (health status, Ressenti form, body entries, history, correlations, share, prompts). Correlations use measured factors only: `ankle` → D−, `forefoot` → shoe drop + shoe wear, `hip` / `lower_back` → GCT; below the sample minimum the factor stays `insufficientData` (never estimated).
- Catalog: `GET /api/health/conditions` (user) → `{ zones: [{ zone, zoneLabel, conditions: [{ key, label }] }] }`, zones in canonical order, each list closed by `{ key: "other", label: "Autre (préciser)" }`. Static per release; the backend (`analysis/conditions.go`) is the only source (the frontend reads it, the mock keeps a copy). Medically sensible, not exhaustive, e.g. ankle: entorse, fracture de cheville (malléole), tendinopathie des fibulaires, tendinopathie du tibial postérieur, instabilité chronique; knee: syndrome fémoro-patellaire, tendinopathie rotulienne, patte d'oie, lésion méniscale, entorse; ITB: syndrome de la bandelette ilio-tibiale; shin: périostite tibiale, fracture de fatigue du tibia, syndrome des loges; achilles: tendinopathie, bursite rétro-calcanéenne, rupture partielle; plantar: aponévrosite plantaire, fracture de fatigue du calcanéum; hamstrings/calves: lésions musculaires, tendinopathie proximale des ischio-jambiers; forefoot: fracture de fatigue métatarsienne, métatarsalgie, névrome de Morton; hip: tendinopathie fessière, psoas, pubalgie, fracture de fatigue du col du fémur; lower back: lombalgie, sciatique, douleur sacro-iliaque.
- Health status items gain `condition` (catalog key of that zone, optional) and `customLabel` (required when `condition = "other"`: trimmed, 1–80 characters, printable text only, rendered as text); the response adds the derived, read-only `conditionLabel` (catalog label or custom label). PUT → 400 for a key unknown or of another zone, `other` without a valid `customLabel`; a `customLabel` sent with another condition is dropped; `conditionLabel` is ignored and never stored. Both fields are omitted when empty. Changing the zone in the editor resets a catalog condition (« Autre » and its text follow).
- Focus: a condition with a dedicated preset keeps it (`patellofemoral_pain` → `knee_pfps`, `achilles_tendinopathy` → `achilles`, `medial_tibial_stress` → `shin_splints`); any other condition → `other` labelled with the condition (or the custom label); no condition → the §14.2 zone mapping.
- Prompts (activity, day, multi-session, plan): the context section adds `- **Suivi santé :** <condition> — <zone>, <côté>, depuis le JJ/MM/AAAA` per tracked zone (` ; ` separated; zone alone without condition), e.g. « Fracture de cheville (malléole) — Cheville, gauche, depuis le 12/10/2026 ».
- Storage: `health_status.zones` JSON carries the new fields (no schema change). Migration `00010_body_zones` rebuilds `body_stress_entries` (SQLite cannot alter a CHECK) with the 10-zone CHECK, copying every row and recreating its indexes; the down migration copies the 4.0-zone rows back, drops entries of the added zones and removes them from `health_status.zones` (conditions of the other zones are kept).

---

## 15. 4.2.0 — Physio/coach portal, FR/EN, dual-pool SQLite, atomic backups, Smart Workout Morphing

`VERSION` = `4.2.0`. Continuity rule: no new framework or layer; extend the existing packages (`domain`, `domainapi`, `httpapi`, `queue`, `sse`, `report`, `workout`, `observability`, `secret`, `egress`, `scheduler`) and frontend structure (`api/types.ts` is the contract, no OpenAPI). Migrations additive: `00011_*` (share portal + morph), `00012_*` (i18n, only if a column is needed). Existing routes keep their paths (`/api/share`, `/api/share/{id}`, `/api/public/share/{token}`).

### 15.1 Share portal permissions
- `share_links` gains `allow_comments`, `allow_planning`, `allow_activity_details` (bool, default false: existing links unchanged). `ShareLink`/`ShareLinkInput` gain `permissions: { comments: boolean; planning: boolean; activityDetails: boolean }` (create + `PATCH /api/share/{id}`).
- Notes (table `share_notes`: `user_id`, `share_link_id` (FK, `ON DELETE SET NULL`), `link_label` snapshot, `kind directive|remark`, `activity_id` (remark only), `author_name`, `author_role physio|coach|other`, `body`, `restrictions_json` (directive only), `valid_until`, timestamps). One directive per link (upsert); one remark per (link, activity) (upsert).
  - `ShareNote = { id, kind, linkId: number|null, linkLabel, authorName: string|null, authorRole, body, activityId: number|null, validUntil: string|null, restrictions: DirectiveRestrictions|null, createdAt, updatedAt }`, `DirectiveRestrictions = { noIntervals: boolean; maxIntensity: 'easy'|'moderate'|null; maxElevLossM: number|null; cadenceTargetSpm: number|null }`.
  - A directive is **active** while its link is neither revoked nor expired and `validUntil` is null or ≥ today.
- Public (token-scoped, no session). Same 404 for unknown/revoked/expired tokens; `403 SHARE_FORBIDDEN` when the permission is off; activities must belong to the owner and lie in the link's period window (else 404):
  - `GET /api/public/share/{token}` adds `linkId: number` (the link's own id, matched against `prescribedBy.linkId`), `permissions`, `directive: ShareNote|null`, `remark` on each `PublicActivityItem`, and when `planning`: `calendar: { planned: PlannedSession[]; events: CalendarEvent[] }` (today −7 d → +42 d).
  - `PUT|DELETE /api/public/share/{token}/directive` `{ authorName?, authorRole?, body (1–2000), validUntil?, restrictions? }` (comments).
  - `PUT|DELETE /api/public/share/{token}/activities/{id}/remark` `{ authorName?, authorRole?, body (1–1000) }` (comments).
  - `GET /api/public/share/{token}/activities/{id}` → `PublicActivityDetail` (activityDetails): metrics (pace, HR avg/max, EF, cadence, GCT, vertical oscillation, L/R balance, D+/D−, impact load, weather), laps, shoe (name, drop, km), pain (during, J+1), `remark`. `anonymize` → no GPS/polyline, no place names.
  - `POST /api/public/share/{token}/plan`, `PATCH|DELETE …/plan/{id}`; `POST /api/public/share/{token}/events`, `PATCH|DELETE …/events/{id}` (planning) — existing `PlannedSessionInput` (with `steps`) / `CalendarEventInput` shapes; dates today −7 d → +120 d; a link may only edit/delete what it created (else 403).
- `PlanSource` gains `'share'`; `PlannedSession` and `CalendarEvent` gain `prescribedBy: { linkId: number|null; label: string; authorName: string|null } | null` (columns `share_link_id` + label snapshot).
- Security: public writes require same-origin (`Origin`/`Sec-Fetch-Site` vs `STRIDE_PUBLIC_URL`), 16 KB body, unknown fields → 400, text sanitized like feedback, rate limit per token **and** IP (writes 30/10 min/token), token redacted to its prefix in logs/audit (`share.public_*` actions, actor `share:<prefix>`). Writes invalidate the 5-min public cache and publish SSE `share.updated` `{ kind: 'directive'|'remark'|'plan'|'event', linkId }` to the owner.
- Athlete side: `GET /api/share/notes?active=1` → `ShareNote[]`; `DELETE /api/share/notes/{id}` (athlete may remove any note). `ActivitySummary`/views expose `remarkCount` where cheap. UI: sober badge « Consigne kiné/coach active » on the dashboard (→ Santé & forme, which lists directives + recent remarks) and on sessions with a remark; « Prescrit par <label> » on prescribed sessions/events; share dialog gains the three checkboxes.
- AI: activity/day/multi/plan prompts, LLM analyses, plan generation/adapt and morph inject, **first** in the context, a section « Consignes kiné / coach (prioritaires) » (active directives with author/role/date/validity/restrictions + remarks on the sessions in the prompt; plan: last 14 days), third-party text quoted in a fenced block and capped. Portal shows « Vos remarques sont automatiquement prises en compte par l'assistant d'entraînement et la planification de l'athlète ».
- Portal layout (`/share/:token`, mobile first): 1. vital summary (health status + pathologies, ACWR, pain trend J+1, directive) — 2. body map + shoe/D− correlations (per scope) — 3. calendar + quick prescription (planning) — 4. recent sessions (pain, shoe, detail, quick remark). Mobile: bottom sheet, 1-tap save; desktop: inline edit.

### 15.2 Internationalisation (FR/EN)
- `frontend/src/i18n/`: `fr.ts` (source, `as const`), `en.ts` (typed from `fr`), `useI18n()` → `{ t, lang, setLang, fmt }` (interpolation `{name}`, `Intl.PluralRules`, `Intl.DateTimeFormat`/`Intl.NumberFormat`), no external library; a test enforces 100 % key parity.
- Language: `AthleteProfile.language: 'fr'|'en'|null` (null = browser). Resolution: profile → `localStorage` `stride.lang` → `navigator.language` (`fr*` → fr) → `en`. Switch in Réglages (persisted to profile + localStorage), Cmd+K, and a discreet `FR | EN` on the login screen and the public share header (localStorage only). The API client sends `Accept-Language` = active language.
- Backend: effective language = profile.language ?? `Accept-Language` (fr/en) ?? `en`, resolved at request time and carried into queued jobs; prompts, report Markdown and LLM system prompts are produced in that language (`frfmt` gains an English formatter). Public share payload labels follow `Accept-Language`. API error `code`s stay stable; the UI translates by code.
- Implementation (4.2.0): `athlete_profiles.language` (migration `00012_i18n`); `domainapi` stores the `Accept-Language` choice in the request context (user routes and `GET /api/public/share/{token}`); reports and prompts are rendered in French then localized by `report.Localize` (format-string table + `frfmt.English` number formatting; French output unchanged); a queued LLM job keeps the language of its prompt (system directive chosen from it); pathology catalog via `analysis.ConditionsIn`. Server-computed view texts (alerts, readiness, job labels) remain French in the API and are translated in prompts and the public portal only.

### 15.3 SQLite dual pool
- `store.Store` keeps `DB` as the **write** pool (`MaxOpenConns(1)`, `_txlock=immediate`) and adds `R`, a **read** pool (`query_only`, `MaxOpenConns = max(4, GOMAXPROCS)`); both with `journal_mode=WAL`, `synchronous=NORMAL`, `busy_timeout=5000`, `foreign_keys=ON`, `temp_store=MEMORY`, `cache_size=-64000`, `mmap_size=268435456`. `:memory:` → `R = DB`. Read-only paths (views, lists, search, public share, exports, admin overview) use `R`; writes and read-modify-write stay on `DB`/`Tx`. No query on `DB` may run inside a `Tx` callback (single writer connection ⇒ deadlock) — guarded by a test.

### 15.4 Atomic backups (local + S3)
- `internal/backup`: `VACUUM INTO` a temp file → gzip → `<STRIDE_DATA_DIR>/backups/stride-<UTC yyyymmddThhmmssZ>-<version>.sqlite.gz` (0600, dir 0700) + JSON sidecar `{ name, sizeBytes, sha256, createdAt, trigger: manual|daily|pre_migration, s3: { status: none|pending|synced|error, key, syncedAt, error } }` (filesystem = source of truth; works before migrations). Local retention: keep the 7 latest (pre-migration snapshots: 3 latest).
- Triggers: daily (scheduler), before applying pending goose migrations at boot (non-empty DB), manual. Uploads run as queue jobs (`backup` kind, retries/backoff); remote retention by age after a successful upload.
- `internal/backup/s3.go`: AWS SigV4 client (stdlib only), path-style and virtual-hosted (AWS, MinIO, R2, Scaleway, Garage, B2): put, list (V2), delete, head-bucket; goes through `egress` (private endpoints need `STRIDE_ALLOW_PRIVATE_EGRESS`).
- Config (admin settings): `{ endpoint, region, bucket, prefix, accessKeyId, secretKeySet: boolean, pathStyle, autoUpload, retentionDays }`; secret sealed with `secret.Sealer` (AAD `backup:0:s3_secret_key`), never returned; seeded once from `STRIDE_S3_ENDPOINT|REGION|BUCKET|PREFIX|ACCESS_KEY_ID|SECRET_ACCESS_KEY|PATH_STYLE|AUTO_UPLOAD|RETENTION_DAYS` when nothing is stored.
- Admin API (admin-only, audited): `GET /api/admin/backups` → `{ items: BackupItem[], s3: S3Config, lastSuccess: { local, s3 }, nextScheduledAt }`; `POST /api/admin/backups` `{ upload: boolean }` → `JobStatus`; `GET /api/admin/backups/{name}/download` (stream); `POST /api/admin/backups/{name}/upload` → `JobStatus`; `DELETE /api/admin/backups/{name}?remote=0|1`; `GET|PUT /api/admin/backups/s3` (empty `secretKey` keeps the stored one); `POST /api/admin/backups/s3/test` `{ config? }` → `{ ok, latencyMs, error? }`. `{name}` strictly matches the file pattern (no traversal).
- Metrics: `stride_backup_last_success_timestamp_seconds{target="local|s3"}`, `stride_backup_last_size_bytes`, `stride_backup_total{target,status="success|error"}` (documented in `docs/METRICS.md` with a staleness alert).
- UI: `/admin` « Backups & S3 » panel (config + test, Backup local / Backup local + S3, list with date, size, SHA-256, `Local`/`Synced S3`, download, push, delete).

### 15.5 Smart Workout Morphing
- Constraints for a planned session (J/J+1): active directive restrictions; pain ≥ 3/10 during or next morning in the last 48 h, or ACWR > 1.3; forecast at the session slot (Open-Meteo) > 22 °C or wind > 30 km/h. `MorphConstraint = { kind: 'directive'|'pain'|'load'|'heat'|'wind'; label; detail; severity: 1|2|3 }`.
- `GET /api/plan/{id}/morph` → `MorphPreview = { sessionId, constraints, original: PlannedSession, proposal: PlannedSession|null, changes: { field: string; stepIndex: number|null; before: string; after: string; reason: string }[] }` (deterministic rules: intervals → easy continuous when `noIntervals`/pain; volume −15 to −30 % on load/pain; target pace slowed with heat; HR/effort cues on wind; kiné cadence injected into steps). `proposal = null` without constraints.
- `POST /api/plan/{id}/morph/refine` `{ note? }` → `JobStatus` (LLM queue, streamed over SSE; `llm.done` carries `morphPreview`). `POST /api/plan/{id}/morph/apply` `{ proposal }` → `PlannedSession`; `POST /api/plan/{id}/morph/revert` → `PlannedSession`.
- `PlannedSession` gains `morph: { at: string; reasons: string[]; original: Pick<PlannedSession,'title'|'targetDurationS'|'targetDistanceM'|'targetPaceMinSecPerKm'|'targetPaceMaxSecPerKm'|'targetCadenceSpm'|'instructions'|'steps'> } | null` (planned trace kept for the cycle). `GET /api/plan/{id}/workout.fit` exports the current (morphed) steps.
- UI: « Adapter la séance » on today's/tomorrow's session (dashboard hero + planning) only when constraints exist → before/after diff, optional « Affiner avec l'IA » (when an LLM is configured), « Valider » then « Télécharger .FIT »; morphed sessions show a small « Adaptée » marker with revert.

### 15.6 Backend notes (4.2.0)
- Public share: the existing `GET /api/public/share/{token}` keeps `410 GONE` for an expired link (4.0 behaviour); every portal route (`…/directive`, `…/remark`, `…/plan`, `…/events`, `…/activities/{id}`) answers the same 404 for unknown, revoked or expired tokens. Portal writes also need the usual `X-Stride-CSRF: 1` header; a foreign or missing `Origin`/`Sec-Fetch-Site` is `403 FORBIDDEN`. `PublicActivityDetail` is flat: `{ id, date, startLocal, name, sport, typeLabel, distanceM, durationS, avgPaceSecPerKm, gapSecPerKm, avgHR, maxHR, efSpeed, avgCadenceSpm, gctMs, verticalOscillationCm, balanceLeftPct, balanceRightPct, elevGainM, elevLossM, impactLoad, trainingLoad, weather: { temperatureC, apparentC, humidityPct, windKph, description }|null, laps: { index, label, durationS, distanceM, paceSecPerKm, avgHR, avgCadenceSpm, gctMs, elevGainM, elevLossM }[], shoe: { name, dropMm, distanceKm }|null, painDuring, painNextMorning, remark }`. A `source` sent by the portal is ignored (forced to `share`).
- `GET /api/share/notes?active=1` = active directives then the remarks of the last 30 days; without `active`, every note (latest 200).
- Morph: `GET …/morph` outside J/J+1 → 400; `refine` → 202 `JobStatus` (kind `llm.morph_refine`; 412 without LLM), `llm.done` `{ kind: 'morph_refine', ref: '<planned session id>', morphPreview }`; `revert` of a plain session → 409. `changes[].field` ∈ `title|targetDurationS|targetDistanceM|targetPace|targetCadenceSpm|steps|steps.note|instructions`; `apply` only takes the morphable fields of `proposal` (date, sport and links stay).
- Backups: jobs `backup.run` / `backup.upload` (group `backup`, 1 worker); `POST /api/admin/backups` and `…/{name}/upload` → 202 `JobStatus`, 412 when S3 is not configured; daily snapshot at 03:00 local; `X-Checksum-Sha256` on downloads.

---

## 16. 4.3.0 — Per-user MCP server

Each user can expose their own data to AI assistants (Claude and others) through the Model Context Protocol, using the official Go SDK (`modelcontextprotocol/go-sdk` v1.8.0) on the **Streamable HTTP** transport, **stateless**. The endpoint is `POST /mcp/{token}` — no session, no CORS: the token **is** the authorisation. Implementation in `internal/mcp` (token lifecycle in `Service`, public handler in `server.go`, tools in `tools.go`, account API in `handlers.go`) and migration `00013_mcp_access`.

### 16.1 Public route `POST /mcp/{token}`
- Stateless: only `POST` is served; `GET` / `DELETE` → 405 + `Allow: POST`. The SDK's localhost DNS-rebinding protection stays enabled; request cancellation propagates (a disconnected client stops the in-flight tool call); body capped at 16 MiB.
- Uniform 404 `NOT_FOUND` for unknown, revoked or disabled tokens and for malformed paths (`/mcp/` alone, nested paths): the subtree never falls through to the SPA and never reveals whether a token is valid.
- Cross-origin protection (Go 1.26 `http.CrossOriginProtection`): `Sec-Fetch-Site: cross-site` or a foreign `Origin` → 403. No CORS headers on `/mcp`.
- Rate limits (in-memory, `internal/ratelimit`): **100 requests / 10 min per token** (key = hex SHA-256 of the token) and **200 requests / 10 min per client IP** → 429 `RATE_LIMITED`.
- The resolved user is injected in the request context (`auth.WithUser`); a tool handler never takes a user id from its arguments. Language of the tool descriptions and errors: profile.language ?? `Accept-Language` ?? en (two pre-built fr/en tool instances, shared).

### 16.2 Tokens & account API
- Token = 256 bits from `crypto/rand`, `sm4_` + base64url (47 chars). Only the SHA-256 is stored (`mcp_access`); the plaintext exists exactly once, in the answer of `POST /api/mcp/token`. `tokenPrefix` = `sm4_` + 4 chars is the only part shown again (account screen, logs, audit).
- `PUT /api/mcp {enabled}` toggles access; **disabling keeps the token** (re-enabling reuses it). Generating a new token **revokes** the previous one immediately. `lastUsedAt` is refreshed with a 5-min minimum interval.
- **Effective quota in the status (4.5.0, issue #136)**: `GET /api/mcp` also returns `quota` — the effective public-route quota of §16.4 (defaults when the row is unreadable), read on every request. The account card's footnote quotes the real limits instead of a hardcoded sentence (« limités à {tokenLimit} requêtes par {tokenWindow} minutes », singular/plural on the minutes, `fmtNumber` grouping for large values), so a quota change in the admin panel is picked up on the next fetch. The card subtitle typo « Exposes tes données » → « Expose tes données » is fixed in the same pass.
- Audit: public action `mcp.request` (actor `mcp:<prefix>`, no user id — the token is the actor) and session actions `mcp.toggle`, `mcp.token_generate`, `mcp.token_revoke`. Logs redact the token to its prefix.

### 16.3 Tools (v1, 13)
- Read: `planning` (week plan + totals), `day` (day view), `activities` (list; `limit` default 30, `from`/`to`, `sport`; response `{items, hasMore}`), `activity` (full detail; unknown id = in-band tool error), `health` (health status + zones), `cycles` (plans → phases → weeks, planned vs actual), `profile` (athlete profile, thresholds, pace targets).
- Write: `plan_create`, `plan_update`, `plan_delete`; `event_create`, `event_update`, `event_delete` (kinds `fatigue`, `pain`, `illness`, `unavailable`, `forced_rest`). Same domain services and validation as the REST routes, including `Normalize()` (`endDate` ← `startDate`, `severity` ← 2). `plan_create` / `plan_update` also take `steps` (§19.1).
- Input schema: the write tools of planned sessions use an explicit schema (`internal/mcp/steps.go`) instead of the inferred one, because the recursive `workout.Step` cannot be inferred; the `steps` property is an array of `$defs/step` and the definition documents every step field (type, duration, distance, repeat, sub-steps, target, note) with its allowed values. Every other property is still inferred from the handler struct, and a test pins that the two never drift.
- Dated tools (`day`, `planning`): the `date` argument is optional — empty or blank resolves to **today in the owner's timezone** (an empty string is never propagated to the Garmin API or SQLite), and a value that is not `YYYY-MM-DD` comes back as an in-band `isError` naming the expected format.
- Argument validation failures come back **in-band** in the tool result (`isError`), not as transport errors.
- Output schema: tools whose view embeds planned sessions (`planning`, `day`, `activities`, `activity`, `cycles`, `plan_create`, `plan_update`) declare no JSON output schema — the recursive `workout.Step` type is rejected by jsonschema generation; the SDK then omits the schema while still returning `structuredContent` plus the text fallback. The other tools keep their typed output.
- Isolation: every tool is scoped to the resolved user; another user's data is unreachable (covered by tests).

### 16.4 Usage & admin quota (4.3.0)
- Migration `00014_mcp_quota`: one row (`id = 1`) holding the public route's quota — `token_limit`/`token_window` and `ip_limit`/`ip_window`, **windows in minutes** (same unit in DB, JSON and UI). Defaults: 100 / 10 and 200 / 10 (the original constants).
- The quota is admin-tunable and persisted: `PUT /api/admin/mcp/quota {tokenLimit, tokenWindow, ipLimit, ipWindow}` (admin, audit `mcp.quota_update`). Input is **clamped, not rejected** — limits to 1–100 000, windows to 1–1440 — and the stored row is returned (200). The change applies live (`ratelimit.Limiter.SetConfig` restarts both windows, so no counter outlives the settings it was counted under) and survives a restart (`Server.New` loads the row, falling back to the defaults when unreadable).
- `GET /api/admin/mcp/usage` (admin) → `{ quota, tokens[], requests24h, denied24h, updatedAt }`: one `tokens[]` entry per stored token (`prefix`, `userId`, `displayName`, `enabled`, `lastUsedAt`, `used` in the current window, `denied24h`, `requests24h`), joined with `users`.
- Usage counters are in-memory (like the provider meter): rolling 24 h in 5-minute buckets (288 slots) per token for requests and denials, plus a global denial ring. A 429 is counted for the token when `Resolve` succeeds (the IP limit is hit or the token window is full), globally only for unknown tokens. `used` never exceeds the limit. Stale entries (revoked tokens whose 24 h rolled out) are swept at each snapshot, so the map is bounded by the live token count. Counters reset on restart; `lastUsedAt` (DB) is the durable signal.
- Frontend: admin tab **MCP** — usage card (24 h totals, per-token table with the current-window load bar) and quota card (four numeric fields, dirty state, save → PUT + toast), both polling every 15 s.

## 17. 4.4.0 — One-time recovery codes, complete and CI-checked `.env.example`

### 17.1 Recovery codes (passkeys-only last resort)

Signing in is passkey-only (WebAuthn discoverable login, no username, no password). Multiple passkeys per account already exist (add/rename/delete, last-one guard 409). The remaining hole: losing **every** device while the passkeys are not backed up at the authenticator (cloud lost, manager wiped) = account lost. One-time **recovery codes** close it (GitHub/Google pattern). See §3.1 for the routes and the full behavioural contract:

- **Codes**: 10 per set, each 10 random bytes → base32 (RFC 4648, no padding) → 16 uppercase chars shown `XXXX-XXXX-XXXX-XXXX` (case and dashes normalized on input). Stored as **SHA-256 of the normalized code only** (`recovery_codes`, migration `00015`, `ON DELETE CASCADE`); the plaintext is shown once at generation and never recoverable.
- **Single use**: consumption is an atomic `UPDATE … WHERE used_at IS NULL` with an affected-rows guard, so a code can never be replayed even under concurrency.
- **Regeneration**: `POST /api/me/recovery` atomically voids the remaining set and stores a fresh one (single transaction).
- **Login**: `POST /api/auth/login/recovery` is public, rate-limited per client IP (10 attempts/min → 429), issues a session like a passkey login. Disabled account → 403 (code still consumed). Unknown/used/malformed → 401.
- **UI**: login screen gets a discrete « code de récupération » path under the passkey button; Settings gains a **Codes de récupération** section (remaining count, generate/regenerate with a confirmation, the fresh set with per-code copy and a « shown once » warning). i18n FR/EN (mirror test) + mock routes + Playwright recovery-login smoke.
- The passkeys « last credential » 409 guard is intentionally **unchanged**: a recovery code never modifies the passkey rules.

### 17.2 `.env.example` complete and drift-proofed

`.env.example` had drifted badly from the code (dead vars `APP_AUTH_*`/`PORT`/`DATA_DIR`/`STRIDE_BIND|PORT|VERSION`, wrong compose names, and missing the bulk of the 4.x surface: `STRIDE_METRICS_ADDR`, all `STRIDE_S3_*`, `STRIDE_MASTER_KEY(_PREVIOUS)`, `STRIDE_DATA_DIR`/`STRIDE_DB_PATH`, `STRIDE_VALKEY_URL`, `STRIDE_REGISTRATION`, `STRIDE_SESSION_TTL`, `STRIDE_WEBAUTHN_*`, `STRIDE_SYNC_*`, `STRIDE_GARMIN_BASE_URL`, `STRIDE_STRAVA_*`, `STRIDE_WEATHER`, `STRIDE_ALLOW_PRIVATE_EGRESS`, `NO_COLOR`, `ENV_FILE`).

- **Rewritten**: every variable read by the binary is documented with its default, grouped and commented; the docker-compose block uses the real names read by `compose.yml` (`STRIDE_HOST_BIND`, `STRIDE_HOST_PORT`, `STRIDE_TAG`, `STRIDE_UID`, `STRIDE_GID`).
- **CI test « toute variable lue est documentée »** (`internal/env`, picked up by the existing backend `go test ./...` job, no `ci.yml` change): parses every non-test `*.go` file for `os.Getenv`/`os.LookupEnv` string literals plus the first-arg `get()`/`decodeKey()` reads in `env.go`, and fails listing any read var missing from `.env.example`. One-directional (compose-only vars stay documentable); test-only vars (`STRIDE_LIVE_ENV`, `live` tag) are out of scope. Adding a new read var without documenting it now turns CI red.

## 18. 4.5.0 — MCP call journal, structured log and the `DEADLINE_EXCEEDED` fix

### 18.1 Fix of the concurrent `DEADLINE_EXCEEDED` (issue #135)

Root cause: with `modelcontextprotocol/go-sdk` **v1.8.0**, the answer of a **stateless POST** on the Streamable HTTP transport is an SSE stream that writes **no byte until the tool call completes** — the early `: ok` flush only fires for the standalone GET stream, and v1.8.0 has no keep-alive on the POST path at all. Any client or proxy enforcing a first-byte or idle timeout (Claude ≈ 20 s) then aborts the connection and the client reports `DEADLINE_EXCEEDED` with nothing received. A slow tool (Garmin sync, LLM report) is enough to cross the timeout; concurrent calls on the same connection make it systematic.

Fix — the two parts are both needed:
- **SDK bump `v1.8.0` → `v1.9.0-pre.1`** (2026-11-17), the only release fixing upstream **go-sdk#1155** — v1.8.0 is still `@latest`, so the pre-release is pinned exactly. Protocol version list and every other `StreamableHTTPOptions` field Stride uses are identical between the two versions.
- **`StreamableHTTPOptions.StreamKeepAlive = 5 * time.Second`** (Go constant, no new env var): the SDK's 30 s default is *longer* than the 20 s client timeout, so it must be set explicitly. The keep-alive emits an SSE comment line; on a POST stream the first comment (which commits the headers) goes out after at most one silent interval, so the client sees bytes well before it gives up. Semantics: `0` = SDK default (30 s), negative = disabled; a failed keep-alive write closes the stream.
- The local concurrency regression (`TestConcurrentPlanCreatesComplete`) already passed on v1.8.0: the bug needs a client/proxy that actually enforces a first-byte timeout, which the test harness is not. The test guards the SDK's batch answer envelope and the journal instead.

### 18.2 Call journal (issue #135)

In-memory journal of the public route's calls — the admin panel's « Journal des appels ». `internal/mcp/journal.go`; in-memory by design (no extra SQLite load on the public path; resets on restart, like the usage counters of §16.4).

- **Ring of 300 calls** (spec: 200–500), mutex-guarded, most recent first. Every call of `POST /mcp/{token}` is recorded: JSON-RPC ones (method, tool, sanitized arguments, request id) and the refusals answered before the SDK handler — 429 per IP and per token (the token is resolved when possible, for attribution), 404 unknown token, 400 / 413 body errors. Non-POST 405s are not recorded.
- **One `Call` =**: `at`, `tokenPrefix` (never the full token — the request body is never stored, only the sanitized summary), `userId` / `userName` (0 / "" for an unknown token), `method`, `tool`, `argsSummary` (string values truncated at 120 chars, whole summary capped at 1024 bytes), `status`, `rpcErrorCode` / `rpcError` (capped at 500; parsed from the answer — JSON, JSON array, or the SSE `data:` lines — a `result` embedding `"error":null` is not an error), `durationMs`, `inFlight` (concurrent calls when the call started), `clientCanceled` (the request context died mid-call).
- **Panic = clean error, never a hung connection**: the SDK handler runs under a dedicated `recover()` (re-throwing only `http.ErrAbortHandler`); the in-flight call id gets a JSON-RPC `-32603 internal error` — SSE event when the stream already committed, JSON otherwise — and the call is journaled as an error.
- **Admin route** `GET /api/admin/mcp/journal` (admin, like usage/quota) → `{ inFlight, entries[] }`: the live concurrency gauge plus the ring, most recent first.
- **Rolling 24 h failed-call counter** (same 5-minute-bucket pattern as the usage meter): a call is a failure when its status is ≥ 400, or it carried an RPC error, or it panicked, or the client cancelled it. Exposed as `errors24h` in `GET /api/admin/mcp/usage` — the « Erreurs / Timeouts · 24 h » KPI of the admin panel.

### 18.3 Structured log & admin panel (issue #135)

- **One structured log line per call**: `applog` entry, category `user_action`, action `mcp.request`, target `/mcp/<prefix>…` (token redacted to its prefix), message `mcp:<prefix> · <method> [tool] · <detail>` (FR). Level: `info` for a normal call, **`warn`** for a 429, a client cancellation or a call slower than 2 s, **`error`** for a panic, an HTTP ≥ 500, an RPC error or any other 4xx. New `applog.LevelWarn` constant (`app_logs.level` is already TEXT — no migration); the admin Logs view and its severity filter now accept `warn`.
- **Audit**: the `/mcp/` branch of `withAudit` is removed — a public call is no longer double-written to the audit trail: the journal entry + the structured line replace it. The session actions (`mcp.toggle`, `mcp.token_generate`, `mcp.token_revoke`, `mcp.quota_update`) are unchanged.
- **Frontend**: the admin MCP tab gains a **Journal des appels** card — the 300 last calls (time, user + token prefix, method + tool, truncated arguments, status with the RPC error, duration in warn color above 2 s, concurrency) with client-side filters (tool, failed/slow, free text over user/token/method/arguments/error) and the « Erreurs / Timeouts · 24 h » KPI tile in the usage card; both poll every 15 s. i18n FR/EN (mirror test) + mock routes.

## 19. 4.6.0 — Structured steps in the MCP write tools

### 19.1 `steps` in `plan_create` / `plan_update` (issue #139)

An assistant could create a session (`3 × 10 min` at `5:05–5:15/km` with `1 min 30` of walking between blocks) but only as free text in `instructions`: the row was stored with `"steps":[]`, the session card showed *« No detailed structure »*, the Structure editor opened empty and the `.FIT` export or the watch push had no native blocks. The write tools now carry the domain's step model, exactly the one the Structure editor and the exporters read.

- **Input (`internal/mcp/steps.go`)**: `planFields` gains `steps *[]workout.Step` (JSON `steps`), so `plan_create` and `plan_update` take the very model of `workout.Step` — `type` (`warmup|run|interval|recovery|rest|cooldown|repeat`), `durationS` or `distanceM`, `repeat` + nested `steps` for a block, `target {kind, min, max}` (pace in s/km, hr bpm, cadence spm, power W) and `note`. The pointer keeps the patch semantics of §16.3: `steps` absent leaves the structure untouched, `[]` clears it, `null` clears it too (JSON null empties a pointer field).
- **Explicit schema**: `jsonschema-go` rejects the recursive `workout.Step` (it panics on the cycle), so the two tools get a hand-written schema instead of the inferred one: the schema of the handler struct is still inferred (names, types, required fields, descriptions from the `jsonschema` tags), then `properties.steps` is pointed at `$defs/step`, itself recursive through its own `steps` property. The definition carries every field with its allowed values (the step and target enums are `workout.StepTypes` / `workout.TargetKinds`, not copies) and the descriptions state the ranges and the `durationS` XOR `distanceM` rule. Two tests pin the contract: `$defs/step` documents exactly the JSON fields of `workout.Step` (and `target` those of `workout.Target`), and the root properties are exactly the writable fields of the handlers.
- **Validation stays in the domain**: the schema enforces the shape (JSON types, required step `type`, the enums); ranges, exclusivity and the nesting depth (`repeat` in `repeat` in `repeat` → 400) are the domain's `workout.Validate`, so the errors come back in-band, localized, naming the faulty step (« étape 1.2 : … »).
- **Persistence and reuse**: no other change — `CreatePlanned` / `PatchPlanned` already normalize and validate `steps` (§11.3), store them in `planned_sessions.steps`, return them from `plan_create` / `plan_update` / `planning` / `day`, render them in the Structure editor and encode them in `GET /api/plan/{id}/workout.fit` and the Garmin push. A call without `steps` behaves exactly as before.
- **`steps` vs `instructions`**: the tool descriptions, the server instructions and the property descriptions now say it — effort blocks, repetitions and recoveries live in `steps`; `instructions` keeps what cannot be modelled as steps (warm-up or strength work before leaving, shoes, guardrails, cool-down). The account card's tool list (i18n FR/EN) mirrors the new wording.

### 19.2 `Patch` decodes into a fresh value (found with issue #139)

Replacing a structure through `plan_update` failed with « étape 2 : seules les étapes « repeat » peuvent contenir des sous-étapes ». Root cause, in `athlete.Patch` (shared by every REST and MCP write): the merged JSON document was decoded **in place** into the current struct, and `encoding/json` reuses the elements of an existing slice — a struct element only gets the fields present in the JSON, so a block replaced by a plain step kept the sub-steps (and `repeat`) of the previous block. The merged document is now decoded into a fresh value and assigned, which is plain JSON semantics: a provided key replaces its whole subtree, the absent keys keep their merged value. `internal/athlete/records_test.go` pins it (shorter `steps` array, `null` clearing, unknown vs read-only keys).

## 20. 4.6.1 — Navigating between sessions older than the loaded page

Reported in 4.1 and still broken in 4.6.0 (issue #21): a session opened from the calendar (`?s=<id>`) left both arrows of the session header disabled. The header derived its neighbours from `useActivities()`, which only holds the pages scrolled so far: for a session older than the first page `items.findIndex` returned -1, so `older` and `newer` were null and `SessionHeader` disabled both buttons.

- **Loaded pages first, exact queries outside them** (`views/dashboard/navigation.ts` + `useSessionNeighbours`): when the selected session is in `items`, nothing changes and no request is added. When it is not, each side gets its own exact query instead of a guessed window:
  - `older`: `GET /api/activities?to=<date>&before=<startLocal>&limit=1` — the list pages newest-first only, so the `before` cursor is the only exact way down, whatever the gap before the session;
  - `newer`: `GET /api/activities?from=<date>&to=<today>` (paged, `useActivitiesInRange`), whose entry just above the session is the neighbour — exact even across a long break, which a ±7-day window could not see. A truncated listing (`hasMore`) leaves that arrow disabled rather than jumping over sessions.
- **Edges**: a session at either end of the history keeps the missing arrow disabled; two sessions of the same day keep the server order (`start_local DESC, id DESC`); the arrows stay usable while the window loads (disabled until the neighbour is known).
- **Drive-by**: the picker's desktop calendar header was hardcoded French (« Calendrier ») in the English UI — now `t('session.picker.calendar')`.
- **Tests**: `navigation.test.ts` (pure neighbour logic: both sides, edges, same-day order, truncated listing), `DashboardView.test.tsx` (a session from `?s=` outside the first page: arrows enabled and jumping to the exact neighbours, oldest session with a single arrow, nominal case with no extra query), `e2e/session-navigation.spec.ts` (picker → April 2025 marathon → next → URL `?s=` and content change → back).

## 21. 4.7.0 — Notification centre (issue #142)

Issue #142. The alerts (biomechanics, load, readiness) and the physio/coach portal writes were only ever toasts: 6 s on screen, gone for good, and two tabs disagreed. A user who came back an hour later had no way to know that the physio had left a directive or that the knee alert had fired. 4.7.0 gives those messages a persistent, per-user history behind a bell in the header.

### 21.1 What feeds it

| Family | Source | Kind / level | Deduplicated on |
|---|---|---|---|
| Analysis alert | `emitAlert` (biomechanics, ACWR, readiness, sessions) | `alert` / `info`, `warn` or `error` | activity date + activity + alert code |
| Physio/coach directive | `PUT /api/public/share/{token}/directive` | `share` / `info`, `code` = `directive` | share link + kind |
| Session remark | `PUT /api/public/share/{token}/activities/{id}/remark` | `share` / `info`, `code` = `remark` | share link + session + kind |
| Prescribed session / constraint | portal plan & event writes | `share` / `info`, `code` = `plan` / `event` | share link + session/date + kind |
| Release available | release checker (§22, issue #143) | `update` / `info`, `code` = version | version |

Writes happen **in the domain**, next to the toast, so the history exists with no tab open. A write on an existing dedup key updates the row and keeps its read state (editing the same directive twice stacks nothing). Deliberately out of scope: synced activities, background jobs, the acknowledgement toasts (« profil enregistré ») — the centre is a notification history, not a log; the log stays in Admin › Logs.

### 21.2 Storage and API

- **Table** `inbox_entries` (migration `00016_inbox`): `user_id`, `kind`, `level`, `title`, `code`, `detail`, `url`, `activity_id`, `date`, `dedup_key`, `created_at`, `read_at`; unique index on `(user_id, dedup_key)` for the upsert, index on `(user_id, created_at DESC, id DESC)` for the list.
- **Language neutrality**: `title` stores an English fallback and `code` the stable identifier (`directive`, `remark`, `CADENCE_LOW`, `4.7.0`); the interface localizes from the code (§15.2 rule already used by the toasts). Alert texts keep the analysis wording — they are already written in the user's language.
- **`GET /api/inbox?limit=&before=`** → `{items, unread, hasMore}`; the cursor is the entry id, resolved server-side to `(created_at, id)`, so a page never skips or repeats a row; `limit` is clamped to 1..200 (default 50).
- **`POST /api/inbox/read {ids}`** and **`POST /api/inbox/read-all`** → `{unread}`. Both are scoped by `user_id`: marking another user's entry is a no-op, not an error (no existence leak), and marking twice is idempotent. There is no delete route — the history is only bounded by retention.
- **SSE**: `notification.created` (a write happened) and `notification.updated` (the unread count changed), published on the per-user channel like `alert.triggered`. The toast remains the immediate signal; the counter and the panel follow the events.
- **Separate from `/api/notifications`**, which manages the external Discord / Telegram / webhook targets: distinct route prefix, query key and vocabulary.

### 21.3 Retention

`GET /api/admin/settings` gains `inboxRetentionDays` (default 10, clamped 1..365) and `inboxRetentionMax` (default 50, clamped 1..200). `PUT /api/admin/settings` clamps them instead of rejecting, treats a missing field as « keep the current value » (so the panel that owns one setting never resets the others) and purges the surplus immediately; `PurgeInbox` also runs on the daily maintenance pass. The admin panel exposes it in **Admin › Instance** (the « Centre de notifications » card), not in Invitations: it has nothing to do with who may join.

### 21.4 Interface

A bell in the header (before the search button) with an unread badge capped at `99+`; the panel is the drawer variant of `Dialog`, grouped by day (« Aujourd'hui », « Hier », then the long date), one line per entry with a level icon, the localized title, the physio label and the session date, and the read/unread styling. Clicking an entry marks it read and opens its session (`/activities/{id}`), the health page for a directive, GitHub for a release. « Mark all as read » empties the badge; the history stays. The empty state explains what will land here.

### 21.5 Tests

- `internal/domain/inbox_test.go` — upsert and dedup keys, read state preserved on update, retention bounds, purge, kind/level mapping, truncation.
- `internal/domainapi/inbox_test.go` — end to end through the API: portal writes feed the centre with code/target/date, another user sees and marks nothing, read is idempotent, no anonymous access, no delete route. It decodes every list into a fresh value: `json.Unmarshal` reuses the elements of an existing slice, and the `omitempty` pointer fields (`activityId`, `date`, `readAt`) would otherwise keep the previous answer — the same trap as §19.2, on the test side.
- `internal/admin/admin_test.go` — clamping, persistence, a retention-only `PUT` leaves the registration mode alone.
- `frontend/src/components/layout/NotificationBell.test.tsx` — badge, grouped history, localized share titles, navigation and read marking, « mark all », empty state.
- `frontend/src/views/admin/InvitationsPanel.test.tsx` — stored values, range validation before the call, saved values.
- `frontend/e2e/inbox.spec.ts` — real browser: bell and badge, panel content, entry → session, mark all, admin retention card.

## 22. 4.7.0 — Detecting a new release (issue #143)

Ships in the same release as §21. Issue #143. A published release was invisible: the version only showed in the admin overview and in `GET /api/version`, so a self-hoster could run a two-month-old binary without knowing. The banner and the notification are for administrators only — they are the ones who deploy.

### 22.1 Server-side check (`internal/updates`)

`GET https://api.github.com/repos/aguyonp/stride/releases/latest` through `egress.Client` (§6), so a redirect or a forged answer can never reach a private address:

- `User-Agent: stride-update-check`, `Accept: application/vnd.github+json`, 5 s timeout on the request and 64 KiB on the body.
- A **constant** repository (`aguyonp/stride`): the link shown to the user is built from it and the validated tag, never from `html_url` — an arbitrary URL is neither followed nor displayed.
- **SemVer comparison** (`version.go`): `4.7.0` and `v4.7.0` are accepted, a missing patch means `.0`, and `available` is true only when the stable release is **strictly newer**. Pre-releases, drafts, build metadata (`4.8.0-rc1`) and a running version ahead of the release announce nothing. A build without a version (`dev`, the default of a local build) reports the release but never announces it.
- **Cache**: one answer for the whole instance, success 6 h, failure 1 h, refreshed under a mutex — the first caller pays the request, the others read the cache. A network error, a 403/429 or a malformed payload is never an error for the caller: no banner, no notification, one log line, retry after the failure TTL.
- « No stable release yet » (only pre-releases published) is an answer, not a failure: cached like a success.

### 22.2 `GET /api/updates` (administrators)

`RequireAdmin`; a regular user gets `403`, an anonymous caller `401`, and neither triggers a GitHub call. Answer: `{current, latest, available, url, publishedAt, checkedAt, notificationId, read}` plus `disabled` when the check is off.

When a release is newer, the route also **writes the notice for every active administrator**, not only for the caller: each admin finds it on the next load from any device. It is one entry per admin and per version — `dedup_key = update:<version>` — and re-checking **keeps the read state** (a release notice is not news again, unlike an alert), so the banner does not come back after being read. Reading it from the centre, on the device or in the notification panel, hides the banner; the entry stays in the history.

### 22.3 The admin switch

`updateCheckEnabled` (default `true`) in `GET/PUT /api/admin/settings`, with its card in **Admin › Instance**, alongside the retention and the bridge: the Invitations tab keeps registration and invitations only. Disabled: no request, no notice, no banner — the route answers `{current, available: false, disabled: true}` without touching the network. No environment variable was added, so `.env.example` is unchanged (§17.2 stays green).

### 22.4 Interface

A banner in `AppShell`, under the header and above the content, only for `role === 'admin'` and only when `available && !read`: the release number, the running version, « See on GitHub » (`target=_blank`, `rel=noopener noreferrer`) and « Later ». Opening the release marks the notice read and hides the banner; « Later » hides it for the visit (`sessionStorage`), keeps the notice unread, and a later release shows up again.

### 22.5 Tests

- `internal/updates/updates_test.go` — version rules (patch/minor/major, equal, older, `v` prefix, pre-release, build metadata, two parts, `dev`, garbage), newer/equal/older/dev through `httptest`, `prerelease: true`, 403/429/5xx/malformed/empty tag, the success and failure TTLs, recovery after a failure, and the constant URL (a `html_url` pointing elsewhere is ignored).
- `internal/admin/updates_test.go` — `401`/`403` without a GitHub call, the full status for an admin, the entry written once per admin and per version, read state kept on re-check, the switch stopping everything, a dev build announcing nothing, GitHub down answering `200` without a banner.
- `internal/domain/inbox_test.go` — dedup by version, read state preserved on rewrite, a newer version becoming unread again.
- `frontend/src/components/layout/UpdateBanner.test.tsx` — banner and wording for an admin, « See on GitHub » marking read, « Later » hiding without reading, hidden once read or up to date, no call at all for a regular user.
- `frontend/src/views/admin/InvitationsPanel.test.tsx` — the switch and its two states.
- `frontend/e2e/updates.spec.ts` — real browser: the banner, the recorded `window.open` URL, « Later », the notice in the centre, and the switch silencing the banner.

## 23. 4.7.0 — Experimental Garmin Connect bridge (issue #147)

The Personal API (§11.3) is read-only: `/workout-service/*` answers 404/403, so a session could only reach the watch with a cable. 4.7.0 removes the cable without pretending the problem is solved: the **.FIT import through Garmin Connect Web** becomes the documented path, and an **experimental, off by default, isolated gateway** to Garmin's *web* API (the one the official app uses) can push a workout directly for the users who accept the risk.

### 23.1 One switch, in the admin panel

`garminBridgeEnabled` (default `false`, **Admin › Instance**) is the only switch. There is no environment variable: an operator who can reach the admin panel does not need a second, hidden switch that requires an edit of `.env` and a restart — and a variable missing from a deployment used to hide the whole feature from the interface, silently.

Off means off: the routes answer `409` (`CONFLICT`, « désactivé par l'administrateur »), `/api/garmin/bridge` answers `{available: false}`, no caller may build a request and the settings card is not rendered. On: the card appears for every user, whether or not they configured the Personal API key — the bridge signs in with their own Garmin account, it does not depend on that key.

Decommissioning remains a code change: delete `internal/garminbridge/`, its five routes and the `garmin_web` rows.

### 23.2 The gateway (`internal/garminbridge`)

A narrow interface — `Login`, `LoginMFA`, `Status`, `Push`, `Disconnect` — and no import of `domain`, `store` or `workout`: the caller converts a planned session into the Garmin workout JSON (`workout.GarminWorkout`, the same payload the Personal API already used), the gateway only speaks HTTP and OAuth.

- **Sign-in**: `sso.garmin.com` form (CSRF field, then `username`/`password`), the service ticket read from the answer or from the redirect that follows, then the OAuth1 → OAuth2 exchange at `connectapi.garmin.com/oauth-service/oauth/exchange/user/2.0` (HMAC-SHA1, the public consumer credentials the open-source Garmin clients ship). Verification is two calls: the social profile and a one-row workout listing.
- **MFA**: an answer carrying a code challenge (`verifyMFA`, `MFA_REQUIRED`…) returns `ErrMFARequired` and keeps the SSO session **in memory** for five minutes — cookies are never persisted, so a restart during the code simply asks to start over. A wrong code keeps the challenge open; the session is consumed on success.
- **Blocked**: `429`, `427` and the CAPTCHA pages map to `ErrBlocked`. The gateway never retries in a loop: the UI says so and points at the .FIT.
- **Tokens**: only the OAuth tokens are persisted, sealed by the caller with the existing per-user AAD (`garmin_web:<uid>:token`), in their own `provider_connections` row — never in the Personal API fields. The password lives for one request: not stored, not logged, not echoed; a test asserts the sealed blob and the answer contain neither.
- **Refresh**: a `401` replays the exchange with the stored OAuth1 token once; a failure is `ErrExpired`.
- **Push**: `PUT` when the workout id is known (or when a workout with the deterministic name exists), `POST` otherwise, then the schedule. The name is `«title» · Stride <sessionId> · <date>`, so a second push updates instead of duplicating. A schedule failure is reported, not hidden.

The endpoints, the consumer credentials and the HTTP client are injectable: the whole flow is tested against `httptest` fixtures, and the CI never reaches Garmin. The live HTML is the one thing fixtures cannot prove — hence the experimental label, the off default and the .FIT fallback.

### 23.3 The publisher seam (`domain`)

`WorkoutPublisher` has two implementations: `personal` (the historical path, which keeps `garmin.status = "unsupported"` and its 24 h capability cache) and `web` (the gateway). The domain picks the bridge when it is switched on **and** the user has tokens; otherwise the Personal API, unchanged. Per-user rules:

- one push at a time (`singleflight` on the user), so a double click cannot create two workouts;
- a breaker: three blocked or expired attempts within ten minutes pause the bridge for fifteen, answering `429` without touching the network; a successful push or a disconnect resets it;
- the recorded outcome is what answers the API (`pushed`, `unsupported`, `failed`), reloaded after the attempt.

`POST /api/plan/push-today` sends the sessions of a day that have steps and are not pushed yet, serially, under the same lock, and answers one result per session (`pushed`, `skipped` with « déjà envoyée » / « pas d'étapes », `failed`, `unsupported`).

### 23.4 Notifications

Expired session, blocked login and refused push write an entry in the notification centre (§21) with `kind: garmin_bridge`, level `warn`, a stable `code` (`GARMIN_BRIDGE_EXPIRED`, `…_BLOCKED`, `…_PUSH`, `…_MFA`) and a link to the settings. A wrong password or a wrong code stays in the form: it is not history.

### 23.5 Interface

- **Settings › Integrations › Garmin** gains a card « Envoi de séances (expérimental) », rendered **whether or not the Personal API key is configured** — the bridge signs in with the user's own Garmin account and does not depend on that key. The warning is readable **before** any field exists; « Activer le pont » opens a consent block (unofficial API, suspension risk, one-shot password, .FIT fallback), then the e-mail/password form, then the code step, then the state (`disconnected | mfa | connected | expired | blocked`, plus « en pause » when the breaker is open) and « Déconnecter ». The card is not rendered when the administrator has not opened the bridge (`available: false`).
- **Planning**: « Envoyer les séances du jour » appears only with a connected bridge (the Personal API cannot push), asks for confirmation, sends and lists the result per session.
- **.FIT, without a cable**: the session drawer replaces the USB instructions with the three steps (download, import in Garmin Connect Web › Training › Import then « Send to Device », sync the watch) and an « Ouvrir Garmin Connect » link; the file is named `stride-YYYY-MM-DD-<sport>.fit`. This path works with the bridge off, expired or decommissioned.

### 23.6 Tests

- `internal/garminbridge` — OAuth1 against the RFC 5849 vector, percent-encoding, sign-in success (password reaches the SSO, nothing survives in the tokens), MFA then code, replayed code refused, credentials refused, `429`/`427`/CAPTCHA, ticket from the redirect, status states, create then update (no duplicate), lookup by name, recreate after a 404, refresh on `401`, expired without refresh, schedule failure, incomplete request, name identity, disconnect.
- `internal/domain` — kill switch (no gateway call), publisher choice (personal without tokens), idempotent push through the API, bulk push with skips, breaker after repeated blocks and its reset, expired session → `failed` + notification, disconnect, FIT file name.
- `internal/domainapi` — routes unmounted without the gateway, `401`/status, wrong password `400`, full login with a fake SSO, sealed blob without the password, disconnect, `409` when the admin switch is off, `push-today` (empty, connected, second run), MFA over HTTP.
- `internal/store` — token round-trip in the `garmin_web` row, the Personal row untouched, the admin switch defaulting to off.
- Front — `GarminBridgeCard` (no card without the gateway, warning before the form, wrong password cleared, connect/disconnect, MFA, expired), `PushTodayButton` (hidden without a bridge, per-session summary), `e2e/garmin-bridge.spec.ts` (MFA sign-in, bulk send, disconnect, the .FIT checklist replacing the cable).
