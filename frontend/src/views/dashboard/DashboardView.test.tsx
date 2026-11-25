// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ActivitySummary, Me } from '../../api/types'
import { db, resetDb } from '../../mocks/db'
import { setFlags } from '../../mocks/flags'
import { mockFetch } from '../../mocks/server'
import { clickEl, installDomPolyfills, mountWithProviders, waitFor, type MountedApp } from '../../test/domHarness'
import { DashboardView } from './DashboardView'
import { setLang } from '../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const PREV = 'button[aria-label="Séance précédente (plus ancienne)"]'
const NEXT = 'button[aria-label="Séance suivante (plus récente)"]'

const me = { id: 1, displayName: 'Alex Démo', role: 'admin' } as Me

/** The mock backend is the fake server of `npm run dev:mock`: same payloads, no network. */
async function history(): Promise<ActivitySummary[]> {
  const out: ActivitySummary[] = []
  let before = ''
  for (let page = 0; page < 20; page++) {
    const qs = `limit=100${before ? `&before=${encodeURIComponent(before)}` : ''}`
    const res = await mockFetch(`/api/activities?${qs}`, { method: 'GET' })
    const body = (await res.json()) as { items: ActivitySummary[]; hasMore: boolean }
    out.push(...body.items)
    if (!body.hasMore || body.items.length === 0) break
    before = body.items[body.items.length - 1].startLocal
  }
  return out
}

const rangeCalls = (calls: string[]) => calls.filter((c) => c.startsWith('/api/activities') && /[?&](from|to)=/.test(c))

describe('DashboardView session navigation (issue #21)', () => {
  let mounted: MountedApp | null = null
  let calls: string[] = []

  beforeEach(async () => {
    installDomPolyfills()
    setFlags([])
    resetDb()
    calls = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string, init?: RequestInit) => {
        calls.push(String(input))
        return mockFetch(String(input), init)
      }),
    )
    // The stub must be in place before the page is fetched below.
    await history()
    calls = []
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
    resetDb()
  })

  const mountAt = async (url: string) => {
    mounted = await mountWithProviders(<DashboardView me={me} />, url)
    return mounted
  }

  const button = (sel: string) => document.querySelector<HTMLButtonElement>(sel)

  it('enables both arrows for a session outside the loaded page and jumps to its neighbours', async () => {
    const items = await history()
    // Beyond the first page of the dashboard list (PAGE_SIZE), and not the oldest of the history.
    const selected = items[40]
    const newer = items[39]
    const older = items[41]

    const m = await mountAt(`/?s=${selected.id}`)

    // The window around the session date is what makes the neighbours known.
    await waitFor(() => {
      expect(button(PREV)?.disabled).toBe(false)
      expect(button(NEXT)?.disabled).toBe(false)
    })
    expect(rangeCalls(calls).length).toBeGreaterThan(0)
    expect(m.container.textContent).toContain(selected.name)

    await clickEl(button(PREV))
    await waitFor(() => expect(window.location.search).toBe(`?s=${older.id}`))

    await waitFor(() => expect(button(NEXT)?.disabled).toBe(false))
    await clickEl(button(NEXT))
    await waitFor(() => expect(window.location.search).toBe(`?s=${selected.id}`))
    expect(newer.id).not.toBe(older.id)
  })

  it('keeps the arrow disabled at the oldest session of the history', async () => {
    const items = await history()
    const oldest = items[items.length - 1]

    await mountAt(`/?s=${oldest.id}`)

    await waitFor(() => expect(button(NEXT)?.disabled).toBe(false))
    // Nothing older exists: the widening passes end and the arrow stays disabled.
    expect(button(PREV)?.disabled).toBe(true)
  })

  it('does not query a window for a session already in the list (nominal case)', async () => {
    const items = await history()
    const selected = items[2]

    await mountAt(`/?s=${selected.id}`)

    await waitFor(() => {
      expect(button(PREV)?.disabled).toBe(false)
      expect(button(NEXT)?.disabled).toBe(false)
    })
    expect(rangeCalls(calls)).toEqual([])

    await clickEl(button(NEXT))
    await waitFor(() => expect(window.location.search).toBe(`?s=${items[1].id}`))
    expect(db().users.length).toBeGreaterThan(0)
  })
})
