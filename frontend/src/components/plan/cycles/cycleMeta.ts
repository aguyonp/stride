import type { CyclePlanStatus, PhaseType } from '../../../api/types'

export type CycleZoom = 'macro' | 'meso' | 'micro'

export const PHASE_TYPE_META: Record<
  PhaseType,
  { label: string; chipCls: string; barCls: string; dotCls: string }
> = {
  foundation: {
    label: 'Fondation',
    chipCls: 'border-info/30 bg-info/12 text-info',
    barCls: 'border-info/35 bg-info/14 text-ink',
    dotCls: 'bg-info',
  },
  build: {
    label: 'Développement',
    chipCls: 'border-accent/30 bg-accent/14 text-accent',
    barCls: 'border-accent/35 bg-accent/16 text-ink',
    dotCls: 'bg-accent',
  },
  threshold: {
    label: 'Seuil',
    chipCls: 'border-warn/30 bg-warn/12 text-warn',
    barCls: 'border-warn/35 bg-warn/14 text-ink',
    dotCls: 'bg-warn',
  },
  specific: {
    label: 'Spécifique',
    chipCls: 'border-violet/30 bg-violet/14 text-violet',
    barCls: 'border-violet/35 bg-violet/16 text-ink',
    dotCls: 'bg-violet',
  },
  assimilation: {
    label: 'Assimilation',
    chipCls: 'border-ok/30 bg-ok/12 text-ok',
    barCls: 'border-ok/35 bg-ok/14 text-ink',
    dotCls: 'bg-ok',
  },
  taper: {
    label: 'Affûtage',
    chipCls: 'border-accent/30 bg-accent/12 text-accent',
    barCls: 'border-accent/30 bg-accent/12 text-ink',
    dotCls: 'bg-accent',
  },
  recovery: {
    label: 'Récupération',
    chipCls: 'border-line-strong bg-track text-ink-2',
    barCls: 'border-line-strong bg-track/80 text-ink-2',
    dotCls: 'bg-ink-3',
  },
}

export const PLAN_STATUS_LABEL: Record<CyclePlanStatus, { label: string; cls: string }> = {
  active: { label: 'Actif', cls: 'border-ok/30 bg-ok/12 text-ok' },
  completed: { label: 'Terminé', cls: 'border-info/30 bg-info/12 text-info' },
  archived: { label: 'Archivé', cls: 'border-line bg-track text-ink-3' },
}

export const PHASE_TYPES = Object.keys(PHASE_TYPE_META) as PhaseType[]
