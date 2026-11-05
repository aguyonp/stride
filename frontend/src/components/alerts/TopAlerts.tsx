import { ChevronDown, ChevronRight, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import type { Alert, AlertLevel } from '../../api/types'
import { cn } from '../../lib/cn'
import { LEVEL_LABEL, LevelIcon } from '../ui/Badge'

const ORDER: Record<AlertLevel, number> = { danger: 0, warn: 1, info: 2, ok: 3 }

/** Top N alerts (danger first), the rest behind « voir plus »; an alert linked to a section opens it. */
export function TopAlerts({ alerts, max = 3, sectionOf, onOpen }: { alerts: Alert[]; max?: number; sectionOf: (code: string) => string | null; onOpen: (section: string) => void }) {
  const [expanded, setExpanded] = useState(false)
  const all = [...alerts].filter((a) => a.level !== 'ok').sort((a, b) => ORDER[a.level] - ORDER[b.level])
  const serious = all.filter((a) => a.level !== 'info')
  if (serious.length === 0) {
    const ok = alerts.find((a) => a.level === 'ok')
    return (
      <section aria-label="Signaux" className="section-group flex items-center gap-3 px-4 py-3 sm:px-5">
        <ShieldCheck className="size-5 flex-none text-ok" aria-hidden="true" />
        <p className="min-w-0 text-sm text-ink-2">
          <span className="font-semibold text-ink">{ok?.title ?? 'Aucun signal d’alerte'}</span>
          {all.length > 0 && <span className="text-ink-3"> · {all.length} info{all.length > 1 ? 's' : ''}</span>}
        </p>
      </section>
    )
  }
  const visible = expanded ? all : all.slice(0, max)
  const rest = all.length - Math.min(max, all.length)
  return (
    <section aria-labelledby="top-alerts-title" className="section-group overflow-hidden">
      <h2 id="top-alerts-title" className="sr-only">
        Signaux à surveiller
      </h2>
      <ul>
        {visible.map((a, i) => {
          const target = sectionOf(a.code)
          const content = (
            <>
              <LevelIcon level={a.level} className="mt-0.5 size-[1.05rem] flex-none" />
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-semibold text-ink">
                  {a.title}
                  <span className="sr-only"> — niveau {LEVEL_LABEL[a.level]}</span>
                </span>
                <span className="block text-sm text-ink-3">{a.message}</span>
              </span>
              {target && <ChevronRight className="mt-0.5 size-4 flex-none text-ink-3" aria-hidden="true" />}
            </>
          )
          return (
            <li key={`${a.code}-${i}`} className={cn(i > 0 && 'border-t border-line')}>
              {target ? (
                <button type="button" onClick={() => onOpen(target)} className="flex w-full items-start gap-3 px-4 py-3 text-left hover:bg-white/[0.03] focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent sm:px-5" aria-label={`${a.title} : voir le détail`}>
                  {content}
                </button>
              ) : (
                <div className="flex items-start gap-3 px-4 py-3 sm:px-5">{content}</div>
              )}
            </li>
          )
        })}
      </ul>
      {rest > 0 && (
        <button type="button" onClick={() => setExpanded((v) => !v)} aria-expanded={expanded} className="flex w-full items-center justify-center gap-1.5 border-t border-line py-2.5 text-sm font-medium text-ink-2 hover:text-ink">
          {expanded ? 'Réduire' : `Voir plus (${rest})`}
          <ChevronDown className={cn('size-4 transition-transform', expanded && 'rotate-180')} aria-hidden="true" />
        </button>
      )}
    </section>
  )
}
