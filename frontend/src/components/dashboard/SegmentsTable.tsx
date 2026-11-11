import { ListOrdered } from 'lucide-react'
import { useState } from 'react'
import type { Segment, Segments } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtDuration, fmtKm, fmtNumber, fmtPace } from '../../lib/format'
import { Card } from '../ui/Card'
import { SegmentedControl } from '../ui/SegmentedControl'
import { NA } from '../ui/Value'
import { isPause } from './segments'
import { t } from '../../i18n'

function paceCell(s: Segment, v: number | null) {
  if (s.kind === 'walk') return <span className="text-ink-3">{t('dashboard.cards.walk')}</span>
  if (s.kind === 'stand') return <span className="text-ink-3">{t('dashboard.cards.stand')}</span>
  return v === null ? <NA /> : fmtPace(v)
}
const num = (v: number | null, unit: string, d = 0) => (v === null ? <NA /> : `${fmtNumber(v, d)} ${unit}`)
const dash = (s: Segment, v: number | null, unit: string, d = 0) => (isPause(s) && v === null ? <span className="text-ink-3">—</span> : num(v, unit, d))
const elev = (s: Segment) => (s.elevGainM === null && s.elevLossM === null ? <NA /> : `+${fmtNumber(s.elevGainM ?? 0)} / −${fmtNumber(s.elevLossM ?? 0)} m`)

/** Blocks / laps table (with GAP); stacked cards on mobile. */
export function SegmentsTable({ segments, className }: { segments: Segments; className?: string }) {
  const hasBlocks = segments.blocks.length > 0
  const [tab, setTab] = useState<'blocks' | 'laps'>(hasBlocks ? segments.primary : 'laps')
  const rows = tab === 'blocks' && hasBlocks ? segments.blocks : segments.laps
  const total = rows.reduce((s, r) => s + r.durationS, 0)
  const heads = (['segment', 'duration', 'dist', 'pace', 'gap', 'hr', 'watts', 'cadence', 'gct', 'vo', 'elev'] as const).map((k) => ({ k, label: t(`dashboard.cards.segHeads.${k}`) }))

  return (
    <Card
      title={t('dashboard.cards.segTitle')}
      icon={<ListOrdered className="size-4" />}
      subtitle={tab === segments.primary ? t('dashboard.cards.segPrompt') : undefined}
      className={className}
      action={
        hasBlocks ? (
          <SegmentedControl
            ariaLabel={t('dashboard.cards.segType')}
            size="sm"
            className="w-44"
            value={tab}
            onChange={setTab}
            options={[
              { value: 'blocks', label: t('dashboard.cards.blocks', { count: segments.blocks.length }) },
              { value: 'laps', label: t('dashboard.cards.laps', { count: segments.laps.length }) },
            ]}
          />
        ) : undefined
      }
    >
      {rows.length === 0 ? (
        <p className="text-sm text-ink-3">{t('dashboard.cards.segEmpty')}</p>
      ) : (
        <>
          {tab === 'blocks' && (
            <div className="mb-3 flex h-2.5 w-full gap-[2px] overflow-hidden rounded-full" aria-hidden="true">
              {rows.map((r) => (
                <div key={r.index} className={cn('h-full rounded-[3px]', isPause(r) ? 'bg-ink-3/35' : 'bg-accent')} style={{ width: `${(r.durationS / total) * 100}%` }} title={r.label} />
              ))}
            </div>
          )}
          <div className="scroll-thin -mx-1 hidden overflow-x-auto md:block">
            <table className="num w-full min-w-[760px] border-separate border-spacing-0 text-sm whitespace-nowrap">
              <caption className="sr-only">{tab === 'blocks' ? t('dashboard.cards.blocksCaption') : t('dashboard.cards.lapsCaption')}</caption>
              <thead>
                <tr className="text-left text-xs text-ink-3">
                  {heads.map((h) => (
                    <th key={h.k} scope="col" className="border-b border-line px-2 py-2 font-medium first:pl-1">
                      {h.k === 'gap' ? <abbr title={t('dashboard.cards.gapAbbr')}>GAP</abbr> : h.label}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr key={r.index} className={cn('transition-colors hover:bg-track/60', isPause(r) && 'text-ink-2')}>
                    <th scope="row" className="border-b border-line px-2 py-2 pl-1 text-left font-semibold">
                      <span className="flex items-center gap-2">
                        <span aria-hidden="true" className={cn('h-4 w-1 rounded-full', isPause(r) ? 'bg-ink-3/40' : 'bg-accent')} />
                        {r.label}
                      </span>
                    </th>
                    <td className="border-b border-line px-2 py-2">{fmtDuration(r.durationS)}</td>
                    <td className="border-b border-line px-2 py-2">{fmtKm(r.distanceM)}</td>
                    <td className="border-b border-line px-2 py-2 font-semibold text-ink">{paceCell(r, r.paceSecPerKm)}</td>
                    <td className="border-b border-line px-2 py-2">{isPause(r) ? <span className="text-ink-3">—</span> : paceCell(r, r.gapSecPerKm)}</td>
                    <td className="border-b border-line px-2 py-2">{num(r.avgHR, 'bpm')}</td>
                    <td className="border-b border-line px-2 py-2">{dash(r, r.avgPower, 'W')}</td>
                    <td className="border-b border-line px-2 py-2">{dash(r, r.avgCadenceSpm, 'spm')}</td>
                    <td className="border-b border-line px-2 py-2">{dash(r, r.gctMs, 'ms')}</td>
                    <td className="border-b border-line px-2 py-2">{dash(r, r.voCm, 'cm', 1)}</td>
                    <td className="border-b border-line px-2 py-2">{elev(r)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <ul className="space-y-2 md:hidden">
            {rows.map((r) => (
              <li key={r.index} className={cn('glass-inset px-3 py-2.5', isPause(r) && 'opacity-80')}>
                <div className="flex items-center justify-between gap-2">
                  <p className="flex min-w-0 items-center gap-2 text-sm font-semibold">
                    <span aria-hidden="true" className={cn('h-4 w-1 flex-none rounded-full', isPause(r) ? 'bg-ink-3/40' : 'bg-accent')} />
                    <span className="truncate">{r.label}</span>
                  </p>
                  <p className="num flex-none text-sm font-semibold">{paceCell(r, r.paceSecPerKm)}</p>
                </div>
                <dl className="num mt-1.5 grid grid-cols-3 gap-x-2 gap-y-1 text-xs xs:grid-cols-4">
                  {[
                    [t('dashboard.cards.segHeads.duration'), fmtDuration(r.durationS)],
                    [t('dashboard.cards.segHeads.dist'), fmtKm(r.distanceM)],
                    ['GAP', isPause(r) ? '—' : paceCell(r, r.gapSecPerKm)],
                    [t('dashboard.cards.segShort.hr'), num(r.avgHR, 'bpm')],
                    [t('dashboard.cards.segShort.cad'), dash(r, r.avgCadenceSpm, 'spm')],
                    ['GCT', dash(r, r.gctMs, 'ms')],
                    [t('dashboard.cards.segHeads.vo'), dash(r, r.voCm, 'cm', 1)],
                    [t('dashboard.cards.segHeads.elev'), elev(r)],
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
