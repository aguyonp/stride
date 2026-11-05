import { Activity, Gauge, PlugZap, Timer, TriangleAlert } from 'lucide-react'
import { useAdminConnections, useAdminMeter, useAdminUsers } from '../../api/queries'
import type { AdminConnection, ProviderStats } from '../../api/types'
import { Badge } from '../../components/ui/Badge'
import { Card } from '../../components/ui/Card'
import { EmptyState } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { cn } from '../../lib/cn'
import { fmtClock, fmtNumber, fmtWhen } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { providerName, UsageBar, userName } from './adminUtils'

function StatusBadge({ status }: { status: string }) {
  if (status === 'ok') return <Badge level="ok">OK</Badge>
  if (status === 'error') return <Badge level="warn">Erreur</Badge>
  if (status === 'disconnected') return <span className="chip border border-line bg-track text-ink-3">Déconnecté</span>
  return <Badge level="info">{status}</Badge>
}

export function ProvidersPanel() {
  return (
    <div className="space-y-5">
      <MeterSection />
      <ConnectionsCard />
    </div>
  )
}

function MeterSection() {
  const meter = useAdminMeter()
  const users = useAdminUsers()
  if (meter.isError) return <ErrorView error={meter.error} onRetry={() => void meter.refetch()} title="Compteur API indisponible" />
  if (!meter.data)
    return (
      <div className="grid gap-4 md:grid-cols-3">
        <Skeleton className="h-56" />
        <Skeleton className="h-56" />
        <Skeleton className="h-56" />
      </div>
    )
  const errors = [...meter.data.recentErrors].sort((a, b) => b.time.localeCompare(a.time))
  return (
    <>
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {meter.data.providers.map((p) => (
          <ProviderMeterCard key={p.provider} p={p} />
        ))}
      </div>
      <Card title="Erreurs récentes" icon={<TriangleAlert className="size-4" />} subtitle="Appels sortants en échec (sans identifiant ni secret)">
        {errors.length === 0 ? (
          <EmptyState title="Aucune erreur récente" />
        ) : (
          <>
            <div className="scroll-thin hidden overflow-x-auto md:block">
              <table className="w-full min-w-[720px] border-separate border-spacing-0 text-sm">
                <caption className="sr-only">Erreurs récentes des fournisseurs</caption>
                <thead>
                  <tr className="text-left text-xs text-ink-3">
                    {['Heure', 'Fournisseur', 'Utilisateur', 'Endpoint', 'Statut', 'Message'].map((h) => (
                      <th key={h} scope="col" className="border-b border-line px-2 py-2 font-medium first:pl-1">
                        {h}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {errors.map((e, i) => (
                    <tr key={i} className="align-top hover:bg-track/50">
                      <td className="num border-b border-line px-2 py-2 pl-1 whitespace-nowrap text-ink-2">{fmtWhen(e.time)}</td>
                      <td className="border-b border-line px-2 py-2 whitespace-nowrap">{providerName(e.provider)}</td>
                      <td className="border-b border-line px-2 py-2 whitespace-nowrap text-ink-2">{userName(users.data, e.userId)}</td>
                      <td className="max-w-[16rem] border-b border-line px-2 py-2 font-mono text-xs break-all text-ink-2">{e.endpoint}</td>
                      <td className="num border-b border-line px-2 py-2">
                        <span className={cn('chip border', e.status === 429 ? 'border-warn/35 bg-warn/10 text-warn' : 'border-danger/30 bg-danger/10 text-danger')}>{e.status || '—'}</span>
                      </td>
                      <td className="border-b border-line px-2 py-2 text-ink-2">{e.message}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <ul className="space-y-2 md:hidden">
              {errors.map((e, i) => (
                <li key={i} className="glass-inset p-3 text-sm">
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-semibold text-ink">{providerName(e.provider)}</span>
                    <span className={cn('chip border', e.status === 429 ? 'border-warn/35 bg-warn/10 text-warn' : 'border-danger/30 bg-danger/10 text-danger')}>{e.status || '—'}</span>
                  </div>
                  <p className="num text-xs text-ink-3">
                    {fmtWhen(e.time)} · {userName(users.data, e.userId)}
                  </p>
                  <p className="mt-1 font-mono text-xs break-all text-ink-2">{e.endpoint}</p>
                  <p className="mt-1 text-xs text-ink-2">{e.message}</p>
                </li>
              ))}
            </ul>
          </>
        )}
      </Card>
    </>
  )
}

function ProviderMeterCard({ p }: { p: ProviderStats }) {
  const cooling = p.cooldownUntil && new Date(p.cooldownUntil).getTime() > Date.now()
  const stats: [string, string, boolean?][] = [
    ['Requêtes 1 h', fmtNumber(p.requests1h)],
    ['Requêtes 24 h', fmtNumber(p.requests24h)],
    ['Erreurs 1 h', fmtNumber(p.errors1h), p.errors1h > 0],
    ['429 sur 24 h', fmtNumber(p.rateLimited24h), p.rateLimited24h > 0],
    ['En cours', fmtNumber(p.inFlight)],
    ['Latence moy.', `${fmtNumber(p.avgLatencyMs)} ms`],
  ]
  return (
    <Card
      title={providerName(p.provider)}
      icon={<Gauge className="size-4" />}
      action={cooling ? <Badge level="warn">Pause</Badge> : <Badge level="ok">Disponible</Badge>}
    >
      {cooling && (
        <p className="mb-3 flex items-center gap-2 rounded-xl border border-warn/30 bg-warn/10 px-3 py-2 text-xs text-ink">
          <Timer className="size-4 flex-none text-warn" aria-hidden="true" />
          Limitation (429) : pause jusqu’à {fmtClock(p.cooldownUntil)}
        </p>
      )}
      <dl className="grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-2 2xl:grid-cols-3">
        {stats.map(([k, v, alert]) => (
          <div key={k} className="glass-inset min-w-0 px-3 py-2">
            <dt className="truncate text-[11px] text-ink-3">{k}</dt>
            <dd className={cn('num truncate text-base font-semibold', alert ? 'text-warn' : 'text-ink')}>{v}</dd>
          </div>
        ))}
      </dl>
      {p.quota && (
        <div className="mt-3 space-y-2.5 border-t border-line pt-3">
          <p className="flex items-center gap-1.5 text-xs font-medium text-ink-2">
            <Activity className="size-3.5" aria-hidden="true" /> Quota de l’application
          </p>
          <UsageBar label="15 minutes" used={p.quota.usage15m} limit={p.quota.limit15m} />
          <UsageBar label="Journée" used={p.quota.usageDaily} limit={p.quota.limitDaily} />
          <p className="num text-[11px] text-ink-3">Mis à jour {fmtWhen(p.quota.updatedAt)}</p>
        </div>
      )}
    </Card>
  )
}

function ConnectionsCard() {
  const conns = useAdminConnections()
  const users = useAdminUsers()
  const items = [...(conns.data ?? [])].sort((a, b) => userName(users.data, a.userId).localeCompare(userName(users.data, b.userId), 'fr') || a.provider.localeCompare(b.provider))
  const lastError = (c: AdminConnection) =>
    c.lastError ? (
      <span className="text-warn">
        {c.lastError}
        {c.lastErrorAt && <span className="num text-ink-3"> · {fmtWhen(c.lastErrorAt)}</span>}
      </span>
    ) : (
      <span className="text-ink-3">—</span>
    )
  return (
    <Card title="Connexions fournisseurs" icon={<PlugZap className="size-4" />} subtitle="Garmin et Strava, par utilisateur">
      {conns.isError ? (
        <ErrorView error={conns.error} onRetry={() => void conns.refetch()} />
      ) : !conns.data ? (
        <Skeleton className="h-28" />
      ) : items.length === 0 ? (
        <EmptyState icon={<PlugZap />} title="Aucune connexion">
          Aucun utilisateur n’a encore connecté Garmin ou Strava.
        </EmptyState>
      ) : (
        <>
          <div className="scroll-thin hidden overflow-x-auto md:block">
            <table className="w-full min-w-[640px] border-separate border-spacing-0 text-sm">
              <caption className="sr-only">Connexions aux fournisseurs</caption>
              <thead>
                <tr className="text-left text-xs text-ink-3">
                  {['Utilisateur', 'Fournisseur', 'Statut', 'Dernière synchro', 'Dernière erreur'].map((h) => (
                    <th key={h} scope="col" className="border-b border-line px-2 py-2 font-medium first:pl-1">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {items.map((c) => (
                  <tr key={`${c.userId}-${c.provider}`} className="align-top hover:bg-track/50">
                    <th scope="row" className="border-b border-line px-2 py-2 pl-1 text-left font-semibold text-ink">
                      {userName(users.data, c.userId)}
                    </th>
                    <td className="border-b border-line px-2 py-2">{providerName(c.provider)}</td>
                    <td className="border-b border-line px-2 py-2">
                      <StatusBadge status={c.status} />
                    </td>
                    <td className="num border-b border-line px-2 py-2 text-ink-2">{c.lastSyncAt ? fmtWhen(c.lastSyncAt) : 'jamais'}</td>
                    <td className="border-b border-line px-2 py-2 text-xs">{lastError(c)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <ul className="space-y-2 md:hidden">
            {items.map((c) => (
              <li key={`${c.userId}-${c.provider}`} className="glass-inset p-3 text-sm">
                <div className="flex items-center justify-between gap-2">
                  <span className="min-w-0 truncate font-semibold text-ink">
                    {userName(users.data, c.userId)} · {providerName(c.provider)}
                  </span>
                  <StatusBadge status={c.status} />
                </div>
                <p className="num text-xs text-ink-3">Synchro : {c.lastSyncAt ? fmtWhen(c.lastSyncAt) : 'jamais'}</p>
                {c.lastError && <p className="mt-1 text-xs">{lastError(c)}</p>}
              </li>
            ))}
          </ul>
        </>
      )}
    </Card>
  )
}
