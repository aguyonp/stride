import { CalendarArrowUp, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useCycleMutations } from '../../../api/queries'
import type { TrainingWeek } from '../../../api/types'
import { fmtDateShort } from '../../../lib/format'
import { Dialog } from '../../ui/Dialog'
import { Checkbox, Field } from '../../ui/Field'
import { InlineError } from '../../ui/Misc'
import { useToast } from '../../ui/Toast'
import { numberField, parseOptionalNumber, validateWeekTargets } from './cycleModel'
import { VolumeLoadGauges } from './Gauges'

export function WeekTuneDialog({
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
  const [volKm, setVolKm] = useState(numberField(week?.targetVolumeKm))
  const [load, setLoad] = useState(numberField(week?.targetLoad))
  const [isAssimilation, setIsAssimilation] = useState(week?.isAssimilation ?? false)
  const [note, setNote] = useState(week?.note ?? '')
  const [errMsg, setErrMsg] = useState<string | null>(null)

  if (!week) return null

  const save = () => {
    const vKm = parseOptionalNumber(volKm)
    const tLoad = parseOptionalNumber(load)
    const invalid = validateWeekTargets(vKm, tLoad)
    setErrMsg(invalid)
    if (invalid) return

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
          <VolumeLoadGauges actuals={week.actuals} volumeLabel="Volume réalisé" loadLabel="Charge réalisée" />
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
