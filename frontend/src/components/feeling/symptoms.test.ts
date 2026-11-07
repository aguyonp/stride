import { describe, expect, it } from 'vitest'
import type { BodyStressEntry } from '../../api/types'
import { diffSymptoms, hasChanges, initialSymptoms, newSymptom } from './symptoms'

const entry = (id: number, over: Partial<BodyStressEntry> = {}): BodyStressEntry => ({
  id,
  date: '2026-11-05',
  activityId: 12,
  activityName: 'Sortie',
  zone: 'knee_pfps',
  zoneLabel: 'Rotule',
  side: 'left',
  painScore: 3,
  coldNextDayScore: 2,
  note: '',
  createdAt: '',
  updatedAt: '',
  ...over,
})

describe('Ressenti symptoms', () => {
  it('starts from saved entries and offers tracked zones empty', () => {
    const rows = initialSymptoms([entry(1)], { mode: 'tracking', zones: [{ zone: 'achilles', side: 'right', since: null, note: '' }] })
    expect(rows).toEqual([
      { id: 1, zone: 'knee_pfps', side: 'left', during: 3, nextMorning: 2 },
      { id: null, zone: 'achilles', side: 'right', during: null, nextMorning: null },
    ])
    expect(newSymptom(rows, null)?.zone).toBe('shin_splints')
    expect(newSymptom([], { mode: 'tracking', zones: [{ zone: 'achilles', side: 'right', since: null, note: '' }] })).toMatchObject({ zone: 'achilles', side: 'right' })
  })

  it('writes one body entry per scored zone, updates changes and removes cleared ones', () => {
    const initial = initialSymptoms([entry(1), entry(2, { zone: 'achilles' })], null)
    const rows = [
      { ...initial[0], during: 5 },
      { ...initial[1], during: null, nextMorning: null },
      { id: null, zone: 'plantar_fascia' as const, side: 'both' as const, during: null, nextMorning: 4 },
      { id: null, zone: 'shin_splints' as const, side: 'left' as const, during: null, nextMorning: null },
    ]
    const c = diffSymptoms(initial, rows, 12, '2026-11-05')
    expect(c.update).toEqual([{ id: 1, zone: 'knee_pfps', side: 'left', painScore: 5, coldNextDayScore: 2 }])
    expect(c.remove).toEqual([2])
    expect(c.create).toEqual([{ zone: 'plantar_fascia', side: 'both', painScore: 0, coldNextDayScore: 4, date: '2026-11-05', activityId: 12 }])
    expect(hasChanges(diffSymptoms(initial, initial, 12, '2026-11-05'))).toBe(false)
    expect(diffSymptoms(initial, [initial[0]], 12, '2026-11-05').remove).toEqual([2])
  })
})
