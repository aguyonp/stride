import { CircleAlert, Loader2, Radio, RefreshCw, Timer } from 'lucide-react'
import { Link } from 'wouter'
import { IS_MOCK } from '../../api/client'
import { useIntegrations, useStartSync, useSyncStatus } from '../../api/queries'
import type { JobQueue, JobStatus, ProviderSync, SyncStatus } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtClock, fmtNumber, fmtWhen } from '../../lib/format'
import { useActiveJobs, useStreamConnected } from '../../state/events'
import { Popover } from '../ui/Popover'

type Tone = 'running' | 'cooldown' | 'error' | 'ok' | 'idle'

const QUEUE_META: Record<JobQueue, { label: string; cls: string }> = {
  critical: { label: 'Priorité critique', cls: 'border-accent/30 bg-accent/15 text-accent' },
  default: { label: 'File standard', cls: 'border-info/30 bg-info/12 text-info' },
  low: { label: 'Tâche de fond', cls: 'border-line bg-track text-ink-3' },
}

function syncTone(s: SyncStatus | undefined, hasActiveJob: boolean, now = Date.now()): Tone {
  if (hasActiveJob || s?.running) return 'running'
  if (!s) return 'idle'
  if (s.cooldownUntil && new Date(s.cooldownUntil).getTime() > now) return 'cooldown'
  if (s.providers.garmin?.error || s.providers.strava?.error) return 'error'
  return s.lastSyncAt ? 'ok' : 'idle'
}

function jobProgressPct(job: JobStatus): number | null {
  if (job.progressPct !== null && Number.isFinite(job.progressPct)) {
    return Math.max(0, Math.min(100, Math.round(job.progressPct)))
  }
  if (job.processed !== null && job.total !== null && job.total > 0) {
    return Math.max(0, Math.min(100, Math.round((job.processed / job.total) * 100)))
  }
  return null
}

function jobHeadline(job: JobStatus): string {
  if (job.processed !== null && job.total !== null && job.total > 0) {
    const unit = job.kind.startsWith('sync.') ? ' séances traitées' : ''
    return `${job.label} : ${job.processed}/${job.total}${unit}`
  }
  return job.label
}

function label(s: SyncStatus | undefined, tone: Tone, primaryJob: JobStatus | null): string {
  if (primaryJob) return jobHeadline(primaryJob)
  if (tone === 'running') return 'Synchro…'
  if (tone === 'cooldown') return `Pause API · ${fmtClock(s?.cooldownUntil)}`
  if (tone === 'error') return 'Synchro partielle'
  if (tone === 'ok') return `Synchro ${fmtWhen(s?.lastSyncAt)}`
  return 'Jamais synchronisé'
}

function ProviderLine({ name, p }: { name: string; p: ProviderSync | null }) {
  if (!p)
    return (
      <li className="flex items-center justify-between gap-3 py-1.5 text-sm">
        <span className="text-ink-2">{name}</span>
        <span className="text-ink-3">non connecté</span>
      </li>
    )
  return (
    <li className="py-1.5 text-sm">
      <div className="flex items-center justify-between gap-3">
        <span className="font-medium text-ink">{name}</span>
        <span className="num text-ink-3">{p.lastSyncAt ? fmtWhen(p.lastSyncAt) : 'jamais'}</span>
      </div>
      <p className="text-xs text-ink-3">
        {p.newActivities > 0
          ? `${fmtNumber(p.newActivities)} nouvelle${p.newActivities > 1 ? 's' : ''} activité${p.newActivities > 1 ? 's' : ''}`
          : 'Aucune nouvelle activité'}
      </p>
      {p.error && <p className="mt-0.5 text-xs text-warn">{p.error}</p>}
    </li>
  )
}

function ActiveJobRow({ job }: { job: JobStatus }) {
  const pct = jobProgressPct(job)
  const q = QUEUE_META[job.queue]
  return (
    <li className="py-2 text-xs">
      <div className="flex items-start justify-between gap-2">
        <span className="min-w-0 font-medium text-ink">{jobHeadline(job)}</span>
        <span className={cn('chip h-5 border px-2 text-[10px]', q.cls)}>{q.label}</span>
      </div>
      {job.state === 'retrying' && (
        <p className="num mt-0.5 text-[11px] text-warn">
          Nouvelle tentative ({job.attempts}/{job.maxAttempts})
        </p>
      )}
      {pct !== null && (
        <div
          className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-track"
          role="progressbar"
          aria-label={jobHeadline(job)}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={pct}
        >
          <div className="h-full rounded-full bg-accent transition-[width] duration-300" style={{ width: `${pct}%` }} />
        </div>
      )}
    </li>
  )
}

export function SyncIndicator() {
  const integ = useIntegrations()
  const connected = integ.data ? integ.data.garmin.connected || integ.data.strava.connected : true
  const activeJobs = useActiveJobs()
  const streamLive = useStreamConnected()
  const status = useSyncStatus(connected)
  const start = useStartSync()
  const s = status.data
  const primaryJob = activeJobs[0] ?? null
  const primaryPct = primaryJob ? jobProgressPct(primaryJob) : null
  const missing = integ.data?.strava.connected ? (integ.data.strava.missingScopes ?? []) : []
  const tone = syncTone(s, activeJobs.length > 0)
  const text = label(s, tone, primaryJob)
  const Icon = tone === 'running' ? Loader2 : tone === 'cooldown' ? Timer : tone === 'error' ? CircleAlert : RefreshCw
  const color = tone === 'cooldown' || tone === 'error' ? 'text-warn' : tone === 'running' ? 'text-accent' : 'text-ink-2'

  if (!connected && activeJobs.length === 0) return null

  return (
    <Popover
      kind="dialog"
      trigger={(p) => (
        <button
          {...p}
          type="button"
          className={cn(
            'inline-flex h-10 min-w-10 flex-none items-center justify-center gap-2 rounded-full border border-line bg-track/70 px-2.5 transition-colors hover:bg-track xl:px-3',
            tone === 'cooldown' || tone === 'error' ? 'border-warn/35' : tone === 'running' ? 'border-accent/35' : '',
          )}
          aria-label={`État de la synchronisation : ${text}`}
        >
          <Icon className={cn('size-[1.125rem]', color, tone === 'running' && 'animate-spin')} aria-hidden="true" />
          <span className="num hidden max-w-[13rem] truncate text-xs font-semibold text-ink-2 xl:inline">{text}</span>
          {primaryPct !== null && (
            <span className="num hidden rounded-full bg-accent/15 px-1.5 py-0.5 text-[10px] font-semibold text-accent sm:inline">
              {primaryPct} %
            </span>
          )}
        </button>
      )}
      panelClassName="w-[min(22rem,calc(100vw-2rem))] p-3.5"
    >
      {() => (
        <div>
          <div className="flex items-center justify-between gap-2">
            <p className="text-sm font-semibold text-ink">Synchronisation & file</p>
            <span className={cn('inline-flex items-center gap-1 text-[11px]', streamLive || IS_MOCK ? 'text-ok' : 'text-ink-3')}>
              <Radio className="size-3" aria-hidden="true" />
              {IS_MOCK ? 'Flux simulé' : streamLive ? 'Temps réel actif' : 'Connexion…'}
            </span>
          </div>
          <p className={cn('num mt-0.5 text-xs', color)} aria-live="polite">
            {text}
          </p>
          {tone === 'cooldown' && (
            <p className="mt-1 text-xs text-ink-3">Le fournisseur limite les requêtes (429). Stride reprendra automatiquement après la pause.</p>
          )}
          {activeJobs.length > 0 && (
            <ul className="mt-2 divide-y divide-line border-t border-line" aria-label="Tâches en cours">
              {activeJobs.map((j) => (
                <ActiveJobRow key={j.id} job={j} />
              ))}
            </ul>
          )}
          {connected && (
            <ul className="mt-2 divide-y divide-line border-t border-line">
              <ProviderLine name="Garmin" p={s?.providers.garmin ?? null} />
              <ProviderLine name="Strava" p={s?.providers.strava ?? null} />
            </ul>
          )}
          {missing.length > 0 && (
            <p className="mt-2 text-xs text-warn">
              Strava : autorisations incomplètes, activités et kudos non lus.{' '}
              <Link href="/settings#integrations" className="font-semibold underline underline-offset-2">
                Reconnecter
              </Link>
            </p>
          )}
          {s?.lastLiveAt && <p className="num mt-2 text-xs text-ink-3">Dernier appel live : {fmtWhen(s.lastLiveAt)}</p>}
          {connected && (
            <button
              type="button"
              className="btn btn-ghost mt-3 w-full"
              data-autofocus
              disabled={tone === 'running' || tone === 'cooldown' || start.isPending}
              onClick={() => start.mutate()}
            >
              <RefreshCw className={cn('size-4', start.isPending && 'animate-spin')} aria-hidden="true" />
              Synchroniser maintenant
            </button>
          )}
        </div>
      )}
    </Popover>
  )
}
