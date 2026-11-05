import type {
  ActivitySummary,
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
  AdminSettings,
  AdminUser,
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
  Shoe,
  ShoeInput,
  SyncStatus,
  WeekPlan,
} from './types'

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
  const headers: Record<string, string> = { Accept: opts.accept ?? 'application/json' }
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
    throw new ApiError('Serveur Stride injoignable. Vérifie ta connexion.', 0, null)
  }
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    let parsed: unknown = null
    try {
      parsed = text ? JSON.parse(text) : null
    } catch {
      parsed = null
    }
    if (isErrorBody(parsed)) throw new ApiError(parsed.error, res.status, parsed.code ?? null)
    const fallback =
      res.status === 401 ? 'Session expirée : reconnecte-toi.' : res.status === 403 ? 'Accès refusé.' : res.status === 429 ? 'Trop de requêtes, réessaie dans un instant.' : `Erreur ${res.status} du serveur Stride.`
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
  analyze: (body: LLMAnalyzeRequest) => json<LLMAnalysis>('/api/llm/analyze', { method: 'POST', body }),
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

  importFile: async (file: File) => {
    const form = new FormData()
    form.append('file', file)
    const res = await send('/api/imports', { method: 'POST', form })
    return (await res.json()) as ImportResult
  },
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
