import { Footprints, Gauge } from 'lucide-react'
import { useEfficiencyTrend } from '../../api/queriesV3'
import type { ActivityView } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtDateShort, fmtNumber, isNum } from '../../lib/format'
import { Card } from '../ui/Card'
import { Stat } from '../ui/Value'

function Sparkline({ values, highlight, className, label }: { values: (number | null)[]; highlight?: number; className?: string; label: string }) {
  const nums = values.filter(isNum)
  if (nums.length < 2) return null
  const min = Math.min(...nums)
  const max = Math.max(...nums)
  const span = max - min || 1
  const W = 240
  const H = 56
  const x = (i: number) => (values.length === 1 ? W / 2 : (i / (values.length - 1)) * W)
  const y = (v: number) => H - 4 - ((v - min) / span) * (H - 8)
  let d = ''
  values.forEach((v, i) => {
    if (!isNum(v)) return
    d += `${d ? 'L' : 'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`
  })
  const hv = highlight !== undefined ? values[highlight] : null
  return (
    <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className={cn('h-14 w-full', className)} role="img" aria-label={label}>
      <path d={d} fill="none" stroke="var(--accent)" strokeWidth={2} vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
      {isNum(hv) && highlight !== undefined && <line x1={x(highlight)} x2={x(highlight)} y1={0} y2={H} stroke="var(--line-strong)" strokeDasharray="3 3" vectorEffect="non-scaling-stroke" />}
    </svg>
  )
}

/** Aerobic efficiency (EF speed / power, 12-week trend) and cumulative impact (steps, joint index). */
export function EfficiencyImpact({ view }: { view: ActivityView }) {
  const ef = view.efficiency
  const im = view.impact
  const trend = useEfficiencyTrend(12, !!ef)
  const pts = trend.data?.points ?? []
  const idx = pts.findIndex((p) => p.activityId === view.activity.id)
  const ch = trend.data?.changePct ?? ef?.trendChangePct ?? null
  return (
    <div className="grid gap-5 xl:grid-cols-2">
      {ef && (
        <Card title="Efficience aérobie" icon={<Gauge className="size-4" />}>
          {ef.eligible ? (
            <>
              <div className="grid grid-cols-2 gap-2.5">
                <Stat label="EF vitesse" sub="m/min par bpm">
                  <span className="num">{fmtNumber(ef.efSpeed, 2)}</span>
                </Stat>
                <Stat label="EF puissance" sub="W par bpm (Pw:HR)">
                  <span className="num">{isNum(ef.efPower) ? fmtNumber(ef.efPower, 2) : 'N/D'}</span>
                </Stat>
              </div>
              <div className="mt-3">
                <div className="mb-1 flex items-baseline justify-between text-xs text-ink-3">
                  <span>Tendance 12 sem. (moyenne glissante)</span>
                  {isNum(ch) && <span className={cn('num font-semibold', ch >= 0 ? 'text-ok' : 'text-warn')}>{`${ch >= 0 ? '+' : '−'}${fmtNumber(Math.abs(ch), 1)} %`}</span>}
                </div>
                <Sparkline values={trend.data?.rollingEfSpeed ?? []} highlight={idx >= 0 ? idx : undefined} label={`Tendance de l’efficience sur 12 semaines${isNum(ch) ? ` : ${ch >= 0 ? '+' : ''}${fmtNumber(ch, 1)} %` : ''}`} />
                {pts.length > 1 && (
                  <div className="flex justify-between text-[11px] text-ink-3">
                    <span>{fmtDateShort(pts[0].date)}</span>
                    <span>{fmtDateShort(pts[pts.length - 1].date)}</span>
                  </div>
                )}
              </div>
              <p className="mt-2 text-xs text-ink-3">Plus l’EF monte à effort stable, plus tu cours vite pour la même FC.</p>
            </>
          ) : (
            <p className="text-sm text-ink-3">{ef.reason ?? 'Non calculée pour cette séance.'}</p>
          )}
        </Card>
      )}
      {im && (
        <Card title="Charge d’impact cumulée" icon={<Footprints className="size-4" />}>
          <div className="grid grid-cols-2 gap-2.5">
            <Stat label="Foulées" sub={isNum(im.stepsPerKm) ? `${fmtNumber(im.stepsPerKm)} / km` : undefined}>
              <span className="num">{isNum(im.steps) ? fmtNumber(im.steps) : 'N/D'}</span>
            </Stat>
            <Stat label="Indice articulaire" sub={`OV ×${fmtNumber(im.voFactor, 2)} · D− ×${fmtNumber(im.descentFactor, 2)}`}>
              <span className="num">{isNum(im.jointIndex) ? fmtNumber(im.jointIndex, 1) : 'N/D'}</span>
            </Stat>
          </div>
          <p className="mt-3 text-sm text-ink-2">
            Cumul 7 jours : <span className="num font-semibold text-ink">{isNum(im.jointIndex7d) ? fmtNumber(im.jointIndex7d, 1) : 'N/D'}</span>
          </p>
          <p className="mt-1 text-xs text-ink-3">Foulées × oscillation verticale × dénivelé négatif. Distinct de la charge d’entraînement et de l’« impact load » Garmin.</p>
        </Card>
      )}
    </div>
  )
}
