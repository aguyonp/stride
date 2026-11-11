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
import { t } from '../../i18n'

/**
 * Full-bleed sticky bar. Brand and actions keep their natural width and the nav is centred in the
 * remaining track, so the menu never overlaps the actions at any width.
 */
export function AppHeader({ me }: { me: Me | null }) {
  const [path] = useLocation()
  const { theme, toggle } = useTheme()
  return (
    <header className="app-header">
      <div className="app-header-grid mx-auto h-14 max-w-[1440px] px-4 sm:px-6 lg:h-16 lg:px-8">
        <Link href="/" className="flex min-w-0 items-center gap-2.5 justify-self-start rounded-xl" aria-label={t('layout.nav.home')}>
          <span className="grid size-9 flex-none place-items-center rounded-xl bg-accent/15 text-accent ring-1 ring-accent/20 ring-inset">
            <StrideMark className="w-6" />
          </span>
          <span className="min-w-0 truncate text-[1.1rem] leading-none font-semibold tracking-tight">Stride</span>
          {IS_MOCK && <span className="chip hidden h-5 flex-none border border-warn/30 bg-warn/12 px-2 text-[10px] text-warn sm:inline-flex">{t('layout.nav.demo')}</span>}
        </Link>

        {me && (
          <nav aria-label={t('layout.nav.main')} className="hidden justify-self-center lg:block">
            <ul className="flex items-center gap-1 rounded-2xl border border-line bg-track/50 p-1">
              {NAV.map((n) => {
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
            <button type="button" className="icon-btn" onClick={ui.openCommand} aria-label={t('layout.nav.search')} title={t('layout.nav.searchTitle')}>
              <Search className="size-[1.125rem]" />
            </button>
          )}
          {me && <SyncIndicator />}
          <button type="button" className="icon-btn" onClick={toggle} aria-label={theme === 'dark' ? t('layout.nav.lightTheme') : t('layout.nav.darkTheme')}>
            {theme === 'dark' ? <Sun className="size-[1.125rem]" /> : <Moon className="size-[1.125rem]" />}
          </button>
          {me && <UserMenu me={me} />}
        </div>
      </div>
    </header>
  )
}
