// Contrat d'API Patella — transcription à l'identique de docs/API_CONTRACT.md (§3).
// Toute réponse JSON est en camelCase ; les valeurs absentes sont `null` (jamais 0 inventé).

/* ------------------------------------------------------------------ */
/* Erreurs                                                            */
/* ------------------------------------------------------------------ */

export type ErrorCode =
  | 'GARMIN_UNAUTHORIZED'
  | 'NO_API_KEY'
  | 'NOT_FOUND'
  | 'GARMIN_UNAVAILABLE'
  | 'BAD_REQUEST'
  | 'INTERNAL'

export interface ApiErrorBody {
  error: string
  code: ErrorCode
}

/* ------------------------------------------------------------------ */
/* Health & settings                                                  */
/* ------------------------------------------------------------------ */

export interface HealthResponse {
  status: 'ok'
  version: string
  garminConfigured: boolean
}

export interface Settings {
  apiKeyConfigured: boolean
  apiKeySource: 'ui' | 'env' | 'none' // ui = saisie dans l'app (prioritaire), env = .env
  apiKeyHint: string | null // "••••a1b2" (4 derniers caractères), jamais la clé
  tokenExpiresAt: string | null // RFC3339
  timezone: string // "Europe/Paris"
  profile: Profile
}

export interface Profile {
  injuredSide: 'left' | 'right'
  cadenceTargetMin: number // 175
  cadenceTargetMax: number // 182
  gctBalanceAlertPct: number // 1.5  (écart |G − D| en points)
  verticalOscillationAlertCm: number // 9
  descentAlertM: number // 30
  rhrAlertDeltaBpm: number // 5
  pace10kMinSecPerKm: number // 215 (3:35)
  pace10kMaxSecPerKm: number // 220 (3:40)
  paceMarathonMinSecPerKm: number // 230 (3:50)
  paceMarathonMaxSecPerKm: number // 240 (4:00)
}

/** Body de `PUT /api/settings`. `apiKey: ""` supprime la clé UI (retour à l'env). */
export interface SettingsUpdate {
  apiKey?: string
  timezone?: string
  profile?: Partial<Profile>
}

/** Réponse de `POST /api/settings/test`. */
export interface ConnectionTestResult {
  ok: boolean
  message: string
  tokenExpiresAt: string | null
}

/* ------------------------------------------------------------------ */
/* Activités                                                          */
/* ------------------------------------------------------------------ */

export interface ActivityListResponse {
  items: ActivitySummary[]
  hasMore: boolean
}

export interface ActivitySummary {
  id: number // activityId (< 2^53, number OK)
  name: string
  type: string // typeKey brut : "running", "trail_running", "cycling", "walking"…
  typeLabel: string // FR : "Course", "Trail", "Vélo", "Marche"…
  isRunning: boolean // running | trail_running | treadmill_running | track_running…
  startLocal: string // "2026-11-02T18:30:12" (heure locale, sans offset)
  date: string // "2026-11-02"
  distanceM: number | null
  durationS: number | null
  avgPaceSecPerKm: number | null
  avgHR: number | null
  avgPower: number | null
  avgCadenceSpm: number | null
  elevGainM: number | null
  elevLossM: number | null
  trainingLoad: number | null
}

/* ------------------------------------------------------------------ */
/* Analyse d'activité                                                 */
/* ------------------------------------------------------------------ */

export type Mode = 'sfp' | 'recovery' | 'performance'

export interface ActivityAnalysis {
  mode: Mode
  generatedAt: string // RFC3339
  activity: ActivityDetail
  biomechanics: Biomechanics
  load: Load
  segments: Segments
  series: Series // pour les graphiques, ≤ 400 points
  recovery: RecoveryContext | null // nuit précédant la séance (calendarDate = date de l'activité)
  performance: Performance | null // calculé seulement si mode = "performance"
  history: HistoryRow[] | null // si history > 0
  alerts: Alert[]
  report: string // rapport markdown prêt à copier
  reportTokensEstimate: number // ≈ len(report)/4
  dataSources: string[] // ex: ["activity","fit","details","sleep","hrv","bodyBattery","dailyHeartRate"]
  warnings: string[] // sources en échec, non bloquantes ("Sommeil indisponible")
}

export interface ActivityDetail extends ActivitySummary {
  locationName: string | null
  movingDurationS: number | null
  maxHR: number | null
  normPower: number | null
  maxCadenceSpm: number | null
  aerobicTE: number | null
  anaerobicTE: number | null
  trainingEffectLabel: string | null
  impactLoad: number | null
  steps: number | null
  calories: number | null
  bodyBatteryDelta: number | null
}

export type AlertLevelSimple = 'ok' | 'warn' | 'danger'

export interface GctBalance {
  available: boolean
  leftPct: number | null // 49.2
  rightPct: number | null // 50.8
  gapPct: number | null // |G − D| = 1.6
  weakerSide: 'left' | 'right' | null
  alert: AlertLevelSimple | null
  reason: string | null // si indisponible : "Non mesuré : nécessite HRM-Pro / HRM 600 / Running Dynamics Pod"
}

export interface Biomechanics {
  gctBalance: GctBalance
  groundContactTimeMs: number | null
  cadenceAvgSpm: number | null // course uniquement (hors marche/arrêt)
  cadenceMaxSpm: number | null
  cadenceInTargetPct: number | null // % du temps de course dans [cadenceTargetMin, cadenceTargetMax]
  cadenceBelow170Pct: number | null
  verticalOscillationCm: number | null
  verticalRatioPct: number | null
  strideLengthM: number | null
}

export type DriftVerdict = 'stable' | 'moderate' | 'high'

export interface Drift {
  method: 'pw:hr' | 'pace:hr'
  decouplingPct: number // (EF1 − EF2)/EF1 × 100
  firstHalfHR: number
  secondHalfHR: number
  lastThirdHrRiseBpm: number // FC moy dernier tiers − FC moy tiers central
  verdict: DriftVerdict // <5 % / 5–10 % / >10 %
}

export interface Load {
  elevGainM: number | null
  elevLossM: number | null
  drift: Drift | null
  trainingLoad: number | null
  aerobicTE: number | null
  anaerobicTE: number | null
  hrZonesS: number[] | null // [z1..z5] secondes
  powerZonesS: number[] | null
}

export type SegmentKind = 'run' | 'walk' | 'stand' | 'active' | 'rest' | 'warmup' | 'cooldown' | 'lap'

export interface Segment {
  index: number // 1-based dans sa liste
  kind: SegmentKind
  label: string // "Bloc 1", "Pause 1", "Km 3", "Échauffement"
  startOffsetS: number // depuis le début de l'activité
  durationS: number
  distanceM: number
  paceSecPerKm: number | null
  avgHR: number | null
  maxHR: number | null
  avgPower: number | null
  avgCadenceSpm: number | null
  gctMs: number | null
  voCm: number | null
  gctBalanceLeftPct: number | null
  elevGainM: number | null
  elevLossM: number | null
}

export interface Segments {
  primary: 'blocks' | 'laps' // ce qu'utilise le rapport (blocks si ≥ 2 segments course/marche)
  blocks: Segment[] // course/marche (FIT rwd_* ou détection maison), sinon []
  laps: Segment[] // laps montre
}

/** Tableaux de même longueur, `null` = trou. */
export interface Series {
  tS: number[] // secondes depuis le départ
  hr: (number | null)[]
  cadenceSpm: (number | null)[]
  powerW: (number | null)[]
  paceSecPerKm: (number | null)[] // null si arrêt / vitesse < 0.5 m/s
  elevationM: (number | null)[]
  gctMs: (number | null)[]
  voCm: (number | null)[]
  gctBalanceLeftPct: (number | null)[]
}

/* ------------------------------------------------------------------ */
/* Récupération                                                       */
/* ------------------------------------------------------------------ */

export interface HrvContext {
  lastNightAvgMs: number | null
  lastNight5MinHighMs: number | null
  weeklyAvgMs: number | null
  baselineLowMs: number | null // balancedLow
  baselineHighMs: number | null // balancedUpper
  status: string | null // brut Garmin
  statusLabel: string | null // "Équilibré" | "Déséquilibré" | "Bas" | "Faible"
  deltaVsWeeklyPct: number | null
}

export interface SleepContext {
  score: number | null
  qualifier: string | null // "EXCELLENT" | "GOOD" | "FAIR" | "POOR"
  durationS: number | null
  deepS: number | null
  remS: number | null
  lightS: number | null
  awakeS: number | null
  avgSleepHR: number | null
  bedtimeLocal: string | null // "23:12"
  wakeLocal: string | null // "07:05"
}

export interface RestingHrContext {
  bpm: number | null
  avg7d: number | null
  delta: number | null
}

export interface BodyBatteryContext {
  atWake: number | null
  max: number | null
  min: number | null
  charged: number | null
  drained: number | null
}

export interface RecoveryContext {
  date: string // date de réveil (calendarDate Garmin)
  hrv: HrvContext | null
  sleep: SleepContext | null
  restingHR: RestingHrContext | null
  bodyBattery: BodyBatteryContext | null
  stressAvg: number | null
  trend: TrendDay[] // 7 jours (J-6 … J), ordre chronologique
}

export interface TrendDay {
  date: string
  hrvMs: number | null
  hrvStatus: string | null
  sleepScore: number | null
  sleepS: number | null
  deepS: number | null
  restingHR: number | null
  steps: number | null
  floorsUp: number | null
  bodyBatteryMax: number | null
  trainingLoad: number | null // somme des activités du jour
}

/* ------------------------------------------------------------------ */
/* Performance                                                        */
/* ------------------------------------------------------------------ */

export interface TargetPace {
  label: string // "Allure 10 km"
  rangeLabel: string // "3:35–3:40"
  seconds: number
  distanceM: number
}

export type AcwrVerdict = 'low' | 'optimal' | 'high' | 'danger'

export interface Acwr {
  acute7d: number
  chronic28d: number
  ratio: number | null
  verdict: AcwrVerdict // 0.8–1.3 optimal, >1.5 danger
}

export interface BestEffort {
  label: string
  seconds: number
}

export interface Performance {
  avgPower: number | null
  normPower: number | null
  timeInZonesS: number[] | null // FC z1..z5
  z4z5S: number | null
  targetPaces: TargetPace[]
  acwr: Acwr | null
  bestEfforts: BestEffort[] // fastestSplit_1000 / 1609 / 5000 si dispo
}

export interface HistoryRow {
  date: string
  type: string
  distanceM: number | null
  durationS: number | null
  paceSecPerKm: number | null
  avgHR: number | null
  avgCadenceSpm: number | null
  elevLossM: number | null
  trainingLoad: number | null
}

export type AlertLevel = 'info' | 'warn' | 'danger' | 'ok'

export interface Alert {
  level: AlertLevel
  code: string // "CADENCE_LOW", "VO_HIGH", "DESCENT_HIGH", "DRIFT_HIGH", "HRV_LOW", "RHR_HIGH", "SLEEP_LOW", "BB_LOW", "GCT_BALANCE", "GCT_BALANCE_NA", "FADE_END"…
  title: string // court : "Cadence basse"
  message: string // explication 1 phrase FR
}

/* ------------------------------------------------------------------ */
/* Analyse jour (mode Santé & Récupération)                           */
/* ------------------------------------------------------------------ */

export interface DaySummary {
  steps: number | null
  stepGoal: number | null
  floorsUp: number | null
  floorsDown: number | null
  intensityModerateMin: number | null
  intensityVigorousMin: number | null
  activeCalories: number | null
  stressAvg: number | null
  bodyBatteryCharged: number | null
  bodyBatteryDrained: number | null
}

export interface DayAnalysis {
  mode: 'recovery'
  date: string
  generatedAt: string
  recovery: RecoveryContext // nuit se terminant ce jour
  day: DaySummary
  activities: ActivitySummary[] // activités du jour (vélotaf, marche, renfo…)
  history: HistoryRow[] | null
  alerts: Alert[]
  report: string
  reportTokensEstimate: number
  warnings: string[]
}
