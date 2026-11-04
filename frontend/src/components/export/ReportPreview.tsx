import { Copy, Download, Share2 } from 'lucide-react'
import { Fragment, useMemo, type ReactNode } from 'react'
import { fmtNumber } from '../../lib/format'
import { Dialog } from '../ui/Dialog'
import { Switch } from '../ui/Switch'
import type { ReportPayload } from './useReportExport'

/** Coloration légère du markdown (titres, gras, tableaux). */
function renderLine(line: string, i: number): ReactNode {
  if (line.startsWith('###')) {
    return (
      <span key={i} className="font-semibold text-accent">
        {line}
      </span>
    )
  }
  const isTable = line.startsWith('|')
  const parts = line.split(/(\*\*[^*]+\*\*|\|)/g).filter((p) => p !== '')
  return (
    <span key={i} className={isTable ? 'text-ink-2' : undefined}>
      {parts.map((p, j) =>
        p === '|' ? (
          <span key={j} className="text-ink-3/70">
            |
          </span>
        ) : p.startsWith('**') ? (
          <span key={j} className="font-semibold text-ink">
            {p}
          </span>
        ) : (
          <Fragment key={j}>{p}</Fragment>
        ),
      )}
    </span>
  )
}

interface ReportPreviewProps {
  open: boolean
  onClose: () => void
  payload: ReportPayload | null
  withHistory: boolean
  onToggleHistory: (v: boolean) => void
  onCopy: () => void
  onDownload: () => void
  onShare: () => void
  canShare: boolean
  refreshing?: boolean
}

export function ReportPreview({ open, onClose, payload, withHistory, onToggleHistory, onCopy, onDownload, onShare, canShare, refreshing }: ReportPreviewProps) {
  const lines = useMemo(() => (payload?.report ?? '').split('\n'), [payload?.report])
  return (
    <Dialog
      open={open}
      onClose={onClose}
      variant="drawer"
      title="Aperçu du rapport"
      description={payload ? `≈ ${fmtNumber(payload.tokens)} tokens · ${fmtNumber(payload.report.length)} caractères · ${payload.filename}` : undefined}
      bodyClassName="px-0 py-0"
      footer={
        <div className="space-y-3">
          <Switch compact checked={withHistory} onChange={onToggleHistory} label="Inclure l’historique 14 j" description="Ajoute un tableau compact des 14 derniers jours." />
          <div className="flex gap-2">
            <button type="button" className="btn btn-primary flex-1" onClick={onCopy} disabled={!payload || refreshing}>
              <Copy className="size-4" aria-hidden="true" /> Copier
            </button>
            <button type="button" className="btn btn-ghost" onClick={onDownload} disabled={!payload} aria-label="Télécharger en .md">
              <Download className="size-4" aria-hidden="true" />
              <span className="hidden sm:inline">.md</span>
            </button>
            {canShare && (
              <button type="button" className="btn btn-ghost" onClick={onShare} disabled={!payload} aria-label="Partager">
                <Share2 className="size-4" aria-hidden="true" />
              </button>
            )}
          </div>
        </div>
      }
    >
      <pre
        id="report-preview-text"
        tabIndex={0}
        aria-label="Texte du rapport"
        className={`min-h-full px-5 py-4 font-mono text-[12px] leading-[1.6] whitespace-pre-wrap text-ink-2 select-text sm:text-[12.5px] ${refreshing ? 'is-refetching' : ''}`}
        style={{ overflowWrap: 'anywhere' }}
      >
        {lines.map((l, i) => (
          <Fragment key={i}>
            {renderLine(l, i)}
            {'\n'}
          </Fragment>
        ))}
      </pre>
    </Dialog>
  )
}
