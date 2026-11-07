import { describe, expect, it } from 'vitest'
import type { CycleActuals, Goal, TrainingPhase, TrainingPlan, TrainingWeek } from '../../../api/types'
import {
  completionPct,
  countdownLabel,
  gaugeFill,
  ganttScale,
  isCurrentSpan,
  mergeWeeks,
  nextPhaseDates,
  numberField,
  parseOptionalNumber,
  phaseBar,
  planFormDefaults,
  resolveCurrentPlan,
  sessionsPct,
  timelineSpan,
  validatePhaseForm,
  validatePlanForm,
  validateWeekTargets,
} from './cycleModel'

const actuals: CycleActuals = {
  plannedVolumeKm: 40,
  actualVolumeKm: 30,
  plannedLoad: 300,
  actualLoad: 240,
  sessionsPlanned: 4,
  sessionsCompleted: 3,
  volumeCompletionPct: null,
  loadCompletionPct: null,
  plannedDurationS: 0,
  actualDurationS: 0,
}

const week = (weekStart: string, weekEnd: string, extra: Partial<TrainingWeek> = {}) =>
  ({ id: 0, weekStart, weekEnd, weekNumber: 1, isAssimilation: false, actuals, ...extra }) as TrainingWeek

const phase = (id: number, startDate: string, endDate: string, weeks: TrainingWeek[] = []) =>
  ({ id, startDate, endDate, weeks, actuals }) as TrainingPhase

const plan = (id: number, extra: Partial<TrainingPlan> = {}) =>
  ({ id, name: `Plan ${id}`, startDate: '2026-09-07', endDate: '2026-11-29', status: 'active', ...extra }) as TrainingPlan

describe('cycle gauges', () => {
  it('prefers the explicit completion, else derives it, and stays null without a plan', () => {
    expect(completionPct(30, 40, 81.6)).toBe(82)
    expect(completionPct(30, 40, null)).toBe(75)
    expect(completionPct(5, 0, null)).toBeNull()
  })

  it('clamps the bar, fills it when work exists without a plan and flags overload above 112 %', () => {
    expect(gaugeFill(30, 40, null)).toEqual({ pct: 75, width: 75, overload: false })
    expect(gaugeFill(60, 40, null)).toEqual({ pct: 150, width: 100, overload: true })
    expect(gaugeFill(5, 0, null)).toEqual({ pct: null, width: 100, overload: false })
    expect(gaugeFill(0, 0, null)).toEqual({ pct: null, width: 0, overload: false })
    expect(sessionsPct({ sessionsCompleted: 3, sessionsPlanned: 4 })).toBe(75)
    expect(sessionsPct({ sessionsCompleted: 1, sessionsPlanned: 0 })).toBeNull()
  })
})

describe('cycle timeline', () => {
  it('merges phase weeks with uncovered standalone weeks in chronological order', () => {
    const w1 = week('2026-09-07', '2026-09-13', { id: 1 })
    const w2 = week('2026-09-14', '2026-09-20', { id: 2 })
    const dup = week('2026-09-07', '2026-09-13', { id: 9 })
    const w0 = week('2026-08-31', '2026-09-06', { id: 3 })
    const merged = mergeWeeks([phase(1, '2026-09-07', '2026-09-20', [w2, w1])], [dup, w0])
    expect(merged.map((w) => w.id)).toEqual([3, 1, 2])
  })

  it('resolves the displayed plan: selection, then active, then first', () => {
    const plans = [plan(1), plan(2)]
    expect(resolveCurrentPlan(plans, 2, plans[0])?.id).toBe(2)
    expect(resolveCurrentPlan(plans, 99, plans[0])?.id).toBe(1)
    expect(resolveCurrentPlan(plans, null, null)?.id).toBe(1)
    expect(resolveCurrentPlan([], null, null)).toBeNull()
  })

  it('spans the plan, else the phases, else the weeks, else 12 weeks from this Monday', () => {
    expect(timelineSpan(plan(1), [], [], '2026-11-07')).toEqual({ start: '2026-09-07', end: '2026-11-29' })
    expect(timelineSpan(null, [phase(1, '2026-10-05', '2026-10-25'), phase(2, '2026-10-26', '2026-11-15')], [], '2026-11-07')).toEqual({
      start: '2026-10-05',
      end: '2026-11-15',
    })
    expect(timelineSpan(null, [], [week('2026-11-02', '2026-11-08')], '2026-11-07')).toEqual({ start: '2026-11-02', end: '2026-11-08' })
    expect(timelineSpan(null, [], [], '2026-11-07')).toEqual({ start: '2026-11-02', end: '2027-01-24' })
  })

  it('places phase bars and the today marker as percentages of the span', () => {
    const scale = ganttScale('2026-11-02', '2026-11-29', '2026-11-09')
    expect(scale.totalDays).toBe(28)
    expect(scale.todayPct).toBe(25)
    expect(ganttScale('2026-11-02', '2026-11-29', '2026-12-31').todayPct).toBeNull()
    expect(ganttScale('2026-11-02', '2026-11-03', '2026-11-02').totalDays).toBe(7)
    expect(phaseBar('2026-11-02', 28, { startDate: '2026-11-09', endDate: '2026-11-15' })).toEqual({ leftPct: 25, widthPct: 25 })
    expect(phaseBar('2026-11-02', 28, { startDate: '2026-10-01', endDate: '2026-10-01' }).leftPct).toBe(0)
    expect(phaseBar('2026-11-02', 28, { startDate: '2026-12-30', endDate: '2026-12-31' })).toEqual({ leftPct: 98, widthPct: 2 })
    expect(isCurrentSpan('2026-11-02', '2026-11-08', '2026-11-08')).toBe(true)
    expect(isCurrentSpan('2026-11-02', '2026-11-08', '2026-11-09')).toBe(false)
  })

  it('chains a new phase after the last one and ends it with the plan', () => {
    expect(nextPhaseDates([phase(1, '2026-09-07', '2026-10-04')], plan(1), '2026-09-07')).toEqual({ start: '2026-10-05', end: '2026-11-29' })
    expect(nextPhaseDates([], null, '2026-09-07')).toEqual({ start: '2026-09-07', end: '2026-10-04' })
  })

  it('labels the countdown to the target date', () => {
    expect(countdownLabel('2026-11-07', '2026-11-08')).toBe('J−1 · 1 semaine restante')
    expect(countdownLabel('2026-11-07', '2026-11-22')).toBe('J−15 · 3 semaines restantes')
    expect(countdownLabel('2026-11-07', '2026-11-07')).toBe('Jour J aujourd’hui')
    expect(countdownLabel('2026-11-07', '2026-11-01')).toBe('Échéance passée')
  })
})

describe('cycle forms', () => {
  it('parses optional French numbers and renders numbers back into fields', () => {
    expect(parseOptionalNumber('  ')).toBeNull()
    expect(parseOptionalNumber('42,5')).toBe(42.5)
    expect(parseOptionalNumber('abc')).toBeNaN()
    expect(numberField(0)).toBe('0')
    expect(numberField(null)).toBe('')
    expect(numberField(undefined)).toBe('')
  })

  it('prefills a new plan from the next active race goal, or keeps an edited plan as is', () => {
    const goals = [
      { id: 1, title: 'Semi passé', status: 'active', targetDate: '2026-10-01' },
      { id: 2, title: 'Marathon de Nice', status: 'active', targetDate: '2026-12-13' },
    ] as Goal[]
    expect(planFormDefaults(null, goals, '2026-11-07')).toMatchObject({
      name: 'Préparation Marathon de Nice',
      goalId: 2,
      startDate: '2026-11-02',
      endDate: '2026-12-13',
      targetEventLabel: 'Marathon de Nice',
      sport: 'run',
      status: 'active',
    })
    expect(planFormDefaults(null, [], '2026-11-07')).toMatchObject({ name: 'Préparation course', goalId: null, endDate: '2027-01-24' })
    expect(planFormDefaults(plan(6, { goalId: null, targetEventLabel: '' }), goals, '2026-11-07')).toMatchObject({ goalId: null, targetEventLabel: '' })
    expect(planFormDefaults(plan(5, { goalId: 7, sport: 'bike', note: 'n' }), goals, '2026-11-07')).toMatchObject({
      name: 'Plan 5',
      goalId: 7,
      sport: 'bike',
      startDate: '2026-09-07',
      note: 'n',
    })
  })

  it('validates plans, phases and week targets with French messages', () => {
    const okPlan = { name: 'Prépa', startDate: '2026-11-02', endDate: '2026-12-13', peakVolume: 52, peakLoad: null }
    expect(validatePlanForm(okPlan)).toBeNull()
    expect(validatePlanForm({ ...okPlan, name: ' ' })).toBe('Indique un nom pour ce macrocycle.')
    expect(validatePlanForm({ ...okPlan, endDate: '2026-11-02' })).toBe('La date de fin doit être postérieure au début.')
    expect(validatePlanForm({ ...okPlan, peakVolume: Number.NaN })).toBe('Volume pic invalide.')
    expect(validatePlanForm({ ...okPlan, peakLoad: 0 })).toBe('Charge pic invalide.')

    expect(validatePhaseForm({ name: 'Bloc', startDate: '2026-11-02', endDate: '2026-11-02' })).toBeNull()
    expect(validatePhaseForm({ name: '', startDate: '2026-11-02', endDate: '2026-11-08' })).toBe('Donne un nom à ce mésocycle.')
    expect(validatePhaseForm({ name: 'Bloc', startDate: '2026-11-08', endDate: '2026-11-02' })).toBe('La fin du mésocycle doit suivre le début.')

    expect(validateWeekTargets(0, null)).toBeNull()
    expect(validateWeekTargets(-1, null)).toBe('Volume cible invalide.')
    expect(validateWeekTargets(null, Number.NaN)).toBe('Charge cible invalide.')
  })
})
