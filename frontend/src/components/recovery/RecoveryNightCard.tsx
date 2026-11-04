import { BatteryMedium, BedDouble, HeartPulse, Moon } from 'lucide-react'
import type { RecoveryContext } from '../../api/types'
import { cn } from '../../lib/cn'
import { addDays, fmtDayMonth, fmtHM, fmtNumber, fmtSigned, isNum } from '../../lib/format'
import { hrvStatus, SLEEP_QUALIFIER } from '../../lib/labels'
import { Sparkline } from '../charts/Sparkline'
import { Badge } from '../ui/Badge'
import { Card } from '../ui/Card'
import { NA, Value } from '../ui/Value'

/** Récupération de la nuit précédant la séance (mode SFP / Performance). */
export function RecoveryNightCard({ rec, className, wide }: { rec: RecoveryContext | null; className?: string; wide?: boolean }) {
  if (!rec) {
    return (
      <Card title="Récupération de la nuit" icon={<Moon className="size-4" />} className={className}>
        <p className="text-sm text-ink-3">Données de récupération indisponibles pour cette nuit.</p>
      </Card>
    )
  }
  const h = rec.hrv
  const st = h ? hrvStatus(h.status, h.statusLabel) : null
  const sl = rec.sleep
  const q = sl?.qualifier ? SLEEP_QUALIFIER[sl.qualifier] : null
  const rhr = rec.restingHR
  const bb = rec.bodyBattery
  const tile = cn('glass-inset min-w-0 px-3 py-2.5', wide && 'md:grid md:grid-cols-[minmax(0,1fr)_auto] md:items-center md:gap-x-3 md:[&>p:nth-child(3)]:col-span-2')
  const bbLevel = !isNum(bb?.atWake) ? null : bb.atWake < 30 ? 'danger' : bb.atWake < 50 ? 'warn' : 'ok'
  return (
    <Card title="Récupération de la nuit" icon={<Moon className="size-4" />} subtitle={`Nuit du ${fmtDayMonth(addDays(rec.date, -1))} au ${fmtDayMonth(rec.date)}`} className={className}>
      <div className={cn(wide && 'gap-3 md:grid md:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]')}>
      <div className="glass-inset p-3.5">
        <div className="flex items-start justify-between gap-2">
          <div>
            <p className="text-xs font-medium text-ink-3">VFC nocturne</p>
            <p className="text-3xl font-semibold tracking-tight">
              <Value value={h?.lastNightAvgMs} format={(v) => fmtNumber(v)} unit="ms" />
            </p>
            <p className="num text-xs text-ink-2">
              Baseline {h?.baselineLowMs ?? 'N/D'}–{h?.baselineHighMs ?? 'N/D'} ms · moy. 7 j <Value value={h?.weeklyAvgMs} format={(v) => fmtNumber(v)} unit="ms" />
            </p>
          </div>
          {st ? <Badge level={st.level}>{st.label}</Badge> : <NA />}
        </div>
        {rec.trend.length > 1 && (
          <div className="mt-2">
            <Sparkline
              values={rec.trend.map((d) => d.hrvMs)}
              band={{ min: h?.baselineLowMs ?? null, max: h?.baselineHighMs ?? null }}
              ariaLabel={`Tendance VFC 7 jours : ${rec.trend.map((d) => (d.hrvMs === null ? 'N/D' : `${d.hrvMs} ms`)).join(', ')}`}
            />
            <p className="mt-1 flex justify-between text-[10px] text-ink-3" aria-hidden="true">
              <span>{fmtDayMonth(rec.trend[0].date)}</span>
              <span>VFC 7 jours · bande = baseline</span>
              <span>{fmtDayMonth(rec.trend[rec.trend.length - 1].date)}</span>
            </p>
          </div>
        )}
      </div>
      <div className={cn('mt-2.5 grid grid-cols-3 gap-2', wide && 'md:mt-0 md:grid-cols-1')}>
        <div className={tile}>
          <p className="flex items-center gap-1 text-[11px] text-ink-3">
            <BedDouble className="size-3.5" aria-hidden="true" /> Sommeil
          </p>
          <p className="num text-lg font-semibold">{isNum(sl?.score) ? <>{sl.score}<span className="text-xs text-ink-3">/100</span></> : <NA />}</p>
          <p className="num truncate text-[11px] text-ink-3">{sl ? `profond ${fmtHM(sl.deepS)}` : 'N/D'}</p>
          {q && <span className="sr-only">{q.label}</span>}
        </div>
        <div className={tile}>
          <p className="flex items-center gap-1 text-[11px] text-ink-3">
            <HeartPulse className="size-3.5" aria-hidden="true" /> FC repos
          </p>
          <p className="text-lg font-semibold">
            <Value value={rhr?.bpm} format={(v) => fmtNumber(v)} unit="bpm" />
          </p>
          <p className={cn('num truncate text-[11px]', isNum(rhr?.delta) && rhr.delta >= 5 ? 'text-warn' : 'text-ink-3')}>{isNum(rhr?.delta) ? `${fmtSigned(rhr.delta)} vs 7 j` : 'N/D'}</p>
        </div>
        <div className={tile}>
          <p className="flex items-center gap-1 text-[11px] text-ink-3">
            <BatteryMedium className="size-3.5" aria-hidden="true" /> Body Batt.
          </p>
          <p className={cn('num text-lg font-semibold', bbLevel === 'danger' ? 'text-danger' : bbLevel === 'warn' ? 'text-warn' : '')}>
            {isNum(bb?.atWake) ? <>{bb.atWake}<span className="text-xs text-ink-3">/100</span></> : <NA />}
          </p>
          <p className="truncate text-[11px] text-ink-3">au réveil</p>
        </div>
      </div>
      </div>
    </Card>
  )
}
