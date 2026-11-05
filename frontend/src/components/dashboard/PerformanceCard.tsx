import { Medal, Timer } from 'lucide-react'
import type { Performance } from '../../api/types'
import { fmtDuration, fmtKm, fmtNumber } from '../../lib/format'
import { Card } from '../ui/Card'
import { NA } from '../ui/Value'

export function PerformanceCard({ perf, className }: { perf: Performance | null; className?: string }) {
  const zones = perf?.timeInZonesS ?? null
  const total = zones?.reduce((s, v) => s + v, 0) ?? 0
  const max = zones ? Math.max(...zones, 1) : 1
  return (
    <Card title="Intensité & performance" icon={<Timer className="size-4" />} className={className}>
      {!perf ? (
        <p className="text-sm text-ink-3">
          <NA reason="non calculée pour cette activité" />
        </p>
      ) : (
        <div className="space-y-4">
          <div>
            <p className="label-caps mb-2">Temps en zones FC</p>
            {!zones || total === 0 ? (
              <NA reason="zones non disponibles" />
            ) : (
              <ul className="space-y-1.5">
                {zones.map((s, i) => {
                  const pct = (s / total) * 100
                  return (
                    <li key={i} className="grid grid-cols-[1.75rem_minmax(0,1fr)_3.25rem_2.5rem] items-center gap-2 text-sm">
                      <span className="font-semibold text-ink-2">Z{i + 1}</span>
                      <span className="relative h-4 rounded-[5px] bg-track" role="img" aria-label={`Zone ${i + 1} : ${fmtDuration(s)}, ${Math.round(pct)} %`}>
                        <span className="absolute inset-y-0 left-0 rounded-[5px]" style={{ width: `${(s / max) * 100}%`, minWidth: s > 0 ? 3 : 0, background: `color-mix(in oklab, var(--accent) ${40 + i * 15}%, var(--surface))` }} />
                      </span>
                      <span className="num text-right font-semibold">{fmtDuration(s)}</span>
                      <span className="num text-right text-xs text-ink-3">{Math.round(pct)} %</span>
                    </li>
                  )
                })}
              </ul>
            )}
            {perf.z4z5S !== null && total > 0 && <p className="num mt-1.5 text-xs text-ink-3">Z4 + Z5 : {fmtDuration(perf.z4z5S)}</p>}
          </div>
          {perf.targetPaces.length > 0 && (
            <div>
              <p className="label-caps mb-1.5">Temps aux allures cibles</p>
              <ul className="space-y-1 text-sm">
                {perf.targetPaces.map((t) => (
                  <li key={t.label} className="flex items-baseline justify-between gap-2">
                    <span className="min-w-0 truncate">
                      {t.label} <span className="num text-xs text-ink-3">{t.rangeLabel}</span>
                    </span>
                    <span className="num flex-none font-semibold">
                      {t.seconds > 0 ? `${fmtDuration(t.seconds)} · ${fmtKm(t.distanceM, 1)}` : <span className="font-normal text-ink-3">—</span>}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          <div>
            <p className="label-caps mb-1.5 flex items-center gap-1.5">
              <Medal className="size-3.5" aria-hidden="true" /> Meilleurs efforts
            </p>
            {perf.bestEfforts.length === 0 ? (
              <p className="text-sm text-ink-3">Aucun effort de référence sur cette séance.</p>
            ) : (
              <ul className="flex flex-wrap gap-2">
                {perf.bestEfforts.map((b) => (
                  <li key={b.label} className="glass-inset rounded-xl px-2.5 py-1.5 text-sm">
                    <span className="text-xs text-ink-3">{b.label}</span> <span className="num font-semibold">{fmtDuration(b.seconds)}</span>
                  </li>
                ))}
              </ul>
            )}
            {(perf.avgPower !== null || perf.normPower !== null) && (
              <p className="num mt-2 text-xs text-ink-3">
                Puissance moy {perf.avgPower !== null ? `${fmtNumber(perf.avgPower)} W` : 'N/D'} · NP {perf.normPower !== null ? `${fmtNumber(perf.normPower)} W` : 'N/D'}
              </p>
            )}
          </div>
        </div>
      )}
    </Card>
  )
}
