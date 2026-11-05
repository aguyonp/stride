import { beforeEach, describe, expect, it } from 'vitest'
import { addDays, todayISO } from '../lib/format'
import { db, resetDb, userData } from './db'
import { visibleItems } from './domain'
import { setFlags } from './flags'
import { mockFetch } from './server'
import { calendar, efficiencyTrend, enrichActivityView, parsePlanText, readinessFor, trackFor } from './v3'
import { buildActivityView } from './view'

function setup(flags: string[]) {
  setFlags(flags)
  resetDb()
  const me = db().users.find((u) => u.role === 'admin')!
  const d = userData(me.id)
  const run = visibleItems(d).find((i) => i.isRunning)!
  return { d, run, view: enrichActivityView(buildActivityView(run, d), run, d) }
}
const post = (path: string, body: unknown) => mockFetch(path, { method: 'POST', headers: { 'X-Stride-CSRF': '1' }, body: JSON.stringify(body) })

describe('v3 mocks', () => {
  beforeEach(() => resetDb())

  it('parses a fenced stride-plan answer and reports bad sessions', () => {
    const text = 'Voici :\n```json\n{"format":"stride-plan","version":1,"sessions":[{"date":"2026-11-10","sport":"run","title":"Footing","steps":[{"type":"warmup","durationS":600},{"type":"sprint"}]},{"title":"Sans date"}]}\n```'
    const r = parsePlanText(text)
    expect(r.drafts).toHaveLength(1)
    expect(r.drafts[0].steps).toHaveLength(1)
    expect(r.errors.some((e) => e.includes('sprint'))).toBe(true)
    expect(r.errors.some((e) => e.includes('Séance 2'))).toBe(true)
    expect(parsePlanText('pas de json').drafts).toHaveLength(0)
  })

  it('enriches the activity view with efficiency, impact, readiness and an auto shoe', () => {
    const { view } = setup(['admin', 'fusion'])
    expect(['green', 'orange', 'red']).toContain(view.readiness?.level)
    expect(view.impact?.steps).toBeGreaterThan(1000)
    expect(view.efficiency).not.toBeNull()
    expect(view.notes.shoeAuto === true || view.notes.shoe !== null).toBe(true)
  })

  it('forces readiness levels and degrades without Garmin', () => {
    const { d, run } = setup(['admin', 'ready-red'])
    expect(readinessFor(run.date, d, visibleItems(d)).level).toBe('red')
    const s = setup(['admin', 'strava'])
    expect(readinessFor(s.run.date, s.d, visibleItems(s.d)).level).toBe('unknown')
  })

  it('builds an efficiency trend, a synthetic GPS loop and a calendar', () => {
    const { d, run, view } = setup(['admin', 'fusion'])
    const t = efficiencyTrend(d, 12)
    expect(t.points.length).toBe(t.rollingEfSpeed.length)
    const track = trackFor(view, run)
    expect(track?.points.length).toBeGreaterThan(10)
    expect(track!.bounds!.minLat).toBeLessThan(track!.bounds!.maxLat)
    const today = todayISO()
    const days = calendar(d, addDays(today, -6), today)
    expect(days).toHaveLength(7)
    expect(days.some((x) => x.planned.length > 0)).toBe(true)
  })

  it('answers push-garmin with 409 unsupported by default and 412 for analyze without LLM', async () => {
    const { d } = setup(['admin', 'fusion'])
    const id = d.plan.find((p) => p.sport === 'run')!.id
    const push = await post(`/api/plan/${id}/push-garmin`, {})
    expect(push.status).toBe(409)
    expect(d.plan.find((p) => p.id === id)!.garmin.status).toBe('unsupported')
    setup(['admin', 'nollm'])
    const res = await post('/api/llm/analyze', { kind: 'day', date: todayISO() })
    expect(res.status).toBe(412)
  })
})
