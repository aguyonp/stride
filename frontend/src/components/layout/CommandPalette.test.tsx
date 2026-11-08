// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Me } from '../../api/types'
import { ui } from '../../state/ui'
import { clickEl, installDomPolyfills, keyDownEl, mountWithProviders, setInputValue, waitFor, type MountedApp } from '../../test/domHarness'
import { CommandPalette } from './CommandPalette'
import { setLang } from '../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const ADMIN_ME: Me = {
  id: 1,
  displayName: 'Camille Laurent',
  role: 'admin',
  createdAt: '2026-01-10T10:00:00Z',
  onboardingCompleted: true,
}

describe('CommandPalette', () => {
  let mounted: MountedApp | null = null

  beforeEach(() => {
    installDomPolyfills()
    ui.reset()
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
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
        if (url.startsWith('/api/search')) {
          const hit = { id: 7, date: '2019-04-14', startLocal: '2019-04-14T08:30:00', name: 'Marathon de Paris', sport: 'run', typeLabel: 'Course', distanceM: 42195, durationS: 12600 }
          return new Response(JSON.stringify({ query: '', items: [hit] }), { status: 200, headers: { 'Content-Type': 'application/json' } })
        }
        if (url.startsWith('/api/activities')) {
          return new Response(
            JSON.stringify({
              items: [
                {
                  id: 42,
                  date: '2026-11-04',
                  startLocal: '2026-11-04T18:15:00',
                  name: 'Seuil lactique Parc Borély',
                  type: 'running',
                  typeLabel: 'Course',
                  sport: 'run',
                  isRunning: true,
                  sources: ['garmin'],
                  distanceM: 11200,
                  durationS: 3120,
                  avgPaceSecPerKm: 278,
                  avgHR: 164,
                  avgCadenceSpm: 176,
                  elevGainM: 45,
                  elevLossM: 45,
                  trainingLoad: 118,
                  rpe: 7,
                  painMax: 1,
                  matchStatus: 'done',
                  alertCount: 0,
                  dangerCount: 0,
                  kudos: 4,
                },
              ],
              hasMore: false,
              freshness: { source: 'cache', updatedAt: '2026-11-06T12:00:00Z' },
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          )
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
    ui.reset()
    vi.unstubAllGlobals()
  })

  it('opens on Ctrl+K, filters commands and sessions, navigates with ArrowDown + Enter, and closes on Escape', async () => {
    mounted = await mountWithProviders(<CommandPalette me={ADMIN_ME} />, '/')

    expect(document.querySelector('[role="dialog"]')).toBeNull()

    await keyDownEl(window, 'k', { ctrlKey: true })

    await waitFor(() => {
      const dialog = document.querySelector('[role="dialog"]')
      expect(dialog).not.toBeNull()
      expect(dialog?.getAttribute('aria-modal')).toBe('true')
    })

    const input = document.querySelector<HTMLInputElement>('input[role="combobox"]')!
    expect(input).toBeTruthy()

    // Filter to the health navigation entry (old « cartographie » wording still matches)
    await setInputValue(input, 'cartographie')
    await waitFor(() => {
      const options = Array.from(document.querySelectorAll('[role="option"]'))
      expect(options.some((o) => o.textContent?.includes('Santé & forme'))).toBe(true)
    })

    // Navigate down to "Santé & forme" (after the quick action when both match)
    const optionsAfterFilter = Array.from(document.querySelectorAll('[role="option"]'))
    const navIdx = optionsAfterFilter.findIndex((o) => o.textContent?.includes('Santé & forme'))
    expect(navIdx).toBeGreaterThanOrEqual(0)
    for (let i = 0; i < navIdx; i++) {
      await keyDownEl(input, 'ArrowDown')
    }
    await keyDownEl(input, 'Enter')

    expect(window.location.pathname).toBe('/health')
    expect(document.querySelector('[role="dialog"]')).toBeNull()

    // Re-open with Meta+K, search for an activity by title, and jump to /activities/42
    await keyDownEl(window, 'k', { metaKey: true })
    const input2 = document.querySelector<HTMLInputElement>('input[role="combobox"]')!
    await waitFor(() => {
      expect(document.body.textContent).toContain('Seuil lactique Parc Borély')
    })
    await setInputValue(input2, 'Borély')
    await waitFor(() => {
      const opts = document.querySelectorAll('[role="option"]')
      expect(opts.length).toBe(1)
      expect(opts[0].textContent).toContain('Seuil lactique Parc Borély')
    })
    await keyDownEl(input2, 'Enter')
    expect(window.location.pathname).toBe('/activities/42')

    // Whole-history search (GET /api/search) reaches sessions outside the loaded pages
    await keyDownEl(window, 'k', { metaKey: true })
    const input3 = document.querySelector<HTMLInputElement>('input[role="combobox"]')!
    await setInputValue(input3, '14 avril')
    await waitFor(() => {
      const opts = Array.from(document.querySelectorAll('[role="option"]'))
      expect(opts.some((o) => o.textContent?.includes('Marathon de Paris'))).toBe(true)
    })
    await keyDownEl(input3, 'Escape')

    // Re-open and close with Escape
    await keyDownEl(window, 'k', { ctrlKey: true })
    expect(document.querySelector('[role="dialog"]')).not.toBeNull()
    const closeInput = document.querySelector<HTMLInputElement>('input[role="combobox"]')!
    await keyDownEl(closeInput, 'Escape')
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })

  it('triggers quick actions such as manual session creation', async () => {
    mounted = await mountWithProviders(<CommandPalette me={ADMIN_ME} />, '/')
    await keyDownEl(window, 'k', { ctrlKey: true })

    await waitFor(() => {
      expect(document.querySelector('[role="dialog"]')).not.toBeNull()
    })

    const manualBtn = Array.from(document.querySelectorAll<HTMLButtonElement>('[role="option"]')).find((b) =>
      b.textContent?.includes('Créer une séance manuelle'),
    )
    expect(manualBtn).toBeTruthy()
    await clickEl(manualBtn!)
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })
})
