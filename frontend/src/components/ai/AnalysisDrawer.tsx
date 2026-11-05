import { Bot, Copy, History, Loader2, RefreshCw, Settings } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Link } from 'wouter'
import { isApiError } from '../../api/client'
import { useAnalyses, useAnalyze, useLlmConfig } from '../../api/queriesV3'
import type { IncludeKey, LLMAnalysis, LLMAnalyzeRequest } from '../../api/types'
import { copyText } from '../../lib/clipboard'
import { cn } from '../../lib/cn'
import { fmtDateTime, fmtNumber } from '../../lib/format'
import { Dialog } from '../ui/Dialog'
import { Markdown } from '../ui/Markdown'
import { InlineError } from '../ui/Misc'
import { Skeleton } from '../ui/Skeleton'
import { useToast } from '../ui/Toast'

export type AnalysisSubject = { kind: 'activity'; activityId: number; label: string } | { kind: 'day'; date: string; label: string }

export const PROVIDER_LABEL: Record<string, string> = { gemini: 'Gemini', anthropic: 'Claude (Anthropic)', openai: 'OpenAI', openai_compat: 'Compatible OpenAI' }

const refOf = (s: AnalysisSubject) => (s.kind === 'activity' ? String(s.activityId) : s.date)

/** Drawer that runs an analysis with the user's LLM and renders the Markdown answer, with past analyses for the same subject. */
export function AnalysisDrawer({ open, onClose, subject, include, runToken }: { open: boolean; onClose: () => void; subject: AnalysisSubject; include: IncludeKey[]; runToken: number }) {
  const llm = useLlmConfig()
  const history = useAnalyses(subject.kind, refOf(subject), open)
  const analyze = useAnalyze()
  const { toast } = useToast()
  const [selected, setSelected] = useState<number | null>(null)
  const [question, setQuestion] = useState('')

  const run = (q?: string) => {
    setSelected(null)
    const body: LLMAnalyzeRequest = subject.kind === 'activity' ? { kind: 'activity', activityId: subject.activityId, include } : { kind: 'day', date: subject.date, include }
    if (q?.trim()) body.question = q.trim()
    analyze.mutate(body, { onSuccess: (a) => setSelected(a.id) })
  }

  useEffect(() => {
    if (runToken > 0) run()
  }, [runToken])

  const list = history.data ?? []
  const current: LLMAnalysis | null = analyze.isPending ? null : (list.find((a) => a.id === selected) ?? (analyze.data && refOf(subject) === analyze.data.ref ? analyze.data : null) ?? list[0] ?? null)
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
          <input id="ai-question" className="field min-w-0 flex-1" placeholder="Question complémentaire (facultatif)" value={question} onChange={(e) => setQuestion(e.target.value)} maxLength={500} />
          <button type="submit" className="btn btn-primary flex-none" disabled={analyze.isPending || !cfg?.configured}>
            {analyze.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <RefreshCw className="size-4" aria-hidden="true" />}
            <span className="max-sm:sr-only">{list.length || analyze.data ? 'Relancer' : 'Analyser'}</span>
          </button>
        </form>
      }
    >
      <div aria-live="polite" aria-busy={analyze.isPending}>
        {analyze.isPending && (
          <div className="space-y-3">
            <p className="flex items-center gap-2 text-sm text-ink-2">
              <Loader2 className="size-4 animate-spin text-accent" aria-hidden="true" />
              Analyse en cours… le fournisseur peut mettre jusqu’à 2 minutes.
            </p>
            <Skeleton className="h-4 w-2/3" />
            <Skeleton className="h-3 w-full" />
            <Skeleton className="h-3 w-5/6" />
            <Skeleton className="h-3 w-3/4" />
          </div>
        )}
        {analyze.isError && (
          <InlineError className="mb-4">
            {analyze.error.message}
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
        {!analyze.isPending && current && (
          <article aria-label="Résultat de l’analyse">
            <div className="mb-3 flex flex-wrap items-center justify-between gap-2 text-xs text-ink-3">
              <span>
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
        {!analyze.isPending && !current && !analyze.isError && (
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
            <h3 id="ai-history" className="label-caps mb-2 flex items-center gap-1.5">
              <History className="size-3.5" aria-hidden="true" /> Analyses précédentes
            </h3>
            <ul className="space-y-1">
              {list.map((a) => (
                <li key={a.id}>
                  <button
                    type="button"
                    onClick={() => setSelected(a.id)}
                    aria-current={current?.id === a.id ? 'true' : undefined}
                    className={cn('flex w-full items-center justify-between gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-track', current?.id === a.id ? 'bg-track text-ink' : 'text-ink-2')}
                  >
                    <span>{fmtDateTime(a.createdAt)}</span>
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
