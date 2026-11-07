import type { BodySide, BodyStressEntry, BodyStressInput, BodyZone, HealthStatusInput } from '../../api/types'
import { ALL_BODY_ZONES } from '../bodymap/bodyMapMeta'
import { primaryZone } from '../health/healthStatus'

/** One declared symptom of a session; `id` is the linked body entry once saved. */
export interface SymptomDraft {
  id: number | null
  zone: BodyZone
  side: BodySide
  during: number | null
  nextMorning: number | null
}

const toSymptomDraft = (e: BodyStressEntry): SymptomDraft => ({ id: e.id, zone: e.zone, side: e.side, during: e.painScore, nextMorning: e.coldNextDayScore })

/** Saved entries first; tracked zones without an entry are offered empty (nothing is written unless scored). */
export function initialSymptoms(entries: BodyStressEntry[], health: HealthStatusInput | null | undefined): SymptomDraft[] {
  const rows = entries.map(toSymptomDraft)
  if (health?.mode === 'tracking') {
    for (const z of health.zones) {
      if (!rows.some((r) => r.zone === z.zone)) rows.push({ id: null, zone: z.zone, side: z.side, during: null, nextMorning: null })
    }
  }
  return rows
}

export function newSymptom(rows: SymptomDraft[], health: HealthStatusInput | null | undefined): SymptomDraft | null {
  const preferred = primaryZone(health)
  const zone = rows.some((r) => r.zone === preferred.zone) ? ALL_BODY_ZONES.find((z) => !rows.some((r) => r.zone === z)) : preferred.zone
  return zone ? { id: null, zone, side: zone === preferred.zone ? preferred.side : 'left', during: null, nextMorning: null } : null
}

const scored = (r: SymptomDraft) => r.during !== null || r.nextMorning !== null

export interface SymptomChanges {
  create: BodyStressInput[]
  update: (Partial<BodyStressInput> & { id: number })[]
  remove: number[]
}

/** Body-entry writes turning the edited rows into the single source of truth for this activity. */
export function diffSymptoms(initial: SymptomDraft[], rows: SymptomDraft[], activityId: number, date: string): SymptomChanges {
  const out: SymptomChanges = { create: [], update: [], remove: [] }
  for (const r of rows) {
    const body = { zone: r.zone, side: r.side, painScore: r.during ?? 0, coldNextDayScore: r.nextMorning }
    if (r.id === null) {
      if (scored(r)) out.create.push({ ...body, date, activityId })
      continue
    }
    if (!scored(r)) {
      out.remove.push(r.id)
      continue
    }
    const before = initial.find((x) => x.id === r.id)
    if (!before || before.zone !== r.zone || before.side !== r.side || before.during !== r.during || before.nextMorning !== r.nextMorning) out.update.push({ id: r.id, ...body })
  }
  for (const b of initial) if (b.id !== null && !rows.some((r) => r.id === b.id)) out.remove.push(b.id)
  return out
}

export const hasChanges = (c: SymptomChanges) => c.create.length + c.update.length + c.remove.length > 0
