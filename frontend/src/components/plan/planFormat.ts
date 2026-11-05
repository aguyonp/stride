import type { PlanMatch, PlannedSession } from '../../api/types'
import { addDays, diffDays, fmtHM, fmtKm, fmtNumber, fmtPaceValue, fmtSigned, isNum, parseLocal } from '../../lib/format'

const MONTHS = ['janv.', 'févr.', 'mars', 'avr.', 'mai', 'juin', 'juil.', 'août', 'sept.', 'oct.', 'nov.', 'déc.']

/** « Semaine du 3 au 9 nov. », « Semaine du 27 oct. au 2 nov. ». */
export function weekLabel(weekStart: string): string {
  const a = parseLocal(weekStart)
  const b = parseLocal(addDays(weekStart, 6))
  if (!a || !b) return 'Semaine'
  const end = `${b.d} ${MONTHS[b.mo - 1]}`
  const start = a.mo === b.mo ? `${a.d}` : `${a.d} ${MONTHS[a.mo - 1]}`
  const year = a.y !== b.y ? ` ${b.y}` : ''
  return `Semaine du ${start} au ${end}${year}`
}

/** « aujourd'hui », « demain », « dans 12 j », « hier », « il y a 3 j ». */
export function relativeDays(date: string, today: string): string {
  const d = diffDays(today, date)
  if (d === 0) return 'aujourd’hui'
  if (d === 1) return 'demain'
  if (d === -1) return 'hier'
  return d > 0 ? `dans ${d} j` : `il y a ${-d} j`
}

export function paceRange(min: number | null, max: number | null): string | null {
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
  if (isNum(m.durationDeltaPct)) out.push(`Durée ${fmtSigned(m.durationDeltaPct)} %`)
  if (isNum(m.distanceDeltaPct)) out.push(`Distance ${fmtSigned(m.distanceDeltaPct)} %`)
  if (isNum(m.paceDeltaSecPerKm)) out.push(`Allure ${fmtSigned(m.paceDeltaSecPerKm)} s/km`)
  if (isNum(m.cadenceDeltaSpm)) out.push(`Cadence ${fmtSigned(m.cadenceDeltaSpm)} spm`)
  return out
}

export const ratioPct = (done: number, planned: number) => (planned > 0 ? Math.round((done / planned) * 100) : null)
