import { HeartPulse, MountainSnow, TrendingDown, TrendingUp } from 'lucide-react'
import type { Drift, Profile } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtNumber, fmtSigned, isNum } from '../../lib/format'
import { DRIFT_VERDICT } from '../../lib/labels'
import { Badge } from '../ui/Badge'
import { Card } from '../ui/Card'
import { NA, Value } from '../ui/Value'

/** Dénivelé négatif mis en avant (charge fémoro-patellaire en descente). */
export function DescentCard({ gain, loss, profile, className }: { gain: number | null; loss: number | null; profile: Profile; className?: string }) {
  const t = profile.descentAlertM
  const level = !isNum(loss) ? null : loss > 2 * t ? 'danger' : loss > t ? 'warn' : 'ok'
  const scaleMax = Math.max(2.4 * t, isNum(loss) ? loss * 1.1 : 0)
  const pct = (v: number) => `${Math.min(100, (v / scaleMax) * 100)}%`
  const color = level === 'danger' ? 'var(--danger)' : level === 'warn' ? 'var(--warn)' : 'var(--ok)'
  return (
    <Card title="Descente (D−)" icon={<TrendingDown className="size-4" />} className={className} action={level ? <Badge level={level}>{level === 'ok' ? 'OK' : level === 'warn' ? '> seuil' : '> 2× seuil'}</Badge> : undefined}>
      <div className="flex items-end justify-between gap-3">
        <p className="text-4xl font-semibold tracking-tight">
          {isNum(loss) ? (
            <span className="num">
              −{fmtNumber(loss)}
              <span className="ml-1 text-base font-medium text-ink-3">m</span>
            </span>
          ) : (
            <NA />
          )}
        </p>
        <p className="num pb-1 text-sm text-ink-2">
          <TrendingUp className="mr-1 inline size-3.5 align-[-2px] text-ink-3" aria-hidden="true" />
          D+ <Value value={gain} format={(v) => `+${fmtNumber(v)}`} unit="m" className="font-semibold text-ink" />
        </p>
      </div>
      {isNum(loss) && (
        <div className="mt-3" role="img" aria-label={`D− ${loss} m, seuil d'alerte ${t} m`}>
          <div className="relative h-2.5 rounded-full bg-track">
            <div className="absolute inset-y-0 left-0 rounded-full transition-all duration-700" style={{ width: pct(loss), background: color }} />
            <div className="absolute -inset-y-1 w-px bg-ink-2" style={{ left: pct(t) }} />
            <div className="absolute -inset-y-1 w-px bg-danger" style={{ left: pct(2 * t) }} />
          </div>
          <div className="num relative mt-1 h-4 text-[10px] text-ink-3" aria-hidden="true">
            <span className="absolute -translate-x-1/2" style={{ left: pct(t) }}>
              {t} m
            </span>
            <span className="absolute -translate-x-1/2" style={{ left: pct(2 * t) }}>
              {2 * t} m
            </span>
          </div>
        </div>
      )}
      <p className="mt-2 text-xs text-ink-3">
        <MountainSnow className="mr-1 inline size-3.5 align-[-2px]" aria-hidden="true" />
        Les descentes augmentent la pression fémoro-patellaire (freinage excentrique).
      </p>
    </Card>
  )
}

/** Dérive cardiaque (découplage Pw:Hr ou Allure:FC). */
export function DriftCard({ drift, className, compact }: { drift: Drift | null; className?: string; compact?: boolean }) {
  const v = drift ? DRIFT_VERDICT[drift.verdict] : null
  return (
    <Card title="Dérive cardiaque" icon={<HeartPulse className="size-4" />} className={className} action={v && drift ? <Badge level={v.level}>{v.label}</Badge> : undefined}>
      {!drift ? (
        <p className="text-sm text-ink-3">
          <NA /> — séance trop courte ou données FC insuffisantes.
        </p>
      ) : (
        <>
          <div className="flex items-baseline gap-2">
            <p className={cn('num text-4xl font-semibold tracking-tight', v?.level === 'danger' ? 'text-danger' : v?.level === 'warn' ? 'text-warn' : 'text-ink')}>
              {fmtSigned(drift.decouplingPct, 1)}
              <span className="ml-0.5 text-base font-medium text-ink-3">%</span>
            </p>
            <span className="text-xs text-ink-3">découplage {drift.method === 'pw:hr' ? 'Pw:Hr' : 'Allure:FC'}</span>
          </div>
          {!compact && (
            <dl className="num mt-3 grid grid-cols-3 gap-2 text-center">
              <div className="glass-inset rounded-xl px-2 py-2">
                <dt className="text-[11px] text-ink-3">FC 1re moitié</dt>
                <dd className="text-sm font-semibold">{drift.firstHalfHR} bpm</dd>
              </div>
              <div className="glass-inset rounded-xl px-2 py-2">
                <dt className="text-[11px] text-ink-3">FC 2e moitié</dt>
                <dd className="text-sm font-semibold">{drift.secondHalfHR} bpm</dd>
              </div>
              <div className="glass-inset rounded-xl px-2 py-2">
                <dt className="text-[11px] text-ink-3">Dernier tiers</dt>
                <dd className="text-sm font-semibold">{fmtSigned(drift.lastThirdHrRiseBpm)} bpm</dd>
              </div>
            </dl>
          )}
          <p className="mt-2 text-xs text-ink-3">&lt; 5 % stable · 5–10 % modérée · &gt; 10 % élevée</p>
        </>
      )}
    </Card>
  )
}
