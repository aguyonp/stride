// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { db, resetDb, userData } from '../../mocks/db'
import { setFlags } from '../../mocks/flags'
import { mockFetch } from '../../mocks/server'
import { installDomPolyfills, mountWithProviders, waitFor, type MountedApp } from '../../test/domHarness'
import { setLang } from '../../i18n/lang'
import { IntegrationsSection } from './IntegrationsSection'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const text = () => document.body.textContent ?? ''

/**
 * The Garmin bridge is an independent path (§23, #147): it signs in with the user's own account, so
 * its setup must be reachable even when the Personal API key is not configured. It used to live
 * inside the « connected » branch and was invisible otherwise.
 */
describe('IntegrationsSection — pont Garmin', () => {
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

  it('offers the bridge setup even without a Garmin API key', async () => {
    const data = userData(1)
    Object.assign(data.integrations.garmin, { connected: false, status: 'disconnected', keyHint: null, tokenExpiresAt: null, lastError: null })
    db().data['1'] = data
    db().garminBridgeEnabled = true

    mounted = await mountWithProviders(<IntegrationsSection />)
    // …and the bridge is still offered, with its warning and its setup button.
    await waitFor(() => expect(text()).toContain('Activer le pont'))
    expect(text()).toContain('Envoi de séances (expérimental)')
    expect(text()).toContain('non officiel, instable')
  })

  it('hides the bridge when the administrator has not opened it', async () => {
    db().garminBridgeEnabled = false
    mounted = await mountWithProviders(<IntegrationsSection />)
    // Let both queries answer before asserting absence: the section first renders its skeleton.
    await waitFor(() => expect(text()).toContain('Garmin Connect'))
    await new Promise((r) => setTimeout(r, 300))
    expect(text()).not.toContain('Envoi de séances')
  })
})
