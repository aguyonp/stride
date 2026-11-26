// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { db, resetDb, userData } from '../../mocks/db'
import { setFlags } from '../../mocks/flags'
import { mockFetch } from '../../mocks/server'
import { clickEl, installDomPolyfills, mountWithProviders, waitFor, type MountedApp } from '../../test/domHarness'
import { setLang } from '../../i18n/lang'
import { NotificationBell } from './NotificationBell'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const BELL = 'button[aria-label^="Notifications"]'

describe('NotificationBell (centre de notifications)', () => {
  let mounted: MountedApp | null = null

  beforeEach(() => {
    installDomPolyfills()
    setFlags([])
    resetDb()
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string, init?: RequestInit) => mockFetch(String(input), init)),
    )
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
    resetDb()
  })

  const openPanel = async () => {
    mounted = await mountWithProviders(<NotificationBell />)
    await waitFor(() => expect(document.querySelector(BELL)?.getAttribute('aria-label')).toBe('Notifications, 2 non lues'))
    await clickEl(document.querySelector(BELL))
    await waitFor(() => expect(document.body.textContent).toContain('Notifications'))
    return mounted
  }

  it('badges the unread count and lists the history grouped by day', async () => {
    await openPanel()
    const text = document.body.textContent ?? ''
    // Two unread entries: the biomechanical alert (server text) and the portal directive (localized
    // from its code); the old remark is read and still listed.
    expect(text).toContain('Cadence basse sur la dernière sortie')
    expect(text).toContain('Consigne kiné/coach mise à jour')
    expect(text).toContain('Nouvelle remarque sur une séance')
    expect(text).toContain('Aujourd’hui')
    expect(text).toContain('Hier')
    // The share entries show the physio label; the remark also names its session.
    expect(text).toContain('Kiné Martin')
    // Unread entries are marked, the read one is not.
    expect(document.querySelectorAll('[aria-label="Non lue"]').length).toBe(2)
  })

  it('marks an entry as read and opens its session', async () => {
    await openPanel()
    const remark = [...document.querySelectorAll('button')].find((b) => b.textContent?.includes('Nouvelle remarque sur une séance'))
    await clickEl(remark ?? null)
    // The remark already was read: only the navigation happens.
    await waitFor(() => expect(window.location.pathname).toMatch(/^\/activities\/\d+$/))

    await clickEl(document.querySelector(BELL))
    const directive = [...document.querySelectorAll('button')].find((b) => b.textContent?.includes('Consigne kiné/coach mise à jour'))
    await clickEl(directive ?? null)
    // A directive has no session: it leads to the health page, the entry is now read.
    await waitFor(() => expect(window.location.pathname).toBe('/health'))
    await waitFor(() => expect(userData(1).inbox?.filter((e) => !e.readAt).length).toBe(1))
  })

  it('empties the badge with « Tout marquer comme lu »', async () => {
    await openPanel()
    const markAll = [...document.querySelectorAll('button')].find((b) => b.textContent?.includes('Tout marquer comme lu'))
    await clickEl(markAll ?? null)
    await waitFor(() => expect(userData(1).inbox?.every((e) => e.readAt)).toBe(true))
    await waitFor(() => expect(document.querySelector(BELL)?.getAttribute('aria-label')).toBe('Notifications'))
    // The history stays visible.
    expect(document.body.textContent).toContain('Cadence basse sur la dernière sortie')
  })

  it('shows an explicit empty state', async () => {
    const d = userData(1)
    d.inbox = []
    db().data['1'] = d
    mounted = await mountWithProviders(<NotificationBell />)
    await waitFor(() => expect(document.querySelector(BELL)?.getAttribute('aria-label')).toBe('Notifications'))
    await clickEl(document.querySelector(BELL))
    await waitFor(() => expect(document.body.textContent).toContain('Aucune notification'))
  })
})
