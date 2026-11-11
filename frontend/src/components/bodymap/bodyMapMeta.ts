import type { BodyCorrelationFactor, BodySide, BodyZone, BodyZoneSummary } from '../../api/types'
import { fmtNumber, isNum } from '../../lib/format'
import { lazyRecord, t } from '../../i18n'

const ZONE_VIEW: Record<BodyZone, 'Antérieure' | 'Postérieure' | 'Plantaire'> = {
  knee_pfps: 'Antérieure',
  shin_splints: 'Antérieure',
  iliotibial_band: 'Antérieure',
  achilles: 'Postérieure',
  plantar_fascia: 'Plantaire',
  hamstrings_calves: 'Postérieure',
  ankle: 'Antérieure',
  forefoot: 'Plantaire',
  hip: 'Antérieure',
  lower_back: 'Postérieure',
}

/** Zone labels in the active language; `view` is a stable internal key (anatomy panel). */
export const BODY_ZONE_META: Record<BodyZone, { label: string; short: string; view: 'Antérieure' | 'Postérieure' | 'Plantaire'; hint: string }> = lazyRecord(
  Object.keys(ZONE_VIEW) as BodyZone[],
  (z) => ({ label: t(`health.zones.${z}.label`), short: t(`health.zones.${z}.short`), view: ZONE_VIEW[z], hint: t(`health.zones.${z}.hint`) }),
)

export const ALL_BODY_ZONES: BodyZone[] = [
  'knee_pfps',
  'shin_splints',
  'iliotibial_band',
  'achilles',
  'plantar_fascia',
  'hamstrings_calves',
  'ankle',
  'forefoot',
  'hip',
  'lower_back',
]

export const BODY_SIDE_LABEL: Record<BodySide, string> = lazyRecord(['left', 'right', 'both'] as const, (k) => t(`health.bodymap.side.${k}`))

const TREND_CLS: Record<BodyZoneSummary['trend'], string> = {
  improving: 'border-ok/30 bg-ok/12 text-ok',
  stable: 'border-line-strong bg-track text-ink-2',
  worsening: 'border-danger/30 bg-danger/12 text-danger',
  none: 'border-line bg-track/60 text-ink-3',
}
export const TREND_META: Record<BodyZoneSummary['trend'], { label: string; cls: string }> = lazyRecord(['improving', 'stable', 'worsening', 'none'] as const, (k) => ({
  label: t(`health.bodymap.trend.${k}`),
  cls: TREND_CLS[k],
}))

const CONFIDENCE_CLS: Record<NonNullable<BodyCorrelationFactor['confidence']>, string> = {
  high: 'border-ok/30 bg-ok/12 text-ok',
  moderate: 'border-info/30 bg-info/12 text-info',
  low: 'border-warn/30 bg-warn/12 text-warn',
  insufficient: 'border-line bg-track text-ink-3',
}
const CONFIDENCE_META: Record<NonNullable<BodyCorrelationFactor['confidence']>, { label: string; cls: string }> = lazyRecord(['high', 'moderate', 'low', 'insufficient'] as const, (k) => ({
  label: t(`health.bodymap.confidence.${k}`),
  cls: CONFIDENCE_CLS[k],
}))

/** True when the backend could not compute a usable comparison for this factor. */
export function isCorrelationInsufficient(c: BodyCorrelationFactor): boolean {
  return c.insufficientData === true || (c.symptomaticMean === null && c.baselineMean === null && c.deltaPct === null)
}

/** Confidence chip, hidden when unknown or redundant with the insufficient-data notice. */
export function correlationConfidence(c: BodyCorrelationFactor): { label: string; cls: string } | null {
  if (c.confidence === null || isCorrelationInsufficient(c)) return null
  return CONFIDENCE_META[c.confidence]
}

export function fmtMeasure(v: number | null, unit: string): string | null {
  return isNum(v) ? `${fmtNumber(v, 1, { trim: true })} ${unit}` : null
}

export function correlationSample(c: BodyCorrelationFactor): string | null {
  if (!isNum(c.symptomaticRuns) || !isNum(c.baselineRuns)) return null
  return t('health.bodymap.sample', { symptomatic: c.symptomaticRuns, baseline: c.baselineRuns })
}

export function shoeHighlightDetail(h: NonNullable<BodyCorrelationFactor['shoeHighlight']>): string {
  const parts: string[] = []
  if (isNum(h.dropMm)) parts.push(t('health.bodymap.drop', { value: fmtNumber(h.dropMm) }))
  if (isNum(h.distanceKm)) parts.push(`${fmtNumber(h.distanceKm, 0)} km`)
  if (isNum(h.painEpisodes)) parts.push(t('health.bodymap.episodes', { count: h.painEpisodes }))
  return parts.join(' · ')
}

export function effectiveZoneScore(z: BodyZoneSummary | undefined): number {
  if (!z) return 0
  const candidates = [z.latestScore, z.latestColdNextDayScore, z.maxScore30d].filter(
    (v): v is number => typeof v === 'number' && Number.isFinite(v),
  )
  if (!candidates.length) return 0
  return Math.max(...candidates)
}

export function painScoreTone(score: number | null | undefined): {
  level: 'none' | 'low' | 'warn' | 'danger'
  textCls: string
  badgeCls: string
  svgFill: string
  svgStroke: string
} {
  const s = score ?? 0
  if (s >= 7) {
    return {
      level: 'danger',
      textCls: 'text-danger',
      badgeCls: 'border-danger/35 bg-danger/14 text-danger',
      svgFill: 'color-mix(in oklab, var(--danger) 38%, transparent)',
      svgStroke: 'var(--danger)',
    }
  }
  if (s >= 4) {
    return {
      level: 'warn',
      textCls: 'text-warn',
      badgeCls: 'border-warn/35 bg-warn/14 text-warn',
      svgFill: 'color-mix(in oklab, var(--warn) 34%, transparent)',
      svgStroke: 'var(--warn)',
    }
  }
  if (s > 0) {
    return {
      level: 'low',
      textCls: 'text-ok',
      badgeCls: 'border-ok/35 bg-ok/14 text-ok',
      svgFill: 'color-mix(in oklab, var(--ok) 28%, transparent)',
      svgStroke: 'var(--ok)',
    }
  }
  return {
    level: 'none',
    textCls: 'text-ink-3',
    badgeCls: 'border-line bg-track text-ink-3',
    svgFill: 'color-mix(in oklab, var(--text-1) 6%, transparent)',
    svgStroke: 'var(--line-strong)',
  }
}
