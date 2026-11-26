// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { db, resetDb, userData } from '../../mocks/db'
import { setFlags } from '../../mocks/flags'
import { mockFetch } from '../../mocks/server'
import { clickEl, installDomPolyfills, mountWithProviders, waitFor, type MountedApp } from '../../test/domHarness'
import { setLang } from '../../i18n/lang'
import { PushTodayButton } from './PushTodayButton'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const text = () => document.body.textContent ?? ''
const btn = (label: string) => [...document.querySelectorAll('button')].find((b) => b.textContent?.includes(label)) ?? null
/** The dialog has its own « Envoyer », the page has the trigger: scope the click. */
const inDialog = (label: string) =>
  [...(document.querySelector('dialog')?.querySelectorAll('button') ?? [])].find((b) => b.textContent?.includes(label)) ?? null

describe('PushTodayButton (séances du jour)', () => {
  let mounted: MountedApp | null = null

  beforeEach(() => {
    installDomPolyfills()
    setFlags([])
    resetDb()
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string, init?: RequestInit) => mockFetch(String(input), init)),
    )
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
    resetDb()
  })

  const today = () => new Date().toISOString().slice(0, 10)

  it('stays out of the way without a connected bridge', async () => {
    mounted = await mountWithProviders(<PushTodayButton date={today()} />)
    await new Promise((r) => setTimeout(r, 120))
    expect(btn('Envoyer les séances du jour')).toBeNull()
  })

  it('sends the sessions of the day and reports each one', async () => {
    userData(1).garminBridge = { state: 'connected', displayName: 'Antoine' }
    db().data['1'] = userData(1)
    // One session with steps today, one without, one already pushed.
    const data = userData(1)
    const template = data.plan[0]
    data.plan.push(
      { ...template, id: 9001, date: today(), title: 'Footing du jour', steps: [{ type: 'warmup', durationS: 600 }], garmin: { status: 'none', workoutId: null, pushedAt: null, error: null } },
      { ...template, id: 9002, date: today(), title: 'Sans structure', steps: [], garmin: { status: 'none', workoutId: null, pushedAt: null, error: null } },
      { ...template, id: 9003, date: today(), title: 'Déjà envoyée', steps: [{ type: 'warmup', durationS: 600 }], garmin: { status: 'pushed', workoutId: 7000, pushedAt: null, error: null } },
    )
    db().data['1'] = data

    mounted = await mountWithProviders(<PushTodayButton date={today()} />)
    await waitFor(() => expect(btn('Envoyer les séances du jour')).not.toBeNull())
    await clickEl(btn('Envoyer les séances du jour'))
    await waitFor(() => expect(text()).toContain('Les séances de'))
    await clickEl(inDialog('Envoyer'))

    await waitFor(() => expect(text()).toContain('Footing du jour'))
    // The per-session summary is what matters: sent, skipped (no steps), skipped (already sent).
    expect(text()).toContain('Pas d’étapes structurées à envoyer.')
    expect(text()).toContain('Déjà envoyée.')
    const sent = userData(1).plan.filter((p) => p.garmin?.status === 'pushed' && p.id === 9001)
    expect(sent.length).toBe(1)
    expect(sent[0].garmin?.workoutId).toBe(18001)
  })
})
