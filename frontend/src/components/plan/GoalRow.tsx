import { Archive, CalendarClock, CircleCheck, MoreHorizontal, Pencil, RotateCcw, Target, Trash2 } from 'lucide-react'
import type { Goal } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtDateShort, fmtNumber, isNum } from '../../lib/format'
import { GOAL_KIND_LABEL } from '../../lib/labels'
import { MenuItem, Popover } from '../ui/Popover'
import { relativeDays } from './planFormat'
import { t } from '../../i18n'

interface GoalRowProps {
  goal: Goal
  today: string
  onEdit?: () => void
  onStatus?: (status: Goal['status']) => void
  onDelete?: () => void
  className?: string
}

function goalTarget(g: Goal): string | null {
  if (!isNum(g.targetValue)) return null
  const v = fmtNumber(g.targetValue, Number.isInteger(g.targetValue) ? 0 : 1)
  return g.targetUnit ? `${v} ${g.targetUnit}` : v
}

/** Goal line: kind, title, target, due date, note. Actions menu only when handlers are given. */
export function GoalRow({ goal: g, today, onEdit, onStatus, onDelete, className }: GoalRowProps) {
  const target = goalTarget(g)
  const overdue = g.status === 'active' && g.targetDate !== null && g.targetDate < today
  const hasActions = !!(onEdit || onStatus || onDelete)
  return (
    <li className={cn('flex min-w-0 items-start gap-3 rounded-2xl border border-line px-3 py-3', g.status !== 'active' && 'opacity-80', className)}>
      <span className={cn('grid size-9 flex-none place-items-center rounded-xl', g.status === 'done' ? 'bg-ok/12 text-ok' : 'bg-accent/12 text-accent')}>
        {g.status === 'done' ? <CircleCheck className="size-[1.125rem]" aria-hidden="true" /> : <Target className="size-[1.125rem]" aria-hidden="true" />}
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-[11px] font-semibold tracking-wide text-ink-3 uppercase">
          {GOAL_KIND_LABEL[g.kind]}
          {g.status === 'done' && <span className="text-ok">{t('plan.goals.reached')}</span>}
          {g.status === 'archived' && <span>{t('plan.goals.archived')}</span>}
        </p>
        <p className="text-sm font-semibold break-words text-ink">{g.title}</p>
        {(target || g.targetDate) && (
          <p className="num mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-ink-2">
            {target && <span>{t('plan.goals.target', { value: target })}</span>}
            {g.targetDate && (
              <span className={cn('inline-flex items-center gap-1', overdue && 'text-warn')}>
                <CalendarClock className="size-3.5" aria-hidden="true" />
                {fmtDateShort(g.targetDate)} · {relativeDays(g.targetDate, today)}
              </span>
            )}
          </p>
        )}
        {g.note && <p className="mt-1 line-clamp-3 text-xs break-words text-ink-3">{g.note}</p>}
      </div>
      {hasActions && (
        <Popover
          className="-mr-1 flex-none"
          trigger={(p) => (
            <button {...p} type="button" className="grid size-9 place-items-center rounded-full text-ink-3 hover:bg-track hover:text-ink" aria-label={t('plan.goals.actions', { title: g.title })}>
              <MoreHorizontal className="size-4" aria-hidden="true" />
            </button>
          )}
          panelClassName="w-52"
        >
          {(close) => (
            <>
              {onEdit && (
                <MenuItem icon={<Pencil />} onSelect={() => (close(), onEdit())}>
                  {t('plan.goals.edit')}
                </MenuItem>
              )}
              {onStatus && g.status === 'active' && (
                <MenuItem icon={<CircleCheck />} onSelect={() => (close(), onStatus('done'))}>
                  {t('plan.goals.markDone')}
                </MenuItem>
              )}
              {onStatus && g.status !== 'archived' && (
                <MenuItem icon={<Archive />} onSelect={() => (close(), onStatus('archived'))}>
                  {t('plan.goals.archive')}
                </MenuItem>
              )}
              {onStatus && g.status !== 'active' && (
                <MenuItem icon={<RotateCcw />} onSelect={() => (close(), onStatus('active'))}>
                  {t('plan.goals.reactivate')}
                </MenuItem>
              )}
              {onDelete && (
                <MenuItem icon={<Trash2 />} danger onSelect={() => (close(), onDelete())}>
                  {t('plan.goals.delete')}
                </MenuItem>
              )}
            </>
          )}
        </Popover>
      )}
    </li>
  )
}
