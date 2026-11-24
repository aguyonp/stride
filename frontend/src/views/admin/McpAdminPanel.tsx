// 4.3.0: usage of the public MCP route (24 h per token, in-memory) and the admin-tunable
// quota (POST /mcp/{token}, persisted server-side). 4.5.0 (issue #135): the call journal —
// the last public-route calls in a ring of 300, with client-side filters (tool, failed/slow,
// free text) and the "errors / timeouts · 24 h" KPI.
import { Activity, Loader2, ScrollText, Webhook } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useAdminControl, useAdminMcpJournal, useAdminMcpUsage } from '../../api/queriesAdmin'
import type { McpJournalCall, McpQuota, McpTokenUsage } from '../../api/types'
import { Badge } from '../../components/ui/Badge'
import { Card } from '../../components/ui/Card'
import { Field } from '../../components/ui/Field'
import { EmptyState, InlineError } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { fmtNumber, fmtWhen } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { getLang, LOCALE, t } from '../../i18n'

const LIMIT_MAX = 100_000
const WINDOW_MAX = 1_440
const SLOW_MS = 2_000

const okInt = (v: number, max: number) => Number.isInteger(v) && v >= 1 && v <= max
const loadColor = (pct: number) => (pct >= 90 ? 'var(--danger)' : pct >= 70 ? 'var(--warn)' : 'var(--accent)')

/** Live load of the token window, with the same accent / warn / danger steps as QuotaGauge. */
function WindowCell({ tu, limit }: { tu: McpTokenUsage; limit: number }) {
  const pct = limit > 0 ? Math.min(100, (tu.used / limit) * 100) : 0
  return (
    <div className="flex min-w-[7.5rem] items-center gap-2">
      <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-track">
        <div className="h-full rounded-full transition-[width] duration-500" style={{ width: `${pct}%`, background: loadColor(pct) }} />
      </div>
      <span className="num text-xs whitespace-nowrap">
        {tu.used}/{limit}
      </span>
    </div>
  )
}

function QuotaForm({ quota }: { quota: McpQuota }) {
  const ctl = useAdminControl()
  const { toast } = useToast()
  const [f, setF] = useState<McpQuota>(quota)
  const set = (k: keyof McpQuota, v: number) => setF((x) => ({ ...x, [k]: v }))
  const dirty = f.tokenLimit !== quota.tokenLimit || f.tokenWindow !== quota.tokenWindow || f.ipLimit !== quota.ipLimit || f.ipWindow !== quota.ipWindow
  const range = (max: number) => t('admin.mcp.rangeError', { max: fmtNumber(max) })
  const num = (k: keyof McpQuota, label: string, max: number, hint?: string) => (
    <Field label={label} hint={hint} error={okInt(f[k], max) ? undefined : range(max)}>
      {(p) => <input {...p} className="field num" type="number" min={1} max={max} step={1} inputMode="numeric" value={Number.isFinite(f[k]) ? f[k] : ''} onChange={(e) => set(k, Number(e.target.value))} />}
    </Field>
  )
  const save = () =>
    ctl.updateMcpQuota.mutate(f, {
      onSuccess: (q) => toast({ title: t('admin.mcp.saved'), description: t('admin.mcp.savedBody', { limit: q.tokenLimit, window: q.tokenWindow }) }),
      onError: (e) => toast({ tone: 'error', title: t('admin.mcp.saveFailed'), description: e.message }),
    })
  return (
    <form
      className="space-y-3.5"
      onSubmit={(e) => {
        e.preventDefault()
        save()
      }}
      autoComplete="off"
    >
      <div className="grid gap-3 sm:grid-cols-2">
        {num('tokenLimit', t('admin.mcp.tokenLimit'), LIMIT_MAX)}
        {num('tokenWindow', t('admin.mcp.tokenWindow'), WINDOW_MAX)}
        {num('ipLimit', t('admin.mcp.ipLimit'), LIMIT_MAX)}
        {num('ipWindow', t('admin.mcp.ipWindow'), WINDOW_MAX)}
      </div>
      <p className="text-xs text-ink-3">{t('admin.mcp.hint')}</p>
      <div className="flex justify-end">
        <button type="submit" className="btn btn-primary" disabled={ctl.updateMcpQuota.isPending || !dirty || !okInt(f.tokenLimit, LIMIT_MAX) || !okInt(f.tokenWindow, WINDOW_MAX) || !okInt(f.ipLimit, LIMIT_MAX) || !okInt(f.ipWindow, WINDOW_MAX)}>
          {ctl.updateMcpQuota.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
          {t('common.actions.save')}
        </button>
      </div>
    </form>
  )
}

const callTimeFmt = { format: (d: Date) => new Intl.DateTimeFormat(LOCALE[getLang()], { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(d) }
const isFailed = (c: McpJournalCall) => c.status >= 400 || c.rpcErrorCode !== 0 || c.clientCanceled

/** 4.5.0 (issue #135): the in-memory journal of the last public-route calls. All the
 * filtering is client-side: the ring never holds more than 300 rows. */
function McpJournalCard() {
  const q = useAdminMcpJournal()
  const [tool, setTool] = useState('all')
  const [kind, setKind] = useState<'all' | 'failed' | 'slow'>('all')
  const [search, setSearch] = useState('')
  const entries = q.data?.entries ?? []
  const tools = useMemo(() => [...new Set(entries.map((c) => c.tool).filter((x) => x !== ''))].sort(), [entries])
  const filtered = entries.filter((c) => {
    if (tool !== 'all' && c.tool !== tool) return false
    if (kind === 'failed' && !isFailed(c)) return false
    if (kind === 'slow' && c.durationMs <= SLOW_MS) return false
    if (search.trim() !== '') {
      const hay = `${c.userName} ${c.tokenPrefix} ${c.method} ${c.tool} ${c.argsSummary} ${c.rpcError} ${c.status}`.toLowerCase()
      if (!hay.includes(search.trim().toLowerCase())) return false
    }
    return true
  })
  const statusCell = (c: McpJournalCall) => (
    <td className={`num border-b border-line px-2 py-2 whitespace-nowrap ${c.status >= 400 || c.rpcErrorCode !== 0 ? 'text-danger' : c.clientCanceled ? 'text-warn' : 'text-ink-2'}`}>
      {c.status}
      {c.rpcErrorCode !== 0 && <span className="block max-w-40 truncate text-[11px] font-normal text-danger" title={c.rpcError}>{c.rpcErrorCode} · {c.rpcError}</span>}
      {c.clientCanceled && <span className="block text-[11px] font-normal text-warn">{t('admin.mcp.journalCancelled')}</span>}
    </td>
  )
  return (
    <Card className="xl:col-span-5" title={t('admin.mcp.journalTitle')} icon={<ScrollText className="size-4" />} subtitle={t('admin.mcp.journalSubtitle', { total: entries.length })}>
      {q.isError ? (
        <ErrorView error={q.error} onRetry={() => void q.refetch()} title={t('admin.mcp.journalTitle')} />
      ) : !q.data ? (
        <Skeleton className="h-64" />
      ) : (
        <div className="space-y-3.5">
          <div className="flex flex-wrap items-center gap-3">
            <select aria-label={t('admin.mcp.journalFilterTool')} className="field h-10 w-auto" value={tool} onChange={(e) => setTool(e.target.value)}>
              <option value="all">{t('admin.mcp.journalAllTools')}</option>
              {tools.map((x) => (
                <option key={x} value={x}>
                  {x}
                </option>
              ))}
            </select>
            <select aria-label={t('admin.mcp.journalFilterKind')} className="field h-10 w-auto" value={kind} onChange={(e) => setKind(e.target.value as 'all' | 'failed' | 'slow')}>
              <option value="all">{t('admin.mcp.journalKindAll')}</option>
              <option value="failed">{t('admin.mcp.journalKindFailed')}</option>
              <option value="slow">{t('admin.mcp.journalKindSlow')}</option>
            </select>
            <input aria-label={t('admin.mcp.journalSearch')} className="field h-10 w-full min-w-52 flex-1 sm:max-w-72" placeholder={t('admin.mcp.journalSearchPlaceholder')} value={search} onChange={(e) => setSearch(e.target.value)} />
            <span className="num ml-auto text-xs text-ink-3">{t('admin.mcp.journalShown', { shown: filtered.length, total: entries.length })}</span>
          </div>
          {filtered.length === 0 ? (
            <EmptyState icon={<ScrollText />} title={entries.length === 0 ? t('admin.mcp.journalEmpty') : t('admin.mcp.journalNoMatch')}>
              {entries.length === 0 ? t('admin.mcp.journalEmptyHint') : t('admin.mcp.journalNoMatchHint')}
            </EmptyState>
          ) : (
            <>
              <div className="scroll-thin hidden overflow-x-auto md:block">
                <table className="w-full min-w-[820px] border-separate border-spacing-0 text-sm">
                  <caption className="sr-only">{t('admin.mcp.journalTitle')}</caption>
                  <thead>
                    <tr className="text-left text-xs text-ink-3">
                      {(['time', 'user', 'call', 'args', 'status', 'duration', 'inFlight'] as const).map((k) => (
                        <th key={k} scope="col" className="border-b border-line px-2 py-2 font-medium first:pl-1">
                          {t(`admin.mcp.journalHeads.${k}`)}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {filtered.map((c, i) => (
                      <tr key={`${c.at}-${i}`} className="align-top hover:bg-track/50">
                        <td className="num border-b border-line px-2 py-2 pl-1 whitespace-nowrap text-ink-3">{callTimeFmt.format(new Date(c.at))}</td>
                        <td className="border-b border-line px-2 py-2 whitespace-nowrap">
                          <span className="text-ink-2">{c.userName || '—'}</span>
                          <span className="block font-mono text-[11px] text-ink-3">{c.tokenPrefix}…</span>
                        </td>
                        <td className="border-b border-line px-2 py-2 whitespace-nowrap font-mono text-xs text-ink">{c.method ? (c.tool ? `${c.method} ${c.tool}` : c.method) : '—'}</td>
                        <td className="max-w-56 border-b border-line px-2 py-2">
                          <span className="block truncate font-mono text-[11px] text-ink-3" title={c.argsSummary}>
                            {c.argsSummary || '—'}
                          </span>
                        </td>
                        {statusCell(c)}
                        <td className={`num border-b border-line px-2 py-2 whitespace-nowrap ${c.durationMs > SLOW_MS ? 'text-warn' : 'text-ink-2'}`}>{fmtNumber(c.durationMs)} ms</td>
                        <td className="num border-b border-line px-2 py-2 whitespace-nowrap text-ink-3">{c.inFlight}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <ul className="space-y-2 md:hidden">
                {filtered.map((c, i) => (
                  <li key={`${c.at}-${i}`} className="glass-inset p-3 text-sm">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <span className="block font-mono text-xs text-ink">{c.method ? (c.tool ? `${c.method} ${c.tool}` : c.method) : c.status}</span>
                        <span className="block text-xs text-ink-3">
                          {c.userName || c.tokenPrefix}… · {callTimeFmt.format(new Date(c.at))}
                        </span>
                      </div>
                      <Badge level={isFailed(c) ? 'danger' : c.clientCanceled ? 'warn' : 'ok'}>
                        {c.status}
                        {c.clientCanceled ? ` · ${t('admin.mcp.journalCancelled')}` : ''}
                      </Badge>
                    </div>
                    {c.argsSummary && <p className="mt-1.5 truncate font-mono text-[11px] text-ink-3" title={c.argsSummary}>{c.argsSummary}</p>}
                    {c.rpcError && <p className="mt-1 text-xs text-danger">{c.rpcErrorCode} · {c.rpcError}</p>}
                    <p className="num mt-1.5 text-xs text-ink-3">
                      {fmtNumber(c.durationMs)} ms · {t('admin.mcp.journalHeads.inFlight')}: {c.inFlight}
                    </p>
                  </li>
                ))}
              </ul>
            </>
          )}
        </div>
      )}
    </Card>
  )
}

export function McpAdminPanel() {
  const q = useAdminMcpUsage()
  const data = q.data
  const tokens = data?.tokens ?? []
  const last = (tu: McpTokenUsage) => (tu.lastUsedAt ? fmtWhen(tu.lastUsedAt) : t('admin.mcp.never'))
  const row = (tu: McpTokenUsage) => (
    <>
      <td className="border-b border-line px-2 py-2 whitespace-nowrap text-ink-2">{tu.displayName}</td>
      <td className="border-b border-line px-2 py-2 whitespace-nowrap">
        <Badge level={tu.enabled ? 'ok' : 'info'}>{tu.enabled ? t('admin.mcp.active') : t('admin.mcp.disabled')}</Badge>
      </td>
      <td className="num border-b border-line px-2 py-2 whitespace-nowrap text-ink-2">{last(tu)}</td>
      <td className="border-b border-line px-2 py-2">{data && <WindowCell tu={tu} limit={data.quota.tokenLimit} />}</td>
      <td className="num border-b border-line px-2 py-2 text-right whitespace-nowrap text-ink-2">{tu.requests24h}</td>
      <td className="num border-b border-line px-2 py-2 text-right whitespace-nowrap text-ink-2">{tu.denied24h}</td>
    </>
  )

  return (
    <div className="grid gap-5 xl:grid-cols-5">
      <Card className="xl:col-span-3" title={t('admin.mcp.usageTitle')} icon={<Activity className="size-4" />} subtitle={t('admin.mcp.usageSubtitle')}>
        {q.isError ? (
          <ErrorView error={q.error} onRetry={() => void q.refetch()} title={t('admin.mcp.usageTitle')} />
        ) : !data ? (
          <Skeleton className="h-64" />
        ) : (
          <div className="space-y-4">
            <dl className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
              {(
                [
                  [t('admin.mcp.req24h'), String(data.requests24h)],
                  [t('admin.mcp.denied24h'), String(data.denied24h)],
                  [t('admin.mcp.errors24h'), String(data.errors24h)],
                  [t('admin.mcp.updatedAt'), fmtWhen(data.updatedAt)],
                ] as const
              ).map(([k, v]) => (
                <div key={k} className="glass-inset flex min-w-0 items-baseline justify-between gap-3 px-3 py-2 sm:block">
                  <dt className="truncate text-[11px] text-ink-3">{k}</dt>
                  <dd className="num truncate text-sm font-semibold text-ink">{v}</dd>
                </div>
              ))}
            </dl>
            {tokens.length === 0 ? (
              <EmptyState icon={<Webhook />} title={t('admin.mcp.noTokens')}>
                {t('admin.mcp.noTokensHint')}
              </EmptyState>
            ) : (
              <>
                <div className="scroll-thin hidden overflow-x-auto md:block">
                  <table className="w-full min-w-[720px] border-separate border-spacing-0 text-sm">
                    <caption className="sr-only">{t('admin.mcp.caption')}</caption>
                    <thead>
                      <tr className="text-left text-xs text-ink-3">
                        {(['token', 'user', 'state', 'lastUsed', 'window', 'req24h', 'denied'] as const).map((k) => (
                          <th key={k} scope="col" className="border-b border-line px-2 py-2 font-medium first:pl-1 last:text-right">
                            {t(`admin.mcp.heads.${k}`)}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {tokens.map((tu) => (
                        <tr key={tu.prefix} className="align-middle hover:bg-track/50">
                          <th scope="row" className="border-b border-line px-2 py-2 pl-1 text-left font-normal">
                            <span className="block font-mono text-xs text-ink">{tu.prefix}…</span>
                          </th>
                          {row(tu)}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <ul className="space-y-2 md:hidden">
                  {tokens.map((tu) => (
                    <li key={tu.prefix} className="glass-inset p-3 text-sm">
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0">
                          <span className="block font-mono text-xs text-ink">{tu.prefix}…</span>
                          <span className="block text-xs text-ink-3">
                            {tu.displayName} · {last(tu)}
                          </span>
                        </div>
                        <Badge level={tu.enabled ? 'ok' : 'info'}>{tu.enabled ? t('admin.mcp.active') : t('admin.mcp.disabled')}</Badge>
                      </div>
                      <p className="num mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-ink-3">
                        {t('admin.mcp.req24h')}: {tu.requests24h} · {t('admin.mcp.denied24h')}: {tu.denied24h}
                      </p>
                      <div className="mt-2">{data && <WindowCell tu={tu} limit={data.quota.tokenLimit} />}</div>
                    </li>
                  ))}
                </ul>
              </>
            )}
          </div>
        )}
      </Card>

      <Card className="xl:col-span-2" title={t('admin.mcp.quotaTitle')} icon={<Webhook className="size-4" />} subtitle={t('admin.mcp.quotaSubtitle')}>
        {q.isError ? <InlineError>{q.error.message}</InlineError> : data ? <QuotaForm key={JSON.stringify(data.quota)} quota={data.quota} /> : <Skeleton className="h-72" />}
      </Card>

      <McpJournalCard />
    </div>
  )
}
