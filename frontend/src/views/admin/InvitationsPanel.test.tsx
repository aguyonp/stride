// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clickEl, mountWithProviders, setInputValue, waitFor, type MountedApp } from '../../test/domHarness'
import { setLang } from '../../i18n/lang'
import { InvitationsPanel } from './InvitationsPanel'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

/** The retention card of the notification centre (issue #142), inside the invitations panel. */
describe('InvitationsPanel — centre de notifications', () => {
  let mounted: MountedApp | null = null
  let putBody: Record<string, unknown> | null = null
  let stored = { registrationMode: 'invite', inboxRetentionDays: 10, inboxRetentionMax: 50 }

  // The invitation card also has a number field: the retention ones are the first two.
  const numInputs = () => (Array.from(document.querySelectorAll('input[type="number"]')) as HTMLInputElement[]).slice(0, 2)
  const cardSave = () =>
    [...document.querySelectorAll('button[type="submit"]')].find((b) => b.textContent?.includes('Enregistrer')) as HTMLButtonElement | undefined

  beforeEach(() => {
    putBody = null
    stored = { registrationMode: 'invite', inboxRetentionDays: 10, inboxRetentionMax: 50 }
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        const ok = (v: unknown) => new Response(JSON.stringify(v), { headers: { 'Content-Type': 'application/json' } })
        if (url === '/api/admin/settings' && (init?.method ?? 'GET') === 'GET') return ok(stored)
        if (url === '/api/admin/settings' && init?.method === 'PUT') {
          putBody = JSON.parse(String(init.body)) as Record<string, unknown>
          // The server clamps and echoes the stored values.
          stored = {
            registrationMode: String(putBody.registrationMode ?? stored.registrationMode),
            inboxRetentionDays: Math.min(365, Math.max(1, Number(putBody.inboxRetentionDays ?? stored.inboxRetentionDays))),
            inboxRetentionMax: Math.min(200, Math.max(1, Number(putBody.inboxRetentionMax ?? stored.inboxRetentionMax))),
          }
          return ok(stored)
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

  it('shows the stored retention and saves the edited values', async () => {
    mounted = await mountWithProviders(<InvitationsPanel />)
    await waitFor(() => expect(numInputs().map((i) => i.value)).toEqual(['10', '50']))
    expect(cardSave()?.disabled).toBe(true) // untouched: nothing to save

    await setInputValue(numInputs()[0], '3')
    await setInputValue(numInputs()[1], '80')
    await waitFor(() => expect(cardSave()?.disabled).toBe(false))
    await clickEl(cardSave() ?? null)
    await waitFor(() => expect(putBody).not.toBeNull())
    expect(putBody).toMatchObject({ inboxRetentionDays: 3, inboxRetentionMax: 80 })
    await waitFor(() => expect(numInputs().map((i) => i.value)).toEqual(['3', '80']))
    expect(cardSave()?.disabled).toBe(true)
  })

  it('refuses out-of-range values before the call', async () => {
    mounted = await mountWithProviders(<InvitationsPanel />)
    await waitFor(() => expect(numInputs().map((i) => i.value)).toEqual(['10', '50']))

    await setInputValue(numInputs()[0], '0')
    await waitFor(() => expect(document.body.textContent).toContain('Entre 1 et 365 jours.'))
    expect(cardSave()?.disabled).toBe(true)

    await setInputValue(numInputs()[0], '30')
    await setInputValue(numInputs()[1], '500')
    await waitFor(() => expect(document.body.textContent).toContain('Entre 1 et 200 entrées.'))
    expect(cardSave()?.disabled).toBe(true)
    expect(putBody).toBeNull()
  })
})
