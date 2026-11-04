import { Download, Eye, Share2, Sparkles } from 'lucide-react'
import { cn } from '../../lib/cn'
import { fmtNumber } from '../../lib/format'
import { Switch } from '../ui/Switch'
import type { ReportPayload } from './useReportExport'

interface ExportPanelProps {
  payload: ReportPayload | null
  loading: boolean
  withHistory: boolean
  onToggleHistory: (v: boolean) => void
  onCopy: () => void
  onPreview: () => void
  onDownload: () => void
  onShare: () => void
  canShare: boolean
  className?: string
}

/** Carte d'export (desktop, colonne latérale). */
export function ExportPanel({ payload, loading, withHistory, onToggleHistory, onCopy, onPreview, onDownload, onShare, canShare, className }: ExportPanelProps) {
  return (
    <section className={cn('glass animate-in p-4', className)} aria-label="Export pour l'IA">
      <div className="mb-3 flex items-baseline justify-between gap-2">
        <h2 className="text-[0.95rem] font-semibold tracking-tight">Rapport pour l’IA</h2>
        <span className="num text-xs text-ink-3">{payload ? `≈ ${fmtNumber(payload.tokens)} tokens` : '—'}</span>
      </div>
      <button type="button" className="btn btn-primary h-12 w-full text-[0.95rem]" onClick={onCopy} disabled={!payload || loading}>
        <Sparkles className="size-[1.125rem]" aria-hidden="true" />
        Exporter pour l’IA
      </button>
      <div className={cn('mt-2 grid gap-2', canShare ? 'grid-cols-3' : 'grid-cols-2')}>
        <button type="button" className="btn btn-ghost px-2" onClick={onPreview} disabled={!payload}>
          <Eye className="size-4" aria-hidden="true" /> Aperçu
        </button>
        <button type="button" className="btn btn-ghost px-2" onClick={onDownload} disabled={!payload} aria-label="Télécharger le rapport en .md">
          <Download className="size-4" aria-hidden="true" /> .md
        </button>
        {canShare && (
          <button type="button" className="btn btn-ghost px-2" onClick={onShare} disabled={!payload} aria-label="Partager le rapport">
            <Share2 className="size-4" aria-hidden="true" />
          </button>
        )}
      </div>
      <Switch compact className="mt-3.5 border-t border-line pt-3.5" checked={withHistory} onChange={onToggleHistory} label="Inclure l’historique 14 j" description="Tableau compact des 14 derniers jours" />
    </section>
  )
}
