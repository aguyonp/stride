// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { TrainingCycleOverview } from '../../api/types'
import { clickEl, installDomPolyfills, mountWithProviders, waitFor, type MountedApp } from '../../test/domHarness'
import { CyclesTimeline } from './CyclesTimeline'
import { setLang } from '../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

function sampleCycles(): TrainingCycleOverview {
  return {
    plans: [
      {
        id: 1,
        goalId: 5,
        name: 'Macrocycle Marathon de Printemps',
        sport: 'run',
        startDate: '2026-10-19',
        endDate: '2026-12-13',
        targetEventLabel: 'Marathon · 3h15',
        status: 'active',
        note: 'Progression 3:1 maîtrisée',
        createdAt: '2026-10-15T10:00:00Z',
        updatedAt: '2026-11-06T10:00:00Z',
      },
    ],
    activePlan: {
      id: 1,
      goalId: 5,
      name: 'Macrocycle Marathon de Printemps',
      sport: 'run',
      startDate: '2026-10-19',
      endDate: '2026-12-13',
      targetEventLabel: 'Marathon · 3h15',
      status: 'active',
      note: 'Progression 3:1 maîtrisée',
      createdAt: '2026-10-15T10:00:00Z',
      updatedAt: '2026-11-06T10:00:00Z',
    },
    phases: [
      {
        id: 11,
        planId: 1,
        name: 'Fondation aérobie',
        phaseType: 'foundation',
        orderIndex: 0,
        startDate: '2026-10-19',
        endDate: '2026-11-15',
        focus: 'Volume souple & côtes courtes',
        weeklyVolumeKm: 52,
        targetWeeklyLoad: 390,
        targetWeeklyDurationS: 16800,
        note: '',
        actuals: {
          plannedVolumeKm: 195,
          actualVolumeKm: 162,
          volumeCompletionPct: 83.1,
          plannedLoad: 1460,
          actualLoad: 1210,
          loadCompletionPct: 82.9,
          plannedDurationS: 64000,
          actualDurationS: 53000,
          sessionsPlanned: 16,
          sessionsCompleted: 13,
        },
        weeks: [
          {
            id: 101,
            planId: 1,
            phaseId: 11,
            weekStart: '2026-10-26',
            weekEnd: '2026-11-01',
            weekNumber: 2,
            isAssimilation: false,
            targetVolumeKm: 54,
            targetLoad: 405,
            targetDurationS: 17200,
            note: '',
            actuals: {
              plannedVolumeKm: 54,
              actualVolumeKm: 53.2,
              volumeCompletionPct: 98.5,
              plannedLoad: 405,
              actualLoad: 398,
              loadCompletionPct: 98.3,
              plannedDurationS: 17200,
              actualDurationS: 17050,
              sessionsPlanned: 4,
              sessionsCompleted: 4,
            },
            workouts: [],
          },
          {
            id: 102,
            planId: 1,
            phaseId: 11,
            weekStart: '2026-11-09',
            weekEnd: '2026-11-15',
            weekNumber: 4,
            isAssimilation: true,
            targetVolumeKm: 38,
            targetLoad: 280,
            targetDurationS: 12200,
            note: 'Semaine d’assimilation (-25 %)',
            actuals: {
              plannedVolumeKm: 38,
              actualVolumeKm: 20,
              volumeCompletionPct: 52.6,
              plannedLoad: 280,
              actualLoad: 150,
              loadCompletionPct: 53.6,
              plannedDurationS: 12200,
              actualDurationS: 6400,
              sessionsPlanned: 3,
              sessionsCompleted: 2,
            },
            workouts: [],
          },
        ],
      },
      {
        id: 12,
        planId: 1,
        name: 'Spécifique allure marathon',
        phaseType: 'specific',
        orderIndex: 1,
        startDate: '2026-11-16',
        endDate: '2026-12-13',
        focus: 'Blocs allure cible',
        weeklyVolumeKm: 60,
        targetWeeklyLoad: 450,
        targetWeeklyDurationS: 19200,
        note: '',
        actuals: {
          plannedVolumeKm: 220,
          actualVolumeKm: 0,
          volumeCompletionPct: 0,
          plannedLoad: 1680,
          actualLoad: 0,
          loadCompletionPct: 0,
          plannedDurationS: 72000,
          actualDurationS: 0,
          sessionsPlanned: 16,
          sessionsCompleted: 0,
        },
        weeks: [],
      },
    ],
    standaloneWeeks: [],
    totals: {
      plannedVolumeKm: 415,
      actualVolumeKm: 162,
      volumeCompletionPct: 39.0,
      plannedLoad: 3140,
      actualLoad: 1210,
      loadCompletionPct: 38.5,
      plannedDurationS: 136000,
      actualDurationS: 53000,
      sessionsPlanned: 32,
      sessionsCompleted: 13,
    },
  }
}

describe('CyclesTimeline', () => {
  let mounted: MountedApp | null = null

  beforeEach(() => {
    installDomPolyfills()
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(JSON.stringify(sampleCycles()), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    )
  })

  afterEach(async () => {
    if (mounted) {
      await mounted.unmount()
      mounted = null
    }
    vi.unstubAllGlobals()
  })

  it('renders Macrocycle, Mesocycle Gantt blocks, assimilation week badges, and switches zoom levels', async () => {
    const onJumpToWeek = vi.fn()
    mounted = await mountWithProviders(
      <CyclesTimeline today="2026-11-06" goals={[]} onJumpToWeek={onJumpToWeek} />,
      '/plan',
    )

    await waitFor(() => {
      expect(document.body.textContent).toContain('Macrocycle Marathon de Printemps')
      expect(document.body.textContent).toContain('Fondation aérobie')
      expect(document.body.textContent).toContain('Spécifique allure marathon')
      expect(document.body.textContent).toContain('Semaine d’assimilation')
    })

    // Switch zoom to Microcycles and verify week details + jump callback
    const microTab = Array.from(document.querySelectorAll('button')).find(
      (b) => b.textContent?.trim() === 'Microcycles',
    )
    expect(microTab).toBeTruthy()
    await clickEl(microTab!)

    await waitFor(() => {
      expect(document.body.textContent).toContain('Semaine d’assimilation (-25 %)')
    })

    const calendarJumpBtn = Array.from(document.querySelectorAll('button')).find((b) =>
      b.getAttribute('aria-label')?.includes('Ouvrir la semaine'),
    )
    if (calendarJumpBtn) {
      await clickEl(calendarJumpBtn)
      expect(onJumpToWeek).toHaveBeenCalled()
    }
  })
})
