# Stride V2 — Specification & shared contract

Stride extracts sports telemetry (Garmin Connect Personal API, Strava), computes biomechanics / load / recovery insights, tracks plan, goals and pain, and generates compact Markdown prompts for an AI coach. V2 turns the single-user V1 into a multi-user service.

**Non-negotiables**
- No metric or idea from the product brief may be dropped. When a source cannot provide a value, return `null` and a human reason; never fabricate.
- Every generated prompt (single session, day/health, multi-session) starts with the objectivity directive (§6.1), verbatim.
- Strict per-user isolation: every domain query filters on `user_id` taken from the authenticated context, never from the request.
- Secrets (Garmin personal API keys, Garmin JWTs, Strava access/refresh tokens) are encrypted at rest with `secret.Sealer` (AES-256-GCM, `STRIDE_MASTER_KEY`), AAD = `"<provider>:<userID>:<field>"`. Never logged, never returned (only hints like `••••a1b2` are allowed in API responses).
- Code comments: very few, English only. UI text: French. Logs: emoji + structured fields (V1 style).

V1 references: `docs/API_CONTRACT.md` §2 lists the verified Garmin Personal API endpoints, payload shapes and FIT conversions (still valid). Its §3–§5 (V1 endpoints, modes) are superseded by this document.

---

## 1. Verified provider facts (2026-11-04 probe)

Garmin Personal API (`https://personal.connectapi.garmin.com`), in addition to V1 §2:
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
- Valkey holds only disposable data with TTL: provider list responses, today's wellness, computed analyses, WebAuthn ceremonies, OAuth states, sync locks. `POST /api/admin/cache/flush` deletes `stride:cache:*` only (sessions live in SQLite, so nobody is logged out).
- Without `STRIDE_VALKEY_URL` (local dev), the in-memory cache is used.

### 2.1 Backend packages and ownership

| Package / file | Owner | Notes |
|---|---|---|
| `internal/env` | skeleton, platform may extend | all env vars (§9) |
| `internal/secret`, `internal/httpx`, `internal/auth/context.go`, `internal/apimeter/apimeter.go` (types+interface), `internal/provider/strava/strava.go` (types+API), `internal/provider/weather/weather.go` (types+Provider) | skeleton — **stable contracts**, additive changes only | |
| `internal/store/store.go`, `migrations/0001_platform.sql`, `store/platform_*.go` | platform | keep `users.id INTEGER PRIMARY KEY` |
| `migrations/0002_domain.sql`, `store/domain_*.go` (or queries inside domain packages) | domain | FKs to `users(id) ON DELETE CASCADE` |
| `cmd/stride`, `internal/httpapi`, `internal/auth/*`, `internal/admin`, `internal/scheduler`, `internal/apimeter/*` (impl), `internal/cache`, `internal/logx`, `internal/web`, `internal/provider/strava/*` (Client impl), `internal/provider/weather/*` (OpenMeteo impl), Dockerfile, compose.yml, .env.example, Makefile, README.md | platform | |
| `internal/domain`, `internal/domainapi`, `internal/garmin`, `internal/fitdata`, `internal/analysis`, `internal/report`, `internal/frfmt`, any new domain package (`internal/training`, `internal/sync`, …), `docs/API_CONTRACT.md` | domain | |
| V1 leftovers: `internal/config/config.go` → platform deletes; `internal/config/settings.go` (Profile) → domain moves into its own package and deletes; V1 `internal/httpapi/server.go` handlers → recreated by domain in `internal/domainapi`, platform deletes the V1 file | | |
| `frontend/` | frontend | |

Integration seams (already in the skeleton): `domain.New(domain.Deps)`, `(*domain.Services).MorningSync / OnUserCreated / PurgeUserCache`, `domainapi.Register(mux, domainapi.Deps{RequireUser, RequireAdmin, ...})`, `auth.UserFrom(ctx)`, `apimeter.Meter` + `apimeter.WithUser(ctx, id)`, `strava.API`, `weather.Provider`, `cache.Cache`, `store.Store`.

---

## 3. HTTP API

JSON camelCase. Errors: `{"error": "message FR lisible", "code": "<httpx.Code*>"}`. Dates `YYYY-MM-DD`, timestamps RFC3339. Every authenticated route returns 401 `UNAUTHORIZED` without a valid session; admin routes 403 `FORBIDDEN` for non-admins. Session = HttpOnly cookie `stride_session` (SameSite=Lax, Secure when `STRIDE_PUBLIC_URL` is https). State-changing requests must carry header `X-Stride-CSRF: 1` (simple CSRF guard; the SPA always sends it).

### 3.1 Platform

| Method & path | Body → Response |
|---|---|
| `GET /health` | liveness → `{"status":"ok"}` |
| `GET /health/ready` | DB + cache ping → 200 `{"status":"ready","checks":{"db":"ok","cache":"ok"}}` or 503 with failing check message |
| `GET /api/auth/state` | → `AuthState` |
| `POST /api/auth/register/begin` | `{displayName, inviteCode?}` → `{publicKey: PublicKeyCredentialCreationOptionsJSON}` (resident key required, user verification preferred) |
| `POST /api/auth/register/finish` | RegistrationResponseJSON (as produced by `@simplewebauthn/browser` `startRegistration`) → `Me` + session cookie |
| `POST /api/auth/login/begin` | `{}` → `{publicKey: PublicKeyCredentialRequestOptionsJSON}` (discoverable credentials, empty allowCredentials) |
| `POST /api/auth/login/finish` | AuthenticationResponseJSON → `Me` + session cookie |
| `POST /api/auth/logout` | → 204 |
| `GET /api/me` · `PATCH /api/me` | `{displayName?, onboardingCompleted?}` → `Me` |
| `DELETE /api/me` | delete own account (cascade) → 204 |
| `GET /api/me/passkeys` | → `Passkey[]` |
| `POST /api/me/passkeys/begin` · `/finish` | add a passkey while logged in |
| `PATCH /api/me/passkeys/{id}` · `DELETE /api/me/passkeys/{id}` | rename `{name}` · delete (refuse last one: 409) |
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
interface AdminUser { id: number; displayName: string; role: "user" | "admin"; status: "active" | "disabled"; createdAt: string; lastLoginAt: string | null; passkeys: number }
interface Invitation { id: number; label: string; codePrefix: string; createdAt: string; expiresAt: string | null; maxUses: number; uses: number; revoked: boolean }
```
Rules: the very first registered user needs no invitation and becomes `admin`. Afterwards `registrationMode` (DB setting, initial value from `STRIDE_REGISTRATION`, default `invite`) applies. Disabled users cannot log in.

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

`ActivityView` keeps every V1 `ActivityAnalysis` field (see V1 contract §3 for `Biomechanics`, `Segment`, `Series`, `RecoveryContext`, `Performance`, `Alert`) **minus** `mode`/`report`, plus:
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
- Header must be flawless at every width (320 → 2560 px), never overflow, sticky without visual artefacts. See the user's screenshot of the V1 header bug (path given in the agent brief).
- Auth: passkey-only (register / login / add passkey), invite links `/register?invite=CODE`, first-user flow. Onboarding wizard (profile & focus area → Garmin → Strava → done), each step skippable.
- Freshness pill (db / cache / live + time) and sync status; `POST /api/sync` on load.
- Planning & goals module (week view, plan vs done), pain form (0–10 step 0.5, labels from focus preset), shoes manager, context tags, daily readiness check-in.
- Multi-session export (select sessions or last N weeks).
- Settings: profile & thresholds, integrations, passkeys, backup export/restore JSON, account deletion.
- `/admin` (admin only): users (enable/disable/delete/role), invitations (create → copy link once, revoke), registration mode, provider connections, API meter (requests, 429s, cooldowns, recent errors), flush cache, resync user, user backup/restore.
- PWA: `manifest.json`, icons (any + maskable), iOS meta tags (`apple-mobile-web-app-capable`, status bar style, `apple-touch-icon`), Android `theme-color`, minimal service worker (network-first navigation, cache-first hashed assets).
- Glassmorphism design kept, mobile-first, accessible, no external network calls, French UI, rebranded « Stride » (new generic icon: no knee/kneecap).

---

## 8. Security rules (all agents)

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
| `STRIDE_DATA_DIR` / `STRIDE_DB_PATH` | `./data` / `<data>/stride.db` | SQLite location |
| `STRIDE_VALKEY_URL` | — (memory) | `redis://valkey:6379/0` in compose |
| `STRIDE_ADMIN_GARMIN_KEY` (alias `GARMIN_API_TOKEN`) | — | auto-connect Garmin for the first admin |
| `STRIDE_STRAVA_CLIENT_ID` / `_SECRET` / `_WEBHOOK_TOKEN` | — | enable Strava |
| `STRIDE_WEATHER` | `open-meteo` | `off` to disable |
| `STRIDE_SYNC_HOUR`, `STRIDE_SYNC_JITTER_MIN`, `STRIDE_SYNC_JITTER_MAX` | `7`, `30s`, `90s` | morning prefetch |
| `STRIDE_GARMIN_BASE_URL` | Garmin personal API | tests |
| `TZ`, `LOG_LEVEL`, `LOG_FORMAT` | `Europe/Paris`, `info`, `pretty` | |
| compose only: `STRIDE_HOST_BIND`, `STRIDE_HOST_PORT` | `127.0.0.1`, `8080` | published port |

## 10. Domain implementation notes (clarifications of §3–§6)

- **Errors**: `404 NOT_FOUND` for any id of another user; `412 PROVIDER_NOT_CONNECTED` when Garmin/Strava is needed but not connected (or Strava not configured); provider refusing stored credentials → `502 PROVIDER_UNAUTHORIZED` (never 401, which means "session expired" to the SPA; `PUT /api/integrations/garmin` keeps `400 PROVIDER_UNAUTHORIZED`); provider cooldown/429 → `503 RATE_LIMITED` + `Retry-After`; provider 5xx/network → `502`/`504 PROVIDER_UNAVAILABLE`.
- **Responses**: `PUT|DELETE /api/integrations/garmin`, `POST …/garmin/test`, `DELETE /api/integrations/strava` return `Integrations`. `POST` goals/plan/shoes → `201` + object, `PATCH` → `200` + object, `DELETE` → `204`. `POST /api/admin/users/{id}/resync` → `202 {"queued":true}`. Strava callback errors: `reason` ∈ `state|denied|scope|exchange|already_linked|disabled|internal`.
- **Partial bodies**: `PUT /api/profile`, `PATCH`, notes and check-in bodies are merged onto the current value (`null` clears a nullable field, read-only keys such as `id`, `match`, `distanceKm`, `shoe`, `updatedAt` are ignored, unknown keys → 400). Changing `focusArea` without `thresholds` resets them to the new preset. `GET /api/days/{date}` and the check-in reject future dates.
- **Sync**: first sync lists 56 days (load ratio + trends); then each sync lists from `lastSync − 2 days`. Bundles (detail, details series, FIT, Strava detail + streams, weather) are prefetched for new activities of the last 7 days (max 6 per sync); older ones are fetched on first view or export (max 10 per export). Lazy sync is skipped for 5 min after a sync (`cooldownUntil`). Strava webhook retries (5 and 30 min) are in-memory; the next lazy sync catches up after a restart.
- **Wellness**: per-day kinds `sleep`, `hrv`, `daily_summary` (range endpoints, ≤ 28-day chunks) and `user_summary` (per day, J and J-1 only). A day without data is only persisted as final after 7 days (late watch syncs). Body Battery at wake, RHR 7 d and intensity minutes come from `usersummary/daily`; the V1 `bodyBattery/reports`, `dailyHeartRate` and `steps` calls are no longer needed. A day with step data but no floors counts as 0 floors.
- **Insights**: fatigue drift verdict falls back on the cadence drop (< −2 moderate, < −4 high spm) when GCT is not recorded; stride length falls back on speed ÷ cadence (warned). GAP uses Garmin `directGradeAdjustedSpeed` when recorded, the Minetti cost model otherwise (warned). Alert codes: V1 codes + `LOAD_RATIO` (was `ACWR`), `GCT_DRIFT`, `DOWNHILL_CADENCE_DROP`, `FLOORS_PREV_DAY`, `PLAN_DEVIATION`, `PAIN_DURING`, `PAIN_NEXT_MORNING`; descent/floors/downhill alerts only for injury presets (`none` keeps load, biomechanics and recovery alerts).
- **Plan**: matching per day and compatible sport (closest duration); `partial` when duration or distance is below target by more than `planDeviationPct`; `paceDeltaSecPerKm` is 0 inside the target range, otherwise the distance to the nearest bound. `ActivitySummary.planStatus = "extra"` only in weeks that have planned sessions (walks/hikes never count).
- **Shoes**: `distanceKm` sums activities whose notes point to the shoe, or (no note shoe) whose Strava gear matches `stravaGearId`.
- **History**: `ActivityView.history` lists every activity of the 14 days up to the session; the report table keeps running rows only.
- **Backup**: notes whose activity is not (yet) in the database are skipped and not counted.
