import { Loader2, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useCycleMutations } from '../../../api/queries'
import type { PhaseType, TrainingPhase, TrainingPhaseInput } from '../../../api/types'
import { Dialog } from '../../ui/Dialog'
import { Field } from '../../ui/Field'
import { InlineError } from '../../ui/Misc'
import { useToast } from '../../ui/Toast'
import { PHASE_TYPE_META, PHASE_TYPES } from './cycleMeta'
import { numberField, parseOptionalNumber, validatePhaseForm } from './cycleModel'

export function PhaseDialog({
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
  const [volKm, setVolKm] = useState(numberField(phase?.weeklyVolumeKm))
  const [load, setLoad] = useState(numberField(phase?.targetWeeklyLoad))
  const [note, setNote] = useState(phase?.note ?? '')
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [errMsg, setErrMsg] = useState<string | null>(null)

  const busy = mut.createPhase.isPending || mut.updatePhase.isPending || mut.removePhase.isPending

  const submit = () => {
    const invalid = validatePhaseForm({ name, startDate, endDate })
    setErrMsg(invalid)
    if (invalid) return
    const vKm = parseOptionalNumber(volKm)
    const tLoad = parseOptionalNumber(load)

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
