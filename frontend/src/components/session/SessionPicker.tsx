import { CalendarDays, List, Loader2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useActivities, useActivitiesInRange } from '../../api/queries'
import type { ActivitySummary } from '../../api/types'
import { useMediaQuery } from '../../hooks/useMediaQuery'
import { capitalize, fmtDateLong, parseLocal } from '../../lib/format'
import { Dialog } from '../ui/Dialog'
import { ChipToggle } from '../ui/Field'
import { SegmentedControl } from '../ui/SegmentedControl'
import { Skeleton } from '../ui/Skeleton'
import { ActivityRow } from './ActivityRow'
import { MiniCalendar, monthBounds } from './MiniCalendar'
import { clampMonth, MONTHS_FR, yearsBack } from './monthJump'

interface SessionPickerProps {
  open: boolean
  onClose: () => void
  selectedId: number | null
  selectedDate: string | null
  today: string
  onSelect: (a: ActivitySummary) => void
}

/** Session picker: recent list (load more) + month calendar. */
export function SessionPicker({ open, onClose, selectedId, selectedDate, today, onSelect }: SessionPickerProps) {
  const isDesktop = useMediaQuery('(min-width: 768px)')
  const [tab, setTab] = useState<'list' | 'calendar'>('list')
  const [runsOnly, setRunsOnly] = useState(false)
  const initial = parseLocal(selectedDate ?? today) ?? parseLocal(today)
  const [ym, setYm] = useState<{ y: number; m: number }>({ y: initial?.y ?? 2026, m: (initial?.mo ?? 1) - 1 })
  const [calDay, setCalDay] = useState<string | null>(null)
  const [prevOpen, setPrevOpen] = useState(open)
  if (open !== prevOpen) {
    setPrevOpen(open)
    if (open && initial) {
      setYm({ y: initial.y, m: initial.mo - 1 })
      setCalDay(null)
    }
  }

  const recent = useActivities(runsOnly ? { sport: 'run' } : {}, open)
  const bounds = monthBounds(ym.y, ym.m)
  const month = useActivitiesInRange(bounds.from, bounds.to, open)
  const monthReady = month.isSuccess && !month.isPlaceholderData
  const byDate = useMemo(() => {
    const map = new Map<string, ActivitySummary[]>()
    for (const a of month.data?.items ?? []) map.set(a.date, [...(map.get(a.date) ?? []), a])
    return map
  }, [month.data])

  const items = (recent.data?.pages.flatMap((p) => p.items) ?? []).filter((a) => !runsOnly || a.isRunning)
  // A day picked in another month stays remembered but only shows while its month is displayed.
  const shownDay = calDay && calDay >= bounds.from && calDay <= bounds.to ? calDay : null
  const dayItems = shownDay ? (byDate.get(shownDay) ?? []) : []
  const dayRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!calDay) return
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    dayRef.current?.scrollIntoView({ block: 'nearest', behavior: reduce ? 'auto' : 'smooth' })
  }, [calDay])
  const goToday = () => {
    const t = parseLocal(today)
    if (t) setYm({ y: t.y, m: t.mo - 1 })
    setCalDay(today)
  }
  const pick = (a: ActivitySummary) => {
    onSelect(a)
    onClose()
  }

  const calendar = (
    <div>
      <div className="mb-3 flex gap-2">
        <select className="field h-9 min-h-9 flex-1 py-1 text-sm" aria-label="Mois" value={ym.m} onChange={(e) => setYm(clampMonth(ym.y, Number(e.target.value), today))}>
          {MONTHS_FR.map((label, i) => (
            <option key={label} value={i}>
              {label}
            </option>
          ))}
        </select>
        <select className="field num h-9 min-h-9 w-28 py-1 text-sm" aria-label="Année" value={ym.y} onChange={(e) => setYm(clampMonth(Number(e.target.value), ym.m, today))}>
          {yearsBack(today).map((y) => (
            <option key={y} value={y}>
              {y}
            </option>
          ))}
        </select>
      </div>
      <MiniCalendar
        year={ym.y}
        month0={ym.m}
        onMonthChange={(y, m) => setYm({ y, m })}
        byDate={byDate}
        selected={shownDay ?? selectedDate}
        today={today}
        loading={month.isFetching}
        onSelect={setCalDay}
        onToday={goToday}
      />
      {month.isError && !month.isFetching && (
        <div role="alert" className="mt-3 flex items-center justify-between gap-3 rounded-xl border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">
          <span className="min-w-0">Impossible de charger les activités de ce mois.</span>
          <button type="button" className="btn btn-ghost h-8 min-h-8 flex-none px-2.5 text-xs" onClick={() => void month.refetch()}>
            Réessayer
          </button>
        </div>
      )}
      {shownDay && (
        <div ref={dayRef} className="mt-4 scroll-mb-4">
          <p className="label-caps mb-1.5">{capitalize(fmtDateLong(shownDay))}</p>
          {!monthReady && !month.isError ? (
            <Skeleton className="h-14" />
          ) : dayItems.length === 0 ? (
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
    </div>
  )

  const list = (
    <div>
      <div className="mb-2 flex gap-1.5">
        <ChipToggle pressed={!runsOnly} onChange={() => setRunsOnly(false)}>
          Toutes
        </ChipToggle>
        <ChipToggle pressed={runsOnly} onChange={() => setRunsOnly(true)}>
          Course
        </ChipToggle>
      </div>
      {recent.isPending ? (
        <div className="space-y-2">
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className="h-14" />
          ))}
        </div>
      ) : recent.isError ? (
        <p className="text-sm text-danger">Impossible de charger les activités.</p>
      ) : items.length === 0 ? (
        <p className="py-6 text-center text-sm text-ink-3">Aucune activité trouvée.</p>
      ) : (
        <ul className="space-y-1">
          {items.map((a) => (
            <ActivityRow key={a.id} activity={a} selected={a.id === selectedId} onSelect={() => pick(a)} />
          ))}
        </ul>
      )}
      {recent.hasNextPage && (
        <button type="button" className="btn btn-ghost mt-3 w-full" onClick={() => void recent.fetchNextPage()} disabled={recent.isFetchingNextPage}>
          {recent.isFetchingNextPage && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
          {recent.isFetchingNextPage ? 'Chargement…' : 'Charger plus'}
        </button>
      )}
    </div>
  )

  return (
    <Dialog open={open} onClose={onClose} title="Choisir une séance" description="Récentes, ou n’importe quel mois de ton historique" size="xl">
      {isDesktop ? (
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
