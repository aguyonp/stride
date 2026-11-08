import { Pencil } from 'lucide-react'
import type { TrainingPhase, TrainingWeek } from '../../../api/types'
import { cn } from '../../../lib/cn'
import { fmtDateShort, fmtNumber } from '../../../lib/format'
import { PHASE_TYPE_META, type CycleZoom } from './cycleMeta'
import { isCurrentSpan } from './cycleModel'
import { VolumeLoadGauges } from './Gauges'
import { t } from '../../../i18n'

export function PhaseList({
  phases,
  today,
  zoom,
  onEditPhase,
  onSelectWeek,
}: {
  phases: TrainingPhase[]
  today: string
  zoom: CycleZoom
  onEditPhase: (p: TrainingPhase) => void
  onSelectWeek: (w: TrainingWeek) => void
}) {
  if (phases.length === 0) return <p className="text-sm text-ink-3">{t('plan.cycles.noPhase')}</p>
  return (
    <ul className="divide-y divide-line">
      {phases.map((p) => {
        const meta = PHASE_TYPE_META[p.phaseType] ?? PHASE_TYPE_META.build
        const isCurrent = isCurrentSpan(p.startDate, p.endDate, today)
        return (
          <li key={p.id} className="py-3.5 first:pt-0 last:pb-0">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h3 className="text-sm font-semibold text-ink">{p.name}</h3>
                  <span className={cn('chip h-5 border px-2 text-[11px]', meta.chipCls)}>{meta.label}</span>
                  {isCurrent && <span className="chip h-5 bg-accent/15 px-2 text-[10px] text-accent">{t('plan.cycles.current')}</span>}
                  <span className="num text-xs text-ink-3">
                    {fmtDateShort(p.startDate)} → {fmtDateShort(p.endDate)} · {p.weeks.length} sem.
                  </span>
                </div>
                {p.focus && <p className="mt-0.5 text-xs text-ink-2">{p.focus}</p>}
              </div>
              <div className="flex items-center gap-2">
                <span className="num text-xs text-ink-3">
                  {t('plan.cycles.sessionsCount', { done: p.actuals.sessionsCompleted, planned: p.actuals.sessionsPlanned })}
                </span>
                <button
                  type="button"
                  className="icon-btn size-8"
                  onClick={() => onEditPhase(p)}
                  aria-label={t('plan.cycles.editPhase', { name: p.name })}
                >
                  <Pencil className="size-3.5" />
                </button>
              </div>
            </div>

            <div className="mt-2.5 grid gap-3 sm:grid-cols-2">
              <VolumeLoadGauges actuals={p.actuals} />
            </div>

            {zoom === 'meso' && p.weeks.length > 0 && (
              <div className="mt-3 flex flex-wrap gap-1.5">
                {p.weeks.map((w) => (
                  <button
                    key={w.weekStart}
                    type="button"
                    onClick={() => onSelectWeek(w)}
                    className={cn(
                      'inline-flex items-center gap-1.5 rounded-xl border px-2.5 py-1 text-xs transition-colors hover:bg-track',
                      w.isAssimilation ? 'border-ok/35 bg-ok/[0.08] text-ink' : 'border-line bg-track/30 text-ink-2',
                    )}
                  >
                    <span className="num font-semibold text-ink">{t('common.extra.weekShort', { n: w.weekNumber })}</span>
                    <span className="num text-ink-3">
                      {fmtNumber(w.actuals.actualVolumeKm, 0)}/{fmtNumber(w.targetVolumeKm ?? w.actuals.plannedVolumeKm, 0)} km
                    </span>
                    {w.isAssimilation && <span className="text-[10px] font-semibold text-ok">{t('plan.cycles.assimWeek')}</span>}
                  </button>
                ))}
              </div>
            )}
          </li>
        )
      })}
    </ul>
  )
}
