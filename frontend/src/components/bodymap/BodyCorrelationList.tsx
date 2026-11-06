import { Footprints } from 'lucide-react'
import type { BodyCorrelationFactor } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtNumber, fmtSigned } from '../../lib/format'
import { CONFIDENCE_META } from './bodyMapMeta'

export function BodyCorrelationList({
  correlations,
  emptyLabel = 'Pas encore assez de sorties associées pour isoler un facteur mécanique.',
}: {
  correlations: BodyCorrelationFactor[]
  emptyLabel?: string
}) {
  if (correlations.length === 0) {
    return <p className="text-sm text-ink-3">{emptyLabel}</p>
  }

  return (
    <ul className="divide-y divide-line" aria-label="Facteurs de corrélation biomécanique">
      {correlations.map((c, idx) => {
        const conf = CONFIDENCE_META[c.confidence]
        const notable = c.deltaPct !== null && Math.abs(c.deltaPct) >= 10
        return (
          <li key={`${c.zone}-${c.factor}-${idx}`} className="py-3.5 first:pt-0 last:pb-0">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h3 className="text-sm font-semibold text-ink">{c.factorLabel}</h3>
                  <span className="chip h-5 border border-line bg-track/60 px-2 text-[11px] text-ink-2">{c.zoneLabel}</span>
                </div>
                {c.summary && <p className="mt-1 text-xs text-ink-2">{c.summary}</p>}
              </div>
              <span className={cn('chip h-6 border px-2.5 text-[11px]', conf.cls)}>{conf.label}</span>
            </div>

            <dl className="mt-2.5 grid grid-cols-2 gap-2 rounded-xl border border-line bg-track/30 px-3 py-2 text-xs sm:grid-cols-4">
              <div>
                <dt className="text-ink-3">Moyenne avec gêne</dt>
                <dd className="num mt-0.5 font-semibold text-ink">
                  {c.symptomaticMean !== null ? `${fmtNumber(c.symptomaticMean, 1, { trim: true })} ${c.unit}` : 'N/D'}
                </dd>
              </div>
              <div>
                <dt className="text-ink-3">Référence sans gêne</dt>
                <dd className="num mt-0.5 font-medium text-ink-2">
                  {c.baselineMean !== null ? `${fmtNumber(c.baselineMean, 1, { trim: true })} ${c.unit}` : 'N/D'}
                </dd>
              </div>
              <div>
                <dt className="text-ink-3">Écart relatif</dt>
                <dd className={cn('num mt-0.5 font-semibold', notable ? 'text-warn' : 'text-ink-2')}>
                  {c.deltaPct !== null ? `${fmtSigned(c.deltaPct, 1)} %` : 'N/D'}
                </dd>
              </div>
              <div>
                <dt className="text-ink-3">Échantillon</dt>
                <dd className="num mt-0.5 text-ink-2">
                  {c.symptomaticRuns} avec gêne · {c.baselineRuns} réf.
                </dd>
              </div>
            </dl>

            {c.shoeHighlight && (
              <div className="mt-2 flex flex-wrap items-center gap-2 rounded-xl border border-warn/25 bg-warn/[0.06] px-3 py-1.5 text-xs">
                <Footprints className="size-3.5 flex-none text-warn" aria-hidden="true" />
                <span className="font-medium text-ink">{c.shoeHighlight.shoeName}</span>
                <span className="num text-ink-2">
                  {c.shoeHighlight.dropMm !== null ? `Drop ${fmtNumber(c.shoeHighlight.dropMm)} mm · ` : ''}
                  {fmtNumber(c.shoeHighlight.distanceKm, 0)} km · {c.shoeHighlight.painEpisodes} épisode
                  {c.shoeHighlight.painEpisodes > 1 ? 's' : ''}
                </span>
              </div>
            )}
          </li>
        )
      })}
    </ul>
  )
}
