// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { HealthStatus, HealthStatusInput } from '../../api/types'
import { conditionCatalog, conditionLabelOf } from '../../mocks/healthConditions'
import { clickEl, installDomPolyfills, mountWithProviders, setInputValue, waitFor, type MountedApp } from '../../test/domHarness'
import { HealthStatusCard } from './HealthStatusCard'
import { setLang } from '../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const ok = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
const button = (text: string) => Array.from(document.querySelectorAll('button')).find((b) => b.textContent?.trim() === text) ?? null
const select = (label: string) => document.querySelector<HTMLSelectElement>(`select[aria-label="${label}"]`)

describe('HealthStatusCard pathology picker', () => {
  let mounted: MountedApp | null = null
  let status: HealthStatus
  let puts: HealthStatusInput[] = []

  beforeEach(() => {
    installDomPolyfills()
    puts = []
    status = { mode: 'tracking', zones: [{ zone: 'ankle', side: 'left', since: '2026-10-12', note: '' }], focusArea: 'other', updatedAt: null }
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (url.startsWith('/api/health/conditions')) return ok(conditionCatalog('fr'))
        if (url.startsWith('/api/health/status') && init?.method === 'PUT') {
          const body = JSON.parse(String(init.body)) as HealthStatusInput
          puts.push(body)
          status = { ...body, zones: body.zones.map((z) => ({ ...z, conditionLabel: conditionLabelOf(z) })), focusArea: 'other', updatedAt: '2026-11-07T10:00:00Z' }
          return ok(status)
        }
        if (url.startsWith('/api/health/status')) return ok(status)
        return ok({})
      }),
    )
  })

  afterEach(async () => {
    if (mounted) await mounted.unmount()
    mounted = null
    vi.unstubAllGlobals()
  })

  it('tracks a catalog condition and a custom one, requiring the custom text', async () => {
    mounted = await mountWithProviders(<HealthStatusCard today="2026-11-07" />)
    await waitFor(() => expect(button('Modifier')).not.toBeNull())
    await clickEl(button('Modifier'))

    // zone → side → condition: the ankle offers its own pathologies, « Autre » last
    await waitFor(() => expect(select('Pathologie (Cheville)')?.disabled).toBe(false))
    const ankle = select('Pathologie (Cheville)')!
    const labels = Array.from(ankle.options).map((o) => o.textContent)
    expect(labels).toContain('Fracture de cheville (malléole)')
    expect(labels).not.toContain('Lésion méniscale')
    expect(labels.at(-1)).toBe('Autre (préciser)')
    await setInputValue(ankle, 'ankle_fracture')
    expect(document.querySelector('input[aria-label="Pathologie (préciser)"]')).toBeNull()

    await clickEl(Array.from(document.querySelectorAll('button')).find((b) => b.textContent?.includes('Ajouter une zone')) ?? null)
    await waitFor(() => expect(select('Pathologie (Rotule / Genou)')).not.toBeNull())
    await setInputValue(select('Pathologie (Rotule / Genou)')!, 'other')
    const custom = document.querySelector<HTMLInputElement>('input[aria-label="Pathologie (préciser)"]')!
    expect(custom.maxLength).toBe(80)

    await clickEl(button('Enregistrer'))
    await waitFor(() => expect(document.body.textContent).toContain('Précise la pathologie suivie (Rotule / Genou).'))
    expect(puts).toHaveLength(0)

    await setInputValue(custom, '  Plica <b>synoviale</b>  ')
    await clickEl(button('Enregistrer'))
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(puts[0].zones).toEqual([
      { zone: 'ankle', side: 'left', since: '2026-10-12', note: '', condition: 'ankle_fracture' },
      { zone: 'knee_pfps', side: 'left', since: '2026-11-07', note: '', condition: 'other', customLabel: 'Plica <b>synoviale</b>' },
    ])

    // read mode: condition first, rendered as plain text
    await waitFor(() => expect(button('Modifier')).not.toBeNull())
    expect(document.body.textContent).toContain('Fracture de cheville (malléole)Cheville (gauche)')
    expect(document.body.textContent).toContain('Plica <b>synoviale</b>')
    expect(document.querySelector('b')).toBeNull()
  })
})
