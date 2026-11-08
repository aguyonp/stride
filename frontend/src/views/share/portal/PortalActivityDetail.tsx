import { MessageSquarePlus, MessageSquareQuote } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { usePublicActivity } from '../../../api/queriesV42'
import type { PublicActivityDetail as Detail } from '../../../api/types'
import { painScoreTone } from '../../../components/bodymap/bodyMapMeta'
import { authorLine } from '../../../components/share/shareNotes'
import { Dialog } from '../../../components/ui/Dialog'
import { InlineError } from '../../../components/ui/Misc'
import { Skeleton } from '../../../components/ui/Skeleton'
import { cn } from '../../../lib/cn'
import { capitalize, fmtDateLong, fmtDuration, fmtKm, fmtNumber, fmtPace, fmtTimeOfDay, isNum } from '../../../lib/format'
import { RemarkEditor } from './PortalEditors'
import { t } from '../../../i18n'

const n = (v: number | null | undefined, unit = '', d = 0) => (isNum(v) ? `${fmtNumber(v, d)}${unit}` : null)

function Metric({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="glass-inset min-w-0 px-3 py-2">
      <dt className="truncate text-[11px] text-ink-3">{label}</dt>
      <dd className="num text-sm font-semibold break-words text-ink">{value ?? 'N/D'}</dd>
    </div>
  )
}

function Body({ d, token, canRemark }: { d: Detail; token: string; canRemark: boolean }) {
  const [editing, setEditing] = useState(false)
  const m = d
  const balance = isNum(m.balanceLeftPct) ? `${fmtNumber(m.balanceLeftPct, 1)} / ${fmtNumber(m.balanceRightPct ?? 100 - m.balanceLeftPct, 1)} %` : null
  const weather = m.weather ? [n(m.weather.temperatureC, ' °C', 1), n(m.weather.windKph, ' km/h'), m.weather.description].filter(Boolean).join(' · ') : null
  const painMax = Math.max(d.painDuring ?? 0, d.painNextMorning ?? 0)
  const pain = isNum(d.painDuring) || isNum(d.painNextMorning) ? { during: d.painDuring, nextMorning: d.painNextMorning } : null
  return (
    <div className="space-y-5">
      <p className="text-sm text-ink-3">
        {capitalize(fmtDateLong(d.date))} · {fmtTimeOfDay(d.startLocal)} · {d.typeLabel}
      </p>
      <dl className="grid grid-cols-2 gap-2 sm:grid-cols-3">
        <Metric label={t('share.portal.distance')} value={isNum(d.distanceM) ? fmtKm(d.distanceM) : null} />
        <Metric label={t('share.portal.duration')} value={isNum(d.durationS) ? fmtDuration(d.durationS) : null} />
        <Metric label={t('share.portal.pace')} value={isNum(m.avgPaceSecPerKm) ? `${fmtPace(m.avgPaceSecPerKm)}${isNum(m.gapSecPerKm) ? ` (${fmtPace(m.gapSecPerKm)})` : ''}` : null} />
        <Metric label={t('share.portal.hr')} value={isNum(m.avgHR) ? `${fmtNumber(m.avgHR)}${isNum(m.maxHR) ? ` / ${fmtNumber(m.maxHR)}` : ''} bpm` : null} />
        <Metric label={t('share.public.ef')} value={n(m.efSpeed, '', 2)} />
        <Metric label="Cadence" value={n(m.avgCadenceSpm, ' spm')} />
        <Metric label={t('share.portal.gct')} value={n(m.gctMs, ' ms')} />
        <Metric label={t('share.portal.vo')} value={n(m.verticalOscillationCm, ' cm', 1)} />
        <Metric label={t('share.portal.balance')} value={balance} />
        <Metric label="D+ / D−" value={isNum(m.elevGainM) || isNum(m.elevLossM) ? `${n(m.elevGainM, ' m') ?? '—'} / ${n(m.elevLossM, ' m') ?? '—'}` : null} />
        <Metric label="Impact Load" value={n(m.impactLoad, ' pts')} />
        <Metric label={t('share.portal.weather')} value={weather} />
      </dl>

      {(d.shoe || pain) && (
        <div className="grid gap-2 sm:grid-cols-2">
          {d.shoe && (
            <div className="glass-inset px-3 py-2.5 text-sm">
              <p className="text-[11px] text-ink-3">{t('share.portal.shoe')}</p>
              <p className="font-semibold text-ink">{d.shoe.name}</p>
              <p className="num text-xs text-ink-3">{[isNum(d.shoe.dropMm) ? t('share.labels.dropMm', { value: fmtNumber(d.shoe.dropMm, 0) }) : null, isNum(d.shoe.distanceKm) ? t('share.labels.totalKm', { value: fmtNumber(d.shoe.distanceKm, 0) }) : null].filter(Boolean).join(' · ') || '—'}</p>
            </div>
          )}
          {pain && (
            <div className="glass-inset px-3 py-2.5 text-sm">
              <p className="text-[11px] text-ink-3">{t('share.portal.painTitle')}</p>
              <p className={cn('num font-semibold', painScoreTone(painMax).textCls)}>
                {isNum(pain.during) ? `${fmtNumber(pain.during, 1, { trim: true })}/10` : '—'} · {isNum(pain.nextMorning) ? `${fmtNumber(pain.nextMorning, 1, { trim: true })}/10` : '—'}
              </p>
            </div>
          )}
        </div>
      )}

      {d.laps.length > 0 && (
        <section aria-labelledby="detail-laps">
          <h3 id="detail-laps" className="mb-2 text-sm font-medium text-ink-2">
            {t('common.misc.laps')}
          </h3>
          <div className="scroll-thin -mx-1 overflow-x-auto px-1">
            <table className="w-full min-w-[520px] text-left text-xs">
              <thead>
                <tr className="border-b border-line text-ink-3">
                  <th className="pr-2 pb-2 font-medium">#</th>
                  <th className="px-2 pb-2 text-right font-medium">{t('share.portal.duration')}</th>
                  <th className="px-2 pb-2 text-right font-medium">{t('share.portal.distance')}</th>
                  <th className="px-2 pb-2 text-right font-medium">{t('share.portal.paceShort')}</th>
                  <th className="px-2 pb-2 text-right font-medium">FC</th>
                  <th className="px-2 pb-2 text-right font-medium">Cadence</th>
                  <th className="pb-2 pl-2 text-right font-medium">D+ / D−</th>
                </tr>
              </thead>
              <tbody className="num divide-y divide-line">
                {d.laps.map((l) => (
                  <tr key={l.index}>
                    <td className="py-1.5 pr-2 text-ink-3" title={l.label || undefined}>
                      {l.index}
                    </td>
                    <td className="px-2 py-1.5 text-right text-ink">{isNum(l.durationS) ? fmtDuration(l.durationS) : '—'}</td>
                    <td className="px-2 py-1.5 text-right text-ink-2">{isNum(l.distanceM) ? fmtKm(l.distanceM) : '—'}</td>
                    <td className="px-2 py-1.5 text-right text-ink">{fmtPace(l.paceSecPerKm)}</td>
                    <td className="px-2 py-1.5 text-right text-ink-2">{n(l.avgHR) ?? '—'}</td>
                    <td className="px-2 py-1.5 text-right text-ink-2">{n(l.avgCadenceSpm) ?? '—'}</td>
                    <td className="py-1.5 pl-2 text-right text-ink-2">
                      {n(l.elevGainM) ?? '—'} / {n(l.elevLossM) ?? '—'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {editing && canRemark ? (
        <RemarkEditor key={d.remark?.updatedAt ?? 'new'} token={token} activity={d} inline open onClose={() => setEditing(false)} />
      ) : d.remark ? (
        <div className="rounded-2xl border border-info/25 bg-info/[0.06] p-3">
          <p className="flex items-start gap-2 text-sm text-ink">
            <MessageSquareQuote className="mt-0.5 size-4 flex-none text-info" aria-hidden="true" />
            <span className="min-w-0 break-words whitespace-pre-line">{d.remark.body}</span>
          </p>
          <div className="mt-1.5 flex flex-wrap items-center justify-between gap-2 pl-6">
            <p className="text-xs text-ink-3">{authorLine(d.remark)}</p>
            {canRemark && (
              <button type="button" className="btn btn-ghost h-8 min-h-8 px-2.5 text-xs" onClick={() => setEditing(true)}>
                {t('share.portal.editRemark')}
              </button>
            )}
          </div>
        </div>
      ) : (
        canRemark && (
          <button type="button" className="btn btn-ghost" onClick={() => setEditing(true)}>
            <MessageSquarePlus className="size-4" aria-hidden="true" /> Ajouter une remarque
          </button>
        )
      )}
    </div>
  )
}

/** Session detail for the recipient (activityDetails): metrics, laps, shoe, pain and this link's remark. */
export function PortalActivityDetail({ token, id, canRemark, onClose }: { token: string; id: number; canRemark: boolean; onClose: () => void }) {
  const q = usePublicActivity(token, id)
  return (
    <Dialog open onClose={onClose} variant="drawer" title={q.data?.name ?? t('share.portal.detailTitle')}>
      {q.isError ? <InlineError>{q.error.message}</InlineError> : q.data ? <Body d={q.data} token={token} canRemark={canRemark} /> : <Skeleton className="h-96" />}
    </Dialog>
  )
}
