import { useMutation } from '@tanstack/react-query'
import { ArrowLeft, Bot, ClipboardPaste, Copy, Loader2, Sparkles, TriangleAlert } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Link } from 'wouter'
import { api } from '../../api/client'
import { useLlmConfig, usePlanV3 } from '../../api/queriesV3'
import type { Goal, PlanDraftsResponse, PlanGenerateRequest, PlanHorizon, PlannedSessionInput, PlanPhase, PlanPromptResponse } from '../../api/types'
import { copyText } from '../../lib/clipboard'
import { cn } from '../../lib/cn'
import { addDays, capitalize, fmtDateLong, fmtDateShort, fmtNumber, mondayOf } from '../../lib/format'
import { PLAN_SPORT_LABEL } from '../../lib/labels'
import { clearLlmStream, isJobStatus, startLlmStream, useLlmStream } from '../../state/events'
import { Dialog } from '../ui/Dialog'
import { Checkbox, Field } from '../ui/Field'
import { InlineError } from '../ui/Misc'
import { SegmentedControl } from '../ui/SegmentedControl'
import { useToast } from '../ui/Toast'
import { phaseTone } from './longterm'
import { targetParts, weekLabel } from './planFormat'
import { stepsSummary } from './steps'
import { t } from '../../i18n'

type Mode = 'llm' | 'prompt'
type Step = 'config' | 'paste' | 'preview'

const TONE_BG = { accent: 'bg-accent', ok: 'bg-ok', warn: 'bg-warn', violet: 'bg-violet', info: 'bg-info' } as const

function PhaseList({ phases }: { phases: PlanPhase[] }) {
  return (
    <section aria-labelledby="gen-phases" className="rounded-2xl bg-track/50 p-3">
      <h3 id="gen-phases" className="label-caps mb-2">
        {t('plan.generate.phases')}
      </h3>
      <ol className="space-y-1.5">
        {phases.map((p, i) => (
          <li key={`${p.name}-${p.startDate}`} className="flex items-start gap-2 text-sm">
            <span className={cn('mt-1.5 size-2 flex-none rounded-full', TONE_BG[phaseTone(p.name, i)])} aria-hidden="true" />
            <span className="min-w-0">
              <span className="font-semibold text-ink">{p.name}</span>{' '}
              <span className="num text-xs text-ink-3">
                {fmtDateShort(p.startDate)} → {fmtDateShort(p.endDate)}
                {p.weeklyVolumeKm ? t('plan.generate.perWeek', { km: fmtNumber(p.weeklyVolumeKm) }) : ''}
              </span>
              {p.focus && <span className="block text-xs text-ink-3">{p.focus}</span>}
            </span>
          </li>
        ))}
      </ol>
    </section>
  )
}

function DraftList({ drafts, errors, selected, onToggle, onAll }: { drafts: PlannedSessionInput[]; errors: string[]; selected: Set<number>; onToggle: (i: number, on: boolean) => void; onAll: (on: boolean) => void }) {
  const weeks = new Map<string, number[]>()
  drafts.forEach((d, i) => {
    const w = mondayOf(d.date)
    weeks.set(w, [...(weeks.get(w) ?? []), i])
  })
  return (
    <div className="space-y-3">
      {errors.length > 0 && (
        <div role="alert" className="rounded-2xl border border-warn/30 bg-warn/[0.07] p-3">
          <p className="flex items-center gap-1.5 text-sm font-semibold text-warn">
            <TriangleAlert className="size-4" aria-hidden="true" /> {t('plan.generate.toCheck', { count: errors.length })}
          </p>
          <ul className="mt-1 list-disc pl-5 text-sm text-ink-2">
            {errors.map((e) => (
              <li key={e}>{e}</li>
            ))}
          </ul>
        </div>
      )}
      {drafts.length === 0 ? (
        <p className="text-sm text-ink-3">{t('plan.generate.noDraft')}</p>
      ) : (
        <>
          {drafts.length > 3 && (
            <div className="flex justify-end">
              <button type="button" className="text-xs font-semibold text-accent underline-offset-2 hover:underline" onClick={() => onAll(selected.size !== drafts.length)}>
                {selected.size === drafts.length ? t('plan.generate.unselectAll') : t('plan.generate.selectAll')}
              </button>
            </div>
          )}
          {[...weeks.entries()].map(([w, idx]) => (
            <section key={w} aria-label={weekLabel(w)}>
              {weeks.size > 1 && <h3 className="label-caps mb-1.5">{weekLabel(w)}</h3>}
              <ul className="space-y-2">
                {idx.map((i) => {
                  const d = drafts[i]
                  return (
                    <li key={i} className={cn('rounded-2xl border p-3 transition-opacity', selected.has(i) ? 'border-line' : 'border-line opacity-55')}>
                      <Checkbox
                        checked={selected.has(i)}
                        onChange={(on) => onToggle(i, on)}
                        label={
                          <span>
                            <span className="block text-xs text-ink-3">
                              {capitalize(fmtDateLong(d.date))} · {PLAN_SPORT_LABEL[d.sport] ?? d.sport}
                            </span>
                            <span className="block font-semibold text-ink">{d.title}</span>
                          </span>
                        }
                        description={
                          <span className="block space-y-0.5">
                            {targetParts(d).length > 0 && <span className="num block">{targetParts(d).join(' · ')}</span>}
                            {d.steps && d.steps.length > 0 && <span className="num block text-ink-2">{stepsSummary(d.steps)}</span>}
                            {d.instructions && <span className="block">{d.instructions}</span>}
                          </span>
                        }
                      />
                    </li>
                  )
                })}
              </ul>
            </section>
          ))}
        </>
      )}
    </div>
  )
}

/** « Générer » : connected LLM → preview → confirm, or prompt → copy → paste answer → preview → confirm bulk create. */
interface GenerateDialogProps {
  open: boolean
  onClose: () => void
  today: string
  /** Active goals with a future date (nearest first). */
  goals?: Goal[]
  initialHorizon?: PlanHorizon
  initialGoalId?: number
}

export function GenerateDialog({ open, onClose, today, goals = [], initialHorizon, initialGoalId }: GenerateDialogProps) {
  const llm = useLlmConfig()
  const plan = usePlanV3()
  const { toast } = useToast()
  const llmOn = llm.data?.configured === true
  const [mode, setMode] = useState<Mode | null>(null)
  const [step, setStep] = useState<Step>('config')
  const [horizon, setHorizon] = useState<PlanHorizon>(initialHorizon && (initialHorizon !== 'goal' || goals.length) ? initialHorizon : 'week')
  const [goalId, setGoalId] = useState<number | undefined>(initialGoalId ?? goals[0]?.id)
  const goal = goals.find((g) => g.id === goalId) ?? goals[0]
  const [startDate, setStartDate] = useState(addDays(today, 1))
  const [note, setNote] = useState('')
  const [prompt, setPrompt] = useState<PlanPromptResponse | null>(null)
  const [answer, setAnswer] = useState('')
  const [result, setResult] = useState<PlanDraftsResponse | null>(null)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [streamJobId, setStreamJobId] = useState<string | null>(null)
  const effMode: Mode = llmOn ? (mode ?? 'llm') : 'prompt'
  const liveStream = useLlmStream(streamJobId, 'plan_generate', startDate)

  const showDrafts = (r: PlanDraftsResponse) => {
    setStreamJobId(null)
    setResult(r)
    setSelected(new Set(r.drafts.map((_, i) => i)))
    setStep('preview')
  }

  const genMut = useMutation({
    mutationFn: (b: PlanGenerateRequest) => api.planGenerateStream(b, b.mode === 'llm'),
    onSuccess: (r) => {
      if (isJobStatus(r)) {
        setStreamJobId(r.id)
        startLlmStream(r.id, 'plan_generate', startDate)
        return
      }
      if ('prompt' in r) {
        setPrompt(r)
        setStep('paste')
      } else {
        showDrafts(r)
      }
    },
  })

  useEffect(() => {
    if (liveStream?.done && liveStream.planDrafts) {
      showDrafts(liveStream.planDrafts)
    }
  }, [liveStream?.done, liveStream?.planDrafts])

  const generate = () => {
    clearLlmStream(streamJobId, 'plan_generate', startDate)
    setStreamJobId(null)
    genMut.mutate({ mode: effMode, horizon, startDate, goalId: horizon === 'goal' ? goal?.id : undefined, note: note.trim() || undefined })
  }
  const parse = () => plan.importText.mutate(answer, { onSuccess: showDrafts })
  const confirm = () => {
    const chosen = (result?.drafts ?? []).filter((_, i) => selected.has(i)).map((d) => ({ ...d, source: d.source ?? (effMode === 'llm' ? 'llm' : 'import') }))
    const phases = result?.phases ?? []
    plan.bulk.mutate(
      { sessions: chosen, phases },
      {
        onSuccess: (r) => {
          toast({ title: t('plan.generate.added', { count: r.length }), description: phases.length ? t('plan.generate.phasesSaved', { count: phases.length }) : undefined, tone: 'success' })
          onClose()
        },
      },
    )
  }
  const phaseCount = result?.phases?.length ?? 0
  const isStreaming = Boolean(streamJobId && (!liveStream || !liveStream.done))
  const busy = genMut.isPending || isStreaming || plan.importText.isPending || plan.bulk.isPending
  const streamErr = liveStream?.done && liveStream.error ? new Error(liveStream.error) : null
  const error = streamErr ?? genMut.error ?? plan.importText.error ?? plan.bulk.error

  const footer =
    step === 'config' ? (
      <div className="flex justify-end gap-2 pb-3">
        <button type="button" className="btn btn-ghost" onClick={onClose}>
          {t('common.actions.cancel')}
        </button>
        <button type="button" className="btn btn-primary" onClick={generate} disabled={busy || !startDate}>
          {genMut.isPending || isStreaming ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : effMode === 'llm' ? <Sparkles className="size-4" aria-hidden="true" /> : <Copy className="size-4" aria-hidden="true" />}
          {effMode === 'llm' ? t('plan.generate.generate') : t('plan.generate.getPrompt')}
        </button>
      </div>
    ) : step === 'paste' ? (
      <div className="flex justify-between gap-2 pb-3">
        <button type="button" className="btn btn-ghost" onClick={() => setStep('config')}>
          <ArrowLeft className="size-4" aria-hidden="true" /> {t('plan.generate.back')}
        </button>
        <button type="button" className="btn btn-primary" onClick={parse} disabled={busy || !answer.trim()}>
          {plan.importText.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <ClipboardPaste className="size-4" aria-hidden="true" />}
          {t('plan.generate.read')}
        </button>
      </div>
    ) : (
      <div className="flex justify-between gap-2 pb-3">
        <button type="button" className="btn btn-ghost" onClick={() => setStep(effMode === 'llm' ? 'config' : 'paste')}>
          <ArrowLeft className="size-4" aria-hidden="true" /> {t('plan.generate.back')}
        </button>
        <button type="button" className="btn btn-primary" onClick={confirm} disabled={busy || (selected.size === 0 && phaseCount === 0)}>
          {plan.bulk.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
          {t('plan.generate.addSessions', { count: selected.size })}
          {phaseCount > 0 ? t('plan.generate.plusPhases', { count: phaseCount }) : ''}
        </button>
      </div>
    )

  return (
    <Dialog
      open={open}
      onClose={onClose}
      size="lg"
      title={t('plan.generate.title')}
      description={step === 'config' ? t('plan.generate.configHint') : step === 'paste' ? t('plan.generate.pasteHint') : t('plan.generate.previewHint')}
      footer={footer}
    >
      <div className="space-y-4" aria-busy={busy}>
        {step === 'config' && (
          <>
            {llmOn && (
              <SegmentedControl<Mode>
                ariaLabel={t('plan.generate.mode')}
                value={effMode}
                onChange={setMode}
                options={[
                  { value: 'llm', label: t('plan.generate.connected'), icon: <Bot /> },
                  { value: 'prompt', label: t('plan.generate.copyPaste'), icon: <ClipboardPaste /> },
                ]}
              />
            )}
            {!llmOn && llm.isSuccess && (
              <p className="text-xs text-ink-3">
                {t('plan.generate.noAi')}{' '}
                <Link href="/settings#ia" className="underline underline-offset-2" onClick={onClose}>
                  {t('plan.generate.connectAi')}
                </Link>{' '}
                {t('plan.generate.orCopyPaste')}
              </p>
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label={t('plan.generate.horizon')} className={horizon === 'goal' && goals.length > 1 ? 'sm:col-span-2' : undefined} hint={horizon === 'goal' ? t('plan.generate.horizonHint') : undefined}>
                {(p) => (
                  <select {...p} className="field" value={horizon} onChange={(e) => setHorizon(e.target.value as PlanHorizon)}>
                    <option value="session">{t('plan.generate.oneSession')}</option>
                    <option value="week">{t('plan.generate.days', { n: 7 })}</option>
                    <option value="2weeks">{t('plan.generate.days', { n: 14 })}</option>
                    <option value="goal" disabled={!goal}>
                      {goal ? t('plan.generate.untilGoal', { date: fmtDateShort(goal.targetDate as string) }) : t('plan.generate.untilGoalNone')}
                    </option>
                  </select>
                )}
              </Field>
              {horizon === 'goal' && goals.length > 1 && (
                <Field label={t('plan.generate.goal')}>
                  {(p) => (
                    <select {...p} className="field" value={goal?.id} onChange={(e) => setGoalId(Number(e.target.value))}>
                      {goals.map((g) => (
                        <option key={g.id} value={g.id}>
                          {g.title} · {fmtDateShort(g.targetDate as string)}
                        </option>
                      ))}
                    </select>
                  )}
                </Field>
              )}
              <Field label={t('plan.generate.from')}>{(p) => <input {...p} type="date" className="field" value={startDate} min={today} onChange={(e) => setStartDate(e.target.value)} />}</Field>
              <Field label={t('plan.generate.note')} className="sm:col-span-2">
                {(p) => <textarea {...p} className="field min-h-20 resize-y" value={note} onChange={(e) => setNote(e.target.value)} maxLength={500} placeholder={t('plan.generate.notePlaceholder')} />}
              </Field>
            </div>
          </>
        )}

        {step === 'paste' && prompt && (
          <>
            <div>
              <div className="mb-1.5 flex items-center justify-between gap-2">
                <p className="text-sm font-medium text-ink-2">{t('plan.generate.promptStep')}</p>
                <span className="num text-xs text-ink-3">≈ {fmtNumber(prompt.tokensEstimate)} tokens</span>
              </div>
              <textarea readOnly className="field num min-h-36 resize-y font-mono text-xs" value={prompt.prompt} aria-label={t('plan.generate.promptAria')} onFocus={(e) => e.currentTarget.select()} />
              <button type="button" className="btn btn-ghost mt-2" onClick={async () => toast((await copyText(prompt.prompt)) ? { title: t('plan.generate.copied'), tone: 'success' } : { title: t('plan.generate.copyFailed'), tone: 'error' })}>
                <Copy className="size-4" aria-hidden="true" /> {t('plan.generate.copy')}
              </button>
            </div>
            <Field label={t('plan.generate.answer')} hint={t('plan.generate.answerHint')}>
              {(p) => <textarea {...p} className="field min-h-36 resize-y font-mono text-xs" value={answer} onChange={(e) => setAnswer(e.target.value)} placeholder={horizon === 'goal' ? '```json\n{"format":"stride-plan","version":2,"phases":[…],"sessions":[…]}\n```' : '```json\n{"format":"stride-plan","version":1,"sessions":[…]}\n```'} />}
            </Field>
          </>
        )}

        {step === 'preview' && result && (
          <>
            {phaseCount > 0 && <PhaseList phases={result.phases ?? []} />}
            <DraftList
              drafts={result.drafts}
              errors={result.errors}
              selected={selected}
              onAll={(on) => setSelected(on ? new Set(result.drafts.map((_, i) => i)) : new Set())}
              onToggle={(i, on) => {
                const next = new Set(selected)
                if (on) next.add(i)
                else next.delete(i)
                setSelected(next)
              }}
            />
          </>
        )}

        {(genMut.isPending || isStreaming) && effMode === 'llm' && (
          <div className="space-y-2 rounded-2xl border border-accent/25 bg-track/35 p-3">
            <p className="flex items-center gap-2 text-sm text-ink-2">
              <Loader2 className="size-4 animate-spin text-accent" aria-hidden="true" />
              {liveStream?.text ? t('plan.generate.streaming') : t('plan.generate.preparing')}
            </p>
            {liveStream?.text && (
              <pre className="scroll-thin max-h-40 overflow-y-auto whitespace-pre-wrap font-mono text-xs text-ink-3">
                {liveStream.text}
                <span className="inline-block h-3.5 w-1.5 animate-pulse bg-accent align-middle" aria-hidden="true" />
              </pre>
            )}
          </div>
        )}
        {error && <InlineError>{error.message}</InlineError>}
      </div>
    </Dialog>
  )
}
