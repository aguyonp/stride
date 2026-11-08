import { CalendarRange, Pencil, Plus } from 'lucide-react'
import { useMemo, useState } from 'react'
import type { CalendarEvent, PlannedSession, PublicShareView } from '../../../api/types'
import { EVENT_ICON, EVENT_TONE } from '../../../components/plan/EventDialog'
import { MorphedTag } from '../../../components/plan/morph/MorphActions'
import { targetParts } from '../../../components/plan/planFormat'
import { stepsSummary } from '../../../components/plan/steps'
import { PrescribedByTag } from '../../../components/share/ShareMarkers'
import { Card } from '../../../components/ui/Card'
import { PlanSportIcon } from '../../../components/ui/icons'
import { cn } from '../../../lib/cn'
import { addDays, capitalize, fmtDateLong, fmtDateShort } from '../../../lib/format'
import { EVENT_KIND, PLAN_SPORT_LABEL, SEVERITY_LABEL } from '../../../lib/labels'
import { PrescriptionEditor, type PrescriptionTarget } from './PortalEditors'
import { ownedByLink, permissionsOf } from './portalUtils'
import { t } from '../../../i18n'

type Day = { date: string; planned: PlannedSession[]; events: CalendarEvent[] }

function EditButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button type="button" className="icon-btn size-8 flex-none" onClick={onClick} aria-label={label}>
      <Pencil className="size-3.5" />
    </button>
  )
}

/** 3. Calendar (today −7 → +42) with quick prescription when the link has the planning permission. */
export function PortalCalendar({ data, token, inline, today }: { data: PublicShareView; token: string; inline: boolean; today: string }) {
  const perms = permissionsOf(data)
  const [showPast, setShowPast] = useState(false)
  const [target, setTarget] = useState<PrescriptionTarget | null>(null)
  const cal = data.calendar
  const from = showPast ? addDays(today, -7) : today

  const days = useMemo(() => {
    const map = new Map<string, Day>()
    const day = (date: string) => map.get(date) ?? map.set(date, { date, planned: [], events: [] }).get(date)!
    for (const s of cal?.planned ?? []) if (s.date >= from) day(s.date).planned.push(s)
    for (const e of cal?.events ?? []) if (e.endDate >= from) day(e.startDate < from ? from : e.startDate).events.push(e)
    return [...map.values()].sort((a, b) => a.date.localeCompare(b.date))
  }, [cal, from])

  if (!cal) return null
  const canWrite = perms.planning
  const close = () => setTarget(null)

  return (
    <Card
      title={t('share.portal.planning')}
      icon={<CalendarRange />}
      subtitle={t('share.portal.planningSubtitle')}
      action={
        canWrite && (
          <button type="button" className="btn btn-primary h-9 min-h-9 px-3.5 text-sm" onClick={() => setTarget({ kind: 'new', date: addDays(today, 1) })}>
            <Plus className="size-4" aria-hidden="true" /> Prescrire
          </button>
        )
      }
    >
      <div className="space-y-4">
        {target && <PrescriptionEditor key={target.kind === 'new' ? 'new' : target.kind === 'session' ? `s${target.s.id}` : `e${target.e.id}`} token={token} target={target} inline={inline} onClose={close} today={today} linkLabel={data.shareLabel} />}
        {days.length === 0 ? (
          <p className="text-sm text-ink-3">{t('share.portal.nothing')}</p>
        ) : (
          <ol className="space-y-4" aria-label={t('share.portal.plannedDays')}>
            {days.map((d) => (
              <li key={d.date} className="grid gap-2 sm:grid-cols-[8.5rem_minmax(0,1fr)] sm:gap-4">
                <h3 className={cn('pt-1.5 text-xs font-semibold', d.date === today ? 'text-accent' : d.date < today ? 'text-ink-3' : 'text-ink-2')}>
                  {capitalize(fmtDateLong(d.date))}
                  {d.date === today && <span className="sr-only">{t('share.portal.todaySr')}</span>}
                </h3>
                <ul className="min-w-0 divide-y divide-line border-b border-line sm:border-b-0">
                  {d.events.map((e) => {
                    const Icon = EVENT_ICON[e.kind]
                    const mine = canWrite && ownedByLink(e, data)
                    return (
                      <li key={`e${e.id}`} className="flex items-start gap-2.5 py-2 first:pt-0">
                        <span className={cn('grid size-7 flex-none place-items-center rounded-lg', EVENT_TONE[e.kind])}>
                          <Icon className="size-3.5" aria-hidden="true" />
                        </span>
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-semibold text-ink">
                            {EVENT_KIND[e.kind].label} · {SEVERITY_LABEL[e.severity].toLowerCase()}
                          </p>
                          <p className="num text-xs text-ink-3">{e.startDate === e.endDate ? fmtDateShort(e.startDate) : `Du ${fmtDateShort(e.startDate)} au ${fmtDateShort(e.endDate)}`}</p>
                          {e.note && <p className="mt-0.5 text-xs break-words text-ink-2">{e.note}</p>}
                          <PrescribedByTag by={e.prescribedBy} className="mt-1" />
                        </div>
                        {mine && <EditButton label={t('share.labels.editEvent', { kind: EVENT_KIND[e.kind].label })} onClick={() => setTarget({ kind: 'event', e })} />}
                      </li>
                    )
                  })}
                  {d.planned.map((s) => {
                    const targets = targetParts(s)
                    const mine = canWrite && ownedByLink(s, data)
                    return (
                      <li key={`s${s.id}`} className="flex items-start gap-2.5 py-2 first:pt-0">
                        <span className={cn('grid size-7 flex-none place-items-center rounded-lg', s.sport === 'rest' ? 'bg-track text-ink-3' : 'bg-accent/12 text-accent')}>
                          <PlanSportIcon sport={s.sport} className="size-3.5" />
                        </span>
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-semibold break-words text-ink">
                            {s.title || PLAN_SPORT_LABEL[s.sport]}
                            <span className="sr-only"> ({PLAN_SPORT_LABEL[s.sport]})</span>
                          </p>
                          {(targets.length > 0 || s.steps.length > 0) && <p className="num mt-0.5 text-xs break-words text-ink-2">{s.steps.length > 0 ? stepsSummary(s.steps) : targets.join(' · ')}</p>}
                          {s.instructions && <p className="mt-0.5 line-clamp-2 text-xs break-words text-ink-3">{s.instructions}</p>}
                          {(s.prescribedBy || s.morph) && (
                            <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1">
                              <MorphedTag s={s} />
                              <PrescribedByTag by={s.prescribedBy} />
                            </div>
                          )}
                        </div>
                        {mine && <EditButton label={t('share.portal.editSession', { title: s.title })} onClick={() => setTarget({ kind: 'session', s })} />}
                      </li>
                    )
                  })}
                </ul>
              </li>
            ))}
          </ol>
        )}
        <button type="button" className="text-xs font-medium text-ink-3 underline-offset-2 hover:text-ink hover:underline" onClick={() => setShowPast((v) => !v)}>
          {showPast ? t('share.portal.hidePast') : t('share.portal.showPast')}
        </button>
      </div>
    </Card>
  )
}
