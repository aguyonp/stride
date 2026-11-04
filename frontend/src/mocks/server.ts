// Faux backend pour `npm run dev:mock` : renvoie de vraies `Response` pour exercer
// la même couche client que le réseau.
//
// Scénarios via l'URL de la page (?mock=…) :
//   nokey         → aucune clé : toutes les données renvoient NO_API_KEY (onboarding)
//   unauthorized  → clé .env refusée par Garmin (GARMIN_UNAUTHORIZED)
//   unavailable   → Garmin indisponible sur les analyses (GARMIN_UNAVAILABLE)
//   empty         → aucune activité
//   slow          → latence ×4 (skeletons)
//
// La clé saisie n'est jamais conservée : seul un indice (4 derniers caractères) est gardé en mémoire.

import { DEFAULT_PROFILE } from '../api/defaults'
import type { ApiErrorBody, ErrorCode, Mode, Profile, Settings } from '../api/types'
import { getCatalog } from './catalog'
import { buildActivityAnalysis, buildDayAnalysis } from './analysis'


const scenario = new URLSearchParams(window.location.search).get('mock') ?? ''

const state: {
  source: Settings['apiKeySource']
  hint: string | null
  envRejected: boolean
  tokenExpiresAt: string | null
  timezone: string
  profile: Profile
} = {
  source: scenario === 'nokey' ? 'none' : 'env',
  hint: scenario === 'nokey' ? null : '••••7f3c',
  envRejected: scenario === 'unauthorized',
  tokenExpiresAt: scenario === 'nokey' || scenario === 'unauthorized' ? null : new Date(Date.now() + 20 * 3600_000).toISOString(),
  timezone: 'Europe/Paris',
  profile: { ...DEFAULT_PROFILE },
}

function settings(): Settings {
  return {
    apiKeyConfigured: state.source !== 'none',
    apiKeySource: state.source,
    apiKeyHint: state.hint,
    tokenExpiresAt: state.tokenExpiresAt,
    timezone: state.timezone,
    profile: { ...state.profile },
  }
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json; charset=utf-8' } })
}

function error(status: number, code: ErrorCode, message: string): Response {
  const body: ApiErrorBody = { error: message, code }
  return json(body, status)
}

function authGuard(): Response | null {
  if (state.source === 'none') return error(401, 'NO_API_KEY', 'Aucune clé API Garmin configurée.')
  if (state.source === 'env' && state.envRejected) return error(401, 'GARMIN_UNAUTHORIZED', 'Garmin a refusé la clé API (révoquée ou expirée).')
  return null
}

function parseBody<T>(init?: RequestInit): T | null {
  if (!init?.body || typeof init.body !== 'string') return null
  try {
    return JSON.parse(init.body) as T
  } catch {
    return null
  }
}

export async function mockFetch(path: string, init?: RequestInit): Promise<Response> {
  const url = new URL(path, window.location.origin)
  const method = (init?.method ?? 'GET').toUpperCase()
  const p = url.pathname
  const slow = scenario === 'slow' ? 4 : 1
  const heavy = /analysis$/.test(p)
  await sleep((heavy ? 450 + Math.random() * 450 : 150 + Math.random() * 200) * slow)

  if (p === '/api/health') return json({ status: 'ok', version: '1.0.0-mock', garminConfigured: state.source !== 'none' })

  if (p === '/api/settings' && method === 'GET') return json(settings())

  if (p === '/api/settings' && method === 'PUT') {
    const body = parseBody<{ apiKey?: string; timezone?: string; profile?: Partial<Profile> }>(init)
    if (!body) return error(400, 'BAD_REQUEST', 'Corps de requête invalide.')
    if (body.apiKey !== undefined) {
      const key = body.apiKey.trim()
      if (key === '') {
        state.source = scenario === 'nokey' ? 'none' : 'env'
        state.hint = state.source === 'env' ? '••••7f3c' : null
        state.tokenExpiresAt = state.source === 'env' && !state.envRejected ? new Date(Date.now() + 20 * 3600_000).toISOString() : null
      } else if (key.length < 12 || /invalid|refus/i.test(key)) {
        return error(400, 'GARMIN_UNAUTHORIZED', 'Clé refusée par Garmin : vérifie qu’elle est active dans Garmin Connect.')
      } else {
        state.source = 'ui'
        state.hint = `••••${key.slice(-4)}`
        state.tokenExpiresAt = new Date(Date.now() + 24 * 3600_000).toISOString()
      }
    }
    if (body.timezone) state.timezone = body.timezone
    if (body.profile) state.profile = { ...state.profile, ...body.profile }
    return json(settings())
  }

  if (p === '/api/settings/test' && method === 'POST') {
    const g = authGuard()
    if (g) return g
    state.tokenExpiresAt = new Date(Date.now() + 24 * 3600_000).toISOString()
    return json({ ok: true, message: 'Connexion Garmin OK', tokenExpiresAt: state.tokenExpiresAt })
  }

  const guard = authGuard()
  if (guard) return guard

  if (p === '/api/activities' && method === 'GET') {
    const limit = Number(url.searchParams.get('limit') ?? 20)
    const start = Number(url.searchParams.get('start') ?? 0)
    const from = url.searchParams.get('from')
    const to = url.searchParams.get('to')
    let items = scenario === 'empty' ? [] : getCatalog().map((it) => it.summary)
    if (from) items = items.filter((a) => a.date >= from)
    if (to) items = items.filter((a) => a.date <= to)
    return json({ items: items.slice(start, start + limit), hasMore: start + limit < items.length })
  }

  let m = /^\/api\/activities\/(\d+)\/(analysis|report)$/.exec(p)
  if (m) {
    if (scenario === 'unavailable') return error(503, 'GARMIN_UNAVAILABLE', 'Garmin Connect ne répond pas, réessaie dans quelques minutes.')
    const item = getCatalog().find((it) => it.summary.id === Number(m?.[1]))
    if (!item) return error(404, 'NOT_FOUND', 'Activité introuvable.')
    const modeParam = url.searchParams.get('mode') ?? 'sfp'
    if (!['sfp', 'recovery', 'performance'].includes(modeParam)) return error(400, 'BAD_REQUEST', 'Mode inconnu.')
    const history = Number(url.searchParams.get('history') ?? 0)
    const analysis = buildActivityAnalysis(item, modeParam as Mode, history, state.profile, { now: new Date() })
    if (m[2] === 'report') return new Response(analysis.report, { headers: { 'Content-Type': 'text/plain; charset=utf-8' } })
    return json(analysis)
  }

  m = /^\/api\/days\/(\d{4}-\d{2}-\d{2})\/(analysis|report)$/.exec(p)
  if (m) {
    if (scenario === 'unavailable') return error(503, 'GARMIN_UNAVAILABLE', 'Garmin Connect ne répond pas, réessaie dans quelques minutes.')
    const history = Number(url.searchParams.get('history') ?? 0)
    const day = buildDayAnalysis(m[1], history, state.profile, { now: new Date() })
    if (m[2] === 'report') return new Response(day.report, { headers: { 'Content-Type': 'text/plain; charset=utf-8' } })
    return json(day)
  }

  return error(404, 'NOT_FOUND', `Route mock inconnue : ${method} ${p}`)
}
