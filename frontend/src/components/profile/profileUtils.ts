import type { FocusArea, FocusPreset, PaceTarget, Thresholds } from '../../api/types'
import { fmtPaceValue, parsePace } from '../../lib/format'
import { FOCUS_FALLBACK } from '../../lib/labels'
import { t } from '../../i18n'

export interface ThresholdField {
  key: keyof Thresholds
  label: string
  unit: string
  step: number
}

const UNIT_KEY: Record<string, 'unitFloors' | 'unitRatio'> = { étages: 'unitFloors', ratio: 'unitRatio' }
const field = (key: keyof Thresholds, unit: string, step: number): ThresholdField => ({
  key,
  step,
  get label() {
    return t(`settings.thresholds.fields.${key}`)
  },
  get unit() {
    return UNIT_KEY[unit] ? t(`settings.thresholds.${UNIT_KEY[unit]}`) : unit
  },
})

export const THRESHOLD_FIELDS: ThresholdField[] = [
  field('cadenceMinSpm', 'spm', 1),
  field('cadenceTargetMin', 'spm', 1),
  field('cadenceTargetMax', 'spm', 1),
  field('gctBalanceGapPct', 'pt', 0.1),
  field('verticalOscillationMaxCm', 'cm', 0.1),
  field('gctDriftMaxPct', '%', 0.5),
  field('descentMaxM', 'm', 5),
  field('floorsPrevDayMax', 'étages', 1),
  field('loadRatioMax', 'ratio', 0.05),
  field('rhrDeltaMaxBpm', 'bpm', 1),
  field('planDeviationPct', '%', 1),
  field('downhillCadenceDropSpm', 'spm', 1),
]

export type ThresholdDraft = Record<keyof Thresholds, string>

export const thresholdsToDraft = (th: Thresholds): ThresholdDraft =>
  Object.fromEntries(THRESHOLD_FIELDS.map((f) => [f.key, String(th[f.key])])) as ThresholdDraft

export function draftToThresholds(d: ThresholdDraft): { value: Thresholds | null; errors: Partial<Record<keyof Thresholds, string>> } {
  const errors: Partial<Record<keyof Thresholds, string>> = {}
  const out: Partial<Thresholds> = {}
  for (const f of THRESHOLD_FIELDS) {
    const raw = d[f.key].replace(',', '.').trim()
    const v = Number(raw)
    if (raw === '' || !Number.isFinite(v) || v < 0) errors[f.key] = t('settings.thresholds.positive')
    else out[f.key] = v
  }
  if (!errors.cadenceTargetMin && !errors.cadenceTargetMax && (out.cadenceTargetMin as number) > (out.cadenceTargetMax as number)) {
    errors.cadenceTargetMax = t('settings.thresholds.gteMin')
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
    if (!p.label.trim()) errors[i] = t('settings.thresholds.nameRequired')
    else if (min === null || max === null) errors[i] = t('settings.thresholds.paceFormat')
    else if (min > max) errors[i] = t('settings.thresholds.paceOrder')
    else out.push({ label: p.label.trim(), minSecPerKm: min, maxSecPerKm: max })
  })
  return { value: Object.keys(errors).length ? null : out, errors }
}

export const presetFor = (presets: FocusPreset[] | undefined, area: FocusArea) => presets?.find((p) => p.focusArea === area) ?? null

export const focusTitle = (presets: FocusPreset[] | undefined, area: FocusArea) => presetFor(presets, area)?.label ?? FOCUS_FALLBACK[area]


export function isValidTimezone(tz: string): boolean {
  try {
    new Intl.DateTimeFormat('en-GB', { timeZone: tz })
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
