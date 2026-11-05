import { beforeEach, describe, expect, it } from 'vitest'
import { addDays, todayISO } from '../lib/format'
import { db, resetDb, userData } from './db'
import { visibleItems } from './domain'
import { setFlags } from './flags'
import { v31Of } from './manual'
import { mockFetch } from './server'
import { adaptPlan, records } from './v31'

const call = (method: string, path: string, body?: unknown) => mockFetch(path, { method, headers: { 'X-Stride-CSRF': '1' }, body: body === undefined ? undefined : JSON.stringify(body) })
function setup(flags: string[]) {
  setFlags(flags)
  resetDb()
  const me = db().users.find((u) => u.role === 'admin')!
  return userData(me.id)
}

describe('3.1 mocks', () => {
  beforeEach(() => resetDb())

  it('runs standalone on manual activities and refuses to edit synced ones', async () => {
    const d = setup(['admin', 'standalone'])
    expect(d.integrations.garmin.connected || d.integrations.strava.connected).toBe(false)
    const items = visibleItems(d)
    expect(items.length).toBeGreaterThan(3)
    expect(items.every((i) => i.manual)).toBe(true)
    const res = await call('POST', '/api/activities', { date: todayISO(), startTime: '07:30', sport: 'run', title: 'Matin', distanceM: 8000, durationS: 2880, avgHR: 145 })
    expect(res.status).toBe(201)
    const a = await res.json()
    expect(a.sources).toEqual(['manual'])
    const view = await (await call('GET', `/api/activities/${a.id}`)).json()
    expect(view.sourceLabel).toBe('Manuel')
    expect(view.series.tS).toHaveLength(0)
    expect((await call('PATCH', `/api/activities/${a.id}`, { durationS: 2700 })).status).toBe(200)
    expect((await call('DELETE', `/api/activities/${a.id}`)).status).toBe(204)
    setup(['admin'])
    const synced = visibleItems(userData(1))[0]
    expect((await call('DELETE', `/api/activities/${synced.id}`)).status).toBe(409)
  })

  it('exposes missing Strava scopes and hides kudos', async () => {
    setup(['admin', 'stravascope'])
    const integ = await (await call('GET', '/api/integrations')).json()
    expect(integ.strava.missingScopes).toContain('activity:read_all')
    const list = await (await call('GET', '/api/activities?limit=5')).json()
    expect(list.items.every((x: { kudos: number | null }) => x.kudos === null)).toBe(true)
  })

  it('computes records with a manual override and resets it', async () => {
    const d = setup(['admin'])
    const recs = records(d)
    expect(recs.map((r) => r.key)).toEqual(['1k', '5k', '10k', 'half', 'marathon', 'longest'])
    expect(recs.find((r) => r.key === 'half')?.source).toBe('manual')
    expect(recs.find((r) => r.key === '5k')?.timeS).toBeGreaterThan(0)
    expect((await call('PUT', '/api/records/marathon', { timeS: 14400, date: '2025-04-06' })).status).toBe(200)
    expect(records(d).find((r) => r.key === 'marathon')?.source).toBe('manual')
    expect((await call('DELETE', '/api/records/half')).status).toBe(204)
    expect(records(d).find((r) => r.key === 'half')?.source).not.toBe('manual')
  })

  it('adapts the plan around an illness and applies it atomically', async () => {
    const d = setup(['admin'])
    const today = todayISO()
    const ev = await (await call('POST', '/api/calendar/events', { kind: 'illness', startDate: addDays(today, 2), endDate: addDays(today, 3), severity: 2, note: '' })).json()
    const preview = adaptPlan(d, today, ev.id)
    expect(preview.removeIds.length).toBeGreaterThan(0)
    expect(preview.sessions.every((s) => s.date < addDays(today, 2) || s.date > addDays(today, 3))).toBe(true)
    const cal = await (await call('GET', `/api/calendar?from=${today}&to=${addDays(today, 6)}`)).json()
    expect(cal.days[2].events).toHaveLength(1)
    const before = d.plan.length
    const applied = await (await call('POST', '/api/plan/replace', { removeIds: preview.removeIds, sessions: preview.sessions })).json()
    expect(applied).toHaveLength(preview.sessions.length)
    expect(userData(1).plan.length).toBe(before - preview.removeIds.length + preview.sessions.length)
  })

  it('shifts a session and generates phases until the goal', async () => {
    const d = setup(['admin', 'longgoal'])
    expect(v31Of(d).phases.length).toBe(4)
    const p = d.plan.find((x) => x.date > todayISO())!
    const was = p.date
    const moved = await (await call('POST', `/api/plan/${p.id}/shift`, { days: 1 })).json()
    expect(moved.date).toBe(addDays(was, 1))
    const gen = await (await call('POST', '/api/plan/generate', { mode: 'llm', horizon: 'goal', startDate: addDays(todayISO(), 1), goalId: d.goals.find((g) => g.kind === 'race')!.id })).json()
    expect(gen.phases.length).toBe(4)
    expect(gen.drafts.length).toBeGreaterThan(8)
    const prompt = await (await call('POST', '/api/plan/generate', { mode: 'prompt', horizon: 'goal', startDate: addDays(todayISO(), 1) })).json()
    expect(prompt.prompt).toContain('"version":2')
    const bulk = await call('POST', '/api/plan/bulk', { sessions: gen.drafts.slice(0, 2), phases: gen.phases })
    expect(bulk.status).toBe(201)
    expect(v31Of(userData(1)).phases.every((x) => x.id > 0)).toBe(true)
  })
})
