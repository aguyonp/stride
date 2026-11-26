import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { IS_MOCK } from '../../api/client'
import { invalidateDomain, qk, qk4, useJobs } from '../../api/queries'
import { qk42, SHARE_UPDATE_ROOTS } from '../../api/queriesV42'
import type { InboxPage, JobListResponse, JobState, StrideStreamEventMap } from '../../api/types'
import { emitStreamEvent, setStreamConnected, subscribeStreamEvent } from '../../state/events'
import { useToast } from '../ui/Toast'
import { t } from '../../i18n'

const INVALIDATE_ON_JOB_DONE = [
  'activities',
  'activity',
  'day',
  'week',
  'calendar',
  'load',
  'readiness',
  'cycles',
  'bodymap',
  'shoes',
  'report',
  'records',
  'efficiency',
  'integrations',
]

const STREAM_EVENTS: (keyof StrideStreamEventMap)[] = [
  'connected',
  'job.updated',
  'sync.updated',
  'alert.triggered',
  'llm.token',
  'llm.done',
  'share.updated',
  'notification.created',
  'notification.updated',
]

const SHARE_KINDS: StrideStreamEventMap['share.updated']['kind'][] = ['directive', 'remark', 'plan', 'event']

/**
 * Maintains the authenticated SSE connection to `/api/events/stream` and routes
 * real or synthetic stream frames into React Query caches and UI notifications.
 * Every (re)connection refetches sync status and jobs, since frames sent while
 * disconnected are lost; the queries themselves poll slowly while the stream is down.
 */
export function EventStreamController({ userId }: { userId: number }) {
  const qc = useQueryClient()
  const { toast } = useToast()
  const prevJobStates = useRef<Map<string, JobState>>(new Map())

  useJobs(20, Boolean(userId))

  useEffect(() => {
    const seen = prevJobStates.current
    seen.clear()

    const unsubs = [
      subscribeStreamEvent('connected', () => {
        void qc.invalidateQueries({ queryKey: qk.sync })
        void qc.invalidateQueries({ queryKey: qk4.jobs })
      }),

      subscribeStreamEvent('job.updated', (job) => {
        const prev = seen.get(job.id)
        seen.set(job.id, job.state)

        qc.setQueryData<JobListResponse>(qk4.jobs, (old) => {
          const rest = (old?.items ?? []).filter((x) => x.id !== job.id)
          const items = [job, ...rest].sort((a, b) => (a.updatedAt > b.updatedAt ? -1 : 1)).slice(0, 20)
          return { items }
        })

        const isSyncOrImport = job.kind.startsWith('sync.') || job.kind === 'import.activity'
        if (isSyncOrImport && job.state === 'completed' && prev !== 'completed') {
          void invalidateDomain(qc, INVALIDATE_ON_JOB_DONE)
          if (job.kind === 'import.activity') {
            toast({ title: t('layout.stream.imported'), description: job.label, tone: 'success' })
          }
        } else if (job.kind.startsWith('backup.') && (job.state === 'completed' || job.state === 'failed') && prev !== job.state) {
          void qc.invalidateQueries({ queryKey: qk42.backups })
          if (job.state === 'failed' && job.error) toast({ title: t('layout.stream.backupFailed'), description: job.error, tone: 'error' })
        } else if (job.state === 'failed' && prev !== 'failed' && job.error) {
          toast({ title: t('layout.stream.jobFailed'), description: job.error, tone: 'error' })
        }
      }),

      subscribeStreamEvent('sync.updated', (sync) => {
        qc.setQueryData(qk.sync, sync)
      }),

      subscribeStreamEvent('alert.triggered', (alert) => {
        toast({
          title: alert.title,
          description: alert.detail,
          tone: alert.level === 'critical' ? 'error' : 'info',
        })
      }),

      subscribeStreamEvent('share.updated', (ev) => {
        void invalidateDomain(qc, SHARE_UPDATE_ROOTS[ev.kind] ?? ['share'])
        if (SHARE_KINDS.includes(ev.kind))
          toast({ title: t(`layout.stream.share.${ev.kind}`), description: ev.kind === 'plan' || ev.kind === 'event' ? t('layout.stream.visiblePlan') : t('layout.stream.visibleHealth'), tone: 'info' })
      }),

      // The centre (§21) refreshes from the server event: the entry itself, or the new unread count
      // after a read from another tab. No toast here — `alert.triggered` and `share.updated` own that.
      subscribeStreamEvent('notification.created', () => void qc.invalidateQueries({ queryKey: qk.inbox })),
      subscribeStreamEvent('notification.updated', (ev) =>
        qc.setQueryData<InboxPage>(qk.inbox, (old) => (old ? { ...old, unread: ev.unread } : old)),
      ),

      subscribeStreamEvent('llm.done', (done) => {
        const a = done.analysis
        if (a) {
          qc.setQueryData(
            ['llm', 'analyses', a.kind, a.ref],
            (list: (typeof a)[] | undefined) => [a, ...(list ?? []).filter((x) => x.id !== a.id)].slice(0, 20),
          )
        }
      }),
    ]

    return () => {
      unsubs.forEach((fn) => fn())
    }
  }, [qc, toast, userId])

  useEffect(() => {
    if (!userId) return
    if (IS_MOCK) {
      emitStreamEvent('connected', { status: 'ok', userId, serverTime: new Date().toISOString() })
      return () => setStreamConnected(false)
    }
    if (typeof EventSource === 'undefined') return

    const es = new EventSource('/api/events/stream', { withCredentials: true })
    const handlers: Array<[string, EventListener]> = []

    for (const name of STREAM_EVENTS) {
      const handler: EventListener = (ev) => {
        const msg = ev as MessageEvent<string>
        if (!msg.data) return
        try {
          const parsed = JSON.parse(msg.data) as StrideStreamEventMap[typeof name]
          emitStreamEvent(name, parsed)
        } catch {
          // Ignore malformed event payloads.
        }
      }
      es.addEventListener(name, handler)
      handlers.push([name, handler])
    }

    es.onerror = () => {
      setStreamConnected(false)
    }

    return () => {
      for (const [name, handler] of handlers) {
        es.removeEventListener(name, handler)
      }
      es.close()
      setStreamConnected(false)
    }
  }, [userId])

  return null
}
