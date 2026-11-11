import { describe, expect, it } from 'vitest'
import type { PlannedSession, PlannedSessionInput } from '../../api/types'
import { adaptDiff, changeCounts } from './adaptDiff'
import { setLang } from '../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const base = { sport: 'run' as const, targetDistanceM: null, targetPaceMinSecPerKm: null, targetPaceMaxSecPerKm: null, targetCadenceSpm: null, instructions: '', steps: [] }
const planned = (id: number, date: string, title: string, durationS = 2700): PlannedSession => ({
  ...base,
  id,
  date,
  title,
  targetDurationS: durationS,
  match: null,
  source: 'manual',
  garmin: { status: 'none', workoutId: null, pushedAt: null, error: null },
})
const draft = (date: string, title: string, durationS = 2700): PlannedSessionInput => ({ ...base, date, title, targetDurationS: durationS })

describe('adaptDiff', () => {
  const current = [planned(1, '2026-11-10', 'Fractionné 6×1′'), planned(2, '2026-11-12', 'Footing 45′'), planned(3, '2026-11-14', 'Sortie longue'), planned(4, '2026-11-16', 'Inchangée')]

  it('classifies moved, modified, removed and created sessions', () => {
    const changes = adaptDiff(current, {
      removeIds: [1, 2, 3, 4, 99],
      sessions: [draft('2026-11-13', 'Footing 45′'), draft('2026-11-10', 'Footing facile 30′', 1800), draft('2026-11-16', 'Inchangée'), draft('2026-11-18', 'Rappel allure')],
    })
    const kinds = changes.map((c) => `${c.kind}:${c.before?.id ?? c.after?.title}`)
    expect(kinds).toEqual(['modified:1', 'moved:2', 'removed:3', 'created:Rappel allure'])
    const mod = changes[0]
    expect(mod.details.some((d) => d.includes('Footing facile'))).toBe(true)
    expect(mod.details.some((d) => d.startsWith('Durée'))).toBe(true)
    expect(changes[1].details[0]).toContain('→')
    expect(changeCounts(changes)).toEqual({ moved: 1, modified: 1, removed: 1, created: 1 })
  })

  it('pairs by explicit id first and ignores unknown ids', () => {
    const changes = adaptDiff(current, { removeIds: [3], sessions: [{ ...draft('2026-11-15', 'Autre titre'), id: 3 } as PlannedSessionInput] })
    expect(changes).toHaveLength(1)
    expect(changes[0].kind).toBe('moved')
    expect(adaptDiff(current, { removeIds: [42], sessions: [] })).toEqual([])
  })
})
