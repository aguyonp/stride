import { CalendarRange, Layers, Plus, SlidersHorizontal } from 'lucide-react'
import { useState } from 'react'
import { useCycleMutations } from '../../api/queries'
import type { Goal, TrainingPhase, TrainingPlan, TrainingWeek } from '../../api/types'
import { fmtDateShort } from '../../lib/format'
import { Card } from '../ui/Card'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { EmptyState, InlineError } from '../ui/Misc'
import { Skeleton } from '../ui/Skeleton'
import { useToast } from '../ui/Toast'
import type { CycleZoom } from './cycles/cycleMeta'
import { nextPhaseDates, timelineSpan } from './cycles/cycleModel'
import { GanttStrip } from './cycles/GanttStrip'
import { GlobalGauges } from './cycles/Gauges'
import { MicrocycleList } from './cycles/MicrocycleList'
import { PhaseDialog } from './cycles/PhaseDialog'
import { PhaseList } from './cycles/PhaseList'
import { PlanDialog } from './cycles/PlanDialog'
import { CyclesToolbar, PlanStatusBar } from './cycles/PlanHeader'
import { useCyclesOverview } from './cycles/useCyclesOverview'
import { WeekTuneDialog } from './cycles/WeekTuneDialog'

export function CyclesTimeline({
  today,
  goals,
  onJumpToWeek,
}: {
  today: string
  goals: Goal[]
  onJumpToWeek: (weekStart: string) => void
}) {
  const [selectedPlanId, setSelectedPlanId] = useState<number | null>(null)
  const [zoom, setZoom] = useState<CycleZoom>('meso')
  const [planDlg, setPlanDlg] = useState<{ open: boolean; plan: TrainingPlan | null }>({ open: false, plan: null })
  const [phaseDlg, setPhaseDlg] = useState<{ open: boolean; phase: TrainingPhase | null }>({ open: false, phase: null })
  const [weekDlg, setWeekDlg] = useState<TrainingWeek | null>(null)
  const [deletePlan, setDeletePlan] = useState<TrainingPlan | null>(null)

  const { query, overview, plans, phases, currentPlan, allWeeks } = useCyclesOverview(selectedPlanId)
  const mut = useCycleMutations()
  const { toast } = useToast()

  if (query.isPending && !overview) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-40" />
        <Skeleton className="h-56" />
      </div>
    )
  }

  if (query.isError && !overview) {
    return <InlineError>{query.error.message}</InlineError>
  }

  const span = timelineSpan(currentPlan, phases, allWeeks, today)
  const nextPhase = nextPhaseDates(phases, currentPlan, span.start)
  const openNewPlan = () => setPlanDlg({ open: true, plan: null })
  const openPhase = (phase: TrainingPhase | null) => setPhaseDlg({ open: true, phase })

  return (
    <div className="space-y-5">
      <Card
        as="section"
        aria-label="Macrocycle et frise temporelle"
        icon={<Layers className="size-4" />}
        title={currentPlan ? currentPlan.name : 'Cycles d’entraînement'}
        subtitle={
          currentPlan
            ? `${fmtDateShort(currentPlan.startDate)} → ${fmtDateShort(currentPlan.endDate)}${currentPlan.targetEventLabel ? ` · ${currentPlan.targetEventLabel}` : ''}`
            : 'Structure ta saison en macrocycle, mésocycles et microcycles.'
        }
        action={
          <CyclesToolbar
            plans={plans}
            currentPlan={currentPlan}
            zoom={zoom}
            onSelectPlan={setSelectedPlanId}
            onZoom={setZoom}
            onEditPlan={(plan) => setPlanDlg({ open: true, plan })}
            onDeletePlan={setDeletePlan}
            onNewPlan={openNewPlan}
          />
        }
      >
        {!currentPlan && phases.length === 0 ? (
          <EmptyState
            icon={<Layers />}
            title="Aucun macrocycle actif"
            action={
              <button type="button" className="btn btn-primary" onClick={openNewPlan}>
                <Plus className="size-4" aria-hidden="true" /> Créer un macrocycle
              </button>
            }
          >
            Découpe automatiquement ta préparation en blocs Fondation, Développement, Spécifique et Affûtage avec suivi en direct du volume (km) et de la charge.
          </EmptyState>
        ) : (
          <div className="space-y-4">
            {currentPlan && <PlanStatusBar plan={currentPlan} today={today} />}
            {overview?.totals && <GlobalGauges totals={overview.totals} />}
            <GanttStrip
              startDate={span.start}
              endDate={span.end}
              today={today}
              phases={phases}
              allWeeks={allWeeks}
              zoom={zoom}
              onSelectPhase={openPhase}
              onSelectWeek={setWeekDlg}
            />
          </div>
        )}
      </Card>

      {(currentPlan || phases.length > 0) && zoom !== 'micro' && (
        <Card
          as="section"
          aria-label="Mésocycles"
          icon={<CalendarRange className="size-4" />}
          title="Mésocycles"
          subtitle="Comparaison prévu vs réalisé en volume et en charge par bloc"
          action={
            <button type="button" className="btn btn-ghost h-9 min-h-9 px-3 text-xs" onClick={() => openPhase(null)}>
              <Plus className="size-4" aria-hidden="true" /> Ajouter un mésocycle
            </button>
          }
        >
          <PhaseList phases={phases} today={today} zoom={zoom} onEditPhase={openPhase} onSelectWeek={setWeekDlg} />
        </Card>
      )}

      {allWeeks.length > 0 && zoom === 'micro' && (
        <Card
          as="section"
          aria-label="Microcycles hebdomadaires"
          icon={<SlidersHorizontal className="size-4" />}
          title="Microcycles (détail hebdomadaire)"
          subtitle="Clique une semaine pour ajuster le volume/charge cible ou basculer vers le calendrier hebdomadaire"
        >
          <MicrocycleList weeks={allWeeks} phases={phases} today={today} onTuneWeek={setWeekDlg} onJumpToWeek={onJumpToWeek} />
        </Card>
      )}

      {planDlg.open && (
        <PlanDialog
          key={planDlg.plan?.id ?? 'new-plan'}
          open
          plan={planDlg.plan}
          goals={goals}
          today={today}
          onClose={() => setPlanDlg({ open: false, plan: null })}
        />
      )}

      {phaseDlg.open && (
        <PhaseDialog
          key={phaseDlg.phase?.id ?? 'new-phase'}
          open
          planId={currentPlan?.id ?? null}
          phase={phaseDlg.phase}
          defaultStart={nextPhase.start}
          defaultEnd={nextPhase.end}
          nextOrderIndex={phases.length}
          onClose={() => setPhaseDlg({ open: false, phase: null })}
        />
      )}

      {weekDlg && (
        <WeekTuneDialog
          key={weekDlg.weekStart}
          week={weekDlg}
          planId={currentPlan?.id ?? null}
          onClose={() => setWeekDlg(null)}
          onJumpToWeek={onJumpToWeek}
        />
      )}

      <ConfirmDialog
        open={deletePlan !== null}
        onClose={() => setDeletePlan(null)}
        title="Supprimer ce macrocycle ?"
        message={
          deletePlan && (
            <p>
              « {deletePlan.name} » ainsi que son découpage en mésocycles seront supprimés. Les séances planifiées et activités réalisées sont conservées.
            </p>
          )
        }
        confirmLabel="Supprimer"
        danger
        busy={mut.removePlan.isPending}
        onConfirm={() =>
          deletePlan &&
          mut.removePlan.mutate(deletePlan.id, {
            onSuccess: () => {
              toast({ title: 'Macrocycle supprimé' })
              setSelectedPlanId(null)
              setDeletePlan(null)
            },
            onError: (e) => toast({ tone: 'error', title: 'Suppression impossible', description: e.message }),
          })
        }
      />
    </div>
  )
}
