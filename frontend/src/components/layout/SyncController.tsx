import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { invalidateDomain, useIntegrations, useStartSync, useSyncStatus } from '../../api/queries'
import { useToast } from '../ui/Toast'
import { t } from '../../i18n'

let kickedFor: number | null = null

/** Lazy sync on load (POST /api/sync) and cache refresh when a sync run finishes; idle in standalone mode. */
export function SyncController({ userId }: { userId: number }) {
  const qc = useQueryClient()
  const start = useStartSync()
  const integ = useIntegrations()
  const connected = integ.data ? integ.data.garmin.connected || integ.data.strava.connected : null
  const status = useSyncStatus(connected === true)
  const { toast } = useToast()
  const prev = useRef<boolean | undefined>(undefined)
  const mutate = start.mutate

  useEffect(() => {
    if (kickedFor === userId || !connected) return
    kickedFor = userId
    mutate()
  }, [userId, mutate, connected])

  const running = status.data?.running
  useEffect(() => {
    if (prev.current === true && running === false) {
      void invalidateDomain(qc, ['activities', 'activity', 'day', 'week', 'report', 'integrations'])
      const p = status.data?.providers
      const n = (p?.garmin?.newActivities ?? 0) + (p?.strava?.newActivities ?? 0)
      if (n > 0) toast({ title: t('layout.syncDone.synced', { count: n }) })
    }
    prev.current = running
  }, [running, qc, status.data?.providers, toast])

  return null
}
