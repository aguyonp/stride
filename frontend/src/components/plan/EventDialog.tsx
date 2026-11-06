import { useMutation } from '@tanstack/react-query'
import { ArrowLeft, Bandage, BatteryLow, BedDouble, Bot, CalendarClock, Loader2, Plane, Sparkles, Thermometer, Trash2, Wand2 } from 'lucide-react'
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

const EVENT_ICON: Record<EventKind, ComponentType<{ className?: string }>> = { fatigue: BatteryLow, pain: Bandage, illness: Thermometer, unavailable: Plane, forced_rest: BedDouble }
const KINDS = Object.keys(EVENT_KIND) as EventKind[]
const EVENT_TONE: Record<EventKind, string> = { fatigue: 'text-warn bg-warn/10', pain: 'text-danger bg-danger/10', illness: 'text-danger bg-danger/10', unavailable: 'text-info bg-info/10', forced_rest: 'text-violet bg-violet/10' }

/** Discreet event marker on a calendar day. */
export function EventChip({ e, onOpen, compact }: { e: CalendarEvent; onOpen?: (e: CalendarEvent) => void; compact?: boolean }) {
  const Icon = EVENT_ICON[e.kind]
  const label = `${EVENT_KIND[e.kind].label}, ${SEVERITY_LABEL[e.severity].toLowerCase()}${e.note ? ` : ${e.note}` : ''}`
  return (
    <button type="button" onClick={() => onOpen?.(e)} title={label} aria-label={`Imprévu : ${label}`} className={cn('inline-flex max-w-full min-w-0 items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-semibold', EVENT_TONE[e.kind])}>
      <Icon className="size-3 flex-none" aria-hidden="true" />
      {!compact && <span className="truncate">{EVENT_KIND[e.kind].short}</span>}
      <span className="flex gap-px" aria-hidden="true">
        {[1, 2, 3].map((i) => (
          <span key={i} className={cn('size-1 rounded-full bg-current', i > e.severity && 'opacity-25')} />
        ))}
      </span>
    </button>
  )
}

const CHANGE_META: Record<ChangeKind, { label: string; cls: string }> = {
  moved: { label: 'Décalée', cls: 'bg-info/12 text-info' },
  modified: { label: 'Allégée', cls: 'bg-warn/12 text-warn' },
  removed: { label: 'Supprimée', cls: 'bg-danger/12 text-danger' },
  created: { label: 'Ajoutée', cls: 'bg-ok/12 text-ok' },
}

function Preview({ preview, current }: { preview: PlanAdaptPreview; current: PlannedSession[] }) {
  const changes = adaptDiff(current, preview)
  const counts = changeCounts(changes)
  return (
    <div className="space-y-4">
      <p className="text-sm text-ink-2">{preview.summary}</p>
      {changes.length === 0 ? (
        <p className="rounded-2xl bg-track/50 p-4 text-sm text-ink-3">Aucun changement proposé : ton plan reste tel quel.</p>
      ) : (
        <>
          <ul className="flex flex-wrap gap-1.5 text-xs" aria-label="Résumé des changements">
            {(Object.keys(counts) as ChangeKind[])
              .filter((k) => counts[k] > 0)
              .map((k) => (
                <li key={k} className={cn('chip h-6', CHANGE_META[k].cls)}>
                  {counts[k]} {CHANGE_META[k].label.toLowerCase()}
                  {counts[k] > 1 ? 's' : ''}
                </li>
              ))}
          </ul>
          <ul className="divide-y divide-line" aria-label="Changements proposés">
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
      toast({ title: event ? 'Imprévu mis à jour' : 'Imprévu enregistré', description: `${EVENT_KIND[e.kind].label} · ${SEVERITY_LABEL[e.severity].toLowerCase()}` })
    }
    if (event) mut.update.mutate({ id: event.id, ...body }, { onSuccess })
    else mut.create.mutate(body, { onSuccess })
  }
  const remove = () =>
    event &&
    mut.remove.mutate(event.id, {
      onSuccess: () => {
        toast({ title: 'Imprévu supprimé' })
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
          toast({ title: 'Plan adapté', description: `${preview.removeIds.length + preview.sessions.length > 0 ? 'Changements appliqués au planning.' : 'Aucun changement.'}`, tone: 'success' })
          onClose()
        },
      },
    )
  const shiftOne = (s: PlannedSession, days: number) =>
    plan.shift.mutate(
      { id: s.id, toDate: addDays(saved && saved.endDate >= s.date ? saved.endDate : s.date, days) },
      { onSuccess: (p) => toast({ title: 'Séance décalée', description: `${p.title} · ${capitalize(fmtDateLong(p.date))}` }) },
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
            Annuler
          </button>
          <button type="button" className="btn btn-primary" onClick={save} disabled={endBefore || mut.create.isPending || mut.update.isPending}>
            {(mut.create.isPending || mut.update.isPending) && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {event ? 'Enregistrer' : 'Signaler'}
          </button>
        </span>
      </div>
    ) : step === 'next' ? (
      <div className="flex justify-between gap-2 pb-3">
        {!adaptOnly ? (
          <button type="button" className="btn btn-ghost" onClick={() => setStep('form')}>
            <ArrowLeft className="size-4" aria-hidden="true" /> Imprévu
          </button>
        ) : (
          <span />
        )}
        <button type="button" className="btn btn-ghost" onClick={onClose}>
          Terminé
        </button>
      </div>
    ) : (
      <div className="flex justify-between gap-2 pb-3">
        <button type="button" className="btn btn-ghost" onClick={() => setStep('next')}>
          <ArrowLeft className="size-4" aria-hidden="true" /> Retour
        </button>
        <button type="button" className="btn btn-primary" onClick={apply} disabled={plan.replace.isPending || changesCount === 0}>
          {plan.replace.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
          Appliquer {changesCount > 0 ? `${changesCount} changement${changesCount > 1 ? 's' : ''}` : ''}
        </button>
      </div>
    )

  return (
    <Dialog
      open={open}
      onClose={onClose}
      size="lg"
      title={step === 'form' ? (event ? 'Modifier l’imprévu' : 'Signaler un imprévu') : step === 'next' ? (adaptOnly ? 'Adapter la suite du plan' : 'Et maintenant ?') : 'Changements proposés'}
      description={
        step === 'form'
          ? 'Stride en tient compte dans tes prompts et peut ajuster le planning.'
          : saved
            ? `${EVENT_KIND[saved.kind].label} · ${SEVERITY_LABEL[saved.severity].toLowerCase()} · ${saved.startDate === saved.endDate ? fmtDateShort(saved.startDate) : `${fmtDateShort(saved.startDate)} → ${fmtDateShort(saved.endDate)}`}`
            : `À partir du ${fmtDateLong(from)}`
      }
      footer={footer}
    >
      <div className="space-y-5" aria-busy={adaptBusy}>
        {step === 'form' && (
          <>
            <fieldset>
              <legend className="mb-2 text-sm font-medium text-ink-2">Type</legend>
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
              <Field label="Du">{(p) => <input {...p} type="date" className="field" value={start} onChange={(e) => (setStart(e.target.value), end < e.target.value && setEnd(e.target.value))} />}</Field>
              <Field label="Au (inclus)" error={endBefore ? 'La fin doit suivre le début.' : null}>
                {(p) => <input {...p} type="date" className="field" value={end} min={start} onChange={(e) => setEnd(e.target.value)} />}
              </Field>
            </div>
            <div>
              <p className="mb-1.5 text-sm font-medium text-ink-2">Intensité</p>
              <SegmentedControl<string>
                ariaLabel="Intensité"
                value={String(severity)}
                onChange={(v) => setSeverity(Number(v) as EventSeverity)}
                options={[1, 2, 3].map((n) => ({ value: String(n), label: SEVERITY_LABEL[n as EventSeverity] }))}
              />
            </div>
            <Field label="Note (facultatif)">
              {(p) => <textarea {...p} className="field min-h-20 resize-y" value={note} onChange={(e) => setNote(e.target.value)} maxLength={500} placeholder="Ex. genou gauche sensible dans les escaliers, fièvre 38,5" />}
            </Field>
          </>
        )}

        {step === 'next' && (
          <>
            {affected.length > 0 && (
              <section aria-labelledby="shift-title" className="space-y-2">
                <h3 id="shift-title" className="flex items-center gap-2 text-sm font-semibold text-ink">
                  <CalendarClock className="size-4 text-accent" aria-hidden="true" /> Décaler la séance du jour
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
                          +1 j
                        </button>
                        <button type="button" className="chip h-8 border border-line px-3 text-ink-2 hover:bg-track" onClick={() => shiftOne(s, 2)} disabled={plan.shift.isPending}>
                          +2 j
                        </button>
                      </span>
                    </li>
                  ))}
                </ul>
                {saved && saved.endDate > saved.startDate && <p className="text-xs text-ink-3">Le décalage part du lendemain de la fin de l’imprévu.</p>}
              </section>
            )}
            <section aria-labelledby="adapt-title" className="space-y-3">
              <h3 id="adapt-title" className="flex items-center gap-2 text-sm font-semibold text-ink">
                <Wand2 className="size-4 text-accent" aria-hidden="true" /> Adapter la suite du plan
              </h3>
              <p className="text-xs text-ink-3">
                {mode === 'rules'
                  ? 'Règles Stride : pas de séance pendant l’imprévu, reprise allégée (−20 à −40 % selon l’intensité, fractionné remplacé par du footing), jamais plus de +10 % de volume.'
                  : 'Ton IA reçoit l’imprévu, ta charge récente, ta préparation et le plan restant, puis propose un plan au format stride-plan.'}
              </p>
              <div className="flex flex-wrap items-center gap-2">
                <SegmentedControl<'rules' | 'llm'>
                  ariaLabel="Méthode d’adaptation"
                  size="sm"
                  className="w-full sm:w-72"
                  value={mode}
                  onChange={(v) => setMode(v)}
                  options={[
                    { value: 'rules', label: 'Auto (règles)', icon: <Wand2 /> },
                    { value: 'llm', label: 'Mon IA', icon: <Bot /> },
                  ]}
                />
                <button type="button" className="btn btn-primary max-sm:w-full" onClick={runAdapt} disabled={adaptBusy || (mode === 'llm' && !llmOn)}>
                  {adaptBusy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Sparkles className="size-4" aria-hidden="true" />}
                  Prévisualiser
                </button>
              </div>
              {mode === 'llm' && !llmOn && llm.isSuccess && <p className="text-xs text-ink-3">Aucune IA connectée : branche-la dans Réglages › IA ou utilise les règles automatiques.</p>}
              {adaptBusy && mode === 'llm' && (
                <div className="space-y-2 rounded-2xl border border-accent/25 bg-track/35 p-3">
                  <p className="text-sm text-ink-2">{liveStream?.text ? 'Adaptation IA en direct…' : 'Ton IA prépare la proposition…'}</p>
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
