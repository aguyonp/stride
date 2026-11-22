// 4.3.0: usage of the public MCP route (24 h per token, in-memory) and the admin-tunable
// quota (POST /mcp/{token}, persisted server-side).
import { Activity, Loader2, Webhook } from 'lucide-react'
import { useState } from 'react'
import { useAdminControl, useAdminMcpUsage } from '../../api/queriesAdmin'
import type { McpQuota, McpTokenUsage } from '../../api/types'
import { Badge } from '../../components/ui/Badge'
import { Card } from '../../components/ui/Card'
import { Field } from '../../components/ui/Field'
import { EmptyState, InlineError } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { fmtNumber, fmtWhen } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { t } from '../../i18n'

const LIMIT_MAX = 100_000
const WINDOW_MAX = 1_440

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
            <dl className="grid gap-2 sm:grid-cols-3">
              {(
                [
                  [t('admin.mcp.req24h'), String(data.requests24h)],
                  [t('admin.mcp.denied24h'), String(data.denied24h)],
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
    </div>
  )
}
