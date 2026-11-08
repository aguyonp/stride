import type { CyclePlanStatus, PhaseType } from '../../../api/types'
import { lazyRecord, t } from '../../../i18n'

export type CycleZoom = 'macro' | 'meso' | 'micro'

export const PHASE_TYPE_META: Record<
  PhaseType,
  { label: string; chipCls: string; barCls: string; dotCls: string }
> = {
  foundation: {
    get label() {
      return t('plan.cycles.phaseType.foundation')
    },
    chipCls: 'border-info/30 bg-info/12 text-info',
    barCls: 'border-info/35 bg-info/14 text-ink',
    dotCls: 'bg-info',
  },
  build: {
    get label() {
      return t('plan.cycles.phaseType.build')
    },
    chipCls: 'border-accent/30 bg-accent/14 text-accent',
    barCls: 'border-accent/35 bg-accent/16 text-ink',
    dotCls: 'bg-accent',
  },
  threshold: {
    get label() {
      return t('plan.cycles.phaseType.threshold')
    },
    chipCls: 'border-warn/30 bg-warn/12 text-warn',
    barCls: 'border-warn/35 bg-warn/14 text-ink',
    dotCls: 'bg-warn',
  },
  specific: {
    get label() {
      return t('plan.cycles.phaseType.specific')
    },
    chipCls: 'border-violet/30 bg-violet/14 text-violet',
    barCls: 'border-violet/35 bg-violet/16 text-ink',
    dotCls: 'bg-violet',
  },
  assimilation: {
    get label() {
      return t('plan.cycles.phaseType.assimilation')
    },
    chipCls: 'border-ok/30 bg-ok/12 text-ok',
    barCls: 'border-ok/35 bg-ok/14 text-ink',
    dotCls: 'bg-ok',
  },
  taper: {
    get label() {
      return t('plan.cycles.phaseType.taper')
    },
    chipCls: 'border-accent/30 bg-accent/12 text-accent',
    barCls: 'border-accent/30 bg-accent/12 text-ink',
    dotCls: 'bg-accent',
  },
  recovery: {
    get label() {
      return t('plan.cycles.phaseType.recovery')
    },
    chipCls: 'border-line-strong bg-track text-ink-2',
    barCls: 'border-line-strong bg-track/80 text-ink-2',
    dotCls: 'bg-ink-3',
  },
}

const PLAN_STATUS_CLS: Record<CyclePlanStatus, string> = { active: 'border-ok/30 bg-ok/12 text-ok', completed: 'border-info/30 bg-info/12 text-info', archived: 'border-line bg-track text-ink-3' }
export const PLAN_STATUS_LABEL: Record<CyclePlanStatus, { label: string; cls: string }> = lazyRecord(['active', 'completed', 'archived'] as const, (k) => ({
  label: t(`plan.cycles.planStatus.${k}`),
  cls: PLAN_STATUS_CLS[k],
}))

export const PHASE_TYPES = Object.keys(PHASE_TYPE_META) as PhaseType[]
