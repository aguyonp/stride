import { describe, expect, it } from 'vitest'
import { symptomWindowDays } from '../../api/queriesV41'
import { clampMonth, yearsBack } from '../session/monthJump'
import { historyFrom, historyProgress } from './historyRange'
import { setLang } from '../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

describe('history import and old sessions', () => {
  it('maps the chosen range to the `from` date of POST /api/sync/history', () => {
    expect(historyFrom('all', '2026-11-07', '')).toBeNull()
    expect(historyFrom('12m', '2026-11-07', '')).toBe('2025-11-07')
    expect(historyFrom('24m', '2026-11-07', '')).toBe('2024-11-07')
    expect(historyFrom('date', '2026-11-07', '2021-03-01')).toBe('2021-03-01')
    expect(historyFrom('date', '2026-11-07', '')).toBeUndefined()
    expect(historyFrom('date', '2026-11-07', '2027-01-01')).toBeUndefined()
  })

  it('describes job progress', () => {
    expect(historyProgress({ state: 'running', processed: 40, total: 160, progressPct: null })).toEqual({ pct: 25, label: '40 / 160 séances' })
    expect(historyProgress({ state: 'completed', processed: 160, total: 160, progressPct: 100 }).label).toBe('160 séances importées')
    expect(historyProgress({ state: 'queued', processed: 0, total: null, progressPct: null }).pct).toBe(0)
  })

  it('lets the session picker reach any past month but not the future', () => {
    expect(yearsBack('2026-11-07', 2023)).toEqual([2026, 2025, 2024, 2023])
    expect(clampMonth(2019, 3, '2026-11-07')).toEqual({ y: 2019, m: 3 })
    expect(clampMonth(2026, 11, '2026-11-07')).toEqual({ y: 2026, m: 10 })
    expect(symptomWindowDays('2026-11-01', '2026-11-07')).toBe(90)
    expect(symptomWindowDays('2026-01-01', '2026-11-07')).toBe(311)
    expect(symptomWindowDays('2010-01-01', '2026-11-07')).toBe(3650)
  })
})
