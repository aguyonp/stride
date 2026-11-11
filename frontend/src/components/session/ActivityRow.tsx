import { Check } from 'lucide-react'
import type { ActivitySummary } from '../../api/types'
import { cn } from '../../lib/cn'
import { capitalize, fmtDuration, fmtKm, fmtNumber, fmtPace, fmtRelative } from '../../lib/format'
import { KudosBadge } from '../dashboard/RecordsCard'
import { ActivityIcon } from '../ui/icons'
import { t } from '../../i18n'

export function ActivityRow({ activity: a, selected, onSelect }: { activity: ActivitySummary; selected: boolean; onSelect: () => void }) {
  return (
    <li>
      <button
        type="button"
        onClick={onSelect}
        aria-current={selected ? 'true' : undefined}
        className={cn('group flex w-full items-center gap-3 rounded-2xl border px-3 py-2.5 text-left transition-colors', selected ? 'border-accent/50 bg-accent/10' : 'border-transparent hover:bg-track')}
      >
        <span className={cn('grid size-10 flex-none place-items-center rounded-xl', a.isRunning ? 'bg-accent/15 text-accent' : 'bg-track text-ink-2')}>
          <ActivityIcon sport={a.sport} type={a.type} className="size-[1.125rem]" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="flex min-w-0 items-center gap-1.5">
            <span className="truncate text-sm font-semibold text-ink">{a.name}</span>
            <KudosBadge count={a.kudos} className="flex-none" />
          </span>
          <span className="block truncate text-xs text-ink-3">
            {capitalize(fmtRelative(a.startLocal))} · {a.typeLabel}
            {a.painDuring !== null && <span className={cn('ml-1', a.painDuring >= 3 ? 'text-warn' : '')}>{t('common.ui.pain', { value: fmtNumber(a.painDuring, 1, { trim: true }) })}</span>}
            {!!a.remarkCount && <span className="ml-1 text-info">{t('session.calendar.remark')}</span>}
          </span>
        </span>
        <span className="num flex-none text-right text-xs">
          <span className="block font-semibold text-ink">{a.distanceM ? fmtKm(a.distanceM) : fmtDuration(a.durationS)}</span>
          <span className="block text-ink-3">{a.distanceM ? `${fmtDuration(a.durationS)} · ${fmtPace(a.avgPaceSecPerKm)}` : '—'}</span>
        </span>
        {selected && <Check className="size-4 flex-none text-accent" aria-hidden="true" />}
      </button>
    </li>
  )
}
