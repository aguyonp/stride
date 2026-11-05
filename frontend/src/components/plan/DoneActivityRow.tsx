import type { ActivitySummary } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtDuration, fmtKm, fmtPace } from '../../lib/format'
import { ActivityIcon } from '../ui/icons'
import { MatchBadge } from './MatchBadge'

/** Activity done that day; `extra` flags it « Hors plan » (no matching planned session). */
export function DoneActivityRow({ activity: a, extra = true, className }: { activity: ActivitySummary; extra?: boolean; className?: string }) {
  return (
    <div className={cn('min-w-0 rounded-2xl border border-line px-3 py-2.5', className)}>
      <div className="flex min-w-0 items-start gap-2.5">
        <span className="grid size-8 flex-none place-items-center rounded-xl bg-track text-ink-2">
          <ActivityIcon sport={a.sport} type={a.type} className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold text-ink" title={a.name}>
            {a.name}
          </p>
          <p className="num truncate text-xs text-ink-3">
            {a.distanceM ? `${fmtKm(a.distanceM, 1)} · ` : ''}
            {fmtDuration(a.durationS)}
            {a.isRunning && a.avgPaceSecPerKm ? ` · ${fmtPace(a.avgPaceSecPerKm)}` : ''}
          </p>
        </div>
      </div>
      {extra && <MatchBadge status="extra" className="mt-2" />}
    </div>
  )
}
