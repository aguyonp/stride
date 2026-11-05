import { CalendarPlus, CalendarRange, ChevronDown, ChevronLeft, ChevronRight, Plus, Target } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useGoalMutations, useGoals, usePlanMutations, useProfile, useWeekPlan } from '../../api/queries'
import type { ActivitySummary, Goal, PlannedSession, WeekPlan } from '../../api/types'
import { DoneActivityRow } from '../../components/plan/DoneActivityRow'
import { GoalDialog } from '../../components/plan/GoalDialog'
import { GoalRow } from '../../components/plan/GoalRow'
import { PlannedSessionCard } from '../../components/plan/PlannedSessionCard'
import { PlannedSessionDialog } from '../../components/plan/PlannedSessionDialog'
import { ratioPct, weekLabel } from '../../components/plan/planFormat'
import { Card } from '../../components/ui/Card'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { EmptyState, InlineError, PageHeader } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { cn } from '../../lib/cn'
import { addDays, capitalize, fmtDateLong, fmtHM, fmtKm, fmtWeekdayShort, mondayOf, parseLocal, todayISO } from '../../lib/format'
import { PLAN_SPORT_LABEL } from '../../lib/labels'

type DayEntry = WeekPlan['days'][number]

function emptyWeek(weekStart: string): DayEntry[] {
  return Array.from({ length: 7 }, (_, i) => ({ date: addDays(weekStart, i), planned: [], done: [] }))
}

function Progress({ label, done, planned, format }: { label: string; done: number; planned: number; format: (v: number) => string }) {
  const pct = ratioPct(done, planned)
  const width = pct === null ? (done > 0 ? 100 : 0) : Math.min(100, pct)
  const over = pct !== null && pct > 110
  return (
    <div className="min-w-0">
      <div className="flex items-baseline justify-between gap-2">
        <p className="text-xs font-medium text-ink-3">{label}</p>
        <p className="num text-xs text-ink-3">{pct === null ? (planned === 0 && done === 0 ? '—' : 'hors plan') : `${pct} %`}</p>
      </div>
      <p className="num mt-0.5 truncate text-lg font-semibold tracking-tight text-ink">
        {format(done)}
        <span className="text-sm font-medium text-ink-3"> / {planned > 0 ? format(planned) : '—'}</span>
      </p>
      <div
        className="mt-1.5 h-2 overflow-hidden rounded-full bg-track"
        role="progressbar"
        aria-label={`${label} : ${format(done)} réalisés sur ${planned > 0 ? format(planned) : 'rien de prévu'}`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(width)}
      >
        <div className={cn('h-full rounded-full transition-[width] duration-700', over ? 'bg-warn' : 'bg-gradient-to-r from-accent to-accent-2')} style={{ width: `${width}%` }} />
      </div>
    </div>
  )
}

function WeekTotals({ week }: { week: WeekPlan | undefined }) {
  const t = week?.totals
  if (!t) {
    return (
      <div className="grid gap-4 sm:grid-cols-3">
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} className="h-16" />
        ))}
      </div>
    )
  }
  return (
    <div className="grid gap-4 sm:grid-cols-3">
      <Progress label="Durée" done={t.doneDurationS} planned={t.plannedDurationS} format={(v) => fmtHM(v)} />
      <Progress label="Distance" done={t.doneDistanceM} planned={t.plannedDistanceM} format={(v) => fmtKm(v, 1)} />
      <Progress label="Séances" done={t.sessionsDone} planned={t.sessionsPlanned} format={(v) => String(v)} />
    </div>
  )
}

interface DayProps {
  day: DayEntry
  today: string
  layout: 'column' | 'row'
  onAdd: (date: string) => void
  onEdit: (s: PlannedSession) => void
  onDelete: (s: PlannedSession) => void
}

function DayBlock({ day, today, layout, onAdd, onEdit, onDelete }: DayProps) {
  const isToday = day.date === today
  const past = day.date < today
  const matchedIds = new Set(day.planned.map((p) => p.match?.activityId).filter((id): id is number => id !== null && id !== undefined))
  const extras = day.done.filter((a) => !matchedIds.has(a.id))
  const byId = new Map<number, ActivitySummary>(day.done.map((a) => [a.id, a]))
  const p = parseLocal(day.date)
  const longLabel = capitalize(fmtDateLong(day.date))
  const empty = day.planned.length === 0 && extras.length === 0
  const compact = layout === 'column'

  return (
    <section
      aria-label={`${longLabel}${isToday ? ', aujourd’hui' : ''}`}
      className={cn('glass min-w-0 p-3', isToday && 'ring-2 ring-accent/60', past && !isToday && 'opacity-90', layout === 'row' && 'sm:p-4')}
    >
      <header className="mb-2.5 flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-baseline gap-1.5">
          <span className={cn('text-xs font-semibold uppercase', isToday ? 'text-accent' : 'text-ink-3')}>{fmtWeekdayShort(day.date).replace('.', '')}</span>
          <span className={cn('num text-lg leading-none font-semibold', isToday ? 'text-ink' : 'text-ink-2')}>{p?.d}</span>
          {isToday && <span className={cn('chip h-5 bg-accent/15 px-2 text-[10px] text-accent', layout === 'column' && 'max-2xl:sr-only')}>Aujourd’hui</span>}
        </div>
        <button type="button" className="icon-btn size-8 flex-none" onClick={() => onAdd(day.date)} aria-label={`Planifier une séance le ${fmtDateLong(day.date)}`}>
          <Plus className="size-4" />
        </button>
      </header>
      {empty ? (
        <p className="rounded-xl border border-dashed border-line px-2 py-3 text-center text-xs text-ink-3">Rien de prévu</p>
      ) : (
        <div className={cn('grid gap-2', layout === 'row' && 'sm:grid-cols-2 lg:grid-cols-3')}>
          {day.planned.map((s) => (
            <PlannedSessionCard
              key={s.id}
              session={s}
              compact={compact}
              doneActivity={s.match?.activityId ? (byId.get(s.match.activityId) ?? null) : null}
              onEdit={() => onEdit(s)}
              onDelete={() => onDelete(s)}
            />
          ))}
          {extras.map((a) => (
            <DoneActivityRow key={a.id} activity={a} extra={a.planStatus !== 'done' && a.planStatus !== 'partial'} />
          ))}
        </div>
      )}
    </section>
  )
}

function GoalGroup({ title, goals, today, open: defaultOpen, children }: { title: string; goals: Goal[]; today: string; open?: boolean; children: (g: Goal) => ReactNode }) {
  if (goals.length === 0) return null
  return (
    <details className="group mt-4 border-t border-line pt-3" open={defaultOpen}>
      <summary className="flex min-h-10 list-none items-center justify-between gap-2 rounded-xl px-1 text-sm font-semibold text-ink-2 [&::-webkit-details-marker]:hidden">
        <span>
          {title} <span className="num text-ink-3">({goals.length})</span>
        </span>
        <ChevronDown className="size-4 text-ink-3 transition-transform group-open:rotate-180" aria-hidden="true" />
      </summary>
      <ul className="mt-2 space-y-2" data-today={today}>
        {goals.map(children)}
      </ul>
    </details>
  )
}

export default function PlanView() {
  useDocumentTitle('Planning')
  const profile = useProfile()
  const today = todayISO(profile.data?.timezone)
  const [anchor, setAnchor] = useState<string | null>(null)
  const weekStart = mondayOf(anchor ?? today)
  const currentWeek = mondayOf(today)
  const week = useWeekPlan(weekStart)
  const goals = useGoals()
  const planMut = usePlanMutations()
  const goalMut = useGoalMutations()
  const { toast } = useToast()

  const [sessionDialog, setSessionDialog] = useState<{ open: boolean; session: PlannedSession | null; date: string }>({ open: false, session: null, date: today })
  const [goalDialog, setGoalDialog] = useState<{ open: boolean; goal: Goal | null }>({ open: false, goal: null })
  const [deleteSession, setDeleteSession] = useState<PlannedSession | null>(null)
  const [deleteGoal, setDeleteGoal] = useState<Goal | null>(null)

  const data = week.data && week.data.weekStart === weekStart ? week.data : undefined
  const days = data?.days ?? emptyWeek(weekStart)
  const stale = week.isFetching && !data

  const grouped = useMemo(() => {
    const list = goals.data ?? []
    const byDate = (a: Goal, b: Goal) => (a.targetDate ?? '9999').localeCompare(b.targetDate ?? '9999')
    return {
      active: list.filter((g) => g.status === 'active').sort(byDate),
      done: list.filter((g) => g.status === 'done').sort(byDate),
      archived: list.filter((g) => g.status === 'archived').sort(byDate),
    }
  }, [goals.data])

  const addSession = (date: string) => setSessionDialog({ open: true, session: null, date })
  const editSession = (s: PlannedSession) => setSessionDialog({ open: true, session: s, date: s.date })
  const defaultAddDate = weekStart === currentWeek ? today : weekStart

  const goalRow = (g: Goal) => (
    <GoalRow
      key={g.id}
      goal={g}
      today={today}
      onEdit={() => setGoalDialog({ open: true, goal: g })}
      onStatus={(status) =>
        goalMut.update.mutate(
          { id: g.id, status },
          { onSuccess: () => toast({ title: status === 'done' ? 'Objectif atteint 🎉' : status === 'archived' ? 'Objectif archivé' : 'Objectif réactivé', description: g.title }) },
        )
      }
      onDelete={() => setDeleteGoal(g)}
    />
  )

  return (
    <div className="animate-in">
      <PageHeader
        title="Planning & objectifs"
        subtitle="Planifie ta semaine, compare le prévu au réalisé et garde tes objectifs en vue."
        actions={
          <>
            <button type="button" className="btn btn-ghost" onClick={() => setGoalDialog({ open: true, goal: null })}>
              <Target className="size-4" aria-hidden="true" /> Nouvel objectif
            </button>
            <button type="button" className="btn btn-primary" onClick={() => addSession(defaultAddDate)}>
              <CalendarPlus className="size-4" aria-hidden="true" /> Planifier une séance
            </button>
          </>
        }
      />

      <Card
        as="section"
        aria-label="Semaine affichée"
        title={<span className="num">{weekLabel(weekStart)}</span>}
        subtitle={weekStart === currentWeek ? 'Semaine en cours' : weekStart < currentWeek ? 'Semaine passée' : 'Semaine à venir'}
        icon={<CalendarRange className="size-4" />}
        action={
          <div className="flex items-center gap-1.5">
            <button type="button" className="icon-btn size-9" onClick={() => setAnchor(addDays(weekStart, -7))} aria-label="Semaine précédente">
              <ChevronLeft className="size-4" />
            </button>
            <button type="button" className="btn btn-ghost h-9 min-h-9 px-3 text-xs" onClick={() => setAnchor(null)} disabled={weekStart === currentWeek}>
              Cette semaine
            </button>
            <button type="button" className="icon-btn size-9" onClick={() => setAnchor(addDays(weekStart, 7))} aria-label="Semaine suivante">
              <ChevronRight className="size-4" />
            </button>
          </div>
        }
      >
        {week.isError && !data ? (
          <InlineError>{week.error.message}</InlineError>
        ) : (
          <div aria-live="polite" aria-busy={week.isFetching}>
            <WeekTotals week={data} />
          </div>
        )}
      </Card>

      <div className={cn('mt-4 transition-opacity', stale && 'is-refetching')} aria-busy={week.isFetching}>
        <div className="hidden gap-3 xl:grid xl:grid-cols-7">
          {days.map((d) => (
            <DayBlock key={d.date} day={d} today={today} layout="column" onAdd={addSession} onEdit={editSession} onDelete={setDeleteSession} />
          ))}
        </div>
        <div className="space-y-3 xl:hidden">
          {days.map((d) => (
            <DayBlock key={d.date} day={d} today={today} layout="row" onAdd={addSession} onEdit={editSession} onDelete={setDeleteSession} />
          ))}
        </div>
        {data && data.days.every((d) => d.planned.length === 0) && (
          <EmptyState
            className="mt-4"
            icon={<CalendarPlus />}
            title="Aucune séance planifiée cette semaine"
            action={
              <button type="button" className="btn btn-primary" onClick={() => addSession(defaultAddDate)}>
                <Plus className="size-4" aria-hidden="true" /> Planifier une séance
              </button>
            }
          >
            Planifie tes séances (durée, distance, allure, cadence) : Stride les comparera automatiquement à ce que tu as réellement fait, et le prompt IA en tiendra compte.
          </EmptyState>
        )}
      </div>

      <Card
        className="mt-6"
        title="Objectifs"
        subtitle="Repris dans le contexte « Objectifs » de tes prompts"
        icon={<Target className="size-4" />}
        action={
          <button type="button" className="btn btn-ghost h-9 min-h-9 px-3 text-xs" onClick={() => setGoalDialog({ open: true, goal: null })}>
            <Plus className="size-4" aria-hidden="true" /> Ajouter
          </button>
        }
      >
        {goals.isPending ? (
          <div className="space-y-2">
            <Skeleton className="h-16" />
            <Skeleton className="h-16" />
          </div>
        ) : goals.isError ? (
          <InlineError>{goals.error.message}</InlineError>
        ) : (goals.data ?? []).length === 0 ? (
          <EmptyState
            icon={<Target />}
            title="Pas encore d’objectif"
            action={
              <button type="button" className="btn btn-primary" onClick={() => setGoalDialog({ open: true, goal: null })}>
                <Plus className="size-4" aria-hidden="true" /> Définir un objectif
              </button>
            }
          >
            Un objectif macro (« Courir 45 min sans gêne »), une course cible, une cadence ou un volume hebdo : l’IA s’en servira pour ajuster ses conseils.
          </EmptyState>
        ) : (
          <>
            {grouped.active.length > 0 ? (
              <ul className="space-y-2">{grouped.active.map(goalRow)}</ul>
            ) : (
              <p className="text-sm text-ink-3">Aucun objectif actif.</p>
            )}
            <GoalGroup title="Atteints" goals={grouped.done} today={today}>
              {goalRow}
            </GoalGroup>
            <GoalGroup title="Archivés" goals={grouped.archived} today={today}>
              {goalRow}
            </GoalGroup>
          </>
        )}
      </Card>

      <PlannedSessionDialog
        open={sessionDialog.open}
        session={sessionDialog.session}
        defaultDate={sessionDialog.date}
        onClose={() => setSessionDialog((s) => ({ ...s, open: false }))}
      />
      <GoalDialog open={goalDialog.open} goal={goalDialog.goal} onClose={() => setGoalDialog((s) => ({ ...s, open: false }))} />
      <ConfirmDialog
        open={deleteSession !== null}
        onClose={() => setDeleteSession(null)}
        title="Supprimer la séance ?"
        message={
          deleteSession && (
            <p>
              « {deleteSession.title} » ({PLAN_SPORT_LABEL[deleteSession.sport].toLowerCase()}, {fmtDateLong(deleteSession.date)}) sera retirée du planning. Les activités réalisées ne sont pas supprimées.
            </p>
          )
        }
        confirmLabel="Supprimer"
        danger
        busy={planMut.remove.isPending}
        onConfirm={() =>
          deleteSession &&
          planMut.remove.mutate(deleteSession.id, {
            onSuccess: () => {
              toast({ title: 'Séance supprimée' })
              setDeleteSession(null)
            },
            onError: (e) => toast({ tone: 'error', title: 'Suppression impossible', description: e.message }),
          })
        }
      />
      <ConfirmDialog
        open={deleteGoal !== null}
        onClose={() => setDeleteGoal(null)}
        title="Supprimer l’objectif ?"
        message={deleteGoal && <p>« {deleteGoal.title} » sera définitivement supprimé. Pour le garder dans l’historique, archive-le plutôt.</p>}
        confirmLabel="Supprimer"
        danger
        busy={goalMut.remove.isPending}
        onConfirm={() =>
          deleteGoal &&
          goalMut.remove.mutate(deleteGoal.id, {
            onSuccess: () => {
              toast({ title: 'Objectif supprimé' })
              setDeleteGoal(null)
            },
            onError: (e) => toast({ tone: 'error', title: 'Suppression impossible', description: e.message }),
          })
        }
      />
    </div>
  )
}
