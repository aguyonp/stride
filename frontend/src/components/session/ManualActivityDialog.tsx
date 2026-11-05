import { Loader2, Trash2 } from 'lucide-react'
import { useId, useState, type FormEvent } from 'react'
import { useManualActivityMutations } from '../../api/queriesV31'
import type { ActivitySummary, ActivityView, ManualActivityInput, Sport } from '../../api/types'
import { capitalize, durationInput, fmtDateLong, isNum, parseDuration } from '../../lib/format'
import { SPORT_LABEL } from '../../lib/labels'
import { Dialog } from '../ui/Dialog'
import { Field } from '../ui/Field'
import { InlineError } from '../ui/Misc'
import { useToast } from '../ui/Toast'

type Existing = Pick<ActivityView['activity'], 'id' | 'date' | 'startLocal' | 'sport' | 'name' | 'distanceM' | 'durationS' | 'avgHR' | 'maxHR' | 'avgCadenceSpm' | 'elevGainM' | 'elevLossM'>

interface FormState {
  date: string
  time: string
  sport: Sport
  title: string
  km: string
  duration: string
  avgHR: string
  maxHR: string
  cadence: string
  gain: string
  loss: string
}

const str = (v: number | null | undefined, d = 0) => (isNum(v) ? String(d ? Math.round(v * 10 ** d) / 10 ** d : Math.round(v)).replace('.', ',') : '')

function toForm(a: Existing | null, today: string): FormState {
  return {
    date: a?.date ?? today,
    time: a?.startLocal.slice(11, 16) ?? '',
    sport: a?.sport ?? 'run',
    title: a?.name ?? '',
    km: isNum(a?.distanceM) ? str(a.distanceM / 1000, 2) : '',
    duration: durationInput(a?.durationS),
    avgHR: str(a?.avgHR),
    maxHR: str(a?.maxHR),
    cadence: str(a?.avgCadenceSpm),
    gain: str(a?.elevGainM),
    loss: str(a?.elevLossM),
  }
}

const num = (v: string) => {
  const t = v.trim()
  if (!t) return undefined
  const n = Number(t.replace(',', '.'))
  return Number.isFinite(n) && n >= 0 ? n : null
}

interface ManualActivityDialogProps {
  open: boolean
  onClose: () => void
  today: string
  /** Activity to edit; null to create one. */
  activity: Existing | null
  onCreated?: (a: ActivitySummary) => void
  onDeleted?: () => void
}

/** « Saisir une séance manuelle » (standalone mode): the Ressenti form follows the creation. */
export function ManualActivityDialog({ open, onClose, today, activity, onCreated, onDeleted }: ManualActivityDialogProps) {
  const formId = useId()
  const { toast } = useToast()
  const m = useManualActivityMutations()
  const [f, setF] = useState(() => toForm(activity, today))
  const [submitted, setSubmitted] = useState(false)
  const [confirm, setConfirm] = useState(false)
  const set = <K extends keyof FormState>(k: K, v: FormState[K]) => setF((s) => ({ ...s, [k]: v }))
  const durationS = parseDuration(f.duration)
  const vals = { km: num(f.km), avgHR: num(f.avgHR), maxHR: num(f.maxHR), cadence: num(f.cadence), gain: num(f.gain), loss: num(f.loss) }
  const errors = {
    date: !/^\d{4}-\d{2}-\d{2}$/.test(f.date) ? 'Date invalide.' : f.date > today ? 'La séance ne peut pas être dans le futur.' : null,
    duration: durationS === null || durationS < 60 ? 'Durée requise : h:mm:ss, mm:ss ou minutes.' : null,
    km: vals.km === null ? 'Distance en km.' : null,
    hr: vals.avgHR === null || vals.maxHR === null ? 'FC en bpm.' : vals.avgHR && vals.maxHR && vals.avgHR > vals.maxHR ? 'La FC moyenne dépasse la FC max.' : null,
    cadence: vals.cadence === null || (vals.cadence ?? 0) > 260 ? 'Cadence en pas/min.' : null,
    elev: vals.gain === null || vals.loss === null ? 'Dénivelé en mètres.' : null,
  }
  const show = (k: keyof typeof errors) => (submitted || k === 'hr' || k === 'km' ? errors[k] : null)
  const pending = m.create.isPending || m.update.isPending
  const error = m.create.error ?? m.update.error ?? m.remove.error
  const isRun = f.sport === 'run'
  const hasDistance = f.sport !== 'strength'

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    setSubmitted(true)
    if (Object.values(errors).some(Boolean) || durationS === null) return
    const opt = (v: number | null | undefined, k = 1) => (v === undefined || v === null ? undefined : Math.round(v * k))
    const body: ManualActivityInput = {
      date: f.date,
      startTime: f.time || undefined,
      sport: f.sport,
      title: f.title.trim() || undefined,
      durationS,
      distanceM: hasDistance ? opt(vals.km, 1000) : undefined,
      avgHR: opt(vals.avgHR),
      maxHR: opt(vals.maxHR),
      avgCadenceSpm: isRun ? opt(vals.cadence) : undefined,
      elevGainM: hasDistance ? opt(vals.gain) : undefined,
      elevLossM: hasDistance ? opt(vals.loss) : undefined,
    }
    if (activity)
      m.update.mutate(
        { id: activity.id, ...body },
        { onSuccess: () => (toast({ title: 'Séance mise à jour', description: capitalize(fmtDateLong(body.date)) }), onClose()) },
      )
    else
      m.create.mutate(body, {
        onSuccess: (a) => {
          toast({ title: 'Séance enregistrée', description: 'Ajoute ton ressenti pour enrichir les analyses.' })
          onClose()
          onCreated?.(a)
        },
      })
  }
  const remove = () =>
    activity &&
    m.remove.mutate(activity.id, {
      onSuccess: () => {
        toast({ title: 'Séance supprimée' })
        onClose()
        onDeleted?.()
      },
    })

  return (
    <Dialog
      open={open}
      onClose={onClose}
      size="lg"
      title={activity ? 'Modifier la séance manuelle' : 'Saisir une séance manuelle'}
      description="Seule la durée est obligatoire ; le reste affine les analyses et les prompts."
      initialFocus={activity ? undefined : 'input[name="title"]'}
      footer={
        <div className="flex flex-wrap items-center justify-between gap-2">
          {activity ? (
            confirm ? (
              <button type="button" className="btn btn-danger" onClick={remove} disabled={m.remove.isPending}>
                <Trash2 className="size-4" aria-hidden="true" /> Confirmer la suppression
              </button>
            ) : (
              <button type="button" className="btn btn-ghost text-danger" onClick={() => setConfirm(true)}>
                <Trash2 className="size-4" aria-hidden="true" /> Supprimer
              </button>
            )
          ) : (
            <span />
          )}
          <span className="flex gap-2">
            <button type="button" className="btn btn-ghost" onClick={onClose}>
              Annuler
            </button>
            <button type="submit" form={formId} className="btn btn-primary" disabled={pending}>
              {pending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
              {activity ? 'Enregistrer' : 'Enregistrer et noter mon ressenti'}
            </button>
          </span>
        </div>
      }
    >
      <form id={formId} onSubmit={onSubmit} noValidate className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <Field label="Date" className="col-span-1 sm:col-span-2" error={show('date')}>
          {(p) => <input {...p} type="date" className="field" value={f.date} max={today} onChange={(e) => set('date', e.target.value)} />}
        </Field>
        <Field label="Heure" className="col-span-1 sm:col-span-1">
          {(p) => <input {...p} type="time" className="field" value={f.time} onChange={(e) => set('time', e.target.value)} />}
        </Field>
        <Field label="Sport" className="col-span-2 sm:col-span-1">
          {(p) => (
            <select {...p} className="field" value={f.sport} onChange={(e) => set('sport', e.target.value as Sport)}>
              {(Object.keys(SPORT_LABEL) as Sport[]).map((s) => (
                <option key={s} value={s}>
                  {SPORT_LABEL[s]}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label="Titre" className="col-span-2 sm:col-span-4">
          {(p) => <input {...p} name="title" className="field" value={f.title} onChange={(e) => set('title', e.target.value)} maxLength={120} placeholder={isRun ? 'Footing du soir' : 'Séance'} autoComplete="off" />}
        </Field>
        {hasDistance && (
          <Field label="Distance (km)" className="col-span-1 sm:col-span-2" error={show('km')}>
            {(p) => <input {...p} className="field num" inputMode="decimal" value={f.km} onChange={(e) => set('km', e.target.value)} placeholder="8,4" />}
          </Field>
        )}
        <Field label="Durée" className={hasDistance ? 'col-span-1 sm:col-span-2' : 'col-span-2 sm:col-span-4'} error={show('duration')} hint={!show('duration') ? 'h:mm:ss, mm:ss ou minutes' : undefined}>
          {(p) => <input {...p} className="field num" inputMode="numeric" value={f.duration} onChange={(e) => set('duration', e.target.value)} placeholder="48:30" />}
        </Field>
        <Field label="FC moyenne" error={show('hr')}>
          {(p) => <input {...p} className="field num" inputMode="numeric" value={f.avgHR} onChange={(e) => set('avgHR', e.target.value)} placeholder="145" />}
        </Field>
        <Field label="FC max">{(p) => <input {...p} className="field num" inputMode="numeric" value={f.maxHR} onChange={(e) => set('maxHR', e.target.value)} placeholder="168" />}</Field>
        {isRun ? (
          <Field label="Cadence (spm)" error={show('cadence')}>
            {(p) => <input {...p} className="field num" inputMode="numeric" value={f.cadence} onChange={(e) => set('cadence', e.target.value)} placeholder="174" />}
          </Field>
        ) : (
          <span className="max-sm:hidden" />
        )}
        <span className="max-sm:hidden" />
        {hasDistance && (
          <>
            <Field label="D+ (m)" error={show('elev')}>
              {(p) => <input {...p} className="field num" inputMode="numeric" value={f.gain} onChange={(e) => set('gain', e.target.value)} placeholder="60" />}
            </Field>
            <Field label="D− (m)">{(p) => <input {...p} className="field num" inputMode="numeric" value={f.loss} onChange={(e) => set('loss', e.target.value)} placeholder="60" />}</Field>
          </>
        )}
        {error && <InlineError className="col-span-2 sm:col-span-4">{error.message}</InlineError>}
      </form>
    </Dialog>
  )
}
