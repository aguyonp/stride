import type { PlanAdaptPreview, PlannedSession, PlannedSessionInput } from '../../api/types'
import { capitalize, fmtDateShort, fmtHM, fmtKm, fmtPaceValue, fmtWeekdayShort, isNum } from '../../lib/format'

export type ChangeKind = 'moved' | 'modified' | 'removed' | 'created'
export interface PlanChange {
  kind: ChangeKind
  before: PlannedSession | null
  after: PlannedSessionInput | null
  details: string[]
}

const norm = (t: string) =>
  t
    .toLowerCase()
    .replace(/\(.*?\)/g, '')
    .replace(/[^a-z0-9à-ÿ]+/g, ' ')
    .trim()

const dayLabel = (d: string) => `${capitalize(fmtWeekdayShort(d))} ${fmtDateShort(d)}`
const dur = (s: number | null | undefined) => (isNum(s) && s > 0 ? fmtHM(s) : '—')
const dist = (m: number | null | undefined) => (isNum(m) && m > 0 ? fmtKm(m, 1) : '—')
const pace = (a: number | null | undefined, b: number | null | undefined) => (isNum(a) || isNum(b) ? `${fmtPaceValue(a ?? b)}–${fmtPaceValue(b ?? a)}` : '—')
const stepsKey = (s: PlannedSessionInput) => JSON.stringify(s.steps ?? [])

function details(b: PlannedSession, a: PlannedSessionInput): string[] {
  const out: string[] = []
  if (b.date !== a.date) out.push(`${dayLabel(b.date)} → ${dayLabel(a.date)}`)
  if (b.title !== a.title) out.push(`« ${a.title} »`)
  if ((b.targetDurationS ?? null) !== (a.targetDurationS ?? null)) out.push(`Durée ${dur(b.targetDurationS)} → ${dur(a.targetDurationS)}`)
  if ((b.targetDistanceM ?? null) !== (a.targetDistanceM ?? null)) out.push(`Distance ${dist(b.targetDistanceM)} → ${dist(a.targetDistanceM)}`)
  if ((b.targetPaceMinSecPerKm ?? null) !== (a.targetPaceMinSecPerKm ?? null) || (b.targetPaceMaxSecPerKm ?? null) !== (a.targetPaceMaxSecPerKm ?? null))
    out.push(`Allure ${pace(b.targetPaceMinSecPerKm, b.targetPaceMaxSecPerKm)} → ${pace(a.targetPaceMinSecPerKm, a.targetPaceMaxSecPerKm)}`)
  if (stepsKey(b) !== stepsKey(a)) out.push((a.steps ?? []).length < (b.steps ?? []).length ? 'Structure simplifiée' : 'Structure modifiée')
  if ((b.instructions ?? '') !== (a.instructions ?? '') && out.length === 0) out.push('Consignes mises à jour')
  return out
}

/** Turns an adapt preview (removeIds + new sessions) into readable changes: moved / modified / removed / created. */
export function adaptDiff(current: PlannedSession[], preview: Pick<PlanAdaptPreview, 'removeIds' | 'sessions'>): PlanChange[] {
  const pool = preview.sessions.map((s, i) => ({ s, i }))
  const take = (pred: (s: PlannedSessionInput & { id?: number }) => boolean) => {
    const k = pool.findIndex((x) => pred(x.s))
    return k < 0 ? null : pool.splice(k, 1)[0].s
  }
  const out: PlanChange[] = []
  for (const id of preview.removeIds) {
    const b = current.find((p) => p.id === id)
    if (!b) continue
    const a = take((s) => !!s.id && s.id === id) ?? take((s) => s.sport === b.sport && norm(s.title) === norm(b.title)) ?? take((s) => s.sport === b.sport && s.date === b.date)
    if (!a) {
      out.push({ kind: 'removed', before: b, after: null, details: [dayLabel(b.date)] })
      continue
    }
    const d = details(b, a)
    if (d.length === 0) continue
    out.push({ kind: b.date !== a.date ? 'moved' : 'modified', before: b, after: a, details: d })
  }
  for (const { s } of pool) out.push({ kind: 'created', before: null, after: s, details: [dayLabel(s.date)] })
  const dateOf = (c: PlanChange) => c.after?.date ?? c.before?.date ?? ''
  return out.sort((x, y) => (dateOf(x) < dateOf(y) ? -1 : dateOf(x) > dateOf(y) ? 1 : 0))
}

export function changeCounts(changes: PlanChange[]): Record<ChangeKind, number> {
  const c = { moved: 0, modified: 0, removed: 0, created: 0 }
  for (const x of changes) c[x.kind]++
  return c
}
