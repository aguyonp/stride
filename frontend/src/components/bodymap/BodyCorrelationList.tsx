import { Footprints } from 'lucide-react'
import type { ReactNode } from 'react'
import type { BodyCorrelationFactor } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtSigned, isNum } from '../../lib/format'
import { Insufficient } from '../ui/Value'
import {
  correlationConfidence,
  correlationSample,
  fmtMeasure,
  isCorrelationInsufficient,
  shoeHighlightDetail,
} from './bodyMapMeta'

function Metric({ label, value, className }: { label: string; value: string | null; className?: string }) {
  return (
    <div>
      <dt className="text-ink-3">{label}</dt>
      <dd className={cn('num mt-0.5', className)}>{value ?? <Insufficient />}</dd>
    </div>
  )
}

function CorrelationItem({ c }: { c: BodyCorrelationFactor }) {
  const insufficient = isCorrelationInsufficient(c)
  const conf = correlationConfidence(c)
  const notable = isNum(c.deltaPct) && Math.abs(c.deltaPct) >= 10
  const shoe = c.shoeHighlight ? shoeHighlightDetail(c.shoeHighlight) : ''

  const body: ReactNode = insufficient ? (
    <p className="mt-1.5">
      <Insufficient />
    </p>
  ) : (
    <dl className="mt-2.5 grid grid-cols-2 gap-2 rounded-xl border border-line bg-track/30 px-3 py-2 text-xs sm:grid-cols-4">
      <Metric label="Moyenne avec gêne" value={fmtMeasure(c.symptomaticMean, c.unit)} className="font-semibold text-ink" />
      <Metric label="Référence sans gêne" value={fmtMeasure(c.baselineMean, c.unit)} className="font-medium text-ink-2" />
      <Metric
        label="Écart relatif"
        value={isNum(c.deltaPct) ? `${fmtSigned(c.deltaPct, 1)} %` : null}
        className={cn('font-semibold', notable ? 'text-warn' : 'text-ink-2')}
      />
      <Metric label="Échantillon" value={correlationSample(c)} className="text-ink-2" />
    </dl>
  )

  return (
    <li className="py-3.5 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-sm font-semibold text-ink">{c.factorLabel}</h3>
            <span className="chip h-5 border border-line bg-track/60 px-2 text-[11px] text-ink-2">{c.zoneLabel}</span>
          </div>
          {!insufficient && c.summary && <p className="mt-1 text-xs text-ink-2">{c.summary}</p>}
        </div>
        {conf && <span className={cn('chip h-6 border px-2.5 text-[11px]', conf.cls)}>{conf.label}</span>}
      </div>

      {body}

      {!insufficient && c.shoeHighlight && (
        <div className="mt-2 flex flex-wrap items-center gap-2 rounded-xl border border-warn/25 bg-warn/[0.06] px-3 py-1.5 text-xs">
          <Footprints className="size-3.5 flex-none text-warn" aria-hidden="true" />
          <span className="font-medium text-ink">{c.shoeHighlight.shoeName}</span>
          {shoe && <span className="num text-ink-2">{shoe}</span>}
        </div>
      )}
    </li>
  )
}

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
      {correlations.map((c, idx) => (
        <CorrelationItem key={`${c.zone}-${c.factor}-${idx}`} c={c} />
      ))}
    </ul>
  )
}
