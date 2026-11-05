import { describe, expect, it } from 'vitest'
import type { Goal, PlanPhase } from '../../api/types'
import { countdown, datedGoals, horizonGoal, monthTicks, phaseAt, phaseLayout, phaseTone } from './longterm'

const phase = (id: number, name: string, startDate: string, endDate: string): PlanPhase => ({ id, name, startDate, endDate, focus: '', weeklyVolumeKm: null, note: '' })
const goal = (id: number, targetDate: string | null, status: Goal['status'] = 'active'): Goal => ({ id, title: `G${id}`, kind: 'race', targetValue: null, targetUnit: null, targetDate, status, note: '' })

describe('long-term helpers', () => {
  it('counts down to the goal and stops once passed', () => {
    expect(countdown('2026-11-17', '2026-11-05')).toEqual({ days: 12, label: 'J-12' })
    expect(countdown('2026-11-05', '2026-11-05')?.label).toBe('Jour J')
    expect(countdown('2026-11-04', '2026-11-05')).toBeNull()
    expect(countdown('2027-01-01', '2026-12-31')?.days).toBe(1)
  })

  it('keeps only active future dated goals, nearest first', () => {
    const list = datedGoals([goal(1, '2027-03-01'), goal(2, null), goal(3, '2026-11-20'), goal(4, '2026-10-01'), goal(5, '2026-12-01', 'done')], '2026-11-05')
    expect(list.map((g) => g.id)).toEqual([3, 1])
  })

  it('prefers the goal the phases lead to, then the nearest race', () => {
    const macro = { ...goal(1, '2026-11-17'), kind: 'macro' as const }
    const race = goal(2, '2027-04-04')
    expect(horizonGoal([macro, race], [])?.id).toBe(2)
    expect(horizonGoal([macro], [])?.id).toBe(1)
    expect(horizonGoal([macro, race], [phase(1, 'Affûtage', '2026-11-10', '2026-11-17')])?.id).toBe(1)
    expect(horizonGoal([], [])).toBeNull()
  })

  it('lays phases out on the timeline, clamped, with a today marker', () => {
    const phases = [phase(2, 'Spécifique', '2026-11-11', '2026-11-20'), phase(1, 'Développement', '2026-10-25', '2026-11-10')]
    const { segments, todayPct, totalDays } = phaseLayout(phases, '2026-11-01', '2026-11-20', '2026-11-06')
    expect(totalDays).toBe(20)
    expect(segments.map((s) => s.phase.id)).toEqual([1, 2])
    expect(segments[0]).toMatchObject({ leftPct: 0, widthPct: 50, current: true, past: false })
    expect(segments[1]).toMatchObject({ leftPct: 50, widthPct: 50, current: false })
    expect(segments.reduce((s, x) => s + x.widthPct, 0)).toBe(100)
    expect(todayPct).toBe(25)
    expect(phaseLayout(phases, '2026-11-01', '2026-11-20', '2026-12-01').todayPct).toBeNull()
    expect(phaseLayout([phase(3, 'Avant', '2026-09-01', '2026-09-30')], '2026-11-01', '2026-11-20', '2026-11-06').segments).toHaveLength(0)
  })

  it('finds the phase of a date and month ticks', () => {
    const phases = [phase(1, 'Développement', '2026-11-01', '2026-11-10')]
    expect(phaseAt(phases, '2026-11-10')?.id).toBe(1)
    expect(phaseAt(phases, '2026-11-11')).toBeNull()
    const ticks = monthTicks('2026-11-15', '2027-02-14')
    expect(ticks.map((t) => t.label)).toEqual(['déc.', 'janv. 2027', 'févr.'])
    expect(ticks[0].pct).toBeGreaterThan(0)
  })

  it('picks a phase tone from its name', () => {
    expect(phaseTone('Affûtage', 0)).toBe('violet')
    expect(phaseTone('Assimilation', 0)).toBe('ok')
    expect(phaseTone('Spécifique 10 km', 0)).toBe('warn')
    expect(phaseTone('Développement', 3)).toBe('accent')
    expect(phaseTone('Bloc X', 1)).toBe('info')
  })
})
