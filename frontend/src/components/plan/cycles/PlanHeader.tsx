import { Flag, Pencil, Plus, Trash2 } from 'lucide-react'
import type { TrainingPlan } from '../../../api/types'
import { cn } from '../../../lib/cn'
import { SegmentedControl } from '../../ui/SegmentedControl'
import { PLAN_STATUS_LABEL, type CycleZoom } from './cycleMeta'
import { countdownLabel } from './cycleModel'
import { t } from '../../../i18n'

export function CyclesToolbar({
  plans,
  currentPlan,
  zoom,
  onSelectPlan,
  onZoom,
  onEditPlan,
  onDeletePlan,
  onNewPlan,
}: {
  plans: TrainingPlan[]
  currentPlan: TrainingPlan | null
  zoom: CycleZoom
  onSelectPlan: (id: number | null) => void
  onZoom: (z: CycleZoom) => void
  onEditPlan: (p: TrainingPlan) => void
  onDeletePlan: (p: TrainingPlan) => void
  onNewPlan: () => void
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      {plans.length > 1 && (
        <select
          aria-label={t('plan.cycles.choosePlan')}
          className="field h-9 min-h-9 w-auto py-1 text-xs"
          value={currentPlan?.id ?? ''}
          onChange={(e) => onSelectPlan(Number(e.target.value) || null)}
        >
          {plans.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
      )}
      <SegmentedControl<CycleZoom>
        ariaLabel={t('plan.cycles.zoom')}
        size="sm"
        value={zoom}
        onChange={onZoom}
        options={[
          { value: 'macro', label: t('plan.cycles.macro') },
          { value: 'meso', label: t('plan.cycles.meso') },
          { value: 'micro', label: t('plan.cycles.micro') },
        ]}
      />
      {currentPlan && (
        <>
          <button
            type="button"
            className="icon-btn size-9"
            onClick={() => onEditPlan(currentPlan)}
            aria-label={t('plan.cycles.editPlan')}
          >
            <Pencil className="size-4" />
          </button>
          <button
            type="button"
            className="icon-btn size-9"
            onClick={() => onDeletePlan(currentPlan)}
            aria-label={t('plan.cycles.deletePlan')}
          >
            <Trash2 className="size-4" />
          </button>
        </>
      )}
      <button
        type="button"
        className="btn btn-primary h-9 min-h-9 px-3 text-xs"
        onClick={onNewPlan}
      >
        <Plus className="size-4" aria-hidden="true" /> Nouveau macrocycle
      </button>
    </div>
  )
}

export function PlanStatusBar({ plan, today }: { plan: TrainingPlan; today: string }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 rounded-2xl border border-line bg-track/30 px-3.5 py-2.5">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <span className={cn('chip border', PLAN_STATUS_LABEL[plan.status].cls)}>
          {PLAN_STATUS_LABEL[plan.status].label}
        </span>
        {plan.targetEventLabel && (
          <span className="inline-flex items-center gap-1.5 font-medium text-ink">
            <Flag className="size-3.5 text-accent" aria-hidden="true" />
            {plan.targetEventLabel}
          </span>
        )}
        {plan.note && <span className="text-ink-3">· {plan.note}</span>}
      </div>
      <span className="num text-xs font-semibold text-accent">{countdownLabel(today, plan.endDate)}</span>
    </div>
  )
}
