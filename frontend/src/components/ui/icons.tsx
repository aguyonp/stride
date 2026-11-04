import { Activity, Bike, Dumbbell, Footprints, Mountain, PersonStanding, Waves, type LucideProps } from 'lucide-react'
import type { ComponentType, SVGProps } from 'react'

/** Marque Patella : rotule stylisée (forme abstraite) au-dessus de la trochlée. */
export function PatellaMark({ className, title }: { className?: string; title?: string }) {
  return (
    <svg viewBox="0 0 64 64" className={className} role={title ? 'img' : undefined} aria-hidden={title ? undefined : true} aria-label={title}>
      <defs>
        <linearGradient id="pm-g" x1="0.15" y1="0" x2="0.85" y2="1">
          <stop offset="0" stopColor="var(--accent)" />
          <stop offset="1" stopColor="var(--accent-2)" />
        </linearGradient>
      </defs>
      <path d="M13 43.5c5.5 9.5 32.5 9.5 38 0" fill="none" stroke="url(#pm-g)" strokeOpacity="0.55" strokeWidth="4" strokeLinecap="round" />
      <path d="M32 11c11.2 0 19.5 5 19.5 12.6 0 8.6-9.6 16.4-19.5 20.9-9.9-4.5-19.5-12.3-19.5-20.9 0-7.6 8.3-12.6 19.5-12.6Z" fill="url(#pm-g)" />
      <path d="M23 19c3-2.2 8-3 12.5-2" fill="none" stroke="#fff" strokeOpacity="0.55" strokeWidth="2.6" strokeLinecap="round" />
    </svg>
  )
}

/** Escalier (étages montés). */
export function StairsIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" {...props}>
      <path d="M3 20h5v-5h5v-5h5V5h3" />
      <path d="M14 3h4v4" />
    </svg>
  )
}

const TYPE_ICON: Record<string, ComponentType<LucideProps>> = {
  running: Footprints,
  treadmill_running: Footprints,
  track_running: Footprints,
  trail_running: Mountain,
  cycling: Bike,
  road_biking: Bike,
  mountain_biking: Bike,
  gravel_cycling: Bike,
  indoor_cycling: Bike,
  walking: PersonStanding,
  hiking: Mountain,
  strength_training: Dumbbell,
  lap_swimming: Waves,
  open_water_swimming: Waves,
}

export function ActivityIcon({ type, className }: { type: string; className?: string }) {
  const Icon = TYPE_ICON[type] ?? Activity
  return <Icon className={className} aria-hidden="true" />
}
