import { Gauge, Medal, Target, Timer } from 'lucide-react'
import { useState } from 'react'
import type { Acwr, BestEffort, TargetPace } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtDuration, fmtKm, fmtNumber, fmtPace, isNum } from '../../lib/format'
import { ACWR_VERDICT } from '../../lib/labels'
import { Badge } from '../ui/Badge'
import { Card } from '../ui/Card'
import { SegmentedControl } from '../ui/SegmentedControl'
import { NA } from '../ui/Value'

/* ------------------------------------------------------------------ */
/* Zones FC / puissance                                               */
/* ------------------------------------------------------------------ */

/** Temps en zones Z1–Z5 (rampe ordinale de la couleur d'accent). */
export function ZonesCard({ hr, power, className }: { hr: number[] | null; power: number[] | null; className?: string }) {
  const [kind, setKind] = useState<'hr' | 'power'>('hr')
  const zones = kind === 'hr' ? hr : power
  const total = zones?.reduce((s, v) => s + v, 0) ?? 0
  const max = zones ? Math.max(...zones, 1) : 1
  const z45 = zones ? zones[3] + zones[4] : null
  return (
    <Card
      title="Temps en zones"
      icon={<Timer className="size-4" />}
      className={className}
      action={
        power ? (
          <SegmentedControl
            ariaLabel="Type de zones"
            size="sm"
            className="w-36"
            value={kind}
            onChange={setKind}
            options={[
              { value: 'hr', label: 'FC' },
              { value: 'power', label: 'Puiss.' },
            ]}
          />
        ) : undefined
      }
    >
      {!zones || total === 0 ? (
        <p className="text-sm text-ink-3">
          <NA /> — zones non disponibles.
        </p>
      ) : (
        <>
          <ul className="space-y-2.5">
            {zones.map((s, i) => {
              const pct = total ? (s / total) * 100 : 0
              return (
                <li key={i} className="grid grid-cols-[2rem_minmax(0,1fr)_3.5rem_2.75rem] items-center gap-2 text-sm">
                  <span className="font-semibold text-ink-2">Z{i + 1}</span>
                  <span className="relative h-5 rounded-[6px] bg-track" role="img" aria-label={`Zone ${i + 1} : ${fmtDuration(s)}, ${Math.round(pct)} %`}>
                    <span
                      className="absolute inset-y-0 left-0 rounded-r-[4px] rounded-l-[6px] transition-all duration-700"
                      style={{ width: `${(s / max) * 100}%`, minWidth: s > 0 ? 3 : 0, background: `color-mix(in oklab, var(--accent) ${40 + i * 15}%, var(--surface))` }}
                    />
                  </span>
                  <span className="num text-right font-semibold">{fmtDuration(s)}</span>
                  <span className="num text-right text-xs text-ink-3">{Math.round(pct)} %</span>
                </li>
              )
            })}
          </ul>
          {z45 !== null && (
            <p className="num mt-3 border-t border-line pt-3 text-sm text-ink-2">
              Z4 + Z5 : <strong className="font-semibold text-ink">{fmtDuration(z45)}</strong> ({Math.round((z45 / total) * 100)} %)
            </p>
          )}
        </>
      )}
    </Card>
  )
}

/* ------------------------------------------------------------------ */
/* ACWR                                                               */
/* ------------------------------------------------------------------ */

const ACWR_MAX = 2
const ZONES: { from: number; to: number; color: string; label: string }[] = [
  { from: 0, to: 0.8, color: 'var(--info)', label: 'faible' },
  { from: 0.8, to: 1.3, color: 'var(--ok)', label: 'optimal' },
  { from: 1.3, to: 1.5, color: 'var(--warn)', label: 'élevé' },
  { from: 1.5, to: ACWR_MAX, color: 'var(--danger)', label: 'danger' },
]

/** Jauge ACWR (charge aiguë 7 j / chronique 28 j). */
export function AcwrCard({ acwr, className }: { acwr: Acwr | null; className?: string }) {
  const v = acwr ? ACWR_VERDICT[acwr.verdict] : null
  const pos = (r: number) => `${(Math.min(ACWR_MAX, Math.max(0, r)) / ACWR_MAX) * 100}%`
  return (
    <Card title="Charge aiguë / chronique" icon={<Gauge className="size-4" />} className={className} action={v ? <Badge level={v.level}>{v.label}</Badge> : undefined}>
      {!acwr ? (
        <p className="text-sm text-ink-3">
          <NA /> — historique de charge insuffisant.
        </p>
      ) : (
        <>
          <div className="flex items-end justify-between gap-3">
            <p className="text-4xl font-semibold tracking-tight">{isNum(acwr.ratio) ? <span className="num">{fmtNumber(acwr.ratio, 2)}</span> : <NA />}</p>
            <dl className="num flex gap-4 pb-1 text-right text-sm">
              <div>
                <dt className="text-xs text-ink-3">Aiguë 7 j</dt>
                <dd className="font-semibold">{fmtNumber(acwr.acute7d)}</dd>
              </div>
              <div>
                <dt className="text-xs text-ink-3">Chronique</dt>
                <dd className="font-semibold">{fmtNumber(acwr.chronic28d)}</dd>
              </div>
            </dl>
          </div>
          <div className="relative mt-5" role="img" aria-label={`Ratio ${isNum(acwr.ratio) ? fmtNumber(acwr.ratio, 2) : 'non disponible'} : zone ${v?.label ?? ''}. Optimal entre 0,8 et 1,3, danger au-delà de 1,5.`}>
            <div className="flex h-3 gap-[2px]">
              {ZONES.map((z) => (
                <div
                  key={z.label}
                  className={cn('h-full first:rounded-l-full last:rounded-r-full transition-opacity')}
                  style={{ width: `${((z.to - z.from) / ACWR_MAX) * 100}%`, background: z.color, opacity: acwr.ratio !== null && acwr.ratio >= z.from && acwr.ratio < z.to ? 0.95 : 0.3 }}
                />
              ))}
            </div>
            {isNum(acwr.ratio) && (
              <div className="absolute top-1/2 size-5 -translate-x-1/2 -translate-y-1/2 rounded-full border-[3px] border-surface bg-ink shadow-md transition-all duration-700" style={{ left: pos(acwr.ratio) }} />
            )}
          </div>
          <div className="num relative mt-1.5 h-4 text-[10px] text-ink-3" aria-hidden="true">
            {[0.8, 1.3, 1.5].map((t) => (
              <span key={t} className="absolute -translate-x-1/2" style={{ left: pos(t) }}>
                {fmtNumber(t, 1)}
              </span>
            ))}
          </div>
          <ul className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-ink-3">
            {ZONES.map((z) => (
              <li key={z.label} className="flex items-center gap-1">
                <span className="size-2 rounded-full" style={{ background: z.color }} aria-hidden="true" />
                {z.label}
              </li>
            ))}
          </ul>
        </>
      )}
    </Card>
  )
}

/* ------------------------------------------------------------------ */
/* Allures cibles & meilleurs efforts                                 */
/* ------------------------------------------------------------------ */

export function TargetPacesCard({ paces, className }: { paces: TargetPace[]; className?: string }) {
  return (
    <Card title="Allures cibles" icon={<Target className="size-4" />} subtitle="Temps passé dans chaque fourchette" className={className}>
      {paces.length === 0 ? (
        <p className="text-sm text-ink-3">Aucune allure cible définie.</p>
      ) : (
        <ul className="space-y-2.5">
          {paces.map((p) => (
            <li key={p.label} className="glass-inset flex items-center gap-3 px-3.5 py-3">
              <div className="min-w-0 flex-1">
                <p className="text-sm font-semibold">{p.label}</p>
                <p className="num text-xs text-ink-3">{p.rangeLabel} /km</p>
              </div>
              <div className="num text-right">
                <p className={cn('text-xl font-semibold tracking-tight', p.seconds === 0 && 'text-ink-3')}>{fmtDuration(p.seconds)}</p>
                <p className="text-xs text-ink-3">{p.distanceM > 0 ? `sur ${fmtKm(p.distanceM)}` : 'non travaillée'}</p>
              </div>
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}

const EFFORT_METERS: Record<string, number> = { '1 km': 1000, '1 mile': 1609, '5 km': 5000, '10 km': 10000 }

export function BestEffortsCard({ efforts, className }: { efforts: BestEffort[]; className?: string }) {
  return (
    <Card title="Meilleurs efforts" icon={<Medal className="size-4" />} className={className}>
      {efforts.length === 0 ? (
        <p className="text-sm text-ink-3">Aucun meilleur effort disponible pour cette séance.</p>
      ) : (
        <ul className="grid grid-cols-3 gap-2">
          {efforts.map((e) => {
            const m = EFFORT_METERS[e.label]
            return (
              <li key={e.label} className="glass-inset px-3 py-2.5 text-center">
                <p className="text-xs text-ink-3">{e.label}</p>
                <p className="num text-lg font-semibold tracking-tight">{fmtDuration(e.seconds)}</p>
                {m && <p className="num text-[11px] text-ink-3">{fmtPace(e.seconds / (m / 1000))}</p>}
              </li>
            )
          })}
        </ul>
      )}
    </Card>
  )
}
