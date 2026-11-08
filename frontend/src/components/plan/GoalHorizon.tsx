import { Flag, Sparkles } from 'lucide-react'
import type { Goal, PlanPhase } from '../../api/types'
import { cn } from '../../lib/cn'
import { capitalize, fmtDateLong, fmtDateShort, fmtNumber } from '../../lib/format'
import { countdown, monthTicks, phaseLayout, phaseTone } from './longterm'
import { t, tx } from '../../i18n'

const TONE_BG = { accent: 'bg-accent', ok: 'bg-ok', warn: 'bg-warn', violet: 'bg-violet', info: 'bg-info' } as const

interface GoalHorizonProps {
  goals: Goal[]
  goal: Goal
  onGoal: (id: number) => void
  phases: PlanPhase[]
  today: string
  onJump: (date: string) => void
  onGenerate: () => void
}

/** Countdown to the goal and a compact timeline of training phases until its date. */
export function GoalHorizon({ goals, goal, onGoal, phases, today, onJump, onGenerate }: GoalHorizonProps) {
  const target = goal.targetDate as string
  const cd = countdown(target, today)
  const first = phases.length ? [...phases].sort((a, b) => (a.startDate < b.startDate ? -1 : 1))[0].startDate : today
  const from = first < today ? first : today
  const { segments, todayPct } = phaseLayout(phases, from, target, today)
  const ticks = monthTicks(from, target)
  const current = segments.find((s) => s.current)?.phase ?? null
  const weeks = cd ? Math.round(cd.days / 7) : 0

  return (
    <section className="glass animate-in mb-4 p-4 sm:p-5" aria-label={t('plan.goals.horizon')}>
      <div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
        <div className="min-w-0">
          <p className="flex items-baseline gap-2">
            <span className="num text-3xl leading-none font-semibold tracking-tight text-ink">{cd?.label ?? '—'}</span>
            <span className="min-w-0 truncate text-sm text-ink-2">
              {tx('plan.goals.before', { goal: <span className="font-semibold text-ink">{goal.title}</span> })}
            </span>
          </p>
          <p className="mt-1 text-xs text-ink-3">
            {capitalize(fmtDateLong(target))}
            {cd && cd.days >= 14 ? t('plan.goals.weeks', { n: weeks }) : ''}
            {current ? t('plan.goals.phase', { name: current.name }) : ''}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {goals.length > 1 && (
            <select className="field h-9 min-h-9 w-auto max-w-[14rem] py-0 text-xs" value={goal.id} onChange={(e) => onGoal(Number(e.target.value))} aria-label={t('plan.goals.shown')}>
              {goals.map((g) => (
                <option key={g.id} value={g.id}>
                  {g.title} · {fmtDateShort(g.targetDate as string)}
                </option>
              ))}
            </select>
          )}
          <button type="button" className="btn btn-ghost h-9 min-h-9 px-3 text-xs" onClick={() => onJump(target)}>
            <Flag className="size-3.5" aria-hidden="true" /> {t('plan.goals.jump')}
          </button>
        </div>
      </div>

      <div className="mt-8">
        <div className="relative h-2.5 rounded-full bg-track" role="img" aria-label={phases.length ? t('plan.goals.phasesAria', { list: segments.map((s) => s.phase.name).join(', ') }) : t('plan.goals.noPhase')}>
          {segments.map((s, i) => (
            <span
              key={s.phase.id || i}
              className={cn('absolute inset-y-0 rounded-full', TONE_BG[phaseTone(s.phase.name, i)], s.past ? 'opacity-30' : s.current ? 'opacity-95' : 'opacity-60')}
              style={{ left: `${s.leftPct}%`, width: `calc(${s.widthPct}% - 2px)` }}
            />
          ))}
          {todayPct !== null && (
            <span className="absolute -top-1.5 -bottom-1.5 w-0.5 rounded-full bg-ink" style={{ left: `calc(${todayPct}% - 1px)` }} aria-hidden="true">
              <span className="absolute -top-4 left-1/2 -translate-x-1/2 text-[10px] font-semibold whitespace-nowrap text-ink-2">{t('plan.goals.todayShort')}</span>
            </span>
          )}
          <Flag className="absolute -top-4 -right-1 size-3.5 text-ink-2" aria-hidden="true" />
        </div>
        <div className="relative mt-1.5 h-4 text-[10px] text-ink-3" aria-hidden="true">
          {ticks
            .filter((t) => t.pct > 4 && t.pct < 94)
            .map((t) => (
              <span key={t.label} className="absolute -translate-x-1/2 whitespace-nowrap" style={{ left: `${t.pct}%` }}>
                {t.label}
              </span>
            ))}
        </div>
      </div>

      {phases.length > 0 ? (
        <ul className="mt-2 flex flex-wrap gap-x-1 gap-y-1" aria-label={t('plan.goals.phases')}>
          {segments.map((s, i) => (
            <li key={s.phase.id || i}>
              <button
                type="button"
                onClick={() => onJump(s.phase.startDate)}
                className={cn('flex items-center gap-1.5 rounded-xl px-2 py-1 text-left text-xs transition-colors hover:bg-track', s.current && 'bg-track/70')}
                title={[s.phase.focus, s.phase.note].filter(Boolean).join(' · ') || undefined}
              >
                <span className={cn('size-2 flex-none rounded-full', TONE_BG[phaseTone(s.phase.name, i)], s.past && 'opacity-40')} aria-hidden="true" />
                <span className={cn('font-semibold', s.past ? 'text-ink-3' : 'text-ink')}>{s.phase.name}</span>
                <span className="num text-ink-3">
                  {fmtDateShort(s.phase.startDate)} → {fmtDateShort(s.phase.endDate)}
                  {s.phase.weeklyVolumeKm ? t('plan.goals.perWeek', { km: fmtNumber(s.phase.weeklyVolumeKm) }) : ''}
                </span>
                {s.current && <span className="text-[10px] font-semibold text-accent uppercase">{t('plan.goals.current')}</span>}
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
          <p className="text-xs text-ink-3">{t('plan.goals.noPeriodization')}</p>
          <button type="button" className="btn btn-ghost h-9 min-h-9 px-3 text-xs" onClick={onGenerate}>
            <Sparkles className="size-3.5 text-accent" aria-hidden="true" /> {t('plan.goals.generate')}
          </button>
        </div>
      )}
    </section>
  )
}
