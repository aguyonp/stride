import { beforeEach, describe, expect, it } from 'vitest'
import { ALL_INCLUDE } from '../lib/include'
import { todayISO } from '../lib/format'
import { db, resetDb, userData } from './db'
import { visibleItems } from './domain'
import { setFlags } from './flags'
import { activityReport, dayReport, DIRECTIVE, multiReport } from './report'
import { buildActivityView, buildDayView } from './view'

function setup(flags: string[]) {
  setFlags(flags)
  resetDb()
  const me = db().users.find((u) => (flags.includes('user') ? u.role === 'user' : u.role === 'admin'))!
  const d = userData(me.id)
  const run = visibleItems(d).find((i) => i.isRunning)!
  return { d, run, view: buildActivityView(run, d) }
}

describe('session prompt', () => {
  beforeEach(() => resetDb())

  it('starts with the verbatim directive and gates sections by chips', () => {
    const { d, view } = setup(['admin', 'fusion'])
    const full = activityReport(view, d, ALL_INCLUDE)
    expect(full.startsWith(DIRECTIVE)).toBe(true)
    for (const h of ['### CONTEXTE & OBJECTIFS', '### RAPPORT SÉANCE', '### BIOMÉCANIQUE & DÉRIVE DE FATIGUE', '### DÉTAIL DES INTERVALLES', '### CHARGE, STAMINA', '### RÉCUPÉRATION & PRÉPARATION', '### HISTORIQUE 14 JOURS', '### ALERTES AUTOMATIQUES']) {
      expect(full).toContain(h)
    }
    const min = activityReport(view, d, [])
    expect(min.startsWith(DIRECTIVE)).toBe(true)
    expect(min).toContain('### RAPPORT SÉANCE')
    expect(min).toContain('### ALERTES AUTOMATIQUES')
    expect(min).not.toContain('### BIOMÉCANIQUE')
    expect(min).not.toContain('### HISTORIQUE 14 JOURS')
    expect(min).not.toContain('Ressenti & Douleur')
  })

  it('never hard-codes the cadence target and keeps the Stamina label', () => {
    const { d, view } = setup(['admin', 'fusion'])
    const md = activityReport(view, d, ALL_INCLUDE)
    const th = d.profile.thresholds
    expect(md).toMatch(new RegExp(`${th.cadenceTargetMin}\\s*[-–]\\s*${th.cadenceTargetMax}`))
    expect(md).toContain('Stamina Fenix 8')
  })
})

describe('Strava-only views', () => {
  it('returns null Garmin metrics with reasons and a Strava label', () => {
    const { view } = setup(['user', 'strava'])
    expect(view.sourceLabel).toBe('Strava')
    expect(view.biomechanics.groundContactTimeMs).toBeNull()
    expect(view.biomechanics.gctBalance.available).toBe(false)
    expect(view.biomechanics.gctBalance.reason).toMatch(/Garmin/)
    expect(view.recovery).toBeNull()
    expect(view.warnings.join(' ')).toMatch(/GAP/)
  })
})

describe('day and multi-session prompts', () => {
  it('day report has the health section and no splits', () => {
    const { d } = setup(['admin', 'fusion'])
    const md = dayReport(buildDayView(todayISO(), d), d, ALL_INCLUDE)
    expect(md.startsWith(DIRECTIVE)).toBe(true)
    expect(md).toContain('### BILAN SANTÉ & CHARGE QUOTIDIENNE')
    expect(md).not.toContain('### DÉTAIL DES INTERVALLES')
  })

  it('multi export summarises N weeks with the request section', () => {
    const { d } = setup(['admin', 'fusion'])
    const { markdown, sessions } = multiReport(d, { weeks: 4, include: ALL_INCLUDE })
    expect(markdown.startsWith(DIRECTIVE)).toBe(true)
    expect(sessions).toBeGreaterThan(0)
    expect(markdown).toContain('### RÉCAPITULATIF DES SÉANCES')
    expect(markdown).toContain('### TENDANCES HEBDOMADAIRES')
    expect(markdown).toContain('### DEMANDE')
    expect(markdown).not.toContain('### DÉTAIL DES INTERVALLES')
  })
})
