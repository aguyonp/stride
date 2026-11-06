import { Footprints, Loader2, Save } from 'lucide-react'
import { useMemo, useState } from 'react'
import { usePresets, useProfile, useSaveNotes, useShoes } from '../../api/queries'
import type { ActivityNotes, ActivityNotesInput, FocusPreset } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtNumber } from '../../lib/format'
import { CONTEXT_TAGS, SHOE_TYPE_LABEL } from '../../lib/labels'
import { Checkbox, Field } from '../ui/Field'
import { InlineError } from '../ui/Misc'
import { useToast } from '../ui/Toast'
import { PainScale } from './PainScale'

const DEFAULT_LABELS = { during: 'Gêne pendant l’effort', cold: 'Gêne à froid (+2 h)', nextMorning: 'Gêne le lendemain matin' }

export function usePainPreset(): { preset: FocusPreset | null; labels: FocusPreset['painLabels']; painEnabled: boolean } {
  const profile = useProfile()
  const presets = usePresets()
  const preset = presets.data?.find((p) => p.focusArea === profile.data?.focusArea) ?? null
  return { preset, labels: preset?.painLabels ?? DEFAULT_LABELS, painEnabled: preset?.painEnabled ?? true }
}

function toInput(n: ActivityNotes): Required<ActivityNotesInput> {
  return { painDuring: n.painDuring, painCold: n.painCold, painNextMorning: n.painNextMorning, rpe: n.rpe, shoeId: n.shoeId, tags: n.tags, note: n.note }
}

interface FeelingFormProps {
  activityId: number
  notes: ActivityNotes
  onSaved?: () => void
  stickyActions?: boolean
}

/** Quick « Ressenti » entry: pains (0–10, step 0.5), shoe, context tags, RPE, note. */
export function FeelingForm({ activityId, notes, onSaved, stickyActions }: FeelingFormProps) {
  const { labels, painEnabled } = usePainPreset()
  const shoes = useShoes()
  const save = useSaveNotes(activityId)
  const { toast } = useToast()
  const initial = useMemo(() => toInput(notes), [notes])
  const [form, setForm] = useState(initial)
  const [showPain, setShowPain] = useState(painEnabled || notes.painDuring !== null)
  const [prevInitial, setPrevInitial] = useState(initial)
  if (prevInitial !== initial) {
    setPrevInitial(initial)
    setForm(initial)
  }
  const dirty = JSON.stringify(form) !== JSON.stringify(initial)
  const set = <K extends keyof typeof form>(k: K, v: (typeof form)[K]) => setForm((f) => ({ ...f, [k]: v }))
  const activeShoes = (shoes.data ?? []).filter((s) => !s.retired || s.id === form.shoeId)

  const submit = () =>
    save.mutate(form, {
      onSuccess: () => {
        toast({ title: 'Ressenti enregistré', description: 'Les prompts et l’historique sont mis à jour.' })
        onSaved?.()
      },
    })

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      {showPain ? (
        <div className="space-y-2.5">
          <PainScale label={labels.during} value={form.painDuring} onChange={(v) => set('painDuring', v)} />
          <PainScale label={labels.cold} value={form.painCold} onChange={(v) => set('painCold', v)} />
          <PainScale label={labels.nextMorning} value={form.painNextMorning} onChange={(v) => set('painNextMorning', v)} hint="À compléter le lendemain matin" />
        </div>
      ) : (
        <button type="button" className="btn btn-ghost w-full" onClick={() => setShowPain(true)}>
          Ajouter une gêne / douleur (optionnel)
        </button>
      )}

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

      {save.isError && <InlineError>{save.error.message}</InlineError>}

      <div className={cn('flex items-center gap-2', stickyActions && 'sticky -bottom-4 z-10 -mx-5 -mb-4 border-t border-line bg-surface px-5 pt-3 pb-4')}>
        {dirty && (
          <button type="button" className="btn btn-ghost" onClick={() => setForm(initial)}>
            Annuler
          </button>
        )}
        <button type="submit" className="btn btn-primary h-12 flex-1" disabled={!dirty || save.isPending}>
          {save.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Save className="size-4" aria-hidden="true" />}
          Enregistrer mon ressenti
        </button>
      </div>
    </form>
  )
}
