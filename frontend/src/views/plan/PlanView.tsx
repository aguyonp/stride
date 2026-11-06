import { CalendarDays, CalendarPlus, CalendarRange, ChevronDown, ChevronLeft, ChevronRight, Flag, MoreHorizontal, Plus, Sparkles, Target, TriangleAlert, Wand2 } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useGoalMutations, useGoals, usePlanMutations, useProfile, useWeekPlan } from '../../api/queries'
import { useCalendarEvents, usePlanPhases } from '../../api/queriesV31'
import type { ActivitySummary, CalendarEvent, Goal, PlannedSession, WeekPlan } from '../../api/types'
import { DoneActivityRow } from '../../components/plan/DoneActivityRow'
import { EventChip, EventDialog } from '../../components/plan/EventDialog'
import { GenerateDialog } from '../../components/plan/GenerateDialog'
import { GoalHorizon } from '../../components/plan/GoalHorizon'
import { datedGoals, horizonGoal, phaseAt } from '../../components/plan/longterm'
import { MonthCalendar } from '../../components/plan/MonthCalendar'
import { SessionDrawer } from '../../components/plan/SessionDrawer'
import { MenuItem, Popover } from '../../components/ui/Popover'
import { SegmentedControl } from '../../components/ui/SegmentedControl'
import { useStoredState } from '../../hooks/useStoredState'
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
import { addDays, capitalize, fmtDateLong, fmtHM, fmtKm, fmtMonthYear, fmtNumber, fmtWeekdayShort, mondayOf, parseLocal, todayISO } from '../../lib/format'
import { PLAN_SPORT_LABEL } from '../../lib/labels'

type DayEntry = WeekPlan['days'][number]
type EventState = { date: string; event: CalendarEvent | null; adaptOnly?: boolean } | null

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
        <div className={cn('h-full rounded-full transition-[width] duration-700', over ? 'bg-warn' : 'bg-accent')} style={{ width: `${width}%` }} />
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
  events: CalendarEvent[]
  goal: Goal | null
  onAdd: (date: string) => void
  onEdit: (s: PlannedSession) => void
  onDelete: (s: PlannedSession) => void
  onOpen: (s: PlannedSession) => void
  onEvent: (e: EventState) => void
}

function DayBlock({ day, today, layout, events, goal, onAdd, onEdit, onDelete, onOpen, onEvent }: DayProps) {
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
      className={cn('min-w-0 rounded-2xl border border-line bg-white/[0.02] p-3', isToday && 'border-accent/50 ring-1 ring-accent/40', past && !isToday && 'opacity-90', layout === 'row' && 'sm:p-4')}
    >
      <header className="mb-2.5 flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-baseline gap-1.5">
          <span className={cn('text-xs font-semibold uppercase', isToday ? 'text-accent' : 'text-ink-3')}>{fmtWeekdayShort(day.date).replace('.', '')}</span>
          <span className={cn('num text-lg leading-none font-semibold', isToday ? 'text-ink' : 'text-ink-2')}>{p?.d}</span>
          {isToday && <span className={cn('chip h-5 bg-accent/15 px-2 text-[10px] text-accent', layout === 'column' && 'max-2xl:sr-only')}>Aujourd’hui</span>}
        </div>
        <span className="flex flex-none items-center">
          <button type="button" className="icon-btn size-8" onClick={() => onAdd(day.date)} aria-label={`Planifier une séance le ${fmtDateLong(day.date)}`}>
            <Plus className="size-4" />
          </button>
          <Popover
            trigger={(p) => (
              <button {...p} type="button" className="icon-btn size-8" aria-label={`Plus d’actions pour le ${fmtDateLong(day.date)}`}>
                <MoreHorizontal className="size-4" />
              </button>
            )}
            panelClassName="w-60"
          >
            {(close) => (
              <>
                <MenuItem icon={<TriangleAlert />} onSelect={() => (close(), onEvent({ date: day.date, event: null }))}>
                  Signaler un imprévu
                </MenuItem>
                {day.date >= today && (
                  <MenuItem icon={<Wand2 />} onSelect={() => (close(), onEvent({ date: day.date, event: null, adaptOnly: true }))}>
                    Adapter la suite du plan
                  </MenuItem>
                )}
              </>
            )}
          </Popover>
        </span>
      </header>
      {(events.length > 0 || goal) && (
        <div className="mb-2 flex flex-wrap gap-1">
          {goal && (
            <span className="chip h-6 bg-violet/12 px-2 text-[11px] text-violet" title={goal.title}>
              <Flag className="size-3" aria-hidden="true" /> <span className="max-w-[9rem] truncate">{goal.title}</span>
            </span>
          )}
          {events.map((e) => (
            <EventChip key={e.id} e={e} onOpen={(ev) => onEvent({ date: day.date, event: ev })} />
          ))}
        </div>
      )}
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
              onOpen={() => onOpen(s)}
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
  const [mode, setMode] = useStoredState<'week' | 'month'>('stride.plan.view', 'week', (v) => (v === 'month' || v === 'week' ? v : undefined))
  const [genOpen, setGenOpen] = useState<{ horizon?: 'goal' } | null>(null)
  const [drawer, setDrawer] = useState<{ s: PlannedSession; edit: boolean } | null>(null)
  const [eventDlg, setEventDlg] = useState<EventState>(null)
  const [goalPick, setGoalPick] = useState<number | null>(null)
  const phases = usePlanPhases()
  const weekEvents = useCalendarEvents(weekStart, addDays(weekStart, 6), mode === 'week')
  const month = (anchor ?? today).slice(0, 7)
  const shiftMonth = (n: number) => {
    const p = parseLocal(`${month}-01`)
    if (!p) return
    const d = new Date(p.y, p.mo - 1 + n, 1)
    setAnchor(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`)
  }
  const monthTitle = (() => {
    const p = parseLocal(`${month}-01`)
    return p ? capitalize(fmtMonthYear(p.y, p.mo - 1)) : month
  })()

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

  const dated = useMemo(() => datedGoals(goals.data ?? [], today), [goals.data, today])
  const goal = dated.find((g) => g.id === goalPick) ?? horizonGoal(dated, phases.data ?? [])
  const goalsByDate = useMemo(() => new Map(dated.map((g) => [g.targetDate as string, g])), [dated])
  const weekPhase = phaseAt(phases.data ?? [], weekStart) ?? phaseAt(phases.data ?? [], addDays(weekStart, 6))
  const eventsOf = (date: string) => (weekEvents.data ?? []).filter((e) => e.startDate <= date && e.endDate >= date)

  const addSession = (date: string) => setSessionDialog({ open: true, session: null, date })
  const editSession = (s: PlannedSession) => setDrawer({ s, edit: true })
  const openSession = (s: PlannedSession) => setDrawer({ s, edit: false })
  const defaultAddDate = weekStart === currentWeek ? today : weekStart
  const jump = (date: string) => {
    setAnchor(date)
    if (mode === 'week' && date > addDays(today, 56)) setMode('month')
  }

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
        subtitle="Planifie jusqu’à ton objectif, adapte-toi aux imprévus, compare le prévu au réalisé."
        actions={
          <>
            <button type="button" className="btn btn-ghost" onClick={() => setEventDlg({ date: today, event: null })}>
              <TriangleAlert className="size-4 text-warn" aria-hidden="true" /> Imprévu
            </button>
            <button type="button" className="btn btn-ghost" onClick={() => setGenOpen({})}>
              <Sparkles className="size-4 text-accent" aria-hidden="true" /> Générer
            </button>
            <button type="button" className="btn btn-primary" onClick={() => addSession(defaultAddDate)}>
              <CalendarPlus className="size-4" aria-hidden="true" /> Planifier une séance
            </button>
          </>
        }
      />

      {goal && (
        <GoalHorizon goals={dated} goal={goal} onGoal={setGoalPick} phases={phases.data ?? []} today={today} onJump={jump} onGenerate={() => setGenOpen({ horizon: 'goal' })} />
      )}

      <Card
        as="section"
        aria-label="Semaine affichée"
        title={<span className="num">{mode === 'month' ? monthTitle : weekLabel(weekStart)}</span>}
        subtitle={
          mode === 'month'
            ? 'Prévu vs réalisé sur le mois'
            : `${weekStart === currentWeek ? 'Semaine en cours' : weekStart < currentWeek ? 'Semaine passée' : 'Semaine à venir'}${weekPhase ? ` · ${weekPhase.name}${weekPhase.weeklyVolumeKm ? ` (${fmtNumber(weekPhase.weeklyVolumeKm)} km visés)` : ''}` : ''}`
        }
        icon={<CalendarRange className="size-4" />}
        action={
          <div className="flex flex-wrap items-center gap-1.5">
            <SegmentedControl
              ariaLabel="Affichage"
              size="sm"
              value={mode}
              onChange={setMode}
              options={[
                { value: 'week', label: 'Semaine', icon: <CalendarRange /> },
                { value: 'month', label: 'Mois', icon: <CalendarDays /> },
              ]}
            />
            <button type="button" className="icon-btn size-9" onClick={() => (mode === 'month' ? shiftMonth(-1) : setAnchor(addDays(weekStart, -7)))} aria-label={mode === 'month' ? 'Mois précédent' : 'Semaine précédente'}>
              <ChevronLeft className="size-4" />
            </button>
            <button type="button" className="btn btn-ghost h-9 min-h-9 px-3 text-xs max-sm:hidden" onClick={() => setAnchor(null)} disabled={mode === 'month' ? month === today.slice(0, 7) : weekStart === currentWeek}>
              Aujourd’hui
            </button>
            <button type="button" className="icon-btn size-9" onClick={() => (mode === 'month' ? shiftMonth(1) : setAnchor(addDays(weekStart, 7)))} aria-label={mode === 'month' ? 'Mois suivant' : 'Semaine suivante'}>
              <ChevronRight className="size-4" />
            </button>
            {goal && (
              <button type="button" className="icon-btn size-9" onClick={() => jump(goal.targetDate as string)} aria-label={`Aller à l’échéance : ${goal.title}`} title="Aller à l’échéance">
                <Flag className="size-4" />
              </button>
            )}
          </div>
        }
      >
        {mode === 'month' ? (
          <MonthCalendar
            month={month}
            today={today}
            onOpenSession={openSession}
            onOpenEvent={(e, date) => setEventDlg({ date, event: e })}
            goals={goalsByDate}
            phases={phases.data ?? []}
            onPickDay={(d) => {
              setAnchor(d)
              setMode('week')
            }}
          />
        ) : week.isError && !data ? (
          <InlineError>{week.error.message}</InlineError>
        ) : (
          <div aria-live="polite" aria-busy={week.isFetching}>
            <WeekTotals week={data} />
          </div>
        )}
      </Card>

      <div className={cn('mt-4 transition-opacity', stale && 'is-refetching', mode === 'month' && 'hidden')} aria-busy={week.isFetching}>
        <div className="hidden gap-3 xl:grid xl:grid-cols-7">
          {days.map((d) => (
            <DayBlock key={d.date} day={d} today={today} layout="column" events={eventsOf(d.date)} goal={goalsByDate.get(d.date) ?? null} onAdd={addSession} onEdit={editSession} onDelete={setDeleteSession} onOpen={openSession} onEvent={setEventDlg} />
          ))}
        </div>
        <div className="space-y-3 xl:hidden">
          {days.map((d) => (
            <DayBlock key={d.date} day={d} today={today} layout="row" events={eventsOf(d.date)} goal={goalsByDate.get(d.date) ?? null} onAdd={addSession} onEdit={editSession} onDelete={setDeleteSession} onOpen={openSession} onEvent={setEventDlg} />
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
      {genOpen && <GenerateDialog open onClose={() => setGenOpen(null)} today={today} goals={dated} initialHorizon={genOpen.horizon} initialGoalId={goal?.id} />}
      <SessionDrawer session={drawer?.s ?? null} startInEdit={drawer?.edit} onClose={() => setDrawer(null)} />
      {eventDlg && <EventDialog open onClose={() => setEventDlg(null)} today={today} date={eventDlg.date} event={eventDlg.event} adaptOnly={eventDlg.adaptOnly} />}
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
