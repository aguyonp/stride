// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { PlannedSession, PublicShareView, ShareNote, SharePermissions } from '../../../api/types'
import { addDays, todayISO } from '../../../lib/format'
import { clickEl, installDomPolyfills, mountWithProviders, setInputValue, waitFor, type MountedApp } from '../../../test/domHarness'
import SharePublicView from '../SharePublicView'
import { setLang } from '../../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const today = todayISO()
const note = (over: Partial<ShareNote>): ShareNote => ({
  id: 1,
  kind: 'remark',
  linkId: 42,
  linkLabel: 'Kiné du sport',
  authorName: 'Claire Martin',
  authorRole: 'physio',
  body: 'Bonne gestion de la descente.',
  activityId: 502,
  validUntil: null,
  restrictions: null,
  createdAt: '2026-11-05T10:00:00Z',
  updatedAt: '2026-11-05T10:00:00Z',
  ...over,
})
const session = (id: number, title: string, prescribed: boolean): PlannedSession => ({
  id,
  date: addDays(today, 2),
  sport: 'run',
  title,
  targetDurationS: 2400,
  targetDistanceM: null,
  targetPaceMinSecPerKm: null,
  targetPaceMaxSecPerKm: null,
  targetCadenceSpm: null,
  instructions: '',
  match: null,
  steps: [],
  source: prescribed ? 'share' : 'manual',
  garmin: { status: 'none', workoutId: null, pushedAt: null, error: null },
  prescribedBy: prescribed ? { linkId: 42, label: 'Kiné du sport', authorName: 'Claire Martin' } : null,
  morph: null,
})
const item = (id: number, name: string, remark: ShareNote | null) => ({
  id,
  date: addDays(today, -1),
  startLocal: `${addDays(today, -1)}T18:00:00`,
  name,
  sport: 'run',
  typeLabel: 'Course',
  distanceM: 8000,
  durationS: 2700,
  avgPaceSecPerKm: 337,
  gapSecPerKm: null,
  avgHR: 142,
  avgCadenceSpm: 174,
  gctMs: 248,
  elevGainM: 30,
  elevLossM: 32,
  trainingLoad: 70,
  impactLoad: null,
  jointIndex: null,
  efSpeed: null,
  shoeName: 'Clifton 9',
  shoeDropMm: 5,
  remark,
})

function view(permissions?: SharePermissions): PublicShareView {
  return {
    generatedAt: '2026-11-06T18:00:00Z',
    linkId: 42,
    shareLabel: 'Kiné du sport',
    expiresAt: null,
    periodWeeks: 12,
    sections: ['activities', 'plan'],
    athlete: { displayName: 'Camille Laurent' },
    recentSessions: [item(501, 'Footing vallonné', null), item(502, 'Sortie longue', note({}))],
    ...(permissions ? { permissions, directive: note({ id: 2, kind: 'directive', activityId: null, body: 'Pas de fractionné.' }) } : {}),
    ...(permissions?.planning ? { calendar: { planned: [session(801, 'Renfo excentrique (kiné)', true), session(802, 'Fractionné court', false)], events: [] } } : {}),
  }
}

const buttons = (re: RegExp) => Array.from(document.querySelectorAll<HTMLButtonElement>('button')).filter((b) => re.test(b.getAttribute('aria-label') ?? b.textContent?.trim() ?? ''))
const byText = (label: string, root: ParentNode = document) => Array.from(root.querySelectorAll<HTMLButtonElement>('button')).find((b) => b.textContent?.trim() === label) ?? null

describe('Share portal (§15.1)', () => {
  let mounted: MountedApp | null = null
  let writes: { url: string; method: string; body: Record<string, unknown> }[] = []
  let payload: PublicShareView

  const stubFetch = () =>
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        const method = init?.method ?? 'GET'
        const ok = (v: unknown, s = 200) => new Response(JSON.stringify(v), { status: s, headers: { 'Content-Type': 'application/json' } })
        if (method === 'GET') return ok(payload)
        const body = init?.body ? JSON.parse(String(init.body)) : {}
        writes.push({ url, method, body })
        if (url.endsWith('/plan')) return ok({ ...session(900, String(body.title), true), ...body }, 201)
        return ok(note({ ...body, kind: url.endsWith('/directive') ? 'directive' : 'remark' }))
      }),
    )
  const setWide = (wide: boolean) => {
    window.matchMedia = ((query: string) => ({ matches: wide && query.includes('min-width'), media: query, onchange: null, addListener() {}, removeListener() {}, addEventListener() {}, removeEventListener() {}, dispatchEvent: () => false })) as unknown as typeof window.matchMedia
  }

  beforeEach(() => {
    installDomPolyfills()
    writes = []
    try {
      window.localStorage.clear()
    } catch {
      /* no storage */
    }
    stubFetch()
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    setWide(false)
    vi.unstubAllGlobals()
  })

  it('stays read only without permissions: no remark, prescription, directive nor detail controls', async () => {
    payload = view()
    setWide(true)
    mounted = await mountWithProviders(<SharePublicView token="tok" />, '/share/tok')
    await waitFor(() => expect(document.body.textContent).toContain('Footing vallonné'))
    const text = document.body.textContent ?? ''
    expect(text).toContain('Lecture seule')
    expect(text).not.toContain('automatiquement prises en compte')
    expect(text).not.toContain('Planning')
    expect(buttons(/Remarque|Prescrire|Rédiger une consigne|^Détail|Modifier/)).toHaveLength(0)
    // The existing remark stays visible, read only
    expect(text).toContain('Bonne gestion de la descente.')
  })

  it('saves a quick remark from the mobile bottom sheet with the remembered signature', async () => {
    payload = view({ comments: true, planning: false, activityDetails: false })
    setWide(false)
    mounted = await mountWithProviders(<SharePublicView token="tok" />, '/share/tok')
    await waitFor(() => expect(document.body.textContent).toContain('Annotations autorisées'))
    expect(document.body.textContent).toContain('Vos remarques sont automatiquement prises en compte par l’assistant d’entraînement et la planification de l’athlète.')
    expect(buttons(/^Détail/)).toHaveLength(0)
    expect(buttons(/Prescrire/)).toHaveLength(0)

    await clickEl(buttons(/^Remarque : Footing vallonné/)[0])
    await waitFor(() => expect(document.querySelector('dialog[open] textarea')).not.toBeNull())
    const dlg = document.querySelector('dialog[open]')!
    await setInputValue(dlg.querySelector<HTMLInputElement>('input[autocomplete="name"]')!, 'Claire Martin')
    await setInputValue(dlg.querySelector('textarea')!, '  Cadence correcte, garder le plat.  ')
    await clickEl(byText('Enregistrer', dlg))
    await waitFor(() => expect(writes).toHaveLength(1))
    expect(writes[0]).toEqual({ url: '/api/public/share/tok/activities/501/remark', method: 'PUT', body: { authorName: 'Claire Martin', authorRole: 'physio', body: 'Cadence correcte, garder le plat.' } })
  })

  it('edits the directive inline on desktop', async () => {
    payload = view({ comments: true, planning: false, activityDetails: false })
    setWide(true)
    mounted = await mountWithProviders(<SharePublicView token="tok" />, '/share/tok')
    await waitFor(() => expect(document.body.textContent).toContain('Consigne en cours'))
    await clickEl(byText('Modifier'))
    await waitFor(() => expect(document.querySelector('section[aria-label="Modifier la consigne"] textarea')).not.toBeNull())
    expect(document.querySelector('dialog[open]')).toBeNull()
    const form = document.querySelector('section[aria-label="Modifier la consigne"]')!
    await setInputValue(form.querySelector('textarea')!, 'Footings 30 min max.')
    await clickEl(byText('Enregistrer', form))
    await waitFor(() => expect(writes).toHaveLength(1))
    expect(writes[0].url).toBe('/api/public/share/tok/directive')
    expect(writes[0].body).toMatchObject({ body: 'Footings 30 min max.', restrictions: { noIntervals: false, maxIntensity: null, maxElevLossM: null, cadenceTargetSpm: null } })
  })

  it('prescribes a session inline and only lets the link edit what it prescribed', async () => {
    payload = view({ comments: false, planning: true, activityDetails: false })
    setWide(true)
    mounted = await mountWithProviders(<SharePublicView token="tok" />, '/share/tok')
    await waitFor(() => expect(document.body.textContent).toContain('Renfo excentrique (kiné)'))
    expect(document.body.textContent).toContain('Prescrit par Kiné du sport (Claire Martin)')
    expect(buttons(/^Modifier la séance Renfo excentrique/)).toHaveLength(1)
    expect(buttons(/^Modifier la séance Fractionné court/)).toHaveLength(0)
    expect(buttons(/^Remarque/)).toHaveLength(0)

    await clickEl(byText('Prescrire'))
    await waitFor(() => expect(document.querySelector('input[name="title"]')).not.toBeNull())
    expect(document.querySelector('dialog[open]')).toBeNull()
    await setInputValue(document.querySelector<HTMLInputElement>('input[name="title"]')!, 'Footing souple 35′')
    await clickEl(byText('Prescrire la séance'))
    await waitFor(() => expect(writes).toHaveLength(1))
    expect(writes[0].url).toBe('/api/public/share/tok/plan')
    expect(writes[0].body).toMatchObject({ title: 'Footing souple 35′', sport: 'run', date: addDays(today, 1) })
  })
})
