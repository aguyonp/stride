import { describe, expect, it } from 'vitest'
import type { RecoverySet, RecoveryStatus } from '../api/types'
import { db, resetDb } from './db'
import { setFlags } from './flags'
import { mockFetch } from './server'
import { setLang } from '../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const call = (method: string, path: string, body?: unknown) => mockFetch(path, { method, headers: { 'X-Stride-CSRF': '1' }, body: body === undefined ? undefined : JSON.stringify(body) })

function setup(flags: string[] = ['admin']) {
  setFlags(flags)
  resetDb()
}

const CODE_RE = /^[A-Z2-7]{4}(-[A-Z2-7]{4}){3}$/

describe('4.4 mocks: recovery codes', () => {
  it('serves status, generation, single-use login and regeneration', async () => {
    setup()
    expect((await (await call('GET', '/api/me/recovery')).json()) as RecoveryStatus).toEqual({ remaining: 0 })

    const set = (await (await call('POST', '/api/me/recovery')).json()) as RecoverySet
    expect(set.codes).toHaveLength(10)
    expect(new Set(set.codes).size).toBe(10)
    for (const c of set.codes) expect(CODE_RE.test(c)).toBe(true)
    expect((await (await call('GET', '/api/me/recovery')).json()) as RecoveryStatus).toEqual({ remaining: 10 })

    // Recovery login works from an anonymous session and consumes the code (case and separators are normalized).
    await call('POST', '/api/auth/logout')
    const me = (await (await call('POST', '/api/auth/login/recovery', { code: ` ${set.codes[0].toLowerCase()} ` })).json()) as { id: number }
    expect(me.id).toBe(1)
    expect((await (await call('GET', '/api/me/recovery')).json()) as RecoveryStatus).toEqual({ remaining: 9 })

    // Replay, unknown and malformed codes are 401.
    await call('POST', '/api/auth/logout')
    const replay = await call('POST', '/api/auth/login/recovery', { code: set.codes[0] })
    expect(replay.status).toBe(401)
    expect(((await replay.json()) as { code: string }).code).toBe('UNAUTHORIZED')
    expect((await call('POST', '/api/auth/login/recovery', { code: 'AAAAAAAA-BBBBBBBB-CCCCCCCC-DDDDDDDD' })).status).toBe(401)
    expect((await call('POST', '/api/auth/login/recovery', { code: 'abc' })).status).toBe(401)

    // Re-sign in with a still-valid code (regeneration is a session route), then regenerate.
    expect((await call('POST', '/api/auth/login/recovery', { code: set.codes[2] })).status).toBe(200)
    const fresh = (await (await call('POST', '/api/me/recovery')).json()) as RecoverySet
    expect(fresh.codes).toHaveLength(10)
    expect((await call('POST', '/api/auth/login/recovery', { code: set.codes[1] })).status).toBe(401)
    expect((await call('POST', '/api/auth/login/recovery', { code: fresh.codes[0] })).status).toBe(200)
    expect((await (await call('GET', '/api/me/recovery')).json()) as RecoveryStatus).toEqual({ remaining: 9 })
  })

  it('refuses recovery login for a disabled account, consuming the code like the backend', async () => {
    setup()
    const set = (await (await call('POST', '/api/me/recovery')).json()) as RecoverySet
    const admin = db().users.find((u) => u.role === 'admin')!
    admin.status = 'disabled'
    await call('POST', '/api/auth/logout')
    const res = await call('POST', '/api/auth/login/recovery', { code: set.codes[0] })
    expect(res.status).toBe(403)
    expect(((await res.json()) as { code: string }).code).toBe('FORBIDDEN')
    admin.status = 'active'
    expect((await call('POST', '/api/auth/login/recovery', { code: set.codes[0] })).status).toBe(401)
  })

  it('keeps /api/me/recovery session-only while the login route stays public', async () => {
    setup()
    const set = (await (await call('POST', '/api/me/recovery')).json()) as RecoverySet
    await call('POST', '/api/auth/logout')
    expect((await call('GET', '/api/me/recovery')).status).toBe(401)
    expect((await call('POST', '/api/auth/login/recovery', { code: set.codes[0] })).status).toBe(200)
  })
})
