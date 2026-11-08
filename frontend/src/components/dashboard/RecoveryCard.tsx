import { BatteryMedium, BedDouble, Gauge, HeartPulse, Moon } from 'lucide-react'
import type { ReactNode } from 'react'
import type { RecoveryWithReadiness } from '../../api/types'
import { cn } from '../../lib/cn'
import { addDays, fmtDayMonth, fmtHM, fmtNumber, fmtSigned, isNum, ND } from '../../lib/format'
import { hrvStatus, SLEEP_QUALIFIER } from '../../lib/labels'
import { Sparkline } from '../charts/Sparkline'
import { Badge } from '../ui/Badge'
import { Card } from '../ui/Card'
import { NA, Value } from '../ui/Value'
import { t } from '../../i18n'

function Tile({ icon, label, children, sub, tone }: { icon: ReactNode; label: string; children: ReactNode; sub?: ReactNode; tone?: 'warn' | 'danger' | null }) {
  return (
    <div className="glass-inset min-w-0 px-3 py-2.5">
      <p className="flex items-center gap-1 text-[11px] text-ink-3">
        <span aria-hidden="true" className="[&>svg]:size-3.5">
          {icon}
        </span>
        {label}
      </p>
      <p className={cn('num truncate text-lg font-semibold', tone === 'danger' ? 'text-danger' : tone === 'warn' ? 'text-warn' : '')}>{children}</p>
      {sub && <p className="num truncate text-[11px] text-ink-3">{sub}</p>}
    </div>
  )
}

/** Night before (HRV, sleep, RHR, Body Battery) + readiness (manual check-in or reason). */
export function RecoveryCard({ rec, title = t('dashboard.recovery.title'), emptyReason, className, action }: { rec: RecoveryWithReadiness | null; title?: string; emptyReason?: string; className?: string; action?: ReactNode }) {
  if (!rec) {
    return (
      <Card title={title} icon={<Moon className="size-4" />} className={className} action={action}>
        <p className="text-sm text-ink-3">
          <NA reason={emptyReason ?? t('dashboard.recovery.noData')} />
        </p>
      </Card>
    )
  }
  const h = rec.hrv
  const st = h ? hrvStatus(h.status, h.statusLabel) : null
  const sl = rec.sleep
  const q = sl?.qualifier ? SLEEP_QUALIFIER[sl.qualifier] : null
  const rhr = rec.restingHR
  const bb = rec.bodyBattery
  const r = rec.readiness
  const bbTone = !isNum(bb?.atWake) ? null : bb.atWake < 30 ? 'danger' : bb.atWake < 50 ? 'warn' : null
  return (
    <Card title={title} icon={<Moon className="size-4" />} subtitle={t('dashboard.recovery.night', { from: fmtDayMonth(addDays(rec.date, -1)), to: fmtDayMonth(rec.date) })} className={className} action={action}>
      <div className="space-y-2.5">
        <div className="glass-inset flex items-start justify-between gap-3 px-3 py-2.5">
          <div className="min-w-0">
            <p className="flex items-center gap-1 text-[11px] text-ink-3">
              <Gauge className="size-3.5" aria-hidden="true" /> Training Readiness
            </p>
            {isNum(r.score) ? (
              <p className="num text-lg font-semibold">
                {r.score}
                <span className="text-xs text-ink-3">/100</span>
                {r.status && <span className="ml-2 text-sm font-medium text-ink-2">{r.status}</span>}
              </p>
            ) : (
              <p className="text-sm">
                <NA reason={r.reason ?? t('dashboard.recovery.readinessNa')} />
              </p>
            )}
          </div>
          {r.source === 'manual' && <span className="chip flex-none border border-line bg-track text-ink-2">{t('dashboard.recovery.manual')}</span>}
        </div>
        <div className="glass-inset p-3">
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <p className="text-[11px] text-ink-3">{t('dashboard.recovery.hrv')}</p>
              <p className="text-2xl font-semibold tracking-tight">
                <Value value={h?.lastNightAvgMs} format={(v) => fmtNumber(v)} unit="ms" />
              </p>
              <p className="num text-xs text-ink-2">
                {t('dashboard.recovery.baseline', { low: h?.baselineLowMs ?? ND, high: h?.baselineHighMs ?? ND })} <Value value={h?.weeklyAvgMs} format={(v) => fmtNumber(v)} unit="ms" />
              </p>
            </div>
            {st ? <Badge level={st.level}>{st.label}</Badge> : <NA />}
          </div>
          {rec.trend.length > 1 && (
            <div className="mt-2">
              <Sparkline
                values={rec.trend.map((d) => d.hrvMs)}
                band={{ min: h?.baselineLowMs ?? null, max: h?.baselineHighMs ?? null }}
                ariaLabel={t('dashboard.recovery.hrvTrend', { values: rec.trend.map((d) => (d.hrvMs === null ? ND : `${d.hrvMs} ms`)).join(', ') })}
                height={40}
              />
              <p className="mt-0.5 flex justify-between text-[10px] text-ink-3" aria-hidden="true">
                <span>{fmtDayMonth(rec.trend[0].date)}</span>
                <span>{t('dashboard.recovery.hrvLegend')}</span>
                <span>{fmtDayMonth(rec.trend[rec.trend.length - 1].date)}</span>
              </p>
            </div>
          )}
        </div>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-2 2xl:grid-cols-4">
          <Tile icon={<BedDouble />} label={t('dashboard.recovery.sleep')} sub={sl ? t('dashboard.recovery.sleepSub', { total: fmtHM(sl.durationS), deep: fmtHM(sl.deepS) }) : ND} tone={isNum(sl?.score) && sl.score < 60 ? 'warn' : null}>
            {isNum(sl?.score) ? (
              <>
                {sl.score}
                <span className="text-xs text-ink-3">/100</span>
                {q && <span className="sr-only"> {q.label}</span>}
              </>
            ) : (
              <NA />
            )}
          </Tile>
          <Tile icon={<HeartPulse />} label={t('dashboard.recovery.rhr')} sub={isNum(rhr?.delta) ? t('dashboard.recovery.vs7d', { delta: fmtSigned(rhr.delta) }) : ND} tone={isNum(rhr?.delta) && rhr.delta >= 5 ? 'warn' : null}>
            <Value value={rhr?.bpm} format={(v) => fmtNumber(v)} unit="bpm" />
          </Tile>
          <Tile icon={<BatteryMedium />} label="Body Battery" sub={t('dashboard.recovery.atWake')} tone={bbTone}>
            {isNum(bb?.atWake) ? (
              <>
                {bb.atWake}
                <span className="text-xs text-ink-3">/100</span>
              </>
            ) : (
              <NA />
            )}
          </Tile>
          <Tile icon={<Gauge />} label={t('dashboard.recovery.stress')} sub={bb && isNum(bb.charged) ? `BB +${bb.charged} / −${bb.drained ?? ND}` : undefined}>
            {isNum(rec.stressAvg) ? (
              <>
                {rec.stressAvg}
                <span className="text-xs text-ink-3">/100</span>
              </>
            ) : (
              <NA />
            )}
          </Tile>
        </div>
      </div>
    </Card>
  )
}
