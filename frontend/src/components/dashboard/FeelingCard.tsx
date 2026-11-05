import { HeartPulse, Pencil } from 'lucide-react'
import type { ActivityView } from '../../api/types'
import { useMediaQuery } from '../../hooks/useMediaQuery'
import { cn } from '../../lib/cn'
import { fmtNumber } from '../../lib/format'
import { tagLabel } from '../../lib/labels'
import { ui } from '../../state/ui'
import { FeelingForm, usePainPreset } from '../feeling/FeelingForm'
import { Card } from '../ui/Card'

const painCls = (v: number) => (v >= 5 ? 'border-danger/35 bg-danger/10 text-danger' : v >= 3 ? 'border-warn/35 bg-warn/10 text-warn' : 'border-ok/30 bg-ok/10 text-ok')

/** Inline form on desktop; compact summary + bottom sheet on mobile. */
export function FeelingCard({ view, className }: { view: ActivityView; className?: string }) {
  const desktop = useMediaQuery('(min-width: 1024px)')
  const { labels } = usePainPreset()
  const n = view.notes
  const pains: [string, number | null][] = [
    [labels.during, n.painDuring],
    [labels.cold, n.painCold],
    [labels.nextMorning, n.painNextMorning],
  ]
  const empty = pains.every(([, v]) => v === null) && n.rpe === null && !n.shoe && n.tags.length === 0 && !n.note
  return (
    <Card title="Mon ressenti" icon={<HeartPulse className="size-4" />} subtitle={n.updatedAt ? 'Enregistré · modifiable' : 'Douleur, chaussures, contexte, RPE'} className={className}>
      {desktop ? (
        <FeelingForm activityId={view.activity.id} notes={n} />
      ) : empty ? (
        <button type="button" className="btn btn-primary h-12 w-full" onClick={ui.openFeeling}>
          <HeartPulse className="size-5" aria-hidden="true" /> Saisir mon ressenti
        </button>
      ) : (
        <div className="space-y-2.5">
          <ul className="flex flex-wrap gap-1.5">
            {pains.map(([label, v]) =>
              v === null ? null : (
                <li key={label} className={cn('chip h-auto min-h-7 border py-1 whitespace-normal', painCls(v))}>
                  {label} : <span className="num">{fmtNumber(v, 1, { trim: true })}/10</span>
                </li>
              ),
            )}
            {n.rpe !== null && <li className="chip border border-line bg-track text-ink-2">RPE {n.rpe}/10</li>}
            {n.shoe && <li className="chip border border-line bg-track text-ink-2">{n.shoe.name}</li>}
          </ul>
          {n.tags.length > 0 && <p className="text-xs text-ink-3">{n.tags.map(tagLabel).join(' · ')}</p>}
          {n.note && <p className="line-clamp-2 text-sm text-ink-2">« {n.note} »</p>}
          <button type="button" className="btn btn-ghost w-full" onClick={ui.openFeeling}>
            <Pencil className="size-4" aria-hidden="true" /> Modifier
          </button>
        </div>
      )}
    </Card>
  )
}
