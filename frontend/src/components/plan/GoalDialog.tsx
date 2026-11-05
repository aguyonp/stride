import { Loader2 } from 'lucide-react'
import { useEffect, useId, useState, type FormEvent } from 'react'
import { useGoalMutations } from '../../api/queries'
import type { Goal, GoalInput, GoalKind } from '../../api/types'
import { isNum } from '../../lib/format'
import { GOAL_KIND_LABEL } from '../../lib/labels'
import { Dialog } from '../ui/Dialog'
import { Field } from '../ui/Field'
import { InlineError } from '../ui/Misc'
import { SegmentedControl } from '../ui/SegmentedControl'
import { useToast } from '../ui/Toast'

interface GoalDialogProps {
  open: boolean
  onClose: () => void
  /** Goal to edit; null to create one. */
  goal: Goal | null
}

const KINDS = Object.keys(GOAL_KIND_LABEL) as GoalKind[]
const DEFAULT_UNIT: Partial<Record<GoalKind, string>> = { cadence: 'spm', weekly_volume: 'km/sem', race: 'km' }
const PLACEHOLDER: Record<GoalKind, string> = {
  macro: 'Courir 45 min en continu sans gêne mi-novembre',
  race: 'Semi-marathon de printemps en 1h35',
  cadence: 'Cadence moyenne ≥ 172 spm en footing',
  weekly_volume: 'Remonter à 35 km par semaine',
  other: 'Renforcement 2 fois par semaine',
}

export function GoalDialog({ open, onClose, goal }: GoalDialogProps) {
  const formId = useId()
  const { toast } = useToast()
  const { create, update } = useGoalMutations()
  const [title, setTitle] = useState('')
  const [kind, setKind] = useState<GoalKind>('macro')
  const [value, setValue] = useState('')
  const [unit, setUnit] = useState('')
  const [date, setDate] = useState('')
  const [status, setStatus] = useState<Goal['status']>('active')
  const [note, setNote] = useState('')
  const [submitted, setSubmitted] = useState(false)
  const openKey = open ? String(goal?.id ?? 'new') : null
  const [lastKey, setLastKey] = useState<string | null>(null)
  if (openKey !== lastKey) {
    setLastKey(openKey)
    if (openKey) {
      setTitle(goal?.title ?? '')
      setKind(goal?.kind ?? 'macro')
      setValue(isNum(goal?.targetValue) ? String(goal.targetValue).replace('.', ',') : '')
      setUnit(goal?.targetUnit ?? '')
      setDate(goal?.targetDate ?? '')
      setStatus(goal?.status ?? 'active')
      setNote(goal?.note ?? '')
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

  const parsedValue = value.trim() === '' ? null : Number(value.replace(',', '.'))
  const errors = {
    title: !title.trim() ? 'Décris l’objectif en une phrase.' : null,
    value: parsedValue !== null && !Number.isFinite(parsedValue) ? 'Valeur numérique attendue.' : null,
  }
  const pending = create.isPending || update.isPending
  const mutationError = create.error ?? update.error

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    setSubmitted(true)
    if (errors.title || errors.value) return
    const body: GoalInput = {
      title: title.trim(),
      kind,
      targetValue: parsedValue,
      targetUnit: unit.trim() || null,
      targetDate: date || null,
      status,
      note: note.trim(),
    }
    const done = () => {
      toast({ title: goal ? 'Objectif mis à jour' : 'Objectif ajouté', description: body.title })
      onClose()
    }
    if (goal) update.mutate({ id: goal.id, ...body }, { onSuccess: done })
    else create.mutate(body, { onSuccess: done })
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={goal ? 'Modifier l’objectif' : 'Nouvel objectif'}
      description="Les objectifs actifs sont repris dans le contexte du prompt IA."
      size="lg"
      initialFocus={goal ? undefined : 'input[name="goal-title"]'}
      footer={
        <div className="flex justify-end gap-2">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Annuler
          </button>
          <button type="submit" form={formId} className="btn btn-primary" disabled={pending}>
            {pending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {goal ? 'Enregistrer' : 'Ajouter'}
          </button>
        </div>
      }
    >
      <form id={formId} onSubmit={onSubmit} noValidate className="grid gap-4 sm:grid-cols-2">
        <Field label="Objectif" className="sm:col-span-2" error={submitted ? errors.title : null}>
          {(p) => <input {...p} name="goal-title" className="field" value={title} onChange={(e) => setTitle(e.target.value)} placeholder={PLACEHOLDER[kind]} maxLength={160} autoComplete="off" />}
        </Field>
        <Field label="Type">
          {(p) => (
            <select
              {...p}
              className="field"
              value={kind}
              onChange={(e) => {
                const k = e.target.value as GoalKind
                setKind(k)
                if (!unit.trim() && DEFAULT_UNIT[k]) setUnit(DEFAULT_UNIT[k] as string)
              }}
            >
              {KINDS.map((k) => (
                <option key={k} value={k}>
                  {GOAL_KIND_LABEL[k]}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label="Échéance" hint="Facultatif">
          {(p) => <input {...p} type="date" className="field" value={date} onChange={(e) => setDate(e.target.value)} />}
        </Field>
        <Field label="Valeur cible" error={errors.value} hint="Facultatif">
          {(p) => <input {...p} className="field num" inputMode="decimal" value={value} onChange={(e) => setValue(e.target.value)} placeholder={kind === 'cadence' ? '172' : kind === 'weekly_volume' ? '35' : '21,1'} />}
        </Field>
        <Field label="Unité">
          {(p) => (
            <>
              <input {...p} className="field" list={`${formId}-units`} value={unit} onChange={(e) => setUnit(e.target.value)} placeholder={DEFAULT_UNIT[kind] ?? 'km'} maxLength={20} />
              <datalist id={`${formId}-units`}>
                {['km', 'km/sem', 'spm', 'min', 'h', 'séances/sem'].map((u) => (
                  <option key={u} value={u} />
                ))}
              </datalist>
            </>
          )}
        </Field>
        <div className="sm:col-span-2">
          <p className="mb-1.5 text-sm font-medium text-ink-2">Statut</p>
          <SegmentedControl<Goal['status']>
            ariaLabel="Statut de l’objectif"
            value={status}
            onChange={setStatus}
            options={[
              { value: 'active', label: 'Actif' },
              { value: 'done', label: 'Atteint' },
              { value: 'archived', label: 'Archivé' },
            ]}
          />
        </div>
        <Field label="Note" className="sm:col-span-2" hint="Contexte utile pour le coach IA (facultatif)">
          {(p) => <textarea {...p} className="field min-h-20 resize-y" value={note} onChange={(e) => setNote(e.target.value)} maxLength={1000} />}
        </Field>
        {mutationError && <InlineError className="sm:col-span-2">{mutationError.message}</InlineError>}
      </form>
    </Dialog>
  )
}
