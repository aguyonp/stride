import { Loader2, RefreshCw, ScrollText } from 'lucide-react'
import { useState } from 'react'
import { useAdminLogs, useAdminUsers } from '../../api/queries'
import type { LogEntry, LogFilter } from '../../api/types'
import { Card } from '../../components/ui/Card'
import { SegmentedControl } from '../../components/ui/SegmentedControl'
import { cn } from '../../lib/cn'
import { fmtNumber } from '../../lib/format'
import { getLang, LOCALE, t } from '../../i18n'

const timeFmt = { format: (d: Date) => new Intl.DateTimeFormat(LOCALE[getLang()], { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(d) }

function describe(e: LogEntry): string {
  if (e.category === 'user_action') return e.action + (e.message ? ` · ${e.message}` : '')
  if (e.method === 'OUT') return `${e.provider} ${e.target}`
  return `${e.method} ${e.target}`
}

export function LogsPanel() {
  const [f, setF] = useState<LogFilter>({ category: 'all', level: 'all', userId: null })
  const users = useAdminUsers()
  const logs = useAdminLogs(f)
  const items = logs.data?.pages.flatMap((p) => p.items) ?? []
  const dropped = logs.data?.pages[0]?.dropped ?? 0
  return (
    <Card title={t('admin.logs.title')} icon={<ScrollText className="size-4" />} subtitle={t('admin.logs.subtitle')}>
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <SegmentedControl<LogFilter['level']>
          ariaLabel={t('admin.logs.severity')}
          value={f.level}
          onChange={(level) => setF({ ...f, level })}
          options={[
            { value: 'all', label: t('admin.logs.all') },
          { value: 'warn', label: t('admin.logs.warnings') },
          { value: 'error', label: t('admin.logs.errors') },
          ]}
        />
        <SegmentedControl<LogFilter['category']>
          ariaLabel={t('admin.logs.category')}
          value={f.category}
          onChange={(category) => setF({ ...f, category })}
          options={[
            { value: 'all', label: t('admin.logs.all') },
          { value: 'request', label: t('admin.logs.requests') },
          { value: 'user_action', label: t('admin.logs.actions') },
          ]}
        />
        <select
          aria-label={t('admin.logs.user')}
          className="field h-10 w-auto min-w-40"
          value={f.userId ?? ''}
          onChange={(e) => setF({ ...f, userId: e.target.value ? Number(e.target.value) : null })}
        >
          <option value="">{t('admin.logs.allUsers')}</option>
          {users.data?.map((u) => (
            <option key={u.id} value={u.id}>
              {u.displayName}
            </option>
          ))}
        </select>
        <button type="button" className="btn btn-ghost ml-auto" onClick={() => void logs.refetch()} aria-label={t('admin.logs.refresh')} disabled={logs.isFetching}>
          <RefreshCw className={cn('size-4', logs.isFetching && 'animate-spin')} aria-hidden="true" />
        </button>
      </div>
      {logs.isError && <p className="text-sm text-danger">{logs.error.message}</p>}
      {logs.isPending ? (
        <p className="text-sm text-ink-3">{t('common.actions.loading')}</p>
      ) : items.length === 0 ? (
        <p className="text-sm text-ink-3">{t('admin.logs.empty')}</p>
      ) : (
        <div className="scroll-thin -mx-1 overflow-x-auto">
          <table className="w-full min-w-[40rem] text-left text-sm">
            <thead className="text-xs text-ink-3">
              <tr>
                <th className="px-1 py-1.5 font-medium">{t('admin.logs.heads.time')}</th>
                <th className="px-1 py-1.5 font-medium">{t('admin.logs.heads.user')}</th>
                <th className="px-1 py-1.5 font-medium">{t('admin.logs.heads.event')}</th>
                <th className="px-1 py-1.5 text-right font-medium">{t('admin.logs.heads.status')}</th>
                <th className="px-1 py-1.5 text-right font-medium">{t('admin.logs.heads.duration')}</th>
              </tr>
            </thead>
            <tbody className="tabular-nums">
              {items.map((e) => (
                <tr key={e.id} className="border-t border-line/60">
                  <td className="whitespace-nowrap px-1 py-1.5 text-ink-3">{timeFmt.format(new Date(e.at))}</td>
                  <td className="max-w-32 truncate px-1 py-1.5 text-ink-2">{e.userName ?? '—'}</td>
                  <td className="px-1 py-1.5">
                    <span className={cn('mr-2 rounded-md px-1.5 py-0.5 text-[11px] font-medium', e.category === 'user_action' ? 'bg-accent/15 text-accent' : 'bg-track text-ink-3')}>
                      {e.category === 'user_action' ? t('admin.logs.action') : e.method === 'OUT' ? t('admin.logs.outgoing') : t('admin.logs.request')}
                    </span>
                    <span className="break-all font-mono text-xs text-ink">{describe(e)}</span>
                    {e.category === 'request' && e.message && <span className="ml-2 text-xs text-ink-3">{e.message}</span>}
                  </td>
                  <td className={cn('px-1 py-1.5 text-right font-mono text-xs', e.level === 'error' ? 'text-danger' : e.level === 'warn' ? 'text-warn' : 'text-ink-2')}>{e.status || '—'}</td>
                  <td className="px-1 py-1.5 text-right text-xs text-ink-3">{e.durationMs ? `${fmtNumber(e.durationMs)} ms` : '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <div className="mt-4 flex items-center justify-between gap-3 text-xs text-ink-3">
        <span>{dropped > 0 ? t('admin.logs.dropped', { count: fmtNumber(dropped) }) : t('admin.logs.shown', { count: fmtNumber(items.length) })}</span>
        {logs.hasNextPage && (
          <button type="button" className="btn btn-ghost" onClick={() => void logs.fetchNextPage()} disabled={logs.isFetchingNextPage}>
            {logs.isFetchingNextPage && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {t('admin.logs.more')}
          </button>
        )}
      </div>
    </Card>
  )
}
