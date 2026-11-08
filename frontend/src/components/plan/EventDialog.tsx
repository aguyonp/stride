import { useMutation } from '@tanstack/react-query'
import { ArrowLeft, Bandage, BatteryLow, BedDouble, Bot, CalendarClock, Loader2, Plane, Sparkles, Thermometer, Trash2, UserRound, Wand2 } from 'lucide-react'
import { useEffect, useState, type ComponentType } from 'react'
import { api } from '../../api/client'
import { useLlmConfig } from '../../api/queriesV3'
import { useEventMutations, usePlanRange, usePlanV31 } from '../../api/queriesV31'
import type { CalendarEvent, EventKind, EventSeverity, PlanAdaptPreview, PlanAdaptRequest, PlannedSession } from '../../api/types'
import { cn } from '../../lib/cn'
import { addDays, capitalize, fmtDateLong, fmtDateShort } from '../../lib/format'
import { EVENT_KIND, PLAN_SPORT_LABEL, SEVERITY_LABEL } from '../../lib/labels'
import { clearLlmStream, isJobStatus, startLlmStream, useLlmStream } from '../../state/events'
import { Dialog } from '../ui/Dialog'
import { Field } from '../ui/Field'
import { InlineError } from '../ui/Misc'
import { SegmentedControl } from '../ui/SegmentedControl'
import { useToast } from '../ui/Toast'
import { adaptDiff, changeCounts, type ChangeKind } from './adaptDiff'
import { prescribedByLine } from '../share/shareNotes'
import { PrescribedByTag } from '../share/ShareMarkers'
import { lazyRecord, t } from '../../i18n'

export const EVENT_ICON: Record<EventKind, ComponentType<{ className?: string }>> = { fatigue: BatteryLow, pain: Bandage, illness: Thermometer, unavailable: Plane, forced_rest: BedDouble }
const KINDS = Object.keys(EVENT_KIND) as EventKind[]
export const EVENT_TONE: Record<EventKind, string> = { fatigue: 'text-warn bg-warn/10', pain: 'text-danger bg-danger/10', illness: 'text-danger bg-danger/10', unavailable: 'text-info bg-info/10', forced_rest: 'text-violet bg-violet/10' }

/** Discreet event marker on a calendar day. */
export function EventChip({ e, onOpen, compact }: { e: CalendarEvent; onOpen?: (e: CalendarEvent) => void; compact?: boolean }) {
  const Icon = EVENT_ICON[e.kind]
  const label = `${EVENT_KIND[e.kind].label}, ${SEVERITY_LABEL[e.severity].toLowerCase()}${e.note ? ` : ${e.note}` : ''}${e.prescribedBy ? ` · ${prescribedByLine(e.prescribedBy)}` : ''}`
  return (
    <button type="button" onClick={() => onOpen?.(e)} title={label} aria-label={t('plan.event.chipAria', { label })} className={cn('inline-flex max-w-full min-w-0 items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-semibold', EVENT_TONE[e.kind])}>
      <Icon className="size-3 flex-none" aria-hidden="true" />
      {!compact && <span className="truncate">{EVENT_KIND[e.kind].short}</span>}
      {e.prescribedBy && <UserRound className="size-3 flex-none" aria-hidden="true" />}
      <span className="flex gap-px" aria-hidden="true">
        {[1, 2, 3].map((i) => (
          <span key={i} className={cn('size-1 rounded-full bg-current', i > e.severity && 'opacity-25')} />
        ))}
      </span>
    </button>
  )
}

const CHANGE_CLS: Record<ChangeKind, string> = { moved: 'bg-info/12 text-info', modified: 'bg-warn/12 text-warn', removed: 'bg-danger/12 text-danger', created: 'bg-ok/12 text-ok' }
const CHANGE_META: Record<ChangeKind, { label: string; cls: string }> = lazyRecord(['moved', 'modified', 'removed', 'created'] as const, (k) => ({ label: t(`plan.event.change.${k}`), cls: CHANGE_CLS[k] }))

function Preview({ preview, current }: { preview: PlanAdaptPreview; current: PlannedSession[] }) {
  const changes = adaptDiff(current, preview)
  const counts = changeCounts(changes)
  return (
    <div className="space-y-4">
      <p className="text-sm text-ink-2">{preview.summary}</p>
      {changes.length === 0 ? (
        <p className="rounded-2xl bg-track/50 p-4 text-sm text-ink-3">{t('plan.event.noChange')}</p>
      ) : (
        <>
          <ul className="flex flex-wrap gap-1.5 text-xs" aria-label={t('plan.event.summary')}>
            {(Object.keys(counts) as ChangeKind[])
              .filter((k) => counts[k] > 0)
              .map((k) => (
                <li key={k} className={cn('chip h-6', CHANGE_META[k].cls)}>
                  {t(`plan.event.changeCount.${k}`, { count: counts[k] })}
                </li>
              ))}
          </ul>
          <ul className="divide-y divide-line" aria-label={t('plan.event.changes')}>
            {changes.map((c, i) => {
              const s = c.after ?? c.before!
              return (
                <li key={i} className="flex items-start gap-3 py-2.5">
                  <span className={cn('chip mt-0.5 h-6 w-[5.5rem] flex-none justify-center', CHANGE_META[c.kind].cls)}>{CHANGE_META[c.kind].label}</span>
                  <span className="min-w-0">
                    <span className={cn('block text-sm font-semibold text-ink', c.kind === 'removed' && 'text-ink-3 line-through')}>{c.before?.title ?? s.title}</span>
                    <span className="num block text-xs text-ink-3">
                      {PLAN_SPORT_LABEL[s.sport]} · {c.details.join(' · ')}
                    </span>
                  </span>
                </li>
              )
            })}
          </ul>
        </>
      )}
    </div>
  )
}

type Step = 'form' | 'next' | 'preview'

interface EventDialogProps {
  open: boolean
  onClose: () => void
  today: string
  /** Day the dialog was opened from. */
  date: string
  /** Existing event to edit; null to report a new one. */
  event: CalendarEvent | null
  /** Skip the event form and go straight to « Adapter la suite » from `date`. */
  adaptOnly?: boolean
}

/** « Signaler un imprévu » → shift today's session or adapt the rest of the plan (rules or AI) → preview → apply. */
export function EventDialog({ open, onClose, today, date, event, adaptOnly }: EventDialogProps) {
  const { toast } = useToast()
  const mut = useEventMutations()
  const plan = usePlanV31()
  const llm = useLlmConfig()
  const llmOn = llm.data?.configured === true
  const [step, setStep] = useState<Step>(adaptOnly ? 'next' : 'form')
  const [saved, setSaved] = useState<CalendarEvent | null>(event)
  const [kind, setKind] = useState<EventKind>(event?.kind ?? 'fatigue')
  const [start, setStart] = useState(event?.startDate ?? date)
  const [end, setEnd] = useState(event?.endDate ?? date)
  const [severity, setSeverity] = useState<EventSeverity>(event?.severity ?? 2)
  const [note, setNote] = useState(event?.note ?? '')
  const [mode, setMode] = useState<'rules' | 'llm'>('rules')
  const [preview, setPreview] = useState<PlanAdaptPreview | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [streamJobId, setStreamJobId] = useState<string | null>(null)

  const from = saved ? (saved.startDate < today ? today : saved.startDate) : date < today ? today : date
  const liveStream = useLlmStream(streamJobId, 'plan_adapt', from)
  const range = usePlanRange(from, addDays(from, 120), open && step !== 'form')
  const current = range.data ?? []
  const affected = current.filter((p) => p.sport !== 'rest' && p.date >= from && p.date <= (saved?.endDate ?? from) && (!p.match || p.match.status === 'upcoming'))

  const adaptMut = useMutation({
    mutationFn: (b: PlanAdaptRequest) => api.planAdaptStream(b, b.mode === 'llm'),
    onSuccess: (r) => {
      if (isJobStatus(r)) {
        setStreamJobId(r.id)
        startLlmStream(r.id, 'plan_adapt', from)
        return
      }
      setStreamJobId(null)
      setPreview(r)
      setStep('preview')
    },
  })

  useEffect(() => {
    if (liveStream?.done && liveStream.adaptPreview) {
      setStreamJobId(null)
      setPreview(liveStream.adaptPreview)
      setStep('preview')
    }
  }, [liveStream?.done, liveStream?.adaptPreview])

  const endBefore = end < start
  const save = () => {
    const body = { kind, startDate: start, endDate: end, severity, note: note.trim() }
    const onSuccess = (e: CalendarEvent) => {
      setSaved(e)
      setStep('next')
      toast({ title: event ? t('plan.event.updated') : t('plan.event.saved'), description: `${EVENT_KIND[e.kind].label} · ${SEVERITY_LABEL[e.severity].toLowerCase()}` })
    }
    if (event) mut.update.mutate({ id: event.id, ...body }, { onSuccess })
    else mut.create.mutate(body, { onSuccess })
  }
  const remove = () =>
    event &&
    mut.remove.mutate(event.id, {
      onSuccess: () => {
        toast({ title: t('plan.event.deleted') })
        onClose()
      },
    })
  const runAdapt = () => {
    clearLlmStream(streamJobId, 'plan_adapt', from)
    setStreamJobId(null)
    adaptMut.mutate({ from, mode, eventId: saved?.id })
  }
  const apply = () =>
    preview &&
    plan.replace.mutate(
      { removeIds: preview.removeIds, sessions: preview.sessions.map(({ id: _id, ...s }) => s) },
      {
        onSuccess: () => {
          toast({ title: t('plan.event.adapted'), description: preview.removeIds.length + preview.sessions.length > 0 ? t('plan.event.applied') : t('plan.event.none'), tone: 'success' })
          onClose()
        },
      },
    )
  const shiftOne = (s: PlannedSession, days: number) =>
    plan.shift.mutate(
      { id: s.id, toDate: addDays(saved && saved.endDate >= s.date ? saved.endDate : s.date, days) },
      { onSuccess: (p) => toast({ title: t('plan.event.shifted'), description: `${p.title} · ${capitalize(fmtDateLong(p.date))}` }) },
    )

  const isStreaming = Boolean(streamJobId && (!liveStream || !liveStream.done))
  const adaptBusy = adaptMut.isPending || isStreaming
  const streamErr = liveStream?.done && liveStream.error ? new Error(liveStream.error) : null
  const error = streamErr ?? mut.create.error ?? mut.update.error ?? mut.remove.error ?? adaptMut.error ?? plan.replace.error ?? plan.shift.error
  const changesCount = preview ? adaptDiff(current, preview).length : 0

  const footer =
    step === 'form' ? (
      <div className="flex flex-wrap items-center justify-between gap-2 pb-3">
        {event ? (
          confirmDelete ? (
            <button type="button" className="btn btn-danger" onClick={remove} disabled={mut.remove.isPending}>
              <Trash2 className="size-4" aria-hidden="true" /> Confirmer
            </button>
          ) : (
            <button type="button" className="btn btn-ghost text-danger" onClick={() => setConfirmDelete(true)}>
              <Trash2 className="size-4" aria-hidden="true" /> Supprimer
            </button>
          )
        ) : (
          <span />
        )}
        <span className="flex gap-2">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button type="button" className="btn btn-primary" onClick={save} disabled={endBefore || mut.create.isPending || mut.update.isPending}>
            {(mut.create.isPending || mut.update.isPending) && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {event ? t('common.actions.save') : t('plan.event.report')}
          </button>
        </span>
      </div>
    ) : step === 'next' ? (
      <div className="flex justify-between gap-2 pb-3">
        {!adaptOnly ? (
          <button type="button" className="btn btn-ghost" onClick={() => setStep('form')}>
            <ArrowLeft className="size-4" aria-hidden="true" /> {t('plan.event.back')}
          </button>
        ) : (
          <span />
        )}
        <button type="button" className="btn btn-ghost" onClick={onClose}>
          {t('plan.event.done')}
        </button>
      </div>
    ) : (
      <div className="flex justify-between gap-2 pb-3">
        <button type="button" className="btn btn-ghost" onClick={() => setStep('next')}>
          <ArrowLeft className="size-4" aria-hidden="true" /> {t('plan.event.previous')}
        </button>
        <button type="button" className="btn btn-primary" onClick={apply} disabled={plan.replace.isPending || changesCount === 0}>
          {plan.replace.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
          {changesCount > 0 ? t('plan.event.applyCount', { count: changesCount }) : t('plan.event.apply')}
        </button>
      </div>
    )

  return (
    <Dialog
      open={open}
      onClose={onClose}
      size="lg"
      title={step === 'form' ? (event ? t('plan.event.editTitle') : t('plan.event.newTitle')) : step === 'next' ? (adaptOnly ? t('plan.event.adaptTitle') : t('plan.event.nextTitle')) : t('plan.event.changes')}
      description={
        step === 'form'
          ? t('plan.event.formHint')
          : saved
            ? `${EVENT_KIND[saved.kind].label} · ${SEVERITY_LABEL[saved.severity].toLowerCase()} · ${saved.startDate === saved.endDate ? fmtDateShort(saved.startDate) : `${fmtDateShort(saved.startDate)} → ${fmtDateShort(saved.endDate)}`}`
            : t('plan.event.fromDate', { date: fmtDateLong(from) })
      }
      footer={footer}
    >
      <div className="space-y-5" aria-busy={adaptBusy}>
        {step === 'form' && (
          <>
            {event?.prescribedBy && <PrescribedByTag by={event.prescribedBy} className="text-xs" />}
            <fieldset>
              <legend className="mb-2 text-sm font-medium text-ink-2">{t('plan.event.type')}</legend>
              <div className="grid gap-2 sm:grid-cols-2" role="radiogroup">
                {KINDS.map((k) => {
                  const Icon = EVENT_ICON[k]
                  const on = kind === k
                  return (
                    <button
                      key={k}
                      type="button"
                      role="radio"
                      aria-checked={on}
                      onClick={() => setKind(k)}
                      className={cn('flex min-h-14 items-center gap-3 rounded-2xl border px-3 py-2 text-left transition-colors', on ? 'border-accent/50 bg-accent/10' : 'border-line hover:bg-track/60')}
                    >
                      <span className={cn('grid size-9 flex-none place-items-center rounded-xl', EVENT_TONE[k])}>
                        <Icon className="size-4" aria-hidden="true" />
                      </span>
                      <span className="min-w-0">
                        <span className="block text-sm font-semibold text-ink">{EVENT_KIND[k].label}</span>
                        <span className="block truncate text-xs text-ink-3">{EVENT_KIND[k].hint}</span>
                      </span>
                    </button>
                  )
                })}
              </div>
            </fieldset>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label={t('plan.event.from')}>{(p) => <input {...p} type="date" className="field" value={start} onChange={(e) => (setStart(e.target.value), end < e.target.value && setEnd(e.target.value))} />}</Field>
              <Field label={t('plan.event.to')} error={endBefore ? t('plan.event.endBefore') : null}>
                {(p) => <input {...p} type="date" className="field" value={end} min={start} onChange={(e) => setEnd(e.target.value)} />}
              </Field>
            </div>
            <div>
              <p className="mb-1.5 text-sm font-medium text-ink-2">{t('plan.event.intensity')}</p>
              <SegmentedControl<string>
                ariaLabel={t('plan.event.intensity')}
                value={String(severity)}
                onChange={(v) => setSeverity(Number(v) as EventSeverity)}
                options={[1, 2, 3].map((n) => ({ value: String(n), label: SEVERITY_LABEL[n as EventSeverity] }))}
              />
            </div>
            <Field label={t('plan.event.note')}>
              {(p) => <textarea {...p} className="field min-h-20 resize-y" value={note} onChange={(e) => setNote(e.target.value)} maxLength={500} placeholder={t('plan.event.notePlaceholder')} />}
            </Field>
          </>
        )}

        {step === 'next' && (
          <>
            {affected.length > 0 && (
              <section aria-labelledby="shift-title" className="space-y-2">
                <h3 id="shift-title" className="flex items-center gap-2 text-sm font-semibold text-ink">
                  <CalendarClock className="size-4 text-accent" aria-hidden="true" /> {t('plan.event.shiftToday')}
                </h3>
                <ul className="space-y-1.5">
                  {affected.map((s) => (
                    <li key={s.id} className="flex flex-wrap items-center justify-between gap-2 rounded-2xl bg-track/50 px-3 py-2">
                      <span className="min-w-0">
                        <span className="block truncate text-sm font-medium text-ink">{s.title}</span>
                        <span className="block text-xs text-ink-3">{capitalize(fmtDateLong(s.date))}</span>
                      </span>
                      <span className="flex gap-1.5">
                        <button type="button" className="chip h-8 border border-line px-3 text-ink-2 hover:bg-track" onClick={() => shiftOne(s, 1)} disabled={plan.shift.isPending}>
                          {t('plan.event.plusDays', { n: 1 })}
                        </button>
                        <button type="button" className="chip h-8 border border-line px-3 text-ink-2 hover:bg-track" onClick={() => shiftOne(s, 2)} disabled={plan.shift.isPending}>
                          {t('plan.event.plusDays', { n: 2 })}
                        </button>
                      </span>
                    </li>
                  ))}
                </ul>
                {saved && saved.endDate > saved.startDate && <p className="text-xs text-ink-3">{t('plan.event.shiftHint')}</p>}
              </section>
            )}
            <section aria-labelledby="adapt-title" className="space-y-3">
              <h3 id="adapt-title" className="flex items-center gap-2 text-sm font-semibold text-ink">
                <Wand2 className="size-4 text-accent" aria-hidden="true" /> {t('plan.event.adapt')}
              </h3>
              <p className="text-xs text-ink-3">
                {mode === 'rules'
                  ? t('plan.event.rulesHint')
                  : t('plan.event.llmHint')}
              </p>
              <div className="flex flex-wrap items-center gap-2">
                <SegmentedControl<'rules' | 'llm'>
                  ariaLabel={t('plan.event.method')}
                  size="sm"
                  className="w-full sm:w-72"
                  value={mode}
                  onChange={(v) => setMode(v)}
                  options={[
                    { value: 'rules', label: t('plan.event.rules'), icon: <Wand2 /> },
                    { value: 'llm', label: t('plan.event.myAi'), icon: <Bot /> },
                  ]}
                />
                <button type="button" className="btn btn-primary max-sm:w-full" onClick={runAdapt} disabled={adaptBusy || (mode === 'llm' && !llmOn)}>
                  {adaptBusy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Sparkles className="size-4" aria-hidden="true" />}
                  {t('plan.event.preview')}
                </button>
              </div>
              {mode === 'llm' && !llmOn && llm.isSuccess && <p className="text-xs text-ink-3">{t('plan.event.noAi')}</p>}
              {adaptBusy && mode === 'llm' && (
                <div className="space-y-2 rounded-2xl border border-accent/25 bg-track/35 p-3">
                  <p className="text-sm text-ink-2">{liveStream?.text ? t('plan.event.streaming') : t('plan.event.preparing')}</p>
                  {liveStream?.text && (
                    <pre className="scroll-thin max-h-36 overflow-y-auto whitespace-pre-wrap font-mono text-xs text-ink-3">
                      {liveStream.text}
                      <span className="inline-block h-3.5 w-1.5 animate-pulse bg-accent align-middle" aria-hidden="true" />
                    </pre>
                  )}
                </div>
              )}
            </section>
          </>
        )}

        {step === 'preview' && preview && <Preview preview={preview} current={current} />}
        {error && <InlineError>{error.message}</InlineError>}
      </div>
    </Dialog>
  )
}
