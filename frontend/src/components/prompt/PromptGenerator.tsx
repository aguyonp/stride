import { Bot, CalendarDays, ChevronDown, Copy, Download, Eye, Footprints, Loader2, Share2, ShieldCheck, Sparkles } from 'lucide-react'
import { forwardRef, useState } from 'react'
import { Link } from 'wouter'
import { cn } from '../../lib/cn'
import { fmtNumber } from '../../lib/format'
import { INCLUDE_CHIPS } from '../../lib/include'
import { ReportPreview } from '../export/ReportPreview'
import { Card } from '../ui/Card'
import { ChipToggle } from '../ui/Field'
import { InlineError } from '../ui/Misc'
import { SegmentedControl } from '../ui/SegmentedControl'
import type { PromptState } from './usePrompt'

interface AiProps {
  p: PromptState
  className?: string
  llmConfigured?: boolean
  onAnalyze?: () => void
}

/** Light AI panel: copy the prompt (always) and analyse with the user's LLM (when configured); prompt options folded. */
export const PromptGenerator = forwardRef<HTMLElement, AiProps>(function PromptGenerator({ p, className, llmConfigured, onAnalyze }, ref) {
  const busy = p.stale || p.report.isPending
  const [options, setOptions] = useState(false)
  return (
    <section ref={ref} className={cn('min-w-0', className)} aria-label="Coach IA">
      <Card
        title="Coach IA"
        icon={<Sparkles className="size-4" />}
        action={
          <span className="num text-xs text-ink-3" aria-live="polite">
            {busy ? <Loader2 className="mr-1 inline size-3.5 animate-spin" aria-hidden="true" /> : null}≈ {fmtNumber(p.tokens)} tokens
          </span>
        }
      >
        <SegmentedControl
          ariaLabel="Sujet"
          size="sm"
          className="mb-3"
          value={p.kind}
          onChange={(k) => p.setKind(k)}
          options={[
            { value: 'activity', label: 'Séance', icon: <Footprints /> },
            { value: 'day', label: 'Bilan du jour', icon: <CalendarDays /> },
          ]}
        />
        {!p.canSession && <p className="-mt-1 mb-3 text-xs text-ink-3">Aucune séance disponible : le prompt porte sur le bilan du jour.</p>}
        {p.report.isError && <InlineError className="mb-3">{p.report.error.message}</InlineError>}

        <div className="grid gap-2">
          <button type="button" className="btn btn-primary h-12 w-full text-[0.95rem]" onClick={() => void p.exporter.copy()} disabled={!p.payload}>
            {busy ? <Loader2 className="size-[1.125rem] animate-spin" aria-hidden="true" /> : <Copy className="size-[1.125rem]" aria-hidden="true" />}
            Copier le prompt
          </button>
          {llmConfigured ? (
            <button type="button" className="btn btn-ghost h-11 w-full" onClick={onAnalyze} disabled={busy}>
              <Bot className="size-[1.125rem] text-accent" aria-hidden="true" />
              Analyser avec mon IA
            </button>
          ) : (
            <p className="text-center text-xs text-ink-3">
              <Link href="/settings#ia" className="underline underline-offset-2 hover:text-ink-2">
                Brancher mon IA
              </Link>{' '}
              pour obtenir l’analyse directement ici.
            </p>
          )}
        </div>

        <button type="button" className="mt-3 flex w-full items-center justify-between gap-2 py-1 text-xs font-medium text-ink-3 hover:text-ink-2" aria-expanded={options} aria-controls="prompt-options" onClick={() => setOptions((o) => !o)}>
          Contenu du prompt ({p.include.length} sections)
          <ChevronDown className={cn('size-4 transition-transform', options && 'rotate-180')} aria-hidden="true" />
        </button>
        <div id="prompt-options" hidden={!options} className="pt-2">
          <div className="flex flex-wrap gap-1.5" role="group" aria-label="Sections incluses dans le prompt">
            {INCLUDE_CHIPS.map((c) => {
              const off = p.kind === 'day' && c.scope === 'session'
              return (
                <ChipToggle key={c.key} pressed={p.include.includes(c.key)} onChange={(on) => p.toggle(c.key, on)} disabled={off} title={off ? 'Non utilisé dans le bilan du jour' : c.hint}>
                  {c.label}
                </ChipToggle>
              )
            })}
          </div>
          <p className="mt-2 flex items-start gap-1.5 text-xs text-ink-3">
            <ShieldCheck className="mt-px size-3.5 flex-none text-accent" aria-hidden="true" />
            La directive d’objectivité est toujours incluse en tête du prompt.
          </p>
          <div className={cn('mt-3 grid gap-2', p.exporter.canShare ? 'grid-cols-3' : 'grid-cols-2')}>
            <button type="button" className="btn btn-ghost px-2" onClick={() => p.setPreviewOpen(true)} disabled={!p.text}>
              <Eye className="size-4" aria-hidden="true" /> Aperçu
            </button>
            <button type="button" className="btn btn-ghost px-2" onClick={p.exporter.download} disabled={!p.payload} aria-label="Télécharger le prompt en .md">
              <Download className="size-4" aria-hidden="true" /> .md
            </button>
            {p.exporter.canShare && (
              <button type="button" className="btn btn-ghost px-2" onClick={() => void p.exporter.share()} disabled={!p.payload} aria-label="Partager le prompt">
                <Share2 className="size-4" aria-hidden="true" />
              </button>
            )}
          </div>
        </div>
      </Card>
      <ReportPreview
        open={p.previewOpen}
        onClose={() => p.setPreviewOpen(false)}
        payload={p.payload ?? (p.text ? { report: p.text, tokens: p.tokens, filename: 'stride.md', title: 'Stride' } : null)}
        refreshing={busy}
        onCopy={() => void p.exporter.copy()}
        onDownload={p.exporter.download}
        onShare={() => void p.exporter.share()}
        canShare={p.exporter.canShare}
      />
    </section>
  )
})

/** Mobile shortcut docked above the bottom nav while the generator card is off-screen. */
export function MobilePromptBar({ p, hidden, onAnalyze }: { p: PromptState; hidden: boolean; onAnalyze?: () => void }) {
  return (
    <div
      className={cn(
        'fixed inset-x-0 bottom-[calc(4.6rem+env(safe-area-inset-bottom))] z-30 px-3 transition-[opacity,transform] duration-200 lg:hidden',
        hidden ? 'pointer-events-none translate-y-3 opacity-0' : 'opacity-100',
      )}
      aria-hidden={hidden}
    >
      <div className="glass-bar mx-auto flex max-w-lg items-center gap-2 rounded-2xl p-1.5">
        <button type="button" className="btn btn-primary h-11 min-w-0 flex-1" onClick={() => void p.exporter.copy()} disabled={!p.payload} tabIndex={hidden ? -1 : 0}>
          <Copy className="size-4 flex-none" aria-hidden="true" />
          <span className="truncate">Copier le prompt</span>
          <span className="num flex-none text-xs font-medium opacity-80">≈ {fmtNumber(p.tokens)}</span>
        </button>
        {onAnalyze ? (
          <button type="button" className="icon-btn size-11 rounded-xl text-accent" onClick={onAnalyze} aria-label="Analyser avec mon IA" tabIndex={hidden ? -1 : 0}>
            <Bot className="size-5" />
          </button>
        ) : (
          <button type="button" className="icon-btn size-11 rounded-xl" onClick={() => p.setPreviewOpen(true)} disabled={!p.text} aria-label="Aperçu du prompt" tabIndex={hidden ? -1 : 0}>
            <Eye className="size-5" />
          </button>
        )}
      </div>
    </div>
  )
}
