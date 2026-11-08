import type { CycleActuals } from '../../../api/types'
import { cn } from '../../../lib/cn'
import { fmtNumber, fmtPct } from '../../../lib/format'
import { gaugeFill, sessionsPct } from './cycleModel'
import { t } from '../../../i18n'

function DualGauge({
  label,
  actual,
  planned,
  pct,
  unit,
  decimals = 0,
  compact = false,
}: {
  label: string
  actual: number
  planned: number
  pct: number | null
  unit: string
  decimals?: number
  compact?: boolean
}) {
  const fill = gaugeFill(actual, planned, pct)
  return (
    <div className="min-w-0">
      <div className="flex items-baseline justify-between gap-2 text-xs">
        <span className="text-ink-3">{label}</span>
        <span className="num font-medium text-ink-2">
          {fmtNumber(actual, decimals)}
          <span className="text-ink-3">
            {' '}
            / {planned > 0 ? fmtNumber(planned, decimals) : '—'} {unit}
          </span>
          {fill.pct !== null && <span className={cn('ml-1.5', fill.overload ? 'text-warn' : 'text-ink-3')}>({fmtPct(fill.pct, 0)})</span>}
        </span>
      </div>
      <div
        className={cn('mt-1 overflow-hidden rounded-full bg-track', compact ? 'h-1.5' : 'h-2')}
        role="progressbar"
        aria-label={t('plan.cycles.gaugeAria', { label, actual: fmtNumber(actual, decimals), planned: planned > 0 ? fmtNumber(planned, decimals) : 0, unit })}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={fill.width}
      >
        <div
          className={cn('h-full rounded-full transition-[width] duration-500', fill.overload ? 'bg-warn' : 'bg-accent')}
          style={{ width: `${fill.width}%` }}
        />
      </div>
    </div>
  )
}

/** Compact volume + load gauges; planned values default to the actuals' own plan. */
export function VolumeLoadGauges({
  actuals,
  plannedKm = actuals.plannedVolumeKm,
  plannedLoad = actuals.plannedLoad,
  volumeLabel = t('plan.cycles.volume'),
  loadLabel = t('plan.cycles.load'),
}: {
  actuals: CycleActuals
  plannedKm?: number
  plannedLoad?: number
  volumeLabel?: string
  loadLabel?: string
}) {
  return (
    <>
      <DualGauge
        label={volumeLabel}
        actual={actuals.actualVolumeKm}
        planned={plannedKm}
        pct={actuals.volumeCompletionPct}
        unit="km"
        decimals={1}
        compact
      />
      <DualGauge
        label={loadLabel}
        actual={actuals.actualLoad}
        planned={plannedLoad}
        pct={actuals.loadCompletionPct}
        unit={t('plan.cycles.pts')}
        decimals={0}
        compact
      />
    </>
  )
}

export function GlobalGauges({ totals }: { totals: CycleActuals }) {
  return (
    <div className="grid gap-4 sm:grid-cols-3">
      <DualGauge
        label={t('plan.cycles.cumVolume')}
        actual={totals.actualVolumeKm}
        planned={totals.plannedVolumeKm}
        pct={totals.volumeCompletionPct}
        unit="km"
        decimals={1}
      />
      <DualGauge
        label={t('plan.cycles.cumLoad')}
        actual={totals.actualLoad}
        planned={totals.plannedLoad}
        pct={totals.loadCompletionPct}
        unit={t('plan.cycles.pts')}
        decimals={0}
      />
      <DualGauge
        label={t('plan.cycles.sessionsDone')}
        actual={totals.sessionsCompleted}
        planned={totals.sessionsPlanned}
        pct={sessionsPct(totals)}
        unit={t('plan.cycles.sessionsUnit')}
        decimals={0}
      />
    </div>
  )
}
