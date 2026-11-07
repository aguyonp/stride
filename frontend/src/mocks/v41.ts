// 4.1.0 mock endpoints (§14): health status golden source, full-history import job, activity search.

import type { ActivitySearchHit, AthleteProfile, BodySide, BodyZone, ErrorCode, HealthStatus, HealthStatusInput, JobStatus } from '../api/types'
import { ALL_BODY_ZONES, BODY_ZONE_META } from '../components/bodymap/bodyMapMeta'
import { addDays, diffDays, todayISO } from '../lib/format'
import { emitStreamEvent } from '../state/events'
import { nextId, PRESETS, type MockUser, type UserData } from './db'
import { visibleItems } from './domain'
import { isEmptyQuery, matchesActivity, parseQuery } from './search'
import { v4Of } from './v4'

type Ctx = { url: URL; params: string[]; init?: RequestInit; me: MockUser; data: UserData }
type RouteFn = (method: string, re: RegExp, handler: (c: Ctx) => Response | Promise<Response>) => void

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const err = (status: number, code: ErrorCode, error: string) => json({ error, code }, status)
const isoNow = () => new Date().toISOString()
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/
const SIDES: BodySide[] = ['left', 'right', 'both']

function parseBody<T>(init?: RequestInit): T | null {
  try {
    return typeof init?.body === 'string' ? (JSON.parse(init.body) as T) : null
  } catch {
    return null
  }
}

const PRESET_ZONES: BodyZone[] = ['knee_pfps', 'achilles', 'shin_splints']

/** Zone → profile focus (spec §14.2): the most recent tracked zone wins; unknown presets become `other`. */
export function focusFromHealth(s: HealthStatusInput): Pick<AthleteProfile, 'focusArea' | 'focusLabel' | 'injuredSide'> {
  if (s.mode === 'performance' || s.zones.length === 0) return { focusArea: 'none', focusLabel: 'Bonne santé', injuredSide: null }
  const latest = [...s.zones].sort((a, b) => (b.since ?? '').localeCompare(a.since ?? ''))[0]
  const preset = PRESET_ZONES.includes(latest.zone) ? (latest.zone as AthleteProfile['focusArea']) : 'other'
  const label = preset === 'other' ? BODY_ZONE_META[latest.zone].label : (PRESETS.find((p) => p.focusArea === preset)?.label ?? BODY_ZONE_META[latest.zone].label)
  return { focusArea: preset, focusLabel: label, injuredSide: latest.side }
}

function healthOf(data: UserData): HealthStatus {
  if (!data.health) {
    const p = data.profile
    const zone: BodyZone = PRESET_ZONES.includes(p.focusArea as BodyZone) ? (p.focusArea as BodyZone) : 'hamstrings_calves'
    const zones = p.focusArea === 'none' ? [] : [{ zone, side: p.injuredSide ?? 'both', since: addDays(todayISO(), -28), note: '' }]
    data.health = { mode: zones.length ? 'tracking' : 'performance', zones, focusArea: p.focusArea, updatedAt: null }
  }
  return data.health
}

function validHealth(b: HealthStatusInput | null): string | null {
  if (!b || (b.mode !== 'performance' && b.mode !== 'tracking') || !Array.isArray(b.zones)) return 'État de santé invalide.'
  if (b.mode === 'tracking' && b.zones.length === 0) return 'Indique au moins une zone suivie.'
  const seen = new Set<string>()
  for (const z of b.zones) {
    if (!ALL_BODY_ZONES.includes(z.zone) || !SIDES.includes(z.side)) return 'Zone ou côté inconnu.'
    if (seen.has(z.zone)) return 'Chaque zone ne peut être suivie qu’une fois.'
    if (z.since !== null && !DATE_RE.test(z.since)) return 'Date de début invalide.'
    if ((z.note ?? '').length > 280) return 'Note trop longue (280 caractères max).'
    seen.add(z.zone)
  }
  return null
}

function runHistoryJob(job: JobStatus) {
  const step = () => {
    const processed = Math.min(job.total ?? 0, (job.processed ?? 0) + Math.ceil((job.total ?? 1) / 6))
    const done = processed >= (job.total ?? 0)
    Object.assign(job, {
      processed,
      progressPct: job.total ? Math.round((processed / job.total) * 100) : 100,
      state: done ? 'completed' : 'running',
      label: done ? 'Historique importé' : `Import de l’historique : ${processed}/${job.total} séances`,
      result: done ? { imported: processed } : null,
      updatedAt: isoNow(),
    })
    emitStreamEvent('job.updated', { ...job })
    if (!done) setTimeout(step, 700)
  }
  setTimeout(step, 500)
}

export function registerV41(route: RouteFn) {
  route('GET', /^\/api\/health\/status$/, ({ data }) => json(healthOf(data)))

  route('PUT', /^\/api\/health\/status$/, ({ data, init }) => {
    const b = parseBody<HealthStatusInput>(init)
    const invalid = validHealth(b)
    if (invalid || !b) return err(400, 'BAD_REQUEST', invalid ?? 'État de santé invalide.')
    const zones = b.mode === 'performance' ? [] : b.zones.map((z) => ({ zone: z.zone, side: z.side, since: z.since ?? null, note: (z.note ?? '').trim() }))
    const focus = focusFromHealth({ mode: b.mode, zones })
    Object.assign(data.profile, focus)
    data.health = { mode: b.mode, zones, focusArea: focus.focusArea, updatedAt: isoNow() }
    return json(data.health)
  })

  route('POST', /^\/api\/sync\/history$/, ({ me, data, init }) => {
    const b = parseBody<{ from: string | null }>(init)
    if (!b || (b.from !== null && !DATE_RE.test(b.from ?? ''))) return err(400, 'BAD_REQUEST', 'Date de début invalide.')
    if (!data.integrations.garmin.connected && !data.integrations.strava.connected) {
      return err(412, 'PROVIDER_NOT_CONNECTED', 'Connecte Garmin ou Strava pour importer ton historique.')
    }
    const total = b.from ? Math.max(6, Math.round(diffDays(b.from, todayISO()) / 3)) : 480
    const job: JobStatus = {
      id: `job_hist_${nextId()}`,
      userId: me.id,
      kind: 'sync.history',
      queue: 'low',
      state: 'queued',
      label: 'Import de l’historique en attente',
      progressPct: 0,
      processed: 0,
      total,
      attempts: 1,
      maxAttempts: 5,
      error: null,
      result: null,
      createdAt: isoNow(),
      updatedAt: isoNow(),
    }
    v4Of(data, me.id).jobs.unshift(job)
    runHistoryJob(job)
    return json(job, 202)
  })

  route('GET', /^\/api\/search$/, ({ data, url }) => {
    const raw = url.searchParams.get('q') ?? ''
    const limit = Math.max(1, Math.min(50, Number(url.searchParams.get('limit') ?? 20) || 20))
    const q = parseQuery(raw)
    if (isEmptyQuery(q)) return json({ query: raw, items: [] })
    const items: ActivitySearchHit[] = visibleItems(data)
      .filter((it) => matchesActivity({ name: it.name, typeLabel: it.typeLabel, date: it.date }, q))
      .slice(0, limit)
      .map((it) => ({
        id: it.id,
        date: it.date,
        startLocal: it.startLocal,
        name: it.name,
        sport: it.sport,
        typeLabel: it.typeLabel,
        distanceM: it.base.distanceM,
        durationS: it.base.durationS,
      }))
    return json({ query: raw, items })
  })
}
