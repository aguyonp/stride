import { describe, expect, it } from 'vitest'
import type { BackupList, MorphPreview, PlannedSession, PublicShareView, ShareLink, ShareNote } from '../api/types'
import { addDays, todayISO } from '../lib/format'
import { db, resetDb, userData } from './db'
import { setFlags } from './flags'
import { mockFetch } from './server'
import { setLang } from '../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const call = (method: string, path: string, body?: unknown) => mockFetch(path, { method, headers: { 'X-Stride-CSRF': '1' }, body: body === undefined ? undefined : JSON.stringify(body) })
function setup(flags: string[] = ['admin']) {
  setFlags(flags)
  resetDb()
  const me = db().users.find((u) => u.role === 'admin')!
  return userData(me.id)
}

describe('4.2 mocks: share portal', () => {
  it('serves permissions, directive, remarks and calendar on the demo link', async () => {
    setup()
    const v = (await (await call('GET', '/api/public/share/demo-kine')).json()) as PublicShareView
    expect(v.permissions).toEqual({ comments: true, planning: true, activityDetails: true })
    expect(v.directive?.restrictions?.noIntervals).toBe(true)
    expect(v.recentSessions?.some((s) => s.remark)).toBe(true)
    expect(v.calendar?.planned.some((p) => p.prescribedBy?.label === 'Kiné du sport')).toBe(true)
  })

  it('refuses writes without the permission and edits only what the link prescribed', async () => {
    setup()
    const created = (await (await call('POST', '/api/share', { label: 'Coach club', expiresInDays: 7, periodWeeks: 12, sections: ['activities'], anonymize: true, permissions: { comments: false, planning: false, activityDetails: false } })).json()) as ShareLink
    const token = created.url.split('/').pop()!
    const ro = (await (await call('GET', `/api/public/share/${token}`)).json()) as PublicShareView
    expect(ro.calendar).toBeUndefined()
    const denied = await call('PUT', `/api/public/share/${token}/directive`, { body: 'Repos.' })
    expect(denied.status).toBe(403)
    expect(((await denied.json()) as { code: string }).code).toBe('SHARE_FORBIDDEN')

    const plan = (await (await call('POST', '/api/public/share/demo-kine/plan', { date: addDays(todayISO(), 3), sport: 'run', title: 'Footing 30′', targetDurationS: 1800, targetDistanceM: null, targetPaceMinSecPerKm: null, targetPaceMaxSecPerKm: null, targetCadenceSpm: 176, instructions: '' })).json()) as PlannedSession
    expect(plan.source).toBe('share')
    expect((await call('PATCH', `/api/public/share/demo-kine/plan/${plan.id}`, { title: 'Footing 25′' })).status).toBe(200)
    const others = (await (await call('GET', `/api/plan?from=${todayISO()}&to=${addDays(todayISO(), 9)}`)).json()) as PlannedSession[]
    const notMine = others.find((p) => !p.prescribedBy)!
    expect((await call('DELETE', `/api/public/share/demo-kine/plan/${notMine.id}`)).status).toBe(403)

    const remark = await call('PUT', `/api/public/share/demo-kine/activities/${ro.recentSessions![0].id}/remark`, { authorRole: 'coach', body: '  Bien géré.  ' })
    expect(((await remark.json()) as ShareNote).body).toBe('Bien géré.')
    const notes = (await (await call('GET', '/api/share/notes?active=1')).json()) as ShareNote[]
    expect(notes.filter((n) => n.kind === 'directive')).toHaveLength(1)
    expect(notes.some((n) => n.body === 'Bien géré.')).toBe(true)
  })
})

describe('4.2 mocks: morphing and backups', () => {
  it('previews constraints for today, applies and reverts', async () => {
    const data = setup()
    const today = data.plan.find((p) => p.date === todayISO())!
    const pv = (await (await call('GET', `/api/plan/${today.id}/morph`)).json()) as MorphPreview
    expect(pv.constraints.map((c) => c.kind)).toContain('directive')
    expect(pv.proposal).not.toBeNull()
    expect(pv.changes.length).toBeGreaterThan(0)
    const applied = (await (await call('POST', `/api/plan/${today.id}/morph/apply`, { proposal: pv.proposal })).json()) as PlannedSession
    expect(applied.morph?.original.title).toBe(today.title)
    const reverted = (await (await call('POST', `/api/plan/${today.id}/morph/revert`)).json()) as PlannedSession
    expect(reverted.morph).toBeNull()
    const later = data.plan.find((p) => p.date > addDays(todayISO(), 1))!
    expect(((await (await call('GET', `/api/plan/${later.id}/morph`)).json()) as MorphPreview).proposal).toBeNull()
  })

  it('lists backups without the S3 secret and keeps it when saved empty', async () => {
    setup()
    const list = (await (await call('GET', '/api/admin/backups')).json()) as BackupList
    expect(list.items.length).toBeGreaterThan(0)
    expect(list.s3.secretKeySet).toBe(true)
    expect(JSON.stringify(list)).not.toContain('mock-secret')
    const saved = await call('PUT', '/api/admin/backups/s3', { ...list.s3, secretKey: '', retentionDays: 14 })
    expect(((await saved.json()) as BackupList['s3']).secretKeySet).toBe(true)
    expect((await call('DELETE', '/api/admin/backups/..%2Fetc%2Fpasswd')).status).toBe(400)
  })
})
