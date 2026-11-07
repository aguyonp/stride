// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from '../../App'
import type { PublicShareView } from '../../api/types'
import { ALL_BODY_ZONES, BODY_ZONE_META } from '../../components/bodymap/bodyMapMeta'
import { installDomPolyfills, mountWithProviders, waitFor, type MountedApp } from '../../test/domHarness'
import SharePublicView from './SharePublicView'

function sampleShareView(): PublicShareView {
  return {
    generatedAt: '2026-11-06T18:00:00Z',
    shareLabel: 'Kiné du sport',
    expiresAt: '2026-12-06T18:00:00Z',
    periodWeeks: 12,
    sections: ['activities', 'health', 'recovery', 'load', 'records', 'plan', 'shoes', 'notes'],
    athlete: {
      displayName: 'Camille Laurent',
      focusArea: 'knee_pfps',
      focusLabel: 'Genou / Fémoro-patellaire',
      injuredSide: 'left',
    },
    readiness: {
      level: 'green',
      title: 'Prêt pour l’entraînement prévu',
      reasons: ['HRV stable sur 7 jours et ratio ACWR optimal.'],
      hrv7dAvg: 64,
      hrvBaselineLow: 56,
      nightsBelowBaseline: 0,
      sleep7dAvg: 7.8,
      loadRatio: 1.06,
    },
    loadSummary: {
      acute: 390,
      chronic: 368,
      acwr: 1.06,
      verdict: 'optimal',
      efLatest: 1.54,
      efTrendPct: 3.1,
      impactLoad7d: 410,
      jointIndex7d: 5.9,
    },
    loadSeries: [
      { date: '2026-10-25', acute: 340, chronic: 350, acwr: 0.97, efSpeed: 1.5, impactLoad7d: 390, jointIndex7d: 5.5 },
      { date: '2026-11-01', acute: 375, chronic: 360, acwr: 1.04, efSpeed: 1.52, impactLoad7d: 405, jointIndex7d: 5.8 },
      { date: '2026-11-06', acute: 390, chronic: 368, acwr: 1.06, efSpeed: 1.54, impactLoad7d: 410, jointIndex7d: 5.9 },
    ],
    cycles: {
      plans: [
        {
          id: 1,
          goalId: null,
          name: 'Prépa Semi de Marseille',
          sport: 'run',
          startDate: '2026-10-05',
          endDate: '2026-12-20',
          targetEventLabel: 'Semi-marathon · 1h38',
          status: 'active',
          note: '',
          createdAt: '2026-10-01T10:00:00Z',
          updatedAt: '2026-11-06T10:00:00Z',
        },
      ],
      activePlan: {
        id: 1,
        goalId: null,
        name: 'Prépa Semi de Marseille',
        sport: 'run',
        startDate: '2026-10-05',
        endDate: '2026-12-20',
        targetEventLabel: 'Semi-marathon · 1h38',
        status: 'active',
        note: '',
        createdAt: '2026-10-01T10:00:00Z',
        updatedAt: '2026-11-06T10:00:00Z',
      },
      phases: [
        {
          id: 10,
          planId: 1,
          name: 'Développement seuil',
          phaseType: 'build',
          orderIndex: 0,
          startDate: '2026-10-05',
          endDate: '2026-11-15',
          focus: 'Endurance active',
          weeklyVolumeKm: 48,
          targetWeeklyLoad: 380,
          targetWeeklyDurationS: 15600,
          note: '',
          actuals: {
            plannedVolumeKm: 192,
            actualVolumeKm: 184,
            volumeCompletionPct: 95.8,
            plannedLoad: 1520,
            actualLoad: 1480,
            loadCompletionPct: 97.4,
            plannedDurationS: 62400,
            actualDurationS: 59800,
            sessionsPlanned: 16,
            sessionsCompleted: 15,
          },
          weeks: [],
        },
      ],
      standaloneWeeks: [],
      totals: {
        plannedVolumeKm: 192,
        actualVolumeKm: 184,
        volumeCompletionPct: 95.8,
        plannedLoad: 1520,
        actualLoad: 1480,
        loadCompletionPct: 97.4,
        plannedDurationS: 62400,
        actualDurationS: 59800,
        sessionsPlanned: 16,
        sessionsCompleted: 15,
      },
    },
    recentSessions: [
      {
        id: 501,
        date: '2026-11-05',
        startLocal: '2026-11-05T18:00:00',
        name: 'Footing vallonné souple',
        sport: 'run',
        typeLabel: 'Course',
        distanceM: 10400,
        durationS: 3240,
        avgPaceSecPerKm: 311,
        gapSecPerKm: 302,
        avgHR: 146,
        avgCadenceSpm: 174,
        gctMs: 244,
        elevGainM: 110,
        elevLossM: 112,
        trainingLoad: 86,
        impactLoad: 92,
        jointIndex: 5.4,
        efSpeed: 1.54,
        shoeName: 'Clifton 9',
        shoeDropMm: 5,
        painDuring: 2,
        painNextMorning: 1,
      },
    ],
    bodyMap: {
      windowDays: 90,
      zones: ALL_BODY_ZONES.map((z) => ({
        zone: z,
        zoneLabel: BODY_ZONE_META[z].label,
        latestScore: z === 'knee_pfps' ? 2 : null,
        latestColdNextDayScore: z === 'knee_pfps' ? 1 : null,
        latestSide: z === 'knee_pfps' ? 'left' : null,
        latestDate: z === 'knee_pfps' ? '2026-11-05' : null,
        maxScore30d: z === 'knee_pfps' ? 3 : null,
        entriesCount90d: z === 'knee_pfps' ? 2 : 0,
        trend: z === 'knee_pfps' ? 'improving' : 'none',
      })),
      entries: [],
      correlations: [],
    },
    shoes: [
      {
        name: 'Clifton 9',
        brand: 'Hoka',
        model: 'Clifton 9',
        dropMm: 5,
        type: 'soft',
        distanceKm: 312.4,
        retired: false,
      },
    ],
  }
}

describe('SharePublicView & unauthenticated /share/:token routing', () => {
  let mounted: MountedApp | null = null

  beforeEach(() => {
    installDomPolyfills()
  })

  afterEach(async () => {
    if (mounted) {
      await mounted.unmount()
      mounted = null
    }
    vi.unstubAllGlobals()
  })

  it('renders the complete read-only clinical dossier with zero internal navigation or edit buttons', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(JSON.stringify(sampleShareView()), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    )

    mounted = await mountWithProviders(<SharePublicView token="valid-token" />, '/share/valid-token')

    await waitFor(() => {
      expect(document.body.textContent).toContain('Stride · Dossier Athlète Partagé')
      expect(document.body.textContent).toContain('Lecture seule')
      expect(document.body.textContent).toContain('Camille Laurent')
      expect(document.body.textContent).toContain('Prépa Semi de Marseille')
      expect(document.body.textContent).toContain('Clifton 9')
      expect(document.body.textContent).toContain('Footing vallonné souple')
    })

    // Zero internal app links (/settings, /admin, /plan, /)
    const anchors = Array.from(document.querySelectorAll('a'))
    expect(anchors).toHaveLength(0)

    // Anatomy hotspots are strictly read-only (not role="button")
    expect(document.querySelectorAll('[data-zone][role="button"]')).toHaveLength(0)
    expect(document.querySelectorAll('[data-zone]').length).toBeGreaterThanOrEqual(6)
  })

  it('shows « Données insuffisantes » instead of numbers when EF, impact or joint index are unknown', async () => {
    const view = sampleShareView()
    view.loadSummary = { ...view.loadSummary!, efLatest: null, efTrendPct: 3.1, impactLoad7d: null, jointIndex7d: null }
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(JSON.stringify(view), { status: 200, headers: { 'Content-Type': 'application/json' } }),
      ),
    )

    mounted = await mountWithProviders(<SharePublicView token="sparse-token" />, '/share/sparse-token')

    const cell = (label: string) =>
      Array.from(document.querySelectorAll('dt')).find((dt) => dt.textContent === label)?.parentElement?.textContent ?? ''
    await waitFor(() => expect(cell('Efficiency Factor')).toBe('Efficiency FactorDonnées insuffisantes'))
    expect(cell('Impact Load (7 j)')).toBe('Impact Load (7 j)Données insuffisantes')
    expect(cell('Indice articulaire (7 j)')).toBe('Indice articulaire (7 j)Données insuffisantes')
  })

  it('renders only the sections the link allows and never shows an internal shoe id', async () => {
    const full = sampleShareView()
    const view: PublicShareView = { ...full, sections: ['shoes'], athlete: { displayName: 'Athlète' } }
    delete view.readiness
    delete view.loadSummary
    delete view.loadSeries
    delete view.cycles
    delete view.recentSessions
    delete view.bodyMap
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify(view), { status: 200, headers: { 'Content-Type': 'application/json' } })),
    )

    mounted = await mountWithProviders(<SharePublicView token="scoped" />, '/share/scoped')

    await waitFor(() => expect(document.body.textContent).toContain('Clifton 9'))
    const text = document.body.textContent ?? ''
    expect(text).toContain('Athlète')
    expect(text).not.toContain('Séances récentes')
    expect(text).not.toContain('Charge d’entraînement')
    expect(text).not.toContain('Corrélations biomécaniques')
    expect(text).not.toContain('Focus :')
    expect(document.querySelectorAll('[data-zone]')).toHaveLength(0)
  })

  it('renders a dedicated French unavailable screen when the token is revoked (404) or expired (410)', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(JSON.stringify({ error: 'Lien révoqué', code: 'NOT_FOUND' }), {
          status: 404,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    )

    mounted = await mountWithProviders(<SharePublicView token="revoked-tok" />, '/share/revoked-tok')

    await waitFor(() => {
      expect(document.body.textContent).toContain('Lien de partage introuvable, expiré ou révoqué')
    })
  })

  it('bypasses auth gating in App.tsx at /share/:token without calling /api/auth/state or redirecting to /login', async () => {
    const fetchSpy = vi.fn(async (url: string) => {
      if (url.startsWith('/api/public/share/')) {
        return new Response(JSON.stringify(sampleShareView()), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      throw new Error(`Unexpected authenticated request on public share route: ${url}`)
    })
    vi.stubGlobal('fetch', fetchSpy)

    mounted = await mountWithProviders(<App />, '/share/ext-physio-123')

    await waitFor(() => {
      expect(document.body.textContent).toContain('Camille Laurent')
      expect(document.body.textContent).toContain('Lecture seule')
    })

    expect(window.location.pathname).toBe('/share/ext-physio-123')
    const calledUrls = fetchSpy.mock.calls.map((c) => String(c[0]))
    expect(calledUrls).toEqual(['/api/public/share/ext-physio-123'])
  })
})
