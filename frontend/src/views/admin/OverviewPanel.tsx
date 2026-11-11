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
import { lazyRecord, t, tx } from '../../i18n'

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
      label: t('admin.overview.db'),
      level: !db.ok ? 'danger' : db.walBytes > WAL_WARN_BYTES || (freeShare > 0.25 && db.sizeBytes > 50 * 1024 * 1024) ? 'warn' : 'ok',
      value: `${fmtBytes(db.sizeBytes)} · WAL ${fmtBytes(db.walBytes)}`,
      detail: !db.ok
        ? t('admin.overview.dbError')
        : db.walBytes > WAL_WARN_BYTES
          ? t('admin.overview.walLarge')
          : freeShare > 0.25
            ? t('admin.overview.vacuum', { size: fmtBytes(db.freeBytes) })
            : t('admin.overview.healthy'),
      tab: 'maintenance',
    },
    {
      id: 'cache',
      label: ov.cache.backend === 'valkey' ? t('admin.overview.cacheValkey') : t('admin.overview.cacheMemory'),
      level: ov.cache.ok ? 'ok' : 'danger',
      value: ov.cache.ok ? (ov.cache.latencyMs !== null ? `${fmtNumber(ov.cache.latencyMs, ov.cache.latencyMs < 10 ? 1 : 0)} ms` : 'OK') : t('admin.overview.unreachable'),
      detail: ov.cache.ok ? (ov.cache.backend === 'valkey' ? t('admin.overview.ping') : t('admin.overview.single')) : t('admin.overview.noCache'),
    },
    {
      id: 'jobs',
      label: t('admin.overview.jobs'),
      level: ov.jobs.failed24h > 0 ? 'warn' : 'ok',
      value: t('admin.overview.jobsValue', { active: t('admin.overview.active', { count: ov.jobs.active }), failed: t('admin.overview.failures', { count: ov.jobs.failed24h }) }),
      detail: ov.jobs.failed24h > 0 ? t('admin.overview.jobsFailed') : t('admin.overview.noFailure'),
      tab: 'jobs',
    },
    {
      id: 'providers',
      label: t('admin.overview.providers'),
      level: paused.length > 0 || limited.length > 0 ? 'warn' : 'ok',
      value: paused.length > 0 ? t('admin.overview2.paused', { list: paused.map((p) => providerName(p.provider)).join(', ') }) : t('admin.overview2.tracked', { count: ov.providers.length }),
      detail:
        paused.length > 0
          ? t('admin.overview.limitedUntil', { time: fmtClock(paused[0].cooldownUntil) })
          : limited.length > 0
            ? t('admin.overview.limited24h', { list: limited.map((p) => providerName(p.provider)).join(', ') })
            : t('admin.overview.noLimit'),
      tab: 'providers',
    },
    {
      id: 'logs',
      label: t('admin.overview.logs'),
      level: ov.logsDropped > 0 ? 'warn' : 'ok',
      value: t('admin.overview2.errors24h', { count: fmtNumber(ov.errors24h) }),
      detail: ov.logsDropped > 0 ? t('admin.overview.logsDropped', { count: fmtNumber(ov.logsDropped) }) : t('admin.overview.logsHint'),
      tab: 'logs',
    },
    {
      id: 'streams',
      label: t('admin.overview.sse'),
      level: 'ok',
      value: t('admin.overview2.streams', { count: fmtNumber(ov.streams.clients) }),
      detail: t('admin.overview.sseUsers', { count: ov.streams.users }),
    },
  ]
}

const RANK: Record<AlertLevel, number> = { ok: 0, info: 0, warn: 1, danger: 2 }

export function overallLevel(ov: AdminOverview): AlertLevel {
  return healthChecks(ov).reduce<AlertLevel>((acc, c) => (RANK[c.level] > RANK[acc] ? c.level : acc), 'ok')
}

const SUMMARY: Record<AlertLevel, string> = lazyRecord(['ok', 'info', 'warn', 'danger'] as const, (k) => t(`admin.overview.summary.${k}`))

function GoTo({ tab, onNavigate, children }: { tab: AdminTab; onNavigate: (t: AdminTab) => void; children: ReactNode }) {
  return (
    <button type="button" className="inline-flex items-center gap-1 rounded-lg text-xs font-medium text-accent hover:underline" onClick={() => onNavigate(tab)}>
      {children}
      <ChevronRight className="size-3.5" aria-hidden="true" />
    </button>
  )
}

export function OverviewPanel({ query, onNavigate }: { query: UseQueryResult<AdminOverview>; onNavigate: (t: AdminTab) => void }) {
  if (query.isError && !query.data) return <ErrorView error={query.error} onRetry={() => void query.refetch()} title={t('admin.overview.unavailable')} />
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
          title={t('admin.overview.status')}
          icon={<Activity className="size-4" />}
          subtitle={t('admin.overview.refreshedAt', { time: fmtClock(ov.generatedAt) })}
          action={
            <button type="button" className="icon-btn size-9" onClick={() => void query.refetch()} disabled={query.isFetching} aria-label={t('admin.overview.refresh')}>
              <RefreshCw className={cn('size-4', query.isFetching && 'animate-spin')} aria-hidden="true" />
            </button>
          }
        >
          <div className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1.5" role="status">
            <Badge level={level === 'info' ? 'ok' : level}>{SUMMARY[level]}</Badge>
            <span className="text-sm text-ink-2">
              {tx('admin.overview.uptime', { version: <span className="num font-medium text-ink">{ov.build.version}</span>, uptime: <span className="num font-medium text-ink">{fmtUptime(ov.uptimeS)}</span> })}
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
                    <span className="sr-only">{ch.label} : </span>
                    {t('admin.overview.see')}
                  </GoTo>
                ) : (
                  <span />
                )}
              </li>
            ))}
          </ul>
        </Card>

        <Card title={t('admin.overview.queues')} icon={<ListChecks className="size-4" />} subtitle={t('admin.overview.queuesHint')} action={<GoTo tab="jobs" onNavigate={onNavigate}>{t('admin.overview.allJobs')}</GoTo>}>
          <ul className="space-y-3">
            {ov.jobs.groups.map((g) => {
              const pct = g.workers > 0 ? Math.min(100, (g.running / g.workers) * 100) : 0
              return (
                <li key={g.group}>
                  <div className="mb-1 flex flex-wrap items-baseline justify-between gap-x-3 text-sm">
                    <span className="font-medium text-ink">{JOB_GROUP_LABEL[g.group] ?? g.group}</span>
                    <span className="num text-xs text-ink-3">
                      {t('admin.overview.workers', { running: g.running, workers: g.workers, queued: g.queued })}
                      {g.delayed > 0 ? t('admin.overview.retryLater', { count: g.delayed }) : ''}
                    </span>
                  </div>
                  <div className="h-1.5 overflow-hidden rounded-full bg-track" role="img" aria-label={t('admin.overview.workersAria', { group: JOB_GROUP_LABEL[g.group] ?? g.group, running: g.running, workers: g.workers })}>
                    <div className={cn('h-full rounded-full transition-[width] duration-500', pct >= 100 ? 'bg-warn' : 'bg-accent')} style={{ width: `${pct}%` }} />
                  </div>
                </li>
              )
            })}
          </ul>
        </Card>
      </div>

      <div className="min-w-0 space-y-5">
        <Card title={t('admin.overview.instance')} icon={<Server className="size-4" />} subtitle={ov.build.label}>
          <KeyValue
            items={[
              [t('admin.overview.users'), t('admin.overview.usersValue', { active: fmtNumber(c.activeUsers), total: fmtNumber(c.users) })],
              [t('admin.overview.admins'), fmtNumber(c.admins)],
              [t('admin.overview.connected'), fmtNumber(c.connectedUsers)],
              [t('admin.overview.llm'), fmtNumber(c.llmUsers)],
              [t('admin.overview.activities'), fmtNumber(c.activities)],
              [t('admin.overview.sessions'), fmtNumber(c.sessions)],
              [t('admin.overview.links'), fmtNumber(c.activeShareLinks)],
              [t('admin.overview.registration'), ov.registrationMode === 'open' ? t('admin.overview.open') : t('admin.overview.invite')],
              [t('admin.overview.started'), fmtDateTime(ov.startedAt)],
              ['Go', ov.build.go],
            ]}
          />
        </Card>
        <Card title={t('admin.overview.external')} icon={<Gauge className="size-4" />} action={<GoTo tab="providers" onNavigate={onNavigate}>{t('admin.overview.details')}</GoTo>}>
          {ov.providers.length === 0 ? (
            <p className="text-sm text-ink-3">{t('admin.overview.noCalls')}</p>
          ) : (
            <table className="w-full border-separate border-spacing-0 text-sm">
              <caption className="sr-only">{t('admin.overview.externalCaption')}</caption>
              <thead>
                <tr className="text-left text-xs text-ink-3">
                  <th scope="col" className="border-b border-line py-1.5 pr-2 font-medium">{t('admin.overview.provider')}</th>
                  <th scope="col" className="border-b border-line px-2 py-1.5 text-right font-medium">24 h</th>
                  <th scope="col" className="border-b border-line px-2 py-1.5 text-right font-medium">429</th>
                  <th scope="col" className="border-b border-line py-1.5 pl-2 text-right font-medium">{t('admin.overview.state')}</th>
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
                        {pause ? <Badge level="warn">{t('admin.overview.pause', { time: fmtClock(p.cooldownUntil) })}</Badge> : p.errors1h > 0 ? <Badge level="warn">{t('admin.overview.errors', { count: p.errors1h })}</Badge> : <Badge level="ok">OK</Badge>}
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
