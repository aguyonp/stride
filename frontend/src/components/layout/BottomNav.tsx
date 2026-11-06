import { HeartPulse } from 'lucide-react'
import { Link, useLocation } from 'wouter'
import { cn } from '../../lib/cn'
import { ui } from '../../state/ui'
import { isActive, NAV } from './nav'

/** Mobile dock: four destinations + a central quick « Ressenti » action for one-thumb use after a run. */
export function BottomNav() {
  const [path] = useLocation()
  const items = NAV.filter((n) => !n.admin)
  const left = items.slice(0, 2)
  const right = items.slice(2)
  const link = (n: (typeof items)[number]) => {
    const active = isActive(n.href, path)
    const Icon = n.icon
    return (
      <li key={n.href} className="min-w-0">
        <Link
          href={n.href}
          aria-current={active ? 'page' : undefined}
          className={cn('flex h-14 flex-col items-center justify-center gap-0.5 rounded-2xl text-[11px] font-medium transition-colors', active ? 'text-accent' : 'text-ink-3 hover:text-ink-2')}
        >
          <Icon className="size-[1.3rem]" aria-hidden="true" />
          <span className="max-w-full truncate px-1">{n.short}</span>
        </Link>
      </li>
    )
  }
  return (
    <nav aria-label="Navigation principale" className="bottom-dock lg:hidden">
      <ul className="mx-auto grid max-w-lg grid-cols-5 items-center px-2 pt-1">
        {left.map(link)}
        <li className="flex justify-center">
          <button
            type="button"
            onClick={ui.openFeeling}
            className="btn btn-primary -mt-5 size-14 flex-col gap-0 rounded-[20px] p-0 text-[10px] shadow-[0_10px_24px_-8px_var(--shadow-1)]"
            aria-label="Saisir mon ressenti sur la dernière séance"
          >
            <HeartPulse className="size-6" aria-hidden="true" />
          </button>
        </li>
        {right.map(link)}
      </ul>
    </nav>
  )
}
