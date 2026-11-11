import type { AcwrVerdict, AlertLevel, DriftVerdict, EventKind, FocusArea, Freshness, GoalKind, MatchStatus, PersonalRecord, PlanSport, ShoeType, Sport } from '../api/types'
import { lazyRecord, t } from '../i18n/t'

// Label tables follow the active language (lazyRecord getters); keys and levels are stable.

const HRV_LEVEL: Record<string, AlertLevel> = { BALANCED: 'ok', UNBALANCED: 'warn', LOW: 'danger', POOR: 'danger' }

export function hrvStatus(status: string | null, label?: string | null): { label: string; level: AlertLevel } | null {
  if (!status) return null
  const level = HRV_LEVEL[status]
  return level ? { label: t(`labels.hrv.${status as 'BALANCED'}`), level } : { label: label ?? status, level: 'info' }
}

const SLEEP_LEVEL = { EXCELLENT: 'ok', GOOD: 'ok', FAIR: 'warn', POOR: 'danger' } as const satisfies Record<string, AlertLevel>
export const SLEEP_QUALIFIER: Record<string, { label: string; level: AlertLevel }> = lazyRecord(Object.keys(SLEEP_LEVEL) as (keyof typeof SLEEP_LEVEL)[], (k) => ({
  label: t(`labels.sleep.${k}`),
  level: SLEEP_LEVEL[k],
}))

const DRIFT_LEVEL: Record<DriftVerdict, AlertLevel> = { stable: 'ok', moderate: 'warn', high: 'danger' }
export const DRIFT_VERDICT: Record<DriftVerdict, { label: string; level: AlertLevel }> = lazyRecord(['stable', 'moderate', 'high'] as const, (k) => ({
  label: t(`labels.drift.${k}`),
  level: DRIFT_LEVEL[k],
}))

const ACWR_LEVEL: Record<AcwrVerdict, AlertLevel> = { low: 'info', optimal: 'ok', high: 'warn', danger: 'danger' }
export const ACWR_VERDICT: Record<AcwrVerdict, { label: string; level: AlertLevel }> = lazyRecord(['low', 'optimal', 'high', 'danger'] as const, (k) => ({
  label: t(`labels.acwr.${k}`),
  level: ACWR_LEVEL[k],
}))

export const SIDE_LABEL: Record<'left' | 'right' | 'both', string> = lazyRecord(['left', 'right', 'both'] as const, (k) => t(`labels.side.${k}`))

const TE_KEYS = ['RECOVERY', 'AEROBIC_BASE', 'TEMPO', 'LACTATE_THRESHOLD', 'VO2MAX', 'ANAEROBIC_CAPACITY', 'SPEED'] as const

export function teLabel(raw: string | null): string | null {
  if (!raw) return null
  return (TE_KEYS as readonly string[]).includes(raw) ? t(`labels.te.${raw as (typeof TE_KEYS)[number]}`) : raw.replace(/_/g, ' ').toLowerCase()
}

export const FOCUS_FALLBACK: Record<FocusArea, string> = lazyRecord(['none', 'knee_pfps', 'achilles', 'shin_splints', 'other'] as const, (k) => t(`labels.focus.${k}`))

const CONTEXT_TAG_KEYS = ['metronome', 'warmup_iso', 'squatting_48h', 'desk_sitting'] as const
export const CONTEXT_TAGS: { key: string; label: string }[] = CONTEXT_TAG_KEYS.map((key) => ({
  key,
  get label() {
    return t(`labels.contextTag.${key}`)
  },
}))
export const tagLabel = (key: string) => CONTEXT_TAGS.find((c) => c.key === key)?.label ?? key

export const PLAN_SPORT_LABEL: Record<PlanSport, string> = lazyRecord(['run', 'bike', 'strength', 'mobility', 'rest', 'other'] as const, (k) => t(`labels.planSport.${k}`))

const MATCH_LEVEL: Record<MatchStatus, AlertLevel> = { done: 'ok', partial: 'warn', missed: 'danger', upcoming: 'info', extra: 'info' }
export const MATCH_STATUS: Record<MatchStatus, { label: string; level: AlertLevel }> = lazyRecord(['done', 'partial', 'missed', 'upcoming', 'extra'] as const, (k) => ({
  label: t(`labels.match.${k}`),
  level: MATCH_LEVEL[k],
}))

export const GOAL_KIND_LABEL: Record<GoalKind, string> = lazyRecord(['macro', 'race', 'cadence', 'weekly_volume', 'other'] as const, (k) => t(`labels.goalKind.${k}`))

export const SHOE_TYPE_LABEL: Record<ShoeType, string> = lazyRecord(['soft', 'tempo', 'carbon'] as const, (k) => t(`labels.shoeType.${k}`))

export const FRESHNESS_LABEL: Record<Freshness['source'], { label: string; hint: string }> = lazyRecord(['db', 'cache', 'live'] as const, (k) => ({
  label: t(`labels.freshness.${k}`),
  hint: t(`labels.freshness.${k}Hint`),
}))

export const SOURCE_LABEL: Record<string, string> = lazyRecord(['garmin', 'strava', 'import', 'manual'] as const, (k) => t(`labels.source.${k}`))

export const SPORT_LABEL: Record<Sport, string> = lazyRecord(['run', 'bike', 'walk', 'hike', 'swim', 'strength', 'other'] as const, (k) => t(`labels.sport.${k}`))

export const EVENT_KIND: Record<EventKind, { label: string; short: string; hint: string }> = lazyRecord(['fatigue', 'pain', 'illness', 'unavailable', 'forced_rest'] as const, (k) => ({
  label: t(`labels.eventKind.${k}.label`),
  short: t(`labels.eventKind.${k}.short`),
  hint: t(`labels.eventKind.${k}.hint`),
}))

export const SEVERITY_LABEL: Record<1 | 2 | 3, string> = lazyRecord([1, 2, 3] as const, (k) => t(`labels.severity.${k}`))

export const RECORD_SOURCE: Record<PersonalRecord['source'], string> = lazyRecord(['computed', 'strava', 'manual'] as const, (k) => t(`labels.recordSource.${k}`))
