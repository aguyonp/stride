import type { CycleActuals, CyclePlanStatus, Goal, TrainingPhase, TrainingPlan, TrainingWeek } from '../../../api/types'
import { addDays, diffDays, mondayOf } from '../../../lib/format'

export function completionPct(actual: number, planned: number, explicitPct: number | null): number | null {
  if (explicitPct !== null && Number.isFinite(explicitPct)) return Math.round(explicitPct)
  if (planned <= 0) return null
  return Math.round((actual / planned) * 100)
}

/** Bar fill for a planned-vs-actual gauge; overload above 112 % of plan. */
export function gaugeFill(actual: number, planned: number, explicitPct: number | null) {
  const pct = completionPct(actual, planned, explicitPct)
  return {
    pct,
    width: pct === null ? (actual > 0 ? 100 : 0) : Math.max(0, Math.min(100, pct)),
    overload: pct !== null && pct > 112,
  }
}

export function sessionsPct(t: Pick<CycleActuals, 'sessionsCompleted' | 'sessionsPlanned'>): number | null {
  return t.sessionsPlanned > 0 ? Math.round((t.sessionsCompleted / t.sessionsPlanned) * 100) : null
}

export const isCurrentSpan = (start: string, end: string, today: string) => start <= today && end >= today

/** Phase weeks first, then standalone weeks not already covered, sorted chronologically. */
export function mergeWeeks(phases: TrainingPhase[], standalone: TrainingWeek[]): TrainingWeek[] {
  const map = new Map<string, TrainingWeek>()
  for (const ph of phases) for (const w of ph.weeks) map.set(w.weekStart, w)
  for (const w of standalone) if (!map.has(w.weekStart)) map.set(w.weekStart, w)
  return [...map.values()].sort((a, b) => a.weekStart.localeCompare(b.weekStart))
}

export function resolveCurrentPlan(
  plans: TrainingPlan[],
  selectedId: number | null,
  activePlan: TrainingPlan | null,
): TrainingPlan | null {
  return (selectedId !== null ? plans.find((p) => p.id === selectedId) : null) ?? activePlan ?? plans[0] ?? null
}

export function timelineSpan(
  plan: TrainingPlan | null,
  phases: TrainingPhase[],
  weeks: TrainingWeek[],
  today: string,
): { start: string; end: string } {
  const start = plan?.startDate ?? phases[0]?.startDate ?? weeks[0]?.weekStart ?? mondayOf(today)
  const end = plan?.endDate ?? phases[phases.length - 1]?.endDate ?? weeks[weeks.length - 1]?.weekEnd ?? addDays(start, 83)
  return { start, end }
}

export function countdownLabel(today: string, endDate: string): string {
  const days = diffDays(today, endDate)
  if (days === 0) return 'Jour J aujourd’hui'
  if (days < 0) return 'Échéance passée'
  const weeks = Math.ceil(days / 7)
  const s = weeks > 1 ? 's' : ''
  return `J−${days} · ${weeks} semaine${s} restante${s}`
}

export function ganttScale(start: string, end: string, today: string) {
  const totalDays = Math.max(7, diffDays(start, end) + 1)
  const todayOffset = diffDays(start, today)
  const todayPct = todayOffset >= 0 && todayOffset <= totalDays ? (todayOffset / totalDays) * 100 : null
  return { totalDays, todayPct }
}

export function phaseBar(spanStart: string, totalDays: number, phase: Pick<TrainingPhase, 'startDate' | 'endDate'>) {
  const leftDays = Math.max(0, diffDays(spanStart, phase.startDate))
  const spanDays = Math.max(1, diffDays(phase.startDate, phase.endDate) + 1)
  const leftPct = Math.min(98, (leftDays / totalDays) * 100)
  const widthPct = Math.max(2, Math.min(100 - leftPct, (spanDays / totalDays) * 100))
  return { leftPct, widthPct }
}

export function nextPhaseDates(phases: TrainingPhase[], plan: TrainingPlan | null, spanStart: string) {
  return {
    start: phases.length ? addDays(phases[phases.length - 1].endDate, 1) : spanStart,
    end: plan?.endDate ?? addDays(spanStart, 27),
  }
}

/** Empty input → null; accepts a French decimal comma. NaN is left for the validators to reject. */
export function parseOptionalNumber(raw: string): number | null {
  return raw.trim() ? Number(raw.replace(',', '.')) : null
}

export const numberField = (v: number | null | undefined) => (v !== null && v !== undefined ? String(v) : '')

export function planFormDefaults(plan: TrainingPlan | null, goals: Goal[], today: string) {
  const raceGoal = goals.find((g) => g.status === 'active' && g.targetDate && g.targetDate > today) ?? null
  return {
    name: plan?.name ?? (raceGoal ? `Préparation ${raceGoal.title}` : 'Préparation course'),
    sport: plan?.sport ?? ('run' as const),
    goalId: plan?.goalId ?? raceGoal?.id ?? null,
    startDate: plan?.startDate ?? mondayOf(today),
    endDate: plan?.endDate ?? raceGoal?.targetDate ?? addDays(mondayOf(today), 83),
    targetEventLabel: plan?.targetEventLabel ?? raceGoal?.title ?? '',
    status: plan?.status ?? ('active' as CyclePlanStatus),
    note: plan?.note ?? '',
  }
}

export function validatePlanForm(f: {
  name: string
  startDate: string
  endDate: string
  peakVolume: number | null
  peakLoad: number | null
}): string | null {
  if (!f.name.trim()) return 'Indique un nom pour ce macrocycle.'
  if (!f.startDate || !f.endDate || f.endDate <= f.startDate) return 'La date de fin doit être postérieure au début.'
  if (f.peakVolume !== null && (!Number.isFinite(f.peakVolume) || f.peakVolume <= 0)) return 'Volume pic invalide.'
  if (f.peakLoad !== null && (!Number.isFinite(f.peakLoad) || f.peakLoad <= 0)) return 'Charge pic invalide.'
  return null
}

export function validatePhaseForm(f: { name: string; startDate: string; endDate: string }): string | null {
  if (!f.name.trim()) return 'Donne un nom à ce mésocycle.'
  if (!f.startDate || !f.endDate || f.endDate < f.startDate) return 'La fin du mésocycle doit suivre le début.'
  return null
}

export function validateWeekTargets(volumeKm: number | null, load: number | null): string | null {
  if (volumeKm !== null && (!Number.isFinite(volumeKm) || volumeKm < 0)) return 'Volume cible invalide.'
  if (load !== null && (!Number.isFinite(load) || load < 0)) return 'Charge cible invalide.'
  return null
}
