import { Check, Download, Loader2, Sparkles } from 'lucide-react'
import { useEffect, useState } from 'react'
import { api } from '../../../api/client'
import { useLlmConfig } from '../../../api/queriesV3'
import { useMorphActions } from '../../../api/queriesV42'
import type { MorphConstraint, MorphPreview, PlannedSession } from '../../../api/types'
import { cn } from '../../../lib/cn'
import { capitalize, fmtDateLong } from '../../../lib/format'
import { clearLlmStream, startLlmStream, useLlmStream } from '../../../state/events'
import { downloadBlob } from '../../profile/download'
import { Dialog } from '../../ui/Dialog'
import { InlineError } from '../../ui/Misc'
import { useToast } from '../../ui/Toast'
import { changeLabel, CONSTRAINT_META, SEVERITY_TEXT } from './morphFormat'
import { t } from '../../../i18n'

/** Stream kind of `llm.token` / `llm.done` for a morph refinement (ref = session id). */
const STREAM_KIND = 'morph_refine'

export function ConstraintChip({ c, compact }: { c: MorphConstraint; compact?: boolean }) {
  const meta = CONSTRAINT_META[c.kind]
  const Icon = meta.icon
  return (
    <li className={cn('inline-flex max-w-full min-w-0 items-start gap-1.5 rounded-2xl border px-2.5 py-1 text-xs', meta.cls)}>
      <Icon className="mt-0.5 size-3.5 flex-none" aria-hidden="true" />
      <span className="min-w-0">
        <span className="font-semibold">{c.label}</span>
        {!compact && c.detail && <span className="text-ink-2"> · {c.detail}</span>}
        <span className="sr-only">{t('plan.morph.intensity', { level: SEVERITY_TEXT[c.severity] })}</span>
      </span>
    </li>
  )
}

/** « Adapter la séance »: today's constraints, field-by-field diff, optional AI refinement, validate then .FIT. */
export function MorphDialog({ session, preview: initial, open, onClose }: { session: PlannedSession; preview: MorphPreview; open: boolean; onClose: () => void }) {
  const { toast } = useToast()
  const llm = useLlmConfig()
  const m = useMorphActions()
  const [preview, setPreview] = useState(initial)
  const [note, setNote] = useState('')
  const [jobId, setJobId] = useState<string | null>(null)
  const [applied, setApplied] = useState<PlannedSession | null>(null)
  const [downloading, setDownloading] = useState(false)
  const ref = String(session.id)
  const stream = useLlmStream(jobId, STREAM_KIND, ref)
  const streaming = Boolean(jobId && (!stream || !stream.done))
  const streamError = stream?.done && stream.error ? stream.error : null
  const llmOn = llm.data?.configured === true

  useEffect(() => {
    if (stream?.done && stream.morphPreview) {
      setPreview(stream.morphPreview)
      setJobId(null)
      toast({ title: t('plan.morph.refined'), tone: 'info' })
    }
  }, [stream?.done, stream?.morphPreview, toast])

  const refine = () => {
    clearLlmStream(jobId, STREAM_KIND, ref)
    m.refine.mutate(
      { id: session.id, note: note.trim() || undefined },
      {
        onSuccess: (job) => {
          setJobId(job.id)
          startLlmStream(job.id, STREAM_KIND, ref)
        },
      },
    )
  }
  const validate = () =>
    preview.proposal &&
    m.apply.mutate(
      { id: session.id, proposal: preview.proposal },
      {
        onSuccess: (s) => {
          setApplied(s)
          toast({ title: t('plan.morph.saved'), description: s.title, tone: 'success' })
        },
      },
    )
  const fit = async () => {
    setDownloading(true)
    try {
      const { blob, filename } = await api.workoutFit(session.id)
      downloadBlob(blob, filename)
      toast({ title: t('plan.morph.fitDownloaded'), description: t('plan.morph.fitHint') })
    } catch (e) {
      toast({ title: t('plan.morph.downloadFailed'), description: e instanceof Error ? e.message : undefined, tone: 'error' })
    } finally {
      setDownloading(false)
    }
  }

  const steps = preview.original.steps ?? []
  const error = m.apply.error?.message ?? m.refine.error?.message ?? streamError

  const footer = applied ? (
    <div className="flex flex-wrap justify-end gap-2 pb-3">
      <button type="button" className="btn btn-ghost" onClick={onClose}>
        {t('plan.morph.close')}
      </button>
      <button type="button" className="btn btn-primary" onClick={() => void fit()} disabled={downloading}>
        {downloading ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Download className="size-4" aria-hidden="true" />}
        {t('plan.morph.downloadFit')}
      </button>
    </div>
  ) : (
    <div className="flex flex-wrap justify-end gap-2 pb-3">
      <button type="button" className="btn btn-ghost" onClick={onClose}>
        {t('common.actions.cancel')}
      </button>
      <button type="button" className="btn btn-primary" onClick={validate} disabled={!preview.proposal || m.apply.isPending || streaming}>
        {m.apply.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Check className="size-4" aria-hidden="true" />}
        {t('plan.morph.validate')}
      </button>
    </div>
  )

  return (
    <Dialog open={open} onClose={onClose} size="lg" title={t('plan.morph.adapt')} description={`${capitalize(fmtDateLong(session.date))} · ${session.title}`} footer={footer}>
      {applied ? (
        <div className="space-y-3" role="status">
          <p className="flex items-center gap-2 text-sm font-semibold text-ok">
            <Check className="size-4" aria-hidden="true" /> {t('plan.morph.savedDone')}
          </p>
          <p className="text-sm text-ink-2">{t('plan.morph.savedHint')}</p>
        </div>
      ) : (
        <div className="space-y-5" aria-busy={streaming}>
          <section aria-labelledby="morph-constraints">
            <h3 id="morph-constraints" className="mb-2 text-sm font-medium text-ink-2">
              {t('plan.morph.todayConstraints')}
            </h3>
            <ul className="flex flex-wrap gap-1.5">
              {preview.constraints.map((c, i) => (
                <ConstraintChip key={`${c.kind}-${i}`} c={c} />
              ))}
            </ul>
          </section>

          <section aria-labelledby="morph-changes">
            <h3 id="morph-changes" className="mb-2 text-sm font-medium text-ink-2">
              {t('plan.morph.changes')}
            </h3>
            {preview.changes.length === 0 ? (
              <p className="rounded-2xl bg-track/50 p-3 text-sm text-ink-3">{t('plan.morph.noChange')}</p>
            ) : (
              <ol className="space-y-2">
                {preview.changes.map((c, i) => (
                  <li key={`${c.field}-${c.stepIndex ?? 'all'}-${i}`} className="glass-inset p-3">
                    <p className="text-xs font-semibold text-ink">{changeLabel(c, steps)}</p>
                    <dl className="mt-1.5 grid grid-cols-2 gap-3">
                      <div className="min-w-0">
                        <dt className="text-[11px] text-ink-3">{t('plan.morph.planned')}</dt>
                        <dd className="num text-sm break-words text-ink-3">{c.before}</dd>
                      </div>
                      <div className="min-w-0">
                        <dt className="text-[11px] text-ink-3">{t('plan.morph.adaptedCol')}</dt>
                        <dd className="num text-sm font-semibold break-words text-ink">{c.after}</dd>
                      </div>
                    </dl>
                    <p className="mt-1.5 text-xs text-ink-2">{c.reason}</p>
                  </li>
                ))}
              </ol>
            )}
          </section>

          {llmOn && preview.proposal && (
            <section aria-labelledby="morph-ai" className="rounded-2xl bg-track/50 p-3.5">
              <h3 id="morph-ai" className="flex items-center gap-2 text-sm font-semibold text-ink">
                <Sparkles className="size-4 text-accent" aria-hidden="true" /> {t('plan.morph.refine')}
              </h3>
              <p className="mt-0.5 text-xs text-ink-3">{t('plan.morph.refineHint')}</p>
              <label className="mt-2.5 block">
                <span className="sr-only">{t('plan.morph.refineNote')}</span>
                <input className="field" value={note} onChange={(e) => setNote(e.target.value)} maxLength={500} placeholder={t('plan.morph.refinePlaceholder')} disabled={streaming} />
              </label>
              <button type="button" className="btn btn-ghost mt-2" onClick={refine} disabled={streaming || m.refine.isPending}>
                {streaming || m.refine.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Sparkles className="size-4" aria-hidden="true" />}
                {streaming ? t('plan.morph.refining') : t('plan.morph.refine')}
              </button>
              {stream?.text && (streaming || stream.done) && (
                <p className="mt-2.5 text-sm whitespace-pre-line text-ink-2" aria-live="polite">
                  {stream.text}
                </p>
              )}
            </section>
          )}
          {error && <InlineError>{error}</InlineError>}
        </div>
      )}
    </Dialog>
  )
}
