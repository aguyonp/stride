import type { TrainingPhase, TrainingWeek } from '../../../api/types'
import { cn } from '../../../lib/cn'
import { fmtDateShort, fmtNumber } from '../../../lib/format'
import { PHASE_TYPE_META, type CycleZoom } from './cycleMeta'
import { ganttScale, isCurrentSpan, phaseBar } from './cycleModel'
import { t } from '../../../i18n'

export function GanttStrip({
  startDate,
  endDate,
  today,
  phases,
  allWeeks,
  zoom,
  onSelectPhase,
  onSelectWeek,
}: {
  startDate: string
  endDate: string
  today: string
  phases: TrainingPhase[]
  allWeeks: TrainingWeek[]
  zoom: CycleZoom
  onSelectPhase: (p: TrainingPhase) => void
  onSelectWeek: (w: TrainingWeek) => void
}) {
  const { totalDays, todayPct } = ganttScale(startDate, endDate, today)
  const minWidthCls = zoom === 'micro' ? 'min-w-[66rem]' : zoom === 'meso' ? 'min-w-[44rem]' : 'min-w-full'

  return (
    <div className="scroll-thin overflow-x-auto rounded-2xl border border-line bg-track/30 p-3.5">
      <div className={cn('relative space-y-2.5', minWidthCls)}>
        <div className="num flex items-center justify-between text-[11px] text-ink-3">
          <span>{fmtDateShort(startDate)}</span>
          {todayPct !== null && <span className="font-semibold text-accent">{t('plan.cycles.today', { date: fmtDateShort(today) })}</span>}
          <span>{fmtDateShort(endDate)}</span>
        </div>

        <div className="relative min-h-14 rounded-xl border border-line bg-surface/40 p-1.5">
          {todayPct !== null && (
            <div
              className="pointer-events-none absolute inset-y-0 z-10 w-px bg-accent"
              style={{ left: `${todayPct}%` }}
              aria-hidden="true"
            >
              <span className="absolute -top-1.5 -translate-x-1/2 rounded-full bg-accent px-1.5 py-0.5 text-[9px] font-semibold text-on-accent">
                {t('common.misc.dayMarker')}
              </span>
            </div>
          )}

          {phases.length > 0 ? (
            <div className="relative h-12 w-full">
              {phases.map((p) => {
                const { leftPct, widthPct } = phaseBar(startDate, totalDays, p)
                const meta = PHASE_TYPE_META[p.phaseType] ?? PHASE_TYPE_META.build
                return (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() => onSelectPhase(p)}
                    style={{ left: `${leftPct}%`, width: `calc(${widthPct}% - 2px)` }}
                    className={cn(
                      'absolute inset-y-0 flex flex-col justify-between overflow-hidden rounded-lg border px-2 py-1 text-left transition-opacity hover:opacity-90',
                      meta.barCls,
                    )}
                    aria-label={t('plan.cycles.mesoAria', { name: p.name, type: meta.label, from: fmtDateShort(p.startDate), to: fmtDateShort(p.endDate) })}
                  >
                    <span className="flex items-center gap-1.5 truncate text-xs font-semibold">
                      <span className={cn('size-2 flex-none rounded-full', meta.dotCls)} aria-hidden="true" />
                      <span className="truncate">{p.name}</span>
                    </span>
                    <span className="num truncate text-[10px] text-ink-2">
                      {meta.label} · {fmtNumber(p.actuals.actualVolumeKm, 0)}/{fmtNumber(p.actuals.plannedVolumeKm, 0)} km
                    </span>
                  </button>
                )
              })}
            </div>
          ) : (
            <p className="py-3 text-center text-xs text-ink-3">{t('plan.cycles.noMeso')}</p>
          )}
        </div>

        {allWeeks.length > 0 && (
          <div
            className="grid gap-1"
            style={{ gridTemplateColumns: `repeat(${allWeeks.length}, minmax(0, 1fr))` }}
            aria-label={t('plan.cycles.weeklyMarks')}
          >
            {allWeeks.map((w) => {
              const isCurrent = isCurrentSpan(w.weekStart, w.weekEnd, today)
              const targetKm = w.targetVolumeKm ?? w.actuals.plannedVolumeKm
              return (
                <button
                  key={`${w.weekStart}-${w.id}`}
                  type="button"
                  onClick={() => onSelectWeek(w)}
                  className={cn(
                    'flex min-w-0 flex-col rounded-lg border px-1.5 py-1 text-left transition-colors hover:bg-track',
                    isCurrent
                      ? 'border-accent/55 bg-accent/10'
                      : w.isAssimilation
                        ? 'border-ok/35 bg-ok/[0.07]'
                        : 'border-line bg-surface/30',
                  )}
                  aria-label={t('plan.cycles.weekAria', { n: w.weekNumber, date: fmtDateShort(w.weekStart), assim: w.isAssimilation ? t('plan.cycles.assimSuffix') : '' })}
                >
                  <span className="flex items-center justify-between gap-1">
                    <span className={cn('num text-[10px] font-semibold', isCurrent ? 'text-accent' : 'text-ink-2')}>
                      S{w.weekNumber}
                    </span>
                    {w.isAssimilation && (
                      <span className="truncate rounded-sm bg-ok/20 px-1 text-[9px] font-semibold text-ok" title={t('plan.cycles.assimWeek')}>
                        {t('common.misc.assimShort')}
                      </span>
                    )}
                  </span>
                  {zoom !== 'macro' && (
                    <span className="num mt-0.5 truncate text-[10px] text-ink-3">
                      {fmtNumber(w.actuals.actualVolumeKm, 0)}/{targetKm ? fmtNumber(targetKm, 0) : '—'} km
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}
