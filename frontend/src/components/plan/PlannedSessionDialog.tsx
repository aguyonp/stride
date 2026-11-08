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
import { t } from '../../i18n'

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

/** Alternative persistence (e.g. the physio/coach share portal) with optional date bounds. */
export interface SessionEditorOptions {
  save?: {
    create: (body: PlannedSessionInput, onSuccess: (saved: PlannedSession) => void) => void
    update: (id: number, body: PlannedSessionInput, onSuccess: (saved: PlannedSession) => void) => void
    pending: boolean
    error: Error | null
    reset: () => void
  }
  minDate?: string
  maxDate?: string
}

/** Full planned-session form (create or edit), shared by the create dialog, the session drawer and the share portal. */
export function useSessionEditor(session: PlannedSession | null, defaultDate: string, open: boolean, onDone: (saved: PlannedSession) => void, opts: SessionEditorOptions = {}): { formId: string; pending: boolean; date: string; form: ReactNode } {
  const formId = useId()
  const { toast } = useToast()
  const own = usePlanMutations()
  const { create, update } = own
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
  const resetOther = opts.save?.reset
  useEffect(() => {
    if (openKey) {
      resetCreate()
      resetUpdate()
      resetOther?.()
    }
  }, [openKey, resetCreate, resetUpdate, resetOther])
  const set = <K extends keyof FormState>(k: K, v: FormState[K]) => setF((s) => ({ ...s, [k]: v }))
  const isRest = f.sport === 'rest'
  const isRun = f.sport === 'run'
  const showDistance = isRun || f.sport === 'bike' || f.sport === 'other'
  const paceMinS = f.paceMin.trim() ? parsePace(f.paceMin) : null
  const paceMaxS = f.paceMax.trim() ? parsePace(f.paceMax) : null
  const errors = {
    title: !f.title.trim() ? t('plan.sessionDialog.titleRequired') : null,
    date: !/^\d{4}-\d{2}-\d{2}$/.test(f.date) ? t('plan.sessionDialog.dateInvalid') : (opts.minDate && f.date < opts.minDate) || (opts.maxDate && f.date > opts.maxDate) ? t('plan.sessionDialog.dateOutOfRange') : null,
    paceMin: f.paceMin.trim() && paceMinS === null ? t('plan.sessionDialog.paceMinFormat') : null,
    paceMax: f.paceMax.trim() && paceMaxS === null ? t('plan.sessionDialog.paceMaxFormat') : null,
    duration: f.durationMin.trim() && num(f.durationMin) === null ? t('plan.sessionDialog.durationInvalid') : null,
    distance: f.distanceKm.trim() && num(f.distanceKm) === null ? t('plan.sessionDialog.distanceInvalid') : null,
    cadence: f.cadence.trim() && (num(f.cadence) === null || (num(f.cadence) as number) > 260) ? t('plan.sessionDialog.cadenceInvalid') : null,
  }
  const invalid = Object.values(errors).some(Boolean)
  const show = (k: keyof typeof errors) => (submitted ? errors[k] : k === 'title' || k === 'date' ? null : errors[k])
  const pending = opts.save ? opts.save.pending : create.isPending || update.isPending
  const mutationError = opts.save ? opts.save.error : (create.error ?? update.error)

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
      toast({ title: session ? t('plan.sessionDialog.updated') : t('plan.sessionDialog.planned'), description: `${capitalize(fmtDateLong(body.date))} · ${body.title}` })
      onDone(saved)
    }
    if (opts.save) {
      if (session) opts.save.update(session.id, body, done)
      else opts.save.create(body, done)
    } else if (session) update.mutate({ id: session.id, ...body }, { onSuccess: done })
    else create.mutate(body, { onSuccess: done })
  }

  const form = (
    <form id={formId} onSubmit={onSubmit} noValidate className="grid gap-4 sm:grid-cols-2">
      <Field label={t('plan.sessionDialog.date')} error={show('date')}>
        {(p) => <input {...p} type="date" className="field" value={f.date} min={opts.minDate} max={opts.maxDate} onChange={(e) => set('date', e.target.value)} required />}
      </Field>
      <Field label={t('plan.sessionDialog.sport')}>
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
      <Field label={t('plan.sessionDialog.title')} className="sm:col-span-2" error={show('title')}>
        {(p) => (
          <input
            {...p}
            name="title"
            className="field"
            value={f.title}
            onChange={(e) => set('title', e.target.value)}
            placeholder={isRest ? t('plan.sessionDialog.titleRest') : isRun ? t('plan.sessionDialog.titleRun') : t('plan.sessionDialog.titleOther')}
            maxLength={120}
            autoComplete="off"
          />
        )}
      </Field>
      {!isRest && (
        <Field label={t('plan.sessionDialog.duration')} error={show('duration')}>
          {(p) => <input {...p} className="field num" inputMode="numeric" value={f.durationMin} onChange={(e) => set('durationMin', e.target.value)} placeholder="45" />}
        </Field>
      )}
      {showDistance && (
        <Field label={t('plan.sessionDialog.distance')} error={show('distance')}>
          {(p) => <input {...p} className="field num" inputMode="decimal" value={f.distanceKm} onChange={(e) => set('distanceKm', e.target.value)} placeholder="8,5" />}
        </Field>
      )}
      {isRun && (
        <>
          <Field label={t('plan.sessionDialog.paceFast')} error={show('paceMin')} hint={t('plan.sessionDialog.paceFastHint')}>
            {(p) => <input {...p} className="field num" inputMode="numeric" value={f.paceMin} onChange={(e) => set('paceMin', e.target.value)} placeholder="5:10" />}
          </Field>
          <Field label={t('plan.sessionDialog.paceSlow')} error={show('paceMax')}>
            {(p) => <input {...p} className="field num" inputMode="numeric" value={f.paceMax} onChange={(e) => set('paceMax', e.target.value)} placeholder="5:20" />}
          </Field>
          <Field label={t('plan.sessionDialog.cadence')} error={show('cadence')}>
            {(p) => <input {...p} className="field num" inputMode="numeric" value={f.cadence} onChange={(e) => set('cadence', e.target.value)} placeholder="172" />}
          </Field>
        </>
      )}
      <Field label={t('plan.sessionDialog.instructions')} className="sm:col-span-2" hint={t('plan.sessionDialog.instructionsHint')}>
        {(p) => (
          <textarea {...p} className="field min-h-24 resize-y" value={f.instructions} onChange={(e) => set('instructions', e.target.value)} maxLength={2000} placeholder={t('plan.sessionDialog.instructionsPlaceholder')} />
        )}
      </Field>
      {!isRest && (
        <fieldset className="sm:col-span-2">
          <legend className="mb-1.5 text-sm font-medium text-ink-2">{t('plan.sessionDialog.structure')}</legend>
          <p className="mb-2 text-xs text-ink-3">{f.steps.length ? stepsSummary(f.steps) : t('plan.sessionDialog.structureHint')}</p>
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
      title={session ? t('plan.sessionDialog.editTitle') : t('plan.sessionDialog.newTitle')}
      description={ed.date ? capitalize(fmtDateLong(ed.date)) : undefined}
      size="lg"
      initialFocus={session ? undefined : 'input[name="title"]'}
      footer={
        <div className="flex justify-end gap-2">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button type="submit" form={ed.formId} className="btn btn-primary" disabled={ed.pending}>
            {ed.pending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {session ? t('common.actions.save') : t('plan.sessionDialog.plan')}
          </button>
        </div>
      }
    >
      {ed.form}
    </Dialog>
  )
}
