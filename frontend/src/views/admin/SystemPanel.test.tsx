// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clickEl, mountWithProviders, setInputValue, waitFor, type MountedApp } from '../../test/domHarness'
import { setLang } from '../../i18n/lang'
import { SystemPanel } from './SystemPanel'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

/**
 * Admin › Instance (§21, §22, §23): the settings of the installation itself. They used to live in
 * the invitations tab, which had nothing to do with them.
 */
describe('SystemPanel — réglages de l’instance', () => {
  let mounted: MountedApp | null = null
  let putBody: Record<string, unknown> | null = null
  let stored = {
    registrationMode: 'invite',
    inboxRetentionDays: 10,
    inboxRetentionMax: 50,
    updateCheckEnabled: true,
    garminBridgeEnabled: false,
  }

  const numInputs = () => Array.from(document.querySelectorAll('input[type="number"]')) as HTMLInputElement[]
  const switches = () => Array.from(document.querySelectorAll('button[role="switch"]')) as HTMLButtonElement[]
  const saveBtn = () => [...document.querySelectorAll('button[type="submit"]')].find((b) => b.textContent?.includes('Enregistrer')) as HTMLButtonElement | undefined

  beforeEach(() => {
    putBody = null
    stored = { registrationMode: 'invite', inboxRetentionDays: 10, inboxRetentionMax: 50, updateCheckEnabled: true, garminBridgeEnabled: false }
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        const ok = (v: unknown) => new Response(JSON.stringify(v), { headers: { 'Content-Type': 'application/json' } })
        if (url === '/api/admin/settings' && (init?.method ?? 'GET') === 'GET') return ok(stored)
        if (url === '/api/admin/settings' && init?.method === 'PUT') {
          putBody = JSON.parse(String(init.body)) as Record<string, unknown>
          // The server clamps and echoes the stored values; an absent field keeps its value.
          stored = {
            registrationMode: String(putBody.registrationMode ?? stored.registrationMode),
            inboxRetentionDays: Math.min(365, Math.max(1, Number(putBody.inboxRetentionDays ?? stored.inboxRetentionDays))),
            inboxRetentionMax: Math.min(200, Math.max(1, Number(putBody.inboxRetentionMax ?? stored.inboxRetentionMax))),
            updateCheckEnabled: putBody.updateCheckEnabled === undefined ? stored.updateCheckEnabled : !!putBody.updateCheckEnabled,
            garminBridgeEnabled: putBody.garminBridgeEnabled === undefined ? stored.garminBridgeEnabled : !!putBody.garminBridgeEnabled,
          }
          return ok(stored)
        }
        return new Response(JSON.stringify({}), { status: 404, headers: { 'Content-Type': 'application/json' } })
      }),
    )
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
  })

  it('shows the three instance settings, with their defaults', async () => {
    mounted = await mountWithProviders(<SystemPanel />)
    await waitFor(() => expect(numInputs().map((i) => i.value)).toEqual(['10', '50']))
    const text = document.body.textContent ?? ''
    // Retention, release check and the experimental bridge: no invitation in sight.
    expect(text).toContain('Centre de notifications')
    expect(text).toContain('Mises à jour')
    expect(text).toContain('Pont Garmin (expérimental)')
    expect(text).not.toContain('Invitations')
    // The update check is on, the bridge is off.
    expect(switches().map((b) => b.getAttribute('aria-checked'))).toEqual(['true', 'false'])
  })

  it('saves the retention and refuses out-of-range values before the call', async () => {
    mounted = await mountWithProviders(<SystemPanel />)
    await waitFor(() => expect(numInputs().map((i) => i.value)).toEqual(['10', '50']))
    expect(saveBtn()?.disabled).toBe(true) // untouched

    await setInputValue(numInputs()[0], '0')
    await waitFor(() => expect(document.body.textContent).toContain('Entre 1 et 365 jours.'))
    expect(saveBtn()?.disabled).toBe(true)
    expect(putBody).toBeNull()

    await setInputValue(numInputs()[0], '3')
    await setInputValue(numInputs()[1], '80')
    await waitFor(() => expect(saveBtn()?.disabled).toBe(false))
    await clickEl(saveBtn() ?? null)
    await waitFor(() => expect(putBody).toMatchObject({ inboxRetentionDays: 3, inboxRetentionMax: 80 }))
    await waitFor(() => expect(numInputs().map((i) => i.value)).toEqual(['3', '80']))
  })

  it('toggles the release check and the bridge without touching the rest', async () => {
    mounted = await mountWithProviders(<SystemPanel />)
    await waitFor(() => expect(switches().length).toBe(2))

    await clickEl(switches()[0])
    await waitFor(() => expect(putBody).toMatchObject({ updateCheckEnabled: false }))
    await waitFor(() => expect(switches()[0].getAttribute('aria-checked')).toBe('false'))

    await clickEl(switches()[1])
    await waitFor(() => expect(putBody).toMatchObject({ garminBridgeEnabled: true }))
    await waitFor(() => expect(switches()[1].getAttribute('aria-checked')).toBe('true'))
    // The bridge on is announced with what it needs from the operator.
    expect(document.body.textContent).toContain('Les utilisateurs peuvent activer le pont')
    // The release check stayed off: each switch sends only its own field.
    expect(switches()[0].getAttribute('aria-checked')).toBe('false')
  })
})
