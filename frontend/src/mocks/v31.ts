// 3.1.0 mock endpoints (§12): personal records, manual activities, calendar events, shift / adapt / replace, phases.
//
// Extra scenario flags: standalone (no provider connected, manual activities seeded) · stravascope (Strava connected
// with the `read` scope only → missingScopes, no kudos) · longgoal (training phases until the spring 10 km goal)

import type { ApiErrorBody, CalendarEvent, ErrorCode, EventKind, ManualActivityInput, PersonalRecord, PlanAdaptPreview, PlannedSessionInput, RecordKey, Sport, WorkoutStep } from '../api/types'
import { addDays, diffDays, fmtDateShort, todayISO } from '../lib/format'
import { nextId, type MockUser, type PlanEntry, type UserData } from './db'
import { plannedWithMatch, sourceMode, summarize, visibleItems } from './domain'
import { manualItem, v31Of, type ManualEntry } from './manual'

interface Ctx {
  url: URL
  params: string[]
  init?: RequestInit
  me: MockUser
  data: UserData
}
type RouteFn = (method: string, re: RegExp, handler: (c: Ctx) => Response | Promise<Response>) => void

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json; charset=utf-8' } })
const err = (status: number, code: ErrorCode, error: string) => json({ error, code } satisfies ApiErrorBody, status)
const noContent = () => new Response(null, { status: 204 })
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/
const TIME_RE = /^\d{2}:\d{2}$/

function body<T>(init?: RequestInit): T | null {
  if (!init?.body || typeof init.body !== 'string') return null
  try {
    return JSON.parse(init.body) as T
  } catch {
    return null
  }
}

/* ---------------------------------- Records ---------------------------------- */

const RECORDS: { key: RecordKey; label: string; distanceM: number }[] = [
  { key: '1k', label: '1 km', distanceM: 1000 },
  { key: '5k', label: '5 km', distanceM: 5000 },
  { key: '10k', label: '10 km', distanceM: 10000 },
  { key: 'half', label: 'Semi-marathon', distanceM: 21097.5 },
  { key: 'marathon', label: 'Marathon', distanceM: 42195 },
  { key: 'longest', label: 'Plus longue sortie', distanceM: 0 },
]

export function records(d: UserData): PersonalRecord[] {
  const runs = visibleItems(d).filter((it) => it.isRunning && it.base.distanceM && it.base.durationS)
  const stravaOn = sourceMode(d) !== 'garmin' && sourceMode(d) !== 'none' && !d.integrations.strava.missingScopes?.length
  const manual = v31Of(d).records
  return RECORDS.map(({ key, label, distanceM }) => {
    const m = manual[key]
    if (m) {
      const dist = key === 'longest' ? (m.distanceM ?? 0) : distanceM
      return { key, label, distanceM: dist, timeS: m.timeS, paceSecPerKm: m.timeS && dist ? Math.round(m.timeS / (dist / 1000)) : null, date: m.date, activityId: null, source: 'manual' }
    }
    if (key === 'longest') {
      const it = [...runs].sort((a, b) => (b.base.distanceM ?? 0) - (a.base.distanceM ?? 0))[0]
      return it
        ? { key, label, distanceM: it.base.distanceM!, timeS: it.base.durationS, paceSecPerKm: it.base.avgPaceSecPerKm, date: it.date, activityId: it.id, source: it.stravaId && stravaOn ? 'strava' : 'computed' }
        : { key, label, distanceM: 0, timeS: null, paceSecPerKm: null, date: null, activityId: null, source: 'computed' }
    }
    let best: { t: number; id: number; date: string; strava: boolean } | null = null
    for (const it of runs) {
      if ((it.base.distanceM ?? 0) < distanceM) continue
      const t = Math.round(((it.base.durationS ?? 0) / ((it.base.distanceM ?? 1) / 1000)) * (distanceM / 1000) * (it.manual ? 1 : 0.975))
      if (!best || t < best.t) best = { t, id: it.id, date: it.date, strava: !!it.stravaId && stravaOn && (key === '5k' || key === '10k') }
    }
    return best
      ? { key, label, distanceM, timeS: best.t, paceSecPerKm: Math.round(best.t / (distanceM / 1000)), date: best.date, activityId: best.id, source: best.strava ? 'strava' : 'computed' }
      : { key, label, distanceM, timeS: null, paceSecPerKm: null, date: null, activityId: null, source: 'computed' }
  })
}

/* ---------------------------------- Adaptation (rules) ---------------------------------- */

const EVENT_LABEL: Record<EventKind, string> = { fatigue: 'Fatigue', pain: 'Douleur', illness: 'Maladie', unavailable: 'Indisponibilité', forced_rest: 'Repos forcé' }
const SEVERITY = ['légère', 'modérée', 'forte']
const isHard = (p: Pick<PlanEntry, 'title' | 'steps'>) => /fraction|interval|vma|seuil|×/i.test(p.title) || p.steps.some((s) => s.type === 'repeat' || s.type === 'interval')

const toInput = (p: PlanEntry): PlannedSessionInput => ({
  date: p.date,
  sport: p.sport,
  title: p.title,
  targetDurationS: p.targetDurationS,
  targetDistanceM: p.targetDistanceM,
  targetPaceMinSecPerKm: p.targetPaceMinSecPerKm,
  targetPaceMaxSecPerKm: p.targetPaceMaxSecPerKm,
  targetCadenceSpm: p.targetCadenceSpm,
  instructions: p.instructions,
  steps: p.steps,
  source: p.source,
})

function soften(s: PlannedSessionInput, severity: number, hardToEasy: boolean): PlannedSessionInput {
  const k = [0.8, 0.7, 0.6][severity - 1] ?? 0.7
  const dur = s.targetDurationS ? Math.round((s.targetDurationS * k) / 60) * 60 : null
  const easy = hardToEasy && isHard({ title: s.title, steps: s.steps ?? [] })
  const steps: WorkoutStep[] = easy && dur ? [{ type: 'run', durationS: dur, target: { kind: 'hr', min: 125, max: 142 } }] : (s.steps ?? [])
  return {
    ...s,
    title: easy ? `Footing facile ${Math.round((dur ?? 1800) / 60)}′` : s.title,
    targetDurationS: dur,
    targetDistanceM: s.targetDistanceM ? Math.round((s.targetDistanceM * k) / 100) * 100 : null,
    targetPaceMinSecPerKm: s.targetPaceMinSecPerKm ? s.targetPaceMinSecPerKm + (easy ? 40 : 10) : null,
    targetPaceMaxSecPerKm: s.targetPaceMaxSecPerKm ? s.targetPaceMaxSecPerKm + (easy ? 40 : 10) : null,
    instructions: `${easy ? 'Intensité retirée après l’imprévu. ' : ''}Volume −${Math.round((1 - k) * 100)} %. ${s.instructions}`.trim(),
    steps,
  }
}

export function adaptPlan(d: UserData, from: string, eventId?: number): PlanAdaptPreview {
  const events = v31Of(d).events.filter((e) => (eventId ? e.id === eventId : e.endDate >= from)).sort((a, b) => (a.startDate < b.startDate ? -1 : 1))
  const e: CalendarEvent | undefined = events[0]
  if (!e) return { sessions: [], removeIds: [], summary: 'Aucun imprévu ne touche le plan à venir : rien à adapter.' }
  const upcoming = d.plan.filter((p) => p.date >= from && p.sport !== 'rest').sort((a, b) => (a.date === b.date ? a.id - b.id : a.date < b.date ? -1 : 1))
  const blocked = (day: string) => events.some((x) => x.startDate <= day && x.endDate >= day)
  const busy = new Set(upcoming.map((p) => p.date))
  const len = diffDays(e.startDate, e.endDate) + 1
  const changed = new Map<number, PlannedSessionInput | null>()
  let moved = 0
  let dropped = 0
  let softened = 0
  for (const p of upcoming.filter((x) => x.date >= e.startDate && x.date <= e.endDate)) {
    if (len > 3 || e.kind === 'forced_rest') {
      changed.set(p.id, null)
      dropped++
      continue
    }
    let day = addDays(e.endDate, 1)
    while (busy.has(day) || blocked(day)) day = addDays(day, 1)
    busy.add(day)
    changed.set(p.id, { ...toInput(p), date: day })
    moved++
  }
  const recovery = e.kind === 'fatigue' ? 2 : e.kind === 'unavailable' ? 0 : 3 + 2 * e.severity
  const end = addDays(e.endDate, recovery)
  for (const p of upcoming) {
    const cur = changed.has(p.id) ? changed.get(p.id) : toInput(p)
    if (!cur || cur.date <= e.endDate || cur.date > end) continue
    changed.set(p.id, soften(cur, e.severity, e.kind !== 'fatigue' || e.severity >= 2))
    if (cur.date === p.date) softened++
  }
  const removeIds = [...changed.keys()]
  const sessions = [...changed.values()].filter((s): s is PlannedSessionInput => s !== null).sort((a, b) => (a.date < b.date ? -1 : 1))
  const parts = [moved && `${moved} séance${moved > 1 ? 's' : ''} décalée${moved > 1 ? 's' : ''}`, dropped && `${dropped} supprimée${dropped > 1 ? 's' : ''}`, softened && `${softened} allégée${softened > 1 ? 's' : ''}`].filter(Boolean)
  const range = e.startDate === e.endDate ? `du ${fmtDateShort(e.startDate)}` : `du ${fmtDateShort(e.startDate)} au ${fmtDateShort(e.endDate)}`
  return {
    sessions,
    removeIds,
    summary: `${EVENT_LABEL[e.kind]} ${SEVERITY[e.severity - 1]} ${range} : ${parts.length ? parts.join(', ') : 'aucune séance concernée'}.${recovery ? ` Reprise progressive sur ${recovery} jours, sans semaine au-dessus de +10 % du volume prévu.` : ''}`,
  }
}

/* ---------------------------------- Validation ---------------------------------- */

const SPORTS: Sport[] = ['run', 'bike', 'walk', 'hike', 'swim', 'strength', 'other']
const KINDS: EventKind[] = ['fatigue', 'pain', 'illness', 'unavailable', 'forced_rest']
const num = (v: unknown, max: number) => v === undefined || v === null || (typeof v === 'number' && Number.isFinite(v) && v >= 0 && v <= max)

function validActivity(b: Partial<ManualActivityInput>, partial: boolean): string | null {
  if ((!partial || b.date !== undefined) && !DATE_RE.test(b.date ?? '')) return 'Date invalide (AAAA-MM-JJ).'
  if (b.date && b.date > todayISO()) return 'La date ne peut pas être dans le futur.'
  if (b.startTime !== undefined && b.startTime !== '' && !TIME_RE.test(b.startTime)) return 'Heure invalide (HH:MM).'
  if ((!partial || b.sport !== undefined) && !SPORTS.includes(b.sport as Sport)) return 'Sport invalide.'
  if ((!partial || b.durationS !== undefined) && !(typeof b.durationS === 'number' && b.durationS >= 60 && b.durationS <= 86_400)) return 'Durée invalide (1 min à 24 h).'
  if (!num(b.distanceM, 500_000)) return 'Distance invalide.'
  if (!num(b.avgHR, 250) || !num(b.maxHR, 250)) return 'Fréquence cardiaque invalide.'
  if (b.avgHR && b.maxHR && b.avgHR > b.maxHR) return 'La FC moyenne dépasse la FC max.'
  if (!num(b.avgCadenceSpm, 260)) return 'Cadence invalide.'
  if (!num(b.elevGainM, 10_000) || !num(b.elevLossM, 10_000)) return 'Dénivelé invalide.'
  if (b.title !== undefined && b.title.length > 120) return 'Titre trop long (120 caractères max).'
  return null
}

function validEvent(b: Partial<CalendarEvent>, partial: boolean): string | null {
  if ((!partial || b.kind !== undefined) && !KINDS.includes(b.kind as EventKind)) return 'Type d’imprévu invalide.'
  if ((!partial || b.startDate !== undefined) && !DATE_RE.test(b.startDate ?? '')) return 'Date de début invalide.'
  if ((!partial || b.endDate !== undefined) && !DATE_RE.test(b.endDate ?? '')) return 'Date de fin invalide.'
  if (b.startDate && b.endDate && b.endDate < b.startDate) return 'La fin doit suivre le début.'
  if ((!partial || b.severity !== undefined) && ![1, 2, 3].includes(b.severity as number)) return 'Intensité invalide (1 à 3).'
  if (b.note !== undefined && b.note.length > 500) return 'Note trop longue (500 caractères max).'
  return null
}

const clean = (b: Partial<ManualActivityInput>): Partial<ManualActivityInput> => Object.fromEntries(Object.entries(b).filter(([, v]) => v !== null && v !== '')) as Partial<ManualActivityInput>

/* ---------------------------------- Routes ---------------------------------- */

export function registerV31(route: RouteFn, isLlmConfigured: (d: UserData) => boolean) {
  route('GET', /^\/api\/records$/, ({ data }) => json({ records: records(data) }))
  route('PUT', /^\/api\/records\/(1k|5k|10k|half|marathon|longest)$/, ({ data, params, init }) => {
    const key = params[0] as RecordKey
    const b = body<{ timeS?: number; distanceM?: number; date?: string }>(init) ?? {}
    if (key === 'longest' ? !(typeof b.distanceM === 'number' && b.distanceM > 0) : !(typeof b.timeS === 'number' && b.timeS > 0)) return err(400, 'BAD_REQUEST', key === 'longest' ? 'Distance requise.' : 'Temps requis.')
    if (b.date !== undefined && b.date !== '' && !DATE_RE.test(b.date)) return err(400, 'BAD_REQUEST', 'Date invalide.')
    v31Of(data).records[key] = { timeS: b.timeS ?? null, distanceM: b.distanceM, date: b.date || null }
    return json(records(data).find((r) => r.key === key))
  })
  route('DELETE', /^\/api\/records\/(1k|5k|10k|half|marathon|longest)$/, ({ data, params }) => {
    delete v31Of(data).records[params[0] as RecordKey]
    return noContent()
  })

  const manualOf = (data: UserData, id: string) => v31Of(data).manual.find((m) => m.id === Number(id)) ?? null
  const summaryOf = (data: UserData, m: ManualEntry) => summarize(manualItem(m), data, plannedWithMatch(data, visibleItems(data), todayISO()))
  route('POST', /^\/api\/activities$/, ({ data, init }) => {
    const b = clean(body<Partial<ManualActivityInput>>(init) ?? {})
    const e = validActivity(b, false)
    if (e) return err(400, 'BAD_REQUEST', e)
    const m: ManualEntry = { ...(b as ManualActivityInput), id: nextId() }
    v31Of(data).manual.push(m)
    return json(summaryOf(data, m), 201)
  })
  route('PATCH', /^\/api\/activities\/(\d+)$/, ({ data, params, init }) => {
    const m = manualOf(data, params[0])
    if (!m) return visibleItems(data).some((it) => it.id === Number(params[0])) ? err(409, 'CONFLICT', 'Seules les séances saisies manuellement sont modifiables.') : err(404, 'NOT_FOUND', 'Activité introuvable.')
    const b = body<Partial<ManualActivityInput>>(init) ?? {}
    const e = validActivity({ ...m, ...clean(b) }, true)
    if (e) return err(400, 'BAD_REQUEST', e)
    for (const [k, v] of Object.entries(b)) (m as unknown as Record<string, unknown>)[k] = v === null || v === '' ? undefined : v
    return json(summaryOf(data, m))
  })
  route('DELETE', /^\/api\/activities\/(\d+)$/, ({ data, params }) => {
    const m = manualOf(data, params[0])
    if (!m) return visibleItems(data).some((it) => it.id === Number(params[0])) ? err(409, 'CONFLICT', 'Seules les séances saisies manuellement sont supprimables.') : err(404, 'NOT_FOUND', 'Activité introuvable.')
    const st = v31Of(data)
    st.manual = st.manual.filter((x) => x.id !== m.id)
    delete data.notes[String(m.id)]
    return noContent()
  })

  route('GET', /^\/api\/calendar\/events$/, ({ data, url }) => {
    const from = url.searchParams.get('from') ?? '0000-00-00'
    const to = url.searchParams.get('to') ?? '9999-12-31'
    return json(v31Of(data).events.filter((e) => e.endDate >= from && e.startDate <= to).sort((a, b) => (a.startDate < b.startDate ? -1 : 1)))
  })
  route('POST', /^\/api\/calendar\/events$/, ({ data, init }) => {
    const b = body<Partial<CalendarEvent>>(init) ?? {}
    const e = validEvent(b, false)
    if (e) return err(400, 'BAD_REQUEST', e)
    const ev: CalendarEvent = { id: nextId(), kind: b.kind!, startDate: b.startDate!, endDate: b.endDate!, severity: b.severity!, note: b.note?.trim() ?? '' }
    v31Of(data).events.push(ev)
    return json(ev, 201)
  })
  route('PATCH', /^\/api\/calendar\/events\/(\d+)$/, ({ data, params, init }) => {
    const ev = v31Of(data).events.find((x) => x.id === Number(params[0]))
    if (!ev) return err(404, 'NOT_FOUND', 'Imprévu introuvable.')
    const b = body<Partial<CalendarEvent>>(init) ?? {}
    const e = validEvent({ ...ev, ...b }, true)
    if (e) return err(400, 'BAD_REQUEST', e)
    Object.assign(ev, { ...b, id: ev.id })
    return json(ev)
  })
  route('DELETE', /^\/api\/calendar\/events\/(\d+)$/, ({ data, params }) => {
    const st = v31Of(data)
    const before = st.events.length
    st.events = st.events.filter((x) => x.id !== Number(params[0]))
    return before === st.events.length ? err(404, 'NOT_FOUND', 'Imprévu introuvable.') : noContent()
  })

  const withMatch = (data: UserData, id: number) => plannedWithMatch(data, visibleItems(data), todayISO()).find((p) => p.id === id)
  route('POST', /^\/api\/plan\/(\d+)\/shift$/, ({ data, params, init }) => {
    const p = data.plan.find((x) => x.id === Number(params[0]))
    if (!p) return err(404, 'NOT_FOUND', 'Séance introuvable.')
    const b = body<{ days?: number; toDate?: string }>(init) ?? {}
    const to = b.toDate ?? (Number.isInteger(b.days) ? addDays(p.date, b.days as number) : '')
    if (!DATE_RE.test(to)) return err(400, 'BAD_REQUEST', 'Indique un nombre de jours ou une date.')
    p.date = to
    return json(withMatch(data, p.id))
  })
  route('POST', /^\/api\/plan\/adapt$/, async ({ data, init }) => {
    const b = body<{ from?: string; mode?: string; eventId?: number }>(init)
    if (!b || !DATE_RE.test(b.from ?? '') || !['rules', 'llm'].includes(b.mode ?? '')) return err(400, 'BAD_REQUEST', 'Paramètres d’adaptation invalides.')
    if (b.eventId && !v31Of(data).events.some((e) => e.id === b.eventId)) return err(404, 'NOT_FOUND', 'Imprévu introuvable.')
    if (b.mode === 'llm') {
      if (!isLlmConfigured(data)) return err(412, 'PROVIDER_NOT_CONNECTED', 'Configure ton IA dans Réglages › IA pour adapter le plan avec elle.')
      await new Promise((r) => setTimeout(r, 1300))
    }
    const r = adaptPlan(data, b.from!, b.eventId)
    return json(b.mode === 'llm' ? { ...r, summary: `Proposition de ton IA : ${r.summary}` } : r)
  })
  route('POST', /^\/api\/plan\/replace$/, ({ data, init }) => {
    const b = body<{ removeIds?: number[]; sessions?: PlannedSessionInput[] }>(init)
    if (!b || !Array.isArray(b.removeIds) || !Array.isArray(b.sessions)) return err(400, 'BAD_REQUEST', 'Corps invalide.')
    if (b.sessions.some((s) => !DATE_RE.test(s.date) || !s.title?.trim())) return err(400, 'BAD_REQUEST', 'Séance invalide : date et titre requis.')
    data.plan = data.plan.filter((p) => !b.removeIds!.includes(p.id))
    const ids = b.sessions.map((s) => {
      const { id: _id, ...rest } = s as PlannedSessionInput & { id?: number }
      const p: PlanEntry = { ...rest, id: nextId(), steps: rest.steps ?? [], source: rest.source ?? 'manual', garmin: { status: 'none', workoutId: null, pushedAt: null, error: null } }
      data.plan.push(p)
      return p.id
    })
    return json(plannedWithMatch(data, visibleItems(data), todayISO()).filter((p) => ids.includes(p.id)))
  })
  route('GET', /^\/api\/plan\/phases$/, ({ data }) => json(v31Of(data).phases))
}
