import { Moon, Settings, Sun } from 'lucide-react'
import type { ReactNode } from 'react'
import { IS_MOCK } from '../../api/client'
import type { Theme } from '../../hooks/useTheme'
import { StrideMark } from '../ui/icons'

interface AppHeaderProps {
  theme: Theme
  onToggleTheme: () => void
  onOpenSettings: () => void
  center?: ReactNode
}

export function AppHeader({ theme, onToggleTheme, onOpenSettings, center }: AppHeaderProps) {
  return (
    <header className="header-fade z-30 lg:sticky lg:top-0 lg:-mt-3 lg:pt-3">
      <div className="lg:glass-bar lg:rounded-[22px]">
      <div className="flex h-14 items-center gap-3 lg:h-16 lg:px-4">
        <a href="./" className="flex min-w-0 items-center gap-2.5 rounded-xl" aria-label="Stride — accueil">
          <span className="grid size-9 flex-none place-items-center rounded-xl bg-gradient-to-br from-accent/20 to-accent-2/5 ring-1 ring-accent/25 ring-inset">
            <StrideMark className="size-6" />
          </span>
          <span className="min-w-0">
            <span className="block text-[1.05rem] leading-tight font-semibold tracking-tight">Stride</span>
            <span className="hidden truncate text-[11px] leading-tight text-ink-3 sm:block">
              Données Garmin Connect{IS_MOCK && ' · démo'}
            </span>
          </span>
        </a>
        <div className="hidden min-w-0 flex-1 justify-center lg:flex">{center}</div>
        <div className="ml-auto flex items-center gap-2 lg:ml-0">
          {IS_MOCK && <span className="chip border border-warn/30 bg-warn/12 text-warn lg:hidden">démo</span>}
          <button type="button" className="icon-btn" onClick={onToggleTheme} aria-label={theme === 'dark' ? 'Passer en thème clair' : 'Passer en thème sombre'}>
            {theme === 'dark' ? <Sun className="size-[1.125rem]" /> : <Moon className="size-[1.125rem]" />}
          </button>
          <button type="button" className="icon-btn" onClick={onOpenSettings} aria-label="Paramètres" aria-haspopup="dialog">
            <Settings className="size-[1.125rem]" />
          </button>
        </div>
      </div>
      </div>
    </header>
  )
}
