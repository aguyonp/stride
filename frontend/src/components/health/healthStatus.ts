import type { BodySide, BodyZone, HealthStatusInput, HealthZone } from '../../api/types'
import { ALL_BODY_ZONES, BODY_SIDE_LABEL, BODY_ZONE_META } from '../bodymap/bodyMapMeta'
import { lazyRecord, t } from '../../i18n'

export const HEALTH_MODE_LABEL: Record<HealthStatusInput['mode'], string> = lazyRecord(['performance', 'tracking'] as const, (k) => t(`health.status.mode.${k}`))

export const zoneText = (z: Pick<HealthZone, 'zone' | 'side'>) => `${BODY_ZONE_META[z.zone].short} (${BODY_SIDE_LABEL[z.side].toLowerCase()})`

/** Catalog key of a condition described by the athlete (free text in `customLabel`). */
export const CONDITION_OTHER = 'other'
export const CUSTOM_LABEL_MAX = 80

/** « Fracture de cheville (malléole) · Cheville (gauche) », or the zone alone without condition. */
export function trackedZoneText(z: Pick<HealthZone, 'zone' | 'side' | 'conditionLabel'>): string {
  const label = z.conditionLabel?.trim()
  return label ? `${label} · ${zoneText(z)}` : zoneText(z)
}

export function healthSummary(s: HealthStatusInput): string {
  if (s.mode === 'performance' || s.zones.length === 0) return HEALTH_MODE_LABEL.performance
  return `${HEALTH_MODE_LABEL.tracking} · ${s.zones.map(trackedZoneText).join(', ')}`
}

export function draftFrom(s: HealthStatusInput | undefined | null): HealthStatusInput {
  return s ? { mode: s.mode, zones: s.zones.map((z) => ({ ...z })) } : { mode: 'performance', zones: [] }
}

/** Zone change in the editor: a catalog condition belongs to its zone and is reset; « Autre » and its text follow. */
export function changeZone(z: HealthZone, zone: BodyZone): HealthZone {
  const keep = z.condition === CONDITION_OTHER
  return { ...z, zone, condition: keep ? CONDITION_OTHER : null, customLabel: keep ? z.customLabel : null, conditionLabel: null }
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

/** Body sent to PUT /api/health/status: performance drops the zones; the derived label is never sent. */
export function toHealthInput(d: HealthStatusInput): HealthStatusInput {
  if (d.mode === 'performance') return { mode: 'performance', zones: [] }
  return {
    mode: 'tracking',
    zones: d.zones.map(({ conditionLabel: _derived, condition, customLabel, ...z }) => {
      const out: HealthZone = { ...z, note: z.note.trim() }
      if (condition) out.condition = condition
      if (condition === CONDITION_OTHER) out.customLabel = (customLabel ?? '').trim()
      return out
    }),
  }
}

export function validateHealthDraft(d: HealthStatusInput): string | null {
  if (d.mode === 'performance') return null
  if (d.zones.length === 0) return t('health.status.zoneRequired')
  if (new Set(d.zones.map((z) => z.zone)).size !== d.zones.length) return t('health.status.zoneUnique')
  for (const z of d.zones) {
    if (z.condition !== CONDITION_OTHER) continue
    const label = (z.customLabel ?? '').trim()
    if (!label) return t('health.status.conditionRequired', { zone: BODY_ZONE_META[z.zone].short })
    if (label.length > CUSTOM_LABEL_MAX) return t('health.status.conditionMax', { max: CUSTOM_LABEL_MAX })
  }
  return null
}

/** Default zone for a new symptom: the most recently tracked one, else the knee. */
export function primaryZone(s: HealthStatusInput | null | undefined): { zone: BodyZone; side: BodySide } {
  const tracked = s?.mode === 'tracking' ? [...s.zones].sort((a, b) => (b.since ?? '').localeCompare(a.since ?? '')) : []
  return tracked[0] ? { zone: tracked[0].zone, side: tracked[0].side } : { zone: 'knee_pfps', side: 'left' }
}
