import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Bot, Copy, History, Loader2, RefreshCw, Settings } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Link } from 'wouter'
import { api, isApiError } from '../../api/client'
import { useAnalyses, useLlmConfig } from '../../api/queriesV3'
import type { IncludeKey, LLMAnalysis, LLMAnalyzeRequest } from '../../api/types'
import { copyText } from '../../lib/clipboard'
import { cn } from '../../lib/cn'
import { fmtDateTime, fmtNumber } from '../../lib/format'
import { clearLlmStream, isJobStatus, startLlmStream, useLlmStream } from '../../state/events'
import { Dialog } from '../ui/Dialog'
import { Markdown } from '../ui/Markdown'
import { InlineError } from '../ui/Misc'
import { Skeleton } from '../ui/Skeleton'
import { useToast } from '../ui/Toast'

export type AnalysisSubject = { kind: 'activity'; activityId: number; label: string } | { kind: 'day'; date: string; label: string }

export const PROVIDER_LABEL: Record<string, string> = {
  gemini: 'Gemini',
  anthropic: 'Claude (Anthropic)',
  openai: 'OpenAI',
  openai_compat: 'Compatible OpenAI',
}

const refOf = (s: AnalysisSubject) => (s.kind === 'activity' ? String(s.activityId) : s.date)

/**
 * Drawer that runs an AI coach analysis and renders live SSE tokens (`llm.token` / `llm.done`)
 * or synchronous responses seamlessly, alongside past analyses for the same subject.
 */
export function AnalysisDrawer({
  open,
  onClose,
  subject,
  include,
  runToken,
}: {
  open: boolean
  onClose: () => void
  subject: AnalysisSubject
  include: IncludeKey[]
  runToken: number
}) {
  const qc = useQueryClient()
  const llm = useLlmConfig()
  const ref = refOf(subject)
  const history = useAnalyses(subject.kind, ref, open)
  const { toast } = useToast()
  const [selected, setSelected] = useState<number | null>(null)
  const [question, setQuestion] = useState('')
  const [streamJobId, setStreamJobId] = useState<string | null>(null)
  const [directResult, setDirectResult] = useState<LLMAnalysis | null>(null)

  const liveStream = useLlmStream(streamJobId, subject.kind, ref)

  const analyze = useMutation({
    mutationFn: (body: LLMAnalyzeRequest) => api.analyzeStream(body, true),
    onSuccess: (res) => {
      if (isJobStatus(res)) {
        setStreamJobId(res.id)
        startLlmStream(res.id, subject.kind, ref)
        return
      }
      setStreamJobId(null)
      setDirectResult(res)
      setSelected(res.id)
      qc.setQueryData<LLMAnalysis[]>(
        ['llm', 'analyses', res.kind, res.ref],
        (list) => [res, ...(list ?? []).filter((x) => x.id !== res.id)].slice(0, 20),
      )
    },
  })

  const run = (q?: string) => {
    setSelected(null)
    setDirectResult(null)
    clearLlmStream(streamJobId, subject.kind, ref)
    setStreamJobId(null)
    const body: LLMAnalyzeRequest =
      subject.kind === 'activity' ? { kind: 'activity', activityId: subject.activityId, include } : { kind: 'day', date: subject.date, include }
    if (q?.trim()) body.question = q.trim()
    analyze.mutate(body)
  }

  useEffect(() => {
    if (runToken > 0) run()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runToken])

  useEffect(() => {
    if (liveStream?.done && liveStream.analysis) {
      setSelected(liveStream.analysis.id)
      setDirectResult(liveStream.analysis)
    }
  }, [liveStream?.done, liveStream?.analysis])

  const isStreaming = Boolean(streamJobId && (!liveStream || !liveStream.done))
  const busy = analyze.isPending || isStreaming
  const streamText = liveStream?.text ?? ''
  const streamError = liveStream?.done ? liveStream.error : null

  const list = history.data ?? []
  const current: LLMAnalysis | null = busy
    ? null
    : (list.find((a) => a.id === selected) ??
      (directResult && ref === directResult.ref ? directResult : null) ??
      (liveStream?.analysis && ref === liveStream.analysis.ref ? liveStream.analysis : null) ??
      list[0] ??
      null)

  const notConnected = isApiError(analyze.error, 'PROVIDER_NOT_CONNECTED')
  const cfg = llm.data

  return (
    <Dialog
      open={open}
      onClose={onClose}
      variant="drawer"
      title="Analyse IA"
      description={`${subject.label}${cfg?.configured ? ` · ${PROVIDER_LABEL[cfg.provider ?? ''] ?? cfg.provider} · ${cfg.model}` : ''}`}
      footer={
        <form
          className="flex gap-2 pb-3"
          onSubmit={(e) => {
            e.preventDefault()
            run(question)
            setQuestion('')
          }}
        >
          <label className="sr-only" htmlFor="ai-question">
            Question complémentaire (facultatif)
          </label>
          <input
            id="ai-question"
            className="field min-w-0 flex-1"
            placeholder="Question complémentaire (facultatif)"
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            maxLength={500}
          />
          <button type="submit" className="btn btn-primary flex-none" disabled={busy || !cfg?.configured}>
            {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <RefreshCw className="size-4" aria-hidden="true" />}
            <span className="max-sm:sr-only">{list.length || current ? 'Relancer' : 'Analyser'}</span>
          </button>
        </form>
      }
    >
      <div aria-live="polite" aria-busy={busy}>
        {busy && (
          <div className="space-y-3">
            <p className="flex items-center gap-2 text-sm text-ink-2">
              <Loader2 className="size-4 animate-spin text-accent" aria-hidden="true" />
              {streamText ? 'Rédaction en direct par ton coach IA…' : 'Analyse en cours… préparation du contexte physiologique.'}
            </p>
            {streamText ? (
              <article aria-label="Flux de l’analyse en cours" className="rounded-2xl border border-accent/25 bg-track/35 p-3.5">
                <Markdown text={streamText} />
                <span className="mt-1 inline-block h-4 w-1.5 animate-pulse rounded-xs bg-accent align-middle" aria-hidden="true" />
              </article>
            ) : (
              <>
                <Skeleton className="h-4 w-2/3" />
                <Skeleton className="h-3 w-full" />
                <Skeleton className="h-3 w-5/6" />
                <Skeleton className="h-3 w-3/4" />
              </>
            )}
          </div>
        )}
        {(analyze.isError || streamError) && (
          <InlineError className="mb-4">
            {streamError ?? analyze.error?.message}
            {notConnected && (
              <>
                {' '}
                <Link href="/settings#ia" className="font-semibold underline" onClick={onClose}>
                  Configurer mon IA
                </Link>
              </>
            )}
          </InlineError>
        )}
        {!busy && current && (
          <article aria-label="Résultat de l’analyse">
            <div className="mb-3 flex flex-wrap items-center justify-between gap-2 text-xs text-ink-3">
              <span className="num">
                {fmtDateTime(current.createdAt)} · {current.model} · {fmtNumber(current.durationMs / 1000, 1)} s · ≈ {fmtNumber(current.promptTokensEstimate)} tokens envoyés
              </span>
              <button
                type="button"
                className="btn btn-ghost h-8 px-2.5 text-xs"
                onClick={async () => toast((await copyText(current.markdown)) ? { title: 'Analyse copiée', tone: 'success' } : { title: 'Copie impossible', tone: 'error' })}
              >
                <Copy className="size-3.5" aria-hidden="true" /> Copier
              </button>
            </div>
            <Markdown text={current.markdown} />
          </article>
        )}
        {!busy && !current && !analyze.isError && !streamError && (
          <div className="grid place-items-center gap-2 py-10 text-center text-sm text-ink-3">
            <Bot className="size-8 text-ink-3" aria-hidden="true" />
            {history.isPending ? 'Chargement…' : cfg?.configured ? 'Aucune analyse pour l’instant.' : 'Aucune IA configurée.'}
            {!cfg?.configured && llm.isSuccess && (
              <Link href="/settings#ia" className="btn btn-ghost mt-1" onClick={onClose}>
                <Settings className="size-4" aria-hidden="true" /> Réglages › IA
              </Link>
            )}
          </div>
        )}

        {list.length > 1 && (
          <section className="mt-6 border-t border-line pt-4" aria-labelledby="ai-history">
            <h3 id="ai-history" className="mb-2 flex items-center gap-1.5 text-xs font-semibold text-ink-3">
              <History className="size-3.5" aria-hidden="true" /> Analyses précédentes
            </h3>
            <ul className="space-y-1">
              {list.map((a) => (
                <li key={a.id}>
                  <button
                    type="button"
                    onClick={() => {
                      setStreamJobId(null)
                      setSelected(a.id)
                    }}
                    aria-current={current?.id === a.id ? 'true' : undefined}
                    className={cn(
                      'flex w-full items-center justify-between gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-track',
                      current?.id === a.id ? 'bg-track text-ink' : 'text-ink-2',
                    )}
                  >
                    <span className="num">{fmtDateTime(a.createdAt)}</span>
                    <span className="truncate text-xs text-ink-3">{a.model}</span>
                  </button>
                </li>
              ))}
            </ul>
          </section>
        )}
      </div>
    </Dialog>
  )
}
