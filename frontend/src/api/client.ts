import type {
  ActivityAnalysis,
  ActivityListResponse,
  ApiErrorBody,
  ConnectionTestResult,
  DayAnalysis,
  ErrorCode,
  HealthResponse,
  Mode,
  Settings,
  SettingsUpdate,
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

export function isAuthError(err: unknown): err is ApiError {
  return err instanceof ApiError && (err.code === 'NO_API_KEY' || err.code === 'GARMIN_UNAUTHORIZED')
}

/**
 * Transport unique : réseau réel, ou fixtures si VITE_MOCK=1.
 * `IS_MOCK` est remplacé statiquement au build : en production, la branche
 * mock (et donc le chunk src/mocks) est éliminée.
 */
async function transport(path: string, init?: RequestInit): Promise<Response> {
  if (IS_MOCK) {
    const { mockFetch } = await import('../mocks/server')
    return mockFetch(path, init)
  }
  return fetch(path, init)
}

function isErrorBody(v: unknown): v is ApiErrorBody {
  return typeof v === 'object' && v !== null && 'error' in v && typeof (v as ApiErrorBody).error === 'string'
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await transport(path, {
      ...init,
      headers: { Accept: 'application/json', ...(init?.body ? { 'Content-Type': 'application/json' } : {}), ...init?.headers },
    })
  } catch {
    throw new ApiError('Serveur Stride injoignable. Vérifie que le backend tourne.', 0, null)
  }

  const text = await res.text()
  let body: unknown = null
  if (text) {
    try {
      body = JSON.parse(text)
    } catch {
      body = null
    }
  }

  if (!res.ok) {
    if (isErrorBody(body)) throw new ApiError(body.error, res.status, body.code ?? null)
    throw new ApiError(`Erreur ${res.status} du serveur Stride.`, res.status, null)
  }
  return body as T
}

function qs(params: Record<string, string | number | undefined | null>): string {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
  }
  const s = sp.toString()
  return s ? `?${s}` : ''
}

export interface ListActivitiesParams {
  limit?: number
  start?: number
  from?: string
  to?: string
}

export const api = {
  health: () => request<HealthResponse>('/api/health'),

  getSettings: () => request<Settings>('/api/settings'),

  /** La clé API n'est jamais conservée côté client : elle ne transite que dans ce body. */
  updateSettings: (body: SettingsUpdate) =>
    request<Settings>('/api/settings', { method: 'PUT', body: JSON.stringify(body) }),

  testConnection: () => request<ConnectionTestResult>('/api/settings/test', { method: 'POST' }),

  listActivities: (p: ListActivitiesParams = {}) =>
    request<ActivityListResponse>(`/api/activities${qs({ limit: p.limit, start: p.start, from: p.from, to: p.to })}`),

  activityAnalysis: (id: number, mode: Mode, history: number) =>
    request<ActivityAnalysis>(`/api/activities/${id}/analysis${qs({ mode, history: history > 0 ? history : undefined })}`),

  dayAnalysis: (date: string, history: number) =>
    request<DayAnalysis>(`/api/days/${date}/analysis${qs({ history: history > 0 ? history : undefined })}`),
}
