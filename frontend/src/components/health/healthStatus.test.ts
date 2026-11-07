import { describe, expect, it } from 'vitest'
import type { HealthStatusInput } from '../../api/types'
import { addZone, changeZone, draftFrom, healthSummary, primaryZone, setMode, toHealthInput, validateHealthDraft } from './healthStatus'

const tracking: HealthStatusInput = {
  mode: 'tracking',
  zones: [
    { zone: 'achilles', side: 'right', since: '2026-09-01', note: ' raideur ' },
    { zone: 'knee_pfps', side: 'left', since: '2026-10-15', note: '' },
  ],
}

describe('health status', () => {
  it('summarises the status with zones and sides', () => {
    expect(healthSummary({ mode: 'performance', zones: [] })).toBe('Bonne santé · mode performance')
    expect(healthSummary(tracking)).toBe('Souci suivi · Tendon d’Achille (droite), Rotule / Genou (gauche)')
  })

  it('switches modes, adds free zones and drops zones when saving performance', () => {
    const t = setMode(draftFrom(null), 'tracking', '2026-11-07')
    expect(t).toEqual({ mode: 'tracking', zones: [{ zone: 'knee_pfps', side: 'left', since: '2026-11-07', note: '' }] })
    expect(addZone(t, '2026-11-07').zones.map((z) => z.zone)).toEqual(['knee_pfps', 'shin_splints'])
    expect(toHealthInput(setMode(tracking, 'performance', '2026-11-07'))).toEqual({ mode: 'performance', zones: [] })
    expect(toHealthInput(tracking).zones[0].note).toBe('raideur')
  })

  it('validates tracked zones and picks the most recent one as default', () => {
    expect(validateHealthDraft({ mode: 'tracking', zones: [] })).toMatch(/au moins une zone/)
    expect(validateHealthDraft({ mode: 'tracking', zones: [tracking.zones[0], tracking.zones[0]] })).toMatch(/qu’une fois/)
    expect(validateHealthDraft(tracking)).toBeNull()
    expect(primaryZone(tracking)).toEqual({ zone: 'knee_pfps', side: 'left' })
    expect(primaryZone({ mode: 'performance', zones: [] })).toEqual({ zone: 'knee_pfps', side: 'left' })
  })

  it('handles pathologies: summary, zone change, custom text validation and payload', () => {
    const fracture = { zone: 'ankle', side: 'left', since: '2026-10-12', note: '', condition: 'ankle_fracture', conditionLabel: 'Fracture de cheville (malléole)' } as const
    expect(healthSummary({ mode: 'tracking', zones: [fracture] })).toBe('Souci suivi · Fracture de cheville (malléole) · Cheville (gauche)')
    expect(changeZone(fracture, 'hip')).toMatchObject({ zone: 'hip', condition: null, customLabel: null, conditionLabel: null })
    const custom = { zone: 'hip', side: 'right', since: null, note: '', condition: 'other', customLabel: '  Conflit fémoro-acétabulaire ' } as const
    expect(changeZone(custom, 'lower_back')).toMatchObject({ zone: 'lower_back', condition: 'other', customLabel: '  Conflit fémoro-acétabulaire ' })
    expect(validateHealthDraft({ mode: 'tracking', zones: [{ ...custom, customLabel: '  ' }] })).toMatch(/Précise la pathologie suivie \(Hanche\)/)
    expect(validateHealthDraft({ mode: 'tracking', zones: [{ ...custom, customLabel: 'x'.repeat(81) }] })).toMatch(/80 caractères/)
    expect(toHealthInput({ mode: 'tracking', zones: [fracture, custom, { ...tracking.zones[1], condition: null, customLabel: 'stale' }] }).zones).toEqual([
      { zone: 'ankle', side: 'left', since: '2026-10-12', note: '', condition: 'ankle_fracture' },
      { zone: 'hip', side: 'right', since: null, note: '', condition: 'other', customLabel: 'Conflit fémoro-acétabulaire' },
      { zone: 'knee_pfps', side: 'left', since: '2026-10-15', note: '' },
    ])
  })
})
