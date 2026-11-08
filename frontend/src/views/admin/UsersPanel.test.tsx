// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminUser, Me } from '../../api/types'
import { clickEl, mountWithProviders, setInputValue, waitFor, type MountedApp } from '../../test/domHarness'
import { isLastActiveAdmin } from './adminUtils'
import { UsersPanel } from './UsersPanel'
import { setLang } from '../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const me: Me = { id: 1, displayName: 'Alex Démo', role: 'admin', createdAt: '2026-08-01T10:00:00Z', onboardingCompleted: true }

function user(p: Partial<AdminUser> & Pick<AdminUser, 'id' | 'displayName'>): AdminUser {
  return {
    role: 'user',
    status: 'active',
    createdAt: '2026-09-01T10:00:00Z',
    lastLoginAt: '2026-11-06T08:00:00Z',
    passkeys: 1,
    lastSeenAt: '2026-11-06T09:00:00Z',
    sessions: 1,
    activities: 0,
    storageBytes: 0,
    integrations: [],
    ...p,
  }
}

const rowsText = () => Array.from(document.querySelectorAll('tbody th[scope="row"]')).map((th) => th.textContent ?? '')
const button = (text: string, root: ParentNode = document) => Array.from(root.querySelectorAll('button')).find((b) => b.textContent?.trim().startsWith(text)) ?? null

describe('UsersPanel (centre de contrôle)', () => {
  let mounted: MountedApp | null = null
  let users: AdminUser[] = []
  let calls: { method: string; url: string; body: unknown }[] = []

  beforeEach(() => {
    calls = []
    users = [
      user({ id: 1, displayName: 'Alex Démo', role: 'admin', sessions: 2, activities: 412, storageBytes: 80_000_000, integrations: ['garmin', 'strava', 'llm'] }),
      user({ id: 2, displayName: 'Sam Testeur', sessions: 2, activities: 120, storageBytes: 20_000_000, integrations: ['garmin'] }),
      user({ id: 3, displayName: 'Lou Essai', status: 'disabled', sessions: 0, lastSeenAt: null }),
    ]
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        const method = (init?.method ?? 'GET').toUpperCase()
        const body = typeof init?.body === 'string' ? JSON.parse(init.body) : undefined
        calls.push({ method, url, body })
        const ok = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } })
        if (url === '/api/admin/users') return ok(users)
        const sessions = /^\/api\/admin\/users\/(\d+)\/sessions$/.exec(url)
        if (sessions && method === 'DELETE') {
          const u = users.find((x) => x.id === Number(sessions[1]))!
          const revoked = u.sessions
          u.sessions = 0
          return ok({ revoked })
        }
        const patch = /^\/api\/admin\/users\/(\d+)$/.exec(url)
        if (patch && method === 'PATCH') {
          const u = users.find((x) => x.id === Number(patch[1]))!
          Object.assign(u, body)
          if (body.status === 'disabled') u.sessions = 0
          return ok(u)
        }
        return ok({})
      }),
    )
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
  })

  it('searches, filters and sorts accounts with their footprint', async () => {
    mounted = await mountWithProviders(<UsersPanel me={me} />, '/admin?tab=users')
    await waitFor(() => expect(rowsText()).toHaveLength(3))
    // Default sort: most recently seen first, never-seen last.
    expect(rowsText()[2]).toContain('Lou Essai')
    expect(document.body.textContent).toContain('76,3 Mo')

    await setInputValue(document.querySelector<HTMLInputElement>('input[type="search"]')!, 'testeur')
    await waitFor(() => expect(rowsText()).toEqual([expect.stringContaining('Sam Testeur')]))

    await setInputValue(document.querySelector<HTMLInputElement>('input[type="search"]')!, '')
    await clickEl(button('Désactivés'))
    await waitFor(() => expect(rowsText()).toEqual([expect.stringContaining('Lou Essai')]))
    expect(button('Désactivés')?.getAttribute('aria-pressed')).toBe('true')
  })

  it('guards the own account and confirms disable and session revocation', async () => {
    mounted = await mountWithProviders(<UsersPanel me={me} />, '/admin?tab=users')
    await waitFor(() => expect(rowsText()).toHaveLength(3))

    // Own account: role and status locked, no destructive action offered.
    expect(document.querySelector<HTMLSelectElement>('select[aria-label="Rôle de Alex Démo"]')?.disabled).toBe(true)
    expect(document.querySelector<HTMLButtonElement>('[aria-label="Compte de Alex Démo actif"]')?.disabled).toBe(true)
    await clickEl(document.querySelector('[aria-label="Actions pour Alex Démo"]'))
    expect(button('Supprimer', document.querySelector('[role="menu"]')!)).toBeNull()
    expect(button('Fermer les sessions', document.querySelector('[role="menu"]')!)).toBeNull()
    await clickEl(document.querySelector('[aria-label="Actions pour Alex Démo"]'))

    // Session revocation goes through a confirmation, then DELETE …/sessions.
    await clickEl(document.querySelector('[aria-label="Actions pour Sam Testeur"]'))
    await clickEl(button('Fermer les sessions (2)', document.querySelector('[role="menu"]')!))
    await waitFor(() => expect(document.querySelector('dialog[open]')?.textContent).toContain('Fermer les sessions de Sam Testeur ?'))
    expect(calls.some((c) => c.method === 'DELETE')).toBe(false)
    await clickEl(button('Fermer les sessions', document.querySelector('dialog[open]')!))
    await waitFor(() => expect(calls).toContainEqual({ method: 'DELETE', url: '/api/admin/users/2/sessions', body: undefined }))

    // Disabling asks first, then PATCHes the status.
    await clickEl(document.querySelector('[aria-label="Compte de Sam Testeur actif"]'))
    await waitFor(() => expect(document.querySelector('dialog[open]')?.textContent).toContain('Désactiver Sam Testeur ?'))
    expect(calls.some((c) => c.method === 'PATCH')).toBe(false)
    await clickEl(button('Désactiver', document.querySelector('dialog[open]')!))
    await waitFor(() => expect(calls).toContainEqual({ method: 'PATCH', url: '/api/admin/users/2', body: { status: 'disabled' } }))
  })

  it('never lets the last active admin be demoted', () => {
    const admin = user({ id: 9, displayName: 'Root', role: 'admin' })
    expect(isLastActiveAdmin([admin, user({ id: 2, displayName: 'Sam' })], admin)).toBe(true)
    expect(isLastActiveAdmin([admin, user({ id: 2, displayName: 'Ana', role: 'admin', status: 'disabled' })], admin)).toBe(true)
    expect(isLastActiveAdmin([admin, user({ id: 2, displayName: 'Ana', role: 'admin' })], admin)).toBe(false)
  })
})
