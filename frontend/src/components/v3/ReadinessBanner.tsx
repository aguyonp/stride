import { CircleCheck, CircleHelp, OctagonAlert, TriangleAlert } from 'lucide-react'
import type { ReadinessBanner as Readiness } from '../../api/types'
import { cn } from '../../lib/cn'
import { t } from '../../i18n'
import { fmtNumber } from '../../lib/format'

const TONE = {
  green: { cls: 'border-ok/25 bg-ok/[0.07]', text: 'text-ok', Icon: CircleCheck, get label() { return t('dashboard.readiness.tone.green') } },
  orange: { cls: 'border-warn/30 bg-warn/[0.08]', text: 'text-warn', Icon: TriangleAlert, get label() { return t('dashboard.readiness.tone.orange') } },
  red: { cls: 'border-danger/30 bg-danger/[0.09]', text: 'text-danger', Icon: OctagonAlert, get label() { return t('dashboard.readiness.tone.red') } },
  unknown: { cls: 'border-line bg-track/50', text: 'text-ink-3', Icon: CircleHelp, get label() { return t('dashboard.readiness.tone.unknown') } },
} as const

/** Readiness traffic light: level + title + FR reasons, with the few numbers behind it; `brief` keeps one line. */
export function ReadinessBanner({ r, label = t('dashboard.readiness.label'), compact, brief, className }: { r: Readiness | null | undefined; label?: string; compact?: boolean; brief?: boolean; className?: string }) {
  if (!r) return null
  const tone = TONE[r.level] ?? TONE.unknown
  if (brief) {
    return (
      <section aria-label={label} className={cn('animate-in flex items-center gap-2.5 rounded-[var(--radius-glass)] border px-4 py-2.5', tone.cls, className)}>
        <tone.Icon className={cn('size-5 flex-none', tone.text)} aria-hidden="true" />
        <p className="min-w-0 truncate text-sm">
          <span className="font-semibold text-ink">{r.title}</span>
          {r.reasons[0] && <span className="text-ink-3"> · {r.reasons[0]}</span>}
        </p>
      </section>
    )
  }
  const facts: [string, string][] = []
  if (r.hrv7dAvg !== null) facts.push([t('dashboard.readiness.hrv7d'), `${fmtNumber(r.hrv7dAvg)} ms${r.hrvBaselineLow !== null ? t('dashboard.readiness.base', { value: fmtNumber(r.hrvBaselineLow) }) : ''}`])
  if (r.sleep7dAvg !== null) facts.push([t('dashboard.readiness.sleep7d'), `${fmtNumber(r.sleep7dAvg)}/100`])
  if (r.loadRatio !== null) facts.push([t('dashboard.readiness.loadRatio'), fmtNumber(r.loadRatio, 2)])
  return (
    <section aria-label={label} className={cn('animate-in rounded-[var(--radius-glass)] border px-4 py-3 sm:px-5', tone.cls, className)}>
      <div className="flex flex-wrap items-start gap-x-3 gap-y-2">
        <tone.Icon className={cn('mt-0.5 size-5 flex-none', tone.text)} aria-hidden="true" />
        <div className="min-w-0 flex-1 basis-56">
          <p className="text-xs text-ink-3">
            {label}
            {!r.title.toLowerCase().startsWith(tone.label.toLowerCase()) && (
              <>
                {' · '}
                <span className={cn('font-semibold', tone.text)}>{tone.label}</span>
              </>
            )}
          </p>
          <p className="font-semibold tracking-tight text-ink">{r.title}</p>
          {r.reasons.length > 0 && (
            <ul className={cn('mt-1 text-sm text-ink-2', compact ? 'space-y-0.5' : 'flex flex-wrap gap-x-4 gap-y-0.5')}>
              {r.reasons.map((x) => (
                <li key={x} className="flex gap-1.5">
                  <span aria-hidden="true" className={tone.text}>
                    •
                  </span>
                  {x}
                </li>
              ))}
            </ul>
          )}
        </div>
        {!compact && facts.length > 0 && (
          <dl className="flex flex-none flex-wrap gap-x-4 gap-y-1 text-xs max-sm:hidden sm:text-right">
            {facts.map(([k, v]) => (
              <div key={k}>
                <dt className="text-ink-3">{k}</dt>
                <dd className="num font-medium text-ink-2">{v}</dd>
              </div>
            ))}
          </dl>
        )}
      </div>
    </section>
  )
}
