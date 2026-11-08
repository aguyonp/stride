// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Shoe } from '../../api/types'
import { seedShoes, syncStravaGear } from '../../mocks/gear'
import { clickEl, mountWithProviders, waitFor, type MountedApp } from '../../test/domHarness'
import ShoesView from './ShoesView'
import { setLang } from '../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const text = (el: Element | null | undefined) => (el?.textContent ?? '').replace(/\s+/g, ' ')
const ok = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })

function stub(shoes: Shoe[], missingScopes: string[] = []) {
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      const method = (init?.method ?? 'GET').toUpperCase()
      calls.push(`${method} ${url}`)
      if (url.startsWith('/api/integrations')) return ok({ strava: { available: true, connected: true, status: 'ok', athleteName: 'Alex D.', lastSyncAt: null, lastError: null, scopes: [], missingScopes } })
      if (url === '/api/shoes/import-strava' && method === 'POST') return ok(shoes)
      if (url.startsWith('/api/shoes')) return ok(shoes)
      return ok({})
    }),
  )
  return calls
}

describe('ShoesView: real Strava mileage and retired pairs', () => {
  let mounted: MountedApp | null = null
  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
  })

  it('shows the Strava total, the analysed share, and a collapsed retired group', async () => {
    let n = 0
    const calls = stub(seedShoes(() => ++n, '2026-11-07T06:00:00Z'))
    mounted = await mountWithProviders(<ShoesView />, '/shoes')
    const c = mounted.container
    await waitFor(() => expect(c.querySelectorAll('ul[aria-label="Paires actives"] > li')).toHaveLength(7))

    const active = [...c.querySelectorAll('ul[aria-label="Paires actives"] > li')]
    // the Strava default pair comes first, then the most worn
    expect(text(active[0])).toContain('Souple du quotidien')
    expect(text(active[0])).toContain('Paire principale')
    expect(text(active[0])).toContain('1 065,5 km')
    expect(text(active[0])).toContain('total Strava')
    expect(text(active[0])).toContain('dont 412,6 km analysés par Stride')
    expect(text(active[1])).toContain('Tempo')
    // a pair Strava does not know: its own sum, labelled as such
    const local = active.find((li) => text(li).includes('Pointes cross'))
    expect(text(local)).toContain('18,2 km')
    expect(text(local)).toContain('total Stride')
    expect(text(c)).not.toContain('g2741001')

    const toggle = [...c.querySelectorAll('button')].find((b) => text(b).includes('Retirées (3)'))!
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(c.querySelectorAll('ul[aria-label="Paires retirées"] > li')).toHaveLength(0)
    await clickEl(toggle)
    const retired = [...c.querySelectorAll('ul[aria-label="Paires retirées"] > li')]
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    expect(retired).toHaveLength(3)
    expect(text(retired[0])).toContain('Vela Cloudline 2')
    expect(text(retired[0])).toContain('Retirée sur Strava')
    const deleted = retired.find((li) => text(li).includes('Orra Ridge 4'))
    expect(text(deleted)).toContain('Supprimée de Strava')
    expect(text(deleted)).toContain('405,9 km')
    expect(text(deleted)).toContain('dernier total Strava')

    const refresh = [...c.querySelectorAll('button')].find((b) => text(b).includes('Actualiser depuis Strava'))
    await clickEl(refresh!)
    await waitFor(() => expect(calls).toContain('POST /api/shoes/import-strava'))
  })

  it('asks to reconnect Strava when the token cannot read the gear', async () => {
    let n = 0
    stub(seedShoes(() => ++n, '2026-11-07T06:00:00Z'), ['profile:read_all'])
    mounted = await mountWithProviders(<ShoesView />, '/shoes')
    const c = mounted.container
    await waitFor(() => expect(text(c)).toContain('Kilométrage Strava indisponible'))
    expect([...c.querySelectorAll('button')].some((b) => text(b).includes('Reconnecter Strava'))).toBe(true)
    expect([...c.querySelectorAll('button')].some((b) => text(b).includes('Actualiser depuis Strava'))).toBe(false)
  })
})

describe('mock Strava gear sync', () => {
  it('retires a pair deleted on Strava and keeps a local retire choice', () => {
    let n = 0
    const shoes = seedShoes(() => ++n, '2026-11-01T06:00:00Z')
    shoes.push({ ...shoes[0], id: ++n, name: 'Ancienne', stravaGearId: 'g9999', stravaState: 'active', retired: false, stravaPrimary: false })
    const tempo = shoes.find((s) => s.name === 'Tempo')!
    tempo.retired = true
    syncStravaGear(shoes, () => ++n, '2026-11-07T06:00:00Z')
    const gone = shoes.find((s) => s.stravaGearId === 'g9999')!
    expect(gone).toMatchObject({ retired: true, stravaState: 'deleted', distanceSource: 'strava' })
    expect(tempo.retired).toBe(true)
    expect(shoes.filter((s) => !s.retired)).toHaveLength(6)
  })
})
