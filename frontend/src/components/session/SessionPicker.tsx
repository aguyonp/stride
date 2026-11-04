import { CalendarDays, List, Loader2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useActivities, useActivitiesInRange } from '../../api/queries'
import type { ActivitySummary } from '../../api/types'
import { useMediaQuery } from '../../hooks/useMediaQuery'
import { capitalize, fmtDateLong, parseLocal } from '../../lib/format'
import { Dialog } from '../ui/Dialog'
import { SegmentedControl } from '../ui/SegmentedControl'
import { Skeleton } from '../ui/Skeleton'
import { ActivityRow } from './ActivityRow'
import { MiniCalendar, monthBounds } from './MiniCalendar'

interface SessionPickerProps {
  open: boolean
  onClose: () => void
  /** activity : choisir une séance ; day : choisir une date (mode Santé & Récup). */
  purpose: 'activity' | 'day'
  selectedId: number | null
  selectedDate: string | null
  today: string
  onSelectActivity: (a: ActivitySummary) => void
  onSelectDate: (iso: string) => void
}

export function SessionPicker({ open, onClose, purpose, selectedId, selectedDate, today, onSelectActivity, onSelectDate }: SessionPickerProps) {
  const isDesktop = useMediaQuery('(min-width: 768px)')
  const [tab, setTab] = useState<'list' | 'calendar'>(purpose === 'day' ? 'calendar' : 'list')
  const initial = parseLocal(selectedDate ?? today) ?? parseLocal(today)
  const [ym, setYm] = useState<{ y: number; m: number }>({ y: initial?.y ?? 2026, m: (initial?.mo ?? 1) - 1 })
  const [calDay, setCalDay] = useState<string | null>(null)
  const [prevOpen, setPrevOpen] = useState(open)
  if (open !== prevOpen) {
    // À chaque ouverture : revenir au mois de la sélection courante.
    setPrevOpen(open)
    if (open && initial) {
      setYm({ y: initial.y, m: initial.mo - 1 })
      setCalDay(null)
    }
  }

  const recent = useActivities()
  const bounds = monthBounds(ym.y, ym.m)
  const month = useActivitiesInRange(bounds.from, bounds.to, open)

  const byDate = useMemo(() => {
    const map = new Map<string, ActivitySummary[]>()
    for (const a of month.data?.items ?? []) {
      const arr = map.get(a.date) ?? []
      arr.push(a)
      map.set(a.date, arr)
    }
    return map
  }, [month.data])

  const recentItems = recent.data?.pages.flatMap((p) => p.items) ?? []
  const dayItems = calDay ? (byDate.get(calDay) ?? []) : []

  const pick = (a: ActivitySummary) => {
    onSelectActivity(a)
    onClose()
  }

  const calendar = (
    <div>
      <MiniCalendar
        year={ym.y}
        month0={ym.m}
        onMonthChange={(y, m) => setYm({ y, m })}
        byDate={byDate}
        selected={purpose === 'day' ? selectedDate : calDay}
        today={today}
        loading={month.isFetching}
        onSelect={(iso) => {
          if (purpose === 'day') {
            onSelectDate(iso)
            onClose()
          } else {
            setCalDay(iso)
          }
        }}
      />
      {purpose === 'activity' && calDay && (
        <div className="mt-4">
          <p className="label-caps mb-1.5">{capitalize(fmtDateLong(calDay))}</p>
          {dayItems.length === 0 ? (
            <p className="rounded-2xl border border-dashed border-line px-3 py-4 text-center text-sm text-ink-3">Aucune activité ce jour-là.</p>
          ) : (
            <ul className="space-y-1">
              {dayItems.map((a) => (
                <ActivityRow key={a.id} activity={a} selected={a.id === selectedId} onSelect={() => pick(a)} />
              ))}
            </ul>
          )}
        </div>
      )}
      {purpose === 'day' && <p className="mt-3 text-xs text-ink-3">Choisis un jour pour afficher son bilan santé & récupération.</p>}
    </div>
  )

  const list = (
    <div>
      {recent.isPending ? (
        <div className="space-y-2">
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className="h-14" />
          ))}
        </div>
      ) : recent.isError ? (
        <p className="text-sm text-danger">Impossible de charger les activités.</p>
      ) : recentItems.length === 0 ? (
        <p className="py-6 text-center text-sm text-ink-3">Aucune activité trouvée.</p>
      ) : (
        <ul className="space-y-1">
          {recentItems.map((a) => (
            <ActivityRow key={a.id} activity={a} selected={a.id === selectedId} onSelect={() => pick(a)} />
          ))}
        </ul>
      )}
      {recent.hasNextPage && (
        <button type="button" className="btn btn-ghost mt-3 w-full" onClick={() => void recent.fetchNextPage()} disabled={recent.isFetchingNextPage}>
          {recent.isFetchingNextPage ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : null}
          {recent.isFetchingNextPage ? 'Chargement…' : 'Charger plus'}
        </button>
      )}
    </div>
  )

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={purpose === 'day' ? 'Choisir un jour' : 'Choisir une séance'}
      description={purpose === 'day' ? 'Bilan santé & récupération' : 'Activités récentes ou calendrier'}
      size={purpose === 'day' ? 'md' : 'xl'}
    >
      {purpose === 'day' ? (
        calendar
      ) : isDesktop ? (
        <div className="grid grid-cols-[minmax(0,300px)_minmax(0,1fr)] gap-6">
          <div>
            <p className="label-caps mb-2 flex items-center gap-1.5">
              <CalendarDays className="size-3.5" aria-hidden="true" /> Calendrier
            </p>
            {calendar}
          </div>
          <div className="min-w-0">
            <p className="label-caps mb-2 flex items-center gap-1.5">
              <List className="size-3.5" aria-hidden="true" /> Activités récentes
            </p>
            {list}
          </div>
        </div>
      ) : (
        <div>
          <SegmentedControl
            ariaLabel="Affichage"
            size="sm"
            className="mb-4"
            value={tab}
            onChange={setTab}
            options={[
              { value: 'list', label: 'Récentes', icon: <List /> },
              { value: 'calendar', label: 'Calendrier', icon: <CalendarDays /> },
            ]}
          />
          {tab === 'list' ? list : calendar}
        </div>
      )}
    </Dialog>
  )
}
