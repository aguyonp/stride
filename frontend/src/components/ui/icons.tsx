import { Activity, Bike, Dumbbell, Footprints, Mountain, Moon, PersonStanding, StretchHorizontal, Waves, type LucideProps } from 'lucide-react'
import type { ComponentType, SVGProps } from 'react'
import type { PlanSport, Sport } from '../../api/types'

/** Stride mark: a faded previous step, then a rising stride arc launching to a dot. */
export function StrideMark({ className, title }: { className?: string; title?: string }) {
  return (
    <svg viewBox="6 14 52 36" className={className} role={title ? 'img' : undefined} aria-hidden={title ? undefined : true} aria-label={title}>
      <defs>
        <linearGradient id="stride-mark-g" x1="0" y1="1" x2="1" y2="0">
          <stop offset="0" stopColor="var(--accent)" />
          <stop offset="1" stopColor="var(--accent-2)" />
        </linearGradient>
      </defs>
      <path d="M10 44c4-15 11.5-15 15.5 0" fill="none" stroke="url(#stride-mark-g)" strokeWidth="6.5" strokeLinecap="round" opacity=".5" />
      <path d="M25.5 44c4-17 12-21 20-21" fill="none" stroke="url(#stride-mark-g)" strokeWidth="6.5" strokeLinecap="round" />
      <circle cx="51" cy="23" r="5.2" fill="currentColor" />
    </svg>
  )
}

export function StairsIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" {...props}>
      <path d="M3 20h5v-5h5v-5h5V5h3" />
      <path d="M14 3h4v4" />
    </svg>
  )
}

const TYPE_ICON: Record<string, ComponentType<LucideProps>> = {
  trail_running: Mountain,
  hiking: Mountain,
}

const SPORT_ICON: Record<Sport, ComponentType<LucideProps>> = {
  run: Footprints,
  bike: Bike,
  walk: PersonStanding,
  hike: Mountain,
  swim: Waves,
  strength: Dumbbell,
  other: Activity,
}

const PLAN_ICON: Record<PlanSport, ComponentType<LucideProps>> = {
  run: Footprints,
  bike: Bike,
  strength: Dumbbell,
  mobility: StretchHorizontal,
  rest: Moon,
  other: Activity,
}

export function ActivityIcon({ sport, type, className }: { sport: Sport; type?: string; className?: string }) {
  const Icon = (type && TYPE_ICON[type]) || SPORT_ICON[sport] || Activity
  return <Icon className={className} aria-hidden="true" />
}

export function PlanSportIcon({ sport, className }: { sport: PlanSport; className?: string }) {
  const Icon = PLAN_ICON[sport] ?? Activity
  return <Icon className={className} aria-hidden="true" />
}
