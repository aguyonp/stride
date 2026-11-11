import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from './client'
import { setLang } from '../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

function stubFetch(status: number, body: unknown, contentType = 'application/json') {
  const fn = vi.fn(async (_url: string, _init?: RequestInit) => new Response(typeof body === 'string' ? body : JSON.stringify(body), { status, headers: { 'Content-Type': contentType } }))
  vi.stubGlobal('fetch', fn)
  return fn
}

afterEach(() => vi.unstubAllGlobals())

describe('api client', () => {
  it('sends the CSRF header and same-origin credentials on state-changing requests', async () => {
    const fn = stubFetch(200, { running: true })
    await api.startSync()
    const init = fn.mock.calls[0][1] as RequestInit
    expect(init.method).toBe('POST')
    expect(init.credentials).toBe('same-origin')
    expect((init.headers as Record<string, string>)['X-Stride-CSRF']).toBe('1')
  })

  it('does not send the CSRF header on GET', async () => {
    const fn = stubFetch(200, { authenticated: false })
    await api.authState()
    const init = fn.mock.calls[0][1] as RequestInit
    expect((init.headers as Record<string, string>)['X-Stride-CSRF']).toBeUndefined()
  })

  it('maps error bodies to ApiError with code', async () => {
    stubFetch(400, { error: 'Clé refusée', code: 'PROVIDER_UNAUTHORIZED' })
    await expect(api.connectGarmin('x')).rejects.toMatchObject({ status: 400, code: 'PROVIDER_UNAUTHORIZED', message: 'Clé refusée' })
  })

  it('sends Accept-Language and translates error codes in English (§15.2)', async () => {
    setLang('en', { persist: false })
    try {
      const fn = stubFetch(400, { error: 'Clé refusée', code: 'PROVIDER_UNAUTHORIZED' })
      const err = await api.connectGarmin('x').catch((e) => e)
      expect((fn.mock.calls[0][1]?.headers as Record<string, string>)['Accept-Language']).toBe('en')
      expect(err).toMatchObject({ code: 'PROVIDER_UNAUTHORIZED', message: 'The provider denied access: reconnect the integration.' })
    } finally {
      setLang('fr', { persist: false })
    }
  })

  it('encodes include=none when every chip is off and returns markdown text', async () => {
    const fn = stubFetch(200, '> **DIRECTIVE', 'text/markdown; charset=utf-8')
    const md = await api.activityReport(42, [])
    expect(md).toBe('> **DIRECTIVE')
    expect(fn.mock.calls[0][0]).toBe('/api/activities/42/report?include=none')
  })

  it('falls back to a French message on 401 without body', async () => {
    stubFetch(401, '')
    const err = await api.me().catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).code).toBe('UNAUTHORIZED')
  })
})
