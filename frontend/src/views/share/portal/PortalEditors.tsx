import { Loader2, Trash2 } from 'lucide-react'
import { useCallback, useId, useState, type ReactNode } from 'react'
import { usePublicShareMutations } from '../../../api/queriesV42'
import type { CalendarEvent, DirectiveRestrictions, EventKind, EventSeverity, PlannedSession, PlannedSessionInput, PublicActivityItem, ShareNote } from '../../../api/types'
import { EVENT_ICON, EVENT_TONE } from '../../../components/plan/EventDialog'
import { useSessionEditor } from '../../../components/plan/PlannedSessionDialog'
import { AUTHOR_ROLE_LABEL, AUTHOR_ROLES, NO_RESTRICTIONS } from '../../../components/share/shareNotes'
import { Dialog } from '../../../components/ui/Dialog'
import { Checkbox, Field } from '../../../components/ui/Field'
import { InlineError } from '../../../components/ui/Misc'
import { SegmentedControl } from '../../../components/ui/SegmentedControl'
import { useToast } from '../../../components/ui/Toast'
import { cn } from '../../../lib/cn'
import { addDays, capitalize, fmtDateLong, fmtDateShort } from '../../../lib/format'
import { EVENT_KIND, SEVERITY_LABEL } from '../../../lib/labels'
import { useAuthor, type Author } from './portalUtils'
import { t } from '../../../i18n'

/** Desktop: the form opens in place. Mobile: a bottom sheet whose save button stays under the thumb. */
export function EditorShell({ inline, open, title, description, onClose, footer, children }: { inline: boolean; open: boolean; title: string; description?: string; onClose: () => void; footer: ReactNode; children: ReactNode }) {
  if (inline) {
    if (!open) return null
    return (
      <section className="glass-inset animate-in max-w-3xl p-4" aria-label={title}>
        <h3 className="text-sm font-semibold text-ink">{title}</h3>
        {description && <p className="mt-0.5 text-xs text-ink-3">{description}</p>}
        <div className="mt-3">{children}</div>
        <div className="mt-4 flex flex-wrap items-center justify-end gap-2">{footer}</div>
      </section>
    )
  }
  return (
    <Dialog open={open} onClose={onClose} title={title} description={description} footer={<div className="flex flex-wrap items-center justify-end gap-2 pb-2">{footer}</div>}>
      {children}
    </Dialog>
  )
}

/** Signature remembered on the device: shown as one line once known. */
function AuthorFields({ author, setAuthor }: { author: Author; setAuthor: (a: Author) => void }) {
  const ids = useId()
  const [editing, setEditing] = useState(!author.name)
  if (!editing)
    return (
      <p className="text-xs text-ink-3">
        {t('share.portal.signed', { name: author.name, role: AUTHOR_ROLE_LABEL[author.role] })}{' '}
        <button type="button" className="font-medium text-accent underline-offset-2 hover:underline" onClick={() => setEditing(true)}>
          {t('share.portal.editSignature')}
        </button>
      </p>
    )
  return (
    <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
      <div>
        <label htmlFor={`${ids}-name`} className="mb-1.5 block text-sm font-medium text-ink-2">
          {t('share.portal.yourName')}
        </label>
        <input id={`${ids}-name`} className="field" value={author.name} maxLength={80} autoComplete="name" onChange={(e) => setAuthor({ ...author, name: e.target.value })} placeholder={t('share.portal.namePlaceholder')} />
      </div>
      <SegmentedControl ariaLabel={t('share.portal.yourRole')} size="sm" value={author.role} onChange={(role) => setAuthor({ ...author, role })} options={AUTHOR_ROLES.map((r) => ({ value: r, label: AUTHOR_ROLE_LABEL[r] }))} />
    </div>
  )
}

const signature = (a: Author) => ({ authorName: a.name.trim() || null, authorRole: a.role })

function DeleteButton({ onConfirm, pending, label }: { onConfirm: () => void; pending: boolean; label: string }) {
  const [armed, setArmed] = useState(false)
  return armed ? (
    <button type="button" className="btn btn-danger mr-auto" onClick={onConfirm} disabled={pending}>
      {pending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Trash2 className="size-4" aria-hidden="true" />}
      {t('share.portal.confirmDelete')}
    </button>
  ) : (
    <button type="button" className="btn btn-ghost mr-auto text-danger" onClick={() => setArmed(true)}>
      <Trash2 className="size-4" aria-hidden="true" /> {label}
    </button>
  )
}

const num = (v: string) => {
  const n = Number(v.replace(',', '.').trim())
  return v.trim() !== '' && Number.isFinite(n) && n > 0 ? Math.round(n) : null
}

/* ---------------------------------- Directive ---------------------------------- */

export function DirectiveEditor({ token, directive, inline, open, onClose, today }: { token: string; directive: ShareNote | null; inline: boolean; open: boolean; onClose: () => void; today: string }) {
  const ids = useId()
  const m = usePublicShareMutations(token)
  const [author, setAuthor] = useAuthor()
  const { toast } = useToast()
  const r = directive?.restrictions ?? NO_RESTRICTIONS
  const [body, setBody] = useState(directive?.body ?? '')
  const [until, setUntil] = useState(directive?.validUntil ?? '')
  const [noIntervals, setNoIntervals] = useState(r.noIntervals)
  const [intensity, setIntensity] = useState<'free' | 'moderate' | 'easy'>(r.maxIntensity ?? 'free')
  const [elev, setElev] = useState(r.maxElevLossM != null ? String(r.maxElevLossM) : '')
  const [cadence, setCadence] = useState(r.cadenceTargetSpm != null ? String(r.cadenceTargetSpm) : '')
  const text = body.trim()
  const cadenceN = num(cadence)
  const invalid = text.length === 0 || text.length > 2000 || (cadence.trim() !== '' && (cadenceN === null || cadenceN < 120 || cadenceN > 220)) || (elev.trim() !== '' && num(elev) === null)

  const save = () => {
    const restrictions: DirectiveRestrictions = { noIntervals, maxIntensity: intensity === 'free' ? null : intensity, maxElevLossM: num(elev), cadenceTargetSpm: cadenceN }
    m.putDirective.mutate(
      { ...signature(author), body: text, validUntil: until || null, restrictions },
      {
        onSuccess: () => {
          toast({ title: t('share.portal.directiveSaved'), description: t('share.portal.directiveSavedHint') })
          onClose()
        },
      },
    )
  }
  const remove = () => m.deleteDirective.mutate(undefined, { onSuccess: () => (toast({ title: t('share.portal.directiveRemoved') }), onClose()) })
  const error = m.putDirective.error ?? m.deleteDirective.error

  return (
    <EditorShell
      inline={inline}
      open={open}
      onClose={onClose}
      title={directive ? t('share.portal.editDirective') : t('share.portal.newDirective')}
      description={t('share.portal.directiveHint')}
      footer={
        <>
          {directive && <DeleteButton label={t('share.portal.remove')} onConfirm={remove} pending={m.deleteDirective.isPending} />}
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button type="button" className="btn btn-primary" onClick={save} disabled={invalid || m.putDirective.isPending}>
            {m.putDirective.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {t('common.actions.save')}
          </button>
        </>
      }
    >
      <div className="space-y-4">
        <div>
          <label htmlFor={`${ids}-body`} className="mb-1.5 block text-sm font-medium text-ink-2">
            {t('share.portalExtra.instruction')}
          </label>
          <textarea id={`${ids}-body`} className="field min-h-28 resize-y" value={body} onChange={(e) => setBody(e.target.value)} maxLength={2000} placeholder={t('share.portal.directivePlaceholder')} autoFocus />
          <p className="mt-1 text-right text-[11px] text-ink-3 num">{text.length} / 2000</p>
        </div>
        <div className="grid gap-3 sm:grid-cols-3">
          <Field label={t('share.portal.validUntil')} hint={t('share.portal.validUntilHint')}>
            {(p) => <input {...p} type="date" className="field" min={today} value={until} onChange={(e) => setUntil(e.target.value)} />}
          </Field>
          <Field label={t('share.portal.maxLoss')}>{(p) => <input {...p} className="field num" inputMode="numeric" value={elev} onChange={(e) => setElev(e.target.value)} placeholder="150" />}</Field>
          <Field label={t('share.portal.cadence')} error={cadence.trim() && (cadenceN === null || cadenceN < 120 || cadenceN > 220) ? t('share.portal.cadenceRange') : undefined}>
            {(p) => <input {...p} className="field num" inputMode="numeric" value={cadence} onChange={(e) => setCadence(e.target.value)} placeholder="176" />}
          </Field>
        </div>
        <div className="grid gap-3 sm:grid-cols-2 sm:items-center">
          <Checkbox checked={noIntervals} onChange={setNoIntervals} label={t('share.portal.noIntervals')} description={t('share.portal.noIntervalsHint')} />
          <fieldset>
            <legend className="mb-1.5 text-sm font-medium text-ink-2">{t('share.portal.maxIntensity')}</legend>
            <SegmentedControl<'free' | 'moderate' | 'easy'>
              ariaLabel={t('share.portal.maxIntensity')}
              size="sm"
              value={intensity}
              onChange={setIntensity}
              options={[
                { value: 'free', label: t('share.portal.free') },
                { value: 'moderate', label: t('share.portal.moderate') },
                { value: 'easy', label: t('share.portal.easy') },
              ]}
            />
          </fieldset>
        </div>
        <AuthorFields author={author} setAuthor={setAuthor} />
        {error && <InlineError>{error.message}</InlineError>}
      </div>
    </EditorShell>
  )
}

/* ---------------------------------- Remark ---------------------------------- */

export function RemarkEditor({ token, activity, inline, open, onClose }: { token: string; activity: Pick<PublicActivityItem, 'id' | 'name' | 'date' | 'remark'>; inline: boolean; open: boolean; onClose: () => void }) {
  const ids = useId()
  const m = usePublicShareMutations(token)
  const [author, setAuthor] = useAuthor()
  const { toast } = useToast()
  const [body, setBody] = useState(activity.remark?.body ?? '')
  const text = body.trim()
  const save = () =>
    m.putRemark.mutate(
      { activityId: activity.id, ...signature(author), body: text },
      {
        onSuccess: () => {
          toast({ title: t('share.portal.remarkSaved'), description: activity.name })
          onClose()
        },
      },
    )
  const remove = () => m.deleteRemark.mutate(activity.id, { onSuccess: () => (toast({ title: t('share.portal.remarkDeleted') }), onClose()) })
  const error = m.putRemark.error ?? m.deleteRemark.error
  return (
    <EditorShell
      inline={inline}
      open={open}
      onClose={onClose}
      title={activity.remark ? t('share.portal.editRemark') : t('share.portal.newRemark')}
      description={`${activity.name} · ${fmtDateShort(activity.date)}`}
      footer={
        <>
          {activity.remark && <DeleteButton label={t('common.actions.delete')} onConfirm={remove} pending={m.deleteRemark.isPending} />}
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button type="button" className="btn btn-primary" onClick={save} disabled={text.length === 0 || text.length > 1000 || m.putRemark.isPending}>
            {m.putRemark.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {t('common.actions.save')}
          </button>
        </>
      }
    >
      <div className="space-y-3">
        <label htmlFor={`${ids}-remark`} className="sr-only">
          {t('share.portal.remark')}
        </label>
        <textarea id={`${ids}-remark`} className="field min-h-24 resize-y" value={body} onChange={(e) => setBody(e.target.value)} maxLength={1000} placeholder={t('share.portal.remarkPlaceholder')} autoFocus />
        <AuthorFields author={author} setAuthor={setAuthor} />
        {error && <InlineError>{error.message}</InlineError>}
      </div>
    </EditorShell>
  )
}

/* ---------------------------------- Prescription ---------------------------------- */

export type PrescriptionTarget = { kind: 'new'; date: string } | { kind: 'session'; s: PlannedSession } | { kind: 'event'; e: CalendarEvent }

const KINDS = Object.keys(EVENT_KIND) as EventKind[]

function ConstraintForm({ formId, event, date, today, onSaved, token }: { formId: string; event: CalendarEvent | null; date: string; today: string; onSaved: () => void; token: string }) {
  const m = usePublicShareMutations(token)
  const { toast } = useToast()
  const [kind, setKind] = useState<EventKind>(event?.kind ?? 'pain')
  const [start, setStart] = useState(event?.startDate ?? date)
  const [end, setEnd] = useState(event?.endDate ?? date)
  const [severity, setSeverity] = useState<EventSeverity>(event?.severity ?? 2)
  const [note, setNote] = useState(event?.note ?? '')
  const min = addDays(today, -7)
  const max = addDays(today, 120)
  const endBefore = end < start
  const outOfRange = start < min || end > max
  const error = m.createEvent.error ?? m.updateEvent.error
  return (
    <form
      id={formId}
      className="space-y-4"
      noValidate
      onSubmit={(e) => {
        e.preventDefault()
        if (endBefore || outOfRange) return
        const body = { kind, startDate: start, endDate: end, severity, note: note.trim() }
        const onSuccess = () => {
          toast({ title: event ? t('share.portal.eventUpdated') : t('share.portal.eventAdded'), description: `${EVENT_KIND[kind].label} · ${capitalize(fmtDateLong(start))}` })
          onSaved()
        }
        if (event) m.updateEvent.mutate({ id: event.id, ...body }, { onSuccess })
        else m.createEvent.mutate(body, { onSuccess })
      }}
    >
      <fieldset>
        <legend className="mb-2 text-sm font-medium text-ink-2">{t('share.portal.type')}</legend>
          <div className="grid gap-2 sm:grid-cols-2" role="radiogroup" aria-label={t('share.portal.eventType')}>
          {KINDS.map((k) => {
            const Icon = EVENT_ICON[k]
            const on = kind === k
            return (
              <button key={k} type="button" role="radio" aria-checked={on} onClick={() => setKind(k)} className={cn('flex min-h-12 items-center gap-3 rounded-2xl border px-3 py-2 text-left transition-colors', on ? 'border-accent/50 bg-accent/10' : 'border-line hover:bg-track/60')}>
                <span className={cn('grid size-8 flex-none place-items-center rounded-xl', EVENT_TONE[k])}>
                  <Icon className="size-4" aria-hidden="true" />
                </span>
                <span className="min-w-0 text-sm font-semibold text-ink">{EVENT_KIND[k].label}</span>
              </button>
            )
          })}
        </div>
      </fieldset>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t('share.portal.from')} error={outOfRange ? t('share.portal.outOfRange') : undefined}>
          {(p) => <input {...p} type="date" className="field" min={min} max={max} value={start} onChange={(e) => (setStart(e.target.value), end < e.target.value && setEnd(e.target.value))} />}
        </Field>
        <Field label={t('share.portal.to')} error={endBefore ? t('share.portal.endBefore') : undefined}>
          {(p) => <input {...p} type="date" className="field" min={start} max={max} value={end} onChange={(e) => setEnd(e.target.value)} />}
        </Field>
      </div>
      <fieldset>
        <legend className="mb-1.5 text-sm font-medium text-ink-2">{t('share.portal.intensity')}</legend>
          <SegmentedControl<string> ariaLabel={t('share.portal.intensity')} size="sm" value={String(severity)} onChange={(v) => setSeverity(Number(v) as EventSeverity)} options={[1, 2, 3].map((n) => ({ value: String(n), label: SEVERITY_LABEL[n as EventSeverity] }))} />
      </fieldset>
      <Field label={t('share.portal.detail')}>
        {(p) => <input {...p} className="field" value={note} onChange={(e) => setNote(e.target.value)} maxLength={500} placeholder={t('share.portal.detailPlaceholder')} />}
      </Field>
      {error && <InlineError>{error.message}</InlineError>}
    </form>
  )
}

/** Quick prescription from the portal: a planned session (steps, targets, cadence, notes) or a constraint event. */
export function PrescriptionEditor({ token, target, inline, onClose, today, linkLabel }: { token: string; target: PrescriptionTarget; inline: boolean; onClose: () => void; today: string; linkLabel: string }) {
  const m = usePublicShareMutations(token)
  const constraintFormId = useId()
  const [tab, setTab] = useState<'session' | 'event'>(target.kind === 'event' ? 'event' : 'session')
  const session = target.kind === 'session' ? target.s : null
  const event = target.kind === 'event' ? target.e : null
  const date = target.kind === 'new' ? target.date : target.kind === 'session' ? target.s.date : target.e.startDate
  const resetCreate = m.createPlanned.reset
  const resetUpdate = m.updatePlanned.reset
  const reset = useCallback(() => {
    resetCreate()
    resetUpdate()
  }, [resetCreate, resetUpdate])
  const ed = useSessionEditor(session, date, tab === 'session', onClose, {
    minDate: addDays(today, -7),
    maxDate: addDays(today, 120),
    save: {
      create: (body: PlannedSessionInput, ok) => m.createPlanned.mutate(body, { onSuccess: ok }),
      update: (id, body, ok) => m.updatePlanned.mutate({ id, ...body }, { onSuccess: ok }),
      pending: m.createPlanned.isPending || m.updatePlanned.isPending,
      error: m.createPlanned.error ?? m.updatePlanned.error,
      reset,
    },
  })
  const pending = tab === 'session' ? ed.pending : m.createEvent.isPending || m.updateEvent.isPending
  const removing = m.deletePlanned.isPending || m.deleteEvent.isPending
  const remove = () => {
    if (session) m.deletePlanned.mutate(session.id, { onSuccess: onClose })
    else if (event) m.deleteEvent.mutate(event.id, { onSuccess: onClose })
  }
  const title = session ? t('share.portal.editPrescribed') : event ? t('share.portal.editEvent') : t('share.portal.prescribe')
  return (
    <EditorShell
      inline={inline}
      open
      onClose={onClose}
      title={title}
      description={t('share.portal.prescribeHint', { label: linkLabel })}
      footer={
        <>
          {(session || event) && <DeleteButton label={t('common.actions.delete')} onConfirm={remove} pending={removing} />}
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button type="submit" form={tab === 'session' ? ed.formId : constraintFormId} className="btn btn-primary" disabled={pending}>
            {pending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {session || event ? t('common.actions.save') : tab === 'session' ? t('share.portal.prescribeSession') : t('share.portal.addEvent')}
          </button>
        </>
      }
    >
      <div className="space-y-4">
        {target.kind === 'new' && (
          <SegmentedControl<'session' | 'event'>
            ariaLabel={t('share.portal.prescriptionType')}
            value={tab}
            onChange={setTab}
            options={[
              { value: 'session', label: t('share.portal.session') },
              { value: 'event', label: t('share.portal.event') },
            ]}
          />
        )}
        {tab === 'session' ? ed.form : <ConstraintForm formId={constraintFormId} event={event} date={date} today={today} onSaved={onClose} token={token} />}
        {(m.deletePlanned.error ?? m.deleteEvent.error) && <InlineError>{(m.deletePlanned.error ?? m.deleteEvent.error)?.message}</InlineError>}
      </div>
    </EditorShell>
  )
}
