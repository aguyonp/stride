# Stride — Contrat d'API & faits Garmin

> **V1 document.** Superseded by [V2_SPEC.md](V2_SPEC.md) except §2 (verified Garmin Personal API facts and FIT conversions), which remains the reference.

Document de référence partagé entre le backend (Go, `backend/`) et le frontend (React, `frontend/`).
Toute réponse JSON utilise **camelCase**. Les valeurs absentes/indisponibles sont `null` (jamais 0 inventé).
Unités dans le nom du champ (`…S` secondes, `…M` mètres, `…Ms` millisecondes, `…Cm`, `…Spm`, `…Pct`, `…Bpm`).

---

## 1. Vue d'ensemble

```
garmin-api-extractor/
├── backend/            Go module (module path: stride) — sert /api/* ET le SPA embarqué
│   ├── cmd/server/     main.go (+ sous-commande `healthcheck` pour Docker)
│   └── internal/
│       ├── config/     env + settings persistés (/data/settings.json)
│       ├── garmin/     client Personal API (token, retry, cache)
│       ├── fitdata/    téléchargement + décodage FIT
│       ├── analysis/   calcul des métriques, segments, dérive, alertes
│       ├── report/     génération du rapport texte (markdown) par mode
│       ├── httpapi/    routes, middlewares (logs emoji, gzip, basic auth optionnelle)
│       ├── logx/       handler slog avec emoji
│       ├── frfmt/      formatage FR partagé (nombres à virgule, durées, allures, dates) — ajout backend
│       └── web/        //go:embed all:dist  (dist = build Vite copié au build Docker)
├── frontend/           Vite + React + TypeScript, build → frontend/dist
├── Dockerfile          multi-stage: node build → go build → distroless
├── docker-compose.yml
└── docs/API_CONTRACT.md
```

- Port HTTP : `8080` (env `PORT`).
- Données persistées : `/data` (env `DATA_DIR`, défaut `./data` en local) → `settings.json`, `token.json`, `cache/`.
- En dev : `vite` sur 5173 proxifie `/api` vers `http://localhost:8080`.

---

## 2. Faits vérifiés sur la Garmin Personal API (sonde réelle du 2026-11-03)

- Base URL : `https://personal.connectapi.garmin.com`
- Échange clé → token : `POST /personalapitoken-service/personal-api/personal-api-key/token`
  - Headers : `Accept: */*`, `Authorization: ApiKey <clé>` — pas de body.
  - Réponse : **texte brut = JWT** (pas de JSON). Expiration = claim `exp` du payload (base64url). Validité 24 h. Pas de refresh token : on rappelle le même endpoint.
- Appels data : `Authorization: Bearer <jwt>`, `Accept: application/json`.
- Variable d'env de la clé dans `.env` : **`GARMIN_API_TOKEN`** (accepter aussi `GARMIN_PERSONAL_API_KEY`, `GARMIN_API_KEY`).

### Endpoints qui répondent 200
| Usage | Chemin | Forme |
|---|---|---|
| Liste activités | `/activitylist-service/activities/search/activities?start=0&limit=20[&startDate=YYYY-MM-DD&endDate=YYYY-MM-DD&activityType=running]` | `[]` d'objets : `activityId, activityName, startTimeLocal ("2026-11-02 18:30:12"), startTimeGMT, activityType{typeKey}, distance(m), duration(s), movingDuration, elapsedDuration, elevationGain, elevationLoss, averageSpeed(m/s), maxSpeed, averageHR, maxHR, averageRunningCadenceInStepsPerMinute, maxRunningCadenceInStepsPerMinute, avgPower, maxPower, normPower, avgVerticalOscillation(cm), avgGroundContactTime(ms), avgStrideLength(cm), avgVerticalRatio(%), aerobicTrainingEffect, anaerobicTrainingEffect, trainingEffectLabel, activityTrainingLoad, lapCount, locationName, steps, calories, avgGradeAdjustedSpeed, differenceBodyBattery, hrTimeInZone_1..5 (s), powerTimeInZone_1..5 (s), moderateIntensityMinutes, vigorousIntensityMinutes, splitSummaries[]` |
| Détail activité | `/activity-service/activity/{id}` | `activityName, activityTypeDTO{typeKey}, locationName, summaryDTO{startTimeLocal ("2026-11-02T18:30:12.0"), startTimeGMT, distance, duration, movingDuration, elevationGain, elevationLoss, averageSpeed, averageMovingSpeed, averageHR, maxHR, minHR, averageRunCadence, maxRunCadence, averagePower, maxPower, normalizedPower, groundContactTime, strideLength(cm), verticalOscillation(cm), verticalRatio, trainingEffect, anaerobicTrainingEffect, trainingEffectLabel, activityTrainingLoad, impactLoad, steps, calories, avgGradeAdjustedSpeed, differenceBodyBattery, beginPotentialStamina, endPotentialStamina}, splitSummaries[{splitType ("RWD_RUN","RWD_WALK","RWD_STAND","INTERVAL_ACTIVE"…), noOfSplits, distance, duration, averageSpeed, averageHR, averageRunCadence, averagePower}]` |
| Time-series | `/activity-service/activity/{id}/details?maxChartSize=N` | `metricDescriptors[{metricsIndex,key,unit{key}}]`, `activityDetailMetrics[{metrics:[…]}]`. Clés dispo : `directTimestamp, sumDuration, sumElapsedDuration, sumMovingDuration, sumDistance, directSpeed, directGradeAdjustedSpeed, directHeartRate, directRunCadence, directDoubleCadence, directFractionalCadence, directPower, directElevation, directVerticalSpeed, directVerticalOscillation(cm), directVerticalRatio, directGroundContactTime(ms), directStrideLength(cm), directImpactLoadFactor, directPerformanceCondition, directBodyBattery, directAvailableStamina, directPotentialStamina, directLatitude, directLongitude`. **Pas d'équilibre GCT ici.** |
| Fichier FIT | `/download-service/files/activity/{id}` avec **exactement** `Accept: application/zip, application/x-zip-compressed, application/octet-stream` (sinon 406) | ZIP contenant 1 `.fit` |
| Workouts de l'activité | `/activity-service/activity/{id}/workouts` | `[{workoutName, sport, steps[]}]` |
| Sommeil jour | `/sleep-service/sleep/dailySleep?date=YYYY-MM-DD` | objet **à plat** : `calendarDate, sleepTimeSeconds, deepSleepSeconds, lightSleepSeconds, remSleepSeconds, awakeSleepSeconds, awakeCount, avgHeartRate, avgSleepStress, averageRespirationValue, sleepStartTimestampGMT(ms), sleepEndTimestampGMT(ms), sleepStartTimestampLocal, sleepEndTimestampLocal, sleepScores{overall{value,qualifierKey}, deepPercentage{value,qualifierKey}, …}, sleepScoreFeedback, sleepNeed{actual,baseline}` |
| Sommeil plage | `/sleep-service/sleep/dailySleepSummariesByDate?startDate=&endDate=` | `[]` du même objet |
| VFC jour | `/hrv-service/hrv/{date}` | `hrvSummary{calendarDate, weeklyAvg, lastNightAvg, lastNight5MinHigh, baseline{lowUpper, balancedLow, balancedUpper, markerValue}, status ("BALANCED","UNBALANCED","LOW","POOR"), feedbackPhrase}, hrvReadings[]` |
| VFC plage | `/hrv-service/hrv/daily/{start}/{end}` | `{hrvSummaries:[hrvSummary…]}` |
| Body Battery | `/wellness-service/wellness/bodyBattery/reports/daily?startDate=&endDate=` | `[{date, charged, drained, bodyBatteryValuesArray:[[tsMillis, level]…], bodyBatteryValueDescriptorDTOList[{bodyBatteryValueDescriptorIndex, bodyBatteryValueDescriptorKey}], bodyBatteryActivityEvent[{eventType, eventStartTimeGmt, durationInMilliseconds, bodyBatteryImpact}]}]` (résolution faible ~6 pts/jour : utiliser le point le plus proche de `sleepEndTimestampGMT`, à vérifier ; param `chunks` change la forme) |
| Résumés quotidiens | `/usersummary-service/v2/dailysummaries?startDate=&endDate=` | `{data:[{calendarDate, movement{steps, floorsAscended, floorsDescended}, heartRate{minValue,maxValue,restingValue}, intensityMinutes{goal,moderate,vigorous}, stress{avgLevel,maxLevel,qualifier}, bodyBattery{minValue,maxValue,chargedValue,drainedValue,latestValue}, calories{burnedActive,burnedTotal}, respiration{avgValue}}]}` |
| Pas | `/usersummary-service/stats/steps/daily/{start}/{end}` | `[{calendarDate,totalSteps,totalDistance,stepGoal}]` |
| Stress | `/usersummary-service/stats/stress/daily/{start}/{end}` | `[{calendarDate, values{overallStressLevel,…}}]` |
| FC jour | `/wellness-service/wellness/dailyHeartRate?date=` | `restingHeartRate, lastSevenDaysAvgRestingHeartRate, minHeartRate, maxHeartRate` |
| Zones FC | `/biometric-service/heartRateZones/v2` | `[{sport, trainingMethod, zone1Floor..zone5Floor, maxHeartRateUsed, lactateThresholdHeartRateUsed, restingHeartRateUsed}]` |
| Profil | `/userprofile-service/userprofile/settings` | (non sondé) |

### Endpoints en 404 (non exposés par la Personal API — ne pas utiliser)
`/activity-service/activity/{id}/splits|typedsplits|split_summaries|hrTimeInZones|powerTimeInZones|laps`, `/metrics-service/*` (training status/readiness/maxmet), exports TCX/GPX, `/wellness-service/wellness/floorsChartData/*`.

### Fichier FIT (lib `github.com/muktihari/fit` v0.28.5, `filedef.NewActivity`)
- Sessions/Laps/Records/Splits/SplitSummaries/TimeInZones présents.
- **Le fichier sondé n'a PAS `stance_time_balance`** (ni session, ni laps, ni records) : capteur requis (HRM‑Pro/HRM 600/Running Dynamics Pod). Le gérer comme `available:false`, jamais inventer.
- Laps : auto-lap distance (1 km) `Intensity=interval`, `LapTrigger=distance`.
- Splits : `SplitType` ∈ `interval_active`, `rwd_run`, `rwd_walk`, `rwd_stand` (+ `interval_rest|warmup|cooldown|recovery`) avec `StartTime`, `EndTime`, `TotalTimerTime`, `TotalDistance`, `AvgSpeed` → détection course/marche = **source des blocs**. Pas de FC/cadence/puissance dans Split : les recalculer depuis les Records de la fenêtre.
- Conversions : cadence FIT en strides/min → **spm = 2 × (cadence + fractional_cadence)** ; `VerticalOscillationScaled()` en mm → /10 = cm ; `StepLengthScaled()` en mm → /1000 = m ; `StanceTimeScaled()` en ms ; `StanceTimeBalanceScaled()` = % **jambe gauche** ; `VerticalRatioScaled()` en %. Les getters `…Scaled()` renvoient `NaN` si invalide ; les entiers invalides valent le max du type (0xFF, 0xFFFF…).
- `fit v0.28.5` impose `go 1.26.0` dans son go.mod → la directive du module `stride` est `go 1.26.0` (et non 1.24) ; build Docker en `golang:1.27-alpine`.

### Compléments vérifiés par le backend (sonde structurelle du 2026-11-03)

- **dailysummaries** : `movement.steps`, `movement.floorsAscended`, `movement.floorsDescended` sont des **objets** `{goal, value, distanceInMeters}` (pas des nombres) ; `value` est absent les jours sans étage. `stress.avgLevel` peut valoir −1/−2 (données insuffisantes → traité comme absent).
- **Body Battery** : descripteurs = `[{0,"timestamp"},{1,"bodyBatteryLevel"}]`, valeurs `[[tsMillis, level]…]`, ~6 points/jour. Sur les jours sondés, le point le plus proche de `sleepEndTimestampGMT` est à < 45 min du réveil et égal au max du jour → `atWake` = point le plus proche, tolérance ±3 h (sinon `null`). Événements `bodyBatteryActivityEvent[].eventType` ∈ `SLEEP|ACTIVITY|STRESS|RECOVERY`.
- **Absence de données** : `dailySleep` → 200 avec champs `null` ; `hrv/{date}` → **204** sans corps ; `dailyHeartRate` → 200 avec `null` ; activité inexistante → 404.
- **Unités du détail d'activité** : `summaryDTO.averageRunCadence`/`maxRunCadence` en **pas/min** (identiques à la liste), `strideLength` en cm, `verticalOscillation` en cm, `groundContactTime` en ms.
- **/details** : valeurs déjà dans l'unité d'affichage malgré `unit.factor` : `directDoubleCadence` = pas/min, `directRunCadence` = foulées/min, `directSpeed` m/s, `sumDuration` s, `directTimestamp` ms GMT. Contient `geoPolylineDTO` + lat/long (GPS) → ignorés. Utilisé uniquement en repli si le FIT est indisponible.
- **FIT TimeInZone (session)** : `time_in_hr_zone` a 7 cases `[sous Z1, Z1..Z5, au-dessus]` (ms) ; Z1..Z5 = cases 1..5 (+ "au-dessus" ajouté à Z5) — identique à `hrTimeInZone_1..5` de la liste.
- **FIT Laps (2026-11-05)** : le `timestamp` des laps décodé par la lib vaut l'heure de début de séance ; fin d'un lap = `StartTime + TotalElapsedTime`.
- **Live V2 (2026-11-05)** : `/details?maxChartSize=100000` expose `directGradeAdjustedSpeed`, `directPotentialStamina`, `directAvailableStamina` à pleine résolution (fusionnés par horodatage dans les records FIT) ; `summaryDTO` porte `startLatitude/startLongitude` (météo), `impactLoad`, `beginPotentialStamina/endPotentialStamina/minAvailableStamina`. `dailySleepSummariesByDate` contient `sleepScores` : un seul appel par plage suffit.
- **FIT Splits rwd_*** : Garmin produit des micro-pauses (1–3 s) ; pendant l'auto-pause, `TotalTimerTime` d'un `rwd_stand` est quasi nul → durée d'une pause = `TotalElapsedTime`, durée d'un bloc de course = `TotalTimerTime`.

---

## 3. Endpoints de l'application

Erreurs : statut HTTP adéquat + `{"error": "message lisible en français", "code": "GARMIN_UNAUTHORIZED" | "NO_API_KEY" | "NOT_FOUND" | "GARMIN_UNAVAILABLE" | "BAD_REQUEST" | "INTERNAL"}`.

### `GET /api/health`
```json
{ "status": "ok", "version": "1.0.0", "garminConfigured": true }
```

### `GET /api/settings` · `PUT /api/settings`
```ts
interface Settings {
  apiKeyConfigured: boolean;
  apiKeySource: "ui" | "env" | "none";   // ui = saisie dans l'app (prioritaire), env = .env
  apiKeyHint: string | null;             // "••••a1b2" (4 derniers caractères), jamais la clé
  tokenExpiresAt: string | null;         // RFC3339
  timezone: string;                      // "Europe/Paris"
  profile: Profile;
}
interface Profile {
  injuredSide: "left" | "right";
  cadenceTargetMin: number;   // 175
  cadenceTargetMax: number;   // 182
  gctBalanceAlertPct: number; // 1.5  (écart |G − D| en points)
  verticalOscillationAlertCm: number; // 9
  descentAlertM: number;      // 30
  rhrAlertDeltaBpm: number;   // 5
  pace10kMinSecPerKm: number; // 215 (3:35)
  pace10kMaxSecPerKm: number; // 220 (3:40)
  paceMarathonMinSecPerKm: number; // 230 (3:50)
  paceMarathonMaxSecPerKm: number; // 240 (4:00)
}
```
`PUT` body : `{ "apiKey"?: string, "timezone"?: string, "profile"?: Partial<Profile> }`.
- `apiKey` non vide → validée par échange de token (400 `GARMIN_UNAUTHORIZED` si refusée) puis persistée (fichier 0600).
- `apiKey: ""` → supprime la clé UI (retour à l'env). Réponse = `Settings`.

### `POST /api/settings/test`
```json
{ "ok": true, "message": "Connexion Garmin OK", "tokenExpiresAt": "2026-11-04T18:00:00Z" }
```

### `GET /api/activities?limit=20&start=0[&from=YYYY-MM-DD&to=YYYY-MM-DD]`
```ts
interface ActivityListResponse { items: ActivitySummary[]; hasMore: boolean }
interface ActivitySummary {
  id: number;                 // activityId (< 2^53, number OK)
  name: string;
  type: string;               // typeKey brut : "running", "trail_running", "cycling", "walking"…
  typeLabel: string;          // FR : "Course", "Trail", "Vélo", "Marche"…
  isRunning: boolean;         // running | trail_running | treadmill_running | track_running…
  startLocal: string;         // "2026-11-02T18:30:12" (heure locale, sans offset)
  date: string;               // "2026-11-02"
  distanceM: number | null;
  durationS: number | null;
  avgPaceSecPerKm: number | null;
  avgHR: number | null;
  avgPower: number | null;
  avgCadenceSpm: number | null;
  elevGainM: number | null;
  elevLossM: number | null;
  trainingLoad: number | null;
}
```

### `GET /api/activities/{id}/analysis?mode=sfp|recovery|performance[&history=14]`
`mode` défaut `sfp`. `history` = nb de jours d'historique compact à ajouter au rapport (0 = aucun, défaut 0).
```ts
type Mode = "sfp" | "recovery" | "performance";

interface ActivityAnalysis {
  mode: Mode;
  generatedAt: string;                 // RFC3339
  activity: ActivityDetail;
  biomechanics: Biomechanics;
  load: Load;
  segments: Segments;
  series: Series;                      // pour les graphiques, ≤ 400 points
  recovery: RecoveryContext | null;    // nuit précédant la séance (calendarDate = date de l'activité)
  performance: Performance | null;     // calculé seulement si mode = "performance"
  history: HistoryRow[] | null;        // si history > 0
  alerts: Alert[];
  report: string;                      // rapport markdown prêt à copier (voir §4)
  reportTokensEstimate: number;        // ≈ len(report)/4
  dataSources: string[];               // ex: ["activity","fit","details","sleep","hrv","bodyBattery","dailyHeartRate"]
  warnings: string[];                  // sources en échec, non bloquantes ("Sommeil indisponible")
}

interface ActivityDetail extends ActivitySummary {
  locationName: string | null;
  movingDurationS: number | null;
  maxHR: number | null;
  normPower: number | null;
  maxCadenceSpm: number | null;
  aerobicTE: number | null;
  anaerobicTE: number | null;
  trainingEffectLabel: string | null;
  impactLoad: number | null;
  steps: number | null;
  calories: number | null;
  bodyBatteryDelta: number | null;
}

interface Biomechanics {
  gctBalance: {
    available: boolean;
    leftPct: number | null;            // 49.2
    rightPct: number | null;           // 50.8
    gapPct: number | null;             // |G − D| = 1.6
    weakerSide: "left" | "right" | null;
    alert: "ok" | "warn" | "danger" | null;
    reason: string | null;             // si indisponible : "Non mesuré : nécessite HRM-Pro / HRM 600 / Running Dynamics Pod"
  };
  groundContactTimeMs: number | null;
  cadenceAvgSpm: number | null;        // course uniquement (hors marche/arrêt)
  cadenceMaxSpm: number | null;
  cadenceInTargetPct: number | null;   // % du temps de course dans [cadenceTargetMin, cadenceTargetMax]
  cadenceBelow170Pct: number | null;
  verticalOscillationCm: number | null;
  verticalRatioPct: number | null;
  strideLengthM: number | null;
}

interface Load {
  elevGainM: number | null;
  elevLossM: number | null;
  drift: {
    method: "pw:hr" | "pace:hr";
    decouplingPct: number;             // (EF1 − EF2)/EF1 × 100, EF = puissance/FC ou vitesse/FC, sur la course uniquement
    firstHalfHR: number; secondHalfHR: number;
    lastThirdHrRiseBpm: number;        // FC moy dernier tiers − FC moy tiers central
    verdict: "stable" | "moderate" | "high";   // <5 % / 5–10 % / >10 %
  } | null;
  trainingLoad: number | null;
  aerobicTE: number | null;
  anaerobicTE: number | null;
  hrZonesS: number[] | null;           // [z1..z5] secondes
  powerZonesS: number[] | null;
}

interface Segment {
  index: number;                       // 1-based dans sa liste
  kind: "run" | "walk" | "stand" | "active" | "rest" | "warmup" | "cooldown" | "lap";
  label: string;                       // "Bloc 1", "Pause 1", "Km 3", "Échauffement"
  startOffsetS: number;                // depuis le début de l'activité
  durationS: number;
  distanceM: number;
  paceSecPerKm: number | null;
  avgHR: number | null;
  maxHR: number | null;
  avgPower: number | null;
  avgCadenceSpm: number | null;
  gctMs: number | null;
  voCm: number | null;
  gctBalanceLeftPct: number | null;
  elevGainM: number | null;
  elevLossM: number | null;
}
interface Segments {
  primary: "blocks" | "laps";          // ce qu'utilise le rapport (blocks si ≥ 2 segments course/marche)
  blocks: Segment[];                   // course/marche (FIT rwd_* ou détection maison), sinon []
  laps: Segment[];                     // laps montre
}

interface Series {                     // tableaux de même longueur, null = trou
  tS: number[];                        // secondes depuis le départ
  hr: (number | null)[];
  cadenceSpm: (number | null)[];
  powerW: (number | null)[];
  paceSecPerKm: (number | null)[];     // null si arrêt / vitesse < 0.5 m/s
  elevationM: (number | null)[];
  gctMs: (number | null)[];
  voCm: (number | null)[];
  gctBalanceLeftPct: (number | null)[];
}

interface RecoveryContext {
  date: string;                        // date de réveil (calendarDate Garmin)
  hrv: {
    lastNightAvgMs: number | null;
    lastNight5MinHighMs: number | null;
    weeklyAvgMs: number | null;
    baselineLowMs: number | null;      // balancedLow
    baselineHighMs: number | null;     // balancedUpper
    status: string | null;             // brut Garmin
    statusLabel: string | null;        // "Équilibré" | "Déséquilibré" | "Bas" | "Faible"
    deltaVsWeeklyPct: number | null;
  } | null;
  sleep: {
    score: number | null;
    qualifier: string | null;          // "EXCELLENT" | "GOOD" | "FAIR" | "POOR"
    durationS: number | null;
    deepS: number | null;
    remS: number | null;
    lightS: number | null;
    awakeS: number | null;
    avgSleepHR: number | null;
    bedtimeLocal: string | null;       // "23:12"
    wakeLocal: string | null;          // "07:05"
  } | null;
  restingHR: { bpm: number | null; avg7d: number | null; delta: number | null } | null;
  bodyBattery: { atWake: number | null; max: number | null; min: number | null; charged: number | null; drained: number | null } | null;
  stressAvg: number | null;
  trend: TrendDay[];                   // 7 jours (J-6 … J), ordre chronologique
}
interface TrendDay {
  date: string;
  hrvMs: number | null;
  hrvStatus: string | null;
  sleepScore: number | null;
  sleepS: number | null;
  deepS: number | null;
  restingHR: number | null;
  steps: number | null;
  floorsUp: number | null;
  bodyBatteryMax: number | null;
  trainingLoad: number | null;         // somme des activités du jour
}

interface Performance {
  avgPower: number | null;
  normPower: number | null;
  timeInZonesS: number[] | null;       // FC z1..z5
  z4z5S: number | null;
  targetPaces: { label: string; rangeLabel: string; seconds: number; distanceM: number }[]; // "Allure 10 km" "3:35–3:40"
  acwr: { acute7d: number; chronic28d: number; ratio: number | null; verdict: "low" | "optimal" | "high" | "danger" } | null; // charge = somme activityTrainingLoad ; chronique = moyenne hebdo sur 28 j ; 0.8–1.3 optimal, >1.5 danger
  bestEfforts: { label: string; seconds: number }[]; // fastestSplit_1000 / 1609 / 5000 si dispo
}

interface HistoryRow {
  date: string; type: string; distanceM: number | null; durationS: number | null;
  paceSecPerKm: number | null; avgHR: number | null; avgCadenceSpm: number | null;
  elevLossM: number | null; trainingLoad: number | null;
}

interface Alert {
  level: "info" | "warn" | "danger" | "ok";
  code: string;                        // "CADENCE_LOW", "VO_HIGH", "DESCENT_HIGH", "DRIFT_HIGH", "HRV_LOW", "RHR_HIGH", "SLEEP_LOW", "BB_LOW", "GCT_BALANCE", "GCT_BALANCE_NA", "FADE_END"…
  title: string;                       // court : "Cadence basse"
  message: string;                     // explication 1 phrase FR
}
```

### `GET /api/activities/{id}/report?mode=…&history=…`
`text/plain; charset=utf-8` — exactement le champ `report` ci-dessus (pour curl/automatisation).

### `GET /api/days/{date}/analysis[?history=14]` (Mode 2 « Santé & Récupération », jour sans course)
```ts
interface DayAnalysis {
  mode: "recovery";
  date: string;
  generatedAt: string;
  recovery: RecoveryContext;           // nuit se terminant ce jour
  day: {
    steps: number | null; stepGoal: number | null;
    floorsUp: number | null; floorsDown: number | null;
    intensityModerateMin: number | null; intensityVigorousMin: number | null;
    activeCalories: number | null;
    stressAvg: number | null;
    bodyBatteryCharged: number | null; bodyBatteryDrained: number | null;
  };
  activities: ActivitySummary[];       // activités du jour (vélotaf, marche, renfo…)
  history: HistoryRow[] | null;
  alerts: Alert[];
  report: string;
  reportTokensEstimate: number;
  warnings: string[];
}
```
### `GET /api/days/{date}/report` → texte brut.

---

## 4. Format des rapports (markdown compact, chiffres FR avec virgule décimale)

Valeur absente → `N/D`. Pas de ligne vide superflue. Pas d'emoji sauf dans `### ALERTES` (⚠️ warn, 🛑 danger, ℹ️ info, ✅ ok).

### Mode SFP (`sfp`)
```
### RAPPORT SÉANCE GARMIN - 02/11/2026 18:30
- **Mode :** Rééducation SFP
- **Séance :** 7,02 km en 42:10 | Allure moy: 6:00/km | Puissance: 285 W
- **Dénivelé :** +32 m / -35 m

### BIOMÉCANIQUE ROTULE & IMPACTS
- **Équilibre contact sol (G/D) :** 49,2 % G / 50,8 % D (écart 1,6 pt)   ← ou "N/D (non mesuré par le capteur)"
- **Temps de contact sol :** 248 ms
- **Cadence moyenne :** 178 spm (Max : 186 spm) | Dans la cible 175-182 : 72 %
- **Oscillation verticale :** 8,1 cm | **Ratio vertical :** 7,6 %
- **Longueur de foulée moyenne :** 1,05 m

### DÉTAIL DES INTERVALLES / SPLITS
| Bloc | Durée | Distance | Allure | FC moy | Watts | Cadence |
|---|---|---|---|---|---|---|
| Bloc 1 | 10:00 | 1,75 km | 5:43/km | 134 bpm | 290 W | 179 spm |
| Pause 1 | 1:00 | 0,08 km | Marche | 118 bpm | — | — |

### CHARGE & FATIGUE
- **Dérive cardiaque (Pw:Hr) :** +3,2 % (stable) | FC dernier tiers : +4 bpm
- **Training Load :** 85 | **TE aérobie :** 3,1 | **Impact load :** 92

### RÉCUPÉRATION & PHYSIOLOGIE (NUIT DU 01/11 AU 02/11)
- **VFC nocturne :** 45 ms (Statut : Équilibré vs baseline 40-55 ms | moy. 7 j : 48 ms)
- **Score Sommeil :** 78/100 (Dont profond : 1h22) | Durée : 7h12
- **FC repos nocturne :** 48 bpm (moy. 7 j : 46 bpm) | **Body Battery réveil :** 82/100

### ALERTES
- ⚠️ Cadence basse : 168 spm < 170, risque d'overstriding.
```
Les pauses (walk/stand) affichent `Marche` / `Arrêt` dans la colonne Allure et `—` en Watts/Cadence.

### Mode Performance (`performance`)
Même en-tête (`Mode : Performance`), puis :
```
### PERFORMANCE
- **Puissance :** moy 285 W | normalisée 298 W
- **Zones FC :** Z1 2:10 | Z2 18:40 | Z3 12:05 | Z4 6:30 | Z5 0:45 (Z4+Z5 : 7:15)
- **Allure 10 km (3:35-3:40) :** 4:20 sur 1,20 km | **Allure marathon (3:50-4:00) :** 0:00
- **Dérive cardiaque (Pw:Hr) :** +3,2 % (stable)
- **Charge aiguë/chronique :** 410 / 365 (ACWR 1,12 – optimal)
- **Training Load séance :** 85 | **TE :** 3,1 aéro / 0,5 anaéro

### BIOMÉCANIQUE (RÉSUMÉ)
- Cadence 178 spm | GCT 248 ms | OV 8,1 cm | RV 7,6 % | Foulée 1,05 m | G/D N/D
```
puis `DÉTAIL DES INTERVALLES / SPLITS`, `RÉCUPÉRATION & PHYSIOLOGIE`, `ALERTES`.

### Mode Récupération (`recovery`, page jour)
```
### BILAN RÉCUPÉRATION GARMIN - 03/11/2026
- **Mode :** Santé & Récupération (hors séance)
- **Activité du jour :** 8 432 pas (objectif 8 000) | Étages : 12 ↑ / 10 ↓ | Intensité : 25 min mod. / 5 min vig.
- **Activités enregistrées :** Vélo 22:10 (6,1 km) ; Marche 35:00 (2,8 km)
- **Stress moyen :** 32 | **Body Battery :** +45 / -60

### RÉCUPÉRATION & PHYSIOLOGIE (NUIT DU 02/11 AU 03/11)
(identique au mode SFP)

### TENDANCE 7 JOURS
| Jour | VFC | Statut | Sommeil | Profond | FC repos | Pas | Étages |
|---|---|---|---|---|---|---|---|
| 28/10 | 46 ms | Équilibré | 81 | 1h31 | 47 | 9 120 | 8 |

### ALERTES
```
Si une **activité** est analysée en mode `recovery`, le rapport = en-tête séance + bloc RÉCUPÉRATION + TENDANCE 7 JOURS + ALERTES (biomécanique résumée en 1 ligne).

### Historique optionnel (`history>0`), ajouté avant ALERTES
```
### HISTORIQUE 14 JOURS
| Date | Type | Distance | Durée | Allure | FC | Cadence | D- | Load |
|---|---|---|---|---|---|---|---|---|
```

---

## 5. Seuils d'alertes (configurables via Profile)
| Code | Condition | Niveau |
|---|---|---|
| CADENCE_LOW | cadence course < 170 | danger ; < cadenceTargetMin → warn |
| VO_HIGH | OV > verticalOscillationAlertCm | warn (> +1 cm → danger) |
| DESCENT_HIGH | D- > descentAlertM | warn (> 2× → danger) |
| GCT_BALANCE | écart > gctBalanceAlertPct (warn), > 2× (danger) ; préciser si le côté le plus court = injuredSide | |
| GCT_BALANCE_NA | équilibre non mesuré | info |
| DRIFT_HIGH | découplage 5–10 % warn, > 10 % danger | |
| FADE_END | allure du dernier bloc de course > 8 % plus lente que la moyenne des blocs précédents avec FC ≥ | warn |
| HRV_LOW | statut LOW/POOR → danger, UNBALANCED → warn, ou lastNightAvg < balancedLow | |
| RHR_HIGH | delta ≥ rhrAlertDeltaBpm | warn |
| SLEEP_LOW | score < 60 warn ; profond < 1 h info | |
| BB_LOW | réveil < 50 warn, < 30 danger | |
| FLOORS_HIGH | (mode recovery) étages montés > 20 | warn |
| ACWR | ratio > 1,3 warn, > 1,5 danger | |
| ALL_GOOD | aucune alerte warn/danger | ok |

---

## 6. Précisions d'implémentation backend (ajouts compatibles, aucun champ renommé)

### Statuts HTTP des erreurs

| `code` | Statut | Cas |
|---|---|---|
| `NO_API_KEY` | 412 | aucune clé (ni UI ni env) |
| `GARMIN_UNAUTHORIZED` | 401 (400 sur `PUT /api/settings`) | clé/jeton refusé par Garmin |
| `NOT_FOUND` | 404 | activité inconnue de Garmin, route `/api/*` inconnue |
| `GARMIN_UNAVAILABLE` | 502 (504 si délai dépassé) | Garmin 5xx/429 après retries, erreur réseau |
| `BAD_REQUEST` | 400 | paramètre invalide (id, mode, history 0–90, limit 1–100, date AAAA-MM-JJ non future, profil/fuseau invalide) |
| `INTERNAL` | 500 | erreur inattendue |
| `UNAUTHORIZED` | 401 + `WWW-Authenticate: Basic` | **ajout** : Basic Auth de l'app active (`APP_AUTH_USER`/`APP_AUTH_PASSWORD`) et identifiants absents/faux. `/api/health` reste public. |

- `POST /api/settings/test` : succès = forme du §3 ; échec = JSON d'erreur ci-dessus (pas de `{ok:false}`).
- `apiKeyHint` : `"••••"` + 4 derniers caractères seulement si la clé fait ≥ 12 caractères, sinon `"••••"` ; `null` sans clé.
- `GET /api/days/today/analysis|report` : alias de la date du jour (fuseau de l'app).
- `GET …/report` : texte exact du champ `report` (qui se termine par un `\n`). `reportTokensEstimate` = nb de caractères (runes) / 4.

### Champs ajoutés

- `Performance.hrZoneFloorsBpm: number[] | null` — planchers Z1..Z5 des zones FC Garmin (sport RUNNING, sinon DEFAULT).
- `HistoryRow.typeLabel: string` — libellé FR (`type` reste le typeKey brut).
- `DayAnalysis.dataSources: string[]` — comme pour l'analyse d'activité.
- `dataSources` possibles : `activity, fit | details, sleep, hrv, bodyBattery, dailyHeartRate, dailySummaries, steps, activityList, hrZones`.
- `warnings[]` : `"<Source> indisponible (HTTP 500 | délai dépassé | erreur réseau)"` ; une réponse vide (204) n'est pas un avertissement (champ `null`).

### Sémantique

- `gctBalance.weakerSide` = côté au **temps de contact le plus court** (le moins chargé). L'alerte `GCT_BALANCE` précise si c'est `Profile.injuredSide`.
- **Masque « course »** (biomécanique, dérive, allures cibles) : records hors fenêtres FIT `rwd_walk`/`rwd_stand`, avec cadence ≥ 130 spm et vitesse ≥ 1,8 m/s ; moyennes pondérées par le temps. `cadenceMaxSpm` = max d'une moyenne glissante sur 5 records (ignore les pics isolés).
- **Blocs** (course à pied uniquement ; `[]` pour vélo, kayak, ski…) : splits FIT `rwd_*` consolidés — pauses adjacentes fusionnées (type dominant `walk`/`stand`), pause < 10 s absorbée dans la course, bloc de course < 20 s entre deux pauses absorbé dans la pause. Sans splits : détection maison (lissage 10 s, segments ≥ 20 s, trou d'enregistrement > 10 s = arrêt). `primary = "blocks"` si ≥ 2 blocs (ou s'il n'y a aucun lap).
- **Laps** : « Km n » si `LapTrigger=distance` (le dernier lap partiel `session_end` après des laps distance garde « Km n »), sinon « Lap n » ; `warmup` « Échauffement », `cooldown` « Retour au calme », `rest`/`recovery` « Récup n » ; `kind:"active"` pour un lap d'entraînement structuré. Valeurs du lap prioritaires, complétées par les records de la fenêtre. Sans FIT : découpage par km depuis `/details`.
- `paceSecPerKm` d'un segment : `null` pour `stand` ou si distance < 10 m.
- **Dérive** : ≥ 10 min de course, Pw:Hr si ≥ 80 % des records ont une puissance, sinon Pace:Hr ; hors course à pied seulement Pw:Hr (sinon `null`).
- **FADE_END** : blocs de course (≥ 200 m) si ≥ 2, sinon laps ≥ 500 m.
- **ACWR** : activités de la liste dont le début ≤ début de la séance analysée ; `acwr = null` si charge chronique nulle.
- `TrendDay.trainingLoad` = 0 si la liste d'activités a été lue et qu'il n'y a pas d'activité ce jour-là ; `null` si la liste est indisponible.
- **Historique** (`history=N`) : activité → activités des N jours se terminant le jour J (séance analysée et séances postérieures exclues) ; jour → les N jours précédant la date (activités du jour déjà listées).
- **Cache** : détail d'activité sur disque seulement si l'activité est terminée depuis > 2 h (évite de figer un résumé Garmin en cours de calcul) ; FIT/`details` sur disque (immuables) ; bien-être sur disque si date de fin < aujourd'hui − 1, sinon mémoire 10 min ; liste d'activités mémoire 2 min ; zones FC mémoire 1 h.

### Rapports

- Tableau des segments : en-tête de 1re colonne `Bloc` (primary = blocks) ou `Lap` (primary = laps) ; `Aucun split disponible.` si vide.
- En-tête « Séance » : `Allure moy: m:ss/km` pour la course, `Vitesse moy: x,x km/h` sinon.
- Historique : `Allure` et `Cadence` = `—` pour les activités hors course ; `Aucune activité sur la période.` si vide.
- Messages d'alerte sans « : » interne (la ligne de rapport est `- <emoji> <title> : <message>`) ; `SLEEP_LOW` sert pour le score (< 60, warn) et le sommeil profond (< 1 h, info).
