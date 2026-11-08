import { useMemo, useState } from 'react'
import { useReport } from '../../api/queries'
import type { IncludeKey } from '../../api/types'
import { useStoredState } from '../../hooks/useStoredState'
import { estimateTokens, fmtDateShort } from '../../lib/format'
import { ALL_INCLUDE, effectiveInclude, parseInclude, serializeInclude } from '../../lib/include'
import { STORAGE_KEYS } from '../../lib/storage'
import { useReportExport, type ReportPayload } from '../export/useReportExport'
import { t } from '../../i18n'

export type PromptSubjectKind = 'activity' | 'day'

export interface PromptTarget {
  activity: { id: number; name: string; date: string } | null
  day: string
}

const parseKind = (raw: string) => (raw === 'day' || raw === 'activity' ? raw : undefined)

/** Prompt generator state shared by the card and the mobile bar: subject, persisted chips, report query, export actions. */
export function usePrompt(target: PromptTarget) {
  const [kindPref, setKind] = useStoredState<PromptSubjectKind>(STORAGE_KEYS.promptSubject, 'activity', parseKind)
  const [include, setInclude] = useStoredState<IncludeKey[]>(STORAGE_KEYS.include, ALL_INCLUDE, parseInclude, serializeInclude)
  const [previewOpen, setPreviewOpen] = useState(false)
  const kind: PromptSubjectKind = target.activity ? kindPref : 'day'
  const keys = effectiveInclude(include, kind)
  const subject = kind === 'activity' && target.activity ? ({ kind: 'activity', id: target.activity.id } as const) : ({ kind: 'day', date: target.day } as const)
  const report = useReport(subject, keys)
  const stale = report.isPlaceholderData || (report.isFetching && !report.data)
  const text = report.data ?? null

  const payload: ReportPayload | null = useMemo(() => {
    if (!text || stale) return null
    const tokens = estimateTokens(text)
    if (kind === 'activity' && target.activity) {
      return { report: text, tokens, filename: t('coach.prompt.sessionFile', { date: target.activity.date }), title: `Stride · ${target.activity.name}` }
    }
    return { report: text, tokens, filename: t('coach.prompt.dayFile', { date: target.day }), title: t('coach.prompt.dayTitle', { date: fmtDateShort(target.day) }) }
  }, [text, stale, kind, target.activity, target.day])

  const exporter = useReportExport(payload, () => setPreviewOpen(true))
  const toggle = (k: IncludeKey, on: boolean) => setInclude(on ? [...include, k] : include.filter((x) => x !== k))
  const sections = useMemo(() => (text ? text.split('\n').filter((l) => l.startsWith('### ')).map((l) => l.slice(4).replace(/\s*-\s*\[?\d.*$/, '').trim()) : []), [text])

  return {
    kind,
    setKind,
    canSession: target.activity !== null,
    include,
    toggle,
    setInclude,
    report,
    text,
    stale,
    payload,
    tokens: estimateTokens(text),
    sections,
    exporter,
    previewOpen,
    setPreviewOpen,
  }
}

export type PromptState = ReturnType<typeof usePrompt>
