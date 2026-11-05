import type { ReactNode } from 'react'
import type { Me } from '../../api/types'
import { cn } from '../../lib/cn'
import { FeelingSheet } from '../feeling/FeelingSheet'
import { Ambient } from './Ambient'
import { AppHeader } from './AppHeader'
import { BottomNav } from './BottomNav'
import { SyncController } from './SyncController'

export function AppShell({ me, minimal, children }: { me: Me | null; minimal?: boolean; children: ReactNode }) {
  const full = !!me && !minimal
  return (
    <div className="app-root">
      <a href="#main" className="sr-only z-[70] rounded-xl bg-surface px-4 py-2 focus:not-sr-only focus:fixed focus:top-3 focus:left-3">
        Aller au contenu
      </a>
      <Ambient />
      <AppHeader me={full ? me : null} />
      <main
        id="main"
        className={cn('mx-auto w-full max-w-[1440px] px-4 pt-4 sm:px-6 lg:px-8 lg:pt-6 lg:pb-14', full ? 'pb-[calc(6.5rem+env(safe-area-inset-bottom))]' : 'pb-[calc(2.5rem+env(safe-area-inset-bottom))]')}
      >
        {children}
      </main>
      {full && (
        <>
          <BottomNav />
          <SyncController userId={me.id} />
          <FeelingSheet />
        </>
      )}
    </div>
  )
}
