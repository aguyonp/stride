import { Copy, Download, Share2 } from 'lucide-react'
import { Fragment, useMemo, type ReactNode } from 'react'
import { fmtNumber } from '../../lib/format'
import { Dialog } from '../ui/Dialog'
import type { ReportPayload } from './useReportExport'

/** Light markdown highlighting (headings, bold, tables, directive). */
function renderLine(line: string, i: number): ReactNode {
  if (line.startsWith('###')) {
    return (
      <span key={i} className="font-semibold text-accent">
        {line}
      </span>
    )
  }
  const isTable = line.startsWith('|')
  const quote = line.startsWith('>')
  const parts = line.split(/(\*\*[^*]+\*\*|\|)/g).filter((p) => p !== '')
  return (
    <span key={i} className={isTable ? 'text-ink-2' : quote ? 'text-ink-3 italic' : undefined}>
      {parts.map((p, j) =>
        p === '|' ? (
          <span key={j} className="text-ink-3/70">
            |
          </span>
        ) : p.startsWith('**') ? (
          <span key={j} className="font-semibold text-ink not-italic">
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
  onCopy: () => void
  onDownload: () => void
  onShare: () => void
  canShare: boolean
  refreshing?: boolean
  title?: string
  extra?: ReactNode
}

export function ReportPreview({ open, onClose, payload, onCopy, onDownload, onShare, canShare, refreshing, title = 'Aperçu du prompt', extra }: ReportPreviewProps) {
  const lines = useMemo(() => (payload?.report ?? '').split('\n'), [payload?.report])
  return (
    <Dialog
      open={open}
      onClose={onClose}
      variant="drawer"
      title={title}
      description={payload ? `≈ ${fmtNumber(payload.tokens)} tokens · ${fmtNumber(payload.report.length)} caractères · ${payload.filename}` : undefined}
      bodyClassName="px-0 py-0"
      footer={
        <div className="space-y-3">
          {extra}
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
        tabIndex={0}
        aria-label="Texte du prompt"
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
