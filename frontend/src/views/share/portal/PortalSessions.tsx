import { Activity, ChevronRight, Footprints, MessageSquarePlus, MessageSquareQuote } from 'lucide-react'
import { Fragment, useState } from 'react'
import type { PublicActivityItem, PublicShareView } from '../../../api/types'
import { painScoreTone } from '../../../components/bodymap/bodyMapMeta'
import { authorLine } from '../../../components/share/shareNotes'
import { Card } from '../../../components/ui/Card'
import { cn } from '../../../lib/cn'
import { fmtDateShort, fmtKm, fmtNumber, fmtPace, isNum } from '../../../lib/format'
import { PortalActivityDetail } from './PortalActivityDetail'
import { RemarkEditor } from './PortalEditors'
import { permissionsOf } from './portalUtils'
import { t } from '../../../i18n'

function PainBadge({ s }: { s: PublicActivityItem }) {
  if (!isNum(s.painDuring) && !isNum(s.painNextMorning)) return <span className="text-ink-3">0 / 0</span>
  const tone = painScoreTone(Math.max(s.painDuring ?? 0, s.painNextMorning ?? 0))
  return (
    <span className={cn('chip h-5 border px-2 text-[11px]', tone.badgeCls)} title={t('share.portal.painTitle')}>
      {isNum(s.painDuring) ? fmtNumber(s.painDuring, 1, { trim: true }) : '—'} / {isNum(s.painNextMorning) ? fmtNumber(s.painNextMorning, 1, { trim: true }) : '—'}
    </span>
  )
}

function ShoeBadge({ s }: { s: PublicActivityItem }) {
  if (!s.shoeName) return null
  return (
    <span className="chip h-5 max-w-full border border-line bg-track/60 px-2 text-[11px] text-ink-2">
      <Footprints className="size-3 flex-none" aria-hidden="true" />
      <span className="truncate">
        {s.shoeName}
        {isNum(s.shoeDropMm) ? ` · ${fmtNumber(s.shoeDropMm, 0)} mm` : ''}
      </span>
    </span>
  )
}

function RemarkQuote({ s }: { s: PublicActivityItem }) {
  if (!s.remark) return null
  return (
    <p className="flex items-start gap-1.5 text-xs text-ink-2">
      <MessageSquareQuote className="mt-px size-3.5 flex-none text-info" aria-hidden="true" />
      <span className="min-w-0 break-words">
        {s.remark.body} <span className="text-ink-3">— {authorLine(s.remark)}</span>
      </span>
    </p>
  )
}

/** 4. Recent sessions: pain, shoe, remark marker; detail and quick remark when the link allows them. */
export function PortalSessions({ data, token, inline }: { data: PublicShareView; token: string; inline: boolean }) {
  const perms = permissionsOf(data)
  const sessions = data.recentSessions
  const showNotes = data.sections.includes('notes')
  const [remarkFor, setRemarkFor] = useState<number | null>(null)
  const [detailId, setDetailId] = useState<number | null>(null)
  if (!sessions) return null
  const editing = sessions.find((s) => s.id === remarkFor) ?? null
  const hasActions = perms.comments || perms.activityDetails
  const remarkLabel = (s: PublicActivityItem) => (s.remark ? t('share.portal.editRemark') : t('share.portal.remark'))

  const actions = (s: PublicActivityItem, compact = false) => (
    <span className={cn('flex flex-wrap items-center gap-1.5', !compact && 'justify-end')}>
      {perms.activityDetails && (
        <button type="button" className="btn btn-ghost h-8 min-h-8 px-2.5 text-xs" onClick={() => setDetailId(s.id)} aria-label={t('share.portal.detailAria', { name: s.name })}>
            {t('share.portal.detailBtn')} <ChevronRight className="size-3.5" aria-hidden="true" />
        </button>
      )}
      {perms.comments && (
        <button type="button" className="btn btn-ghost h-8 min-h-8 px-2.5 text-xs" onClick={() => setRemarkFor(s.id)} aria-label={t('share.labels.remarkFor', { label: remarkLabel(s), name: s.name })}>
          <MessageSquarePlus className="size-3.5" aria-hidden="true" />
          {remarkLabel(s)}
        </button>
      )}
    </span>
  )

  return (
    <Card title={t('share.portal.recent')} subtitle={t('share.portal.recentSubtitle')} icon={<Activity />}>
      {sessions.length === 0 ? (
        <p className="text-sm text-ink-3">{t('share.portal.noRecent')}</p>
      ) : inline ? (
        <div className="scroll-thin overflow-x-auto">
          <table className="w-full min-w-[820px] text-left text-xs">
            <thead>
              <tr className="border-b border-line text-ink-3">
                <th className="pr-3 pb-2.5 font-medium">{t('share.portal.date')}</th>
                <th className="px-2 pb-2.5 font-medium">{t('share.portal.session')}</th>
                <th className="px-2 pb-2.5 text-right font-medium">{t('share.portal.distance')}</th>
                <th className="px-2 pb-2.5 text-right font-medium">{t('share.portal.paceGap')}</th>
                <th className="px-2 pb-2.5 text-right font-medium">FC</th>
                <th className="px-2 pb-2.5 text-right font-medium">GCT</th>
                <th className="px-2 pb-2.5 text-right font-medium">D−</th>
                <th className="px-2 pb-2.5 text-right font-medium">{t('share.portal.load')}</th>
                <th className="px-2 pb-2.5 font-medium">{t('share.portal.shoe')}</th>
                {showNotes && <th className="px-2 pb-2.5 text-right font-medium">{t('share.portal.painShort')}</th>}
                {hasActions && (
                  <th className="pb-2.5 pl-2 text-right font-medium">
                    <span className="sr-only">{t('share.portal.actions')}</span>
                  </th>
                )}
              </tr>
            </thead>
            <tbody>
              {sessions.map((s) => {
                const cols = 9 + (showNotes ? 1 : 0) + (hasActions ? 1 : 0)
                const open = remarkFor === s.id && perms.comments
                return (
                  <Fragment key={s.id}>
                    <tr className={cn('hover:bg-track/30', !s.remark && !open && 'border-b border-line')}>
                      <td className="num py-2.5 pr-3 whitespace-nowrap text-ink-2">{fmtDateShort(s.date)}</td>
                      <td className="px-2 py-2.5">
                        <p className="max-w-[14rem] truncate font-semibold text-ink">{s.name}</p>
                        <p className="text-[11px] text-ink-3">{s.typeLabel}</p>
                      </td>
                      <td className="num px-2 py-2.5 text-right font-medium whitespace-nowrap text-ink">{fmtKm(s.distanceM)}</td>
                      <td className="num px-2 py-2.5 text-right whitespace-nowrap text-ink-2">
                        {fmtPace(s.avgPaceSecPerKm)}
                        {isNum(s.gapSecPerKm) && s.gapSecPerKm !== s.avgPaceSecPerKm && <span className="block text-[11px] text-ink-3">GAP {fmtPace(s.gapSecPerKm)}</span>}
                      </td>
                      <td className="num px-2 py-2.5 text-right whitespace-nowrap text-ink-2">{isNum(s.avgHR) ? `${fmtNumber(s.avgHR)} bpm` : 'N/D'}</td>
                      <td className="num px-2 py-2.5 text-right whitespace-nowrap text-ink-2">{isNum(s.gctMs) ? `${fmtNumber(s.gctMs)} ms` : 'N/D'}</td>
                      <td className="num px-2 py-2.5 text-right whitespace-nowrap text-ink-2">{isNum(s.elevLossM) ? `${fmtNumber(s.elevLossM)} m` : 'N/D'}</td>
                      <td className="num px-2 py-2.5 text-right font-medium whitespace-nowrap text-ink">{isNum(s.trainingLoad) ? fmtNumber(s.trainingLoad, 0) : 'N/D'}</td>
                      <td className="px-2 py-2.5 text-ink-2">
                        {s.shoeName ? (
                          <span className="block max-w-[9rem] truncate">
                            {s.shoeName}
                            {isNum(s.shoeDropMm) ? ` (${fmtNumber(s.shoeDropMm, 0)} mm)` : ''}
                          </span>
                        ) : (
                          <span className="text-ink-3">—</span>
                        )}
                      </td>
                      {showNotes && (
                        <td className="num px-2 py-2.5 text-right whitespace-nowrap">
                          <PainBadge s={s} />
                        </td>
                      )}
                      {hasActions && <td className="py-2 pl-2 whitespace-nowrap">{actions(s)}</td>}
                    </tr>
                    {(s.remark || open) && (
                      <tr className="border-b border-line">
                        <td colSpan={cols} className="pt-0 pb-3">
                          {open ? <RemarkEditor key={s.remark?.updatedAt ?? 'new'} token={token} activity={s} inline open onClose={() => setRemarkFor(null)} /> : <RemarkQuote s={s} />}
                        </td>
                      </tr>
                    )}
                  </Fragment>
                )
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <ul className="divide-y divide-line" aria-label={t('share.portal.recent')}>
          {sessions.map((s) => (
            <li key={s.id} className="space-y-2 py-3 first:pt-0 last:pb-0">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <p className="truncate text-sm font-semibold text-ink">{s.name}</p>
                  <p className="num text-xs text-ink-3">
                    {fmtDateShort(s.date)} · {s.typeLabel}
                  </p>
                </div>
                {showNotes && (isNum(s.painDuring) || isNum(s.painNextMorning)) && (
                  <span className="flex-none">
                    <PainBadge s={s} />
                  </span>
                )}
              </div>
              <p className="num flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-ink-2">
                <span className="font-semibold text-ink">{fmtKm(s.distanceM)}</span>
                <span>{fmtPace(s.avgPaceSecPerKm)}</span>
                {isNum(s.avgHR) && <span>{fmtNumber(s.avgHR)} bpm</span>}
                {isNum(s.elevLossM) && <span>D− {fmtNumber(s.elevLossM)} m</span>}
                {isNum(s.gctMs) && <span>GCT {fmtNumber(s.gctMs)} ms</span>}
              </p>
              <ShoeBadge s={s} />
              <RemarkQuote s={s} />
              {hasActions && actions(s, true)}
            </li>
          ))}
        </ul>
      )}
      {!inline && editing && perms.comments && <RemarkEditor key={editing.id} token={token} activity={editing} inline={false} open onClose={() => setRemarkFor(null)} />}
      {perms.activityDetails && detailId !== null && <PortalActivityDetail token={token} id={detailId} canRemark={perms.comments} onClose={() => setDetailId(null)} />}
    </Card>
  )
}
