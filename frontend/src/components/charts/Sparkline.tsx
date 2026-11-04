import { useElementWidth } from '../../hooks/useElementWidth'
import { extent, linear, linePath, niceDomain } from './scale'

interface SparklineProps {
  values: (number | null)[]
  band?: { min: number | null; max: number | null }
  height?: number
  color?: string
  ariaLabel: string
}

/** Mini-tendance (7 points) avec bande de référence et point final. */
export function Sparkline({ values, band, height = 44, color = 'var(--accent)', ariaLabel }: SparklineProps) {
  const [ref, width] = useElementWidth<HTMLDivElement>()
  const extra = band && band.min !== null && band.max !== null ? [band.min, band.max] : []
  const ext = extent(values, extra)
  const { domain } = ext ? niceDomain(ext[0], ext[1], 2) : { domain: [0, 1] as [number, number] }
  const xs = values.map((_, i) => i)
  const x = linear([0, Math.max(1, values.length - 1)], [5, Math.max(10, width - 5)])
  const y = linear(domain, [height - 5, 5])
  let lastIdx = -1
  values.forEach((v, i) => {
    if (v !== null) lastIdx = i
  })
  return (
    <div ref={ref} role="img" aria-label={ariaLabel} className="w-full">
      {width > 0 && (
        <svg width={width} height={height} className="block overflow-visible" aria-hidden="true">
          {band && band.min !== null && band.max !== null && (
            <rect x={0} y={y(band.max)} width={width} height={Math.max(1, y(band.min) - y(band.max))} rx={4} fill="color-mix(in oklab, var(--ok) 14%, transparent)" />
          )}
          <path d={linePath(xs, values, x, y)} fill="none" stroke={color} strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />
          {lastIdx >= 0 && <circle cx={x(lastIdx)} cy={y(values[lastIdx] as number)} r={4} fill={color} stroke="var(--surface)" strokeWidth={2} />}
        </svg>
      )}
      {width === 0 && <div style={{ height }} />}
    </div>
  )
}
