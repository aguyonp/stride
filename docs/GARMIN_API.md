# Garmin Connect Personal API — verified facts

Reference for `backend/internal/garmin` and `backend/internal/fitdata`. Additional V2 facts (404 endpoints, Stamina, GAP, `usersummary/daily`): [SPEC.md](SPEC.md) §1.

## Verified facts (probes of 2026-11-03)

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

### Pont expérimental (API web, non officielle) — §23

`/workout-service/*` n'existe pas sur le palier Personal (404/403). L'envoi de séances passe donc par un gateway isolé et désactivé par défaut (`internal/garminbridge`) qui parle à l'API *web* (`connectapi.garmin.com`) comme l'application officielle : SSO `sso.garmin.com` (widget, code MFA), ticket de service, échange d'un jeton **DI OAuth2** sur `diauth.garmin.com`, puis `POST /workout-service/workout`, `PUT /workout-service/workout/{id}` et `POST /workout-service/schedule/{id}`. Rien de tout cela n'est vérifié contractuellement par Garmin : le paquet est jetable, 

Pour diagnostiquer un échec sans partager d'identifiant : `cd backend && go run ./cmd/garmin-probe`. L'outil rejoue exactement le flux du serveur (même paquet), lit le mot de passe sans écho, ne l'écrit nulle part, gère l'étape du code et affiche l'état, l'étape et l'erreur. Il vit dans un paquet `main` séparé : rien n'en est embarqué dans le binaire.

En cas d'échec, l'erreur nomme l'étape (`formulaire`, `identifiants`, `code`, `ticket`, `jetons`, `profil`) et distingue un mot de passe refusé (400) d'une limitation 429, d'un CAPTCHA et d'une page inattendue (502) : le log ne contient jamais l'e-mail, le mot de passe ni un jeton.

l'interrupteur est un réglage admin (`garminBridgeEnabled`, Admin › Instance, désactivé par défaut) et le repli `.FIT` (import Garmin Connect Web) reste la voie supportée.

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
