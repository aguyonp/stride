import type { AthleteProfile, FocusArea, FocusPreset, InjuredSide, PaceTarget, Thresholds } from '../../api/types'
import { fmtPaceValue, parsePace } from '../../lib/format'
import { FOCUS_FALLBACK } from '../../lib/labels'

export const FOCUS_ORDER: FocusArea[] = ['none', 'knee_pfps', 'achilles', 'shin_splints', 'other']

export const FOCUS_HINT: Record<FocusArea, string> = {
  none: 'Formulaire de douleur facultatif, alertes blessure désactivées (sauf charge).',
  knee_pfps: 'Cadence, descentes, oscillation verticale, étages et dénivelé négatif surveillés.',
  achilles: 'Cadence, dérive du temps de contact, montées / D+ et charge du mollet surveillés.',
  shin_splints: 'Cadence, impact, sauts de volume et surfaces dures surveillés.',
  other: 'Zone personnalisée : donne-lui un nom, il apparaîtra dans les prompts.',
}

export const SIDE_OPTIONS: { value: InjuredSide | 'none'; label: string }[] = [
  { value: 'left', label: 'Gauche' },
  { value: 'right', label: 'Droite' },
  { value: 'both', label: 'Les deux' },
  { value: 'none', label: 'Aucun' },
]

export interface ThresholdField {
  key: keyof Thresholds
  label: string
  unit: string
  step: number
}

export const THRESHOLD_FIELDS: ThresholdField[] = [
  { key: 'cadenceMinSpm', label: 'Cadence minimale', unit: 'spm', step: 1 },
  { key: 'cadenceTargetMin', label: 'Cible cadence (min)', unit: 'spm', step: 1 },
  { key: 'cadenceTargetMax', label: 'Cible cadence (max)', unit: 'spm', step: 1 },
  { key: 'gctBalanceGapPct', label: 'Écart d’équilibre G/D max', unit: 'pt', step: 0.1 },
  { key: 'verticalOscillationMaxCm', label: 'Oscillation verticale max', unit: 'cm', step: 0.1 },
  { key: 'gctDriftMaxPct', label: 'Dérive du temps de contact max', unit: '%', step: 0.5 },
  { key: 'descentMaxM', label: 'Dénivelé négatif max', unit: 'm', step: 5 },
  { key: 'floorsPrevDayMax', label: 'Étages montés la veille max', unit: 'étages', step: 1 },
  { key: 'loadRatioMax', label: 'Ratio de charge max', unit: 'ratio', step: 0.05 },
  { key: 'rhrDeltaMaxBpm', label: 'Hausse de FC repos max', unit: 'bpm', step: 1 },
  { key: 'planDeviationPct', label: 'Écart au plan max', unit: '%', step: 1 },
  { key: 'downhillCadenceDropSpm', label: 'Chute de cadence en descente max', unit: 'spm', step: 1 },
]

export type ThresholdDraft = Record<keyof Thresholds, string>

export const thresholdsToDraft = (t: Thresholds): ThresholdDraft =>
  Object.fromEntries(THRESHOLD_FIELDS.map((f) => [f.key, String(t[f.key])])) as ThresholdDraft

export function draftToThresholds(d: ThresholdDraft): { value: Thresholds | null; errors: Partial<Record<keyof Thresholds, string>> } {
  const errors: Partial<Record<keyof Thresholds, string>> = {}
  const out: Partial<Thresholds> = {}
  for (const f of THRESHOLD_FIELDS) {
    const raw = d[f.key].replace(',', '.').trim()
    const v = Number(raw)
    if (raw === '' || !Number.isFinite(v) || v < 0) errors[f.key] = 'Nombre positif attendu'
    else out[f.key] = v
  }
  if (!errors.cadenceTargetMin && !errors.cadenceTargetMax && (out.cadenceTargetMin as number) > (out.cadenceTargetMax as number)) {
    errors.cadenceTargetMax = 'Doit être ≥ à la cible min'
  }
  return { value: Object.keys(errors).length ? null : (out as Thresholds), errors }
}

export interface PaceDraft {
  label: string
  min: string
  max: string
}

export const paceToDraft = (p: PaceTarget): PaceDraft => ({ label: p.label, min: fmtPaceValue(p.minSecPerKm), max: fmtPaceValue(p.maxSecPerKm) })

export function draftToPaces(d: PaceDraft[]): { value: PaceTarget[] | null; errors: Record<number, string> } {
  const errors: Record<number, string> = {}
  const out: PaceTarget[] = []
  d.forEach((p, i) => {
    const min = parsePace(p.min)
    const max = parsePace(p.max)
    if (!p.label.trim()) errors[i] = 'Nom requis'
    else if (min === null || max === null) errors[i] = 'Allures au format m:ss (ex. 3:35)'
    else if (min > max) errors[i] = 'L’allure la plus rapide doit venir en premier'
    else out.push({ label: p.label.trim(), minSecPerKm: min, maxSecPerKm: max })
  })
  return { value: Object.keys(errors).length ? null : out, errors }
}

export const presetFor = (presets: FocusPreset[] | undefined, area: FocusArea) => presets?.find((p) => p.focusArea === area) ?? null

export const focusTitle = (presets: FocusPreset[] | undefined, area: FocusArea) => presetFor(presets, area)?.label ?? FOCUS_FALLBACK[area]

export function browserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'Europe/Paris'
  } catch {
    return 'Europe/Paris'
  }
}

export function isValidTimezone(tz: string): boolean {
  try {
    new Intl.DateTimeFormat('fr-FR', { timeZone: tz })
    return true
  } catch {
    return false
  }
}

export const TIMEZONES = [
  'Europe/Paris',
  'Europe/Brussels',
  'Europe/Zurich',
  'Europe/Luxembourg',
  'Europe/London',
  'Europe/Madrid',
  'Europe/Berlin',
  'America/Montreal',
  'America/New_York',
  'America/Los_Angeles',
  'America/Martinique',
  'Indian/Reunion',
  'Pacific/Noumea',
  'UTC',
]

/** Builds a full profile for a focus area, keeping other user settings. */
export function profileForFocus(base: AthleteProfile | undefined, presets: FocusPreset[] | undefined, area: FocusArea, customLabel: string, side: InjuredSide | null): AthleteProfile | null {
  const preset = presetFor(presets, area)
  const thresholds = preset?.thresholds ?? base?.thresholds
  if (!thresholds) return null
  return {
    focusArea: area,
    focusLabel: area === 'other' ? customLabel.trim() || FOCUS_FALLBACK.other : (preset?.label ?? FOCUS_FALLBACK[area]),
    injuredSide: area === 'none' ? null : side,
    timezone: base?.timezone ?? browserTimezone(),
    thresholds: { ...thresholds },
    paceTargets: base?.paceTargets ?? [],
  }
}
