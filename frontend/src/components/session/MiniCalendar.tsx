import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import type { ActivitySummary } from '../../api/types'
import { cn } from '../../lib/cn'
import { addDays, capitalize, fmtDateLong, fmtMonthYear, parseLocal, toISODate } from '../../lib/format'

interface MiniCalendarProps {
  year: number
  month0: number
  onMonthChange: (year: number, month0: number) => void
  byDate: Map<string, ActivitySummary[]>
  selected: string | null
  onSelect: (iso: string) => void
  today: string
  loading?: boolean
}

const WEEKDAYS = ['L', 'M', 'M', 'J', 'V', 'S', 'D']
const WEEKDAYS_LONG = ['lundi', 'mardi', 'mercredi', 'jeudi', 'vendredi', 'samedi', 'dimanche']

export function monthBounds(year: number, month0: number): { from: string; to: string } {
  return { from: toISODate(new Date(year, month0, 1)), to: toISODate(new Date(year, month0 + 1, 0)) }
}

/** Compact month grid with activity dots and arrow-key navigation. */
export function MiniCalendar({ year, month0, onMonthChange, byDate, selected, onSelect, today, loading }: MiniCalendarProps) {
  const gridRef = useRef<HTMLDivElement>(null)
  const days = useMemo(() => {
    const first = new Date(year, month0, 1)
    const offset = (first.getDay() + 6) % 7
    const count = new Date(year, month0 + 1, 0).getDate()
    const cells: (string | null)[] = Array.from({ length: offset }, () => null)
    for (let d = 1; d <= count; d++) cells.push(toISODate(new Date(year, month0, d)))
    while (cells.length % 7) cells.push(null)
    return cells
  }, [year, month0])

  const inMonth = (iso: string | null) => !!iso && days.includes(iso)
  const [focusDate, setFocusDate] = useState<string>(() => (selected && inMonth(selected) ? selected : (days.find((d) => d === today) ?? days.find(Boolean) ?? today)))
  const pendingFocus = useRef(false)

  useEffect(() => {
    if (!days.includes(focusDate)) setFocusDate(days.find((d) => d === selected) ?? days.find((d) => d === today) ?? (days.find(Boolean) as string))
  }, [days, focusDate, selected, today])

  useEffect(() => {
    if (pendingFocus.current) {
      pendingFocus.current = false
      gridRef.current?.querySelector<HTMLButtonElement>(`[data-date="${focusDate}"]`)?.focus()
    }
  }, [focusDate, days])

  const shiftMonth = (delta: number) => {
    const d = new Date(year, month0 + delta, 1)
    onMonthChange(d.getFullYear(), d.getMonth())
  }
  const nextDisabled = toISODate(new Date(year, month0 + 1, 1)) > today

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const delta = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7 }[e.key]
    if (delta === undefined) return
    e.preventDefault()
    const next = addDays(focusDate, delta)
    if (next > today) return
    const p = parseLocal(next)
    if (p && (p.y !== year || p.mo - 1 !== month0)) onMonthChange(p.y, p.mo - 1)
    pendingFocus.current = true
    setFocusDate(next)
  }

  return (
    <div className="select-none">
      <div className="mb-2 flex items-center justify-between">
        <button type="button" className="icon-btn size-8" onClick={() => shiftMonth(-1)} aria-label="Mois précédent">
          <ChevronLeft className="size-4" />
        </button>
        <p className="text-sm font-semibold capitalize" aria-live="polite">
          {fmtMonthYear(year, month0)}
          {loading && <span className="sr-only"> — chargement</span>}
        </p>
        <button type="button" className="icon-btn size-8" onClick={() => shiftMonth(1)} disabled={nextDisabled} aria-label="Mois suivant">
          <ChevronRight className="size-4" />
        </button>
      </div>
      <div className="grid grid-cols-7 gap-1 text-center" aria-hidden="true">
        {WEEKDAYS.map((w, i) => (
          <span key={i} className="py-1 text-[11px] font-semibold text-ink-3">
            {w}
          </span>
        ))}
      </div>
      <div ref={gridRef} role="grid" aria-label={capitalize(fmtMonthYear(year, month0))} onKeyDown={onKeyDown} className={cn('grid grid-cols-7 gap-1 transition-opacity', loading && 'opacity-60')}>
        {days.map((iso, i) => {
          if (!iso) return <span key={`e${i}`} role="presentation" />
          const acts = byDate.get(iso) ?? []
          const runs = acts.filter((a) => a.isRunning).length
          const isSel = iso === selected
          const isToday = iso === today
          const future = iso > today
          const p = parseLocal(iso)
          const label = `${capitalize(fmtDateLong(iso))}${acts.length ? `, ${acts.length} activité${acts.length > 1 ? 's' : ''}${runs ? ` dont ${runs} course${runs > 1 ? 's' : ''}` : ''}` : ', aucune activité'}`
          return (
            <button
              key={iso}
              type="button"
              role="gridcell"
              data-date={iso}
              aria-label={label}
              aria-selected={isSel}
              aria-current={isToday ? 'date' : undefined}
              tabIndex={iso === focusDate ? 0 : -1}
              disabled={future}
              onClick={() => {
                setFocusDate(iso)
                onSelect(iso)
              }}
              className={cn(
                'relative flex aspect-square min-h-9 flex-col items-center justify-center rounded-xl text-sm transition-colors',
                isSel ? 'bg-accent font-medium text-on-accent' : 'hover:bg-track',
                !isSel && isToday && 'font-bold text-accent ring-1 ring-accent/50 ring-inset',
                !isSel && !isToday && (acts.length ? 'text-ink' : 'text-ink-3'),
                future && 'opacity-35',
              )}
            >
              <span className="num leading-none">{p?.d}</span>
              <span className="mt-1 flex h-1.5 gap-0.5" aria-hidden="true">
                {acts.slice(0, 3).map((a) => (
                  <span key={a.id} className={cn('size-1.5 rounded-full', isSel ? 'bg-on-accent' : a.isRunning ? 'bg-accent' : 'bg-ink-3/70')} />
                ))}
              </span>
            </button>
          )
        })}
      </div>
      <p className="mt-2 flex items-center gap-3 text-[11px] text-ink-3">
        <span className="flex items-center gap-1">
          <span className="size-1.5 rounded-full bg-accent" aria-hidden="true" /> Course
        </span>
        <span className="flex items-center gap-1">
          <span className="size-1.5 rounded-full bg-ink-3/70" aria-hidden="true" /> Autre activité
        </span>
        <span className="sr-only">{WEEKDAYS_LONG.join(', ')}</span>
      </p>
    </div>
  )
}
