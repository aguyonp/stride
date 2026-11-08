import { useId, useMemo, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { useElementWidth } from '../../hooks/useElementWidth'
import { cn } from '../../lib/cn'
import { fmtDuration } from '../../lib/format'
import { areaPath, extent, linear, linePath, nearestIndex, niceDomain, timeStep } from './scale'
import { t } from '../../i18n'

interface ChartBand {
  min: number
  max: number
  label: string
  color?: string
}

export interface ChartPanel {
  id: string
  label: string
  unit: string
  values: (number | null)[]
  color: string
  format: (v: number) => string
  summary?: string
  bands?: ChartBand[]
  threshold?: { value: number; label: string }
  area?: boolean
  reverse?: boolean
  domain?: [number, number]
  ticks?: number[]
  height?: number
}

interface ChartShade {
  start: number
  end: number
}

interface Props {
  tS: number[]
  panels: ChartPanel[]
  shades?: ChartShade[]
  shadeLabel?: string
  ariaLabel: string
  className?: string
  segmentAt?: (t: number) => string | null
}

const PAD_L = 38
const PAD_R = 10
const AXIS_H = 22

/** Synchronised small multiples with a shared time axis and crosshair (keyboard: arrows, Home, End). */
export function TimeSeriesStack({ tS, panels, shades = [], shadeLabel, ariaLabel, className, segmentAt }: Props) {
  const [wrapRef, width] = useElementWidth<HTMLDivElement>()
  const [hover, setHover] = useState<number | null>(null)
  const clipBase = useId().replace(/:/g, '')
  const tMax = tS.length ? tS[tS.length - 1] : 1
  const plotW = Math.max(10, width - PAD_L - PAD_R)
  const x = useMemo(() => linear([0, tMax || 1], [PAD_L, PAD_L + plotW]), [tMax, plotW])

  const prepared = useMemo(
    () =>
      panels.map((p) => {
        const h = p.height ?? 84
        const bandEdges = (p.bands ?? []).flatMap((b) => [b.min, b.max])
        const ext = p.domain ?? extent(p.values, [...bandEdges, ...(p.threshold ? [p.threshold.value] : [])])
        const nice = ext ? niceDomain(ext[0], ext[1], 3) : { domain: [0, 1] as [number, number], ticks: [] }
        const domain = p.domain ?? nice.domain
        const ticks = p.ticks ?? (p.domain ? niceDomain(p.domain[0], p.domain[1], 3).ticks.filter((t) => t >= p.domain![0] && t <= p.domain![1]) : nice.ticks)
        const y = linear(domain, p.reverse ? [6, h - 4] : [h - 4, 6])
        return { p, h, y, ticks, line: linePath(tS, p.values, x, y), area: p.area ? areaPath(tS, p.values, x, y, h - 4) : '' }
      }),
    [panels, tS, x],
  )

  const xTicks = useMemo(() => {
    const step = timeStep(tMax, Math.max(2, Math.floor(plotW / 70)))
    const out: number[] = []
    for (let t = 0; t <= tMax; t += step) out.push(t)
    return out
  }, [tMax, plotW])

  const setFromClientX = (clientX: number, el: HTMLElement) => {
    const rect = el.getBoundingClientRect()
    const px = clientX - rect.left
    if (px < PAD_L - 6 || px > PAD_L + plotW + 6) return
    const t = ((px - PAD_L) / plotW) * tMax
    setHover(nearestIndex(tS, Math.min(tMax, Math.max(0, t))))
  }

  const onPointerMove = (e: PointerEvent<HTMLDivElement>) => setFromClientX(e.clientX, e.currentTarget)
  const onPointerLeave = (e: PointerEvent<HTMLDivElement>) => {
    if (e.pointerType === 'mouse') setHover(null)
  }
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const stepN = Math.max(1, Math.round(tS.length / 60))
    const cur = hover ?? 0
    if (e.key === 'ArrowRight') setHover(Math.min(tS.length - 1, cur + stepN))
    else if (e.key === 'ArrowLeft') setHover(Math.max(0, cur - stepN))
    else if (e.key === 'Home') setHover(0)
    else if (e.key === 'End') setHover(tS.length - 1)
    else if (e.key === 'Escape') setHover(null)
    else return
    e.preventDefault()
  }

  const hx = hover !== null ? x(tS[hover]) : null
  const hoverSeg = hover !== null && segmentAt ? segmentAt(tS[hover]) : null

  return (
    <div className={cn('min-w-0', className)}>
      <div
        ref={wrapRef}
        role="group"
        tabIndex={0}
        aria-label={t('session.charts.keyboard', { label: ariaLabel })}
        onPointerMove={onPointerMove}
        onPointerDown={onPointerMove}
        onPointerLeave={onPointerLeave}
        onKeyDown={onKeyDown}
        onBlur={() => setHover(null)}
        className="relative touch-pan-y rounded-xl select-none focus-visible:outline-offset-4"
      >
        {width > 0 &&
          prepared.map(({ p, h, y, ticks, line, area }, pi) => {
            const v = hover !== null ? p.values[hover] : null
            const clipId = `${clipBase}-${pi}`
            return (
              <div key={p.id} className={cn(pi > 0 && 'mt-2.5')}>
                <div className="mb-1 flex items-baseline justify-between gap-2 pl-[38px] text-xs">
                  <span className="flex min-w-0 items-center gap-1.5">
                    <span aria-hidden="true" className="inline-block h-[3px] w-3.5 flex-none rounded-full" style={{ background: p.color }} />
                    <span className="truncate font-medium text-ink-2">{p.label}</span>
                  </span>
                  <span className="num flex-none text-ink-3" aria-live="off">
                    {hover !== null ? (
                      v !== null ? (
                        <>
                          <strong className="font-semibold text-ink">{p.format(v)}</strong> {p.unit}
                        </>
                      ) : (
                        '—'
                      )
                    ) : (
                      p.summary
                    )}
                  </span>
                </div>
                <svg width={width} height={h} className="block overflow-visible" aria-hidden="true">
                  <defs>
                    <clipPath id={clipId}>
                      <rect x={PAD_L} y={0} width={plotW} height={h} />
                    </clipPath>
                  </defs>
                  {shades.map((s, i) => (
                    <rect key={i} x={x(s.start)} y={0} width={Math.max(1, x(s.end) - x(s.start))} height={h} fill="color-mix(in oklab, var(--text-1) 6%, transparent)" />
                  ))}
                  {ticks.map((t) => (
                    <g key={t}>
                      <line x1={PAD_L} x2={PAD_L + plotW} y1={y(t)} y2={y(t)} stroke="var(--line)" strokeWidth={1} />
                      <text x={PAD_L - 6} y={y(t)} dy="0.32em" textAnchor="end" className="num fill-[var(--text-3)] text-[10px]">
                        {p.format(t)}
                      </text>
                    </g>
                  ))}
                  <g clipPath={`url(#${clipId})`}>
                    {(p.bands ?? []).map((b) => {
                      const y1 = y(b.max)
                      const y2 = y(b.min)
                      const top = Math.min(y1, y2)
                      const bh = Math.abs(y2 - y1)
                      const c = b.color ?? 'var(--accent)'
                      return (
                        <g key={b.label}>
                          <rect x={PAD_L} y={top} width={plotW} height={Math.max(1, bh)} fill={`color-mix(in oklab, ${c} 13%, transparent)`} />
                          <line x1={PAD_L} x2={PAD_L + plotW} y1={top} y2={top} stroke={`color-mix(in oklab, ${c} 45%, transparent)`} strokeWidth={1} />
                          <line x1={PAD_L} x2={PAD_L + plotW} y1={top + bh} y2={top + bh} stroke={`color-mix(in oklab, ${c} 45%, transparent)`} strokeWidth={1} />
                        </g>
                      )
                    })}
                    {p.threshold && (
                      <line x1={PAD_L} x2={PAD_L + plotW} y1={y(p.threshold.value)} y2={y(p.threshold.value)} stroke="color-mix(in oklab, var(--danger) 70%, transparent)" strokeWidth={1} />
                    )}
                    {area && <path d={area} fill={`color-mix(in oklab, ${p.color} 12%, transparent)`} />}
                    <path d={line} fill="none" stroke={p.color} strokeWidth={1.75} strokeLinejoin="round" strokeLinecap="round" />
                  </g>
                  {(p.bands ?? []).map((b, bi) => (
                    <text
                      key={b.label}
                      x={bi % 2 === 0 ? PAD_L + plotW - 4 : PAD_L + 4}
                      y={p.reverse ? Math.max(y(b.max), y(b.min)) + 11 : Math.min(y(b.max), y(b.min)) - 3}
                      textAnchor={bi % 2 === 0 ? 'end' : 'start'}
                      className="fill-[var(--text-3)] text-[10px] font-medium"
                    >
                      {b.label}
                    </text>
                  ))}
                  {p.threshold && (
                    <text x={PAD_L + plotW - 4} y={y(p.threshold.value) - 3} textAnchor="end" className="fill-[var(--danger)] text-[10px] font-medium">
                      {p.threshold.label}
                    </text>
                  )}
                  {hx !== null && (
                    <g>
                      <line x1={hx} x2={hx} y1={0} y2={h} stroke="var(--text-2)" strokeOpacity={0.55} strokeWidth={1} />
                      {v !== null && <circle cx={hx} cy={y(v)} r={4} fill={p.color} stroke="var(--surface)" strokeWidth={2} />}
                    </g>
                  )}
                </svg>
              </div>
            )
          })}

        {width > 0 && (
          <svg width={width} height={AXIS_H} className="mt-1 block overflow-visible" aria-hidden="true">
            <line x1={PAD_L} x2={PAD_L + plotW} y1={0.5} y2={0.5} stroke="var(--line-strong)" />
            {xTicks.map((t) => (
              <text key={t} x={x(t)} y={15} textAnchor={t === 0 ? 'start' : 'middle'} className="num fill-[var(--text-3)] text-[10px]">
                {fmtDuration(t)}
              </text>
            ))}
            {hx !== null && hover !== null && (() => {
              const cx = Math.min(Math.max(hx, PAD_L + 30), PAD_L + plotW - 30)
              return (
                <g>
                  <rect x={cx - 30} y={2} width={60} height={18} rx={6} fill="var(--text-1)" />
                  <text x={cx} y={14.5} textAnchor="middle" className="num fill-[var(--bg)] text-[10.5px] font-semibold">
                    {fmtDuration(tS[hover])}
                  </text>
                </g>
              )
            })()}
          </svg>
        )}
        {width === 0 && <div style={{ height: panels.reduce((s, p) => s + (p.height ?? 84) + 30, AXIS_H) }} />}
      </div>
      {(shades.length > 0 || hoverSeg) && (
        <div className="mt-1 flex min-h-5 items-center justify-between gap-3 pl-[38px] text-xs text-ink-3">
          {shades.length > 0 && shadeLabel ? (
            <span className="flex items-center gap-1.5">
              <span aria-hidden="true" className="inline-block size-3 rounded-[3px] bg-ink/10" />
              {shadeLabel}
            </span>
          ) : (
            <span />
          )}
          {hoverSeg && <span className="num font-medium text-ink-2">{hoverSeg}</span>}
        </div>
      )}
    </div>
  )
}
