// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { MorphPreview, PlannedSession } from '../../../api/types'
import { emitStreamEvent, resetEventStore } from '../../../state/events'
import { clickEl, mountWithProviders, setInputValue, waitFor, type MountedApp } from '../../../test/domHarness'
import { MorphDialog } from './MorphDialog'
import { changeLabel, isMorphCandidate } from './morphFormat'
import { setLang } from '../../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const session: PlannedSession = {
  id: 12,
  date: '2026-11-08',
  sport: 'run',
  title: 'Fractionné court 6×1′',
  targetDurationS: 2400,
  targetDistanceM: 7000,
  targetPaceMinSecPerKm: 290,
  targetPaceMaxSecPerKm: 300,
  targetCadenceSpm: 170,
  instructions: '6×1′ vite / 1′30 trot.',
  match: null,
  steps: [{ type: 'warmup', durationS: 900 }, { type: 'repeat', repeat: 6, steps: [{ type: 'interval', durationS: 60 }, { type: 'recovery', durationS: 90 }] }, { type: 'cooldown', durationS: 600 }],
  source: 'manual',
  garmin: { status: 'none', workoutId: null, pushedAt: null, error: null },
  prescribedBy: null,
  morph: null,
}
const proposal: PlannedSession = { ...session, title: 'Fractionné court → footing continu', targetDurationS: 1920, steps: [{ type: 'warmup', durationS: 600, target: { kind: 'cadence', min: 174, max: 178 } }, { type: 'run', durationS: 1020 }, { type: 'cooldown', durationS: 300 }] }
const preview: MorphPreview = {
  sessionId: 12,
  constraints: [
    { kind: 'directive', label: 'Consigne kiné', detail: 'pas de fractionné, cadence 176 spm', severity: 2 },
    { kind: 'pain', label: 'Douleur 4/10', detail: 'Rotule / Genou, gauche · hier', severity: 2 },
  ],
  original: session,
  proposal,
  changes: [
    { field: 'steps', stepIndex: null, before: 'Éch. 15′ · 6 × (1′ + 1′30 récup) · Retour 10′', after: 'Éch. 10′ · 17′ · Retour 5′', reason: 'Consigne kiné : pas de fractionné.' },
    { field: 'targetDurationS', stepIndex: null, before: '40 min', after: '32 min', reason: 'Douleur récente : volume −20 %.' },
    { field: 'steps', stepIndex: 0, before: 'Éch. 15′', after: 'Éch. 10′ @174–178 spm', reason: 'Cadence du kiné injectée dans l’étape.' },
  ],
}

describe('MorphDialog (§15.5)', () => {
  let mounted: MountedApp | null = null
  let calls: { url: string; method: string; body: Record<string, unknown> | null }[] = []

  beforeEach(() => {
    calls = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        const method = init?.method ?? 'GET'
        const body = init?.body ? JSON.parse(String(init.body)) : null
        calls.push({ url, method, body })
        const ok = (v: unknown, s = 200) => new Response(JSON.stringify(v), { status: s, headers: { 'Content-Type': 'application/json' } })
        if (url === '/api/llm') return ok({ configured: true, provider: 'gemini', model: 'gemini-2.5-flash', baseUrl: null, keyHint: '…abcd', updatedAt: null })
        if (url === '/api/plan/12/morph/refine') return ok({ id: 'job_m1', userId: 1, kind: 'llm.morph_refine', queue: 'default', state: 'running', label: 'Affinage', progressPct: 5, processed: null, total: null, attempts: 1, maxAttempts: 2, error: null, result: null, createdAt: '', updatedAt: '' }, 202)
        if (url === '/api/plan/12/morph/apply') return ok({ ...proposal, morph: { at: '2026-11-07T10:00:00Z', reasons: ['Consigne kiné', 'Douleur 4/10'], original: session } })
        return ok({})
      }),
    )
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    resetEventStore()
    vi.unstubAllGlobals()
  })

  it('labels each change by field or step and only targets today and tomorrow', () => {
    expect(changeLabel(preview.changes[1], session.steps)).toBe('Durée')
    expect(changeLabel(preview.changes[0], session.steps)).toBe('Structure')
    expect(changeLabel(preview.changes[2], session.steps)).toBe('Étape 1 · Échauffement')
    expect(isMorphCandidate(session, '2026-11-07')).toBe(true)
    expect(isMorphCandidate(session, '2026-11-05')).toBe(false)
    expect(isMorphCandidate({ ...session, sport: 'rest' }, '2026-11-08')).toBe(false)
  })

  it('shows constraints and a before/after diff, then validates and offers the .FIT', async () => {
    mounted = await mountWithProviders(<MorphDialog open session={session} preview={preview} onClose={() => undefined} />)
    await waitFor(() => expect(document.querySelector('dialog[open]')).not.toBeNull())
    const dlg = document.querySelector('dialog[open]')!
    expect(dlg.textContent).toContain('Consigne kiné')
    expect(dlg.textContent).toContain('Douleur 4/10')
    const items = Array.from(dlg.querySelectorAll('ol > li'))
    expect(items).toHaveLength(3)
    expect(items[1].textContent).toContain('Durée')
    expect(items[1].querySelector('dd')?.textContent).toBe('40 min')
    expect(items[1].querySelectorAll('dd')[1]?.textContent).toBe('32 min')
    expect(items[2].textContent).toContain('Étape 1 · Échauffement')

    await clickEl(Array.from(dlg.querySelectorAll('button')).find((b) => b.textContent?.trim() === 'Valider')!)
    await waitFor(() => expect(dlg.textContent).toContain('Séance adaptée et enregistrée'))
    expect(calls.find((c) => c.url === '/api/plan/12/morph/apply')!.body).toEqual({ proposal })
    expect(Array.from(dlg.querySelectorAll('button')).some((b) => b.textContent?.trim() === 'Télécharger .FIT')).toBe(true)
  })

  it('refines with the AI over the event stream and replaces the proposal', async () => {
    mounted = await mountWithProviders(<MorphDialog open session={session} preview={preview} onClose={() => undefined} />)
    await waitFor(() => expect(document.body.textContent).toContain('Affiner avec l’IA'))
    const dlg = document.querySelector('dialog[open]')!
    await setInputValue(dlg.querySelector('input')!, 'jambes lourdes')
    await clickEl(Array.from(dlg.querySelectorAll('button')).find((b) => b.textContent?.trim() === 'Affiner avec l’IA')!)
    await waitFor(() => expect(calls.some((c) => c.url === '/api/plan/12/morph/refine')).toBe(true))
    expect(calls.find((c) => c.url === '/api/plan/12/morph/refine')!.body).toEqual({ note: 'jambes lourdes' })
    emitStreamEvent('llm.token', { jobId: 'job_m1', kind: 'morph_refine', ref: '12', delta: 'Échauffement articulaire de 8 min.' })
    await waitFor(() => expect(dlg.textContent).toContain('Échauffement articulaire de 8 min.'))
    const refined: MorphPreview = { ...preview, changes: [...preview.changes, { field: 'instructions', stepIndex: null, before: '6×1′ vite / 1′30 trot.', after: 'Échauffement articulaire 8 min.', reason: 'Affinage IA.' }] }
    emitStreamEvent('llm.done', { jobId: 'job_m1', kind: 'morph_refine', ref: '12', morphPreview: refined })
    await waitFor(() => expect(dlg.querySelectorAll('ol > li')).toHaveLength(4))
    expect(dlg.textContent).toContain('Consignes')
  })
})
