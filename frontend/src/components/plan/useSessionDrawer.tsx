import { lazy, Suspense, useState } from 'react'
import type { PlannedSession } from '../../api/types'

const SessionDrawer = lazy(() => import('./SessionDrawer').then((m) => ({ default: m.SessionDrawer })))

/** Lazily loaded session drawer for read-mostly screens (dashboard cards). */
export function useSessionDrawer() {
  const [session, setSession] = useState<PlannedSession | null>(null)
  const node = session && (
    <Suspense fallback={null}>
      <SessionDrawer session={session} onClose={() => setSession(null)} />
    </Suspense>
  )
  return { open: setSession, node }
}
