// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { MeterSnapshot, ProviderStats } from '../../api/types'
import { mountWithProviders, waitFor, type MountedApp } from '../../test/domHarness'
import { setLang } from '../../i18n/lang'
import { ProvidersPanel } from './ProvidersPanel'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const provider = (p: Partial<ProviderStats> & Pick<ProviderStats, 'provider'>): ProviderStats => ({
  requests1h: 12,
  requests24h: 340,
  errors1h: 0,
  rateLimited24h: 2,
  inFlight: 0,
  cooldownUntil: null,
  quota: null,
  avgLatencyMs: 420,
  ...p,
})

const meter: MeterSnapshot = {
  providers: [
    provider({ provider: 'garmin', quota: { limit15m: 12, usage15m: 3, limitDaily: 400, usageDaily: 100, updatedAt: '2026-11-21T07:00:00Z' } }),
    provider({ provider: 'strava', quota: { limit15m: 100, usage15m: 95, limitDaily: 3000, usageDaily: 2700, updatedAt: '2026-11-21T07:00:00Z' } }),
    provider({ provider: 'open-meteo' }),
  ],
  recentErrors: [],
}

const gauge = (labelStart: string) =>
  Array.from(document.querySelectorAll('[role="img"]')).find((el) => (el.getAttribute('aria-label') ?? '').startsWith(labelStart))

const gaugeFill = (el: Element) => el.querySelector('div') as HTMLElement | null

// jsdom does not always resolve the `background` shorthand: check the raw attribute or the longhand.
const hasColor = (el: HTMLElement, color: string) => el.getAttribute('style')?.includes(color) || el.style.backgroundColor === color

const fillOf = (labelStart: string): HTMLElement => {
  const g = gauge(labelStart)
  const fill = g ? gaugeFill(g) : null
  if (!fill) throw new Error(`gauge « ${labelStart} » manquante`)
  return fill
}

describe('ProvidersPanel (quota verticale)', () => {
  let mounted: MountedApp | null = null

  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        const ok = (v: unknown) => new Response(JSON.stringify(v), { headers: { 'Content-Type': 'application/json' } })
        if (url === '/api/admin/meter') return ok(meter)
        if (url === '/api/admin/users') return ok([])
        if (url === '/api/admin/connections') return ok([])
        return new Response(JSON.stringify({}), { status: 404, headers: { 'Content-Type': 'application/json' } })
      }),
    )
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
  })

  it('measures the quota with a vertical gauge filled from the bottom', async () => {
    mounted = await mountWithProviders(<ProvidersPanel />)

    await waitFor(() => expect(gauge('15 minutes :')).toBeTruthy())
    // Garmin: 3/12 → 25 % of the track, filled from the bottom.
    const g15 = fillOf('15 minutes :')
    expect(g15.style.height).toBe('25%')
    expect(hasColor(g15, 'var(--accent)')).toBe(true)
    // Strava: 95/100 → 95 %, over the 90 % threshold.
    const s15 = fillOf('15 minutes : 95')
    expect(s15.style.height).toBe('95%')
    expect(hasColor(s15, 'var(--danger)')).toBe(true)

    // Both windows are present for each provider with a quota; the counts stay readable.
    expect(gauge('Journée : 100 sur 400')).toBeTruthy()
    expect(mounted!.container.textContent).toContain('3/12')
    expect(mounted!.container.textContent).toContain('100/400')
    expect(mounted!.container.textContent).toContain('95/100')

    // A provider without a quota has no gauge.
    expect(document.querySelectorAll('[role="img"]').length).toBe(4)
  })
})
