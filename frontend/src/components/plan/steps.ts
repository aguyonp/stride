import type { StepType, TargetKind, WorkoutStep } from '../../api/types'
import { fmtNumber, fmtPaceValue, isNum } from '../../lib/format'

export const STEP_LABEL: Record<StepType, string> = {
  warmup: 'Échauffement',
  run: 'Course',
  interval: 'Fractionné',
  recovery: 'Récupération',
  rest: 'Repos',
  cooldown: 'Retour au calme',
  repeat: 'Répétition',
}

export const TARGET_LABEL: Record<TargetKind, string> = { none: 'Aucune', pace: 'Allure', hr: 'FC', cadence: 'Cadence', power: 'Puissance' }

export const STEP_TONE: Record<StepType, string> = {
  warmup: 'bg-info',
  run: 'bg-accent',
  interval: 'bg-danger',
  recovery: 'bg-ok',
  rest: 'bg-ink-3',
  cooldown: 'bg-info',
  repeat: 'bg-violet',
}

/** « 10′ », « 1′30 », « 45″ ». */
function fmtStepDuration(s: number): string {
  const m = Math.floor(s / 60)
  const r = Math.round(s % 60)
  if (m === 0) return `${r}″`
  return r ? `${m}′${String(r).padStart(2, '0')}` : `${m}′`
}

function fmtStepDistance(m: number): string {
  return m >= 1000 ? `${fmtNumber(m / 1000, m % 1000 ? 1 : 0)} km` : `${fmtNumber(m)} m`
}

export function targetText(t: WorkoutStep['target']): string | null {
  if (!t || t.kind === 'none') return null
  const range = (f: (v: number) => string, unit: string) => {
    if (isNum(t.min) && isNum(t.max) && t.min !== t.max) return `${f(t.min)}–${f(t.max)}${unit}`
    const v = isNum(t.min) ? t.min : t.max
    return isNum(v) ? `${f(v)}${unit}` : null
  }
  switch (t.kind) {
    case 'pace':
      return range(fmtPaceValue, '/km')
    case 'hr':
      return range((v) => fmtNumber(v), ' bpm')
    case 'cadence':
      return range((v) => fmtNumber(v), ' spm')
    case 'power':
      return range((v) => fmtNumber(v), ' W')
  }
}

export function stepAmount(s: WorkoutStep): string | null {
  if (isNum(s.durationS) && s.durationS > 0) return fmtStepDuration(s.durationS)
  if (isNum(s.distanceM) && s.distanceM > 0) return fmtStepDistance(s.distanceM)
  return null
}

const SHORT: Partial<Record<StepType, string>> = { warmup: 'Éch.', recovery: 'récup', cooldown: 'Retour', rest: 'repos' }

/** One-line structure: « Éch. 10′ · 6 × (1′ @4:10–4:20/km + 1′30 récup) · Retour 10′ ». */
export function stepsSummary(steps: WorkoutStep[]): string {
  const one = (s: WorkoutStep, inner = false): string => {
    if (s.type === 'repeat') return `${s.repeat ?? 1} × (${(s.steps ?? []).map((x) => one(x, true)).join(' + ')})`
    const amount = stepAmount(s) ?? ''
    const t = targetText(s.target)
    const label = inner ? (s.type === 'interval' || s.type === 'run' ? '' : (SHORT[s.type] ?? '')) : (SHORT[s.type] ?? (s.type === 'interval' ? '' : STEP_LABEL[s.type]))
    return [inner ? amount : [label, amount].filter(Boolean).join(' '), t && `@${t}`, inner && label].filter(Boolean).join(' ')
  }
  return steps.map((s) => one(s)).join(' · ')
}

export function totalDurationS(steps: WorkoutStep[]): number | null {
  let total = 0
  let known = true
  const walk = (list: WorkoutStep[], mult: number) => {
    for (const s of list) {
      if (s.type === 'repeat') walk(s.steps ?? [], mult * (s.repeat ?? 1))
      else if (isNum(s.durationS)) total += s.durationS * mult
      else known = false
    }
  }
  walk(steps, 1)
  return known && total > 0 ? total : null
}
