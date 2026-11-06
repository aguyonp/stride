// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { BodyMapOverview, BodyStressEntry } from '../../api/types'
import { ALL_BODY_ZONES, BODY_ZONE_META } from '../../components/bodymap/bodyMapMeta'
import { clickEl, installDomPolyfills, mountWithProviders, setInputValue, waitFor, type MountedApp } from '../../test/domHarness'
import BodyMapView from './BodyMapView'

function sampleOverview(entries: BodyStressEntry[]): BodyMapOverview {
  return {
    windowDays: 90,
    zones: ALL_BODY_ZONES.map((zone) => ({
      zone,
      zoneLabel: BODY_ZONE_META[zone].label,
      latestScore: zone === 'knee_pfps' ? 4 : null,
      latestColdNextDayScore: zone === 'knee_pfps' ? 3 : null,
      latestSide: zone === 'knee_pfps' ? 'left' : null,
      latestDate: zone === 'knee_pfps' ? '2026-11-05' : null,
      maxScore30d: zone === 'knee_pfps' ? 4 : null,
      entriesCount90d: zone === 'knee_pfps' ? entries.length : 0,
      trend: zone === 'knee_pfps' ? 'stable' : 'none',
    })),
    entries,
    correlations: [
      {
        zone: 'knee_pfps',
        zoneLabel: BODY_ZONE_META.knee_pfps.label,
        factor: 'elev_loss',
        factorLabel: 'Dénivelé négatif (D−)',
        unit: 'm',
        symptomaticMean: 172,
        baselineMean: 52,
        deltaPct: 230.8,
        symptomaticRuns: 4,
        baselineRuns: 16,
        confidence: 'high',
        shoeHighlight: {
          shoeId: 7,
          shoeName: 'Pegasus 41',
          dropMm: 10,
          distanceKm: 584,
          painEpisodes: 3,
        },
        summary: 'Les gênes rotuliennes apparaissent lors des sorties à fort D−.',
      },
    ],
  }
}

describe('BodyMapView', () => {
  let mounted: MountedApp | null = null
  let entries: BodyStressEntry[] = []
  let createdPayloads: unknown[] = []

  beforeEach(() => {
    installDomPolyfills()
    createdPayloads = []
    entries = [
      {
        id: 101,
        date: '2026-11-05',
        activityId: 12,
        activityName: 'Sortie vallonnée',
        zone: 'knee_pfps',
        zoneLabel: BODY_ZONE_META.knee_pfps.label,
        side: 'left',
        painScore: 4,
        coldNextDayScore: 3,
        note: 'Tiraillement rotulien en descente.',
        createdAt: '2026-11-05T19:00:00Z',
        updatedAt: '2026-11-05T19:00:00Z',
      },
    ]

    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        const method = (init?.method ?? 'GET').toUpperCase()
        if (url.startsWith('/api/profile')) {
          return new Response(
            JSON.stringify({
              focusArea: 'knee_pfps',
              focusLabel: 'Genou',
              injuredSide: 'left',
              timezone: 'Europe/Paris',
              thresholds: {},
              paceTargets: [],
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          )
        }
        if (url.startsWith('/api/activities')) {
          return new Response(
            JSON.stringify({
              items: [],
              hasMore: false,
              freshness: { source: 'db', updatedAt: '2026-11-06T10:00:00Z' },
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          )
        }
        if (url.startsWith('/api/bodymap/entries') && method === 'POST') {
          const parsed = JSON.parse(String(init?.body ?? '{}')) as {
            date: string
            zone: typeof ALL_BODY_ZONES[number]
            side?: 'left' | 'right' | 'both'
            painScore: number
            coldNextDayScore?: number | null
            note?: string
          }
          createdPayloads.push(parsed)
          const created: BodyStressEntry = {
            id: 202,
            date: parsed.date,
            activityId: null,
            activityName: null,
            zone: parsed.zone,
            zoneLabel: BODY_ZONE_META[parsed.zone].label,
            side: parsed.side ?? 'both',
            painScore: parsed.painScore,
            coldNextDayScore: parsed.coldNextDayScore ?? null,
            note: parsed.note ?? '',
            createdAt: '2026-11-06T08:00:00Z',
            updatedAt: '2026-11-06T08:00:00Z',
          }
          entries = [created, ...entries]
          return new Response(JSON.stringify(created), {
            status: 201,
            headers: { 'Content-Type': 'application/json' },
          })
        }
        if (url.startsWith('/api/bodymap')) {
          return new Response(JSON.stringify(sampleOverview(entries)), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          })
        }
        return new Response(JSON.stringify({}), { status: 200, headers: { 'Content-Type': 'application/json' } })
      }),
    )
  })

  afterEach(async () => {
    if (mounted) {
      await mounted.unmount()
      mounted = null
    }
    vi.unstubAllGlobals()
  })

  it('renders all 6 anatomical hotspots, biomechanical correlations, and submits a new pain entry', async () => {
    mounted = await mountWithProviders(<BodyMapView />, '/body')

    await waitFor(() => {
      for (const zone of ALL_BODY_ZONES) {
        const hotspot = document.querySelector(`[data-zone="${zone}"][role="button"]`)
        expect(hotspot).not.toBeNull()
      }
      expect(document.body.textContent).toContain('Dénivelé négatif (D−)')
      expect(document.body.textContent).toContain('Fiabilité élevée')
      expect(document.body.textContent).toContain('Pegasus 41')
    })

    // Click the Achilles anatomical hotspot to filter
    const achillesHotspot = document.querySelector('[data-zone="achilles"][role="button"]')!
    await clickEl(achillesHotspot)
    expect(achillesHotspot.getAttribute('aria-pressed')).toBe('true')

    // Open the entry drawer and submit a new Achilles observation
    const addBtn = Array.from(document.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Signaler une gêne'),
    )
    expect(addBtn).toBeTruthy()
    await clickEl(addBtn!)

    await waitFor(() => {
      expect(document.querySelector('dialog[open]')).not.toBeNull()
    })

    const noteField = document.querySelector<HTMLTextAreaElement>('dialog textarea')!
    await setInputValue(noteField, 'Raideur matinale tendon droit 5 min au réveil')

    const saveBtn = Array.from(document.querySelectorAll('dialog button')).find((b) =>
      b.textContent?.trim() === 'Enregistrer',
    )
    expect(saveBtn).toBeTruthy()
    await clickEl(saveBtn!)

    await waitFor(() => {
      expect(createdPayloads.length).toBe(1)
      expect(createdPayloads[0]).toMatchObject({
        zone: 'achilles',
        note: 'Raideur matinale tendon droit 5 min au réveil',
      })
    })
  })
})
