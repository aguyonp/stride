import type { UseQueryResult } from '@tanstack/react-query'
import { Activity, ChevronRight, Gauge, ListChecks, RefreshCw, Server } from 'lucide-react'
import type { ReactNode } from 'react'
import type { AdminOverview, AlertLevel, ProviderStats } from '../../api/types'
import { Badge, LevelIcon } from '../../components/ui/Badge'
import { Card } from '../../components/ui/Card'
import { KeyValue } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { cn } from '../../lib/cn'
import { fmtClock, fmtDateTime, fmtNumber } from '../../lib/format'
import { ErrorView } from '../StateViews'
import type { AdminTab } from './AdminView'
import { fmtBytes, fmtUptime, JOB_GROUP_LABEL, providerName } from './adminUtils'

const WAL_WARN_BYTES = 64 * 1024 * 1024

interface Check {
  id: string
  label: string
  level: AlertLevel
  value: string
  detail: string
  tab?: AdminTab
}

const cooling = (p: ProviderStats, now: number) => !!p.cooldownUntil && new Date(p.cooldownUntil).getTime() > now

/** Health checks derived from the overview: what an operator looks at first, worst first in the summary. */
export function healthChecks(ov: AdminOverview, now = Date.now()): Check[] {
  const db = ov.database
  const freeShare = db.sizeBytes > 0 ? db.freeBytes / db.sizeBytes : 0
  const paused = ov.providers.filter((p) => cooling(p, now))
  const limited = ov.providers.filter((p) => p.rateLimited24h > 0)
  return [
    {
      id: 'db',
      label: 'Base SQLite',
      level: !db.ok ? 'danger' : db.walBytes > WAL_WARN_BYTES || (freeShare > 0.25 && db.sizeBytes > 50 * 1024 * 1024) ? 'warn' : 'ok',
      value: `${fmtBytes(db.sizeBytes)} · WAL ${fmtBytes(db.walBytes)}`,
      detail: !db.ok
        ? 'Lecture des métadonnées impossible.'
        : db.walBytes > WAL_WARN_BYTES
          ? 'Journal WAL volumineux : lance une optimisation.'
          : freeShare > 0.25
            ? `${fmtBytes(db.freeBytes)} récupérables par VACUUM.`
            : 'Intègre et compacte.',
      tab: 'maintenance',
    },
    {
      id: 'cache',
      label: ov.cache.backend === 'valkey' ? 'Cache Valkey' : 'Cache mémoire',
      level: ov.cache.ok ? 'ok' : 'danger',
      value: ov.cache.ok ? (ov.cache.latencyMs !== null ? `${fmtNumber(ov.cache.latencyMs, ov.cache.latencyMs < 10 ? 1 : 0)} ms` : 'OK') : 'Injoignable',
      detail: ov.cache.ok ? (ov.cache.backend === 'valkey' ? 'Ping réussi.' : 'Instance unique, sans Valkey.') : 'Les vues sont recalculées à chaque appel.',
    },
    {
      id: 'jobs',
      label: 'Tâches de fond',
      level: ov.jobs.failed24h > 0 ? 'warn' : 'ok',
      value: `${fmtNumber(ov.jobs.active)} active${ov.jobs.active > 1 ? 's' : ''} · ${fmtNumber(ov.jobs.failed24h)} échec${ov.jobs.failed24h > 1 ? 's' : ''} 24 h`,
      detail: ov.jobs.failed24h > 0 ? 'Des synchronisations ou analyses ont échoué.' : 'Aucun échec sur 24 h.',
      tab: 'jobs',
    },
    {
      id: 'providers',
      label: 'Fournisseurs',
      level: paused.length > 0 || limited.length > 0 ? 'warn' : 'ok',
      value: paused.length > 0 ? `${paused.map((p) => providerName(p.provider)).join(', ')} en pause` : `${ov.providers.length} suivi${ov.providers.length > 1 ? 's' : ''}`,
      detail:
        paused.length > 0
          ? `Limitation (429) jusqu’à ${fmtClock(paused[0].cooldownUntil)}.`
          : limited.length > 0
            ? `429 reçus sur 24 h : ${limited.map((p) => providerName(p.provider)).join(', ')}.`
            : 'Aucune limitation en cours.',
      tab: 'providers',
    },
    {
      id: 'logs',
      label: 'Journaux',
      level: ov.logsDropped > 0 ? 'warn' : 'ok',
      value: `${fmtNumber(ov.errors24h)} erreur${ov.errors24h > 1 ? 's' : ''} 24 h`,
      detail: ov.logsDropped > 0 ? `${fmtNumber(ov.logsDropped)} entrées perdues (tampon plein).` : 'Requêtes ≥ 400 et appels sortants en échec.',
      tab: 'logs',
    },
    {
      id: 'streams',
      label: 'Temps réel (SSE)',
      level: 'ok',
      value: `${fmtNumber(ov.streams.clients)} flux`,
      detail: `${fmtNumber(ov.streams.users)} utilisateur${ov.streams.users > 1 ? 's' : ''} connecté${ov.streams.users > 1 ? 's' : ''} à cette instance.`,
    },
  ]
}

const RANK: Record<AlertLevel, number> = { ok: 0, info: 0, warn: 1, danger: 2 }

export function overallLevel(ov: AdminOverview): AlertLevel {
  return healthChecks(ov).reduce<AlertLevel>((acc, c) => (RANK[c.level] > RANK[acc] ? c.level : acc), 'ok')
}

const SUMMARY: Record<AlertLevel, string> = { ok: 'Opérationnel', info: 'Opérationnel', warn: 'À surveiller', danger: 'Incident' }

function GoTo({ tab, onNavigate, children }: { tab: AdminTab; onNavigate: (t: AdminTab) => void; children: ReactNode }) {
  return (
    <button type="button" className="inline-flex items-center gap-1 rounded-lg text-xs font-medium text-accent hover:underline" onClick={() => onNavigate(tab)}>
      {children}
      <ChevronRight className="size-3.5" aria-hidden="true" />
    </button>
  )
}

export function OverviewPanel({ query, onNavigate }: { query: UseQueryResult<AdminOverview>; onNavigate: (t: AdminTab) => void }) {
  if (query.isError && !query.data) return <ErrorView error={query.error} onRetry={() => void query.refetch()} title="Vue d’ensemble indisponible" />
  const ov = query.data
  if (!ov)
    return (
      <div className="grid gap-5 lg:grid-cols-[minmax(0,1.35fr)_minmax(0,1fr)]">
        <Skeleton className="h-96" />
        <Skeleton className="h-96" />
      </div>
    )
  const checks = healthChecks(ov)
  const level = overallLevel(ov)
  const c = ov.counts
  return (
    <div className="grid gap-5 lg:grid-cols-[minmax(0,1.35fr)_minmax(0,1fr)]">
      <div className="min-w-0 space-y-5">
        <Card
          title="État du système"
          icon={<Activity className="size-4" />}
          subtitle={`Actualisé à ${new Date(ov.generatedAt).toLocaleTimeString('fr-FR')} · toutes les 15 s`}
          action={
            <button type="button" className="icon-btn size-9" onClick={() => void query.refetch()} disabled={query.isFetching} aria-label="Actualiser l’état du système">
              <RefreshCw className={cn('size-4', query.isFetching && 'animate-spin')} aria-hidden="true" />
            </button>
          }
        >
          <div className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1.5" role="status">
            <Badge level={level === 'info' ? 'ok' : level}>{SUMMARY[level]}</Badge>
            <span className="text-sm text-ink-2">
              Stride <span className="num font-medium text-ink">{ov.build.version}</span> · en service depuis <span className="num font-medium text-ink">{fmtUptime(ov.uptimeS)}</span>
            </span>
          </div>
          <ul className="divide-y divide-line border-t border-line">
            {checks.map((ch) => (
              <li key={ch.id} className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-x-3 py-2.5">
                <LevelIcon level={ch.level} className="mt-0.5" />
                <div className="min-w-0">
                  <p className="flex flex-wrap items-baseline gap-x-2 text-sm">
                    <span className="font-semibold text-ink">{ch.label}</span>
                    <span className="num text-ink-2">{ch.value}</span>
                  </p>
                  <p className="text-xs text-ink-3">{ch.detail}</p>
                </div>
                {ch.tab ? (
                  <GoTo tab={ch.tab} onNavigate={onNavigate}>
                    <span className="sr-only">{ch.label} : </span>Voir
                  </GoTo>
                ) : (
                  <span />
                )}
              </li>
            ))}
          </ul>
        </Card>

        <Card title="Files de tâches" icon={<ListChecks className="size-4" />} subtitle="Une file par famille : l’IA ne bloque jamais la synchro" action={<GoTo tab="jobs" onNavigate={onNavigate}>Toutes les tâches</GoTo>}>
          <ul className="space-y-3">
            {ov.jobs.groups.map((g) => {
              const pct = g.workers > 0 ? Math.min(100, (g.running / g.workers) * 100) : 0
              return (
                <li key={g.group}>
                  <div className="mb-1 flex flex-wrap items-baseline justify-between gap-x-3 text-sm">
                    <span className="font-medium text-ink">{JOB_GROUP_LABEL[g.group] ?? g.group}</span>
                    <span className="num text-xs text-ink-3">
                      <span className="text-ink-2">{g.running}</span>/{g.workers} actifs · {g.queued} en attente{g.delayed > 0 ? ` · ${g.delayed} à réessayer` : ''}
                    </span>
                  </div>
                  <div className="h-1.5 overflow-hidden rounded-full bg-track" role="img" aria-label={`${JOB_GROUP_LABEL[g.group] ?? g.group} : ${g.running} travailleurs occupés sur ${g.workers}`}>
                    <div className={cn('h-full rounded-full transition-[width] duration-500', pct >= 100 ? 'bg-warn' : 'bg-accent')} style={{ width: `${pct}%` }} />
                  </div>
                </li>
              )
            })}
          </ul>
        </Card>
      </div>

      <div className="min-w-0 space-y-5">
        <Card title="Instance" icon={<Server className="size-4" />} subtitle={ov.build.label}>
          <KeyValue
            items={[
              ['Utilisateurs', `${fmtNumber(c.activeUsers)} actifs / ${fmtNumber(c.users)}`],
              ['Administrateurs actifs', fmtNumber(c.admins)],
              ['Connectés à Garmin ou Strava', fmtNumber(c.connectedUsers)],
              ['IA configurée', fmtNumber(c.llmUsers)],
              ['Activités stockées', fmtNumber(c.activities)],
              ['Sessions ouvertes', fmtNumber(c.sessions)],
              ['Liens de partage actifs', fmtNumber(c.activeShareLinks)],
              ['Inscriptions', ov.registrationMode === 'open' ? 'ouvertes' : 'sur invitation'],
              ['Démarré', fmtDateTime(ov.startedAt)],
              ['Go', ov.build.go],
            ]}
          />
        </Card>
        <Card title="Fournisseurs externes" icon={<Gauge className="size-4" />} action={<GoTo tab="providers" onNavigate={onNavigate}>Détails</GoTo>}>
          {ov.providers.length === 0 ? (
            <p className="text-sm text-ink-3">Aucun appel sortant depuis le démarrage.</p>
          ) : (
            <table className="w-full border-separate border-spacing-0 text-sm">
              <caption className="sr-only">Charge des fournisseurs externes</caption>
              <thead>
                <tr className="text-left text-xs text-ink-3">
                  <th scope="col" className="border-b border-line py-1.5 pr-2 font-medium">Fournisseur</th>
                  <th scope="col" className="border-b border-line px-2 py-1.5 text-right font-medium">24 h</th>
                  <th scope="col" className="border-b border-line px-2 py-1.5 text-right font-medium">429</th>
                  <th scope="col" className="border-b border-line py-1.5 pl-2 text-right font-medium">État</th>
                </tr>
              </thead>
              <tbody>
                {ov.providers.map((p) => {
                  const pause = cooling(p, Date.now())
                  return (
                    <tr key={p.provider}>
                      <th scope="row" className="border-b border-line py-2 pr-2 text-left font-medium text-ink">{providerName(p.provider)}</th>
                      <td className="num border-b border-line px-2 py-2 text-right text-ink-2">{fmtNumber(p.requests24h)}</td>
                      <td className={cn('num border-b border-line px-2 py-2 text-right', p.rateLimited24h > 0 ? 'text-warn' : 'text-ink-2')}>{fmtNumber(p.rateLimited24h)}</td>
                      <td className="border-b border-line py-2 pl-2 text-right">
                        {pause ? <Badge level="warn">Pause {fmtClock(p.cooldownUntil)}</Badge> : p.errors1h > 0 ? <Badge level="warn">{p.errors1h} err. 1 h</Badge> : <Badge level="ok">OK</Badge>}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          )}
        </Card>
      </div>
    </div>
  )
}
