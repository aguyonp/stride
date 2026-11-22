import { describe, expect, it } from 'vitest'
import type { McpStatus, McpToken } from '../api/types'
import { resetDb } from './db'
import { setFlags } from './flags'
import { mockFetch } from './server'

const call = (method: string, path: string, body?: unknown) =>
  mockFetch(path, { method, headers: { 'X-Stride-CSRF': '1' }, body: body === undefined ? undefined : JSON.stringify(body) })

const setup = () => {
  setFlags(['admin'])
  resetDb()
}

describe('4.3 mocks: MCP server', () => {
  it('starts disabled, without a token', async () => {
    setup()
    const st = (await (await call('GET', '/api/mcp')).json()) as McpStatus
    expect(st).toEqual({ enabled: false, hasToken: false, tokenPrefix: '', createdAt: null, lastUsedAt: null })
  })

  it('toggles, generates a 47-char token shown once, and revokes it', async () => {
    setup()
    expect((await call('PUT', '/api/mcp', { enabled: true })).status).toBe(204)

    const gen = (await (await call('POST', '/api/mcp/token')).json()) as McpToken
    expect(gen.token.startsWith('sm4_')).toBe(true)
    expect(gen.token).toHaveLength(47)
    expect(gen.prefix).toBe(gen.token.slice(0, 8))

    const st = (await (await call('GET', '/api/mcp')).json()) as McpStatus
    expect(st.hasToken).toBe(true)
    expect(st.tokenPrefix).toBe(gen.prefix)
    expect(st.createdAt).not.toBeNull()
    expect(st.lastUsedAt).toBeNull()

    // The session API never exposes the plaintext token (only its prefix).
    const raw = await (await call('GET', '/api/mcp')).text()
    expect(raw).not.toContain(gen.token)

    const rev = await call('DELETE', '/api/mcp/token')
    expect(rev.status).toBe(204)
    const st2 = (await (await call('GET', '/api/mcp')).json()) as McpStatus
    expect(st2).toEqual({ enabled: true, hasToken: false, tokenPrefix: '', createdAt: null, lastUsedAt: null })

    expect((await call('PUT', '/api/mcp', { enabled: false })).status).toBe(204)
    expect(((await (await call('GET', '/api/mcp')).json()) as McpStatus).enabled).toBe(false)
  })

  it('regeneration replaces the token and its prefix', async () => {
    setup()
    const a = (await (await call('POST', '/api/mcp/token')).json()) as McpToken
    const b = (await (await call('POST', '/api/mcp/token')).json()) as McpToken
    expect(b.token).not.toBe(a.token)
    const st = (await (await call('GET', '/api/mcp')).json()) as McpStatus
    expect(st.tokenPrefix).toBe(b.prefix)
    expect(st.tokenPrefix).not.toBe(a.prefix)
  })
})
