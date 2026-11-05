import type { Goal, PlanPhase } from '../../api/types'
import { diffDays, parseLocal } from '../../lib/format'

/** Active goals with a target date not yet passed, nearest first. */
export function datedGoals(goals: Goal[], today: string): Goal[] {
  return goals.filter((g) => g.status === 'active' && g.targetDate && g.targetDate >= today).sort((a, b) => (a.targetDate! < b.targetDate! ? -1 : 1))
}

/** Goal shown on the horizon: the one the phases lead to, else the nearest race, else the nearest dated goal. */
export function horizonGoal(dated: Goal[], phases: PlanPhase[]): Goal | null {
  return dated.find((g) => phases.some((p) => p.endDate === g.targetDate)) ?? dated.find((g) => g.kind === 'race') ?? dated[0] ?? null
}

/** « J-12 » until the target, « Jour J » on the day, null once passed. */
export function countdown(target: string, today: string): { days: number; label: string } | null {
  const days = diffDays(today, target)
  if (days < 0) return null
  return { days, label: days === 0 ? 'Jour J' : `J-${days}` }
}

export const phaseAt = (phases: PlanPhase[], date: string): PlanPhase | null => phases.find((p) => p.startDate <= date && p.endDate >= date) ?? null

export interface PhaseSegment {
  phase: PlanPhase
  leftPct: number
  widthPct: number
  current: boolean
  past: boolean
}

const pct = (v: number) => Math.round(v * 10000) / 100

/** Positions phases on a [from, to] timeline (inclusive days), clamped; today marker when inside. */
export function phaseLayout(phases: PlanPhase[], from: string, to: string, today: string): { segments: PhaseSegment[]; todayPct: number | null; totalDays: number } {
  const totalDays = Math.max(1, diffDays(from, to) + 1)
  const segments = [...phases]
    .sort((a, b) => (a.startDate < b.startDate ? -1 : 1))
    .filter((p) => p.endDate >= from && p.startDate <= to)
    .map((p) => {
      const s = p.startDate < from ? from : p.startDate
      const e = p.endDate > to ? to : p.endDate
      return { phase: p, leftPct: pct(diffDays(from, s) / totalDays), widthPct: pct((diffDays(s, e) + 1) / totalDays), current: p.startDate <= today && p.endDate >= today, past: p.endDate < today }
    })
  const todayPct = today >= from && today <= to ? pct(diffDays(from, today) / totalDays) : null
  return { segments, todayPct, totalDays }
}

const MONTHS = ['janv.', 'févr.', 'mars', 'avr.', 'mai', 'juin', 'juil.', 'août', 'sept.', 'oct.', 'nov.', 'déc.']

/** First day of each month strictly after `from` up to `to`, positioned on the same scale as phaseLayout. */
export function monthTicks(from: string, to: string): { label: string; pct: number }[] {
  const totalDays = Math.max(1, diffDays(from, to) + 1)
  const out: { label: string; pct: number }[] = []
  const p = parseLocal(from)
  if (!p) return out
  let y = p.y
  let m = p.mo
  for (;;) {
    m++
    if (m > 12) {
      m = 1
      y++
    }
    const iso = `${y}-${String(m).padStart(2, '0')}-01`
    if (iso > to) break
    out.push({ label: m === 1 ? `${MONTHS[0]} ${y}` : MONTHS[m - 1], pct: pct(diffDays(from, iso) / totalDays) })
  }
  return out
}

/** Visual tone of a phase from its name (build, recovery, specific, taper), falling back on its rank. */
export function phaseTone(name: string, index: number): 'accent' | 'ok' | 'warn' | 'violet' | 'info' {
  const n = name.toLowerCase()
  if (/aff[uû]t|taper|course|compét/.test(n)) return 'violet'
  if (/assimil|récup|recup|allég|coupure/.test(n)) return 'ok'
  if (/spécif|specif|seuil|vma|qualit/.test(n)) return 'warn'
  if (/dével|devel|base|foncier|volume|reprise/.test(n)) return 'accent'
  return (['accent', 'info', 'warn', 'violet'] as const)[index % 4]
}
