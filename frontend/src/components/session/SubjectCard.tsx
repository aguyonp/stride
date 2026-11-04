import { CalendarDays, ChevronLeft, ChevronRight, ChevronsUpDown, Footprints, Loader2, MapPin } from 'lucide-react'
import type { ActivitySummary } from '../../api/types'
import { cn } from '../../lib/cn'
import { addDays, capitalize, fmtDateLong, fmtDuration, fmtKm, fmtPace, fmtRelative } from '../../lib/format'
import { ActivityIcon } from '../ui/icons'
import { Skeleton } from '../ui/Skeleton'

interface ActivityCardProps {
  activity: ActivitySummary | null
  locationName?: string | null
  loading?: boolean
  fetching?: boolean
  onOpenPicker: () => void
  className?: string
}

/** Carte « séance sélectionnée » + ouverture du sélecteur. */
export function ActivitySubjectCard({ activity: a, locationName, loading, fetching, onOpenPicker, className }: ActivityCardProps) {
  return (
    <section className={cn('glass animate-in p-3.5 sm:p-4', className)} aria-label="Séance sélectionnée">
      {loading || !a ? (
        <div className="flex items-center gap-3">
          <Skeleton className="size-12 rounded-2xl" />
          <div className="flex-1 space-y-2">
            <Skeleton className="h-4 w-2/3" />
            <Skeleton className="h-3 w-1/2" />
          </div>
        </div>
      ) : (
        <>
          <div className="flex items-center gap-3">
            <span className="grid size-12 flex-none place-items-center rounded-2xl bg-gradient-to-br from-accent/25 to-accent-2/10 text-accent ring-1 ring-accent/25 ring-inset">
              <ActivityIcon type={a.type} className="size-6" />
            </span>
            <div className="min-w-0 flex-1">
              <p className="label-caps flex items-center gap-1.5">
                Séance analysée
                {fetching && <Loader2 className="size-3 animate-spin text-accent" aria-label="Chargement" />}
              </p>
              <h2 className="truncate text-base font-semibold tracking-tight text-ink">{a.name}</h2>
              <p className="truncate text-xs text-ink-3">
                {capitalize(fmtRelative(a.startLocal))} · {a.typeLabel}
                {locationName && (
                  <>
                    {' '}
                    · <MapPin className="inline size-3 align-[-2px]" aria-hidden="true" /> {locationName}
                  </>
                )}
              </p>
            </div>
          </div>
          <dl className="num mt-3 grid grid-cols-3 gap-2 text-center">
            {[
              ['Distance', a.distanceM ? fmtKm(a.distanceM) : '—'],
              ['Durée', fmtDuration(a.durationS)],
              ['Allure', fmtPace(a.avgPaceSecPerKm)],
            ].map(([k, v]) => (
              <div key={k} className="glass-inset rounded-xl px-2 py-1.5">
                <dt className="text-[11px] text-ink-3">{k}</dt>
                <dd className="truncate text-sm font-semibold text-ink">{v}</dd>
              </div>
            ))}
          </dl>
        </>
      )}
      <button type="button" className="btn btn-ghost mt-3 w-full" onClick={onOpenPicker} aria-haspopup="dialog">
        <ChevronsUpDown className="size-4" aria-hidden="true" />
        Changer de séance
      </button>
    </section>
  )
}

interface DayCardProps {
  date: string
  today: string
  fetching?: boolean
  onChangeDate: (iso: string) => void
  onOpenPicker: () => void
  runs: ActivitySummary[]
  onAnalyzeRun: (a: ActivitySummary) => void
  analyzingRunId: number | null
  onBackToDay?: () => void
  className?: string
}

/** Carte « jour analysé » (mode Santé & Récup) : navigation jour par jour + course éventuelle. */
export function DaySubjectCard({ date, today, fetching, onChangeDate, onOpenPicker, runs, onAnalyzeRun, analyzingRunId, onBackToDay, className }: DayCardProps) {
  const isToday = date === today
  const rel = isToday ? 'Aujourd’hui' : date === addDays(today, -1) ? 'Hier' : null
  return (
    <section className={cn('glass animate-in p-3.5 sm:p-4', className)} aria-label="Jour analysé">
      <div className="flex items-center gap-3">
        <span className="grid size-12 flex-none place-items-center rounded-2xl bg-gradient-to-br from-accent/25 to-accent-2/10 text-accent ring-1 ring-accent/25 ring-inset">
          <CalendarDays className="size-6" aria-hidden="true" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="label-caps flex items-center gap-1.5">
            {analyzingRunId ? 'Course du jour' : 'Bilan du jour'}
            {fetching && <Loader2 className="size-3 animate-spin text-accent" aria-label="Chargement" />}
          </p>
          <h2 className="truncate text-base font-semibold tracking-tight text-ink">{capitalize(fmtDateLong(date))}</h2>
          {rel && <p className="text-xs text-ink-3">{rel}</p>}
        </div>
      </div>
      <div className="mt-3 flex items-center gap-2">
        <button type="button" className="icon-btn" onClick={() => onChangeDate(addDays(date, -1))} aria-label="Jour précédent">
          <ChevronLeft className="size-4" />
        </button>
        <button type="button" className="btn btn-ghost flex-1" onClick={onOpenPicker} aria-haspopup="dialog">
          <CalendarDays className="size-4" aria-hidden="true" />
          Choisir une date
        </button>
        <button type="button" className="icon-btn" onClick={() => onChangeDate(addDays(date, 1))} disabled={date >= today} aria-label="Jour suivant">
          <ChevronRight className="size-4" />
        </button>
      </div>
      {runs.length > 0 && (
        <div className="mt-3 space-y-1.5 border-t border-line pt-3">
          {analyzingRunId && onBackToDay ? (
            <button type="button" className="btn btn-ghost w-full" onClick={onBackToDay}>
              <ChevronLeft className="size-4" aria-hidden="true" />
              Revenir au bilan du jour
            </button>
          ) : (
            runs.map((r) => (
              <button
                key={r.id}
                type="button"
                className="flex w-full items-center gap-2.5 rounded-xl border border-accent/30 bg-accent/10 px-3 py-2 text-left text-sm transition-colors hover:bg-accent/15"
                onClick={() => onAnalyzeRun(r)}
              >
                <Footprints className="size-4 flex-none text-accent" aria-hidden="true" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-semibold text-ink">Analyser la course de ce jour</span>
                  <span className="num block truncate text-xs text-ink-3">
                    {r.name} · {fmtKm(r.distanceM)} · {fmtDuration(r.durationS)}
                  </span>
                </span>
                <ChevronRight className="size-4 flex-none text-ink-3" aria-hidden="true" />
              </button>
            ))
          )}
        </div>
      )}
    </section>
  )
}
