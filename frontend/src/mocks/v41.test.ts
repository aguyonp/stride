import { describe, expect, it } from 'vitest'
import type { HealthStatus, PublicShareView, SearchResponse, ShareLink } from '../api/types'
import { db, resetDb, userData } from './db'
import { visibleItems } from './domain'
import { setFlags } from './flags'
import { matchesActivity, parseQuery } from './search'
import { mockFetch } from './server'
import { focusFromHealth } from './v41'
import { setLang } from '../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const call = (method: string, path: string, body?: unknown) => mockFetch(path, { method, headers: { 'X-Stride-CSRF': '1' }, body: body === undefined ? undefined : JSON.stringify(body) })
function setup() {
  setFlags(['admin'])
  resetDb()
  const me = db().users.find((u) => u.role === 'admin')!
  return userData(me.id)
}

describe('4.1 search parsing', () => {
  it('splits French dates from name words', () => {
    expect(parseQuery('Marathon 14 avril')).toEqual({ text: ['marathon'], day: 14, month: 4, year: null })
    expect(parseQuery('avril 2025')).toEqual({ text: [], day: null, month: 4, year: 2025 })
    expect(parseQuery('14/04/19')).toEqual({ text: [], day: 14, month: 4, year: 2019 })
    expect(parseQuery('2026-11-05')).toEqual({ text: [], day: 5, month: 11, year: 2026 })
    expect(parseQuery('10 km')).toEqual({ text: ['10', 'km'], day: null, month: null, year: null })
  })

  it('matches names accent-insensitively and dates exactly', () => {
    const a = { name: 'Seuil lactique Parc Borély', typeLabel: 'Course', date: '2026-04-14' }
    expect(matchesActivity(a, parseQuery('borely'))).toBe(true)
    expect(matchesActivity(a, parseQuery('14 avril'))).toBe(true)
    expect(matchesActivity(a, parseQuery('15 avril'))).toBe(false)
    expect(matchesActivity(a, parseQuery('seuil mai'))).toBe(false)
  })
})

describe('4.1 mocks', () => {
  it('derives the profile focus from the health status (most recent zone wins)', async () => {
    expect(focusFromHealth({ mode: 'performance', zones: [] })).toMatchObject({ focusArea: 'none', injuredSide: null })
    const data = setup()
    const res = await call('PUT', '/api/health/status', {
      mode: 'tracking',
      zones: [
        { zone: 'achilles', side: 'right', since: '2026-09-01', note: '' },
        { zone: 'iliotibial_band', side: 'left', since: '2026-10-20', note: 'dévers' },
      ],
    })
    expect(res.status).toBe(200)
    const s = (await res.json()) as HealthStatus
    expect(s).toMatchObject({ mode: 'tracking', focusArea: 'other' })
    expect(data.profile).toMatchObject({ focusArea: 'other', injuredSide: 'left' })
    expect((await call('PUT', '/api/health/status', { mode: 'tracking', zones: [] })).status).toBe(400)
  })

  it('finds activities over the whole stored history by name or date', async () => {
    const data = setup()
    const sample = visibleItems(data)[3]
    const res = (await (await call('GET', `/api/search?q=${encodeURIComponent(sample.name)}`)).json()) as SearchResponse
    expect(res.items.some((h) => h.id === sample.id)).toBe(true)
    const byDate = (await (await call('GET', `/api/search?q=${sample.date}`)).json()) as SearchResponse
    expect(byDate.items.every((h) => h.date === sample.date)).toBe(true)
  })

  it('projects Ressenti body entries onto the activity notes (max per activity)', async () => {
    const data = setup()
    const it0 = visibleItems(data)[0]
    const base = { date: it0.date, activityId: it0.id, side: 'left' }
    await call('POST', '/api/bodymap/entries', { ...base, zone: 'knee_pfps', painScore: 3, coldNextDayScore: 5 })
    const second = await (await call('POST', '/api/bodymap/entries', { ...base, zone: 'achilles', painScore: 6, coldNextDayScore: null })).json()
    expect(data.notes[String(it0.id)]).toMatchObject({ painDuring: 6, painNextMorning: 5 })
    await call('DELETE', `/api/bodymap/entries/${second.id}`)
    expect(data.notes[String(it0.id)]).toMatchObject({ painDuring: 3, painNextMorning: 5 })
  })

  it('scopes share links: only allowed sections, period window, anonymised name, editable', async () => {
    setup()
    const created = (await (await call('POST', '/api/share', { label: 'Coach', expiresInDays: 7, periodWeeks: 4, sections: ['shoes', 'activities'], anonymize: true })).json()) as ShareLink
    expect(created).toMatchObject({ periodWeeks: 4, sections: ['activities', 'shoes'], anonymize: true })
    const token = created.url.replace('/share/', '')
    const view = (await (await call('GET', `/api/public/share/${token}`)).json()) as PublicShareView
    expect(view.athlete.displayName).toBe('Athlète')
    expect(Object.keys(view)).not.toContain('bodyMap')
    expect(Object.keys(view)).not.toContain('loadSummary')
    expect(view.shoes?.every((s) => !('id' in s))).toBe(true)
    expect(view.recentSessions?.every((r) => !('painDuring' in r))).toBe(true)

    const patched = (await (await call('PATCH', `/api/share/${created.id}`, { sections: ['health'], anonymize: false })).json()) as ShareLink
    expect(patched).toMatchObject({ sections: ['health'], anonymize: false, periodWeeks: 4 })
    const view2 = (await (await call('GET', `/api/public/share/${token}`)).json()) as PublicShareView
    expect(view2.bodyMap).toBeDefined()
    expect(view2.shoes).toBeUndefined()
  })

  it('starts a low-priority history import job', async () => {
    setup()
    const res = await call('POST', '/api/sync/history', { from: '2025-01-01' })
    expect(res.status).toBe(202)
    expect(await res.json()).toMatchObject({ kind: 'sync.history', queue: 'low', processed: 0 })
    expect((await call('POST', '/api/sync/history', { from: 'hier' })).status).toBe(400)
  })
})
