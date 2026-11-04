import { Clock, Gauge, HeartPulse, Route, TrendingDown, TrendingUp, Zap } from 'lucide-react'
import type { ActivityDetail } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtDuration, fmtNumber, fmtPaceValue } from '../../lib/format'
import { Stat, Value } from '../ui/Value'

/** KPIs de séance. */
export function KpiGrid({ a, className, showElevation = true }: { a: ActivityDetail; className?: string; showElevation?: boolean }) {
  return (
    <div className={cn('grid grid-cols-2 gap-2.5 sm:grid-cols-3 xl:grid-cols-6', className)}>
      <Stat label="Distance" icon={<Route />} emphasis>
        <Value value={a.distanceM === null ? null : a.distanceM / 1000} format={(v) => fmtNumber(v, 2)} unit="km" />
      </Stat>
      <Stat label="Durée" icon={<Clock />} emphasis sub={a.movingDurationS && a.durationS && a.movingDurationS < a.durationS - 30 ? `en mouvement ${fmtDuration(a.movingDurationS)}` : undefined}>
        <Value value={a.durationS} format={fmtDuration} />
      </Stat>
      <Stat label="Allure moyenne" icon={<Gauge />} emphasis>
        <Value value={a.avgPaceSecPerKm} format={fmtPaceValue} unit="/km" />
      </Stat>
      <Stat label="Puissance" icon={<Zap />} emphasis sub={a.normPower ? `NP ${fmtNumber(a.normPower)} W` : undefined}>
        <Value value={a.avgPower} format={(v) => fmtNumber(v)} unit="W" />
      </Stat>
      <Stat label="FC moyenne" icon={<HeartPulse />} emphasis sub={a.maxHR ? `max ${a.maxHR} bpm` : undefined}>
        <Value value={a.avgHR} format={(v) => fmtNumber(v)} unit="bpm" />
      </Stat>
      {showElevation ? (
        <Stat label="D+ / D−" icon={<TrendingUp />} emphasis>
          <span className="num">
            <Value value={a.elevGainM} format={(v) => `+${fmtNumber(v)}`} />
            <span className="mx-1 text-ink-3">/</span>
            <Value value={a.elevLossM} format={(v) => `−${fmtNumber(v)}`} unit="m" />
          </span>
        </Stat>
      ) : (
        <Stat label="Training Load" icon={<TrendingDown />} emphasis>
          <Value value={a.trainingLoad} format={(v) => fmtNumber(v)} />
        </Stat>
      )}
    </div>
  )
}
