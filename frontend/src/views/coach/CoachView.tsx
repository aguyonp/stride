import { Bot, CalendarDays, Footprints, History } from 'lucide-react'
import { lazy, Suspense, useMemo, useState } from 'react'
import { Link } from 'wouter'
import { useActivities, useProfile } from '../../api/queries'
import { useAnalyses, useLlmConfig } from '../../api/queriesV3'
import type { ActivitySummary, LLMAnalysis } from '../../api/types'
import type { AnalysisSubject } from '../../components/ai/AnalysisDrawer'
import { PromptGenerator } from '../../components/prompt/PromptGenerator'
import { usePrompt } from '../../components/prompt/usePrompt'
import { Card } from '../../components/ui/Card'
import { PageHeader } from '../../components/ui/Misc'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { fmtDateTime, todayISO } from '../../lib/format'
import { MultiExportSection } from './MultiExportSection'

const AnalysisDrawer = lazy(() => import('../../components/ai/AnalysisDrawer').then((m) => ({ default: m.AnalysisDrawer })))

const firstLine = (md: string) => md.replace(/^[#>*\s-]+/gm, '').split('\n').find((l) => l.trim()) ?? ''

function AnalysesCard({ today, latest, onOpen }: { today: string; latest: ActivitySummary | null; onOpen: (s: AnalysisSubject, run: boolean) => void }) {
  const day = useAnalyses('day', today)
  const session = useAnalyses('activity', latest ? String(latest.id) : null)
  const daySubject: AnalysisSubject = { kind: 'day', date: today, label: 'Bilan du jour' }
  const sessionSubject: AnalysisSubject | null = latest ? { kind: 'activity', activityId: latest.id, label: latest.name } : null
  const history = useMemo(
    () => [...(day.data ?? []), ...(session.data ?? [])].sort((a, b) => b.createdAt.localeCompare(a.createdAt)).slice(0, 6),
    [day.data, session.data],
  )
  const subjectOf = (a: LLMAnalysis): AnalysisSubject => (a.kind === 'activity' && sessionSubject ? sessionSubject : daySubject)

  return (
    <Card title="Analyse IA" icon={<Bot className="size-4" />}>
      <div className="grid gap-2 sm:grid-cols-2">
        <button type="button" className="btn btn-primary" onClick={() => onOpen(daySubject, true)}>
          <CalendarDays className="size-4" aria-hidden="true" /> Bilan du jour
        </button>
        {sessionSubject && (
          <button type="button" className="btn btn-ghost min-w-0" onClick={() => onOpen(sessionSubject, true)}>
            <Footprints className="size-4 flex-none" aria-hidden="true" />
            <span className="truncate">{sessionSubject.label}</span>
          </button>
        )}
      </div>
      <h3 className="mt-5 mb-2 flex items-center gap-1.5 text-xs font-semibold text-ink-3">
        <History className="size-3.5" aria-hidden="true" /> Historique
      </h3>
      {history.length === 0 ? (
        <p className="text-sm text-ink-3">Aucune analyse pour l’instant.</p>
      ) : (
        <ul className="divide-y divide-line">
          {history.map((a) => (
            <li key={a.id}>
              <button type="button" className="flex w-full min-w-0 flex-col py-2 text-left hover:text-ink" onClick={() => onOpen(subjectOf(a), false)}>
                <span className="num text-xs text-ink-3">
                  {a.kind === 'day' ? 'Bilan' : 'Séance'} · {fmtDateTime(a.createdAt)}
                </span>
                <span className="truncate text-sm text-ink-2">{firstLine(a.markdown)}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}

/** Coach IA: prompt for a session or the day, analysis and history with the user's LLM, multi-session export. */
export default function CoachView() {
  useDocumentTitle('Coach IA')
  const profile = useProfile()
  const today = todayISO(profile.data?.timezone)
  const list = useActivities()
  const items = list.data?.pages[0]?.items ?? []
  const latest = items.find((a) => a.isRunning) ?? items[0] ?? null
  const llm = useLlmConfig()
  const llmOn = llm.data?.configured === true
  const prompt = usePrompt({ activity: latest ? { id: latest.id, name: latest.name, date: latest.date } : null, day: today })
  const [ai, setAi] = useState<{ subject: AnalysisSubject; run: number; key: number } | null>(null)
  const open = (subject: AnalysisSubject, run: boolean) => setAi((cur) => ({ subject, run: run ? (cur?.run ?? 0) + 1 : 0, key: (cur?.key ?? 0) + 1 }))
  const promptSubject: AnalysisSubject =
    prompt.kind === 'activity' && latest ? { kind: 'activity', activityId: latest.id, label: latest.name } : { kind: 'day', date: today, label: 'Bilan du jour' }

  return (
    <div className="animate-in space-y-6">
      <PageHeader title="Coach IA" />
      <div className="grid gap-4 lg:grid-cols-2 lg:gap-6">
        <PromptGenerator p={prompt} llmConfigured={llmOn} onAnalyze={() => open(promptSubject, true)} />
        {llmOn ? (
          <AnalysesCard today={today} latest={latest} onOpen={open} />
        ) : (
          <Card title="Analyse IA" icon={<Bot className="size-4" />}>
            <p className="text-sm text-ink-2">Branche ton IA pour obtenir l’analyse et son historique ici.</p>
            <Link href="/settings#ia" className="btn btn-ghost mt-3">
              Configurer mon IA
            </Link>
          </Card>
        )}
      </div>
      <MultiExportSection today={today} />
      {ai && (
        <Suspense fallback={null}>
          <AnalysisDrawer key={ai.key} open onClose={() => setAi(null)} subject={ai.subject} include={prompt.include} runToken={ai.run} />
        </Suspense>
      )}
    </div>
  )
}
