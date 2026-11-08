import { Footprints, Gauge } from 'lucide-react'
import { useEfficiencyTrend } from '../../api/queriesV3'
import type { ActivityView } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtDateShort, fmtNumber, fmtPct, isNum, ND } from '../../lib/format'
import { Card } from '../ui/Card'
import { Stat } from '../ui/Value'
import { t, tx } from '../../i18n'

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
        <Card title={t('session.efficiency.title')} icon={<Gauge className="size-4" />}>
          {ef.eligible ? (
            <>
              <div className="grid grid-cols-2 gap-2.5">
                <Stat label={t('session.efficiency.efSpeed')} sub={t('session.efficiency.efSpeedSub')}>
                  <span className="num">{fmtNumber(ef.efSpeed, 2)}</span>
                </Stat>
                <Stat label={t('session.efficiency.efPower')} sub={t('session.efficiency.efPowerSub')}>
                  <span className="num">{isNum(ef.efPower) ? fmtNumber(ef.efPower, 2) : ND}</span>
                </Stat>
              </div>
              <div className="mt-3">
                <div className="mb-1 flex items-baseline justify-between text-xs text-ink-3">
                  <span>{t('session.efficiency.trend')}</span>
                  {isNum(ch) && <span className={cn('num font-semibold', ch >= 0 ? 'text-ok' : 'text-warn')}>{`${ch >= 0 ? '+' : '−'}${fmtPct(Math.abs(ch), 1)}`}</span>}
                </div>
                <Sparkline values={trend.data?.rollingEfSpeed ?? []} highlight={idx >= 0 ? idx : undefined} label={t('session.efficiency.trendAria', { change: isNum(ch) ? `: ${ch >= 0 ? '+' : ''}${fmtPct(ch, 1)}` : '' })} />
                {pts.length > 1 && (
                  <div className="flex justify-between text-[11px] text-ink-3">
                    <span>{fmtDateShort(pts[0].date)}</span>
                    <span>{fmtDateShort(pts[pts.length - 1].date)}</span>
                  </div>
                )}
              </div>
              <p className="mt-2 text-xs text-ink-3">{t('session.efficiency.hint')}</p>
            </>
          ) : (
            <p className="text-sm text-ink-3">{ef.reason ?? t('session.efficiency.notComputed')}</p>
          )}
        </Card>
      )}
      {im && (
        <Card title={t('session.efficiency.impactTitle')} icon={<Footprints className="size-4" />}>
          <div className="grid grid-cols-2 gap-2.5">
            <Stat label={t('session.efficiency.steps')} sub={isNum(im.stepsPerKm) ? `${fmtNumber(im.stepsPerKm)} / km` : undefined}>
              <span className="num">{isNum(im.steps) ? fmtNumber(im.steps) : ND}</span>
            </Stat>
            <Stat label={t('session.efficiency.jointIndex')} sub={t('session.efficiency.voFactor', { vo: fmtNumber(im.voFactor, 2), descent: fmtNumber(im.descentFactor, 2) })}>
              <span className="num">{isNum(im.jointIndex) ? fmtNumber(im.jointIndex, 1) : ND}</span>
            </Stat>
          </div>
          <p className="mt-3 text-sm text-ink-2">
            {tx('session.efficiency.cumul7d', { value: <span className="num font-semibold text-ink">{isNum(im.jointIndex7d) ? fmtNumber(im.jointIndex7d, 1) : ND}</span> })}
          </p>
          <p className="mt-1 text-xs text-ink-3">{t('session.efficiency.impactHint')}</p>
        </Card>
      )}
    </div>
  )
}
