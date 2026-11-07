// @vitest-environment jsdom
import { createElement } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { EventStreamController } from '../components/layout/EventStreamController'
import { db, resetDb } from '../mocks/db'
import { setFlags } from '../mocks/flags'
import { mockFetch } from '../mocks/server'
import { emitStreamEvent, getEventStoreState, resetEventStore, startLlmStream } from '../state/events'
import { installDomPolyfills, mountWithProviders, waitFor } from '../test/domHarness'
import { FALLBACK_POLL_MS } from './polling'
import type { BodyMapOverview, PublicShareView } from './types'

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

describe('Stride 4.0.0 architecture invariants, SSE controller & v4 mocks', () => {
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

  it('confines polling to the stream fallbacks declared in the API query layer', () => {
    const entries = Object.entries(rawModules).filter(
      ([path]) => !/\.test\.(ts|tsx)$/.test(path),
    )
    expect(entries.length).toBeGreaterThan(20)

    const violations: string[] = []
    for (const [path, content] of entries) {
      if (/\bsetInterval\s*\(/.test(content)) violations.push(`${path}: contains setInterval(`)
      for (const line of content.split('\n')) {
        if (!line.includes('refetchInterval')) continue
        const allowed = /^\.\/queries(V\d+|Admin)?\.ts$/.test(path) && /poll\(|ADMIN_METER_POLL_MS/.test(line)
        if (!allowed) violations.push(`${path}: ${line.trim()}`)
      }
    }
    expect(violations).toEqual([])
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
    const jobsFetches = () => vi.mocked(fetch).mock.calls.filter(([u]) => String(u).startsWith('/api/jobs')).length
    const jobsInterval = () => {
      const q = mounted.queryClient.getQueryCache().find({ queryKey: ['jobs'] })
      const opt = q?.observers[0]?.options.refetchInterval
      return typeof opt === 'function' && q ? opt(q) : opt
    }

    // Stream down: jobs fall back to slow polling
    await waitFor(() => expect(jobsFetches()).toBe(1))
    expect(jobsInterval()).toBe(FALLBACK_POLL_MS)

    // Every (re)connection refetches jobs and stops polling while idle
    emitStreamEvent('connected', { status: 'ok', userId: 1, serverTime: '2026-11-06T20:00:00Z' })
    expect(getEventStoreState().connected).toBe(true)
    await waitFor(() => expect(jobsFetches()).toBe(2))
    await waitFor(() => expect(jobsInterval()).toBe(false))

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
    await waitFor(() => expect(jobsInterval()).toBe(FALLBACK_POLL_MS))

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

    emitStreamEvent('job.updated', {
      ...getEventStoreState().jobs['job_sync_99']!,
      state: 'completed',
      updatedAt: '2026-11-06T20:01:00Z',
    })
    await waitFor(() => expect(jobsInterval()).toBe(false))

    emitStreamEvent('connected', { status: 'ok', userId: 1, serverTime: '2026-11-06T20:05:00Z' })
    await waitFor(() => expect(jobsFetches()).toBe(3))

    await mounted.unmount()
  })

  it('serves complete v4 mock endpoints for cycles, bodymap, share links, and public share tokens', async () => {
    expect(db().users.length).toBeGreaterThan(0)

    const ver = await (await mockCall('GET', '/version')).json()
    expect(ver.version).toBe('4.1.0')

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

    // BodyMap: realistic factors where data allows, explicit insufficient data elsewhere
    const bm = (await (await mockCall('GET', '/api/bodymap?days=90')).json()) as BodyMapOverview
    expect(bm.zones).toHaveLength(10)
    expect(bm.correlations.some((c) => !c.insufficientData && c.symptomaticMean !== null)).toBe(true)
    const thin = bm.correlations.find((c) => c.insufficientData)
    expect(thin).toMatchObject({ symptomaticMean: null, baselineMean: null, deltaPct: null, confidence: null })

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
    const pubPayload = (await pubOk.json()) as PublicShareView
    expect(pubPayload.shareLabel).toBe('Cabinet Kiné Sport')
    expect(pubPayload.loadSummary?.efLatest).not.toBeNull()

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

  it('returns nulls and insufficientData flags, never fabricated values, under the sparse scenario', async () => {
    setFlags(['admin', 'sparse'])
    resetDb()

    const bm = (await (await mockCall('GET', '/api/bodymap?days=90')).json()) as BodyMapOverview
    expect(bm.correlations.length).toBeGreaterThan(0)
    for (const c of bm.correlations) {
      expect(c).toMatchObject({ insufficientData: true, symptomaticMean: null, symptomaticRuns: null, confidence: null, summary: null })
    }

    const share = await (await mockCall('POST', '/api/share', { label: 'Coach', expiresInDays: 7 })).json()
    const view = (await (await mockCall('GET', `/api/public/share/${String(share.url).replace('/share/', '')}`)).json()) as PublicShareView
    expect(view.loadSummary).toMatchObject({ efLatest: null, efTrendPct: null, impactLoad7d: null, jointIndex7d: null })
    expect(view.loadSeries?.every((p) => p.efSpeed === null && p.impactLoad7d === null && p.jointIndex7d === null)).toBe(true)
    expect(view.recentSessions?.every((r) => r.efSpeed === null && r.impactLoad === null && r.jointIndex === null)).toBe(true)
  })
})
