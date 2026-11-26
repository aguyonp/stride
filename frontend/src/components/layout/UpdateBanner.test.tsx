// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { db, resetDb, userData } from '../../mocks/db'
import { setFlags } from '../../mocks/flags'
import { mockFetch } from '../../mocks/server'
import { clickEl, installDomPolyfills, mountWithProviders, waitFor, type MountedApp } from '../../test/domHarness'
import { setLang } from '../../i18n/lang'
import type { Me } from '../../api/types'
import { UpdateBanner } from './UpdateBanner'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const admin: Me = { id: 1, displayName: 'Antoine', role: 'admin', createdAt: '2026-11-01T00:00:00Z', onboardingCompleted: true }
const member: Me = { ...admin, id: 2, role: 'user' }

describe('UpdateBanner (nouvelle version)', () => {
  let mounted: MountedApp | null = null
  let opened: string[] = []
  let calls: string[] = []

  beforeEach(() => {
    opened = []
    calls = []
    installDomPolyfills()
    setFlags([])
    resetDb()
    sessionStorage.clear()
    vi.stubGlobal('open', vi.fn((u: string) => { opened.push(String(u)); return null }))
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string, init?: RequestInit) => {
        calls.push(`${init?.method ?? 'GET'} ${input}`)
        return mockFetch(String(input), init)
      }),
    )
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
    resetDb()
  })

  const banner = () => document.body.textContent ?? ''

  it('announces the release to an administrator and opens GitHub', async () => {
    mounted = await mountWithProviders(<UpdateBanner me={admin} />)
    await waitFor(() => expect(banner()).toContain('Nouvelle version disponible'))
    expect(banner()).toContain('Stride 4.8.0 est publiée (tu es en 4.7.0)')

    await clickEl([...document.querySelectorAll('button')].find((b) => b.textContent?.includes('Voir sur GitHub')) ?? null)
    expect(opened).toEqual(['https://github.com/aguyonp/stride/releases/tag/4.8.0'])
    // Opening the release reads the notice: the entry stays in the history, marked read.
    await waitFor(() => expect(calls.some((c) => c.startsWith('POST /api/inbox/read'))).toBe(true))
    await waitFor(() => expect(userData(1).inbox?.every((e) => e.kind !== 'update' || e.readAt)).toBe(true))
  })

  it('« Plus tard » hides it for the visit without reading the notice', async () => {
    mounted = await mountWithProviders(<UpdateBanner me={admin} />)
    await waitFor(() => expect(banner()).toContain('Nouvelle version disponible'))
    await clickEl([...document.querySelectorAll('button')].find((b) => b.textContent?.includes('Plus tard')) ?? null)
    await waitFor(() => expect(banner()).not.toContain('Nouvelle version disponible'))
    expect(sessionStorage.getItem('stride.update.later')).toBe('4.8.0')
    // The notice is still unread in the centre.
    expect(userData(1).inbox?.filter((e) => e.kind === 'update' && !e.readAt).length).toBe(1)
    expect(calls.some((c) => c.startsWith('POST /api/inbox/read'))).toBe(false)
  })

  it('stays hidden once the notice is read or the release is up to date', async () => {
    await mounted?.unmount()
    mounted = await mountWithProviders(<UpdateBanner me={admin} />)
    await waitFor(() => expect(banner()).toContain('Nouvelle version disponible'))
    // Reading the entry from the centre hides the banner on the next render.
    const d = userData(1)
    d.inbox = (d.inbox ?? []).map((e) => (e.kind === 'update' ? { ...e, readAt: new Date().toISOString() } : e))
    db().data['1'] = d
    await mounted.unmount()
    mounted = await mountWithProviders(<UpdateBanner me={admin} />)
    await waitFor(() => expect(calls.filter((c) => c === 'GET /api/updates').length).toBe(2))
    expect(banner()).not.toContain('Nouvelle version disponible')

    db().mockRelease = null
    await mounted.unmount()
    mounted = await mountWithProviders(<UpdateBanner me={admin} />)
    await waitFor(() => expect(calls.filter((c) => c === 'GET /api/updates').length).toBe(3))
    expect(banner()).not.toContain('Nouvelle version disponible')
  })

  it('never shows and never calls the route for a regular user', async () => {
    mounted = await mountWithProviders(<UpdateBanner me={member} />)
    await new Promise((r) => setTimeout(r, 120))
    expect(banner()).not.toContain('Nouvelle version disponible')
    expect(calls).toEqual([])
  })
})
