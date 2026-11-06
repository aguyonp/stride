// @vitest-environment jsdom
import { createElement } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { EventStreamController } from '../components/layout/EventStreamController'
import { db, resetDb } from '../mocks/db'
import { setFlags } from '../mocks/flags'
import { mockFetch } from '../mocks/server'
import { emitStreamEvent, getEventStoreState, resetEventStore, startLlmStream } from '../state/events'
import { installDomPolyfills, mountWithProviders, waitFor } from '../test/domHarness'
import { openapiRequest } from './openapiClient'

const rawModules = import.meta.glob('../**/*.{ts,tsx}', {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>

const mockCall = (method: string, path: string, body?: unknown) =>
  mockFetch(path, {
    method,
    headers: method === 'GET' ? {} : { 'X-Stride-CSRF': '1' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })

describe('Stride 4.0.0 architecture invariants, OpenAPI client, SSE controller & v4 mocks', () => {
  beforeEach(() => {
    installDomPolyfills()
    resetEventStore()
    setFlags(['admin'])
    resetDb()
  })

  afterEach(() => {
    resetEventStore()
    vi.unstubAllGlobals()
  })

  it('enforces the Zero Polling invariant across all production frontend source files', () => {
    const entries = Object.entries(rawModules).filter(
      ([path]) => !/\.test\.(ts|tsx)$/.test(path),
    )
    expect(entries.length).toBeGreaterThan(20)

    const violations: string[] = []
    for (const [path, content] of entries) {
      if (content.includes('refetchInterval')) {
        violations.push(`${path}: contains refetchInterval`)
      }
      if (/\bsetInterval\s*\(/.test(content)) {
        violations.push(`${path}: contains setInterval(`)
      }
    }
    expect(violations).toEqual([])
  })

  it('executes strictly typed openapiRequest calls with path params, query params, and CSRF protection', async () => {
    const fetchSpy = vi.fn(async (_url: string, _init?: RequestInit) => {
      return new Response(
        JSON.stringify({
          id: 9,
          label: 'Coach club',
          tokenPrefix: 'sh_abc12',
          url: '/share/sh_abc12345',
          expiresAt: null,
          revoked: false,
          revokedAt: null,
          accessCount: 0,
          lastAccessedAt: null,
          createdAt: '2026-11-06T12:00:00Z',
        }),
        { status: 201, headers: { 'Content-Type': 'application/json' } },
      )
    })
    vi.stubGlobal('fetch', fetchSpy)

    const link = await openapiRequest('post', '/api/share', {
      body: { label: 'Coach club', expiresInDays: 30 },
    })
    expect(link.label).toBe('Coach club')
    expect(fetchSpy.mock.calls[0][0]).toBe('/api/share')
    const postInit = fetchSpy.mock.calls[0][1] as RequestInit
    expect((postInit.headers as Record<string, string>)['X-Stride-CSRF']).toBe('1')

    fetchSpy.mockImplementationOnce(async () =>
      new Response(
        JSON.stringify({ zones: [], entries: [], correlations: [], windowDays: 90 }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    )
    await openapiRequest('get', '/api/bodymap', {
      query: { days: 90, zone: 'knee_pfps' },
    })
    expect(fetchSpy.mock.calls[1][0]).toBe('/api/bodymap?days=90&zone=knee_pfps')
  })

  it('processes SSE events in EventStreamController (job progress, sync cache, and LLM token streaming)', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(JSON.stringify({ items: [] }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    )

    const mounted = await mountWithProviders(createElement(EventStreamController, { userId: 1 }), '/')

    emitStreamEvent('connected', { status: 'ok', userId: 1, serverTime: '2026-11-06T20:00:00Z' })
    expect(getEventStoreState().connected).toBe(true)

    emitStreamEvent('job.updated', {
      id: 'job_sync_99',
      userId: 1,
      kind: 'sync.provider',
      queue: 'default',
      state: 'running',
      label: 'Synchro Strava : 3/5 séances traitées',
      progressPct: 60,
      processed: 3,
      total: 5,
      attempts: 1,
      maxAttempts: 4,
      error: null,
      result: null,
      createdAt: '2026-11-06T20:00:00Z',
      updatedAt: '2026-11-06T20:00:02Z',
    })
    expect(getEventStoreState().jobs['job_sync_99']?.processed).toBe(3)

    // Token-by-token LLM stream accumulation
    startLlmStream('job_llm_77', 'day', '2026-11-06')
    emitStreamEvent('llm.token', { jobId: 'job_llm_77', kind: 'day', ref: '2026-11-06', delta: 'Bonjour ' })
    emitStreamEvent('llm.token', { jobId: 'job_llm_77', kind: 'day', ref: '2026-11-06', delta: 'Camille !' })
    expect(getEventStoreState().llmByJob['job_llm_77']?.text).toBe('Bonjour Camille !')

    emitStreamEvent('llm.done', { jobId: 'job_llm_77', kind: 'day', ref: '2026-11-06' })
    expect(getEventStoreState().llmByJob['job_llm_77']?.done).toBe(true)

    // Sync cache direct update
    emitStreamEvent('sync.updated', {
      running: false,
      lastSyncAt: '2026-11-06T20:01:00Z',
      lastLiveAt: '2026-11-06T20:01:00Z',
      cooldownUntil: null,
      providers: { garmin: null, strava: null },
    })
    await waitFor(() => {
      const cached = mounted.queryClient.getQueryData<{ lastSyncAt: string }>(['sync', 'status'])
      expect(cached?.lastSyncAt).toBe('2026-11-06T20:01:00Z')
    })

    await mounted.unmount()
  })

  it('serves complete v4 mock endpoints for cycles, bodymap, share links, and public share tokens', async () => {
    expect(db().users.length).toBeGreaterThan(0)

    const ver = await (await mockCall('GET', '/version')).json()
    expect(ver.version).toBe('4.0.0')

    // Cycles
    const cycles = await (await mockCall('GET', '/api/cycles')).json()
    expect(cycles.activePlan).not.toBeNull()
    expect(cycles.phases.length).toBeGreaterThanOrEqual(3)

    const createdCyclesRes = await mockCall('POST', '/api/cycles/plans', {
      name: 'Prépa 10 km Hiver',
      startDate: '2026-11-09',
      endDate: '2027-01-17',
      autoScaffold: true,
      peakWeeklyVolumeKm: 46,
      peakWeeklyLoad: 350,
    })
    expect(createdCyclesRes.status).toBe(201)
    const createdCycles = await createdCyclesRes.json()
    expect(createdCycles.activePlan.name).toBe('Prépa 10 km Hiver')

    // BodyMap
    const bm = await (await mockCall('GET', '/api/bodymap?days=90')).json()
    expect(bm.zones).toHaveLength(6)
    expect(bm.correlations.length).toBeGreaterThan(0)

    const newEntryRes = await mockCall('POST', '/api/bodymap/entries', {
      date: '2026-11-06',
      zone: 'plantar_fascia',
      side: 'right',
      painScore: 3,
      coldNextDayScore: 4,
      note: 'Premiers pas sensibles au lever',
    })
    expect(newEntryRes.status).toBe(201)

    // Share Links + Public Share View + 1-click Revoke
    const createShareRes = await mockCall('POST', '/api/share', {
      label: 'Cabinet Kiné Sport',
      expiresInDays: 7,
    })
    expect(createShareRes.status).toBe(201)
    const createdShare = await createShareRes.json()
    const token = String(createdShare.url).replace('/share/', '')

    // Unauthenticated public share access works even when logged out
    db().sessionUserId = null
    const pubOk = await mockCall('GET', `/api/public/share/${token}`)
    expect(pubOk.status).toBe(200)
    const pubPayload = await pubOk.json()
    expect(pubPayload.shareLabel).toBe('Cabinet Kiné Sport')

    // Expired link returns 410 GONE
    const pubExpired = await mockCall('GET', '/api/public/share/expired-token')
    expect(pubExpired.status).toBe(410)

    // Log back in as admin, revoke the link, and verify public endpoint returns 404
    db().sessionUserId = 1
    const revokeRes = await mockCall('DELETE', `/api/share/${createdShare.id}`)
    expect(revokeRes.status).toBe(204)

    const pubRevoked = await mockCall('GET', `/api/public/share/${token}`)
    expect(pubRevoked.status).toBe(404)
  })
})
