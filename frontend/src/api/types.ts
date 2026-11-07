// Stride API contract (docs/SPEC.md §3).

export type ErrorCode =
  | 'BAD_REQUEST'
  | 'UNAUTHORIZED'
  | 'FORBIDDEN'
  | 'NOT_FOUND'
  | 'GONE'
  | 'CONFLICT'
  | 'PRECONDITION_FAILED'
  | 'RATE_LIMITED'
  | 'PROVIDER_UNAUTHORIZED'
  | 'PROVIDER_UNAVAILABLE'
  | 'PROVIDER_NOT_CONNECTED'
  | 'INVITE_REQUIRED'
  | 'INTERNAL'

export interface ApiErrorBody {
  error: string
  code: ErrorCode
}

/* ---------------------------------- Platform (§3.1) ---------------------------------- */

export type Role = 'user' | 'admin'
export type RegistrationMode = 'open' | 'invite'

export interface AuthState {
  authenticated: boolean
  user: Me | null
  registrationMode: RegistrationMode
  firstUser: boolean
  rpId: string
}
export interface Me {
  id: number
  displayName: string
  role: Role
  createdAt: string
  onboardingCompleted: boolean
}
export interface Passkey {
  id: string
  name: string
  createdAt: string
  lastUsedAt: string | null
  backedUp: boolean
}
export type AdminIntegration = 'garmin' | 'strava' | 'llm'
export interface AdminUser {
  id: number
  displayName: string
  role: Role
  status: 'active' | 'disabled'
  createdAt: string
  lastLoginAt: string | null
  passkeys: number
  /** Latest session activity (4.1 control center). */
  lastSeenAt: string | null
  sessions: number
  activities: number
  storageBytes: number
  integrations: AdminIntegration[]
}
export interface Invitation {
  id: number
  label: string
  codePrefix: string
  createdAt: string
  expiresAt: string | null
  maxUses: number
  uses: number
  revoked: boolean
}
export interface InvitationCreated {
  invitation: Invitation
  code: string
  link: string
}
export interface AdminSettings {
  registrationMode: RegistrationMode
}

/** WebAuthn options are opaque JSON for the UI; @simplewebauthn/browser consumes them. */
export interface PasskeyBegin<T> {
  publicKey: T
}

interface MeterQuota {
  limit15m: number
  usage15m: number
  limitDaily: number
  usageDaily: number
  updatedAt: string
}
export interface ProviderStats {
  provider: string
  requests1h: number
  requests24h: number
  errors1h: number
  rateLimited24h: number
  inFlight: number
  cooldownUntil: string | null
  quota: MeterQuota | null
  avgLatencyMs: number
}
interface MeterErrorEvent {
  time: string
  provider: string
  userId: number
  endpoint: string
  status: number
  message: string
}
export interface MeterSnapshot {
  providers: ProviderStats[]
  recentErrors: MeterErrorEvent[]
}

/* ---------------------------------- Admin control center (§14.6) ---------------------------------- */

export type JobGroup = 'sync' | 'llm' | 'webhook'
export interface JobGroupStats {
  group: JobGroup
  workers: number
  running: number
  queued: number
  delayed: number
}
/** Non-secret runtime configuration (allowlist, never keys, tokens or internal addresses). */
export interface RuntimeConfig {
  publicUrl: string
  rpId: string
  secureCookies: boolean
  sessionTtlHours: number
  timezone: string
  logLevel: string
  logFormat: string
  cacheBackend: string
  metricsEnabled: boolean
  stravaEnabled: boolean
  weatherProvider: string
  allowPrivateEgress: boolean
  syncHour: number
}
export interface DatabaseStatus {
  ok: boolean
  sizeBytes: number
  walBytes: number
  freeBytes: number
}
export interface AdminOverview {
  generatedAt: string
  build: BuildInfo
  startedAt: string
  uptimeS: number
  registrationMode: RegistrationMode
  database: DatabaseStatus
  cache: { backend: string; ok: boolean; latencyMs: number | null }
  jobs: { groups: JobGroupStats[]; active: number; failed24h: number }
  streams: { clients: number; users: number }
  errors24h: number
  logsDropped: number
  providers: ProviderStats[]
  counts: { users: number; activeUsers: number; admins: number; connectedUsers: number; llmUsers: number; activities: number; activeShareLinks: number; sessions: number; feedbackNew: number }
  config: RuntimeConfig
}
/** A job of any user; `result` is always null here (it may hold a private AI analysis). */
export interface AdminJob extends JobStatus {
  userName: string
  retryable: boolean
  cancellable: boolean
}
export type AdminJobFilter = 'active' | 'failed' | 'all'
export interface AdminJobList {
  items: AdminJob[]
  groups: JobGroupStats[]
}
/** A share link seen by an administrator: token prefix only, never the URL. */
export interface AdminShareLink {
  id: number
  userId: number
  userName: string
  label: string
  tokenPrefix: string
  createdAt: string
  expiresAt: string | null
  revokedAt: string | null
  accessCount: number
  lastAccessedAt: string | null
  state: 'active' | 'expired' | 'revoked'
  periodWeeks: SharePeriodWeeks
  sections: ShareSection[]
  anonymize: boolean
}
export interface PurgeResult {
  removed: Record<'sessions' | 'invitations' | 'apiEvents' | 'logs' | 'jobs' | 'shareLinks', number>
}
export interface OptimizeResult {
  vacuum: boolean
  before: DatabaseStatus
  after: DatabaseStatus
  durationMs: number
}
export type FeedbackCategory = 'bug' | 'idea' | 'other'
export type FeedbackStatus = 'new' | 'read' | 'done'
/** Non-sensitive environment attached to a feedback (path only, never query strings). */
export interface FeedbackContext {
  appVersion: string
  route: string
  viewport: string
  userAgent: string
}
export interface FeedbackInput {
  category: FeedbackCategory
  message: string
  contactOk: boolean
  /** Sent only when `contactOk` is true. */
  contactEmail?: string
  context: FeedbackContext
}
export interface FeedbackItem {
  id: number
  userId: number
  userName: string
  category: FeedbackCategory
  message: string
  contactOk: boolean
  contactEmail: string | null
  context: FeedbackContext
  status: FeedbackStatus
  adminNote: string
  createdAt: string
  updatedAt: string
}
export interface FeedbackList {
  items: FeedbackItem[]
  counts: { new: number; read: number; done: number; total: number }
}
export interface IntegrityResult {
  ok: boolean
  problems: string[]
  total: number
  durationMs: number
}

/* ---------------------------------- Domain (§3.2) ---------------------------------- */

export type Source = 'garmin' | 'strava' | 'import' | 'manual'
export interface Freshness {
  source: 'db' | 'cache' | 'live'
  updatedAt: string
}

export type ProviderStatus = 'ok' | 'error' | 'disconnected'
export interface Integrations {
  garmin: { connected: boolean; status: ProviderStatus; keyHint: string | null; tokenExpiresAt: string | null; lastSyncAt: string | null; lastError: string | null }
  strava: { available: boolean; connected: boolean; status: ProviderStatus; athleteName: string | null; lastSyncAt: string | null; lastError: string | null; scopes?: string[]; missingScopes?: string[] }
  weather: { enabled: boolean; provider: string }
}
export interface ProviderSync {
  lastSyncAt: string | null
  newActivities: number
  error: string | null
}
export interface SyncStatus {
  running: boolean
  lastSyncAt: string | null
  lastLiveAt: string | null
  cooldownUntil: string | null
  providers: { garmin: ProviderSync | null; strava: ProviderSync | null }
  jobId?: string | null
}
export interface AdminConnection {
  userId: number
  provider: string
  status: string
  lastSyncAt: string | null
  lastError: string | null
  lastErrorAt: string | null
}

export type Sport = 'run' | 'bike' | 'walk' | 'hike' | 'swim' | 'strength' | 'other'
export interface ActivitySummary {
  id: number
  garminId: number | null
  stravaId: number | null
  sources: Source[]
  name: string
  type: string
  typeLabel: string
  sport: Sport
  isRunning: boolean
  startLocal: string
  date: string
  distanceM: number | null
  durationS: number | null
  avgPaceSecPerKm: number | null
  gapSecPerKm: number | null
  avgHR: number | null
  avgPower: number | null
  avgCadenceSpm: number | null
  elevGainM: number | null
  elevLossM: number | null
  trainingLoad: number | null
  painDuring: number | null
  hasNotes: boolean
  planStatus: 'done' | 'partial' | 'extra' | null
  /** Strava kudos_count; null when unknown (Garmin likes are not exposed). */
  kudos?: number | null
}
export interface ActivityListResponse {
  items: ActivitySummary[]
  hasMore: boolean
  freshness: Freshness
}

interface ActivityDetail extends ActivitySummary {
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

export type AlertLevel = 'info' | 'warn' | 'danger' | 'ok'
type AlertLevelSimple = 'ok' | 'warn' | 'danger'
export interface Alert {
  level: AlertLevel
  code: string
  title: string
  message: string
}

export interface GctBalance {
  available: boolean
  leftPct: number | null
  rightPct: number | null
  gapPct: number | null
  weakerSide: 'left' | 'right' | null
  alert: AlertLevelSimple | null
  reason: string | null
}
export interface Biomechanics {
  gctBalance: GctBalance
  groundContactTimeMs: number | null
  cadenceAvgSpm: number | null
  cadenceMaxSpm: number | null
  cadenceInTargetPct: number | null
  cadenceBelow170Pct: number | null
  verticalOscillationCm: number | null
  verticalRatioPct: number | null
  strideLengthM: number | null
}

export type DriftVerdict = 'stable' | 'moderate' | 'high'
export interface Drift {
  method: 'pw:hr' | 'pace:hr'
  decouplingPct: number
  firstHalfHR: number
  secondHalfHR: number
  lastThirdHrRiseBpm: number
  verdict: DriftVerdict
}
interface Load {
  elevGainM: number | null
  elevLossM: number | null
  drift: Drift | null
  trainingLoad: number | null
  aerobicTE: number | null
  anaerobicTE: number | null
  hrZonesS: number[] | null
  powerZonesS: number[] | null
}

export type SegmentKind = 'run' | 'walk' | 'stand' | 'active' | 'rest' | 'warmup' | 'cooldown' | 'lap'
export interface Segment {
  index: number
  kind: SegmentKind
  label: string
  startOffsetS: number
  durationS: number
  distanceM: number
  paceSecPerKm: number | null
  gapSecPerKm: number | null
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
  primary: 'blocks' | 'laps'
  blocks: Segment[]
  laps: Segment[]
}

export interface Series {
  tS: number[]
  hr: (number | null)[]
  cadenceSpm: (number | null)[]
  powerW: (number | null)[]
  paceSecPerKm: (number | null)[]
  elevationM: (number | null)[]
  gctMs: (number | null)[]
  voCm: (number | null)[]
  gctBalanceLeftPct: (number | null)[]
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
  trainingLoad: number | null
}
export interface RecoveryContext {
  date: string
  hrv: {
    lastNightAvgMs: number | null
    lastNight5MinHighMs: number | null
    weeklyAvgMs: number | null
    baselineLowMs: number | null
    baselineHighMs: number | null
    status: string | null
    statusLabel: string | null
    deltaVsWeeklyPct: number | null
  } | null
  sleep: {
    score: number | null
    qualifier: string | null
    durationS: number | null
    deepS: number | null
    remS: number | null
    lightS: number | null
    awakeS: number | null
    avgSleepHR: number | null
    bedtimeLocal: string | null
    wakeLocal: string | null
  } | null
  restingHR: { bpm: number | null; avg7d: number | null; delta: number | null } | null
  bodyBattery: { atWake: number | null; max: number | null; min: number | null; charged: number | null; drained: number | null } | null
  stressAvg: number | null
  trend: TrendDay[]
}

export type AcwrVerdict = 'low' | 'optimal' | 'high' | 'danger'
interface Acwr {
  acute7d: number
  chronic28d: number
  ratio: number | null
  verdict: AcwrVerdict
}
interface TargetPace {
  label: string
  rangeLabel: string
  seconds: number
  distanceM: number
}
interface BestEffort {
  label: string
  seconds: number
}
export interface Performance {
  avgPower: number | null
  normPower: number | null
  timeInZonesS: number[] | null
  z4z5S: number | null
  targetPaces: TargetPace[]
  acwr: Acwr | null
  bestEfforts: BestEffort[]
}

interface WeatherConditions {
  source: string
  temperatureC: number | null
  apparentC: number | null
  humidityPct: number | null
  windKph: number | null
  windGustKph: number | null
  windDirDeg: number | null
  precipMm: number | null
  weatherCode: number | null
  description: string
  observedAt: string
}

export interface Stamina {
  startPct: number | null
  endPct: number | null
  minPct: number | null
}
interface StravaExtras {
  title: string
  description: string
  privateNote: string
  rpe: number | null
  relativeEffort: number | null
  gearName: string | null
}
export interface FatigueDrift {
  gctStartMs: number | null
  gctEndMs: number | null
  gctDeltaMs: number | null
  gctDeltaPct: number | null
  voStartCm: number | null
  voEndCm: number | null
  voDeltaCm: number | null
  cadenceStartSpm: number | null
  cadenceEndSpm: number | null
  cadenceDeltaSpm: number | null
  verdict: DriftVerdict
}
export interface SlopeBucket {
  cadenceSpm: number | null
  strideM: number | null
  strideMaxM: number | null
  paceSecPerKm: number | null
  durationS: number
  distanceM: number
}
export interface Slope {
  thresholdPct: number
  flat: SlopeBucket | null
  downhill: SlopeBucket | null
  uphill: SlopeBucket | null
  downhillCadenceDropSpm: number | null
}
export interface LoadRatio {
  acute: number
  chronic: number
  ratio: number | null
  verdict: AcwrVerdict
  source: 'computed'
}
export interface Readiness {
  score: number | null
  status: string | null
  source: 'manual' | null
  reason: string | null
}
export type RecoveryWithReadiness = RecoveryContext & { readiness: Readiness }

interface CrossLoadDay {
  date: string
  rides: number
  distanceKm: number
  durationS: number
  avgHR: number | null
  trainingLoad: number | null
}
export interface CrossLoad {
  windowStart: string
  windowEnd: string
  bike: { rides: number; distanceKm: number; durationS: number; avgHR: number | null; trainingLoad: number | null }
  bikeByDay: CrossLoadDay[]
  floors: { prevDayUp: number | null; prevDayDown: number | null; dayUp: number | null; dayDown: number | null }
  steps: { prevDay: number | null; day: number | null }
  stressPrevDay: number | null
  otherActivities: ActivitySummary[]
}

export type ShoeType = 'soft' | 'tempo' | 'carbon'
/** Strava-side state of a linked pair at the last sync (§14.5). */
export type ShoeStravaState = 'active' | 'retired' | 'deleted'
export interface Shoe {
  id: number
  name: string
  brand: string
  model: string
  dropMm: number | null
  type: ShoeType
  stravaGearId: string | null
  retired: boolean
  /** Real total: Strava's all-time gear distance when known, else `analyzedKm`. */
  distanceKm: number
  /** Sum of the activities Stride holds for this pair (partial when Strava knows more). */
  analyzedKm: number
  distanceSource: 'strava' | 'stride'
  stravaDistanceKm: number | null
  stravaPrimary: boolean
  stravaState: ShoeStravaState | null
  stravaSyncedAt: string | null
  note: string
}
type ShoeReadOnly = 'id' | 'distanceKm' | 'analyzedKm' | 'distanceSource' | 'stravaDistanceKm' | 'stravaPrimary' | 'stravaState' | 'stravaSyncedAt' | 'stravaGearId'
export type ShoeInput = Omit<Shoe, ShoeReadOnly> & { stravaGearId?: string | null }

export interface ActivityNotes {
  painDuring: number | null
  painCold: number | null
  painNextMorning: number | null
  rpe: number | null
  shoeId: number | null
  shoe: Shoe | null
  tags: string[]
  note: string
  updatedAt: string | null
  shoeAuto?: boolean
}
export type ActivityNotesInput = Partial<Omit<ActivityNotes, 'shoe' | 'updatedAt' | 'shoeAuto'>>

export interface HistoryRow {
  activityId: number
  date: string
  type: string
  typeLabel: string
  distanceM: number | null
  durationS: number | null
  paceSecPerKm: number | null
  avgCadenceSpm: number | null
  gctMs: number | null
  elevLossM: number | null
  trainingLoad: number | null
  shoeName: string | null
  painDuring: number | null
  painCold: number | null
  painNextMorning: number | null
}

export type GoalKind = 'macro' | 'race' | 'cadence' | 'weekly_volume' | 'other'
export interface Goal {
  id: number
  title: string
  kind: GoalKind
  targetValue: number | null
  targetUnit: string | null
  targetDate: string | null
  status: 'active' | 'done' | 'archived'
  note: string
}
export type GoalInput = Omit<Goal, 'id'>

export type PlanSport = 'run' | 'bike' | 'strength' | 'mobility' | 'rest' | 'other'
export type MatchStatus = 'done' | 'partial' | 'missed' | 'upcoming' | 'extra'
export interface PlanMatch {
  plannedId: number
  activityId: number | null
  status: MatchStatus
  durationDeltaPct: number | null
  distanceDeltaPct: number | null
  paceDeltaSecPerKm: number | null
  cadenceDeltaSpm: number | null
  summary: string
}
export interface PlannedSession {
  id: number
  date: string
  sport: PlanSport
  title: string
  targetDurationS: number | null
  targetDistanceM: number | null
  targetPaceMinSecPerKm: number | null
  targetPaceMaxSecPerKm: number | null
  targetCadenceSpm: number | null
  instructions: string
  match: PlanMatch | null
  steps: WorkoutStep[]
  source: PlanSource
  garmin: GarminPush
}
export type PlannedSessionInput = Omit<PlannedSession, 'id' | 'match' | 'steps' | 'source' | 'garmin'> & { steps?: WorkoutStep[]; source?: PlanSource }

export interface WeekPlan {
  weekStart: string
  days: { date: string; planned: PlannedSession[]; done: ActivitySummary[] }[]
  totals: { plannedDurationS: number; doneDurationS: number; plannedDistanceM: number; doneDistanceM: number; sessionsPlanned: number; sessionsDone: number }
}

export interface ActivityView {
  generatedAt: string
  freshness: Freshness
  sourceLabel: 'Garmin' | 'Strava' | 'Fusion' | 'Import' | 'Manuel'
  activity: ActivityDetail & {
    gapSecPerKm: number | null
    stamina: Stamina | null
    strava: StravaExtras | null
  }
  weather: (WeatherConditions & { windDirLabel: string | null }) | null
  biomechanics: Biomechanics & { fatigueDrift: FatigueDrift | null; slope: Slope | null }
  load: Load & { impactLoad: number | null; stamina: Stamina | null; loadRatio: LoadRatio | null; stravaRelativeEffort: number | null }
  segments: Segments
  series: Series & { gapSecPerKm: (number | null)[]; staminaPct: (number | null)[] }
  crossLoad: CrossLoad
  recovery: RecoveryWithReadiness | null
  performance: Performance | null
  plan: PlanMatch | null
  notes: ActivityNotes
  goals: Goal[]
  upcoming: PlannedSession[]
  history: HistoryRow[]
  alerts: Alert[]
  warnings: string[]
  dataSources: string[]
  efficiency?: ActivityEfficiency | null
  impact?: ActivityImpact | null
  readiness?: ReadinessBanner
}

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
export interface DailyCheckin {
  date: string
  trainingReadiness: number | null
  readinessStatus: string | null
  note: string
  updatedAt: string
}
export interface DailyCheckinInput {
  trainingReadiness?: number | null
  readinessStatus?: string | null
  note?: string
}
export interface DayView {
  date: string
  generatedAt: string
  freshness: Freshness
  recovery: RecoveryWithReadiness | null
  day: DaySummary
  crossLoad: CrossLoad
  activities: ActivitySummary[]
  planned: PlannedSession[]
  checkin: DailyCheckin | null
  goals: Goal[]
  upcoming: PlannedSession[]
  alerts: Alert[]
  warnings: string[]
  readiness?: ReadinessBanner
}

export type FocusArea = 'none' | 'knee_pfps' | 'achilles' | 'shin_splints' | 'other'
type InjuredSide = 'left' | 'right' | 'both'
export interface Thresholds {
  cadenceMinSpm: number
  cadenceTargetMin: number
  cadenceTargetMax: number
  gctBalanceGapPct: number
  verticalOscillationMaxCm: number
  gctDriftMaxPct: number
  descentMaxM: number
  floorsPrevDayMax: number
  loadRatioMax: number
  rhrDeltaMaxBpm: number
  planDeviationPct: number
  downhillCadenceDropSpm: number
}
export interface PaceTarget {
  label: string
  minSecPerKm: number
  maxSecPerKm: number
}
export interface AthleteProfile {
  focusArea: FocusArea
  focusLabel: string
  injuredSide: InjuredSide | null
  timezone: string
  thresholds: Thresholds
  paceTargets: PaceTarget[]
}
export interface FocusPreset {
  focusArea: FocusArea
  label: string
  thresholds: Thresholds
  painLabels: { during: string; cold: string; nextMorning: string }
  painEnabled: boolean
  emphasis: string[]
}

export type IncludeKey = 'pain' | 'biomech' | 'health' | 'plan' | 'load' | 'splits' | 'history'

export interface MultiExportRequest {
  activityIds?: number[]
  weeks?: number
  include?: IncludeKey[]
}
export interface MultiExportResponse {
  markdown: string
  tokensEstimate: number
  sessions: number
}

export interface RestoreResult {
  imported: { goals: number; plan: number; shoes: number; notes: number; checkins: number; profile: number | boolean }
}

/* ---------------------------------- V3 (§11) ---------------------------------- */

export type LLMProvider = 'gemini' | 'anthropic' | 'openai' | 'openai_compat'
export interface LLMConfig {
  configured: boolean
  provider: LLMProvider | null
  model: string | null
  baseUrl: string | null
  keyHint: string | null
  updatedAt: string | null
}
export interface LLMConfigInput {
  provider: LLMProvider
  model: string
  baseUrl?: string
  apiKey?: string
}
export interface LLMTestResult {
  ok: boolean
  latencyMs: number
  message: string
}
type LLMAnalysisKind = 'activity' | 'day' | 'multi'
export interface LLMAnalyzeRequest {
  kind: LLMAnalysisKind
  activityId?: number
  date?: string
  activityIds?: number[]
  weeks?: number
  include?: IncludeKey[]
  question?: string
}
export interface LLMAnalysis {
  id: number
  kind: string
  ref: string
  provider: string
  model: string
  markdown: string
  promptTokensEstimate: number
  durationMs: number
  createdAt: string
}

export type StepType = 'warmup' | 'run' | 'interval' | 'recovery' | 'rest' | 'cooldown' | 'repeat'
export type TargetKind = 'none' | 'pace' | 'hr' | 'cadence' | 'power'
export interface WorkoutStep {
  type: StepType
  durationS?: number
  distanceM?: number
  repeat?: number
  steps?: WorkoutStep[]
  target?: { kind: TargetKind; min?: number; max?: number }
  note?: string
}
type PlanSource = 'manual' | 'llm' | 'import'
export interface GarminPush {
  status: 'none' | 'pushed' | 'failed' | 'unsupported'
  workoutId: number | null
  pushedAt: string | null
  error: string | null
}
export type PlanHorizon = 'session' | 'week' | '2weeks' | 'goal'
export interface PlanGenerateRequest {
  mode: 'prompt' | 'llm'
  horizon: PlanHorizon
  startDate: string
  goalId?: number
  note?: string
}
export interface PlanPromptResponse {
  prompt: string
  tokensEstimate: number
}
export interface PlanDraftsResponse {
  drafts: PlannedSessionInput[]
  errors: string[]
  raw?: string
  phases?: PlanPhase[]
}
export interface CalendarDay {
  date: string
  planned: PlannedSession[]
  done: ActivitySummary[]
  events?: CalendarEvent[]
}

export interface ActivityEfficiency {
  eligible: boolean
  reason: string | null
  efSpeed: number | null
  efPower: number | null
  trendChangePct: number | null
}
export interface ActivityImpact {
  steps: number | null
  stepsPerKm: number | null
  voFactor: number | null
  descentFactor: number | null
  jointIndex: number | null
  jointIndex7d: number | null
}
interface EfficiencyPoint {
  date: string
  activityId: number
  efSpeed: number | null
  efPower: number | null
  avgHR: number | null
  paceSecPerKm: number | null
}
export interface EfficiencyTrend {
  points: EfficiencyPoint[]
  rollingEfSpeed: (number | null)[]
  changePct: number | null
}
type ReadinessLevel = 'green' | 'orange' | 'red' | 'unknown'
export interface ReadinessBanner {
  level: ReadinessLevel
  title: string
  reasons: string[]
  hrv7dAvg: number | null
  hrvBaselineLow: number | null
  nightsBelowBaseline: number | null
  sleep7dAvg: number | null
  loadRatio: number | null
}

export type TrackPoint = [lat: number, lon: number, tS: number, cadenceSpm: number | null, gradePct: number | null]
export interface ActivityTrack {
  points: TrackPoint[]
  bounds: { minLat: number; maxLat: number; minLon: number; maxLon: number } | null
}

export type NotificationKind = 'discord' | 'telegram' | 'json'
export interface NotificationTarget {
  id: number
  kind: NotificationKind
  label: string
  enabled: boolean
  hint: string
  lastStatus: string | null
  lastSentAt: string | null
  lastError: string | null
}
export interface NotificationInput {
  kind: NotificationKind
  label: string
  enabled: boolean
  url?: string
  botToken?: string
  chatId?: string
}
export interface NotificationTestResult {
  ok: boolean
  status: number | string
  message: string
}
export interface ImportResult {
  activityId: number
  merged: boolean
}

export interface BuildInfo {
  version: string
  commit: string
  builtAt: string
  go: string
  label: string
}

type LogCategory = 'request' | 'user_action'
export interface LogEntry {
  id: number
  at: string
  category: LogCategory
  level: 'info' | 'error'
  userId: number | null
  userName: string | null
  method: string
  target: string
  provider: string
  status: number
  durationMs: number
  action: string
  message: string
}
export interface LogPage {
  items: LogEntry[]
  nextBefore: number | null
  dropped: number
}
export interface LogFilter {
  category: 'all' | LogCategory
  level: 'all' | 'error'
  userId: number | null
}

/* ---------------------------------- 3.1.0 (§12) ---------------------------------- */

export type RecordKey = '1k' | '5k' | '10k' | 'half' | 'marathon' | 'longest'
export interface PersonalRecord {
  key: RecordKey
  label: string
  distanceM: number
  timeS: number | null
  paceSecPerKm: number | null
  date: string | null
  activityId: number | null
  source: 'computed' | 'strava' | 'manual'
}
export interface RecordInput {
  timeS?: number
  distanceM?: number
  date?: string
}

export interface ManualActivityInput {
  date: string
  startTime?: string
  sport: Sport
  title?: string
  distanceM?: number
  durationS: number
  avgHR?: number
  maxHR?: number
  avgCadenceSpm?: number
  elevGainM?: number
  elevLossM?: number
}

export type EventKind = 'fatigue' | 'pain' | 'illness' | 'unavailable' | 'forced_rest'
export type EventSeverity = 1 | 2 | 3
export interface CalendarEvent {
  id: number
  kind: EventKind
  startDate: string
  endDate: string
  severity: EventSeverity
  note: string
}
export type CalendarEventInput = Omit<CalendarEvent, 'id'>

export interface PlanPhase {
  id: number
  name: string
  startDate: string
  endDate: string
  focus: string
  weeklyVolumeKm: number | null
  note: string
}

export interface PlanAdaptRequest {
  from: string
  mode: 'rules' | 'llm'
  eventId?: number
}
export interface PlanAdaptPreview {
  sessions: (PlannedSessionInput & { id?: number })[]
  removeIds: number[]
  summary: string
}

/* ---------------------------------- 4.0.0 (§13) ---------------------------------- */

export type JobQueue = 'critical' | 'default' | 'low'
export type JobState = 'queued' | 'running' | 'retrying' | 'completed' | 'failed'
type JobKind =
  | 'sync.provider'
  | 'sync.strava_webhook'
  | 'sync.resync'
  | 'sync.history'
  | 'import.activity'
  | 'llm.analyze'
  | 'llm.plan_generate'
  | 'llm.plan_adapt'
  | 'webhook.deliver'

export interface JobStatus {
  id: string
  userId: number
  kind: JobKind
  queue: JobQueue
  state: JobState
  label: string
  progressPct: number | null
  processed: number | null
  total: number | null
  attempts: number
  maxAttempts: number
  error: string | null
  result: Record<string, unknown> | null
  createdAt: string
  updatedAt: string
}

export interface JobListResponse {
  items: JobStatus[]
}

interface StreamConnectedEvent {
  status: 'ok'
  userId: number
  serverTime: string
}

interface StreamAlertEvent {
  id: string
  activityId: number | null
  date: string
  level: 'info' | 'warn' | 'critical'
  code: string
  title: string
  detail: string
  createdAt: string
}

interface StreamLLMTokenEvent {
  jobId: string
  kind: string
  ref: string
  delta: string
}

interface StreamLLMDoneEvent {
  jobId: string
  kind: string
  ref: string
  error?: string | null
  analysis?: LLMAnalysis | null
  planDrafts?: PlanDraftsResponse | null
  adaptPreview?: PlanAdaptPreview | null
}

export interface StrideStreamEventMap {
  connected: StreamConnectedEvent
  'job.updated': JobStatus
  'sync.updated': SyncStatus
  'alert.triggered': StreamAlertEvent
  'llm.token': StreamLLMTokenEvent
  'llm.done': StreamLLMDoneEvent
}

export type PhaseType = 'foundation' | 'build' | 'threshold' | 'specific' | 'assimilation' | 'taper' | 'recovery'
export type CyclePlanStatus = 'active' | 'completed' | 'archived'

export interface CycleActuals {
  plannedVolumeKm: number
  actualVolumeKm: number
  volumeCompletionPct: number | null
  plannedLoad: number
  actualLoad: number
  loadCompletionPct: number | null
  plannedDurationS: number
  actualDurationS: number
  sessionsPlanned: number
  sessionsCompleted: number
}

export interface TrainingPlan {
  id: number
  goalId: number | null
  name: string
  sport: 'run' | 'bike' | 'other'
  startDate: string
  endDate: string
  targetEventLabel: string
  status: CyclePlanStatus
  note: string
  createdAt: string
  updatedAt: string
}

export interface TrainingPlanInput {
  name: string
  goalId?: number | null
  sport?: 'run' | 'bike' | 'other'
  startDate: string
  endDate: string
  targetEventLabel?: string
  status?: CyclePlanStatus
  note?: string
  autoScaffold?: boolean
  peakWeeklyVolumeKm?: number | null
  peakWeeklyLoad?: number | null
}

export interface TrainingWeek {
  id: number
  planId: number | null
  phaseId: number | null
  weekStart: string
  weekEnd: string
  weekNumber: number
  isAssimilation: boolean
  targetVolumeKm: number | null
  targetLoad: number | null
  targetDurationS: number | null
  note: string
  actuals: CycleActuals
  workouts: PlannedSession[]
}

export interface TrainingWeekInput {
  planId?: number | null
  phaseId?: number | null
  weekStart: string
  isAssimilation?: boolean
  targetVolumeKm?: number | null
  targetLoad?: number | null
  targetDurationS?: number | null
  note?: string
}

export interface TrainingPhase {
  id: number
  planId: number | null
  name: string
  phaseType: PhaseType
  orderIndex: number
  startDate: string
  endDate: string
  focus: string
  weeklyVolumeKm: number | null
  targetWeeklyLoad: number | null
  targetWeeklyDurationS: number | null
  note: string
  actuals: CycleActuals
  weeks: TrainingWeek[]
}

export interface TrainingPhaseInput {
  planId?: number | null
  name: string
  phaseType?: PhaseType
  orderIndex?: number
  startDate: string
  endDate: string
  focus?: string
  weeklyVolumeKm?: number | null
  targetWeeklyLoad?: number | null
  targetWeeklyDurationS?: number | null
  note?: string
}

export interface TrainingCycleOverview {
  plans: TrainingPlan[]
  activePlan: TrainingPlan | null
  phases: TrainingPhase[]
  standaloneWeeks: TrainingWeek[]
  totals: CycleActuals
}

export type BodyZone =
  | 'knee_pfps'
  | 'achilles'
  | 'plantar_fascia'
  | 'shin_splints'
  | 'iliotibial_band'
  | 'hamstrings_calves'
  | 'ankle'
  | 'forefoot'
  | 'hip'
  | 'lower_back'

export type BodySide = 'left' | 'right' | 'both'

export interface BodyStressEntry {
  id: number
  date: string
  activityId: number | null
  activityName: string | null
  zone: BodyZone
  zoneLabel: string
  side: BodySide
  painScore: number
  coldNextDayScore: number | null
  note: string
  createdAt: string
  updatedAt: string
}

export interface BodyStressInput {
  date: string
  activityId?: number | null
  zone: BodyZone
  side?: BodySide
  painScore: number
  coldNextDayScore?: number | null
  note?: string
}

export interface BodyZoneSummary {
  zone: BodyZone
  zoneLabel: string
  latestScore: number | null
  latestColdNextDayScore: number | null
  latestSide: BodySide | null
  latestDate: string | null
  maxScore30d: number | null
  entriesCount90d: number
  trend: 'improving' | 'stable' | 'worsening' | 'none'
}

interface BodyShoeHighlight {
  shoeId: number
  shoeName: string
  dropMm: number | null
  distanceKm: number | null
  painEpisodes: number | null
}

// Every measured value is null when the backend lacks the data to compute it; no fallback is fabricated.
export interface BodyCorrelationFactor {
  zone: BodyZone
  zoneLabel: string
  factor: 'shoe_drop' | 'shoe_wear' | 'elev_loss' | 'gct' | 'impact_load'
  factorLabel: string
  unit: string
  symptomaticMean: number | null
  baselineMean: number | null
  deltaPct: number | null
  symptomaticRuns: number | null
  baselineRuns: number | null
  confidence: 'insufficient' | 'low' | 'moderate' | 'high' | null
  shoeHighlight: BodyShoeHighlight | null
  summary: string | null
  insufficientData?: boolean
}

export interface BodyMapOverview {
  zones: BodyZoneSummary[]
  entries: BodyStressEntry[]
  correlations: BodyCorrelationFactor[]
  windowDays: number
}

export interface ShareLink {
  id: number
  label: string
  tokenPrefix: string
  url: string
  expiresAt: string | null
  revoked: boolean
  revokedAt: string | null
  accessCount: number
  lastAccessedAt: string | null
  createdAt: string
  periodWeeks: SharePeriodWeeks
  sections: ShareSection[]
  anonymize: boolean
}

export type ShareSection = 'activities' | 'health' | 'recovery' | 'load' | 'records' | 'plan' | 'shoes' | 'notes'
export type ShareExpiryDays = 1 | 7 | 30 | 90
export type SharePeriodWeeks = 4 | 8 | 12 | 26 | 52

export interface ShareLinkInput {
  label: string
  expiresInDays: ShareExpiryDays | null
  periodWeeks: SharePeriodWeeks
  sections: ShareSection[]
  anonymize: boolean
}

export interface PublicActivityItem {
  id: number
  date: string
  startLocal: string
  name: string
  sport: string
  typeLabel: string
  distanceM: number | null
  durationS: number | null
  avgPaceSecPerKm: number | null
  gapSecPerKm: number | null
  avgHR: number | null
  avgCadenceSpm: number | null
  gctMs: number | null
  elevGainM: number | null
  elevLossM: number | null
  trainingLoad: number | null
  impactLoad: number | null
  jointIndex: number | null
  efSpeed: number | null
  shoeName: string | null
  shoeDropMm: number | null
  painDuring?: number | null
  painNextMorning?: number | null
  note?: string | null
}

export interface PublicLoadPoint {
  date: string
  acute: number
  chronic: number
  acwr: number | null
  efSpeed: number | null
  impactLoad7d: number | null
  jointIndex7d: number | null
}

interface PublicShoeItem {
  name: string
  brand: string
  model: string
  dropMm: number | null
  type: 'soft' | 'tempo' | 'carbon'
  distanceKm: number
  retired: boolean
}

// Sections the link does not allow are absent from the payload (never empty placeholders).
export interface PublicShareView {
  generatedAt: string
  shareLabel: string
  expiresAt: string | null
  periodWeeks: SharePeriodWeeks
  sections: ShareSection[]
  athlete: {
    displayName: string
    focusArea?: string
    focusLabel?: string
    injuredSide?: 'left' | 'right' | 'both' | null
  }
  readiness?: ReadinessBanner
  loadSummary?: {
    acute: number
    chronic: number
    acwr: number | null
    verdict: 'low' | 'optimal' | 'high' | 'danger' | null
    efLatest: number | null
    efTrendPct: number | null
    impactLoad7d: number | null
    jointIndex7d: number | null
  }
  loadSeries?: PublicLoadPoint[]
  cycles?: TrainingCycleOverview
  recentSessions?: PublicActivityItem[]
  bodyMap?: BodyMapOverview
  records?: PersonalRecord[]
  shoes?: PublicShoeItem[]
}

/* ---------------------------------- 4.1.0 (§14) ---------------------------------- */

export interface HealthZone {
  zone: BodyZone
  side: BodySide
  since: string | null
  note: string
  /** Catalog key of the zone's condition (§14.7); `other` requires `customLabel`. */
  condition?: string | null
  /** Free text (1–80 chars) describing an `other` condition; rendered as text only. */
  customLabel?: string | null
  /** Derived, read only: catalog label or custom label. */
  conditionLabel?: string | null
}

/** One pathology of the backend catalog (GET /api/health/conditions). */
export interface HealthCondition {
  key: string
  label: string
}

export interface HealthConditionZone {
  zone: BodyZone
  zoneLabel: string
  /** Zone-specific conditions, « Autre (préciser) » (`other`) last. */
  conditions: HealthCondition[]
}

export interface HealthConditionCatalog {
  zones: HealthConditionZone[]
}

export interface HealthStatusInput {
  mode: 'performance' | 'tracking'
  zones: HealthZone[]
}

export interface HealthStatus extends HealthStatusInput {
  focusArea: FocusArea
  updatedAt: string | null
}

export interface HistorySyncRequest {
  from: string | null
}

export interface ActivitySearchHit {
  id: number
  date: string
  startLocal: string
  name: string
  sport: Sport
  typeLabel: string
  distanceM: number | null
  durationS: number | null
}

export interface SearchResponse {
  query: string
  items: ActivitySearchHit[]
}
