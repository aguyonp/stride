// Shared mock domain logic: visible activities per connected source, summaries, notes, plan matching, freshness.

import type { ActivityNotes, ActivitySummary, Freshness, HistoryRow, LoadRatio, PlanMatch, PlannedSession, Source, WeekPlan } from '../api/types'
import { addDays, fmtSigned, mondayOf, todayISO } from '../lib/format'
import { getCatalog, type CatalogItem } from './catalog'
import { db, type PlanEntry, type UserData } from './db'
import { has } from './flags'
import { manualItems } from './manual'
import { gradeFactor, isRunKind, mean, round } from './sim'

export type SourceMode = 'fusion' | 'garmin' | 'strava' | 'none'

export function sourceMode(d: UserData): SourceMode {
  const g = d.integrations.garmin.connected
  const s = d.integrations.strava.connected
  return g && s ? 'fusion' : g ? 'garmin' : s ? 'strava' : 'none'
}

export function visibleItems(d: UserData, today = todayISO()): CatalogItem[] {
  const mode = sourceMode(d)
  const manual = manualItems(d)
  if (mode === 'none' || has('empty')) return manual
  const items = getCatalog(today)
  const synced = mode === 'strava' ? items.filter((it) => it.stravaId !== null) : items
  return manual.length ? [...manual, ...synced].sort((a, b) => (a.startLocal < b.startLocal ? 1 : -1)) : synced
}

function sourcesOf(it: CatalogItem, mode: SourceMode): Source[] {
  if (it.manual) return ['manual']
  if (mode === 'strava') return ['strava']
  if (mode === 'garmin') return ['garmin']
  return it.stravaId !== null ? ['garmin', 'strava'] : ['garmin']
}

export const garminOn = (mode: SourceMode) => mode === 'garmin' || mode === 'fusion'

/* ---------------------------------- Grade-adjusted pace ---------------------------------- */

function eqDistance(it: CatalogItem, i0: number, i1: number): number {
  const pts = it.sim.points
  let eq = 0
  for (let i = Math.max(1, i0 + 1); i <= i1 && i < pts.length; i++) eq += (pts[i].dist - pts[i - 1].dist) * gradeFactor(pts[i].gradePct)
  return eq
}

export function gapBetween(it: CatalogItem, i0: number, i1: number): number | null {
  const pts = it.sim.points
  if (!it.spec.elev) return null
  const eq = eqDistance(it, i0, i1)
  const dur = pts[Math.min(i1, pts.length - 1)].t - pts[i0].t
  return eq > 20 ? Math.round(dur / (eq / 1000)) : null
}

export function gapSummary(it: CatalogItem): number | null {
  if (!it.isRunning) return null
  if (!it.spec.elev) return it.base.avgPaceSecPerKm
  return gapBetween(it, 0, it.sim.points.length - 1)
}

export function gapSeries(it: CatalogItem): (number | null)[] {
  return it.sim.series.paceSecPerKm.map((p, i) => (p === null || !it.isRunning ? null : Math.round(p / gradeFactor(it.sim.points[i].gradePct))))
}

/* ---------------------------------- Notes ---------------------------------- */

export function notesOf(d: UserData, id: number): ActivityNotes {
  const n = d.notes[String(id)]
  if (!n) return { painDuring: null, painCold: null, painNextMorning: null, rpe: null, shoeId: null, shoe: null, tags: [], note: '', updatedAt: null }
  return { ...n, shoe: d.shoes.find((s) => s.id === n.shoeId) ?? null }
}

/* ---------------------------------- Plan matching ---------------------------------- */

function compatible(p: PlanEntry, it: CatalogItem): boolean {
  switch (p.sport) {
    case 'run':
      return it.isRunning
    case 'bike':
      return it.sport === 'bike'
    case 'strength':
      return it.sport === 'strength'
    case 'mobility':
      return it.sport === 'strength' || it.sport === 'other'
    case 'other':
      return !it.isRunning
    default:
      return false
  }
}

const pctDelta = (actual: number | null, target: number | null) => (actual === null || target === null || target === 0 ? null : Math.round(((actual - target) / target) * 1000) / 10)

function matchPlan(p: PlanEntry, d: UserData, items: CatalogItem[], today: string, used = new Set<number>()): PlanMatch {
  const base = { plannedId: p.id, activityId: null, durationDeltaPct: null, distanceDeltaPct: null, paceDeltaSecPerKm: null, cadenceDeltaSpm: null }
  if (p.sport === 'rest') {
    const ran = items.some((it) => it.date === p.date && it.isRunning)
    if (p.date >= today) return { ...base, status: 'upcoming', summary: 'Repos prévu' }
    return ran ? { ...base, status: 'partial', summary: 'Course réalisée un jour de repos' } : { ...base, status: 'done', summary: 'Repos respecté' }
  }
  const candidates = items.filter((it) => it.date === p.date && compatible(p, it) && !used.has(it.id)).sort((a, b) => (b.base.durationS ?? 0) - (a.base.durationS ?? 0))
  const it = candidates[0]
  if (!it) {
    return p.date < today ? { ...base, status: 'missed', summary: 'Séance non réalisée' } : { ...base, status: 'upcoming', summary: p.date === today ? 'Prévue aujourd’hui' : 'À venir' }
  }
  used.add(it.id)
  const durationDeltaPct = pctDelta(it.base.durationS, p.targetDurationS)
  const distanceDeltaPct = pctDelta(it.base.distanceM, p.targetDistanceM)
  let paceDeltaSecPerKm: number | null = null
  const pace = it.base.avgPaceSecPerKm
  if (pace !== null && (p.targetPaceMinSecPerKm !== null || p.targetPaceMaxSecPerKm !== null)) {
    const lo = p.targetPaceMinSecPerKm ?? p.targetPaceMaxSecPerKm!
    const hi = p.targetPaceMaxSecPerKm ?? p.targetPaceMinSecPerKm!
    paceDeltaSecPerKm = pace < lo ? pace - lo : pace > hi ? pace - hi : 0
  }
  const cadenceDeltaSpm = it.base.avgCadenceSpm !== null && p.targetCadenceSpm !== null ? Math.round(it.base.avgCadenceSpm - p.targetCadenceSpm) : null
  const dev = d.profile.thresholds.planDeviationPct
  const off = [durationDeltaPct, distanceDeltaPct].some((v) => v !== null && Math.abs(v) > dev) || (paceDeltaSecPerKm !== null && Math.abs(paceDeltaSecPerKm) > 10)
  const parts: string[] = []
  if (durationDeltaPct !== null) parts.push(`Durée ${fmtSigned(durationDeltaPct, 0)} %`)
  if (distanceDeltaPct !== null) parts.push(`Distance ${fmtSigned(distanceDeltaPct, 0)} %`)
  if (paceDeltaSecPerKm !== null) parts.push(paceDeltaSecPerKm === 0 ? 'Allure dans la cible' : `Allure ${fmtSigned(paceDeltaSecPerKm)} s/km hors cible`)
  if (cadenceDeltaSpm !== null) parts.push(`Cadence ${fmtSigned(cadenceDeltaSpm)} spm`)
  return { plannedId: p.id, activityId: it.id, status: off ? 'partial' : 'done', durationDeltaPct, distanceDeltaPct, paceDeltaSecPerKm, cadenceDeltaSpm, summary: parts.join(' · ') || 'Réalisée' }
}

export function plannedWithMatch(d: UserData, items: CatalogItem[], today: string, from?: string, to?: string): PlannedSession[] {
  const used = new Set<number>()
  return [...d.plan]
    .sort((a, b) => (a.date === b.date ? a.id - b.id : a.date < b.date ? -1 : 1))
    .map((p) => ({ ...p, match: matchPlan(p, d, items, today, used) }))
    .filter((p) => (!from || p.date >= from) && (!to || p.date <= to))
}

function planStatusFor(it: CatalogItem, planned: PlannedSession[], d: UserData): ActivitySummary['planStatus'] {
  const m = planned.find((p) => p.match?.activityId === it.id)
  if (m?.match && (m.match.status === 'done' || m.match.status === 'partial')) return m.match.status
  if (it.isRunning && d.plan.length) return 'extra'
  return null
}

export function summarize(it: CatalogItem, d: UserData, planned: PlannedSession[]): ActivitySummary {
  const mode = sourceMode(d)
  const g = garminOn(mode)
  const n = d.notes[String(it.id)]
  const kudosOn = mode !== 'garmin' && it.stravaId !== null && !d.integrations.strava.missingScopes?.length
  const remarks = ((d.v42 as { notes?: { kind: string; activityId: number | null }[] } | undefined)?.notes ?? []).filter((x) => x.kind === 'remark' && x.activityId === it.id).length
  return {
    remarkCount: remarks,
    id: it.id,
    garminId: g && !it.manual ? it.garminId : null,
    stravaId: mode === 'garmin' ? null : it.stravaId,
    sources: sourcesOf(it, mode),
    name: mode !== 'garmin' && it.strava ? it.strava.title : it.name,
    type: it.type,
    typeLabel: it.typeLabel,
    sport: it.sport,
    isRunning: it.isRunning,
    startLocal: it.startLocal,
    date: it.date,
    distanceM: it.base.distanceM,
    durationS: it.base.durationS,
    avgPaceSecPerKm: it.base.avgPaceSecPerKm,
    gapSecPerKm: gapSummary(it),
    avgHR: it.base.avgHR,
    avgPower: g ? it.base.avgPower : null,
    avgCadenceSpm: it.base.avgCadenceSpm,
    elevGainM: it.base.elevGainM,
    elevLossM: it.base.elevLossM,
    trainingLoad: g ? it.base.trainingLoad : null,
    painDuring: n?.painDuring ?? null,
    hasNotes: !!n && (n.note !== '' || n.painDuring !== null || n.rpe !== null || n.tags.length > 0),
    planStatus: planStatusFor(it, planned, d),
    kudos: kudosOn ? (it.id * 7) % 13 : null,
  }
}

export function weekPlan(d: UserData, items: CatalogItem[], date: string, _today: string, planned: PlannedSession[]): WeekPlan {
  const weekStart = mondayOf(date)
  const days: WeekPlan['days'] = []
  const totals = { plannedDurationS: 0, doneDurationS: 0, plannedDistanceM: 0, doneDistanceM: 0, sessionsPlanned: 0, sessionsDone: 0 }
  for (let i = 0; i < 7; i++) {
    const day = addDays(weekStart, i)
    const pl = planned.filter((p) => p.date === day)
    const done = items.filter((it) => it.date === day).map((it) => summarize(it, d, planned)).sort((a, b) => (a.startLocal < b.startLocal ? -1 : 1))
    for (const p of pl) {
      if (p.sport === 'rest') continue
      totals.sessionsPlanned++
      totals.plannedDurationS += p.targetDurationS ?? 0
      totals.plannedDistanceM += p.targetDistanceM ?? 0
      if (p.match && (p.match.status === 'done' || p.match.status === 'partial')) totals.sessionsDone++
    }
    for (const a of done) {
      const matched = pl.some((p) => p.match?.activityId === a.id)
      if (a.isRunning || matched) {
        totals.doneDurationS += a.durationS ?? 0
        totals.doneDistanceM += a.distanceM ?? 0
      }
    }
    days.push({ date: day, planned: pl, done })
  }
  return { weekStart, days, totals }
}

/* ---------------------------------- Load ---------------------------------- */

export function loadRatio(items: CatalogItem[], date: string, mode: SourceMode): LoadRatio | null {
  if (!garminOn(mode)) return null
  const sum = (from: string, to: string) => items.filter((it) => it.date > from && it.date <= to).reduce((s, it) => s + (it.base.trainingLoad ?? 0), 0)
  const acute = sum(addDays(date, -7), date)
  const chronic = Math.round(sum(addDays(date, -28), date) / 4)
  const ratio = chronic > 0 ? Math.round((acute / chronic) * 100) / 100 : null
  const verdict = ratio === null || ratio < 0.8 ? 'low' : ratio <= 1.3 ? 'optimal' : ratio <= 1.5 ? 'high' : 'danger'
  return { acute, chronic, ratio, verdict, source: 'computed' }
}

export function runIndices(it: CatalogItem): number[] {
  return it.sim.points.map((p, i) => (isRunKind(p.kind) ? i : -1)).filter((i) => i >= 0)
}

export function avgGct(it: CatalogItem): number | null {
  return round(mean(runIndices(it).map((i) => it.sim.series.gctMs[i])))
}

export function historyRows(d: UserData, items: CatalogItem[], date: string, mode: SourceMode): HistoryRow[] {
  const from = addDays(date, -13)
  const g = garminOn(mode)
  return items
    .filter((it) => it.isRunning && it.date >= from && it.date <= date)
    .sort((a, b) => (a.startLocal < b.startLocal ? -1 : 1))
    .map((it) => {
      const n = notesOf(d, it.id)
      return {
        activityId: it.id,
        date: it.date,
        type: it.type,
        typeLabel: it.typeLabel,
        distanceM: it.base.distanceM,
        durationS: it.base.durationS,
        paceSecPerKm: it.base.avgPaceSecPerKm,
        avgCadenceSpm: it.base.avgCadenceSpm,
        gctMs: g ? avgGct(it) : null,
        elevLossM: it.base.elevLossM,
        trainingLoad: g ? it.base.trainingLoad : null,
        shoeName: n.shoe?.name ?? null,
        painDuring: n.painDuring,
        painCold: n.painCold,
        painNextMorning: n.painNextMorning,
      }
    })
}

/* ---------------------------------- Freshness ---------------------------------- */

export function freshness(kind: 'list' | 'activity' | 'day', date: string | null, today = todayISO()): Freshness {
  const s = db()
  const now = Date.now()
  const forced = has('fresh-db') ? 'db' : has('fresh-cache') ? 'cache' : has('fresh-live') ? 'live' : null
  const lastSync = s.sync.lastSyncAt ?? new Date(now - 3 * 3600_000).toISOString()
  if (forced === 'live' || (!forced && s.sync.liveUntil > now)) return { source: 'live', updatedAt: s.sync.lastLiveAt ?? new Date(now).toISOString() }
  if (forced === 'cache') return { source: 'cache', updatedAt: new Date(now - 4 * 60_000).toISOString() }
  if (forced === 'db') return { source: 'db', updatedAt: lastSync }
  if (kind === 'list') return { source: 'cache', updatedAt: new Date(now - 90_000).toISOString() }
  const old = date !== null && date < addDays(today, kind === 'activity' ? -2 : -1)
  return old ? { source: 'db', updatedAt: lastSync } : { source: 'cache', updatedAt: new Date(now - 3 * 60_000).toISOString() }
}
