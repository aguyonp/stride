// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { McpUsage } from '../../api/types'
import { clickEl, mountWithProviders, setInputValue, waitFor, type MountedApp } from '../../test/domHarness'
import { setLang } from '../../i18n/lang'
import { McpAdminPanel } from './McpAdminPanel'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const usage: McpUsage = {
  quota: { tokenLimit: 100, tokenWindow: 10, ipLimit: 200, ipWindow: 10 },
  tokens: [
    { prefix: 'sm4_ab12', userId: 2, displayName: 'Alice', enabled: true, lastUsedAt: '2026-11-22T09:00:00Z', used: 7, denied24h: 2, requests24h: 41 },
    { prefix: 'sm4_cd34', userId: 3, displayName: 'Bob', enabled: false, lastUsedAt: null, used: 0, denied24h: 0, requests24h: 0 },
  ],
  requests24h: 41,
  denied24h: 3,
  updatedAt: '2026-11-22T10:00:00Z',
}

const numInputs = () => Array.from(document.querySelectorAll('input[type="number"]')) as HTMLInputElement[]
const saveBtn = () => document.querySelector('button[type="submit"]') as HTMLButtonElement | null

describe('McpAdminPanel', () => {
  let mounted: MountedApp | null = null
  let putBody: Record<string, number> | null = null

  beforeEach(() => {
    putBody = null
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        const ok = (v: unknown) => new Response(JSON.stringify(v), { headers: { 'Content-Type': 'application/json' } })
        if (url === '/api/admin/mcp/usage') return ok(usage)
        if (url === '/api/admin/mcp/quota' && init?.method === 'PUT') {
          putBody = JSON.parse(String(init.body)) as Record<string, number>
          return ok(putBody) // the server returns the stored (clamped) quota
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

  it('shows the per-token counters, the window load and the active quota', async () => {
    mounted = await mountWithProviders(<McpAdminPanel />)

    await waitFor(() => expect(numInputs().length).toBe(4))
    const text = mounted!.container.textContent ?? ''
    // The two tokens, their states, their 24 h counters and the current window load.
    expect(text).toContain('sm4_ab12')
    expect(text).toContain('sm4_cd34')
    expect(text).toContain('Alice')
    expect(text).toContain('Bob')
    expect(text).toContain('actif')
    expect(text).toContain('désactivé')
    expect(text).toContain('7/100')
    expect(text).toContain('41')
    expect(text).toContain('jamais')
    // The quota form is pre-filled with the stored values.
    expect(numInputs().map((i) => i.value)).toEqual(['100', '10', '200', '10'])
    // The save button stays idle until the form is touched.
    expect(saveBtn()?.disabled).toBe(true)
  })

  it('saves the quota: the PUT carries the four values and the form follows the stored row', async () => {
    mounted = await mountWithProviders(<McpAdminPanel />)
    await waitFor(() => expect(numInputs().length).toBe(4))

    await setInputValue(numInputs()[0], '5')
    await waitFor(() => expect(saveBtn()?.disabled).toBe(false))
    await clickEl(saveBtn())

    await waitFor(() => expect(putBody).not.toBeNull())
    expect(putBody).toEqual({ tokenLimit: 5, tokenWindow: 10, ipLimit: 200, ipWindow: 10 })
    // The stored row (tokenLimit 5) is now displayed and the form is idle again.
    await waitFor(() => expect(numInputs().map((i) => i.value)).toEqual(['5', '10', '200', '10']))
    expect(saveBtn()?.disabled).toBe(true)
    expect(document.body.textContent).toContain('Quota mis à jour')
  })

  it('blocks a non-integer or out-of-bounds value with the range hint', async () => {
    mounted = await mountWithProviders(<McpAdminPanel />)
    await waitFor(() => expect(numInputs().length).toBe(4))

    await setInputValue(numInputs()[0], '0')
    await waitFor(() => expect(saveBtn()?.disabled).toBe(true))
    expect(mounted!.container.textContent).toContain('Valeur entière entre 1 et')
  })
})
