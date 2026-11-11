import type { TrainingPhase, TrainingWeek } from '../../../api/types'
import { cn } from '../../../lib/cn'
import { fmtDateShort } from '../../../lib/format'
import { isCurrentSpan } from './cycleModel'
import { VolumeLoadGauges } from './Gauges'
import { t } from '../../../i18n'

export function MicrocycleList({
  weeks,
  phases,
  today,
  onTuneWeek,
  onJumpToWeek,
}: {
  weeks: TrainingWeek[]
  phases: TrainingPhase[]
  today: string
  onTuneWeek: (w: TrainingWeek) => void
  onJumpToWeek: (weekStart: string) => void
}) {
  return (
    <div className="divide-y divide-line">
      {weeks.map((w) => {
        const isCurrent = isCurrentSpan(w.weekStart, w.weekEnd, today)
        const phase = phases.find((p) => p.id === w.phaseId)
        const plannedKm = w.targetVolumeKm ?? w.actuals.plannedVolumeKm
        const plannedLoad = w.targetLoad ?? w.actuals.plannedLoad
        return (
          <div key={w.weekStart} className="flex flex-col gap-3 py-3 first:pt-0 last:pb-0 lg:flex-row lg:items-center lg:justify-between">
            <div className="min-w-0 lg:w-72">
              <div className="flex flex-wrap items-center gap-1.5">
                <span className={cn('num text-sm font-semibold', isCurrent ? 'text-accent' : 'text-ink')}>
                  S{w.weekNumber}
                </span>
                <span className="num text-xs text-ink-3">
                  {fmtDateShort(w.weekStart)} → {fmtDateShort(w.weekEnd)}
                </span>
                {w.isAssimilation && (
                  <span className="chip h-5 border border-ok/30 bg-ok/12 px-2 text-[10px] text-ok">
                    {t('plan.cycles.assimWeek')}
                  </span>
                )}
              </div>
              <p className="mt-0.5 truncate text-xs text-ink-3">
                {phase ? phase.name : t('plan.cycles.outsideMeso')}
                {w.note ? ` · ${w.note}` : ''}
              </p>
            </div>

            <div className="grid flex-1 gap-3 sm:grid-cols-2">
              <VolumeLoadGauges actuals={w.actuals} plannedKm={plannedKm} plannedLoad={plannedLoad} />
            </div>

            <div className="flex flex-none items-center justify-end gap-1.5">
              <span className="num mr-1 text-xs text-ink-3">
                {w.actuals.sessionsCompleted}/{w.actuals.sessionsPlanned}
              </span>
              <button
                type="button"
                className="btn btn-ghost h-8 min-h-8 px-2.5 text-xs"
                onClick={() => onTuneWeek(w)}
              >
                {t('common.misc.adjust')}
              </button>
              <button
                type="button"
                className="btn btn-ghost h-8 min-h-8 px-2.5 text-xs"
                onClick={() => onJumpToWeek(w.weekStart)}
              >
                {t('common.misc.week')}
              </button>
            </div>
          </div>
        )
      })}
    </div>
  )
}
