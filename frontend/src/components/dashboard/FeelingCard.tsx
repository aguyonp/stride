import { HeartPulse, Pencil } from 'lucide-react'
import { useProfile } from '../../api/queries'
import { useActivitySymptoms } from '../../api/queriesV41'
import type { ActivityView } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtNumber, todayISO } from '../../lib/format'
import { tagLabel } from '../../lib/labels'
import { ui } from '../../state/ui'
import { zoneText } from '../health/healthStatus'
import { Card } from '../ui/Card'

const painCls = (v: number) => (v >= 5 ? 'border-danger/35 bg-danger/10 text-danger' : v >= 3 ? 'border-warn/35 bg-warn/10 text-warn' : 'border-ok/30 bg-ok/10 text-ok')
const score = (v: number | null) => (v === null ? '—' : fmtNumber(v, 1, { trim: true }))

/** « Ressenti » at a glance: symptoms per zone (body entries of this session), RPE, shoe; the form opens in the sheet. */
export function FeelingCard({ view, className }: { view: ActivityView; className?: string }) {
  const profile = useProfile()
  const today = todayISO(profile.data?.timezone)
  const { entries } = useActivitySymptoms(view.activity.id, view.activity.date, today)
  const n = view.notes
  const empty = entries.length === 0 && n.rpe === null && !n.shoe && n.tags.length === 0 && !n.note

  return (
    <Card title="Ressenti" icon={<HeartPulse className="size-4" />} className={className}>
      {empty ? (
        <button type="button" className="btn btn-primary h-12 w-full" onClick={ui.openFeeling}>
          <HeartPulse className="size-5" aria-hidden="true" /> Saisir mon ressenti
        </button>
      ) : (
        <div className="space-y-2.5">
          <ul className="flex flex-wrap gap-1.5">
            {entries.map((e) => (
              <li key={e.id} className={cn('chip h-auto min-h-7 border py-1 whitespace-normal', painCls(Math.max(e.painScore, e.coldNextDayScore ?? 0)))}>
                {zoneText(e)} · <span className="num">{score(e.painScore)} / J+1 {score(e.coldNextDayScore)}</span>
              </li>
            ))}
            {n.rpe !== null && <li className="chip border border-line bg-track text-ink-2">RPE {n.rpe}/10</li>}
            {n.shoe && <li className="chip border border-line bg-track text-ink-2">{n.shoe.name}</li>}
          </ul>
          {(n.tags.length > 0 || n.note) && <p className="line-clamp-1 text-xs text-ink-3">{[n.tags.map(tagLabel).join(' · '), n.note].filter(Boolean).join(' — ')}</p>}
          <button type="button" className="btn btn-ghost h-9 min-h-9 w-full text-xs" onClick={ui.openFeeling}>
            <Pencil className="size-3.5" aria-hidden="true" /> Modifier
          </button>
        </div>
      )}
    </Card>
  )
}
