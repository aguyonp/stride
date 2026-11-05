import { useCalendar } from '../../api/queriesV3'
import type { CalendarDay, MatchStatus, PlannedSession } from '../../api/types'
import { cn } from '../../lib/cn'
import { addDays, capitalize, fmtDateLong, mondayOf, parseLocal } from '../../lib/format'
import { InlineError } from '../ui/Misc'
import { PlanSportIcon } from '../ui/icons'

const WEEKDAYS = ['Lun', 'Mar', 'Mer', 'Jeu', 'Ven', 'Sam', 'Dim']

const STATUS_CLS: Record<MatchStatus, string> = {
  done: 'border-ok/35 bg-ok/10 text-ink',
  partial: 'border-warn/40 bg-warn/10 text-ink',
  missed: 'border-danger/35 bg-danger/[0.07] text-ink-3 line-through decoration-danger/50',
  upcoming: 'border-line bg-track/60 text-ink-2',
  extra: 'border-info/35 bg-info/10 text-ink',
}
const STATUS_DOT: Record<MatchStatus, string> = { done: 'bg-ok', partial: 'bg-warn', missed: 'bg-danger', upcoming: 'bg-ink-3', extra: 'bg-info' }

function monthRange(month: string): { from: string; to: string } {
  const from = mondayOf(`${month}-01`)
  return { from, to: addDays(from, 41) }
}

function dayLabel(d: CalendarDay): string {
  const parts = [capitalize(fmtDateLong(d.date))]
  if (d.planned.length) parts.push(`${d.planned.length} prévue(s) : ${d.planned.map((p) => p.title).join(', ')}`)
  if (d.done.length) parts.push(`${d.done.length} réalisée(s)`)
  return parts.join('. ')
}

/** Month grid (6 weeks): planned sessions coloured by match status, done activities as dots; a day opens its week. */
export function MonthCalendar({ month, today, onPickDay, onOpenSession }: { month: string; today: string; onPickDay: (date: string) => void; onOpenSession: (s: PlannedSession) => void }) {
  const { from, to } = monthRange(month)
  const q = useCalendar(from, to)
  const days = q.data?.days ?? Array.from({ length: 42 }, (_, i) => ({ date: addDays(from, i), planned: [], done: [] }))
  if (q.isError) return <InlineError>{q.error.message}</InlineError>
  return (
    <div className={cn('transition-opacity', q.isPlaceholderData && 'opacity-60')} aria-busy={q.isFetching}>
      <div className="grid grid-cols-7 gap-1 sm:gap-1.5" role="group" aria-label="Calendrier du mois">
        <div className="contents" aria-hidden="true">
          {WEEKDAYS.map((w) => (
            <div key={w} className="pb-1 text-center text-[11px] font-semibold tracking-wide text-ink-3 uppercase">
              {w}
            </div>
          ))}
        </div>
        {days.map((d) => {
          const p = parseLocal(d.date)
          const inMonth = d.date.startsWith(month)
          const isToday = d.date === today
          const matched = new Set(d.planned.map((x) => x.match?.activityId).filter(Boolean))
          const extras = d.done.filter((a) => !matched.has(a.id))
          return (
            <div
              key={d.date}
              className={cn('flex min-h-16 min-w-0 flex-col rounded-xl border p-1 sm:min-h-24 sm:p-1.5', inMonth ? 'border-line bg-white/[0.02]' : 'border-transparent opacity-45', isToday && 'border-accent/60 ring-1 ring-accent/40')}
            >
              <button type="button" onClick={() => onPickDay(d.date)} className="flex items-center justify-between rounded-md px-0.5 text-left hover:text-accent focus-visible:outline-2 focus-visible:outline-accent" aria-label={`${dayLabel(d)}. Ouvrir la semaine`}>
                <span className={cn('num text-xs font-semibold', isToday ? 'text-accent' : 'text-ink-2')}>{p?.d}</span>
                {extras.length > 0 && <span className="size-1.5 rounded-full bg-info sm:hidden" aria-hidden="true" />}
              </button>
              <ul className="mt-1 hidden min-w-0 space-y-1 sm:block">
                {d.planned.slice(0, 3).map((s) => {
                  const st = s.match?.status ?? 'upcoming'
                  return (
                    <li key={s.id}>
                      <button type="button" onClick={() => onOpenSession(s)} className={cn('flex w-full min-w-0 items-center gap-1 rounded-md border px-1 py-0.5 text-left text-[11px] leading-tight', STATUS_CLS[st])} title={s.title}>
                        <PlanSportIcon sport={s.sport} className="size-3 flex-none" />
                        <span className="truncate">{s.title}</span>
                      </button>
                    </li>
                  )
                })}
                {extras.slice(0, 2).map((a) => (
                  <li key={a.id} className={cn('flex min-w-0 items-center gap-1 rounded-md border px-1 py-0.5 text-[11px] leading-tight', STATUS_CLS.extra)} title={a.name}>
                    <span className="size-1.5 flex-none rounded-full bg-info" aria-hidden="true" />
                    <span className="truncate">{a.name}</span>
                  </li>
                ))}
                {d.planned.length > 3 && <li className="text-[10px] text-ink-3">+{d.planned.length - 3}</li>}
              </ul>
              <div className="mt-auto flex flex-wrap gap-0.5 sm:hidden" aria-hidden="true">
                {d.planned.map((s) => (
                  <span key={s.id} className={cn('size-1.5 rounded-full', STATUS_DOT[s.match?.status ?? 'upcoming'])} />
                ))}
              </div>
            </div>
          )
        })}
      </div>
      <ul className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink-3" aria-label="Légende">
        {(
          [
            ['done', 'Réalisée'],
            ['partial', 'Partielle'],
            ['missed', 'Manquée'],
            ['upcoming', 'À venir'],
            ['extra', 'Hors plan'],
          ] as [MatchStatus, string][]
        ).map(([k, l]) => (
          <li key={k} className="flex items-center gap-1.5">
            <span className={cn('size-2 rounded-full', STATUS_DOT[k])} aria-hidden="true" />
            {l}
          </li>
        ))}
      </ul>
    </div>
  )
}
