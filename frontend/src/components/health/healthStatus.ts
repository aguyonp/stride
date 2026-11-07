import type { BodySide, BodyZone, HealthStatusInput, HealthZone } from '../../api/types'
import { ALL_BODY_ZONES, BODY_SIDE_LABEL, BODY_ZONE_META } from '../bodymap/bodyMapMeta'

export const HEALTH_MODE_LABEL: Record<HealthStatusInput['mode'], string> = {
  performance: 'Bonne santé · mode performance',
  tracking: 'Souci suivi',
}

export const zoneText = (z: Pick<HealthZone, 'zone' | 'side'>) => `${BODY_ZONE_META[z.zone].short} (${BODY_SIDE_LABEL[z.side].toLowerCase()})`

export function healthSummary(s: HealthStatusInput): string {
  if (s.mode === 'performance' || s.zones.length === 0) return HEALTH_MODE_LABEL.performance
  return `${HEALTH_MODE_LABEL.tracking} · ${s.zones.map(zoneText).join(', ')}`
}

export function draftFrom(s: HealthStatusInput | undefined | null): HealthStatusInput {
  return s ? { mode: s.mode, zones: s.zones.map((z) => ({ ...z })) } : { mode: 'performance', zones: [] }
}

export function addZone(d: HealthStatusInput, today: string): HealthStatusInput {
  const free = ALL_BODY_ZONES.find((z) => !d.zones.some((x) => x.zone === z))
  if (!free) return d
  return { mode: 'tracking', zones: [...d.zones, { zone: free, side: 'left', since: today, note: '' }] }
}

export function setMode(d: HealthStatusInput, mode: HealthStatusInput['mode'], today: string): HealthStatusInput {
  if (mode === 'performance') return { mode, zones: d.zones }
  return d.zones.length ? { mode, zones: d.zones } : addZone({ mode, zones: [] }, today)
}

/** Body sent to PUT /api/health/status: performance drops the zones. */
export function toHealthInput(d: HealthStatusInput): HealthStatusInput {
  return d.mode === 'performance' ? { mode: 'performance', zones: [] } : { mode: 'tracking', zones: d.zones.map((z) => ({ ...z, note: z.note.trim() })) }
}

export function validateHealthDraft(d: HealthStatusInput): string | null {
  if (d.mode === 'performance') return null
  if (d.zones.length === 0) return 'Ajoute au moins une zone suivie.'
  if (new Set(d.zones.map((z) => z.zone)).size !== d.zones.length) return 'Chaque zone ne peut apparaître qu’une fois.'
  return null
}

/** Default zone for a new symptom: the most recently tracked one, else the knee. */
export function primaryZone(s: HealthStatusInput | null | undefined): { zone: BodyZone; side: BodySide } {
  const tracked = s?.mode === 'tracking' ? [...s.zones].sort((a, b) => (b.since ?? '').localeCompare(a.since ?? '')) : []
  return tracked[0] ? { zone: tracked[0].zone, side: tracked[0].side } : { zone: 'knee_pfps', side: 'left' }
}
