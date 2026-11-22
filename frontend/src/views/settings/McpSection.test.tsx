// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { McpStatus } from '../../api/types'
import { clickEl, mountWithProviders, waitFor, type MountedApp } from '../../test/domHarness'
import { setLang } from '../../i18n/lang'
import { McpSection } from './McpSection'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const TOKEN = 'sm4_' + 'a1B2'.repeat(10) + 'c3' // 47 chars
const dialogButton = (label: string) => Array.from(document.querySelectorAll<HTMLButtonElement>('dialog button')).find((b) => b.textContent?.trim() === label) ?? null
const sectionButton = (label: string) => Array.from(document.querySelectorAll<HTMLButtonElement>('section[id="mcp"] button')).find((b) => b.textContent?.trim() === label) ?? null

type McpState = { status: McpStatus; calls: { method: string; url: string; body?: unknown }[] }

function stubApi(state: McpState) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      state.calls.push({ method, url, body: init?.body ? JSON.parse(String(init.body)) : undefined })
      if (url === '/api/mcp' && method === 'GET') return new Response(JSON.stringify(state.status), { headers: { 'Content-Type': 'application/json' } })
      if (url === '/api/mcp' && method === 'PUT') {
        state.status = { ...state.status, enabled: (init?.body ? (JSON.parse(String(init.body)) as { enabled: boolean }).enabled : false) }
        return new Response(null, { status: 204 })
      }
      if (url === '/api/mcp/token' && method === 'POST') {
        state.status = { ...state.status, hasToken: true, tokenPrefix: TOKEN.slice(0, 8), createdAt: '2026-11-21T10:00:00Z', lastUsedAt: null }
        return new Response(JSON.stringify({ token: TOKEN, prefix: TOKEN.slice(0, 8) }), { headers: { 'Content-Type': 'application/json' } })
      }
      if (url === '/api/mcp/token' && method === 'DELETE') {
        state.status = { ...state.status, hasToken: false, tokenPrefix: '', createdAt: null, lastUsedAt: null }
        return new Response(null, { status: 204 })
      }
      return new Response(JSON.stringify({ error: `mock inconnu ${method} ${url}` }), { status: 404, headers: { 'Content-Type': 'application/json' } })
    }),
  )
}

describe('McpSection', () => {
  let mounted: MountedApp | null = null

  beforeEach(() => vi.unstubAllGlobals())
  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
  })

  it('shows the disabled state and generates a one-time token', async () => {
    const state: McpState = {
      status: { enabled: false, hasToken: false, tokenPrefix: '', createdAt: null, lastUsedAt: null },
      calls: [],
    }
    stubApi(state)
    mounted = await mountWithProviders(<McpSection />)

    await waitFor(() => expect(mounted!.container.textContent).toContain('Désactivé'))
    expect(mounted!.container.textContent).toContain('Serveur MCP')

    // Toggle → PUT /api/mcp
    await clickEl(mounted!.container.querySelector('[role="switch"]'))
    await waitFor(() => {
      expect(state.calls.some((c) => c.method === 'PUT' && c.url === '/api/mcp' && (c.body as { enabled: boolean }).enabled === true)).toBe(true)
    })

    // Generate → the plaintext is displayed once, with the endpoint.
    await clickEl(sectionButton('Générer un jeton'))
    await waitFor(() => expect(mounted!.container.textContent).toContain('Copie ce jeton maintenant : il ne sera jamais réaffiché.'))
    await waitFor(() => {
      expect(state.calls.some((c) => c.method === 'POST' && c.url === '/api/mcp/token')).toBe(true)
      expect(mounted!.container.textContent).toContain(TOKEN)
      expect(mounted!.container.textContent).toContain(`/mcp/${TOKEN}`)
    })
  })

  it('offers regenerate/revoke once a token exists and revokes through the dialog', async () => {
    const state: McpState = {
      status: { enabled: true, hasToken: true, tokenPrefix: TOKEN.slice(0, 8), createdAt: '2026-11-01T08:00:00Z', lastUsedAt: '2026-11-20T18:00:00Z' },
      calls: [],
    }
    stubApi(state)
    mounted = await mountWithProviders(<McpSection />)

    await waitFor(() => expect(mounted!.container.textContent).toContain('Activé'))
    await waitFor(() => expect(sectionButton('Révoquer le jeton')).toBeTruthy())
    // No token is ever displayed in the steady state.
    expect(mounted!.container.textContent).not.toContain(TOKEN)

    // Revoke → confirm dialog → DELETE /api/mcp/token.
    await clickEl(sectionButton('Révoquer le jeton'))
    await waitFor(() => expect(dialogButton('Révoquer le jeton')).toBeTruthy())
    await clickEl(dialogButton('Révoquer le jeton'))
    await waitFor(() => {
      expect(state.calls.some((c) => c.method === 'DELETE' && c.url === '/api/mcp/token')).toBe(true)
      expect(mounted!.container.textContent).toContain('Générer un jeton')
    })
  })

  it('lists the 13 functions and offers a paste-ready setup message with the real endpoint', async () => {
    const state: McpState = {
      status: { enabled: true, hasToken: false, tokenPrefix: '', createdAt: null, lastUsedAt: null },
      calls: [],
    }
    stubApi(state)
    mounted = await mountWithProviders(<McpSection />)

    // The functions list expands inline (no popover: it can never be clipped by the viewport).
    const toolsBtn = sectionButton('Fonctions')
    expect(toolsBtn?.getAttribute('aria-expanded')).toBe('false')
    await clickEl(toolsBtn)
    await waitFor(() => {
      expect(toolsBtn?.getAttribute('aria-expanded')).toBe('true')
      expect(mounted!.container.querySelector('#mcp-tools')?.hasAttribute('hidden')).toBe(false)
      expect(mounted!.container.textContent).toContain('Fonctions disponibles')
      expect(mounted!.container.textContent).toContain('Lecture (7)')
      expect(mounted!.container.textContent).toContain('Écriture (6)')
      for (const name of ['planning', 'day', 'activities', 'activity', 'health', 'cycles', 'profile', 'plan_create', 'plan_update', 'plan_delete', 'event_create', 'event_update', 'event_delete']) {
        expect(mounted!.container.textContent).toContain(name)
      }
    })
    // Toggling again hides the list.
    await clickEl(toolsBtn)
    await waitFor(() => expect(mounted!.container.querySelector('#mcp-tools')?.hasAttribute('hidden')).toBe(true))

    // Generate → the paste-ready message embeds the real endpoint.
    await clickEl(sectionButton('Générer un jeton'))
    await waitFor(() => {
      expect(mounted!.container.textContent).toContain(
        `Aide-moi à ajouter un serveur MCP dans ma configuration globale. Je veux connecter ce serveur MCP : ${window.location.origin}/mcp/${TOKEN}`,
      )
    })
  })
})
