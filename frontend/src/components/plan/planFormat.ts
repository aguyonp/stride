import type { PlanMatch, PlannedSession } from '../../api/types'
import { addDays, diffDays, fmtHM, fmtKm, fmtNumber, fmtPaceValue, fmtSigned, fmtSignedPct, isNum, monthShort, parseLocal } from '../../lib/format'
import { t } from '../../i18n'

const MONTH = (m1: number) => monthShort(m1)

/** « Semaine du 3 au 9 nov. », « Semaine du 27 oct. au 2 nov. ». */
export function weekLabel(weekStart: string): string {
  const a = parseLocal(weekStart)
  const b = parseLocal(addDays(weekStart, 6))
  if (!a || !b) return t('plan.format.week')
  const end = `${b.d} ${MONTH(b.mo)}`
  const start = a.mo === b.mo ? `${a.d}` : `${a.d} ${MONTH(a.mo)}`
  const year = a.y !== b.y ? ` ${b.y}` : ''
  return `${t('plan.format.weekOf', { start, end })}${year}`
}

/** « aujourd'hui », « demain », « dans 12 j », « hier », « il y a 3 j ». */
export function relativeDays(date: string, today: string): string {
  const d = diffDays(today, date)
  if (d === 0) return t('plan.format.today')
  if (d === 1) return t('plan.format.tomorrow')
  if (d === -1) return t('plan.format.yesterday')
  return d > 0 ? t('plan.format.inDays', { n: d }) : t('plan.format.daysAgo', { n: -d })
}

function paceRange(min: number | null, max: number | null): string | null {
  if (!isNum(min) && !isNum(max)) return null
  if (isNum(min) && isNum(max) && min !== max) return `${fmtPaceValue(min)}–${fmtPaceValue(max)}/km`
  return `${fmtPaceValue((isNum(min) ? min : max) as number)}/km`
}

/** Compact target list, e.g. ["45 min", "8,0 km", "5:10–5:20/km", "172 spm"]. */
export function targetParts(s: Pick<PlannedSession, 'targetDurationS' | 'targetDistanceM' | 'targetPaceMinSecPerKm' | 'targetPaceMaxSecPerKm' | 'targetCadenceSpm'>): string[] {
  const out: string[] = []
  if (isNum(s.targetDurationS) && s.targetDurationS > 0) out.push(fmtHM(s.targetDurationS))
  if (isNum(s.targetDistanceM) && s.targetDistanceM > 0) out.push(fmtKm(s.targetDistanceM, 1))
  const pace = paceRange(s.targetPaceMinSecPerKm, s.targetPaceMaxSecPerKm)
  if (pace) out.push(pace)
  if (isNum(s.targetCadenceSpm) && s.targetCadenceSpm > 0) out.push(`${fmtNumber(s.targetCadenceSpm)} spm`)
  return out
}

/** Signed deltas of a plan match, e.g. ["Durée +8 %", "Allure −4 s/km"]. */
export function matchDeltas(m: PlanMatch): string[] {
  const out: string[] = []
  if (isNum(m.durationDeltaPct)) out.push(t('plan.format.deltaDuration', { value: fmtSignedPct(m.durationDeltaPct, 0) }))
  if (isNum(m.distanceDeltaPct)) out.push(t('plan.format.deltaDistance', { value: fmtSignedPct(m.distanceDeltaPct, 0) }))
  if (isNum(m.paceDeltaSecPerKm)) out.push(t('plan.format.deltaPace', { value: fmtSigned(m.paceDeltaSecPerKm) }))
  if (isNum(m.cadenceDeltaSpm)) out.push(t('plan.format.deltaCadence', { value: fmtSigned(m.cadenceDeltaSpm) }))
  return out
}

export const ratioPct = (done: number, planned: number) => (planned > 0 ? Math.round((done / planned) * 100) : null)
