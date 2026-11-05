import { CalendarSearch, ChevronLeft, ChevronRight, MapPin } from 'lucide-react'
import type { ActivitySummary, ActivityView } from '../../api/types'
import { cn } from '../../lib/cn'
import { capitalize, fmtDateLong, fmtTimeOfDay } from '../../lib/format'
import { SOURCE_LABEL } from '../../lib/labels'
import { MatchBadge } from '../plan/MatchBadge'
import { FreshnessPill } from '../ui/FreshnessPill'
import { ActivityIcon } from '../ui/icons'
import { Skeleton } from '../ui/Skeleton'

interface Props {
  summary: ActivitySummary | null
  view: ActivityView | null
  loading: boolean
  onOpenPicker: () => void
  onPrev: (() => void) | null
  onNext: (() => void) | null
}

/** Selected session identity: name, date, sources, freshness, picker and previous/next navigation. */
export function SessionHeader({ summary, view, loading, onOpenPicker, onPrev, onNext }: Props) {
  const a = view?.activity ?? summary
  return (
    <section className="glass animate-in p-3 sm:p-4" aria-label="Séance sélectionnée">
      <div className="flex flex-wrap items-center gap-3">
        <span className={cn('grid size-12 flex-none place-items-center rounded-2xl', a?.isRunning ? 'bg-accent/15 text-accent' : 'bg-track text-ink-2')}>
          {a ? <ActivityIcon sport={a.sport} type={a.type} className="size-6" /> : null}
        </span>
        <div className="min-w-0 flex-1 basis-48">
          {a ? (
            <>
              <p className="label-caps">Séance analysée</p>
              <h1 className="truncate text-lg leading-tight font-semibold tracking-tight sm:text-xl">{a.name}</h1>
              <p className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-sm text-ink-3">
                <span>
                  {capitalize(fmtDateLong(a.date))} · {fmtTimeOfDay(a.startLocal)}
                </span>
                <span>· {a.typeLabel}</span>
                {view?.activity.locationName && (
                  <span className="inline-flex min-w-0 items-center gap-1">
                    <MapPin className="size-3.5 flex-none" aria-hidden="true" />
                    <span className="truncate">{view.activity.locationName}</span>
                  </span>
                )}
              </p>
            </>
          ) : loading ? (
            <div className="space-y-1.5">
              <Skeleton className="h-3 w-24" />
              <Skeleton className="h-5 w-56 max-w-full" />
              <Skeleton className="h-3 w-40" />
            </div>
          ) : (
            <p className="text-sm text-ink-3">Aucune séance sélectionnée.</p>
          )}
        </div>
        <div className="flex w-full flex-wrap items-center gap-1.5 sm:w-auto sm:justify-end">
          {view && (
            <span className="chip border border-line bg-track text-ink-2" title={`Sources : ${view.activity.sources.map((s) => SOURCE_LABEL[s] ?? s).join(' + ')}`}>
              {view.sourceLabel}
            </span>
          )}
          {a?.planStatus && a.planStatus !== 'done' && <MatchBadge status={a.planStatus} />}
          <FreshnessPill freshness={view?.freshness} />
          <span className="ml-auto flex items-center gap-1.5 sm:ml-1">
            <button type="button" className="icon-btn size-10" onClick={onPrev ?? undefined} disabled={!onPrev} aria-label="Séance précédente (plus ancienne)">
              <ChevronLeft className="size-4" />
            </button>
            <button type="button" className="btn btn-ghost h-10 px-3" onClick={onOpenPicker}>
              <CalendarSearch className="size-4" aria-hidden="true" />
              Changer
            </button>
            <button type="button" className="icon-btn size-10" onClick={onNext ?? undefined} disabled={!onNext} aria-label="Séance suivante (plus récente)">
              <ChevronRight className="size-4" />
            </button>
          </span>
        </div>
      </div>
    </section>
  )
}
