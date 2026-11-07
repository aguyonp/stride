// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ActivitySummary } from '../../api/types'
import { addDays } from '../../lib/format'
import { clickEl, mountWithProviders, setInputValue, waitFor, type MountedApp } from '../../test/domHarness'
import { SessionPicker } from './SessionPicker'

const TODAY = '2026-11-07'

function act(id: number, startLocal: string, name: string, isRunning = false): ActivitySummary {
  return {
    id,
    garminId: null,
    stravaId: null,
    sources: ['garmin'],
    name,
    type: isRunning ? 'running' : 'cycling',
    typeLabel: isRunning ? 'Course' : 'Vélo',
    sport: isRunning ? 'run' : 'bike',
    isRunning,
    startLocal,
    date: startLocal.slice(0, 10),
    distanceM: isRunning ? 42195 : 6000,
    durationS: isRunning ? 11100 : 1300,
    avgPaceSecPerKm: null,
    gapSecPerKm: null,
    avgHR: null,
    avgPower: null,
    avgCadenceSpm: null,
    elevGainM: null,
    elevLossM: null,
    trainingLoad: null,
    painDuring: null,
    hasNotes: false,
    planStatus: null,
  }
}

// April 2025 holds more activities than one page can carry: 7 commutes a day after the 14th, the marathon on the 14th
// itself, so it only comes back on the second page of the month.
function history(): ActivitySummary[] {
  const out: ActivitySummary[] = [act(1, `${TODAY}T08:10:00`, 'Vélotaf du jour')]
  let id = 10
  for (let d = '2025-04-15'; d <= '2025-04-30'; d = addDays(d, 1)) {
    for (const h of ['06', '07', '08', '12', '17', '18', '19']) out.push(act(id++, `${d}T${h}:00:00`, 'Vélotaf'))
  }
  out.push(act(500, '2025-04-14T08:45:00', 'Marathon', true))
  return out.sort((a, b) => (a.startLocal < b.startLocal ? 1 : -1))
}

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

/** Mirrors backend/internal/domainapi listActivities: limit 1..100, from/to on the local date, before on startLocal. */
function fakeActivities(url: URL) {
  const q = url.searchParams
  const limit = Number(q.get('limit') ?? 30)
  if (!Number.isInteger(limit) || limit < 1 || limit > 100) return json({ error: 'limit doit être un entier entre 1 et 100.', code: 'BAD_REQUEST' }, 400)
  const before = q.get('before')
  const from = q.get('from')
  const to = q.get('to')
  const rows = history().filter((a) => (!before || a.startLocal < before) && (!from || a.date >= from) && (!to || a.date <= to))
  return json({ items: rows.slice(0, limit), hasMore: rows.length > limit, freshness: { source: 'db', updatedAt: `${TODAY}T08:00:00Z` } })
}

describe('SessionPicker calendar: far month jump', () => {
  let mounted: MountedApp | null = null
  let requests: URL[] = []

  beforeEach(() => {
    requests = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string) => {
        const url = new URL(input, 'http://stride.test')
        requests.push(url)
        if (url.pathname === '/api/activities') return fakeActivities(url)
        return json({ error: 'introuvable', code: 'NOT_FOUND' }, 404)
      }),
    )
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
  })

  const q = <T extends Element>(sel: string) => document.querySelector<T>(sel)
  const cell = (iso: string) => q<HTMLButtonElement>(`[data-date="${iso}"]`)

  async function openCalendar(onSelect = vi.fn()) {
    mounted = await mountWithProviders(<SessionPicker open onClose={() => undefined} selectedId={1} selectedDate={TODAY} today={TODAY} onSelect={onSelect} />)
    const tab = [...document.querySelectorAll<HTMLElement>('[role="radio"]')].find((el) => el.textContent?.includes('Calendrier'))
    await clickEl(tab ?? null)
    await waitFor(() => expect(cell(TODAY)).not.toBeNull())
    return onSelect
  }

  it('shows 14 April 2025 and its marathon right after choosing April 2025, within the server page cap', async () => {
    const onSelect = await openCalendar()

    await setInputValue(q<HTMLSelectElement>('select[aria-label="Année"]')!, '2025')
    await setInputValue(q<HTMLSelectElement>('select[aria-label="Mois"]')!, '3')

    await waitFor(() => expect(cell('2025-04-14')?.getAttribute('aria-label')).toBe('Lundi 14 avril 2025, 1 activité dont 1 course'))
    expect(q('[role="grid"]')?.getAttribute('aria-label')).toBe('Avril 2025')
    expect(q('[role="alert"]')).toBeNull()

    const monthCalls = requests.filter((u) => u.pathname === '/api/activities' && u.searchParams.get('from') === '2025-04-01')
    expect(monthCalls.length).toBeGreaterThan(1)
    for (const u of requests) expect(Number(u.searchParams.get('limit'))).toBeLessThanOrEqual(100)
    expect(monthCalls.every((u) => u.searchParams.get('to') === '2025-04-30')).toBe(true)

    await clickEl(cell('2025-04-14'))
    expect(cell('2025-04-14')?.getAttribute('aria-selected')).toBe('true')
    const marathon = [...document.querySelectorAll('li button')].find((b) => b.textContent?.includes('Marathon'))
    expect(marathon).toBeDefined()
    await clickEl(marathon ?? null)
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 500, date: '2025-04-14' }))
  })

  it('returns to the current month with « Aujourd’hui » and keeps prev/next working', async () => {
    await openCalendar()
    const todayBtn = () => [...document.querySelectorAll<HTMLButtonElement>('button')].find((b) => b.textContent === 'Aujourd’hui')
    expect(todayBtn()?.disabled).toBe(true)

    await setInputValue(q<HTMLSelectElement>('select[aria-label="Année"]')!, '2025')
    await setInputValue(q<HTMLSelectElement>('select[aria-label="Mois"]')!, '3')
    await waitFor(() => expect(cell('2025-04-14')).not.toBeNull())
    await clickEl(q('button[aria-label="Mois suivant"]'))
    expect(q('[role="grid"]')?.getAttribute('aria-label')).toBe('Mai 2025')
    await clickEl(q('button[aria-label="Mois précédent"]'))
    await waitFor(() => expect(cell('2025-04-14')?.getAttribute('aria-label')).toContain('1 activité'))

    expect(todayBtn()?.disabled).toBe(false)
    await clickEl(todayBtn() ?? null)
    expect(q('[role="grid"]')?.getAttribute('aria-label')).toBe('Novembre 2026')
    expect(cell(TODAY)?.getAttribute('aria-selected')).toBe('true')
    await waitFor(() => expect([...document.querySelectorAll('li button')].some((b) => b.textContent?.includes('Vélotaf du jour'))).toBe(true))
  })

  it('surfaces a month that fails to load instead of an empty grid', async () => {
    await openCalendar()
    vi.mocked(fetch).mockImplementation(async () => json({ error: 'boom', code: 'INTERNAL' }, 500))
    await clickEl(q('button[aria-label="Mois précédent"]'))
    await waitFor(() => expect(q('[role="alert"]')?.textContent).toContain('Impossible de charger les activités de ce mois.'))
  })
})
