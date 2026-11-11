import { useMemo, useState } from 'react'
import { useTrack } from '../../api/queriesV3'
import type { ActivityTrack, Thresholds } from '../../api/types'
import { fmtPct } from '../../lib/format'
import { InlineError, Spinner } from '../ui/Misc'
import { SegmentedControl } from '../ui/SegmentedControl'
import { t, tx } from '../../i18n'

type Mode = 'cadence' | 'slope'
const W = 600
const PAD = 14

interface Run {
  hot: boolean
  d: string
}

export function projectTrack(t: ActivityTrack, mode: Mode, cadenceMin: number) {
  const { minLat, maxLat, minLon, maxLon } = t.bounds ?? { minLat: 0, maxLat: 0, minLon: 0, maxLon: 0 }
  const k = Math.cos((((minLat + maxLat) / 2) * Math.PI) / 180)
  const spanX = Math.max(1e-9, (maxLon - minLon) * k)
  const spanY = Math.max(1e-9, maxLat - minLat)
  const h = Math.round(Math.min(420, Math.max(220, (W * spanY) / spanX)))
  const s = Math.min((W - 2 * PAD) / spanX, (h - 2 * PAD) / spanY)
  const ox = (W - spanX * s) / 2
  const oy = (h - spanY * s) / 2
  const xy = t.points.map(([lat, lon]) => [ox + (lon - minLon) * k * s, oy + (maxLat - lat) * s] as const)
  const hotAt = (i: number) => {
    const p = t.points[i]
    return mode === 'cadence' ? p[3] !== null && p[3] < cadenceMin : p[4] !== null && p[4] < -2
  }
  const runs: Run[] = []
  let hotCount = 0
  let cur: Run | null = null
  xy.forEach(([x, y], i) => {
    const hot = hotAt(i)
    if (hot) hotCount++
    const pt = `${x.toFixed(1)},${y.toFixed(1)}`
    if (!cur || cur.hot !== hot) {
      const prev = i > 0 ? `M${xy[i - 1][0].toFixed(1)},${xy[i - 1][1].toFixed(1)}L${pt}` : `M${pt}`
      cur = { hot, d: prev }
      runs.push(cur)
    } else cur.d += `L${pt}`
  })
  return { h, runs, start: xy[0], end: xy[xy.length - 1], hotPct: t.points.length ? Math.round((hotCount / t.points.length) * 100) : 0 }
}

/** Lightweight GPS trace (no tiles, no external call) coloured by cadence or slope. */
export function GpsTrack({ activityId, thresholds }: { activityId: number; thresholds: Thresholds | null }) {
  const [mode, setMode] = useState<Mode>('cadence')
  const track = useTrack(activityId)
  const cadMin = thresholds?.cadenceMinSpm ?? 170
  const geo = useMemo(() => (track.data?.bounds && track.data.points.length > 1 ? projectTrack(track.data, mode, cadMin) : null), [track.data, mode, cadMin])

  if (track.isPending) return <Spinner label={t('session.gps.loading')} />
  if (track.isError) return <InlineError>{track.error.message}</InlineError>
  if (!geo) return <p className="text-sm text-ink-3">{t('session.gps.empty')}</p>

  const hotLabel = mode === 'cadence' ? t('session.gps.hotCadence', { value: cadMin }) : t('session.gps.hotSlope')
  return (
    <div>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <SegmentedControl
          ariaLabel={t('session.gps.colorBy')}
          size="sm"
          value={mode}
          onChange={setMode}
          options={[
            { value: 'cadence', label: t('session.gps.cadence') },
            { value: 'slope', label: t('session.gps.slope') },
          ]}
        />
        <p className="text-xs text-ink-3">
          {tx('session.gps.share', { pct: <span className="num font-semibold text-warn">{fmtPct(geo.hotPct, 0)}</span>, label: hotLabel })}
        </p>
      </div>
      <svg viewBox={`0 0 ${W} ${geo.h}`} className="h-auto w-full rounded-2xl bg-track/40" role="img" aria-label={t('session.gps.aria', { pct: fmtPct(geo.hotPct, 0), label: hotLabel })}>
        {geo.runs.map((r, i) => (
          <path key={i} d={r.d} fill="none" stroke={r.hot ? 'var(--warn)' : 'var(--accent)'} strokeOpacity={r.hot ? 1 : 0.75} strokeWidth={r.hot ? 4.5 : 3} strokeLinecap="round" strokeLinejoin="round" />
        ))}
        <rect x={geo.end[0] - 7} y={geo.end[1] - 7} width={14} height={14} rx={3} fill="var(--text-1)" stroke="var(--surface)" strokeWidth={2} />
        <circle cx={geo.start[0]} cy={geo.start[1]} r={4.5} fill="var(--ok)" stroke="var(--surface)" strokeWidth={1.5} />
      </svg>
      <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink-3" aria-label={t('session.gps.legend')}>
        <li className="flex items-center gap-1.5">
          <span className="h-1 w-4 rounded bg-accent" aria-hidden="true" /> {t('session.gps.normal')}
        </li>
        <li className="flex items-center gap-1.5">
          <span className="h-1.5 w-4 rounded bg-warn" aria-hidden="true" /> {hotLabel}
        </li>
        <li className="flex items-center gap-1.5">
          <span className="size-2.5 rounded-full bg-ok" aria-hidden="true" /> {t('session.gps.start')}
        </li>
        <li className="flex items-center gap-1.5">
          <span className="size-2.5 rounded-sm bg-ink" aria-hidden="true" /> {t('session.gps.finish')}
        </li>
      </ul>
    </div>
  )
}
