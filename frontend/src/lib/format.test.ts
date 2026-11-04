import { describe, expect, it } from 'vitest'
import {
  addDays,
  diffDays,
  fmtDuration,
  fmtHM,
  fmtKm,
  fmtNumber,
  fmtPace,
  fmtPct,
  fmtRelative,
  fmtSigned,
  fmtUnit,
  ND,
  parsePace,
  todayISO,
} from './format'

// Les séparateurs FR (espace fine insécable, insécable) sont normalisés pour la lisibilité.
const norm = (s: string) => s.replace(/[  ]/g, ' ')

describe('allure', () => {
  it('formate m:ss/km', () => {
    expect(fmtPace(343)).toBe('5:43/km')
    expect(fmtPace(360)).toBe('6:00/km')
    expect(fmtPace(215)).toBe('3:35/km')
  })
  it('arrondit sans produire :60', () => {
    expect(fmtPace(359.6)).toBe('6:00/km')
  })
  it('null / 0 → N/D', () => {
    expect(fmtPace(null)).toBe(ND)
    expect(fmtPace(0)).toBe(ND)
    expect(fmtPace(Number.NaN)).toBe(ND)
  })
  it('parse les saisies m:ss', () => {
    expect(parsePace('3:35')).toBe(215)
    expect(parsePace(' 4:00 ')).toBe(240)
    expect(parsePace("3'40")).toBe(220)
    expect(parsePace('3:75')).toBeNull()
    expect(parsePace('abc')).toBeNull()
  })
})

describe('durées', () => {
  it('chrono', () => {
    expect(fmtDuration(2530)).toBe('42:10')
    expect(fmtDuration(3730)).toBe('1:02:10')
    expect(fmtDuration(59)).toBe('0:59')
    expect(fmtDuration(null)).toBe(ND)
  })
  it('heures-minutes', () => {
    expect(fmtHM(4920)).toBe('1h22')
    expect(fmtHM(25920)).toBe('7h12')
    expect(fmtHM(3600)).toBe('1h00')
    expect(norm(fmtHM(2700))).toBe('45 min')
    expect(fmtHM(undefined)).toBe(ND)
  })
})

describe('nombres', () => {
  it('km avec virgule', () => {
    expect(norm(fmtKm(7020))).toBe('7,02 km')
    expect(norm(fmtKm(80))).toBe('0,08 km')
    expect(fmtKm(null)).toBe(ND)
  })
  it('milliers et décimales', () => {
    expect(norm(fmtNumber(8432))).toBe('8 432')
    expect(norm(fmtNumber(3.1, 1))).toBe('3,1')
    expect(norm(fmtUnit(248, 'ms'))).toBe('248 ms')
    expect(norm(fmtPct(49.2))).toBe('49,2 %')
  })
  it('signés', () => {
    expect(fmtSigned(4)).toBe('+4')
    expect(fmtSigned(-3.2, 1)).toBe('−3,2')
    expect(fmtSigned(0)).toBe('0')
  })
  it("jamais 0 inventé", () => {
    expect(fmtNumber(null)).toBe(ND)
    expect(fmtNumber(0)).toBe('0')
  })
})

describe('dates', () => {
  const now = new Date(2026, 10, 3, 9, 0) // 3 nov. 2026
  it('relatives', () => {
    expect(fmtRelative('2026-11-03T08:10:00', now)).toBe("aujourd'hui 08:10")
    expect(fmtRelative('2026-11-02T18:30:12', now)).toBe('hier 18:30')
    expect(fmtRelative('2026-10-31T10:00:00', now)).toBe('samedi 10:00')
    expect(fmtRelative('2026-10-20T07:05:00', now)).toBe('20 oct. 07:05')
    expect(fmtRelative('2025-10-20T07:05:00', now)).toBe('20 oct. 2025')
  })
  it('arithmétique', () => {
    expect(addDays('2026-11-01', -1)).toBe('2026-10-31')
    expect(addDays('2026-12-31', 1)).toBe('2027-01-01')
    expect(diffDays('2026-10-28', '2026-11-03')).toBe(6)
  })
  it('aujourd’hui dans un fuseau', () => {
    const late = new Date(Date.UTC(2026, 10, 3, 23, 30))
    expect(todayISO('Europe/Paris', late)).toBe('2026-11-04')
    expect(todayISO('America/New_York', late)).toBe('2026-11-03')
  })
})
