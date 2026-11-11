import { Bandage, Gauge, Stethoscope, Thermometer, Wind, type LucideIcon } from 'lucide-react'
import type { MorphChange, MorphConstraintKind, PlannedSession, WorkoutStep } from '../../../api/types'
import { addDays } from '../../../lib/format'
import { STEP_LABEL } from '../steps'
import { lazyRecord, t } from '../../../i18n'

const FIELDS = ['title', 'sport', 'targetDurationS', 'targetDistanceM', 'pace', 'targetPaceMinSecPerKm', 'targetPaceMaxSecPerKm', 'targetCadenceSpm', 'instructions', 'steps'] as const
const FIELD_LABEL: Record<string, string> = lazyRecord(FIELDS, (k) => t(`plan.morph.fields.${k}`))

/** « Durée », « Structure », « Étape 2 · Échauffement ». */
export function changeLabel(c: MorphChange, steps: WorkoutStep[] = []): string {
  if (c.stepIndex === null) return FIELD_LABEL[c.field] ?? c.field
  const step = steps[c.stepIndex]
  const label = t('plan.morph.step', { n: c.stepIndex + 1 })
  return step ? `${label} · ${STEP_LABEL[step.type]}` : label
}

const CONSTRAINT_STYLE: Record<MorphConstraintKind, { icon: LucideIcon; cls: string }> = {
  directive: { icon: Stethoscope, cls: 'border-info/30 bg-info/10 text-info' },
  pain: { icon: Bandage, cls: 'border-danger/30 bg-danger/10 text-danger' },
  load: { icon: Gauge, cls: 'border-warn/35 bg-warn/12 text-warn' },
  heat: { icon: Thermometer, cls: 'border-warn/35 bg-warn/12 text-warn' },
  wind: { icon: Wind, cls: 'border-info/30 bg-info/10 text-info' },
}
export const CONSTRAINT_META: Record<MorphConstraintKind, { icon: LucideIcon; cls: string; name: string }> = lazyRecord(['directive', 'pain', 'load', 'heat', 'wind'] as const, (k) => ({
  ...CONSTRAINT_STYLE[k],
  name: t(`plan.morph.constraint.${k}`),
}))

export const SEVERITY_TEXT: Record<1 | 2 | 3, string> = lazyRecord([1, 2, 3] as const, (k) => t(`plan.morph.severity.${k}`))

/** Today's or tomorrow's session still to run: the only ones morphing applies to (§15.5). */
export function isMorphCandidate(s: PlannedSession, today: string): boolean {
  return s.sport !== 'rest' && (s.date === today || s.date === addDays(today, 1)) && (!s.match || s.match.status === 'upcoming')
}
