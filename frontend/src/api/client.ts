import type {
  ActivitySummary,
  HealthConditionCatalog,
  HealthStatus,
  HealthStatusInput,
  HistorySyncRequest,
  SearchResponse,
  CalendarEvent,
  CalendarEventInput,
  ManualActivityInput,
  PersonalRecord,
  PlanAdaptPreview,
  PlanAdaptRequest,
  PlanPhase,
  RecordInput,
  RecordKey,
  BuildInfo,
  LogFilter,
  LogPage,
  ActivityListResponse,
  ActivityNotes,
  ActivityTrack,
  CalendarDay,
  EfficiencyTrend,
  ImportResult,
  LLMAnalysis,
  LLMAnalyzeRequest,
  LLMConfig,
  LLMConfigInput,
  LLMTestResult,
  NotificationInput,
  NotificationTarget,
  NotificationTestResult,
  PlanDraftsResponse,
  PlanGenerateRequest,
  PlanPromptResponse,
  ActivityNotesInput,
  ActivityView,
  AdminConnection,
  AdminJob,
  AdminJobFilter,
  AdminJobList,
  AdminOverview,
  AdminSettings,
  AdminShareLink,
  AdminUser,
  FeedbackCategory,
  FeedbackInput,
  FeedbackItem,
  FeedbackList,
  FeedbackStatus,
  IntegrityResult,
  OptimizeResult,
  PurgeResult,
  ApiErrorBody,
  AthleteProfile,
  AuthState,
  DailyCheckin,
  DailyCheckinInput,
  DayView,
  ErrorCode,
  FocusPreset,
  Goal,
  GoalInput,
  IncludeKey,
  Integrations,
  Invitation,
  InvitationCreated,
  Me,
  MeterSnapshot,
  MultiExportRequest,
  MultiExportResponse,
  Passkey,
  PasskeyBegin,
  PlannedSession,
  PlannedSessionInput,
  RestoreResult,
  ShareLink,
  ShareLinkInput,
  PublicShareView,
  Shoe,
  ShoeInput,
  SyncStatus,
  TrainingCycleOverview,
  TrainingPhase,
  TrainingPhaseInput,
  TrainingPlan,
  TrainingPlanInput,
  TrainingWeek,
  TrainingWeekInput,
  WeekPlan,
  BodyMapOverview,
  BodyStressEntry,
  BodyStressInput,
  BodyZone,
  JobListResponse,
  JobStatus,
  BackupList,
  MorphPreview,
  PublicActivityDetail,
  S3Config,
  S3ConfigInput,
  S3TestResult,
  ShareDirectiveInput,
  ShareNote,
  ShareRemarkInput,
  McpStatus,
  McpToken,
  McpQuota,
  McpUsage,
} from './types'
import { getLang } from '../i18n/lang'
import { t } from '../i18n/t'

export const IS_MOCK = import.meta.env.VITE_MOCK === '1'

export class ApiError extends Error {
  readonly status: number
  readonly code: ErrorCode | null

  constructor(message: string, status: number, code: ErrorCode | null) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

export const isApiError = (e: unknown, code?: ErrorCode): e is ApiError => e instanceof ApiError && (code === undefined || e.code === code)

/**
 * Message shown for an API error (§15.2: codes are stable, the UI translates by code). French keeps the
 * server's own (more specific, French) message; English uses the code's generic translation.
 */
export function apiErrorMessage(serverMessage: string, code: ErrorCode | null): string {
  if (getLang() === 'fr' && serverMessage) return serverMessage
  return code ? t(`errors.code.${code}`) : serverMessage
}

const SAFE_METHODS = new Set(['GET', 'HEAD', 'OPTIONS'])

async function transport(path: string, init: RequestInit): Promise<Response> {
  if (import.meta.env.VITE_MOCK === '1') {
    const { mockFetch } = await import('../mocks/server')
    return mockFetch(path, init)
  }
  return fetch(path, init)
}

function isErrorBody(v: unknown): v is ApiErrorBody {
  return typeof v === 'object' && v !== null && 'error' in v && typeof (v as ApiErrorBody).error === 'string'
}

type Body = unknown

async function send(path: string, opts: { method?: string; body?: Body; accept?: string; rawBody?: BodyInit; form?: FormData } = {}): Promise<Response> {
  const method = (opts.method ?? 'GET').toUpperCase()
  const headers: Record<string, string> = { Accept: opts.accept ?? 'application/json', 'Accept-Language': getLang() }
  if (!SAFE_METHODS.has(method)) headers['X-Stride-CSRF'] = '1'
  let body: BodyInit | undefined = opts.rawBody
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(opts.body)
  }
  if (opts.form) body = opts.form
  let res: Response
  try {
    res = await transport(path, { method, headers, body, credentials: 'same-origin' })
  } catch {
    throw new ApiError(t('errors.http.network'), 0, null)
  }
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    let parsed: unknown = null
    try {
      parsed = text ? JSON.parse(text) : null
    } catch {
      parsed = null
    }
    if (isErrorBody(parsed)) throw new ApiError(apiErrorMessage(parsed.error, parsed.code ?? null), res.status, parsed.code ?? null)
    const fallback =
      res.status === 401
        ? t('errors.http.unauthorized')
        : res.status === 403
          ? t('errors.http.forbidden')
          : res.status === 429
            ? t('errors.http.rateLimited')
            : t('errors.http.status', { status: res.status })
    throw new ApiError(fallback, res.status, res.status === 401 ? 'UNAUTHORIZED' : res.status === 403 ? 'FORBIDDEN' : null)
  }
  return res
}

async function json<T>(path: string, opts: { method?: string; body?: Body } = {}): Promise<T> {
  const res = await send(path, opts)
  if (res.status === 204) return undefined as T
  const text = await res.text()
  return (text ? JSON.parse(text) : undefined) as T
}

async function text(path: string): Promise<string> {
  const res = await send(path, { accept: 'text/markdown, text/plain' })
  return res.text()
}

function qs(params: Record<string, string | number | undefined | null>): string {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
  const s = sp.toString()
  return s ? `?${s}` : ''
}

function filenameFrom(res: Response, fallback: string): string {
  const cd = res.headers.get('Content-Disposition') ?? ''
  const m = /filename\*?=(?:UTF-8'')?"?([^";]+)"?/i.exec(cd)
  return m ? decodeURIComponent(m[1]) : fallback
}

export interface ListActivitiesParams {
  limit?: number
  before?: string
  from?: string
  to?: string
  sport?: string
}

const includeParam = (include: IncludeKey[]) => (include.length ? include.join(',') : 'none')

export const api = {
  authState: () => json<AuthState>('/api/auth/state'),
  registerBegin: (body: { displayName: string; inviteCode?: string }) => json<PasskeyBegin<unknown>>('/api/auth/register/begin', { method: 'POST', body }),
  registerFinish: (credential: unknown) => json<Me>('/api/auth/register/finish', { method: 'POST', body: credential }),
  loginBegin: () => json<PasskeyBegin<unknown>>('/api/auth/login/begin', { method: 'POST', body: {} }),
  loginFinish: (credential: unknown) => json<Me>('/api/auth/login/finish', { method: 'POST', body: credential }),
  logout: () => json<void>('/api/auth/logout', { method: 'POST' }),

  me: () => json<Me>('/api/me'),
  updateMe: (body: { displayName?: string; onboardingCompleted?: boolean }) => json<Me>('/api/me', { method: 'PATCH', body }),
  deleteMe: () => json<void>('/api/me', { method: 'DELETE' }),
  passkeys: () => json<Passkey[]>('/api/me/passkeys'),
  passkeyBegin: () => json<PasskeyBegin<unknown>>('/api/me/passkeys/begin', { method: 'POST', body: {} }),
  passkeyFinish: (credential: unknown) => json<Passkey>('/api/me/passkeys/finish', { method: 'POST', body: credential }),
  renamePasskey: (id: string, name: string) => json<Passkey>(`/api/me/passkeys/${encodeURIComponent(id)}`, { method: 'PATCH', body: { name } }),
  deletePasskey: (id: string) => json<void>(`/api/me/passkeys/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  adminUsers: () => json<AdminUser[]>('/api/admin/users'),
  adminUpdateUser: (id: number, body: { status?: AdminUser['status']; role?: AdminUser['role'] }) => json<AdminUser>(`/api/admin/users/${id}`, { method: 'PATCH', body }),
  adminDeleteUser: (id: number) => json<void>(`/api/admin/users/${id}`, { method: 'DELETE' }),
  adminInvitations: () => json<Invitation[]>('/api/admin/invitations'),
  adminCreateInvitation: (body: { label?: string; maxUses?: number; expiresInHours?: number }) => json<InvitationCreated>('/api/admin/invitations', { method: 'POST', body }),
  adminRevokeInvitation: (id: number) => json<void>(`/api/admin/invitations/${id}`, { method: 'DELETE' }),
  adminSettings: () => json<AdminSettings>('/api/admin/settings'),
  adminUpdateSettings: (body: AdminSettings) => json<AdminSettings>('/api/admin/settings', { method: 'PUT', body }),
  adminMeter: () => json<MeterSnapshot>('/api/admin/meter'),
  adminMcpUsage: () => json<McpUsage>('/api/admin/mcp/usage'),
  adminMcpQuota: (q: McpQuota) => json<McpQuota>('/api/admin/mcp/quota', { method: 'PUT', body: q }),
  adminLogs: (f: LogFilter, before?: number) => {
    const q = new URLSearchParams({ category: f.category, level: f.level, limit: '100' })
    if (f.userId) q.set('userId', String(f.userId))
    if (before) q.set('before', String(before))
    return json<LogPage>(`/api/admin/logs?${q.toString()}`)
  },
  version: () => json<BuildInfo>('/api/version'),
  trackAction: (action: 'prompt.copy' | 'prompt.download' | 'prompt.share') => json<void>('/api/actions', { method: 'POST', body: { action } }),
  adminFlushCache: () => json<{ removed: number }>('/api/admin/cache/flush', { method: 'POST' }),
  adminConnections: () => json<AdminConnection[]>('/api/admin/connections'),
  adminResync: (id: number) => json<void>(`/api/admin/users/${id}/resync`, { method: 'POST' }),
  adminBackup: async (id: number) => {
    const res = await send(`/api/admin/users/${id}/backup`)
    return { blob: await res.blob(), filename: filenameFrom(res, `stride-backup-user-${id}.json`) }
  },
  adminRestore: (id: number, mode: 'merge' | 'replace', backup: string) =>
    send(`/api/admin/users/${id}/restore${qs({ mode })}`, { method: 'POST', rawBody: backup, accept: 'application/json' }).then((r) => r.json() as Promise<RestoreResult>),
  // 4.1 control center (§14.6)
  adminOverview: () => json<AdminOverview>('/api/admin/overview'),
  adminRevokeSessions: (id: number) => json<{ revoked: number }>(`/api/admin/users/${id}/sessions`, { method: 'DELETE' }),
  adminSyncUser: (id: number) => json<AdminJob>(`/api/admin/users/${id}/sync`, { method: 'POST' }),
  adminJobs: (state: AdminJobFilter) => json<AdminJobList>(`/api/admin/jobs${qs({ state, limit: 100 })}`),
  adminRetryJob: (id: string) => json<AdminJob>(`/api/admin/jobs/${encodeURIComponent(id)}/retry`, { method: 'POST' }),
  adminCancelJob: (id: string) => json<AdminJob>(`/api/admin/jobs/${encodeURIComponent(id)}/cancel`, { method: 'POST' }),
  adminShares: (state: 'active' | 'all') => json<AdminShareLink[]>(`/api/admin/shares${qs({ state })}`),
  adminRevokeShare: (id: number) => json<void>(`/api/admin/shares/${id}`, { method: 'DELETE' }),
  adminPurge: () => json<PurgeResult>('/api/admin/maintenance/purge', { method: 'POST' }),
  adminOptimize: (vacuum: boolean) => json<OptimizeResult>('/api/admin/maintenance/optimize', { method: 'POST', body: { vacuum } }),
  adminCheck: () => json<IntegrityResult>('/api/admin/maintenance/check', { method: 'POST' }),
  sendFeedback: (body: FeedbackInput) => json<{ id: number; createdAt: string }>('/api/feedback', { method: 'POST', body }),
  adminFeedback: (status: FeedbackStatus | 'all', category: FeedbackCategory | 'all') => json<FeedbackList>(`/api/admin/feedback${qs({ status, category })}`),
  adminUpdateFeedback: (id: number, body: { status?: FeedbackStatus; adminNote?: string }) => json<FeedbackItem>(`/api/admin/feedback/${id}`, { method: 'PATCH', body }),

  integrations: () => json<Integrations>('/api/integrations'),
  connectGarmin: (apiKey: string) => json<Integrations>('/api/integrations/garmin', { method: 'PUT', body: { apiKey } }),
  testGarmin: () => json<Integrations | { ok: boolean; message?: string }>('/api/integrations/garmin/test', { method: 'POST' }),
  disconnectGarmin: () => json<Integrations | void>('/api/integrations/garmin', { method: 'DELETE' }),
  disconnectStrava: () => json<Integrations | void>('/api/integrations/strava', { method: 'DELETE' }),
  stravaConnectUrl: (returnPath: string) => `/api/integrations/strava/connect${qs({ return: returnPath })}`,

  startSync: () => json<SyncStatus>('/api/sync', { method: 'POST' }),
  syncStatus: () => json<SyncStatus>('/api/sync/status'),

  activities: (p: ListActivitiesParams = {}) => json<ActivityListResponse>(`/api/activities${qs({ limit: p.limit, before: p.before, from: p.from, to: p.to, sport: p.sport })}`),
  activity: (id: number) => json<ActivityView>(`/api/activities/${id}`),
  activityReport: (id: number, include: IncludeKey[]) => text(`/api/activities/${id}/report${qs({ include: includeParam(include) })}`),
  saveNotes: (id: number, body: ActivityNotesInput) => json<ActivityNotes>(`/api/activities/${id}/notes`, { method: 'PUT', body }),

  day: (date: string) => json<DayView>(`/api/days/${date}`),
  dayReport: (date: string, include: IncludeKey[]) => text(`/api/days/${date}/report${qs({ include: includeParam(include) })}`),
  saveCheckin: (date: string, body: DailyCheckinInput) => json<DailyCheckin>(`/api/days/${date}/checkin`, { method: 'PUT', body }),

  profile: () => json<AthleteProfile>('/api/profile'),
  saveProfile: (body: AthleteProfile) => json<AthleteProfile>('/api/profile', { method: 'PUT', body }),
  presets: () => json<FocusPreset[]>('/api/profile/presets'),

  goals: () => json<Goal[]>('/api/goals'),
  createGoal: (body: GoalInput) => json<Goal>('/api/goals', { method: 'POST', body }),
  updateGoal: (id: number, body: Partial<GoalInput>) => json<Goal>(`/api/goals/${id}`, { method: 'PATCH', body }),
  deleteGoal: (id: number) => json<void>(`/api/goals/${id}`, { method: 'DELETE' }),

  plan: (from: string, to: string) => json<PlannedSession[]>(`/api/plan${qs({ from, to })}`),
  createPlanned: (body: PlannedSessionInput) => json<PlannedSession>('/api/plan', { method: 'POST', body }),
  updatePlanned: (id: number, body: Partial<PlannedSessionInput>) => json<PlannedSession>(`/api/plan/${id}`, { method: 'PATCH', body }),
  deletePlanned: (id: number) => json<void>(`/api/plan/${id}`, { method: 'DELETE' }),
  week: (date: string) => json<WeekPlan>(`/api/plan/week${qs({ date })}`),

  shoes: () => json<Shoe[]>('/api/shoes'),
  createShoe: (body: ShoeInput) => json<Shoe>('/api/shoes', { method: 'POST', body }),
  updateShoe: (id: number, body: Partial<ShoeInput>) => json<Shoe>(`/api/shoes/${id}`, { method: 'PATCH', body }),
  deleteShoe: (id: number) => json<void>(`/api/shoes/${id}`, { method: 'DELETE' }),
  importStravaShoes: () => json<Shoe[]>('/api/shoes/import-strava', { method: 'POST' }),

  multiExport: (body: MultiExportRequest) => json<MultiExportResponse>('/api/exports/multi', { method: 'POST', body }),

  backup: async () => {
    const res = await send('/api/backup')
    return { blob: await res.blob(), filename: filenameFrom(res, 'stride-backup.json') }
  },
  restore: (mode: 'merge' | 'replace', backup: string) =>
    send(`/api/backup/restore${qs({ mode })}`, { method: 'POST', rawBody: backup }).then((r) => r.json() as Promise<RestoreResult>),

  llm: () => json<LLMConfig>('/api/llm'),
  saveLlm: (body: LLMConfigInput) => json<LLMConfig>('/api/llm', { method: 'PUT', body }),
  deleteLlm: () => json<void>('/api/llm', { method: 'DELETE' }),
  testLlm: () => json<LLMTestResult>('/api/llm/test', { method: 'POST' }),
  analyses: (kind: string, ref: string) => json<LLMAnalysis[]>(`/api/llm/analyses${qs({ kind, ref })}`),

  planGenerate: <T extends PlanPromptResponse | PlanDraftsResponse>(body: PlanGenerateRequest) => json<T>('/api/plan/generate', { method: 'POST', body }),
  planImport: (text: string) => json<PlanDraftsResponse>('/api/plan/import', { method: 'POST', body: { text } }),
  planBulk: (sessions: PlannedSessionInput[], phases?: PlanPhase[]) => json<PlannedSession[]>('/api/plan/bulk', { method: 'POST', body: phases?.length ? { sessions, phases } : { sessions } }),
  pushGarmin: (id: number) => json<PlannedSession>(`/api/plan/${id}/push-garmin`, { method: 'POST' }),
  workoutFitUrl: (id: number) => `/api/plan/${id}/workout.fit`,
  workoutFit: async (id: number) => {
    const res = await send(`/api/plan/${id}/workout.fit`, { accept: 'application/octet-stream' })
    return { blob: await res.blob(), filename: filenameFrom(res, `stride-workout-${id}.fit`) }
  },
  calendar: (from: string, to: string) => json<{ days: CalendarDay[] }>(`/api/calendar${qs({ from, to })}`),

  efficiencyTrend: (weeks = 12) => json<EfficiencyTrend>(`/api/trends/efficiency${qs({ weeks })}`),
  track: (id: number) => json<ActivityTrack>(`/api/activities/${id}/track`),

  notifications: () => json<NotificationTarget[]>('/api/notifications'),
  createNotification: (body: NotificationInput) => json<NotificationTarget>('/api/notifications', { method: 'POST', body }),
  updateNotification: (id: number, body: Partial<NotificationInput>) => json<NotificationTarget>(`/api/notifications/${id}`, { method: 'PATCH', body }),
  deleteNotification: (id: number) => json<void>(`/api/notifications/${id}`, { method: 'DELETE' }),
  testNotification: (id: number) => json<NotificationTestResult>(`/api/notifications/${id}/test`, { method: 'POST' }),

  mergeShoe: (id: number, intoId: number) => json<Shoe | void>(`/api/shoes/${id}/merge`, { method: 'POST', body: { intoId } }),

  records: () => json<{ records: PersonalRecord[] }>('/api/records'),
  saveRecord: (key: RecordKey, body: RecordInput) => json<PersonalRecord>(`/api/records/${key}`, { method: 'PUT', body }),
  deleteRecord: (key: RecordKey) => json<void>(`/api/records/${key}`, { method: 'DELETE' }),

  createActivity: (body: ManualActivityInput) => json<ActivitySummary>('/api/activities', { method: 'POST', body }),
  updateActivity: (id: number, body: Partial<ManualActivityInput>) => json<ActivitySummary>(`/api/activities/${id}`, { method: 'PATCH', body }),
  deleteActivity: (id: number) => json<void>(`/api/activities/${id}`, { method: 'DELETE' }),

  events: async (from: string, to: string) => {
    const r = await json<CalendarEvent[] | { events: CalendarEvent[] }>(`/api/calendar/events${qs({ from, to })}`)
    return Array.isArray(r) ? r : (r?.events ?? [])
  },
  createEvent: (body: CalendarEventInput) => json<CalendarEvent>('/api/calendar/events', { method: 'POST', body }),
  updateEvent: (id: number, body: Partial<CalendarEventInput>) => json<CalendarEvent>(`/api/calendar/events/${id}`, { method: 'PATCH', body }),
  deleteEvent: (id: number) => json<void>(`/api/calendar/events/${id}`, { method: 'DELETE' }),

  shiftPlanned: (id: number, body: { days?: number; toDate?: string }) => json<PlannedSession>(`/api/plan/${id}/shift`, { method: 'POST', body }),
  planAdapt: (body: PlanAdaptRequest) => json<PlanAdaptPreview>('/api/plan/adapt', { method: 'POST', body }),
  planReplace: (body: { removeIds: number[]; sessions: PlannedSessionInput[] }) => json<PlannedSession[]>('/api/plan/replace', { method: 'POST', body }),
  planPhases: () => json<PlanPhase[]>('/api/plan/phases'),

  /* 4.0.0 endpoints */
  jobs: (limit = 20) => json<JobListResponse>(`/api/jobs${qs({ limit })}`),
  job: (id: string) => json<JobStatus>(`/api/jobs/${encodeURIComponent(id)}`),
  importFileAsync: async (file: File, asyncMode = true) => {
    const form = new FormData()
    form.append('file', file)
    const res = await send(`/api/imports${qs({ async: asyncMode ? 1 : undefined })}`, { method: 'POST', form })
    return (await res.json()) as ImportResult | JobStatus
  },
  analyzeStream: (body: LLMAnalyzeRequest, asyncMode = true) =>
    json<LLMAnalysis | JobStatus>(`/api/llm/analyze${qs({ async: asyncMode ? 1 : undefined })}`, { method: 'POST', body }),
  planGenerateStream: (body: PlanGenerateRequest, asyncMode = body.mode === 'llm') =>
    json<PlanPromptResponse | PlanDraftsResponse | JobStatus>(`/api/plan/generate${qs({ async: asyncMode ? 1 : undefined })}`, { method: 'POST', body }),
  planAdaptStream: (body: PlanAdaptRequest, asyncMode = body.mode === 'llm') =>
    json<PlanAdaptPreview | JobStatus>(`/api/plan/adapt${qs({ async: asyncMode ? 1 : undefined })}`, { method: 'POST', body }),

  cycles: (planId?: number | null) => json<TrainingCycleOverview>(`/api/cycles${qs({ planId })}`),
  createCyclePlan: (body: TrainingPlanInput) => json<TrainingCycleOverview>('/api/cycles/plans', { method: 'POST', body }),
  updateCyclePlan: (id: number, body: Partial<TrainingPlanInput>) => json<TrainingPlan>(`/api/cycles/plans/${id}`, { method: 'PATCH', body }),
  deleteCyclePlan: (id: number) => json<void>(`/api/cycles/plans/${id}`, { method: 'DELETE' }),
  createCyclePhase: (body: TrainingPhaseInput) => json<TrainingPhase>('/api/cycles/phases', { method: 'POST', body }),
  updateCyclePhase: (id: number, body: Partial<TrainingPhaseInput>) => json<TrainingPhase>(`/api/cycles/phases/${id}`, { method: 'PATCH', body }),
  deleteCyclePhase: (id: number) => json<void>(`/api/cycles/phases/${id}`, { method: 'DELETE' }),
  upsertCycleWeek: (body: TrainingWeekInput) => json<TrainingWeek>('/api/cycles/weeks', { method: 'PUT', body }),

  bodyMap: (days = 90, zone?: BodyZone | null) => json<BodyMapOverview>(`/api/bodymap${qs({ days, zone })}`),
  createBodyEntry: (body: BodyStressInput) => json<BodyStressEntry>('/api/bodymap/entries', { method: 'POST', body }),
  updateBodyEntry: (id: number, body: Partial<BodyStressInput>) => json<BodyStressEntry>(`/api/bodymap/entries/${id}`, { method: 'PATCH', body }),
  deleteBodyEntry: (id: number) => json<void>(`/api/bodymap/entries/${id}`, { method: 'DELETE' }),

  shareLinks: () => json<ShareLink[]>('/api/share'),
  createShareLink: (body: ShareLinkInput) => json<ShareLink>('/api/share', { method: 'POST', body }),
  revokeShareLink: (id: number) => json<void>(`/api/share/${id}`, { method: 'DELETE' }),
  updateShareLink: (id: number, body: Partial<ShareLinkInput>) => json<ShareLink>(`/api/share/${id}`, { method: 'PATCH', body }),

  /* 4.1.0 endpoints */
  healthStatus: () => json<HealthStatus>('/api/health/status'),
  saveHealthStatus: (body: HealthStatusInput) => json<HealthStatus>('/api/health/status', { method: 'PUT', body }),
  healthConditions: () => json<HealthConditionCatalog>('/api/health/conditions'),
  syncHistory: (body: HistorySyncRequest) => json<JobStatus>('/api/sync/history', { method: 'POST', body }),
  search: (q: string, limit = 20) => json<SearchResponse>(`/api/search${qs({ q, limit })}`),
  publicShare: (token: string) => json<PublicShareView>(`/api/public/share/${encodeURIComponent(token)}`),

  /* 4.2.0 endpoints (§15) */
  shareNotes: (active: boolean) => json<ShareNote[]>(`/api/share/notes${qs({ active: active ? 1 : undefined })}`),
  deleteShareNote: (id: number) => json<void>(`/api/share/notes/${id}`, { method: 'DELETE' }),
  publicPutDirective: (token: string, body: ShareDirectiveInput) => json<ShareNote>(`${pub(token)}/directive`, { method: 'PUT', body }),
  publicDeleteDirective: (token: string) => json<void>(`${pub(token)}/directive`, { method: 'DELETE' }),
  publicPutRemark: (token: string, activityId: number, body: ShareRemarkInput) => json<ShareNote>(`${pub(token)}/activities/${activityId}/remark`, { method: 'PUT', body }),
  publicDeleteRemark: (token: string, activityId: number) => json<void>(`${pub(token)}/activities/${activityId}/remark`, { method: 'DELETE' }),
  publicActivity: (token: string, activityId: number) => json<PublicActivityDetail>(`${pub(token)}/activities/${activityId}`),
  publicCreatePlanned: (token: string, body: PlannedSessionInput) => json<PlannedSession>(`${pub(token)}/plan`, { method: 'POST', body }),
  publicUpdatePlanned: (token: string, id: number, body: Partial<PlannedSessionInput>) => json<PlannedSession>(`${pub(token)}/plan/${id}`, { method: 'PATCH', body }),
  publicDeletePlanned: (token: string, id: number) => json<void>(`${pub(token)}/plan/${id}`, { method: 'DELETE' }),
  publicCreateEvent: (token: string, body: CalendarEventInput) => json<CalendarEvent>(`${pub(token)}/events`, { method: 'POST', body }),
  publicUpdateEvent: (token: string, id: number, body: Partial<CalendarEventInput>) => json<CalendarEvent>(`${pub(token)}/events/${id}`, { method: 'PATCH', body }),
  publicDeleteEvent: (token: string, id: number) => json<void>(`${pub(token)}/events/${id}`, { method: 'DELETE' }),

  adminBackups: () => json<BackupList>('/api/admin/backups'),
  adminCreateBackup: (upload: boolean) => json<JobStatus>('/api/admin/backups', { method: 'POST', body: { upload } }),
  adminDownloadBackup: async (name: string) => {
    const res = await send(`/api/admin/backups/${encodeURIComponent(name)}/download`, { accept: 'application/gzip, application/octet-stream' })
    return { blob: await res.blob(), filename: filenameFrom(res, name) }
  },
  adminUploadBackup: (name: string) => json<JobStatus>(`/api/admin/backups/${encodeURIComponent(name)}/upload`, { method: 'POST' }),
  adminDeleteBackup: (name: string, remote: boolean) => json<void>(`/api/admin/backups/${encodeURIComponent(name)}${qs({ remote: remote ? 1 : 0 })}`, { method: 'DELETE' }),
  adminS3Config: () => json<S3Config>('/api/admin/backups/s3'),
  adminSaveS3Config: (body: S3ConfigInput) => json<S3Config>('/api/admin/backups/s3', { method: 'PUT', body }),
  adminTestS3: (config?: S3ConfigInput) => json<S3TestResult>('/api/admin/backups/s3/test', { method: 'POST', body: config ? { config } : {} }),

  morphPreview: (id: number) => json<MorphPreview>(`/api/plan/${id}/morph`),
  morphRefine: (id: number, note?: string) => json<JobStatus>(`/api/plan/${id}/morph/refine`, { method: 'POST', body: note ? { note } : {} }),
  morphApply: (id: number, proposal: PlannedSession) => json<PlannedSession>(`/api/plan/${id}/morph/apply`, { method: 'POST', body: { proposal } }),
  morphRevert: (id: number) => json<PlannedSession>(`/api/plan/${id}/morph/revert`, { method: 'POST' }),

  /* 4.3.0 endpoints (§MCP) */
  mcpStatus: () => json<McpStatus>('/api/mcp'),
  mcpSetEnabled: (enabled: boolean) => json<void>('/api/mcp', { method: 'PUT', body: { enabled } }),
  mcpGenerateToken: () => json<McpToken>('/api/mcp/token', { method: 'POST' }),
  mcpRevokeToken: () => json<void>('/api/mcp/token', { method: 'DELETE' }),
}

function pub(token: string) {
  return `/api/public/share/${encodeURIComponent(token)}`
}

/** Full-page navigation to the Strava OAuth flow (the mock resolves the redirect locally). */
export async function goToStravaConnect(returnPath: string) {
  const url = api.stravaConnectUrl(returnPath)
  if (IS_MOCK) {
    const res = await transport(url, { method: 'GET', credentials: 'same-origin' })
    window.location.assign(res.headers.get('Location') ?? returnPath)
    return
  }
  window.location.assign(url)
}
