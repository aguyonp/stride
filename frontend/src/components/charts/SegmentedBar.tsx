import { cn } from '../../lib/cn'

export interface BarPart {
  key: string
  label: string
  value: number
  color: string
  display: string
}

export function SegmentedBar({ parts, ariaLabel, className }: { parts: BarPart[]; ariaLabel: string; className?: string }) {
  const total = parts.reduce((s, p) => s + Math.max(0, p.value), 0)
  return (
    <div className={className}>
      <div className="flex h-7 w-full gap-[2px] overflow-hidden rounded-[6px]" role="img" aria-label={ariaLabel}>
        {parts.map((p) => {
          const pct = total ? (p.value / total) * 100 : 0
          if (pct <= 0) return null
          return (
            <div
              key={p.key}
              className="relative flex h-full min-w-[3px] items-center rounded-[4px] px-1.5 text-[11px] font-semibold text-white"
              style={{ width: `${pct}%`, background: p.color }}
              title={`${p.label} : ${p.display} (${Math.round(pct)} %)`}
            >
              {pct >= 13 && <span className="num truncate drop-shadow-[0_1px_1px_rgb(0_0_0/0.35)]">{Math.round(pct)}%</span>}
            </div>
          )
        })}
      </div>
      <ul className="mt-2.5 grid grid-cols-2 gap-x-5 gap-y-1.5 text-xs">
        {parts.map((p) => (
          <li key={p.key} className="flex min-w-0 items-center gap-1.5">
            <span aria-hidden="true" className={cn('size-2.5 flex-none rounded-[3px]')} style={{ background: p.color }} />
            <span className="truncate text-ink-3">{p.label}</span>
            <span className="num ml-auto font-semibold text-ink">{p.display}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
