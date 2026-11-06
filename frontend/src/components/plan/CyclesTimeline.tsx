import { CalendarArrowUp, CalendarRange, Flag, Layers, Loader2, Pencil, Plus, SlidersHorizontal, Trash2 } from 'lucide-react'
import { useId, useMemo, useState } from 'react'
import { useCycleMutations, useCycles } from '../../api/queries'
import type {
  CycleActuals,
  CyclePlanStatus,
  Goal,
  PhaseType,
  TrainingPhase,
  TrainingPhaseInput,
  TrainingPlan,
  TrainingPlanInput,
  TrainingWeek,
} from '../../api/types'
import { cn } from '../../lib/cn'
import { addDays, diffDays, fmtDateShort, fmtNumber, mondayOf } from '../../lib/format'
import { Card } from '../ui/Card'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { Dialog } from '../ui/Dialog'
import { Checkbox, Field } from '../ui/Field'
import { EmptyState, InlineError } from '../ui/Misc'
import { SegmentedControl } from '../ui/SegmentedControl'
import { Skeleton } from '../ui/Skeleton'
import { useToast } from '../ui/Toast'

export type CycleZoom = 'macro' | 'meso' | 'micro'

export const PHASE_TYPE_META: Record<
  PhaseType,
  { label: string; chipCls: string; barCls: string; dotCls: string }
> = {
  foundation: {
    label: 'Fondation',
    chipCls: 'border-info/30 bg-info/12 text-info',
    barCls: 'border-info/35 bg-info/14 text-ink',
    dotCls: 'bg-info',
  },
  build: {
    label: 'Développement',
    chipCls: 'border-accent/30 bg-accent/14 text-accent',
    barCls: 'border-accent/35 bg-accent/16 text-ink',
    dotCls: 'bg-accent',
  },
  threshold: {
    label: 'Seuil',
    chipCls: 'border-warn/30 bg-warn/12 text-warn',
    barCls: 'border-warn/35 bg-warn/14 text-ink',
    dotCls: 'bg-warn',
  },
  specific: {
    label: 'Spécifique',
    chipCls: 'border-violet/30 bg-violet/14 text-violet',
    barCls: 'border-violet/35 bg-violet/16 text-ink',
    dotCls: 'bg-violet',
  },
  assimilation: {
    label: 'Assimilation',
    chipCls: 'border-ok/30 bg-ok/12 text-ok',
    barCls: 'border-ok/35 bg-ok/14 text-ink',
    dotCls: 'bg-ok',
  },
  taper: {
    label: 'Affûtage',
    chipCls: 'border-accent/30 bg-accent/12 text-accent',
    barCls: 'border-accent/30 bg-accent/12 text-ink',
    dotCls: 'bg-accent',
  },
  recovery: {
    label: 'Récupération',
    chipCls: 'border-line-strong bg-track text-ink-2',
    barCls: 'border-line-strong bg-track/80 text-ink-2',
    dotCls: 'bg-ink-3',
  },
}

const PLAN_STATUS_LABEL: Record<CyclePlanStatus, { label: string; cls: string }> = {
  active: { label: 'Actif', cls: 'border-ok/30 bg-ok/12 text-ok' },
  completed: { label: 'Terminé', cls: 'border-info/30 bg-info/12 text-info' },
  archived: { label: 'Archivé', cls: 'border-line bg-track text-ink-3' },
}

const PHASE_TYPES = Object.keys(PHASE_TYPE_META) as PhaseType[]

function pctOf(actual: number, planned: number, explicitPct: number | null): number | null {
  if (explicitPct !== null && Number.isFinite(explicitPct)) return Math.round(explicitPct)
  if (planned <= 0) return null
  return Math.round((actual / planned) * 100)
}

function DualGauge({
  label,
  actual,
  planned,
  pct,
  unit,
  decimals = 0,
  compact = false,
}: {
  label: string
  actual: number
  planned: number
  pct: number | null
  unit: string
  decimals?: number
  compact?: boolean
}) {
  const computed = pctOf(actual, planned, pct)
  const barWidth = computed === null ? (actual > 0 ? 100 : 0) : Math.max(0, Math.min(100, computed))
  const overload = computed !== null && computed > 112
  return (
    <div className="min-w-0">
      <div className="flex items-baseline justify-between gap-2 text-xs">
        <span className="text-ink-3">{label}</span>
        <span className="num font-medium text-ink-2">
          {fmtNumber(actual, decimals)}
          <span className="text-ink-3">
            {' '}
            / {planned > 0 ? fmtNumber(planned, decimals) : '—'} {unit}
          </span>
          {computed !== null && <span className={cn('ml-1.5', overload ? 'text-warn' : 'text-ink-3')}>({computed} %)</span>}
        </span>
      </div>
      <div
        className={cn('mt-1 overflow-hidden rounded-full bg-track', compact ? 'h-1.5' : 'h-2')}
        role="progressbar"
        aria-label={`${label} : ${fmtNumber(actual, decimals)} sur ${planned > 0 ? fmtNumber(planned, decimals) : 0} ${unit}`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={barWidth}
      >
        <div
          className={cn('h-full rounded-full transition-[width] duration-500', overload ? 'bg-warn' : 'bg-accent')}
          style={{ width: `${barWidth}%` }}
        />
      </div>
    </div>
  )
}

function GlobalGauges({ totals }: { totals: CycleActuals }) {
  const sessPct = totals.sessionsPlanned > 0 ? Math.round((totals.sessionsCompleted / totals.sessionsPlanned) * 100) : null
  return (
    <div className="grid gap-4 sm:grid-cols-3">
      <DualGauge
        label="Volume cumulé"
        actual={totals.actualVolumeKm}
        planned={totals.plannedVolumeKm}
        pct={totals.volumeCompletionPct}
        unit="km"
        decimals={1}
      />
      <DualGauge
        label="Charge cumulée"
        actual={totals.actualLoad}
        planned={totals.plannedLoad}
        pct={totals.loadCompletionPct}
        unit="pts"
        decimals={0}
      />
      <DualGauge
        label="Séances validées"
        actual={totals.sessionsCompleted}
        planned={totals.sessionsPlanned}
        pct={sessPct}
        unit="séances"
        decimals={0}
      />
    </div>
  )
}

function GanttStrip({
  startDate,
  endDate,
  today,
  phases,
  allWeeks,
  zoom,
  onSelectPhase,
  onSelectWeek,
}: {
  startDate: string
  endDate: string
  today: string
  phases: TrainingPhase[]
  allWeeks: TrainingWeek[]
  zoom: CycleZoom
  onSelectPhase: (p: TrainingPhase) => void
  onSelectWeek: (w: TrainingWeek) => void
}) {
  const totalDays = Math.max(7, diffDays(startDate, endDate) + 1)
  const todayOffset = diffDays(startDate, today)
  const todayPct = todayOffset >= 0 && todayOffset <= totalDays ? (todayOffset / totalDays) * 100 : null
  const minWidthCls = zoom === 'micro' ? 'min-w-[66rem]' : zoom === 'meso' ? 'min-w-[44rem]' : 'min-w-full'

  return (
    <div className="scroll-thin overflow-x-auto rounded-2xl border border-line bg-track/30 p-3.5">
      <div className={cn('relative space-y-2.5', minWidthCls)}>
        <div className="num flex items-center justify-between text-[11px] text-ink-3">
          <span>{fmtDateShort(startDate)}</span>
          {todayPct !== null && <span className="font-semibold text-accent">Aujourd’hui · {fmtDateShort(today)}</span>}
          <span>{fmtDateShort(endDate)}</span>
        </div>

        <div className="relative min-h-14 rounded-xl border border-line bg-surface/40 p-1.5">
          {todayPct !== null && (
            <div
              className="pointer-events-none absolute inset-y-0 z-10 w-px bg-accent"
              style={{ left: `${todayPct}%` }}
              aria-hidden="true"
            >
              <span className="absolute -top-1.5 -translate-x-1/2 rounded-full bg-accent px-1.5 py-0.5 text-[9px] font-semibold text-on-accent">
                J
              </span>
            </div>
          )}

          {phases.length > 0 ? (
            <div className="relative h-12 w-full">
              {phases.map((p) => {
                const leftDays = Math.max(0, diffDays(startDate, p.startDate))
                const spanDays = Math.max(1, diffDays(p.startDate, p.endDate) + 1)
                const leftPct = Math.min(98, (leftDays / totalDays) * 100)
                const widthPct = Math.max(2, Math.min(100 - leftPct, (spanDays / totalDays) * 100))
                const meta = PHASE_TYPE_META[p.phaseType] ?? PHASE_TYPE_META.build
                return (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() => onSelectPhase(p)}
                    style={{ left: `${leftPct}%`, width: `calc(${widthPct}% - 2px)` }}
                    className={cn(
                      'absolute inset-y-0 flex flex-col justify-between overflow-hidden rounded-lg border px-2 py-1 text-left transition-opacity hover:opacity-90',
                      meta.barCls,
                    )}
                    aria-label={`Mésocycle ${p.name} (${meta.label}) du ${fmtDateShort(p.startDate)} au ${fmtDateShort(p.endDate)}`}
                  >
                    <span className="flex items-center gap-1.5 truncate text-xs font-semibold">
                      <span className={cn('size-2 flex-none rounded-full', meta.dotCls)} aria-hidden="true" />
                      <span className="truncate">{p.name}</span>
                    </span>
                    <span className="num truncate text-[10px] text-ink-2">
                      {meta.label} · {fmtNumber(p.actuals.actualVolumeKm, 0)}/{fmtNumber(p.actuals.plannedVolumeKm, 0)} km
                    </span>
                  </button>
                )
              })}
            </div>
          ) : (
            <p className="py-3 text-center text-xs text-ink-3">Aucun mésocycle découpé sur cette période.</p>
          )}
        </div>

        {allWeeks.length > 0 && (
          <div
            className="grid gap-1"
            style={{ gridTemplateColumns: `repeat(${allWeeks.length}, minmax(0, 1fr))` }}
            aria-label="Repères hebdomadaires (microcycles)"
          >
            {allWeeks.map((w) => {
              const isCurrent = w.weekStart <= today && w.weekEnd >= today
              const targetKm = w.targetVolumeKm ?? w.actuals.plannedVolumeKm
              return (
                <button
                  key={`${w.weekStart}-${w.id}`}
                  type="button"
                  onClick={() => onSelectWeek(w)}
                  className={cn(
                    'flex min-w-0 flex-col rounded-lg border px-1.5 py-1 text-left transition-colors hover:bg-track',
                    isCurrent
                      ? 'border-accent/55 bg-accent/10'
                      : w.isAssimilation
                        ? 'border-ok/35 bg-ok/[0.07]'
                        : 'border-line bg-surface/30',
                  )}
                  aria-label={`Semaine S${w.weekNumber} (${fmtDateShort(w.weekStart)})${w.isAssimilation ? ', semaine d’assimilation' : ''}`}
                >
                  <span className="flex items-center justify-between gap-1">
                    <span className={cn('num text-[10px] font-semibold', isCurrent ? 'text-accent' : 'text-ink-2')}>
                      S{w.weekNumber}
                    </span>
                    {w.isAssimilation && (
                      <span className="truncate rounded-sm bg-ok/20 px-1 text-[9px] font-semibold text-ok" title="Semaine d’assimilation">
                        Assim.
                      </span>
                    )}
                  </span>
                  {zoom !== 'macro' && (
                    <span className="num mt-0.5 truncate text-[10px] text-ink-3">
                      {fmtNumber(w.actuals.actualVolumeKm, 0)}/{targetKm ? fmtNumber(targetKm, 0) : '—'} km
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}

function PlanDialog({
  open,
  plan,
  goals,
  today,
  onClose,
}: {
  open: boolean
  plan: TrainingPlan | null
  goals: Goal[]
  today: string
  onClose: () => void
}) {
  const ids = useId()
  const mut = useCycleMutations()
  const { toast } = useToast()
  const raceGoal = goals.find((g) => g.status === 'active' && g.targetDate && g.targetDate > today) ?? null

  const [name, setName] = useState(plan?.name ?? (raceGoal ? `Préparation ${raceGoal.title}` : 'Préparation course'))
  const [sport, setSport] = useState<'run' | 'bike' | 'other'>(plan?.sport ?? 'run')
  const [goalId, setGoalId] = useState<number | null>(plan?.goalId ?? raceGoal?.id ?? null)
  const [startDate, setStartDate] = useState(plan?.startDate ?? mondayOf(today))
  const [endDate, setEndDate] = useState(plan?.endDate ?? raceGoal?.targetDate ?? addDays(mondayOf(today), 83))
  const [targetEventLabel, setTargetEventLabel] = useState(plan?.targetEventLabel ?? raceGoal?.title ?? '')
  const [status, setStatus] = useState<CyclePlanStatus>(plan?.status ?? 'active')
  const [autoScaffold, setAutoScaffold] = useState(true)
  const [peakVolume, setPeakVolume] = useState('52')
  const [peakLoad, setPeakLoad] = useState('420')
  const [note, setNote] = useState(plan?.note ?? '')
  const [errMsg, setErrMsg] = useState<string | null>(null)

  const busy = mut.createPlan.isPending || mut.updatePlan.isPending

  const onGoalPick = (nextId: number | null) => {
    setGoalId(nextId)
    const g = goals.find((x) => x.id === nextId)
    if (g && !plan) {
      if (g.targetDate) setEndDate(g.targetDate)
      setTargetEventLabel(g.title)
      if (!name.trim() || name.startsWith('Préparation')) setName(`Préparation ${g.title}`)
    }
  }

  const submit = () => {
    if (!name.trim()) return setErrMsg('Indique un nom pour ce macrocycle.')
    if (!startDate || !endDate || endDate <= startDate) return setErrMsg('La date de fin doit être postérieure au début.')
    const pv = peakVolume.trim() ? Number(peakVolume.replace(',', '.')) : null
    const pl = peakLoad.trim() ? Number(peakLoad.replace(',', '.')) : null
    if (pv !== null && (!Number.isFinite(pv) || pv <= 0)) return setErrMsg('Volume pic invalide.')
    if (pl !== null && (!Number.isFinite(pl) || pl <= 0)) return setErrMsg('Charge pic invalide.')
    setErrMsg(null)

    const payload: TrainingPlanInput = {
      name: name.trim(),
      goalId,
      sport,
      startDate,
      endDate,
      targetEventLabel: targetEventLabel.trim(),
      status,
      note: note.trim(),
      ...(plan ? {} : { autoScaffold, peakWeeklyVolumeKm: pv, peakWeeklyLoad: pl }),
    }

    if (plan) {
      mut.updatePlan.mutate(
        { id: plan.id, ...payload },
        {
          onSuccess: () => {
            toast({ title: 'Macrocycle mis à jour', tone: 'success' })
            onClose()
          },
          onError: (e) => setErrMsg(e.message),
        },
      )
    } else {
      mut.createPlan.mutate(payload, {
        onSuccess: () => {
          toast({ title: 'Macrocycle créé', description: autoScaffold ? 'Mésocycles et semaines d’assimilation générés.' : undefined, tone: 'success' })
          onClose()
        },
        onError: (e) => setErrMsg(e.message),
      })
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      size="lg"
      title={plan ? 'Modifier le macrocycle' : 'Nouveau macrocycle'}
      description="Structure ta préparation en mésocycles progressifs et semaines d’assimilation jusqu’à ton échéance."
      initialFocus={`#${CSS.escape(`${ids}-name`)}`}
      footer={
        <div className="flex justify-end gap-2 pb-2">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Annuler
          </button>
          <button type="button" className="btn btn-primary" onClick={submit} disabled={busy}>
            {busy && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {plan ? 'Enregistrer' : 'Créer le macrocycle'}
          </button>
        </div>
      }
    >
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <label htmlFor={`${ids}-name`} className="mb-1.5 block text-sm font-medium text-ink-2">
              Nom du macrocycle
            </label>
            <input
              id={`${ids}-name`}
              className="field"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={100}
              placeholder="Ex. Préparation Semi-Marathon d’automne"
            />
          </div>
          <Field label="Discipline principale">
            {(p) => (
              <select {...p} className="field" value={sport} onChange={(e) => setSport(e.target.value as 'run' | 'bike' | 'other')}>
                <option value="run">Course à pied</option>
                <option value="bike">Cyclisme</option>
                <option value="other">Multisport / Autre</option>
              </select>
            )}
          </Field>
          <Field label="Objectif lié (facultatif)">
            {(p) => (
              <select {...p} className="field" value={goalId ?? ''} onChange={(e) => onGoalPick(e.target.value ? Number(e.target.value) : null)}>
                <option value="">Aucun objectif lié</option>
                {goals.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.title}
                    {g.targetDate ? ` (${fmtDateShort(g.targetDate)})` : ''}
                  </option>
                ))}
              </select>
            )}
          </Field>
          <Field label="Début du cycle">{(p) => <input {...p} type="date" className="field num" value={startDate} onChange={(e) => setStartDate(e.target.value)} />}</Field>
          <Field label="Échéance / Course cible">{(p) => <input {...p} type="date" className="field num" value={endDate} onChange={(e) => setEndDate(e.target.value)} />}</Field>
          <Field label="Libellé de l’échéance" className={plan ? undefined : 'sm:col-span-2'}>
            {(p) => (
              <input
                {...p}
                className="field"
                value={targetEventLabel}
                onChange={(e) => setTargetEventLabel(e.target.value)}
                maxLength={100}
                placeholder="Ex. 10 km de Lyon — objectif 37 min"
              />
            )}
          </Field>
          {plan && (
            <Field label="Statut">
              {(p) => (
                <select {...p} className="field" value={status} onChange={(e) => setStatus(e.target.value as CyclePlanStatus)}>
                  <option value="active">Actif</option>
                  <option value="completed">Terminé</option>
                  <option value="archived">Archivé</option>
                </select>
              )}
            </Field>
          )}
        </div>

        {!plan && (
          <div className="space-y-3 rounded-2xl border border-line bg-track/35 p-3.5">
            <Checkbox
              checked={autoScaffold}
              onChange={setAutoScaffold}
              label="Découper automatiquement en mésocycles et semaines d’assimilation"
              description="Génère les blocs Fondation, Développement, Spécifique et Affûtage avec une semaine d’assimilation toutes les 3 à 4 semaines."
            />
            {autoScaffold && (
              <div className="grid gap-3 pt-1 sm:grid-cols-2">
                <Field label="Volume hebdo pic visé (km)">
                  {(p) => <input {...p} type="number" min={5} max={300} step={1} className="field num" value={peakVolume} onChange={(e) => setPeakVolume(e.target.value)} />}
                </Field>
                <Field label="Charge hebdo pic visée (pts)">
                  {(p) => <input {...p} type="number" min={50} max={3000} step={10} className="field num" value={peakLoad} onChange={(e) => setPeakLoad(e.target.value)} />}
                </Field>
              </div>
            )}
          </div>
        )}

        <Field label="Notes d’orientation (facultatif)">
          {(p) => (
            <textarea
              {...p}
              className="field min-h-20 resize-y"
              value={note}
              onChange={(e) => setNote(e.target.value)}
              maxLength={600}
              placeholder="Ex. progression prudente sur le D−, maintien renfo excentrique 2×/semaine"
            />
          )}
        </Field>
        {errMsg && <InlineError>{errMsg}</InlineError>}
      </form>
    </Dialog>
  )
}

function PhaseDialog({
  open,
  planId,
  phase,
  defaultStart,
  defaultEnd,
  nextOrderIndex,
  onClose,
}: {
  open: boolean
  planId: number | null
  phase: TrainingPhase | null
  defaultStart: string
  defaultEnd: string
  nextOrderIndex: number
  onClose: () => void
}) {
  const mut = useCycleMutations()
  const { toast } = useToast()
  const [phaseType, setPhaseType] = useState<PhaseType>(phase?.phaseType ?? 'build')
  const [name, setName] = useState(phase?.name ?? PHASE_TYPE_META[phase?.phaseType ?? 'build'].label)
  const [startDate, setStartDate] = useState(phase?.startDate ?? defaultStart)
  const [endDate, setEndDate] = useState(phase?.endDate ?? defaultEnd)
  const [focus, setFocus] = useState(phase?.focus ?? '')
  const [volKm, setVolKm] = useState(phase?.weeklyVolumeKm !== null && phase?.weeklyVolumeKm !== undefined ? String(phase.weeklyVolumeKm) : '')
  const [load, setLoad] = useState(phase?.targetWeeklyLoad !== null && phase?.targetWeeklyLoad !== undefined ? String(phase.targetWeeklyLoad) : '')
  const [note, setNote] = useState(phase?.note ?? '')
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [errMsg, setErrMsg] = useState<string | null>(null)

  const busy = mut.createPhase.isPending || mut.updatePhase.isPending || mut.removePhase.isPending

  const submit = () => {
    if (!name.trim()) return setErrMsg('Donne un nom à ce mésocycle.')
    if (!startDate || !endDate || endDate < startDate) return setErrMsg('La fin du mésocycle doit suivre le début.')
    const vKm = volKm.trim() ? Number(volKm.replace(',', '.')) : null
    const tLoad = load.trim() ? Number(load.replace(',', '.')) : null
    setErrMsg(null)

    const body: TrainingPhaseInput = {
      planId,
      name: name.trim(),
      phaseType,
      orderIndex: phase?.orderIndex ?? nextOrderIndex,
      startDate,
      endDate,
      focus: focus.trim(),
      weeklyVolumeKm: vKm,
      targetWeeklyLoad: tLoad,
      note: note.trim(),
    }

    if (phase) {
      mut.updatePhase.mutate(
        { id: phase.id, ...body },
        {
          onSuccess: () => {
            toast({ title: 'Mésocycle mis à jour', tone: 'success' })
            onClose()
          },
          onError: (e) => setErrMsg(e.message),
        },
      )
    } else {
      mut.createPhase.mutate(body, {
        onSuccess: () => {
          toast({ title: 'Mésocycle ajouté', tone: 'success' })
          onClose()
        },
        onError: (e) => setErrMsg(e.message),
      })
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={phase ? 'Modifier le mésocycle' : 'Nouveau mésocycle'}
      description="Définis la dominante physiologique et les repères hebdomadaires du bloc."
      footer={
        <div className="flex flex-wrap items-center justify-between gap-2 pb-2">
          {phase ? (
            confirmDelete ? (
              <button
                type="button"
                className="btn btn-danger"
                disabled={busy}
                onClick={() =>
                  mut.removePhase.mutate(phase.id, {
                    onSuccess: () => {
                      toast({ title: 'Mésocycle supprimé' })
                      onClose()
                    },
                    onError: (e) => setErrMsg(e.message),
                  })
                }
              >
                <Trash2 className="size-4" aria-hidden="true" /> Confirmer la suppression
              </button>
            ) : (
              <button type="button" className="btn btn-ghost text-danger" onClick={() => setConfirmDelete(true)}>
                <Trash2 className="size-4" aria-hidden="true" /> Supprimer
              </button>
            )
          ) : (
            <span />
          )}
          <div className="flex gap-2">
            <button type="button" className="btn btn-ghost" onClick={onClose}>
              Annuler
            </button>
            <button type="button" className="btn btn-primary" onClick={submit} disabled={busy}>
              {busy && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
              Enregistrer
            </button>
          </div>
        </div>
      }
    >
      <div className="space-y-3.5">
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Type de bloc">
            {(p) => (
              <select
                {...p}
                className="field"
                value={phaseType}
                onChange={(e) => {
                  const next = e.target.value as PhaseType
                  const prevDefault = PHASE_TYPE_META[phaseType].label
                  setPhaseType(next)
                  if (!phase && (!name.trim() || name === prevDefault)) setName(PHASE_TYPE_META[next].label)
                }}
              >
                {PHASE_TYPES.map((t) => (
                  <option key={t} value={t}>
                    {PHASE_TYPE_META[t].label}
                  </option>
                ))}
              </select>
            )}
          </Field>
          <Field label="Nom du mésocycle">
            {(p) => <input {...p} className="field" value={name} onChange={(e) => setName(e.target.value)} maxLength={80} />}
          </Field>
          <Field label="Début">{(p) => <input {...p} type="date" className="field num" value={startDate} onChange={(e) => setStartDate(e.target.value)} />}</Field>
          <Field label="Fin">{(p) => <input {...p} type="date" className="field num" value={endDate} onChange={(e) => setEndDate(e.target.value)} />}</Field>
          <Field label="Volume hebdo cible (km)">
            {(p) => <input {...p} type="number" min={0} max={300} step={0.5} className="field num" value={volKm} onChange={(e) => setVolKm(e.target.value)} />}
          </Field>
          <Field label="Charge hebdo cible (pts)">
            {(p) => <input {...p} type="number" min={0} max={3000} step={5} className="field num" value={load} onChange={(e) => setLoad(e.target.value)} />}
          </Field>
        </div>
        <Field label="Dominante / Focus">
          {(p) => <input {...p} className="field" value={focus} onChange={(e) => setFocus(e.target.value)} maxLength={140} placeholder="Ex. Endurance fondamentale + vélocité" />}
        </Field>
        <Field label="Note (facultatif)">
          {(p) => <textarea {...p} className="field min-h-16 resize-y" value={note} onChange={(e) => setNote(e.target.value)} maxLength={400} />}
        </Field>
        {errMsg && <InlineError>{errMsg}</InlineError>}
      </div>
    </Dialog>
  )
}

function WeekTuneDialog({
  week,
  planId,
  onClose,
  onJumpToWeek,
}: {
  week: TrainingWeek | null
  planId: number | null
  onClose: () => void
  onJumpToWeek: (weekStart: string) => void
}) {
  const mut = useCycleMutations()
  const { toast } = useToast()
  const [volKm, setVolKm] = useState(week?.targetVolumeKm !== null && week?.targetVolumeKm !== undefined ? String(week.targetVolumeKm) : '')
  const [load, setLoad] = useState(week?.targetLoad !== null && week?.targetLoad !== undefined ? String(week.targetLoad) : '')
  const [isAssimilation, setIsAssimilation] = useState(week?.isAssimilation ?? false)
  const [note, setNote] = useState(week?.note ?? '')
  const [errMsg, setErrMsg] = useState<string | null>(null)

  if (!week) return null

  const save = () => {
    const vKm = volKm.trim() ? Number(volKm.replace(',', '.')) : null
    const tLoad = load.trim() ? Number(load.replace(',', '.')) : null
    if (vKm !== null && (!Number.isFinite(vKm) || vKm < 0)) return setErrMsg('Volume cible invalide.')
    if (tLoad !== null && (!Number.isFinite(tLoad) || tLoad < 0)) return setErrMsg('Charge cible invalide.')
    setErrMsg(null)

    mut.upsertWeek.mutate(
      {
        planId: week.planId ?? planId,
        phaseId: week.phaseId,
        weekStart: week.weekStart,
        isAssimilation,
        targetVolumeKm: vKm,
        targetLoad: tLoad,
        note: note.trim(),
      },
      {
        onSuccess: () => {
          toast({ title: `Microcycle S${week.weekNumber} enregistré`, tone: 'success' })
          onClose()
        },
        onError: (e) => setErrMsg(e.message),
      },
    )
  }

  return (
    <Dialog
      open
      onClose={onClose}
      title={`Microcycle S${week.weekNumber} · ${fmtDateShort(week.weekStart)} → ${fmtDateShort(week.weekEnd)}`}
      description="Ajuste les objectifs de volume, de charge ou marque une semaine d’assimilation."
      footer={
        <div className="flex flex-wrap items-center justify-between gap-2 pb-2">
          <button
            type="button"
            className="btn btn-ghost text-xs"
            onClick={() => {
              onClose()
              onJumpToWeek(week.weekStart)
            }}
          >
            <CalendarArrowUp className="size-4" aria-hidden="true" /> Ouvrir dans Semaine
          </button>
          <div className="flex gap-2">
            <button type="button" className="btn btn-ghost" onClick={onClose}>
              Annuler
            </button>
            <button type="button" className="btn btn-primary" onClick={save} disabled={mut.upsertWeek.isPending}>
              {mut.upsertWeek.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
              Enregistrer
            </button>
          </div>
        </div>
      }
    >
      <div className="space-y-4">
        <div className="grid gap-3 rounded-2xl border border-line bg-track/30 p-3 sm:grid-cols-2">
          <DualGauge
            label="Volume réalisé"
            actual={week.actuals.actualVolumeKm}
            planned={week.actuals.plannedVolumeKm}
            pct={week.actuals.volumeCompletionPct}
            unit="km"
            decimals={1}
            compact
          />
          <DualGauge
            label="Charge réalisée"
            actual={week.actuals.actualLoad}
            planned={week.actuals.plannedLoad}
            pct={week.actuals.loadCompletionPct}
            unit="pts"
            decimals={0}
            compact
          />
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Volume cible (km)">
            {(p) => <input {...p} type="number" min={0} max={300} step={0.5} className="field num" value={volKm} onChange={(e) => setVolKm(e.target.value)} />}
          </Field>
          <Field label="Charge cible (pts)">
            {(p) => <input {...p} type="number" min={0} max={3000} step={5} className="field num" value={load} onChange={(e) => setLoad(e.target.value)} />}
          </Field>
        </div>
        <Checkbox
          checked={isAssimilation}
          onChange={setIsAssimilation}
          label="Semaine d’assimilation"
          description="Allège le volume (−20 à −30 %) pour absorber la charge des semaines précédentes."
        />
        <Field label="Consigne de la semaine">
          {(p) => <textarea {...p} className="field min-h-20 resize-y" value={note} onChange={(e) => setNote(e.target.value)} maxLength={400} placeholder="Ex. Priorité fraîcheur musculaire, 1 seul travail d’allure" />}
        </Field>
        {errMsg && <InlineError>{errMsg}</InlineError>}
      </div>
    </Dialog>
  )
}

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

  const cycles = useCycles(selectedPlanId)
  const mut = useCycleMutations()
  const { toast } = useToast()

  const overview = cycles.data
  const plans = overview?.plans ?? []
  const currentPlan =
    (selectedPlanId !== null ? plans.find((p) => p.id === selectedPlanId) : null) ??
    overview?.activePlan ??
    plans[0] ??
    null
  const phases = overview?.phases ?? []

  const allWeeks = useMemo(() => {
    const map = new Map<string, TrainingWeek>()
    for (const ph of phases) {
      for (const w of ph.weeks) map.set(w.weekStart, w)
    }
    for (const w of overview?.standaloneWeeks ?? []) {
      if (!map.has(w.weekStart)) map.set(w.weekStart, w)
    }
    return [...map.values()].sort((a, b) => a.weekStart.localeCompare(b.weekStart))
  }, [phases, overview?.standaloneWeeks])

  if (cycles.isPending && !overview) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-40" />
        <Skeleton className="h-56" />
      </div>
    )
  }

  if (cycles.isError && !overview) {
    return <InlineError>{cycles.error.message}</InlineError>
  }

  const remainingDays = currentPlan ? diffDays(today, currentPlan.endDate) : null
  const remainingWeeks = remainingDays !== null && remainingDays > 0 ? Math.ceil(remainingDays / 7) : 0
  const spanStart = currentPlan?.startDate ?? phases[0]?.startDate ?? allWeeks[0]?.weekStart ?? mondayOf(today)
  const spanEnd = currentPlan?.endDate ?? phases[phases.length - 1]?.endDate ?? allWeeks[allWeeks.length - 1]?.weekEnd ?? addDays(spanStart, 83)

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
          <div className="flex flex-wrap items-center gap-2">
            {plans.length > 1 && (
              <select
                aria-label="Choisir un macrocycle"
                className="field h-9 min-h-9 w-auto py-1 text-xs"
                value={currentPlan?.id ?? ''}
                onChange={(e) => setSelectedPlanId(Number(e.target.value) || null)}
              >
                {plans.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            )}
            <SegmentedControl<CycleZoom>
              ariaLabel="Niveau de zoom des cycles"
              size="sm"
              value={zoom}
              onChange={setZoom}
              options={[
                { value: 'macro', label: 'Macrocycle' },
                { value: 'meso', label: 'Mésocycles' },
                { value: 'micro', label: 'Microcycles' },
              ]}
            />
            {currentPlan && (
              <>
                <button
                  type="button"
                  className="icon-btn size-9"
                  onClick={() => setPlanDlg({ open: true, plan: currentPlan })}
                  aria-label="Modifier le macrocycle"
                >
                  <Pencil className="size-4" />
                </button>
                <button
                  type="button"
                  className="icon-btn size-9"
                  onClick={() => setDeletePlan(currentPlan)}
                  aria-label="Supprimer le macrocycle"
                >
                  <Trash2 className="size-4" />
                </button>
              </>
            )}
            <button
              type="button"
              className="btn btn-primary h-9 min-h-9 px-3 text-xs"
              onClick={() => setPlanDlg({ open: true, plan: null })}
            >
              <Plus className="size-4" aria-hidden="true" /> Nouveau macrocycle
            </button>
          </div>
        }
      >
        {!currentPlan && phases.length === 0 ? (
          <EmptyState
            icon={<Layers />}
            title="Aucun macrocycle actif"
            action={
              <button type="button" className="btn btn-primary" onClick={() => setPlanDlg({ open: true, plan: null })}>
                <Plus className="size-4" aria-hidden="true" /> Créer un macrocycle
              </button>
            }
          >
            Découpe automatiquement ta préparation en blocs Fondation, Développement, Spécifique et Affûtage avec suivi en direct du volume (km) et de la charge.
          </EmptyState>
        ) : (
          <div className="space-y-4">
            {currentPlan && (
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-2xl border border-line bg-track/30 px-3.5 py-2.5">
                <div className="flex flex-wrap items-center gap-2 text-xs">
                  <span className={cn('chip border', PLAN_STATUS_LABEL[currentPlan.status].cls)}>
                    {PLAN_STATUS_LABEL[currentPlan.status].label}
                  </span>
                  {currentPlan.targetEventLabel && (
                    <span className="inline-flex items-center gap-1.5 font-medium text-ink">
                      <Flag className="size-3.5 text-accent" aria-hidden="true" />
                      {currentPlan.targetEventLabel}
                    </span>
                  )}
                  {currentPlan.note && <span className="text-ink-3">· {currentPlan.note}</span>}
                </div>
                {remainingDays !== null && (
                  <span className="num text-xs font-semibold text-accent">
                    {remainingDays > 0
                      ? `J−${remainingDays} · ${remainingWeeks} semaine${remainingWeeks > 1 ? 's' : ''} restante${remainingWeeks > 1 ? 's' : ''}`
                      : remainingDays === 0
                        ? 'Jour J aujourd’hui'
                        : 'Échéance passée'}
                  </span>
                )}
              </div>
            )}

            {overview?.totals && <GlobalGauges totals={overview.totals} />}

            <GanttStrip
              startDate={spanStart}
              endDate={spanEnd}
              today={today}
              phases={phases}
              allWeeks={allWeeks}
              zoom={zoom}
              onSelectPhase={(p) => setPhaseDlg({ open: true, phase: p })}
              onSelectWeek={(w) => setWeekDlg(w)}
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
            <button
              type="button"
              className="btn btn-ghost h-9 min-h-9 px-3 text-xs"
              onClick={() => setPhaseDlg({ open: true, phase: null })}
            >
              <Plus className="size-4" aria-hidden="true" /> Ajouter un mésocycle
            </button>
          }
        >
          {phases.length === 0 ? (
            <p className="text-sm text-ink-3">Aucun mésocycle enregistré pour ce plan.</p>
          ) : (
            <ul className="divide-y divide-line">
              {phases.map((p) => {
                const meta = PHASE_TYPE_META[p.phaseType] ?? PHASE_TYPE_META.build
                const isCurrent = p.startDate <= today && p.endDate >= today
                return (
                  <li key={p.id} className="py-3.5 first:pt-0 last:pb-0">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <h3 className="text-sm font-semibold text-ink">{p.name}</h3>
                          <span className={cn('chip h-5 border px-2 text-[11px]', meta.chipCls)}>{meta.label}</span>
                          {isCurrent && <span className="chip h-5 bg-accent/15 px-2 text-[10px] text-accent">En cours</span>}
                          <span className="num text-xs text-ink-3">
                            {fmtDateShort(p.startDate)} → {fmtDateShort(p.endDate)} · {p.weeks.length} sem.
                          </span>
                        </div>
                        {p.focus && <p className="mt-0.5 text-xs text-ink-2">{p.focus}</p>}
                      </div>
                      <div className="flex items-center gap-2">
                        <span className="num text-xs text-ink-3">
                          {p.actuals.sessionsCompleted}/{p.actuals.sessionsPlanned} séances
                        </span>
                        <button
                          type="button"
                          className="icon-btn size-8"
                          onClick={() => setPhaseDlg({ open: true, phase: p })}
                          aria-label={`Modifier le mésocycle ${p.name}`}
                        >
                          <Pencil className="size-3.5" />
                        </button>
                      </div>
                    </div>

                    <div className="mt-2.5 grid gap-3 sm:grid-cols-2">
                      <DualGauge
                        label="Volume"
                        actual={p.actuals.actualVolumeKm}
                        planned={p.actuals.plannedVolumeKm}
                        pct={p.actuals.volumeCompletionPct}
                        unit="km"
                        decimals={1}
                        compact
                      />
                      <DualGauge
                        label="Charge"
                        actual={p.actuals.actualLoad}
                        planned={p.actuals.plannedLoad}
                        pct={p.actuals.loadCompletionPct}
                        unit="pts"
                        decimals={0}
                        compact
                      />
                    </div>

                    {zoom === 'meso' && p.weeks.length > 0 && (
                      <div className="mt-3 flex flex-wrap gap-1.5">
                        {p.weeks.map((w) => (
                          <button
                            key={w.weekStart}
                            type="button"
                            onClick={() => setWeekDlg(w)}
                            className={cn(
                              'inline-flex items-center gap-1.5 rounded-xl border px-2.5 py-1 text-xs transition-colors hover:bg-track',
                              w.isAssimilation ? 'border-ok/35 bg-ok/[0.08] text-ink' : 'border-line bg-track/30 text-ink-2',
                            )}
                          >
                            <span className="num font-semibold text-ink">S{w.weekNumber}</span>
                            <span className="num text-ink-3">
                              {fmtNumber(w.actuals.actualVolumeKm, 0)}/{fmtNumber(w.targetVolumeKm ?? w.actuals.plannedVolumeKm, 0)} km
                            </span>
                            {w.isAssimilation && <span className="text-[10px] font-semibold text-ok">Semaine d’assimilation</span>}
                          </button>
                        ))}
                      </div>
                    )}
                  </li>
                )
              })}
            </ul>
          )}
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
          <div className="divide-y divide-line">
            {allWeeks.map((w) => {
              const isCurrent = w.weekStart <= today && w.weekEnd >= today
              const phase = phases.find((p) => p.id === w.phaseId)
              const plannedKm = w.targetVolumeKm ?? w.actuals.plannedVolumeKm
              const plannedLoad = w.targetLoad ?? w.actuals.plannedLoad
              return (
                <div key={w.weekStart} className="flex flex-col gap-3 py-3 first:pt-0 last:pb-0 lg:flex-row lg:items-center lg:justify-between">
                  <div className="min-w-0 lg:w-72">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className={cn('num text-sm font-semibold', isCurrent ? 'text-accent' : 'text-ink')}>
                        S{w.weekNumber}
                      </span>
                      <span className="num text-xs text-ink-3">
                        {fmtDateShort(w.weekStart)} → {fmtDateShort(w.weekEnd)}
                      </span>
                      {w.isAssimilation && (
                        <span className="chip h-5 border border-ok/30 bg-ok/12 px-2 text-[10px] text-ok">
                          Semaine d’assimilation
                        </span>
                      )}
                    </div>
                    <p className="mt-0.5 truncate text-xs text-ink-3">
                      {phase ? phase.name : 'Hors mésocycle'}
                      {w.note ? ` · ${w.note}` : ''}
                    </p>
                  </div>

                  <div className="grid flex-1 gap-3 sm:grid-cols-2">
                    <DualGauge
                      label="Volume"
                      actual={w.actuals.actualVolumeKm}
                      planned={plannedKm}
                      pct={w.actuals.volumeCompletionPct}
                      unit="km"
                      decimals={1}
                      compact
                    />
                    <DualGauge
                      label="Charge"
                      actual={w.actuals.actualLoad}
                      planned={plannedLoad}
                      pct={w.actuals.loadCompletionPct}
                      unit="pts"
                      decimals={0}
                      compact
                    />
                  </div>

                  <div className="flex flex-none items-center justify-end gap-1.5">
                    <span className="num mr-1 text-xs text-ink-3">
                      {w.actuals.sessionsCompleted}/{w.actuals.sessionsPlanned}
                    </span>
                    <button
                      type="button"
                      className="btn btn-ghost h-8 min-h-8 px-2.5 text-xs"
                      onClick={() => setWeekDlg(w)}
                    >
                      Ajuster
                    </button>
                    <button
                      type="button"
                      className="btn btn-ghost h-8 min-h-8 px-2.5 text-xs"
                      onClick={() => onJumpToWeek(w.weekStart)}
                    >
                      Semaine
                    </button>
                  </div>
                </div>
              )
            })}
          </div>
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
          defaultStart={phases.length ? addDays(phases[phases.length - 1].endDate, 1) : spanStart}
          defaultEnd={currentPlan?.endDate ?? addDays(spanStart, 27)}
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
