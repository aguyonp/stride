import { Flag } from 'lucide-react'
import { useCalendar } from '../../api/queriesV3'
import type { CalendarDay, CalendarEvent, Goal, MatchStatus, PlannedSession, PlanPhase } from '../../api/types'
import { cn } from '../../lib/cn'
import { EVENT_KIND, MATCH_STATUS } from '../../lib/labels'
import { EventChip } from './EventDialog'
import { phaseAt, phaseTone } from './longterm'
import { addDays, capitalize, fmtDateLong, mondayOf, parseLocal, weekdayNames } from '../../lib/format'
import { InlineError } from '../ui/Misc'
import { PlanSportIcon } from '../ui/icons'
import { t } from '../../i18n'

const weekdays = () => weekdayNames('short').map((w) => w.replace('.', ''))

const STATUS_CLS: Record<MatchStatus, string> = {
  done: 'border-ok/35 bg-ok/10 text-ink',
  partial: 'border-warn/40 bg-warn/10 text-ink',
  missed: 'border-danger/35 bg-danger/[0.07] text-ink-3 line-through decoration-danger/50',
  upcoming: 'border-line bg-track/60 text-ink-2',
  extra: 'border-info/35 bg-info/10 text-ink',
}
const STATUS_DOT: Record<MatchStatus, string> = { done: 'bg-ok', partial: 'bg-warn', missed: 'bg-danger', upcoming: 'bg-ink-3', extra: 'bg-info' }
const TONE_BG = { accent: 'bg-accent', ok: 'bg-ok', warn: 'bg-warn', violet: 'bg-violet', info: 'bg-info' } as const

function monthRange(month: string): { from: string; to: string } {
  const from = mondayOf(`${month}-01`)
  return { from, to: addDays(from, 41) }
}

function dayLabel(d: CalendarDay): string {
  const parts = [capitalize(fmtDateLong(d.date))]
  if (d.planned.length) parts.push(t('plan.card.planned', { count: d.planned.length, list: d.planned.map((p) => p.title).join(', ') }))
  if (d.done.length) parts.push(t('plan.card.doneCount', { count: d.done.length }))
  for (const e of d.events ?? []) parts.push(t('plan.card.event', { kind: EVENT_KIND[e.kind].label }))
  return parts.join('. ')
}

/** Month grid (6 weeks): planned sessions coloured by match status, done activities as dots; a day opens its week. */
interface MonthCalendarProps {
  month: string
  today: string
  onPickDay: (date: string) => void
  onOpenSession: (s: PlannedSession) => void
  onOpenEvent?: (e: CalendarEvent, date: string) => void
  goals?: Map<string, Goal>
  phases?: PlanPhase[]
}

export function MonthCalendar({ month, today, onPickDay, onOpenSession, onOpenEvent, goals, phases = [] }: MonthCalendarProps) {
  const { from, to } = monthRange(month)
  const q = useCalendar(from, to)
  const days: CalendarDay[] = q.data?.days ?? Array.from({ length: 42 }, (_, i) => ({ date: addDays(from, i), planned: [], done: [] }))
  if (q.isError) return <InlineError>{q.error.message}</InlineError>
  return (
    <div className={cn('transition-opacity', q.isPlaceholderData && 'opacity-60')} aria-busy={q.isFetching}>
      <div className="grid grid-cols-7 gap-1 sm:gap-1.5" role="group" aria-label={t('plan.card.monthGrid')}>
        <div className="contents" aria-hidden="true">
          {weekdays().map((w) => (
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
          const events = d.events ?? []
          const goal = goals?.get(d.date)
          const phase = phaseAt(phases, d.date)
          return (
            <div
              key={d.date}
              className={cn('relative flex min-h-16 min-w-0 flex-col overflow-hidden rounded-xl border p-1 sm:min-h-24 sm:p-1.5', inMonth ? 'border-line bg-white/[0.02]' : 'border-transparent opacity-45', isToday && 'border-accent/60 ring-1 ring-accent/40', goal && 'border-violet/50')}
            >
              <button type="button" onClick={() => onPickDay(d.date)} className="flex items-center justify-between gap-1 rounded-md px-0.5 text-left hover:text-accent focus-visible:outline-2 focus-visible:outline-accent" aria-label={`${dayLabel(d)}${goal ? t('plan.monthExtra.goal', { title: goal.title }) : ''}${t('plan.monthExtra.openWeek')}`}>
                <span className={cn('num text-xs font-semibold', isToday ? 'text-accent' : 'text-ink-2')}>{p?.d}</span>
                <span className="flex items-center gap-0.5">
                  {goal && <Flag className="size-3 text-violet" aria-hidden="true" />}
                  {events.length > 0 && <span className="size-1.5 rounded-full bg-warn sm:hidden" aria-hidden="true" />}
                  {extras.length > 0 && <span className="size-1.5 rounded-full bg-info sm:hidden" aria-hidden="true" />}
                </span>
              </button>
              {events.length > 0 && (
                <div className="mt-1 hidden flex-wrap gap-0.5 sm:flex">
                  {events.map((e) => (
                    <EventChip key={e.id} e={e} compact onOpen={onOpenEvent ? (ev) => onOpenEvent(ev, d.date) : undefined} />
                  ))}
                </div>
              )}
              {phase && <span className={cn('absolute inset-x-1 bottom-0.5 h-0.5 rounded-full opacity-60', TONE_BG[phaseTone(phase.name, phases.indexOf(phase))])} title={phase.name} aria-hidden="true" />}
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
      <ul className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink-3" aria-label={t('plan.card.legend')}>
        {(
          [
            ['done', MATCH_STATUS.done.label],
            ['partial', MATCH_STATUS.partial.label],
            ['missed', MATCH_STATUS.missed.label],
            ['upcoming', MATCH_STATUS.upcoming.label],
            ['extra', MATCH_STATUS.extra.label],
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
