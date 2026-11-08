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
import { t } from '../../i18n'

interface AiProps {
  p: PromptState
  className?: string
  llmConfigured?: boolean
  onAnalyze?: () => void
  /** Dashboard variant: the two actions only, options live in Coach IA. */
  compact?: boolean
}

/** Light AI panel: copy the prompt (always) and analyse with the user's LLM (when configured); prompt options folded. */
export const PromptGenerator = forwardRef<HTMLElement, AiProps>(function PromptGenerator({ p, className, llmConfigured, onAnalyze, compact }, ref) {
  const busy = p.stale || p.report.isPending
  const [options, setOptions] = useState(false)
  return (
    <section ref={ref} className={cn('min-w-0', className)} aria-label={t('coach.prompt.title')}>
      <Card
        title={t('coach.prompt.title')}
        icon={<Sparkles className="size-4" />}
        action={
          compact ? (
            <Link href="/coach" className="text-xs font-semibold text-ink-3 hover:text-accent">
              {t('coach.prompt.options')}
            </Link>
          ) : (
            <span className="num text-xs text-ink-3" aria-live="polite">
              {busy ? <Loader2 className="mr-1 inline size-3.5 animate-spin" aria-hidden="true" /> : null}
              {t('coach.prompt.tokens', { count: fmtNumber(p.tokens) })}
            </span>
          )
        }
      >
        {!compact && (
        <SegmentedControl
          ariaLabel={t('coach.prompt.subject')}
          size="sm"
          className="mb-3"
          value={p.kind}
          onChange={(k) => p.setKind(k)}
          options={[
            { value: 'activity', label: t('coach.prompt.session'), icon: <Footprints /> },
            { value: 'day', label: t('coach.prompt.dayReview'), icon: <CalendarDays /> },
          ]}
        />
        )}
        {!compact && !p.canSession && <p className="-mt-1 mb-3 text-xs text-ink-3">{t('coach.prompt.noSession')}</p>}
        {p.report.isError && <InlineError className="mb-3">{p.report.error.message}</InlineError>}

        <div className="grid gap-2">
          <button type="button" className="btn btn-primary h-12 w-full text-[0.95rem]" onClick={() => void p.exporter.copy()} disabled={!p.payload}>
            {busy ? <Loader2 className="size-[1.125rem] animate-spin" aria-hidden="true" /> : <Copy className="size-[1.125rem]" aria-hidden="true" />}
            {t('coach.prompt.copy')}
          </button>
          {llmConfigured ? (
            <button type="button" className="btn btn-ghost h-11 w-full" onClick={onAnalyze} disabled={busy}>
              <Bot className="size-[1.125rem] text-accent" aria-hidden="true" />
              {t('coach.prompt.analyze')}
            </button>
          ) : compact ? null : (
            <p className="text-center text-xs text-ink-3">
              <Link href="/settings#ia" className="underline underline-offset-2 hover:text-ink-2">
                {t('coach.prompt.connectAi')}
              </Link>{' '}
              {t('coach.prompt.connectAiHint')}
            </p>
          )}
        </div>

        {!compact && (
        <>
        <button type="button" className="mt-3 flex w-full items-center justify-between gap-2 py-1 text-xs font-medium text-ink-3 hover:text-ink-2" aria-expanded={options} aria-controls="prompt-options" onClick={() => setOptions((o) => !o)}>
          {t('coach.prompt.content', { count: p.include.length })}
          <ChevronDown className={cn('size-4 transition-transform', options && 'rotate-180')} aria-hidden="true" />
        </button>
        <div id="prompt-options" hidden={!options} className="pt-2">
          <div className="flex flex-wrap gap-1.5" role="group" aria-label={t('coach.prompt.sectionsLabel')}>
            {INCLUDE_CHIPS.map((c) => {
              const off = p.kind === 'day' && c.scope === 'session'
              return (
                <ChipToggle key={c.key} pressed={p.include.includes(c.key)} onChange={(on) => p.toggle(c.key, on)} disabled={off} title={off ? t('coach.prompt.notInDay') : c.hint}>
                  {c.label}
                </ChipToggle>
              )
            })}
          </div>
          <p className="mt-2 flex items-start gap-1.5 text-xs text-ink-3">
            <ShieldCheck className="mt-px size-3.5 flex-none text-accent" aria-hidden="true" />
            {t('coach.prompt.directive')}
          </p>
          <div className={cn('mt-3 grid gap-2', p.exporter.canShare ? 'grid-cols-3' : 'grid-cols-2')}>
            <button type="button" className="btn btn-ghost px-2" onClick={() => p.setPreviewOpen(true)} disabled={!p.text}>
              <Eye className="size-4" aria-hidden="true" /> {t('coach.prompt.preview')}
            </button>
            <button type="button" className="btn btn-ghost px-2" onClick={p.exporter.download} disabled={!p.payload} aria-label={t('coach.prompt.downloadMd')}>
              <Download className="size-4" aria-hidden="true" /> .md
            </button>
            {p.exporter.canShare && (
              <button type="button" className="btn btn-ghost px-2" onClick={() => void p.exporter.share()} disabled={!p.payload} aria-label={t('coach.prompt.share')}>
                <Share2 className="size-4" aria-hidden="true" />
              </button>
            )}
          </div>
        </div>
        </>
        )}
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
