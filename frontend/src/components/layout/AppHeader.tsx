import { Moon, Search, Sun } from 'lucide-react'
import { Link, useLocation } from 'wouter'
import { IS_MOCK } from '../../api/client'
import type { Me } from '../../api/types'
import { useTheme } from '../../hooks/useTheme'
import { cn } from '../../lib/cn'
import { ui } from '../../state/ui'
import { StrideMark } from '../ui/icons'
import { isActive, NAV } from './nav'
import { SyncIndicator } from './SyncIndicator'
import { UserMenu } from './UserMenu'

/**
 * Full-bleed sticky bar (top: 0, opaque glass, hairline border) so nothing scrolls visibly above or beside it.
 * Three-column grid (1fr auto 1fr) keeps the nav optically centred whatever the brand/actions widths.
 */
export function AppHeader({ me }: { me: Me | null }) {
  const [path] = useLocation()
  const { theme, toggle } = useTheme()
  const items = NAV.filter((n) => !n.admin || me?.role === 'admin')
  return (
    <header className="app-header">
      <div className="app-header-grid mx-auto h-14 max-w-[1440px] px-4 sm:px-6 lg:h-16 lg:px-8">
        <Link href="/" className="flex min-w-0 items-center gap-2.5 justify-self-start rounded-xl" aria-label="Stride, tableau de bord">
          <span className="grid size-9 flex-none place-items-center rounded-xl bg-accent/15 text-accent ring-1 ring-accent/20 ring-inset">
            <StrideMark className="w-6" />
          </span>
          <span className="min-w-0 truncate text-[1.1rem] leading-none font-semibold tracking-tight">Stride</span>
          {IS_MOCK && <span className="chip hidden h-5 flex-none border border-warn/30 bg-warn/12 px-2 text-[10px] text-warn sm:inline-flex">démo</span>}
        </Link>

        {me && (
          <nav aria-label="Navigation principale" className="hidden justify-self-center lg:block">
            <ul className="flex items-center gap-1 rounded-2xl border border-line bg-track/50 p-1">
              {items.map((n) => {
                const active = isActive(n.href, path)
                const Icon = n.icon
                return (
                  <li key={n.href}>
                    <Link
                      href={n.href}
                      aria-current={active ? 'page' : undefined}
                      className={cn(
                        'flex h-9 items-center gap-2 rounded-xl px-3 text-sm font-medium whitespace-nowrap transition-colors xl:px-3.5',
                        active ? 'bg-accent text-on-accent shadow-xs' : 'text-ink-2 hover:bg-track hover:text-ink',
                      )}
                    >
                      <Icon className="size-4 flex-none" aria-hidden="true" />
                      <span className="xl:hidden">{n.short}</span>
                      <span className="hidden xl:inline">{n.label}</span>
                    </Link>
                  </li>
                )
              })}
            </ul>
          </nav>
        )}

        <div className="col-start-2 flex min-w-0 items-center justify-end gap-1.5 justify-self-end sm:gap-2 lg:col-start-3">
          {me && (
            <button
              type="button"
              onClick={ui.openCommand}
              aria-label="Ouvrir la palette de commandes (Ctrl+K)"
              className="inline-flex h-9 items-center gap-2 rounded-xl border border-line bg-track/55 px-2.5 text-xs text-ink-2 transition-colors hover:bg-track hover:text-ink"
            >
              <Search className="size-3.5 flex-none text-ink-3" aria-hidden="true" />
              <span className="hidden md:inline">Rechercher…</span>
              <kbd className="hidden rounded-md border border-line bg-surface px-1.5 py-0.5 font-mono text-[10px] text-ink-3 sm:inline">
                ⌘K
              </kbd>
            </button>
          )}
          {me && <SyncIndicator />}
          <button type="button" className="icon-btn" onClick={toggle} aria-label={theme === 'dark' ? 'Passer en thème clair' : 'Passer en thème sombre'}>
            {theme === 'dark' ? <Sun className="size-[1.125rem]" /> : <Moon className="size-[1.125rem]" />}
          </button>
          {me && <UserMenu me={me} />}
        </div>
      </div>
    </header>
  )
}
