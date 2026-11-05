import { CircleAlert, Loader2, RefreshCw, Timer } from 'lucide-react'
import { useStartSync, useSyncStatus } from '../../api/queries'
import type { ProviderSync, SyncStatus } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtClock, fmtNumber, fmtWhen } from '../../lib/format'
import { Popover } from '../ui/Popover'

type Tone = 'running' | 'cooldown' | 'error' | 'ok' | 'idle'

function syncTone(s: SyncStatus | undefined, now = Date.now()): Tone {
  if (!s) return 'idle'
  if (s.running) return 'running'
  if (s.cooldownUntil && new Date(s.cooldownUntil).getTime() > now) return 'cooldown'
  if (s.providers.garmin?.error || s.providers.strava?.error) return 'error'
  return s.lastSyncAt ? 'ok' : 'idle'
}

function label(s: SyncStatus | undefined, tone: Tone): string {
  if (tone === 'running') return 'Synchro…'
  if (tone === 'cooldown') return `Pause API · ${fmtClock(s?.cooldownUntil)}`
  if (tone === 'error') return 'Synchro partielle'
  if (tone === 'ok') return `Synchro ${fmtWhen(s?.lastSyncAt)}`
  return 'Jamais synchronisé'
}

function ProviderLine({ name, p }: { name: string; p: ProviderSync | null }) {
  if (!p) return (
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
      <p className="text-xs text-ink-3">{p.newActivities > 0 ? `${fmtNumber(p.newActivities)} nouvelle${p.newActivities > 1 ? 's' : ''} activité${p.newActivities > 1 ? 's' : ''}` : 'Aucune nouvelle activité'}</p>
      {p.error && <p className="mt-0.5 text-xs text-warn">{p.error}</p>}
    </li>
  )
}

export function SyncIndicator() {
  const status = useSyncStatus()
  const start = useStartSync()
  const s = status.data
  const tone = syncTone(s)
  const text = label(s, tone)
  const Icon = tone === 'running' ? Loader2 : tone === 'cooldown' ? Timer : tone === 'error' ? CircleAlert : RefreshCw
  const color = tone === 'cooldown' || tone === 'error' ? 'text-warn' : tone === 'running' ? 'text-accent' : 'text-ink-2'
  return (
    <Popover
      kind="dialog"
      trigger={(p) => (
        <button
          {...p}
          type="button"
          className={cn('inline-flex h-10 min-w-10 flex-none items-center justify-center gap-2 rounded-full border border-line bg-track/70 px-2.5 transition-colors hover:bg-track xl:px-3', tone === 'cooldown' || tone === 'error' ? 'border-warn/35' : '')}
          aria-label={`État de la synchronisation : ${text}`}
        >
          <Icon className={cn('size-[1.125rem]', color, tone === 'running' && 'animate-spin')} aria-hidden="true" />
          <span className="num hidden max-w-[11rem] truncate text-xs font-semibold text-ink-2 xl:inline">{text}</span>
        </button>
      )}
      panelClassName="w-[min(20rem,calc(100vw-2rem))] p-3"
    >
      {() => (
        <div>
          <p className="text-sm font-semibold text-ink">Synchronisation</p>
          <p className={cn('mt-0.5 text-xs', color)} aria-live="polite">
            {text}
          </p>
          {tone === 'cooldown' && <p className="mt-1 text-xs text-ink-3">Le fournisseur limite les requêtes (429). Stride reprendra automatiquement après la pause.</p>}
          <ul className="mt-2 divide-y divide-line border-t border-line">
            <ProviderLine name="Garmin" p={s?.providers.garmin ?? null} />
            <ProviderLine name="Strava" p={s?.providers.strava ?? null} />
          </ul>
          {s?.lastLiveAt && <p className="num mt-2 text-xs text-ink-3">Dernier appel live : {fmtWhen(s.lastLiveAt)}</p>}
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
        </div>
      )}
    </Popover>
  )
}
