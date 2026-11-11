import { lazy, Suspense, type ReactNode } from 'react'
import { useLocation } from 'wouter'
import { useProfile } from '../../api/queries'
import type { Me } from '../../api/types'
import { cn } from '../../lib/cn'
import { todayISO } from '../../lib/format'
import { ALL_INCLUDE, effectiveInclude } from '../../lib/include'
import { ui, useUi } from '../../state/ui'
import { FeedbackDialog } from '../feedback/FeedbackDialog'
import { FeelingSheet } from '../feeling/FeelingSheet'
import { ManualActivityDialog } from '../session/ManualActivityDialog'
import { Ambient } from './Ambient'
import { AppHeader } from './AppHeader'
import { BottomNav } from './BottomNav'
import { CommandPalette } from './CommandPalette'
import { EventStreamController } from './EventStreamController'
import { SyncController } from './SyncController'
import { t } from '../../i18n'

const AnalysisDrawer = lazy(() => import('../ai/AnalysisDrawer').then((m) => ({ default: m.AnalysisDrawer })))

function GlobalOverlays({ me }: { me: Me }) {
  const [, navigate] = useLocation()
  const profile = useProfile()
  const today = todayISO(profile.data?.timezone)
  const manualOpen = useUi((s) => s.manualActivityOpen)
  const dayAiOpen = useUi((s) => s.dayAnalysisOpen)
  const dayAiRun = useUi((s) => s.dayAnalysisRun)
  const feedbackOpen = useUi((s) => s.feedbackOpen)

  return (
    <>
      <EventStreamController userId={me.id} />
      <CommandPalette me={me} />
      {feedbackOpen && <FeedbackDialog open onClose={ui.closeFeedback} />}
      {manualOpen && (
        <ManualActivityDialog
          open
          onClose={ui.closeManualActivity}
          today={today}
          activity={null}
          onCreated={(a) => {
            ui.closeManualActivity()
            ui.selectActivity(a.id)
            navigate(`/activities/${a.id}`)
            ui.openFeeling()
          }}
          onDeleted={ui.closeManualActivity}
        />
      )}
      {dayAiRun > 0 && (
        <Suspense fallback={null}>
          <AnalysisDrawer
            open={dayAiOpen}
            onClose={ui.closeDayAnalysis}
            subject={{ kind: 'day', date: today, label: t('layout.nav.dayReview') }}
            include={effectiveInclude(ALL_INCLUDE, 'day')}
            runToken={dayAiRun}
          />
        </Suspense>
      )}
    </>
  )
}

export function AppShell({ me, minimal, children }: { me: Me | null; minimal?: boolean; children: ReactNode }) {
  const full = !!me && !minimal
  return (
    <div className="app-root">
      <a href="#main" className="sr-only z-[70] rounded-xl bg-surface px-4 py-2 focus:not-sr-only focus:fixed focus:top-3 focus:left-3">
        {t('layout.nav.skip')}
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
          <GlobalOverlays me={me} />
        </>
      )}
    </div>
  )
}
