import { useState } from 'react'
import { useElementWidth } from '../../hooks/useElementWidth'
import { cn } from '../../lib/cn'
import { extent, linear, niceDomain } from './scale'

export interface BarDatum {
  key: string
  label: string
  value: number | null
  tooltip: string
  highlight?: boolean
}

interface BarTrendProps {
  data: BarDatum[]
  band?: { min: number; max: number; label: string } | null
  format: (v: number) => string
  height?: number
  ariaLabel: string
  zeroBased?: boolean
}

/** Columns with a reference band; each bar is focusable and reveals its value. */
export function BarTrend({ data, band, format, height = 132, ariaLabel, zeroBased = false }: BarTrendProps) {
  const [ref, width] = useElementWidth<HTMLDivElement>()
  const [active, setActive] = useState<number | null>(null)
  const padL = 30
  const padB = 22
  const padT = 18
  const ext = extent(
    data.map((d) => d.value),
    band ? [band.min, band.max] : [],
  )
  const lo = ext ? (zeroBased ? 0 : Math.max(0, ext[0] - (ext[1] - ext[0]) * 0.6)) : 0
  const { domain, ticks } = niceDomain(lo, ext ? ext[1] : 1, 3)
  const plotW = Math.max(10, width - padL)
  const slot = plotW / Math.max(1, data.length)
  const barW = Math.min(24, slot * 0.56)
  const y = linear(domain, [height - padB, padT])
  const base = height - padB

  return (
    <div ref={ref} className="relative w-full" role="group" aria-label={ariaLabel}>
      {width > 0 && (
        <svg width={width} height={height} className="block overflow-visible">
          {ticks.map((t) => (
            <g key={t} aria-hidden="true">
              <line x1={padL} x2={width} y1={y(t)} y2={y(t)} stroke="var(--line)" />
              <text x={padL - 6} y={y(t)} dy="0.32em" textAnchor="end" className="num fill-[var(--text-3)] text-[10px]">
                {format(t)}
              </text>
            </g>
          ))}
          {band && (
            <g aria-hidden="true">
              <rect x={padL} y={y(band.max)} width={plotW} height={Math.max(1, y(band.min) - y(band.max))} fill="color-mix(in oklab, var(--ok) 12%, transparent)" />
              <text x={width - 2} y={y(band.max) - 4} textAnchor="end" className="fill-[var(--text-3)] text-[10px] font-medium">
                {band.label}
              </text>
            </g>
          )}
          {data.map((d, i) => {
            const cx = padL + slot * i + slot / 2
            const isActive = active === i
            const v = d.value
            const top = v !== null ? y(v) : base
            const h = Math.max(0, base - top)
            const r = Math.min(4, h)
            return (
              <g
                key={d.key}
                tabIndex={0}
                role="img"
                aria-label={d.tooltip}
                onPointerEnter={() => setActive(i)}
                onPointerLeave={() => setActive(null)}
                onFocus={() => setActive(i)}
                onBlur={() => setActive(null)}
                className="outline-none"
              >
                <rect x={cx - slot / 2} y={padT - 4} width={slot} height={base - padT + 4} fill="transparent" />
                {v !== null ? (
                  <path
                    d={`M${cx - barW / 2},${base}V${top + r}Q${cx - barW / 2},${top} ${cx - barW / 2 + r},${top}H${cx + barW / 2 - r}Q${cx + barW / 2},${top} ${cx + barW / 2},${top + r}V${base}Z`}
                    fill="var(--accent)"
                    fillOpacity={d.highlight || isActive ? 1 : 0.5}
                    className="transition-[fill-opacity] duration-200"
                  />
                ) : (
                  <text x={cx} y={base - 6} textAnchor="middle" className="fill-[var(--text-3)] text-[10px]">
                    N/D
                  </text>
                )}
                {(d.highlight || isActive) && v !== null && (
                  <text x={cx} y={top - 6} textAnchor="middle" className="num fill-[var(--text-1)] text-[11px] font-semibold">
                    {format(v)}
                  </text>
                )}
                <text x={cx} y={height - 6} textAnchor="middle" className={cn('num text-[10px]', d.highlight ? 'fill-[var(--text-1)] font-semibold' : 'fill-[var(--text-3)]')}>
                  {d.label}
                </text>
              </g>
            )
          })}
          <line x1={padL} x2={width} y1={base + 0.5} y2={base + 0.5} stroke="var(--line-strong)" aria-hidden="true" />
        </svg>
      )}
      {width === 0 && <div style={{ height }} />}
      {active !== null && (
        <div
          className="pointer-events-none absolute -top-1 z-10 -translate-x-1/2 -translate-y-full rounded-lg border border-line bg-surface px-2.5 py-1.5 text-xs whitespace-nowrap text-ink shadow-lg"
          style={{ left: Math.min(Math.max(padL + slot * active + slot / 2, 90), width - 90) }}
          role="presentation"
        >
          {data[active].tooltip}
        </div>
      )}
    </div>
  )
}
