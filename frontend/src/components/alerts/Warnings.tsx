import { Info } from 'lucide-react'
import { cn } from '../../lib/cn'
import { t, tx } from '../../i18n'

export function Warnings({ warnings, sources, className }: { warnings: string[]; sources?: string[]; className?: string }) {
  if (!warnings.length && !sources?.length) return null
  return (
    <div className={cn('flex flex-wrap items-center gap-1.5', className)}>
      {warnings.map((w) => (
        <span key={w} className="chip border border-line bg-track text-ink-2" title={t('coach.prompt.sourceNa')}>
          <Info className="size-3.5 text-ink-3" aria-hidden="true" />
          {w}
        </span>
      ))}
      {sources && sources.length > 0 && (
        <span className="px-1 text-[11px] text-ink-3">
          {tx('coach.prompt.sources', { sources: <span className="font-medium">{sources.join(' · ')}</span> })}
        </span>
      )}
    </div>
  )
}
