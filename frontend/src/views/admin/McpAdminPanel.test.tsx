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
  errors24h: 2,
  updatedAt: '2026-11-22T10:00:00Z',
}

// 4.5.0 (issue #135): the call journal — one clean tools/call and one unknown-token 404.
const journal = {
  inFlight: 1,
  entries: [
    {
      at: '2026-11-22T09:41:12Z', tokenPrefix: 'sm4_ab12', userId: 2, userName: 'Alice',
      method: 'tools/call', tool: 'plan_create', argsSummary: '{"date":"2026-11-24","sport":"run"}',
      status: 200, rpcErrorCode: 0, rpcError: '', durationMs: 480, inFlight: 1, clientCanceled: false,
    },
    {
      at: '2026-11-22T08:15:03Z', tokenPrefix: 'sm4_unkno0', userId: 0, userName: '',
      method: '', tool: '', argsSummary: '',
      status: 404, rpcErrorCode: 0, rpcError: '', durationMs: 2, inFlight: 0, clientCanceled: false,
    },
  ],
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
        if (url === '/api/admin/mcp/journal') return ok(journal)
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

  it('shows the call journal, the 24 h errors KPI and the failed-only filter', async () => {
    mounted = await mountWithProviders(<McpAdminPanel />)

    // The card titles render before the card bodies (the usage KPIs and the journal rows come from
    // two independent fetches): wait on the rendered state each assertion targets.
    const kpiValue = (label: string) =>
      Array.from(document.querySelectorAll('dt')).find((d) => d.textContent === label)?.nextElementSibling?.textContent ?? ''
    await waitFor(() => expect(kpiValue('Erreurs / Timeouts · 24 h')).toBe('2'))

    // The journal is the second table of the page (the per-token table comes first); the journal
    // table only exists once its rows have rendered.
    await waitFor(() => expect(document.querySelectorAll('table').length).toBe(2))
    const journalTable = document.querySelectorAll('table')[1]
    await waitFor(() => expect(journalTable.querySelectorAll('tbody tr').length).toBe(2))

    // Both filters of the journal card are its two selects (the card holds no other).
    const selects = document.querySelectorAll('select')
    await setInputValue(selects[selects.length - 1], 'failed')
    await waitFor(() => expect(journalTable.querySelectorAll('tbody tr').length).toBe(1))
    expect(journalTable.textContent).toContain('404')
  })
})
