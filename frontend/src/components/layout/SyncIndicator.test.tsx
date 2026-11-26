// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { db, resetDb, userData } from '../../mocks/db'
import { setFlags } from '../../mocks/flags'
import { mockFetch } from '../../mocks/server'
import { clickEl, installDomPolyfills, mountWithProviders, waitFor, type MountedApp } from '../../test/domHarness'
import { setLang } from '../../i18n/lang'
import { SyncIndicator } from './SyncIndicator'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const text = () => document.body.textContent ?? ''

/**
 * The header used to drop the sync button when no source was connected. A toolbar that loses a
 * button looks like a regression (field report), so the indicator stays — and says why there is
 * nothing to sync yet.
 */
describe('SyncIndicator', () => {
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

  it('stays in the toolbar with nothing connected, and says why', async () => {
    const data = userData(1)
    Object.assign(data.integrations.garmin, { connected: false, status: 'disconnected' })
    Object.assign(data.integrations.strava, { connected: false, status: 'disconnected' })
    db().data['1'] = data

    mounted = await mountWithProviders(<SyncIndicator />)
    const trigger = document.querySelector('button[aria-label^="État de la synchronisation"]')
    expect(trigger).not.toBeNull()

    await clickEl(trigger as HTMLElement)
    // The popover explains the state instead of offering a sync that could not read anything.
    await waitFor(() => expect(text()).toContain('Aucune source connectée'))
    expect(text()).toContain('Configurer une intégration')
    expect(document.body.textContent).not.toContain('Synchroniser maintenant')
  })

  it('offers the sync as soon as a source answers', async () => {
    const data = userData(1)
    Object.assign(data.integrations.garmin, { connected: true, status: 'connected' })
    db().data['1'] = data

    mounted = await mountWithProviders(<SyncIndicator />)
    const trigger = document.querySelector('button[aria-label^="État de la synchronisation"]')
    await waitFor(() => expect(trigger?.getAttribute('aria-label')).toMatch(/synchronisé|jamais|en cours/i))
    await clickEl(trigger as HTMLElement)
    await waitFor(() => expect(text()).toContain('Synchroniser maintenant'))
    expect(text()).not.toContain('Aucune source connectée')
  })
})
