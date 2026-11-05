import { MoreHorizontal, Pencil, Trash2, Watch } from 'lucide-react'
import type { ActivitySummary, PlannedSession } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtDuration, fmtKm } from '../../lib/format'
import { PLAN_SPORT_LABEL } from '../../lib/labels'
import { PlanSportIcon } from '../ui/icons'
import { MenuItem, Popover } from '../ui/Popover'
import { MatchBadge } from './MatchBadge'
import { matchDeltas, targetParts } from './planFormat'
import { GarminStatus } from './GarminStatus'
import { stepsSummary } from './steps'

interface PlannedSessionCardProps {
  session: PlannedSession
  /** Activity matched to this session (from the same day), shown as « Réalisé : … ». */
  doneActivity?: ActivitySummary | null
  compact?: boolean
  onEdit?: () => void
  onDelete?: () => void
  onOpen?: () => void
  className?: string
}

/** Planned session: sport, title, targets, match status and deltas. Actions appear only when handlers are given. */
export function PlannedSessionCard({ session: s, doneActivity, compact, onEdit, onDelete, onOpen, className }: PlannedSessionCardProps) {
  const targets = targetParts(s)
  const deltas = s.match ? matchDeltas(s.match) : []
  const isRest = s.sport === 'rest'
  return (
    <article className={cn('glass-inset min-w-0', compact ? 'p-2.5' : 'p-3', isRest && 'border-dashed', className)} aria-label={`${PLAN_SPORT_LABEL[s.sport]} : ${s.title}`}>
      <div className="flex min-h-8 items-center gap-2">
        <span className={cn('grid size-7 flex-none place-items-center rounded-lg', isRest ? 'bg-track text-ink-3' : 'bg-accent/12 text-accent')}>
          <PlanSportIcon sport={s.sport} className="size-3.5" />
        </span>
        <p className={cn('min-w-0 flex-1 truncate text-[11px] font-semibold tracking-wide text-ink-3 uppercase', compact && 'max-2xl:text-transparent max-2xl:select-none')}>{PLAN_SPORT_LABEL[s.sport]}</p>
        {(onEdit || onDelete) && (
          <Popover
            className="-mr-1 flex-none"
            trigger={(p) => (
              <button {...p} type="button" className="grid size-8 place-items-center rounded-full text-ink-3 hover:bg-track hover:text-ink" aria-label={`Actions pour « ${s.title} »`}>
                <MoreHorizontal className="size-4" aria-hidden="true" />
              </button>
            )}
            panelClassName="w-48"
          >
            {(close) => (
              <>
                {onOpen && (
                  <MenuItem icon={<Watch />} onSelect={() => (close(), onOpen())}>
                    Détails & montre
                  </MenuItem>
                )}
                {onEdit && (
                  <MenuItem icon={<Pencil />} onSelect={() => (close(), onEdit())}>
                    Modifier
                  </MenuItem>
                )}
                {onDelete && (
                  <MenuItem icon={<Trash2 />} danger onSelect={() => (close(), onDelete())}>
                    Supprimer
                  </MenuItem>
                )}
              </>
            )}
          </Popover>
        )}
      </div>
      <h3 className="mt-1.5 text-sm leading-snug font-semibold break-words hyphens-auto text-ink">
        {onOpen ? (
          <button type="button" onClick={onOpen} className="text-left hover:text-accent focus-visible:outline-2 focus-visible:outline-accent">
            {s.title || 'Séance'}
          </button>
        ) : (
          s.title || 'Séance'
        )}
      </h3>
      {s.steps?.length > 0 && (
        <p className="num mt-1 line-clamp-2 text-[11px] text-ink-2" title={stepsSummary(s.steps)}>
          {stepsSummary(s.steps)}
        </p>
      )}
      <div className="mt-1">
        <GarminStatus s={s} compact />
      </div>

      {targets.length > 0 && (
        <ul className="mt-2 flex flex-wrap gap-1" aria-label="Cibles">
          {targets.map((t) => (
            <li key={t} className="num rounded-lg bg-track px-1.5 py-0.5 text-[11px] font-semibold text-ink-2">
              {t}
            </li>
          ))}
        </ul>
      )}

      {s.instructions && !compact && <p className="mt-2 line-clamp-3 text-xs break-words text-ink-3">{s.instructions}</p>}
      {s.instructions && compact && (
        <p className="mt-2 line-clamp-2 text-xs break-words text-ink-3" title={s.instructions}>
          {s.instructions}
        </p>
      )}

      {s.match && (
        <div className="mt-2.5 border-t border-line pt-2">
          <MatchBadge status={s.match.status} />
          {doneActivity && (
            <p className={cn('num mt-1.5 text-xs text-ink-2', !compact && 'truncate')} title={doneActivity.name}>
              Réalisé : {doneActivity.distanceM ? `${fmtKm(doneActivity.distanceM, 1)} · ` : ''}
              {fmtDuration(doneActivity.durationS)}
            </p>
          )}
          {deltas.length > 0 && (
            <ul className="num mt-1 flex flex-wrap gap-x-2 gap-y-0.5 text-[11px] text-ink-3">
              {deltas.map((d) => (
                <li key={d}>{d}</li>
              ))}
            </ul>
          )}
          {s.match.summary && !compact && <p className="mt-1 text-xs break-words text-ink-2">{s.match.summary}</p>}
          {s.match.summary && compact && (
            <p className="mt-1 line-clamp-2 text-[11px] break-words text-ink-3" title={s.match.summary}>
              {s.match.summary}
            </p>
          )}
        </div>
      )}
    </article>
  )
}
