import { describe, expect, it } from 'vitest'
import type { Profile } from '../api/types'
import { buildActivityAnalysis, buildDayAnalysis } from './analysis'
import { getCatalog } from './catalog'
import { todayISO } from '../lib/format'

const profile: Profile = {
  injuredSide: 'left',
  cadenceTargetMin: 175,
  cadenceTargetMax: 182,
  gctBalanceAlertPct: 1.5,
  verticalOscillationAlertCm: 9,
  descentAlertM: 30,
  rhrAlertDeltaBpm: 5,
  pace10kMinSecPerKm: 215,
  pace10kMaxSecPerKm: 220,
  paceMarathonMinSecPerKm: 230,
  paceMarathonMaxSecPerKm: 240,
}
const now = new Date()

describe('fixtures', () => {
  const runs = getCatalog().filter((it) => it.summary.isRunning)

  it('la dernière course est un run/walk ~7 km / 42:10, équilibre G/D indisponible', () => {
    const a = buildActivityAnalysis(runs[0], 'sfp', 0, profile, { now })
    expect(a.activity.durationS).toBe(2530)
    expect(a.activity.distanceM).toBeGreaterThan(6800)
    expect(a.activity.distanceM).toBeLessThan(7300)
    expect(a.segments.primary).toBe('blocks')
    expect(a.segments.blocks.map((b) => b.label).slice(0, 3)).toEqual(['Bloc 1', 'Pause 1', 'Bloc 2'])
    expect(a.segments.laps[0].label).toBe('Km 1')
    expect(a.biomechanics.gctBalance.available).toBe(false)
    expect(a.alerts.some((x) => x.code === 'GCT_BALANCE_NA' && x.level === 'info')).toBe(true)
    expect(a.report).toContain('### BIOMÉCANIQUE ROTULE & IMPACTS')
    expect(a.report).toContain('N/D (non mesuré par le capteur)')
    expect(a.series.tS.length).toBeLessThanOrEqual(400)
  })

  it('la variante a un équilibre G/D mesuré et des alertes danger', () => {
    const a = buildActivityAnalysis(runs[1], 'sfp', 14, profile, { now })
    expect(a.biomechanics.gctBalance.available).toBe(true)
    expect(a.biomechanics.gctBalance.leftPct).toBeGreaterThan(48.5)
    expect(a.biomechanics.gctBalance.leftPct).toBeLessThan(50)
    expect(a.alerts[0].level).toBe('danger')
    expect(a.history?.length).toBeGreaterThan(0)
    expect(a.report).toContain('### HISTORIQUE 14 JOURS')
  })

  it('performance et jour', () => {
    const p = buildActivityAnalysis(runs[0], 'performance', 0, profile, { now })
    expect(p.performance?.timeInZonesS).toHaveLength(5)
    expect(p.report).toContain('### PERFORMANCE')
    const d = buildDayAnalysis(todayISO(), 0, profile, { now })
    expect(d.alerts.some((x) => x.code === 'FLOORS_HIGH')).toBe(true)
    expect(d.recovery.trend).toHaveLength(7)
    expect(d.report).toContain('### TENDANCE 7 JOURS')
  })
})
