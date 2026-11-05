import { Loader2 } from 'lucide-react'
import { useEffect, useId, useState, type FormEvent, type ReactNode } from 'react'
import { usePlanMutations } from '../../api/queries'
import type { PlannedSession, PlannedSessionInput, PlanSport, WorkoutStep } from '../../api/types'
import { capitalize, fmtDateLong, isNum, paceInput, parsePace } from '../../lib/format'
import { PLAN_SPORT_LABEL } from '../../lib/labels'
import { Dialog } from '../ui/Dialog'
import { Field } from '../ui/Field'
import { InlineError } from '../ui/Misc'
import { useToast } from '../ui/Toast'
import { StepsEditor } from './StepsEditor'
import { stepsSummary, totalDurationS } from './steps'

interface PlannedSessionDialogProps {
  open: boolean
  onClose: () => void
  /** Session to edit; null to create one on `defaultDate`. */
  session: PlannedSession | null
  defaultDate: string
}

interface FormState {
  date: string
  sport: PlanSport
  title: string
  durationMin: string
  distanceKm: string
  paceMin: string
  paceMax: string
  cadence: string
  instructions: string
  steps: WorkoutStep[]
}

const SPORTS = Object.keys(PLAN_SPORT_LABEL) as PlanSport[]

function toForm(s: PlannedSession | null, date: string): FormState {
  return {
    date: s?.date ?? date,
    sport: s?.sport ?? 'run',
    title: s?.title ?? '',
    durationMin: isNum(s?.targetDurationS) ? String(Math.round(s.targetDurationS / 60)) : '',
    distanceKm: isNum(s?.targetDistanceM) ? String(Math.round(s.targetDistanceM / 100) / 10).replace('.', ',') : '',
    paceMin: paceInput(s?.targetPaceMinSecPerKm),
    paceMax: paceInput(s?.targetPaceMaxSecPerKm),
    cadence: isNum(s?.targetCadenceSpm) ? String(s.targetCadenceSpm) : '',
    instructions: s?.instructions ?? '',
    steps: s?.steps ?? [],
  }
}

const num = (v: string) => {
  const n = Number(v.replace(',', '.').trim())
  return v.trim() !== '' && Number.isFinite(n) && n > 0 ? n : null
}

/** Full planned-session form (create or edit), shared by the create dialog and the session drawer. */
export function useSessionEditor(session: PlannedSession | null, defaultDate: string, open: boolean, onDone: (saved: PlannedSession) => void): { formId: string; pending: boolean; date: string; form: ReactNode } {
  const formId = useId()
  const { toast } = useToast()
  const { create, update } = usePlanMutations()
  const [f, setF] = useState<FormState>(() => toForm(session, defaultDate))
  const [submitted, setSubmitted] = useState(false)
  const openKey = open ? `${session?.id ?? 'new'}:${defaultDate}` : null
  const [lastKey, setLastKey] = useState<string | null>(openKey)
  if (openKey !== lastKey) {
    setLastKey(openKey)
    if (openKey) {
      setF(toForm(session, defaultDate))
      setSubmitted(false)
    }
  }
  const resetCreate = create.reset
  const resetUpdate = update.reset
  useEffect(() => {
    if (openKey) {
      resetCreate()
      resetUpdate()
    }
  }, [openKey, resetCreate, resetUpdate])
  const set = <K extends keyof FormState>(k: K, v: FormState[K]) => setF((s) => ({ ...s, [k]: v }))
  const isRest = f.sport === 'rest'
  const isRun = f.sport === 'run'
  const showDistance = isRun || f.sport === 'bike' || f.sport === 'other'
  const paceMinS = f.paceMin.trim() ? parsePace(f.paceMin) : null
  const paceMaxS = f.paceMax.trim() ? parsePace(f.paceMax) : null
  const errors = {
    title: !f.title.trim() ? 'Donne un titre à la séance.' : null,
    date: !/^\d{4}-\d{2}-\d{2}$/.test(f.date) ? 'Date invalide.' : null,
    paceMin: f.paceMin.trim() && paceMinS === null ? 'Format m:ss attendu (ex. 5:10).' : null,
    paceMax: f.paceMax.trim() && paceMaxS === null ? 'Format m:ss attendu (ex. 5:20).' : null,
    duration: f.durationMin.trim() && num(f.durationMin) === null ? 'Durée en minutes (nombre positif).' : null,
    distance: f.distanceKm.trim() && num(f.distanceKm) === null ? 'Distance en km (nombre positif).' : null,
    cadence: f.cadence.trim() && (num(f.cadence) === null || (num(f.cadence) as number) > 260) ? 'Cadence en pas/min (ex. 172).' : null,
  }
  const invalid = Object.values(errors).some(Boolean)
  const show = (k: keyof typeof errors) => (submitted ? errors[k] : k === 'title' || k === 'date' ? null : errors[k])
  const pending = create.isPending || update.isPending
  const mutationError = create.error ?? update.error

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    setSubmitted(true)
    if (invalid) return
    const [lo, hi] = paceMinS !== null && paceMaxS !== null && paceMinS > paceMaxS ? [paceMaxS, paceMinS] : [paceMinS, paceMaxS]
    const duration = num(f.durationMin)
    const distance = num(f.distanceKm)
    const body: PlannedSessionInput = {
      date: f.date,
      sport: f.sport,
      title: f.title.trim(),
      targetDurationS: !isRest && duration !== null ? Math.round(duration * 60) : null,
      targetDistanceM: showDistance && distance !== null ? Math.round(distance * 1000) : null,
      targetPaceMinSecPerKm: isRun ? lo : null,
      targetPaceMaxSecPerKm: isRun ? hi : null,
      targetCadenceSpm: isRun && num(f.cadence) !== null ? Math.round(num(f.cadence) as number) : null,
      instructions: f.instructions.trim(),
      steps: isRest ? [] : f.steps,
    }
    if (body.targetDurationS === null && !isRest) body.targetDurationS = totalDurationS(f.steps)
    const done = (saved: PlannedSession) => {
      toast({ title: session ? 'Séance mise à jour' : 'Séance planifiée', description: `${capitalize(fmtDateLong(body.date))} · ${body.title}` })
      onDone(saved)
    }
    if (session) update.mutate({ id: session.id, ...body }, { onSuccess: done })
    else create.mutate(body, { onSuccess: done })
  }

  const form = (
    <form id={formId} onSubmit={onSubmit} noValidate className="grid gap-4 sm:grid-cols-2">
      <Field label="Date" error={show('date')}>
        {(p) => <input {...p} type="date" className="field" value={f.date} onChange={(e) => set('date', e.target.value)} required />}
      </Field>
      <Field label="Sport">
        {(p) => (
          <select {...p} className="field" value={f.sport} onChange={(e) => set('sport', e.target.value as PlanSport)}>
            {SPORTS.map((s) => (
              <option key={s} value={s}>
                {PLAN_SPORT_LABEL[s]}
              </option>
            ))}
          </select>
        )}
      </Field>
      <Field label="Titre" className="sm:col-span-2" error={show('title')}>
        {(p) => (
          <input
            {...p}
            name="title"
            className="field"
            value={f.title}
            onChange={(e) => set('title', e.target.value)}
            placeholder={isRest ? 'Repos complet' : isRun ? 'Footing 45 min continu' : 'Séance'}
            maxLength={120}
            autoComplete="off"
          />
        )}
      </Field>
      {!isRest && (
        <Field label="Durée cible (min)" error={show('duration')}>
          {(p) => <input {...p} className="field num" inputMode="numeric" value={f.durationMin} onChange={(e) => set('durationMin', e.target.value)} placeholder="45" />}
        </Field>
      )}
      {showDistance && (
        <Field label="Distance cible (km)" error={show('distance')}>
          {(p) => <input {...p} className="field num" inputMode="decimal" value={f.distanceKm} onChange={(e) => set('distanceKm', e.target.value)} placeholder="8,5" />}
        </Field>
      )}
      {isRun && (
        <>
          <Field label="Allure rapide (m:ss/km)" error={show('paceMin')} hint="Borne la plus rapide de la plage">
            {(p) => <input {...p} className="field num" inputMode="numeric" value={f.paceMin} onChange={(e) => set('paceMin', e.target.value)} placeholder="5:10" />}
          </Field>
          <Field label="Allure lente (m:ss/km)" error={show('paceMax')}>
            {(p) => <input {...p} className="field num" inputMode="numeric" value={f.paceMax} onChange={(e) => set('paceMax', e.target.value)} placeholder="5:20" />}
          </Field>
          <Field label="Cadence cible (spm)" error={show('cadence')}>
            {(p) => <input {...p} className="field num" inputMode="numeric" value={f.cadence} onChange={(e) => set('cadence', e.target.value)} placeholder="172" />}
          </Field>
        </>
      )}
      <Field label="Consignes" className="sm:col-span-2" hint="Échauffement, blocs, sensations visées… (repris dans le prompt IA)">
        {(p) => (
          <textarea {...p} className="field min-h-24 resize-y" value={f.instructions} onChange={(e) => set('instructions', e.target.value)} maxLength={2000} placeholder="10 min échauffement, 4 × 5 min allure seuil, récup 2 min trot" />
        )}
      </Field>
      {!isRest && (
        <fieldset className="sm:col-span-2">
          <legend className="mb-1.5 text-sm font-medium text-ink-2">Structure (étapes)</legend>
          <p className="mb-2 text-xs text-ink-3">{f.steps.length ? stepsSummary(f.steps) : 'Facultatif : utilisée pour le fichier .FIT et l’envoi sur la montre.'}</p>
          <StepsEditor steps={f.steps} onChange={(steps) => set('steps', steps)} />
        </fieldset>
      )}
      {mutationError && <InlineError className="sm:col-span-2">{mutationError.message}</InlineError>}
    </form>
  )
  return { formId, pending, date: f.date, form }
}

export function PlannedSessionDialog({ open, onClose, session, defaultDate }: PlannedSessionDialogProps) {
  const ed = useSessionEditor(session, defaultDate, open, () => onClose())
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={session ? 'Modifier la séance' : 'Planifier une séance'}
      description={ed.date ? capitalize(fmtDateLong(ed.date)) : undefined}
      size="lg"
      initialFocus={session ? undefined : 'input[name="title"]'}
      footer={
        <div className="flex justify-end gap-2">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Annuler
          </button>
          <button type="submit" form={ed.formId} className="btn btn-primary" disabled={ed.pending}>
            {ed.pending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {session ? 'Enregistrer' : 'Planifier'}
          </button>
        </div>
      }
    >
      {ed.form}
    </Dialog>
  )
}
