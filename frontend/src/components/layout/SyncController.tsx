import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { invalidateDomain, useStartSync, useSyncStatus } from '../../api/queries'
import { useToast } from '../ui/Toast'

let kickedFor: number | null = null

/** Lazy sync on load (POST /api/sync) and cache refresh when a sync run finishes. */
export function SyncController({ userId }: { userId: number }) {
  const qc = useQueryClient()
  const start = useStartSync()
  const status = useSyncStatus()
  const { toast } = useToast()
  const prev = useRef<boolean | undefined>(undefined)
  const mutate = start.mutate

  useEffect(() => {
    if (kickedFor === userId) return
    kickedFor = userId
    mutate()
  }, [userId, mutate])

  const running = status.data?.running
  useEffect(() => {
    if (prev.current === true && running === false) {
      void invalidateDomain(qc, ['activities', 'activity', 'day', 'week', 'report', 'integrations'])
      const p = status.data?.providers
      const n = (p?.garmin?.newActivities ?? 0) + (p?.strava?.newActivities ?? 0)
      if (n > 0) toast({ title: `${n} nouvelle${n > 1 ? 's' : ''} activité${n > 1 ? 's' : ''} synchronisée${n > 1 ? 's' : ''}` })
    }
    prev.current = running
  }, [running, qc, status.data?.providers, toast])

  return null
}
