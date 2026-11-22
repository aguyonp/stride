import { ChevronDown, Copy, KeyRound, ListChecks, Loader2, RefreshCw, Trash2, Webhook } from 'lucide-react'
import { useState } from 'react'
import { useMcpMutations, useMcpStatus } from '../../api/queries'
import type { McpToken } from '../../api/types'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { InlineError } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { fmtDay } from '../../components/profile/download'
import { copyText } from '../../lib/clipboard'
import { cn } from '../../lib/cn'
import { fmtWhen } from '../../lib/format'
import { t, tDyn } from '../../i18n'
import { ErrorView } from '../StateViews'
import { Section } from './Section'

const READ_TOOLS = ['planning', 'day', 'activities', 'activity', 'health', 'cycles', 'profile'] as const
const WRITE_TOOLS = ['plan_create', 'plan_update', 'plan_delete', 'event_create', 'event_update', 'event_delete'] as const

/** The 13 MCP tools (7 read / 6 write) as a compact inline reference list, expanded in place. */
function ToolsList({ hidden }: { hidden: boolean }) {
  const group = (titleKey: 'settings.mcp.toolsRead' | 'settings.mcp.toolsWrite', prefix: 'read' | 'write', names: readonly string[]) => (
    <div>
      <p className="mb-1.5 text-xs font-semibold tracking-wide text-ink-3 uppercase">{t(titleKey, { count: names.length })}</p>
      <ul className="grid gap-x-6 gap-y-1 sm:grid-cols-2">
        {names.map((n) => (
          <li key={n} className="flex items-baseline gap-2 text-xs">
            <code className="num w-28 flex-none text-ink">{n}</code>
            <span className="min-w-0 text-ink-2">{tDyn(`settings.mcp.tools.${prefix}.${n}`)}</span>
          </li>
        ))}
      </ul>
    </div>
  )
  return (
    <div id="mcp-tools" hidden={hidden} className="space-y-3 rounded-xl border border-line bg-track/40 p-3">
      <p className="text-sm font-semibold">{t('settings.mcp.toolsTitle')}</p>
      {group('settings.mcp.toolsRead', 'read', READ_TOOLS)}
      {group('settings.mcp.toolsWrite', 'write', WRITE_TOOLS)}
    </div>
  )
}

/** Settings › Serveur MCP: per-user MCP endpoint. The token is shown once, at generation time. */
export function McpSection() {
  const { toast } = useToast()
  const st = useMcpStatus()
  const m = useMcpMutations()
  const [fresh, setFresh] = useState<McpToken | null>(null)
  const [confirm, setConfirm] = useState<null | 'regenerate' | 'revoke'>(null)
  const [showTools, setShowTools] = useState(false)

  const endpoint = (token: string) => `${window.location.origin}/mcp/${token}`
  const copy = (text: string, what: string) => {
    void copyText(text).then((ok) =>
      ok ? toast({ title: t('settings.mcp.copied'), description: what }) : toast({ tone: 'error', title: t('settings.mcp.copyFailed') }),
    )
  }
  const generate = (replace: boolean) => {
    m.generateToken.mutate(undefined, {
      onSuccess: (tk) => {
        setFresh(tk)
        setConfirm(null)
        toast({ title: replace ? t('settings.mcp.regenerated') : t('settings.mcp.tokenGenerated'), tone: 'success' })
      },
      onError: (e) => {
        setConfirm(null)
        toast({ tone: 'error', title: t('settings.mcp.error'), description: e.message })
      },
    })
  }

  const s = st.data

  return (
    <Section
      id="mcp"
      title={t('settings.mcp.title')}
      icon={<Webhook />}
      description={t('settings.mcp.description')}
      actions={
        <>
          {s && !s.hasToken && !fresh ? (
            <button type="button" className="btn btn-primary" disabled={m.generateToken.isPending} onClick={() => generate(false)}>
              {m.generateToken.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <KeyRound className="size-4" aria-hidden="true" />}
              {t('settings.mcp.generate')}
            </button>
          ) : null}
          <button type="button" className="btn btn-ghost" aria-expanded={showTools} aria-controls="mcp-tools" onClick={() => setShowTools((v) => !v)}>
            <ListChecks className="size-4" aria-hidden="true" />
            {t('settings.mcp.toolsButton')}
            <ChevronDown className={cn('size-4 text-ink-3 transition-transform duration-200', showTools && 'rotate-180')} aria-hidden="true" />
          </button>
        </>
      }
    >
      {st.isPending ? (
        <div className="space-y-2">
          <Skeleton className="h-12" />
          <Skeleton className="h-16" />
        </div>
      ) : st.isError ? (
        <ErrorView error={st.error} onRetry={() => void st.refetch()} title={t('settings.mcp.unavailable')} />
      ) : s ? (
        <div className="space-y-4">
          <ToolsList hidden={!showTools} />
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <label className="flex min-w-0 items-center gap-3">
              <button
                type="button"
                role="switch"
                aria-checked={s.enabled}
                aria-label={t('settings.mcp.enableLabel')}
                disabled={m.setEnabled.isPending}
                onClick={() =>
                  m.setEnabled.mutate(!s.enabled, {
                    onError: (e) => toast({ tone: 'error', title: t('settings.mcp.error'), description: e.message }),
                  })
                }
                className={cn('relative h-6 w-11 flex-none rounded-full transition-colors', s.enabled ? 'bg-accent' : 'bg-line')}
              >
                <span className={cn('absolute top-0.5 left-0.5 size-5 rounded-full bg-card shadow transition-transform', s.enabled && 'translate-x-5')} />
              </button>
              <span className="text-sm">
                <span className={s.enabled ? 'font-semibold text-ink' : 'text-ink-2'}>{s.enabled ? t('settings.mcp.on') : t('settings.mcp.off')}</span>
                {m.setEnabled.isPending && <Loader2 className="ml-2 inline size-3.5 animate-spin text-ink-3" aria-hidden="true" />}
              </span>
            </label>
            {s.hasToken && (
              <p className="num text-xs text-ink-3">
                <KeyRound className="mr-1 inline size-3.5" aria-hidden="true" />
                {s.tokenPrefix}
                {s.createdAt && <> · {t('settings.mcp.created', { date: fmtDay(s.createdAt) })}</>}
                {s.lastUsedAt ? <> · {t('settings.mcp.used', { when: fmtWhen(s.lastUsedAt) })}</> : <> · {t('settings.mcp.neverUsed')}</>}
              </p>
            )}
          </div>
          {m.generateToken.isError && <InlineError>{m.generateToken.error.message}</InlineError>}
          {m.revokeToken.isError && <InlineError>{m.revokeToken.error.message}</InlineError>}
          {fresh && (
            <div className="space-y-2 rounded-xl border border-warn/40 bg-warn/10 p-3">
              <p className="text-sm font-semibold text-warn">{t('settings.mcp.tokenOnce')}</p>
              <div className="flex flex-wrap items-center gap-2">
                <code className="num min-w-0 flex-1 break-all rounded-lg bg-track/60 px-2 py-1.5 text-xs">{fresh.token}</code>
                <button type="button" className="icon-btn size-9" aria-label={t('settings.mcp.copyToken')} onClick={() => copy(fresh.token, t('settings.mcp.token'))}>
                  <Copy className="size-4" />
                </button>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <code className="num min-w-0 flex-1 break-all rounded-lg bg-track/60 px-2 py-1.5 text-xs">{endpoint(fresh.token)}</code>
                <button type="button" className="icon-btn size-9" aria-label={t('settings.mcp.copyEndpoint')} onClick={() => copy(endpoint(fresh.token), t('settings.mcp.endpoint'))}>
                  <Copy className="size-4" />
                </button>
              </div>
              <div className="space-y-1.5 border-t border-warn/30 pt-2">
                <p className="text-xs font-semibold text-ink-2">{t('settings.mcp.pasteTitle')}</p>
                <div className="flex items-start gap-2">
                  <code className="num min-w-0 flex-1 break-all rounded-lg bg-track/60 px-2 py-1.5 text-xs">
                    {t('settings.mcp.pasteMessage', { endpoint: endpoint(fresh.token) })}
                  </code>
                  <button
                    type="button"
                    className="icon-btn size-9"
                    aria-label={t('settings.mcp.copyPaste')}
                    onClick={() => copy(t('settings.mcp.pasteMessage', { endpoint: endpoint(fresh.token) }), t('settings.mcp.paste'))}
                  >
                    <Copy className="size-4" />
                  </button>
                </div>
              </div>
              <p className="text-xs text-ink-3">{t('settings.mcp.howTo')}</p>
            </div>
          )}
          {!fresh && s.hasToken && (
            <div className="flex flex-wrap gap-2">
              <button type="button" className="btn" disabled={m.generateToken.isPending} onClick={() => setConfirm('regenerate')}>
                <RefreshCw className="size-4" aria-hidden="true" /> {t('settings.mcp.regenerate')}
              </button>
              <button type="button" className="btn btn-danger" onClick={() => setConfirm('revoke')}>
                <Trash2 className="size-4" aria-hidden="true" /> {t('settings.mcp.revoke')}
              </button>
            </div>
          )}
          <p className="text-xs text-ink-3">{t('settings.mcp.hint')}</p>
        </div>
      ) : null}
      <ConfirmDialog
        open={confirm !== null}
        onClose={() => setConfirm(null)}
        title={confirm === 'revoke' ? t('settings.mcp.revokeTitle') : t('settings.mcp.regenerateTitle')}
        message={confirm === 'revoke' ? t('settings.mcp.revokeBody') : t('settings.mcp.regenerateBody')}
        confirmLabel={confirm === 'revoke' ? t('settings.mcp.revoke') : t('settings.mcp.regenerate')}
        danger
        busy={m.generateToken.isPending || m.revokeToken.isPending}
        onConfirm={() => {
          if (confirm === 'revoke') {
            m.revokeToken.mutate(undefined, {
              onSuccess: () => {
                setConfirm(null)
                toast({ title: t('settings.mcp.revoked') })
              },
              onError: (e) => {
                setConfirm(null)
                toast({ tone: 'error', title: t('settings.mcp.error'), description: e.message })
              },
            })
          } else {
            generate(true)
          }
        }}
      />
    </Section>
  )
}
