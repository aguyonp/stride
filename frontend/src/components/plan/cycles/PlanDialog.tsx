import { Loader2 } from 'lucide-react'
import { useId, useState } from 'react'
import { useCycleMutations } from '../../../api/queries'
import type { CyclePlanStatus, Goal, TrainingPlan, TrainingPlanInput } from '../../../api/types'
import { fmtDateShort } from '../../../lib/format'
import { Dialog } from '../../ui/Dialog'
import { Checkbox, Field } from '../../ui/Field'
import { InlineError } from '../../ui/Misc'
import { useToast } from '../../ui/Toast'
import { parseOptionalNumber, planFormDefaults, validatePlanForm } from './cycleModel'

export function PlanDialog({
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
  const [init] = useState(() => planFormDefaults(plan, goals, today))

  const [name, setName] = useState(init.name)
  const [sport, setSport] = useState<'run' | 'bike' | 'other'>(init.sport)
  const [goalId, setGoalId] = useState<number | null>(init.goalId)
  const [startDate, setStartDate] = useState(init.startDate)
  const [endDate, setEndDate] = useState(init.endDate)
  const [targetEventLabel, setTargetEventLabel] = useState(init.targetEventLabel)
  const [status, setStatus] = useState<CyclePlanStatus>(init.status)
  const [autoScaffold, setAutoScaffold] = useState(true)
  const [peakVolume, setPeakVolume] = useState('52')
  const [peakLoad, setPeakLoad] = useState('420')
  const [note, setNote] = useState(init.note)
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
    const pv = parseOptionalNumber(peakVolume)
    const pl = parseOptionalNumber(peakLoad)
    const invalid = validatePlanForm({ name, startDate, endDate, peakVolume: pv, peakLoad: pl })
    setErrMsg(invalid)
    if (invalid) return

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
