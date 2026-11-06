import { ArrowRight, CalendarCheck, CalendarRange, Target } from 'lucide-react'
import { Link } from 'wouter'
import { useGoals, useWeekPlan } from '../../api/queries'
import type { Goal, PlanMatch, PlannedSession } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtHM, fmtKm, fmtWeekdayShort, parseLocal } from '../../lib/format'
import { MATCH_STATUS } from '../../lib/labels'
import { GoalRow } from '../plan/GoalRow'
import { MatchBadge } from '../plan/MatchBadge'
import { matchDeltas, ratioPct, weekLabel } from '../plan/planFormat'
import { PlannedSessionCard } from '../plan/PlannedSessionCard'
import { useSessionDrawer } from '../plan/useSessionDrawer'
import { Card } from '../ui/Card'
import { PlanSportIcon } from '../ui/icons'
import { Skeleton } from '../ui/Skeleton'

const more = (href: string, label: string) => (
  <Link href={href} className="inline-flex h-8 items-center gap-1 rounded-lg px-2 text-xs font-semibold text-accent hover:bg-accent/10">
    {label} <ArrowRight className="size-3.5" aria-hidden="true" />
  </Link>
)

/** Planned vs done for the selected session + the next planned sessions. */
export function PlanMatchCard({ match, upcoming, className }: { match: PlanMatch | null; upcoming: PlannedSession[]; className?: string }) {
  const deltas = match ? matchDeltas(match) : []
  const drawer = useSessionDrawer()
  return (
    <Card title="Prévu vs réalisé" icon={<CalendarCheck className="size-4" />} action={more('/plan', 'Planning')} className={className}>
      {match ? (
        <div className="glass-inset p-3.5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <MatchBadge status={match.status} />
          </div>
          <p className="mt-1.5 text-sm text-ink">{match.summary}</p>
          {deltas.length > 0 && (
            <ul className="num mt-2 flex flex-wrap gap-1.5">
              {deltas.map((d) => (
                <li key={d} className="chip border border-line bg-track text-ink-2">
                  {d}
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : (
        <p className="text-sm text-ink-3">Aucune séance prévue ce jour-là : séance hors plan.</p>
      )}
      <p className="label-caps mt-4 mb-1.5">Prochaines séances prévues</p>
      {upcoming.length === 0 ? (
        <p className="text-sm text-ink-3">Rien de planifié. Ajoute tes séances dans le planning pour que le coach IA en tienne compte.</p>
      ) : (
        <ul className="space-y-2">
          {upcoming.map((s) => (
            <li key={s.id}>
              <PlannedSessionCard session={s} compact onOpen={() => drawer.open(s)} />
            </li>
          ))}
        </ul>
      )}
      {drawer.node}
    </Card>
  )
}

const dot: Record<string, string> = {
  done: 'bg-ok',
  partial: 'bg-warn',
  missed: 'bg-danger',
  upcoming: 'bg-info',
  extra: 'bg-ink-3',
}

export function WeekCard({ date, today, className }: { date: string; today: string; className?: string }) {
  const week = useWeekPlan(date)
  const w = week.data
  const t = w?.totals
  const pct = t ? ratioPct(t.doneDurationS, t.plannedDurationS) : null
  return (
    <Card title="Semaine" icon={<CalendarRange className="size-4" />} subtitle={w ? weekLabel(w.weekStart) : undefined} action={more('/plan', 'Ouvrir')} className={className}>
      {!w ? (
        <Skeleton className="h-28" />
      ) : (
        <>
          <ol className="grid grid-cols-7 gap-1">
            {w.days.map((d) => {
              const p = parseLocal(d.date)
              const isToday = d.date === today
              const statuses = d.planned.map((s) => s.match?.status ?? 'upcoming')
              const extra = d.done.filter((a) => a.planStatus === 'extra').length
              const label = `${fmtWeekdayShort(d.date)} ${p?.d} : ${d.planned.length ? d.planned.map((s) => `${s.title} (${MATCH_STATUS[s.match?.status ?? 'upcoming'].label})`).join(', ') : 'rien de prévu'}${d.done.length ? `, ${d.done.length} activité(s) réalisée(s)` : ''}`
              return (
                <li key={d.date} aria-label={label} className={cn('flex min-w-0 flex-col items-center gap-1 rounded-xl border px-0.5 py-1.5', isToday ? 'border-accent/50 bg-accent/10' : 'border-transparent')}>
                  <span className={cn('text-[10px] font-semibold uppercase', isToday ? 'text-accent' : 'text-ink-3')} aria-hidden="true">
                    {fmtWeekdayShort(d.date).slice(0, 2)}
                  </span>
                  <span className="num text-sm font-semibold" aria-hidden="true">
                    {p?.d}
                  </span>
                  <span className="flex h-5 items-center justify-center" aria-hidden="true">
                    {d.planned[0] ? <PlanSportIcon sport={d.planned[0].sport} className="size-4 text-ink-2" /> : <span className="text-ink-3">·</span>}
                  </span>
                  <span className="flex h-1.5 gap-0.5" aria-hidden="true">
                    {statuses.slice(0, 2).map((s, i) => (
                      <span key={i} className={cn('size-1.5 rounded-full', dot[s])} />
                    ))}
                    {extra > 0 && <span className="size-1.5 rounded-full bg-ink-3" />}
                  </span>
                </li>
              )
            })}
          </ol>
          {t && (
            <div className="mt-3 space-y-1.5 text-xs">
              <div className="flex justify-between gap-2 text-ink-2">
                <span>
                  Séances <strong className="num text-ink">{t.sessionsDone}</strong> / {t.sessionsPlanned}
                </span>
                <span className="num">
                  {fmtHM(t.doneDurationS)} / {fmtHM(t.plannedDurationS)} · {fmtKm(t.doneDistanceM, 0)} / {fmtKm(t.plannedDistanceM, 0)}
                </span>
              </div>
              <div className="h-2 rounded-full bg-track" role="img" aria-label={`Volume réalisé ${pct ?? 0} % du prévu`}>
                <div className="h-full rounded-full bg-accent" style={{ width: `${Math.min(100, pct ?? 0)}%` }} />
              </div>
            </div>
          )}
        </>
      )}
    </Card>
  )
}

export function GoalsCard({ today, goals: given, className }: { today: string; goals?: Goal[]; className?: string }) {
  const q = useGoals()
  const goals = (given ?? q.data ?? []).filter((g) => g.status === 'active')
  return (
    <Card title="Objectifs" icon={<Target className="size-4" />} action={more('/plan#objectifs', 'Gérer')} className={className}>
      {!given && q.isPending ? (
        <Skeleton className="h-20" />
      ) : goals.length === 0 ? (
        <p className="text-sm text-ink-3">Aucun objectif actif. Définis un objectif macro, une course ou une cible de cadence dans le planning.</p>
      ) : (
        <ul className="space-y-2">
          {goals.map((g) => (
            <GoalRow key={g.id} goal={g} today={today} />
          ))}
        </ul>
      )}
    </Card>
  )
}
