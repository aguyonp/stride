import { Loader2 } from 'lucide-react'
import { useId, useState } from 'react'
import { useShareMutations } from '../../api/queries'
import type { ShareLink } from '../../api/types'
import { Dialog } from '../ui/Dialog'
import { Checkbox, ChipToggle } from '../ui/Field'
import { InlineError } from '../ui/Misc'
import { SegmentedControl } from '../ui/SegmentedControl'
import { DEFAULT_SHARE_INPUT, EXPIRY_CHOICES, formFromLink, PERIOD_CHOICES, SHARE_PERMISSIONS, SHARE_SECTIONS, toShareInput, validateShareForm, type ShareForm } from './shareScope'
import { t } from '../../i18n'

const presetLabels = () => (['physio', 'coach', 'doctor'] as const).map((k) => t(`share.dialog.presets.${k}`))
const expiryKey = (v: ShareForm['expiresInDays']) => (v === null ? 'none' : String(v))

/** Create or edit a share link: recipient, validity, period window, sections, anonymisation and what the recipient may write. */
export function ShareLinkDialog({ link, onClose, onCreated }: { link: ShareLink | null; onClose: () => void; onCreated: (l: ShareLink) => void }) {
  const ids = useId()
  const m = useShareMutations()
  const [form, setForm] = useState<ShareForm>(() => formFromLink(link))
  const [error, setError] = useState<string | null>(null)
  const set = <K extends keyof ShareForm>(k: K, v: ShareForm[K]) => setForm((f) => ({ ...f, [k]: v }))
  const busy = m.create.isPending || m.update.isPending
  const expiryOptions = [...(link ? [{ value: 'keep', label: t('share.dialog.unchanged') }] : []), ...EXPIRY_CHOICES.map((c) => ({ value: expiryKey(c.value), label: c.label }))]

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
      title={link ? t('share.dialog.editTitle') : t('share.dialog.newTitle')}
      description={t('share.dialog.description')}
      initialFocus={`#${CSS.escape(`${ids}-label`)}`}
      footer={
        <div className="flex justify-end gap-2 pb-2">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button type="button" className="btn btn-primary" onClick={submit} disabled={busy}>
            {busy && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {link ? t('common.actions.save') : t('share.dialog.createCopy')}
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
            {t('share.dialog.recipient')}
          </label>
          <input id={`${ids}-label`} className="field" value={form.label} maxLength={80} onChange={(e) => set('label', e.target.value)} />
          <div className="mt-2 flex flex-wrap gap-1.5">
            {presetLabels().map((p) => (
              <ChipToggle key={p} pressed={form.label === p} onChange={() => set('label', p)}>
                {p}
              </ChipToggle>
            ))}
          </div>
        </div>

        <fieldset>
          <legend className="mb-1.5 text-sm font-medium text-ink-2">{t('share.dialog.validity')}</legend>
          <SegmentedControl<string>
            ariaLabel={t('share.dialog.validity')}
            size="sm"
            value={expiryKey(form.expiresInDays)}
            onChange={(v) => set('expiresInDays', v === 'keep' ? 'keep' : v === 'none' ? null : (Number(v) as 1 | 7 | 30 | 90))}
            options={expiryOptions}
          />
        </fieldset>

        <fieldset>
          <legend className="mb-1.5 text-sm font-medium text-ink-2">{t('share.dialog.period')}</legend>
          <SegmentedControl<string>
            ariaLabel={t('share.dialog.period')}
            size="sm"
            value={String(form.periodWeeks)}
            onChange={(v) => set('periodWeeks', Number(v) as ShareForm['periodWeeks'])}
            options={PERIOD_CHOICES.map((c) => ({ value: String(c.value), label: c.label }))}
          />
        </fieldset>

        <fieldset>
          <legend className="mb-1.5 text-sm font-medium text-ink-2">{t('share.dialog.sections')}</legend>
          <div className="flex flex-wrap gap-1.5" role="group" aria-label={t('share.dialog.sections')}>
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

        <Checkbox checked={form.anonymize} onChange={(v) => set('anonymize', v)} label={t('share.dialog.anonymize')} description={t('share.dialog.anonymizeHint')} />

        <fieldset>
          <legend className="mb-1 text-sm font-medium text-ink-2">{t('share.dialog.canAlso')}</legend>
          <p className="mb-2 text-xs text-ink-3">{t('share.dialog.canAlsoHint')}</p>
          <div className="space-y-2">
            {SHARE_PERMISSIONS.map((p) => (
              <Checkbox
                key={p.key}
                checked={form.permissions[p.key]}
                onChange={(v) => set('permissions', { ...form.permissions, [p.key]: v })}
                label={p.label}
                description={p.description}
              />
            ))}
          </div>
        </fieldset>
        {error && <InlineError>{error}</InlineError>}
      </form>
    </Dialog>
  )
}
