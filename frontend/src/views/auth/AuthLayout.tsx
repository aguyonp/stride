import { Activity, BrainCircuit, Gauge } from 'lucide-react'
import type { ReactNode } from 'react'
import { StrideMark } from '../../components/ui/icons'
import { LanguageToggle } from '../../components/ui/LanguageToggle'
import { t } from '../../i18n'

const FEATURES = [
  { icon: Activity, n: 1 },
  { icon: Gauge, n: 2 },
  { icon: BrainCircuit, n: 3 },
] as const

export function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className="mx-auto grid max-w-5xl items-center gap-8 py-4 sm:py-8 lg:min-h-[calc(100dvh-10rem)] lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)] lg:gap-14">
      <div className="min-w-0">
        <div className="flex items-center gap-3">
          <span className="grid size-14 flex-none place-items-center rounded-2xl bg-accent/15 text-accent ring-1 ring-accent/20 ring-inset">
            <StrideMark className="w-9" />
          </span>
          <div className="min-w-0 flex-1">
            <p className="text-3xl font-semibold tracking-tight">Stride</p>
            <p className="text-sm text-ink-3">{t('auth.layout.tagline')}</p>
          </div>
          <LanguageToggle className="flex-none self-start" />
        </div>
        <ul className="mt-7 hidden space-y-4 sm:block">
          {FEATURES.map(({ icon: Icon, n }) => (
            <li key={n} className="flex gap-3">
              <span className="grid size-10 flex-none place-items-center rounded-xl bg-accent/12 text-accent">
                <Icon className="size-5" aria-hidden="true" />
              </span>
              <div className="min-w-0">
                <p className="font-semibold text-ink">{t(`auth.layout.f${n}Title`)}</p>
                <p className="text-sm text-ink-3">{t(`auth.layout.f${n}Text`)}</p>
              </div>
            </li>
          ))}
        </ul>
      </div>
      <div className="min-w-0">{children}</div>
    </div>
  )
}
