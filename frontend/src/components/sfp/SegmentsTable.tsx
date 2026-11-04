import { ListOrdered } from 'lucide-react'
import { useState } from 'react'
import type { Segment, Segments } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtDuration, fmtKm, fmtNumber, fmtPace } from '../../lib/format'
import { Card } from '../ui/Card'
import { SegmentedControl } from '../ui/SegmentedControl'
import { NA } from '../ui/Value'

const PAUSE_KINDS = new Set(['walk', 'stand', 'rest'])
export const isPause = (s: Segment) => PAUSE_KINDS.has(s.kind)

function paceCell(s: Segment) {
  if (s.kind === 'walk') return <span className="text-ink-3">Marche</span>
  if (s.kind === 'stand') return <span className="text-ink-3">Arrêt</span>
  return s.paceSecPerKm === null ? <NA /> : fmtPace(s.paceSecPerKm)
}

const num = (v: number | null, unit: string, d = 0) => (v === null ? <NA /> : `${fmtNumber(v, d)} ${unit}`)
const dash = (s: Segment, v: number | null, unit: string, d = 0) => (isPause(s) && v === null ? <span className="text-ink-3">—</span> : num(v, unit, d))

/** Tableau des segments (onglets Blocs / Laps) ; cartes empilées sur mobile. */
export function SegmentsTable({ segments, className, showDynamics = true }: { segments: Segments; className?: string; showDynamics?: boolean }) {
  const hasBlocks = segments.blocks.length > 0
  const [tab, setTab] = useState<'blocks' | 'laps'>(hasBlocks ? segments.primary : 'laps')
  const rows = tab === 'blocks' ? segments.blocks : segments.laps
  const total = rows.reduce((s, r) => s + r.durationS, 0)

  return (
    <Card
      title="Détail des intervalles"
      icon={<ListOrdered className="size-4" />}
      subtitle={tab === segments.primary ? 'Utilisé dans le rapport' : undefined}
      className={className}
      action={
        hasBlocks ? (
          <SegmentedControl
            ariaLabel="Type de segments"
            size="sm"
            className="w-44"
            value={tab}
            onChange={setTab}
            options={[
              { value: 'blocks', label: `Blocs (${segments.blocks.length})` },
              { value: 'laps', label: `Laps (${segments.laps.length})` },
            ]}
          />
        ) : undefined
      }
    >
      {rows.length === 0 ? (
        <p className="text-sm text-ink-3">Aucun segment pour cette activité.</p>
      ) : (
        <>
          {/* Frise course / marche */}
          {tab === 'blocks' && (
            <div className="mb-3 flex h-2.5 w-full gap-[2px] overflow-hidden rounded-full" aria-hidden="true">
              {rows.map((r) => (
                <div key={r.index} className={cn('h-full rounded-[3px]', isPause(r) ? 'bg-ink-3/35' : 'bg-gradient-to-r from-accent to-accent-2')} style={{ width: `${(r.durationS / total) * 100}%` }} title={r.label} />
              ))}
            </div>
          )}

          {/* Desktop : tableau */}
          <div className="scroll-thin -mx-1 hidden overflow-x-auto md:block">
            <table className="num w-full min-w-[680px] border-separate border-spacing-0 text-sm whitespace-nowrap">
              <caption className="sr-only">{tab === 'blocks' ? 'Blocs course / marche' : 'Laps montre'}</caption>
              <thead>
                <tr className="text-left text-xs text-ink-3">
                  {['Segment', 'Durée', 'Distance', 'Allure', 'FC moy', 'Watts', 'Cadence', ...(showDynamics ? ['GCT', 'OV'] : []), 'D−'].map((h) => (
                    <th key={h} scope="col" className="border-b border-line px-2 py-2 font-medium first:pl-1">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr key={r.index} className={cn('transition-colors hover:bg-track/60', isPause(r) && 'text-ink-2')}>
                    <th scope="row" className="border-b border-line px-2 py-2 pl-1 text-left font-semibold whitespace-nowrap">
                      <span className="flex items-center gap-2">
                        <span aria-hidden="true" className={cn('h-4 w-1 rounded-full', isPause(r) ? 'bg-ink-3/40' : 'bg-accent')} />
                        {r.label}
                      </span>
                    </th>
                    <td className="border-b border-line px-2 py-2">{fmtDuration(r.durationS)}</td>
                    <td className="border-b border-line px-2 py-2">{fmtKm(r.distanceM)}</td>
                    <td className="border-b border-line px-2 py-2 font-semibold text-ink">{paceCell(r)}</td>
                    <td className="border-b border-line px-2 py-2">{num(r.avgHR, 'bpm')}</td>
                    <td className="border-b border-line px-2 py-2">{isPause(r) ? <span className="text-ink-3">—</span> : num(r.avgPower, 'W')}</td>
                    <td className="border-b border-line px-2 py-2">{isPause(r) ? <span className="text-ink-3">—</span> : num(r.avgCadenceSpm, 'spm')}</td>
                    {showDynamics && (
                      <>
                        <td className="border-b border-line px-2 py-2">{dash(r, r.gctMs, 'ms')}</td>
                        <td className="border-b border-line px-2 py-2">{dash(r, r.voCm, 'cm', 1)}</td>
                      </>
                    )}
                    <td className="border-b border-line px-2 py-2">{r.elevLossM === null ? <NA /> : `${r.elevLossM} m`}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Mobile : cartes empilées */}
          <ul className="space-y-2 md:hidden">
            {rows.map((r) => (
              <li key={r.index} className={cn('glass-inset px-3 py-2.5', isPause(r) && 'opacity-80')}>
                <div className="flex items-center justify-between gap-2">
                  <p className="flex items-center gap-2 text-sm font-semibold">
                    <span aria-hidden="true" className={cn('h-4 w-1 rounded-full', isPause(r) ? 'bg-ink-3/40' : 'bg-accent')} />
                    {r.label}
                  </p>
                  <p className="num text-sm font-semibold">{paceCell(r)}</p>
                </div>
                <dl className="num mt-1.5 grid grid-cols-4 gap-1 text-xs">
                  {[
                    ['Durée', fmtDuration(r.durationS)],
                    ['Dist.', fmtKm(r.distanceM)],
                    ['FC', num(r.avgHR, 'bpm')],
                    [isPause(r) ? 'D−' : 'Cad.', isPause(r) ? (r.elevLossM === null ? <NA key="na" /> : `${r.elevLossM} m`) : num(r.avgCadenceSpm, 'spm')],
                  ].map(([k, v], i) => (
                    <div key={i} className="min-w-0">
                      <dt className="text-ink-3">{k}</dt>
                      <dd className="truncate font-medium text-ink">{v}</dd>
                    </div>
                  ))}
                </dl>
              </li>
            ))}
          </ul>
        </>
      )}
    </Card>
  )
}
