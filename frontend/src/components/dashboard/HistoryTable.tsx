import { History } from 'lucide-react'
import { Link } from 'wouter'
import type { HistoryRow } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtDayMonth, fmtDuration, fmtKm, fmtNumber, fmtPace, fmtWeekdayShort, isNum } from '../../lib/format'
import { Card } from '../ui/Card'
import { t } from '../../i18n'

const pain = (v: number | null) =>
  isNum(v) ? <span className={cn('font-semibold', v >= 5 ? 'text-danger' : v >= 3 ? 'text-warn' : 'text-ok')}>{fmtNumber(v, 1, { trim: true })}</span> : <span className="text-ink-3">—</span>

/** 14 days before (and including) the session, with shoes and pains. */
export function HistoryTable({ rows, currentId, className }: { rows: HistoryRow[]; currentId: number; className?: string }) {
  return (
    <Card title={t('dashboard.cards.historyTitle')} icon={<History className="size-4" />} subtitle={t('dashboard.cards.historySubtitle')} className={className}>
      {rows.length === 0 ? (
        <p className="text-sm text-ink-3">{t('dashboard.cards.historyEmpty')}</p>
      ) : (
        <div className="scroll-thin -mx-1 overflow-x-auto px-1">
          <table className="num w-full min-w-[820px] border-separate border-spacing-0 text-sm whitespace-nowrap">
            <caption className="sr-only">{t('dashboard.cards.historyCaption')}</caption>
            <thead>
              <tr className="text-left text-xs text-ink-3">
                {(['date', 'type', 'dist', 'duration', 'pace', 'cadence', 'gct', 'loss', 'load', 'shoes', 'painDuring', 'cold', 'nextDay'] as const).map((k) => t(`dashboard.cards.historyHeads.${k}`)).map((h) => (
                  <th key={h} scope="col" className="border-b border-line px-2 py-2 font-medium first:pl-1">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => {
                const cur = r.activityId === currentId
                return (
                  <tr key={r.activityId} className={cn('transition-colors hover:bg-track/60', cur && 'bg-accent/8')}>
                    <th scope="row" className="border-b border-line px-2 py-1.5 pl-1 text-left font-semibold">
                      <Link href={`/?s=${r.activityId}`} className="rounded-md capitalize underline-offset-2 hover:underline" aria-current={cur ? 'true' : undefined}>
                        {fmtWeekdayShort(r.date)} {fmtDayMonth(r.date)}
                      </Link>
                    </th>
                    <td className="border-b border-line px-2 py-1.5">{r.typeLabel}</td>
                    <td className="border-b border-line px-2 py-1.5">{fmtKm(r.distanceM, 1)}</td>
                    <td className="border-b border-line px-2 py-1.5">{fmtDuration(r.durationS)}</td>
                    <td className="border-b border-line px-2 py-1.5">{fmtPace(r.paceSecPerKm)}</td>
                    <td className="border-b border-line px-2 py-1.5">{isNum(r.avgCadenceSpm) ? r.avgCadenceSpm : '—'}</td>
                    <td className="border-b border-line px-2 py-1.5">{isNum(r.gctMs) ? `${r.gctMs} ms` : '—'}</td>
                    <td className="border-b border-line px-2 py-1.5">{isNum(r.elevLossM) ? `${r.elevLossM} m` : '—'}</td>
                    <td className="border-b border-line px-2 py-1.5">{isNum(r.trainingLoad) ? r.trainingLoad : '—'}</td>
                    <td className="max-w-[10rem] truncate border-b border-line px-2 py-1.5">{r.shoeName ?? <span className="text-ink-3">—</span>}</td>
                    <td className="border-b border-line px-2 py-1.5 text-center">{pain(r.painDuring)}</td>
                    <td className="border-b border-line px-2 py-1.5 text-center">{pain(r.painCold)}</td>
                    <td className="border-b border-line px-2 py-1.5 text-center">{pain(r.painNextMorning)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  )
}
