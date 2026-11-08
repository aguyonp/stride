import { CalendarSearch, ChevronLeft, ChevronRight, GitMerge, MapPin, Pencil } from 'lucide-react'
import type { ReactNode } from 'react'
import type { ActivitySummary, ActivityView } from '../../api/types'
import { cn } from '../../lib/cn'
import { capitalize, fmtDateLong, fmtTimeOfDay } from '../../lib/format'
import { SOURCE_LABEL } from '../../lib/labels'
import { MatchBadge } from '../plan/MatchBadge'
import { FreshnessPill } from '../ui/FreshnessPill'
import { ActivityIcon } from '../ui/icons'
import { Skeleton } from '../ui/Skeleton'
import { KudosBadge } from './RecordsCard'
import { RemarkMarker } from '../share/ShareMarkers'
import { t } from '../../i18n'

interface Props {
  summary: ActivitySummary | null
  view: ActivityView | null
  loading: boolean
  onOpenPicker: () => void
  onPrev: (() => void) | null
  onNext: (() => void) | null
  /** Manual activities only: opens the edit form. */
  onEditManual?: () => void
  children?: ReactNode
}

/** Selected session identity: name, date, sources, freshness, picker and previous/next navigation. */
export function SessionHeader({ summary, view, loading, onOpenPicker, onPrev, onNext, onEditManual, children }: Props) {
  const a = view?.activity ?? summary
  const manual = a?.sources.includes('manual') ?? false
  return (
    <section className="glass animate-in p-4 sm:p-5" aria-label={t('dashboard.session.selected')}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <span className={cn('hidden size-11 flex-none place-items-center rounded-2xl sm:grid', a?.isRunning ? 'bg-accent/15 text-accent' : 'bg-track text-ink-2')}>
          {a ? <ActivityIcon sport={a.sport} type={a.type} className="size-6" /> : null}
        </span>
        <div className="min-w-0 flex-1 basis-48">
          {a ? (
            <>
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
            <p className="text-sm text-ink-3">{t('dashboard.session.noneSelected')}</p>
          )}
        </div>
        <div className="flex w-full flex-wrap items-center gap-1.5 sm:w-auto sm:justify-end">
          {view &&
            (view.activity.sources.length > 1 ? (
              <span className="chip border border-violet/30 bg-violet/10 text-violet" title={t('dashboard.session.mergedHint')}>
                <GitMerge className="size-3.5" aria-hidden="true" />
                Garmin + Strava
              </span>
            ) : (
              <span className="chip border border-line bg-track text-ink-2" title={t('dashboard.session.source', { sources: view.activity.sources.map((s) => SOURCE_LABEL[s] ?? s).join(' + ') })}>
                {view.sourceLabel}
              </span>
            ))}
          {manual && onEditManual && (
            <button type="button" className="chip border border-line bg-transparent text-ink-2 hover:bg-track" onClick={onEditManual}>
              <Pencil className="size-3.5" aria-hidden="true" /> {t('dashboard.session.edit')}
            </button>
          )}
          <RemarkMarker count={a?.remarkCount} />
          <KudosBadge count={a?.kudos} className="px-1" />
          {a?.planStatus && a.planStatus !== 'done' && <MatchBadge status={a.planStatus} />}
          {!manual && <FreshnessPill freshness={view?.freshness} className="max-sm:hidden" />}
          <span className="ml-auto flex items-center gap-1.5 sm:ml-1">
            <button type="button" className="icon-btn size-10" onClick={onPrev ?? undefined} disabled={!onPrev} aria-label={t('dashboard.session.prev')}>
              <ChevronLeft className="size-4" />
            </button>
            <button type="button" className="btn btn-ghost h-10 px-3" onClick={onOpenPicker}>
              <CalendarSearch className="size-4" aria-hidden="true" />
              <span className="max-sm:sr-only">{t('dashboard.session.change')}</span>
            </button>
            <button type="button" className="icon-btn size-10" onClick={onNext ?? undefined} disabled={!onNext} aria-label={t('dashboard.session.next')}>
              <ChevronRight className="size-4" />
            </button>
          </span>
        </div>
      </div>
      {children}
    </section>
  )
}
