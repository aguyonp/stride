import { Loader2 } from 'lucide-react'
import { useId, useState } from 'react'
import { useShareMutations } from '../../api/queries'
import type { ShareLink } from '../../api/types'
import { Dialog } from '../ui/Dialog'
import { Checkbox, ChipToggle } from '../ui/Field'
import { InlineError } from '../ui/Misc'
import { SegmentedControl } from '../ui/SegmentedControl'
import { DEFAULT_SHARE_INPUT, EXPIRY_CHOICES, formFromLink, PERIOD_CHOICES, SHARE_SECTIONS, toShareInput, validateShareForm, type ShareForm } from './shareScope'

const PRESET_LABELS = ['Kiné du sport', 'Coach club', 'Médecin du sport']
const expiryKey = (v: ShareForm['expiresInDays']) => (v === null ? 'none' : String(v))

/** Create or edit a read-only share link: recipient, validity, period window, sections and anonymisation. */
export function ShareLinkDialog({ link, onClose, onCreated }: { link: ShareLink | null; onClose: () => void; onCreated: (l: ShareLink) => void }) {
  const ids = useId()
  const m = useShareMutations()
  const [form, setForm] = useState<ShareForm>(() => formFromLink(link))
  const [error, setError] = useState<string | null>(null)
  const set = <K extends keyof ShareForm>(k: K, v: ShareForm[K]) => setForm((f) => ({ ...f, [k]: v }))
  const busy = m.create.isPending || m.update.isPending
  const expiryOptions = [...(link ? [{ value: 'keep', label: 'Inchangée' }] : []), ...EXPIRY_CHOICES.map((c) => ({ value: expiryKey(c.value), label: c.label }))]

  const submit = () => {
    const invalid = validateShareForm(form)
    setError(invalid)
    if (invalid) return
    const body = toShareInput(form)
    const onError = (e: Error) => setError(e.message)
    if (link) m.update.mutate({ id: link.id, ...body }, { onSuccess: onClose, onError })
    else m.create.mutate({ ...DEFAULT_SHARE_INPUT, ...body }, { onSuccess: onCreated, onError })
  }

  return (
    <Dialog
      open
      onClose={onClose}
      title={link ? 'Modifier le partage' : 'Nouveau lien de partage'}
      description="Accès en lecture seule, sans trace GPS ni identifiant."
      initialFocus={`#${CSS.escape(`${ids}-label`)}`}
      footer={
        <div className="flex justify-end gap-2 pb-2">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Annuler
          </button>
          <button type="button" className="btn btn-primary" onClick={submit} disabled={busy}>
            {busy && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {link ? 'Enregistrer' : 'Créer et copier'}
          </button>
        </div>
      }
    >
      <form
        className="space-y-5"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div>
          <label htmlFor={`${ids}-label`} className="mb-1.5 block text-sm font-medium text-ink-2">
            Destinataire
          </label>
          <input id={`${ids}-label`} className="field" value={form.label} maxLength={80} onChange={(e) => set('label', e.target.value)} />
          <div className="mt-2 flex flex-wrap gap-1.5">
            {PRESET_LABELS.map((p) => (
              <ChipToggle key={p} pressed={form.label === p} onChange={() => set('label', p)}>
                {p}
              </ChipToggle>
            ))}
          </div>
        </div>

        <fieldset>
          <legend className="mb-1.5 text-sm font-medium text-ink-2">Validité du lien</legend>
          <SegmentedControl<string>
            ariaLabel="Validité du lien"
            size="sm"
            value={expiryKey(form.expiresInDays)}
            onChange={(v) => set('expiresInDays', v === 'keep' ? 'keep' : v === 'none' ? null : (Number(v) as 1 | 7 | 30 | 90))}
            options={expiryOptions}
          />
        </fieldset>

        <fieldset>
          <legend className="mb-1.5 text-sm font-medium text-ink-2">Période visible</legend>
          <SegmentedControl<string>
            ariaLabel="Période visible"
            size="sm"
            value={String(form.periodWeeks)}
            onChange={(v) => set('periodWeeks', Number(v) as ShareForm['periodWeeks'])}
            options={PERIOD_CHOICES.map((c) => ({ value: String(c.value), label: c.label }))}
          />
        </fieldset>

        <fieldset>
          <legend className="mb-1.5 text-sm font-medium text-ink-2">Rubriques partagées</legend>
          <div className="flex flex-wrap gap-1.5" role="group" aria-label="Rubriques partagées">
            {SHARE_SECTIONS.map((sec) => (
              <ChipToggle
                key={sec.key}
                pressed={form.sections.includes(sec.key)}
                onChange={(on) => set('sections', on ? [...form.sections, sec.key] : form.sections.filter((k) => k !== sec.key))}
              >
                {sec.label}
              </ChipToggle>
            ))}
          </div>
        </fieldset>

        <Checkbox checked={form.anonymize} onChange={(v) => set('anonymize', v)} label="Anonymiser" description="Ton nom est remplacé par « Athlète »." />
        {error && <InlineError>{error}</InlineError>}
      </form>
    </Dialog>
  )
}
