import { Footprints, Loader2, Plus, Save, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useBodyMapMutations, useProfile, useSaveNotes, useShoes } from '../../api/queries'
import { useActivitySymptoms, useHealthStatus } from '../../api/queriesV41'
import type { ActivityNotes, ActivityNotesInput, BodySide, BodyZone } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtNumber, todayISO } from '../../lib/format'
import { CONTEXT_TAGS, SHOE_TYPE_LABEL } from '../../lib/labels'
import { ALL_BODY_ZONES, BODY_SIDE_LABEL, BODY_ZONE_META } from '../bodymap/bodyMapMeta'
import { Checkbox, Field } from '../ui/Field'
import { InlineError } from '../ui/Misc'
import { SegmentedControl } from '../ui/SegmentedControl'
import { useToast } from '../ui/Toast'
import { PainScale } from './PainScale'
import { diffSymptoms, hasChanges, initialSymptoms, newSymptom, type SymptomDraft } from './symptoms'

type NotesForm = Required<Pick<ActivityNotesInput, 'rpe' | 'shoeId' | 'tags' | 'note'>>

const toInput = (n: ActivityNotes): NotesForm => ({ rpe: n.rpe, shoeId: n.shoeId, tags: n.tags, note: n.note })
const SIDES: BodySide[] = ['left', 'right', 'both']

interface FeelingFormProps {
  activityId: number
  date: string
  notes: ActivityNotes
  onSaved?: () => void
  stickyActions?: boolean
}

function SymptomRow({ row, taken, onChange, onRemove }: { row: SymptomDraft; taken: BodyZone[]; onChange: (r: SymptomDraft) => void; onRemove: () => void }) {
  return (
    <li className="glass-inset space-y-2.5 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <select className="field h-9 min-h-9 w-auto flex-1 py-1 text-sm" aria-label="Zone" value={row.zone} onChange={(e) => onChange({ ...row, zone: e.target.value as BodyZone })}>
          {ALL_BODY_ZONES.map((z) => (
            <option key={z} value={z} disabled={z !== row.zone && taken.includes(z)}>
              {BODY_ZONE_META[z].short}
            </option>
          ))}
        </select>
        <SegmentedControl<BodySide> ariaLabel="Côté" size="sm" value={row.side} onChange={(side) => onChange({ ...row, side })} options={SIDES.map((v) => ({ value: v, label: BODY_SIDE_LABEL[v] }))} />
        <button type="button" className="icon-btn size-9" onClick={onRemove} aria-label={`Retirer ${BODY_ZONE_META[row.zone].short}`}>
          <X className="size-4" />
        </button>
      </div>
      <PainScale label="Pendant l’effort" value={row.during} onChange={(during) => onChange({ ...row, during })} />
      <PainScale label="Le lendemain matin (J+1)" value={row.nextMorning} onChange={(nextMorning) => onChange({ ...row, nextMorning })} hint="À compléter le lendemain" />
    </li>
  )
}

/** Quick « Ressenti »: symptoms per zone (saved as body entries linked to the session), RPE, shoe, context, note. */
export function FeelingForm({ activityId, date, notes, onSaved, stickyActions }: FeelingFormProps) {
  const profile = useProfile()
  const today = todayISO(profile.data?.timezone)
  const health = useHealthStatus()
  const symptoms = useActivitySymptoms(activityId, date, today)
  const bodyMut = useBodyMapMutations()
  const shoes = useShoes()
  const save = useSaveNotes(activityId)
  const { toast } = useToast()
  const initial = useMemo(() => toInput(notes), [notes])
  const baseRows = useMemo(() => initialSymptoms(symptoms.entries, health.data), [symptoms.entries, health.data])
  const [form, setForm] = useState(initial)
  const [rows, setRows] = useState(baseRows)
  const [prev, setPrev] = useState({ initial, baseRows })
  if (prev.initial !== initial || prev.baseRows !== baseRows) {
    setPrev({ initial, baseRows })
    if (prev.initial !== initial) setForm(initial)
    if (prev.baseRows !== baseRows) setRows(baseRows)
  }
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const changes = diffSymptoms(baseRows, rows, activityId, date)
  const dirty = JSON.stringify(form) !== JSON.stringify(initial) || hasChanges(changes)
  const set = <K extends keyof NotesForm>(k: K, v: NotesForm[K]) => setForm((f) => ({ ...f, [k]: v }))
  const activeShoes = (shoes.data ?? []).filter((s) => !s.retired || s.id === form.shoeId)
  const addRow = () => {
    const r = newSymptom(rows, health.data)
    if (r) setRows([...rows, r])
  }

  const submit = async () => {
    setBusy(true)
    setError(null)
    try {
      for (const id of changes.remove) await bodyMut.removeEntry.mutateAsync(id)
      for (const u of changes.update) await bodyMut.updateEntry.mutateAsync(u)
      for (const c of changes.create) await bodyMut.createEntry.mutateAsync(c)
      if (JSON.stringify(form) !== JSON.stringify(initial)) await save.mutateAsync(form)
      toast({ title: 'Ressenti enregistré' })
      onSaved?.()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Enregistrement impossible.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <fieldset>
        <legend className="mb-1.5 text-sm font-medium text-ink-2">Gênes & douleurs</legend>
        {rows.length > 0 && (
          <ul className="mb-2 space-y-2">
            {rows.map((r, i) => (
              <SymptomRow
                key={`${r.id ?? 'new'}-${i}`}
                row={r}
                taken={rows.map((x) => x.zone)}
                onChange={(next) => setRows(rows.map((x, j) => (j === i ? next : x)))}
                onRemove={() => setRows(rows.filter((_, j) => j !== i))}
              />
            ))}
          </ul>
        )}
        {rows.length < ALL_BODY_ZONES.length && (
          <button type="button" className="btn btn-ghost w-full" onClick={addRow} disabled={!symptoms.ready}>
            <Plus className="size-4" aria-hidden="true" /> Signaler une gêne
          </button>
        )}
      </fieldset>

      <fieldset>
        <legend className="mb-1.5 text-sm font-medium text-ink-2">Effort perçu (RPE)</legend>
        <div className="grid grid-cols-5 gap-1.5 sm:grid-cols-10" role="radiogroup" aria-label="Effort perçu de 1 à 10">
          {Array.from({ length: 10 }, (_, i) => i + 1).map((v) => {
            const on = form.rpe === v
            return (
              <button
                key={v}
                type="button"
                role="radio"
                aria-checked={on}
                onClick={() => set('rpe', on ? null : v)}
                className={cn(
                  'num h-11 rounded-xl border text-sm font-semibold transition-colors',
                  on ? 'border-transparent bg-accent text-on-accent' : 'border-line text-ink-2 hover:bg-track',
                )}
              >
                {v}
              </button>
            )
          })}
        </div>
        <p className="mt-1 text-xs text-ink-3">1 très facile · 5 soutenu · 10 maximal</p>
      </fieldset>

      <Field
        label="Chaussures"
        hint={activeShoes.length === 0 ? 'Ajoute tes paires dans Réglages › Chaussures.' : notes.shoeAuto && form.shoeId === notes.shoeId ? 'Attribuée automatiquement depuis l’équipement Strava : choisis-en une autre pour corriger.' : undefined}
      >
        {(p) => (
          <div className="relative">
            <Footprints className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-3" aria-hidden="true" />
            <select {...p} className="field pl-9" value={form.shoeId ?? ''} onChange={(e) => set('shoeId', e.target.value ? Number(e.target.value) : null)}>
              <option value="">Non renseignées</option>
              {activeShoes.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name} · {SHOE_TYPE_LABEL[s.type]}
                  {s.dropMm !== null ? ` · drop ${fmtNumber(s.dropMm)} mm` : ''}
                </option>
              ))}
            </select>
          </div>
        )}
      </Field>

      <fieldset>
        <legend className="mb-1.5 text-sm font-medium text-ink-2">Contexte</legend>
        <div className="grid gap-2 sm:grid-cols-2">
          {CONTEXT_TAGS.map((t) => (
            <Checkbox key={t.key} label={t.label} checked={form.tags.includes(t.key)} onChange={(on) => set('tags', on ? [...form.tags, t.key] : form.tags.filter((k) => k !== t.key))} />
          ))}
        </div>
      </fieldset>

      <Field label="Note libre">
        {(p) => <textarea {...p} className="field min-h-20 resize-y" value={form.note} maxLength={2000} placeholder="Sensations, terrain, chaleur…" onChange={(e) => set('note', e.target.value)} />}
      </Field>

      {error && <InlineError>{error}</InlineError>}

      <div className={cn('flex items-center gap-2', stickyActions && 'sticky -bottom-4 z-10 -mx-5 -mb-4 border-t border-line bg-surface px-5 pt-3 pb-4')}>
        {dirty && (
          <button type="button" className="btn btn-ghost" onClick={() => (setForm(initial), setRows(baseRows))}>
            Annuler
          </button>
        )}
        <button type="submit" className="btn btn-primary h-12 flex-1" disabled={!dirty || busy}>
          {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Save className="size-4" aria-hidden="true" />}
          Enregistrer mon ressenti
        </button>
      </div>
    </form>
  )
}
