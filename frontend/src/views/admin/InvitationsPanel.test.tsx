// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clickEl, mountWithProviders, waitFor, type MountedApp } from '../../test/domHarness'
import { setLang } from '../../i18n/lang'
import { InvitationsPanel } from './InvitationsPanel'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

/**
 * Admin › Invitations: this tab is about *who may join*. The instance settings — notification
 * retention, release check, experimental Garmin bridge — live in Admin › Instance (§21–§23) and
 * must not come back here.
 */
describe('InvitationsPanel', () => {
  let mounted: MountedApp | null = null
  let putBody: Record<string, unknown> | null = null

  beforeEach(() => {
    putBody = null
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        const ok = (v: unknown) => new Response(JSON.stringify(v), { headers: { 'Content-Type': 'application/json' } })
        if (url === '/api/admin/settings' && (init?.method ?? 'GET') === 'GET') {
          return ok({ registrationMode: 'invite', inboxRetentionDays: 10, inboxRetentionMax: 50, updateCheckEnabled: true, garminBridgeEnabled: false })
        }
        if (url === '/api/admin/settings' && init?.method === 'PUT') {
          putBody = JSON.parse(String(init.body)) as Record<string, unknown>
          return ok({ registrationMode: putBody.registrationMode ?? 'invite', inboxRetentionDays: 10, inboxRetentionMax: 50, updateCheckEnabled: true, garminBridgeEnabled: false })
        }
        if (url === '/api/admin/invitations') return ok([])
        return new Response(JSON.stringify({}), { status: 404, headers: { 'Content-Type': 'application/json' } })
      }),
    )
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
  })

  it('keeps registration and invitations, and nothing of the instance settings', async () => {
    mounted = await mountWithProviders(<InvitationsPanel />)
    await waitFor(() => expect(document.body.textContent).toContain('Inscriptions'))
    const text = document.body.textContent ?? ''
    // What belongs here.
    expect(text).toContain('Qui peut créer un compte')
    expect(text).toContain('Nouvelle invitation')
    // What was moved to Admin › Instance.
    expect(text).not.toContain('Centre de notifications')
    expect(text).not.toContain('Mises à jour')
    expect(text).not.toContain('Pont Garmin')
    expect(document.querySelector('button[role="switch"]')).toBeNull()
  })

  it('still switches the registration mode, without the instance fields', async () => {
    mounted = await mountWithProviders(<InvitationsPanel />)
    await waitFor(() => expect(document.querySelector('[role="radiogroup"]')).not.toBeNull())
    const open = [...document.querySelectorAll('[role="radiogroup"] button')].find((b) => b.textContent?.trim() === 'Ouvertes')
    await clickEl(open ?? null)
    await waitFor(() => expect(putBody).toMatchObject({ registrationMode: 'open' }))
    // Only what this card owns: the other settings keep their value server-side.
    expect(putBody).not.toHaveProperty('garminBridgeEnabled')
    expect(putBody).not.toHaveProperty('inboxRetentionDays')
  })
})
