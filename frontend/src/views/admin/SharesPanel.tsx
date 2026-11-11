import { EyeOff, Link2, Loader2, Unlink } from 'lucide-react'
import { useState } from 'react'
import { useAdminControl, useAdminShares } from '../../api/queries'
import type { AdminShareLink } from '../../api/types'
import { Badge } from '../../components/ui/Badge'
import { Card } from '../../components/ui/Card'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { EmptyState } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { fmtDateTime, fmtWhen } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { FilterChips } from './adminUtils'
import { lazyRecord, t } from '../../i18n'

const STATE_LEVEL = { active: 'ok', expired: 'info', revoked: 'danger' } as const
const STATE = lazyRecord(['active', 'expired', 'revoked'] as const, (k) => ({ label: t(`admin.shares.state.${k}`), level: STATE_LEVEL[k] }))

const scope = (l: AdminShareLink) => t('admin.shares.scope', { weeks: l.periodWeeks, count: l.sections.length })
const expiry = (l: AdminShareLink) => (l.revokedAt ? t('admin.shares.revokedAt', { when: fmtWhen(l.revokedAt) }) : l.expiresAt ? fmtDateTime(l.expiresAt) : t('admin.shares.never'))

export function SharesPanel() {
  const [filter, setFilter] = useState<'active' | 'all'>('active')
  const shares = useAdminShares(filter)
  const ctl = useAdminControl()
  const { toast } = useToast()
  const [revoke, setRevoke] = useState<AdminShareLink | null>(null)
  const items = shares.data ?? []

  const views = (l: AdminShareLink) => (
    <>
      <span className="num text-ink">{l.accessCount}</span>
      {l.lastAccessedAt && <span className="num block text-[11px] text-ink-3">{fmtWhen(l.lastAccessedAt)}</span>}
    </>
  )
  const revokeBtn = (l: AdminShareLink) =>
    l.state === 'active' && (
      <button type="button" className="btn btn-danger min-h-9 px-3 text-xs" onClick={() => setRevoke(l)} disabled={ctl.revokeShare.isPending && ctl.revokeShare.variables === l.id}>
        {ctl.revokeShare.isPending && ctl.revokeShare.variables === l.id ? <Loader2 className="size-3.5 animate-spin" aria-hidden="true" /> : <Unlink className="size-3.5" aria-hidden="true" />}
        {t('admin.shares.revoke')}
        <span className="sr-only">{t('admin.shares.revokeSr', { label: l.label })}</span>
      </button>
    )
  const anon = (l: AdminShareLink) =>
    l.anonymize && (
      <span className="chip h-5 border border-line bg-track/60 px-1.5 text-[10px] font-medium text-ink-2">
        <EyeOff className="size-3" aria-hidden="true" />
        {t('admin.shares.anonymous')}
      </span>
    )

  return (
    <Card title={t('admin.shares.title')} icon={<Link2 className="size-4" />} subtitle={t('admin.shares.subtitle')}>
      <div className="mb-4">
        <FilterChips<'active' | 'all'>
          ariaLabel={t('admin.shares.filter')}
          value={filter}
          onChange={setFilter}
          options={[
            { value: 'active', label: t('admin.shares.active') },
            { value: 'all', label: t('admin.shares.all') },
          ]}
        />
      </div>
      {shares.isError ? (
        <ErrorView error={shares.error} onRetry={() => void shares.refetch()} title={t('admin.shares.unavailable')} />
      ) : !shares.data ? (
        <Skeleton className="h-28" />
      ) : items.length === 0 ? (
        <EmptyState icon={<Link2 />} title={filter === 'active' ? t('admin.shares.noActive') : t('admin.shares.none')}>
            {t('admin.shares.emptyHint')}
        </EmptyState>
      ) : (
        <>
          <div className="scroll-thin hidden overflow-x-auto md:block">
            <table className="w-full min-w-[720px] border-separate border-spacing-0 text-sm">
              <caption className="sr-only">{t('admin.shares.caption')}</caption>
              <thead>
                <tr className="text-left text-xs text-ink-3">
                  {[...(['link', 'owner', 'scope', 'expiry', 'views', 'state'] as const).map((k) => t(`admin.shares.heads.${k}`)), ''].map((h, i) => (
                    <th key={i} scope="col" className="border-b border-line px-2 py-2 font-medium first:pl-1">
                      {h || <span className="sr-only">{t('admin.shares.actions')}</span>}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {items.map((l) => (
                  <tr key={l.id} className="align-top hover:bg-track/50">
                    <th scope="row" className="border-b border-line px-2 py-2 pl-1 text-left font-normal">
                      <span className="block font-medium text-ink">{l.label}</span>
                      <span className="block font-mono text-[11px] text-ink-3">{l.tokenPrefix}…</span>
                    </th>
                    <td className="border-b border-line px-2 py-2 whitespace-nowrap text-ink-2">{l.userName}</td>
                    <td className="border-b border-line px-2 py-2 text-ink-2">
                      <span className="num block whitespace-nowrap">{scope(l)}</span>
                      {anon(l)}
                    </td>
                    <td className="num border-b border-line px-2 py-2 whitespace-nowrap text-ink-2">{expiry(l)}</td>
                    <td className="border-b border-line px-2 py-2">{views(l)}</td>
                    <td className="border-b border-line px-2 py-2">
                      <Badge level={STATE[l.state].level}>{STATE[l.state].label}</Badge>
                    </td>
                    <td className="border-b border-line px-2 py-2 text-right">{revokeBtn(l)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <ul className="space-y-2 md:hidden">
            {items.map((l) => (
              <li key={l.id} className="glass-inset p-3 text-sm">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <span className="block truncate font-medium text-ink">{l.label}</span>
                    <span className="block text-xs text-ink-3">
                      {l.userName} · <span className="font-mono">{l.tokenPrefix}…</span>
                    </span>
                  </div>
                  <Badge level={STATE[l.state].level}>{STATE[l.state].label}</Badge>
                </div>
                <p className="num mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-ink-3">
                  {t('admin.shares.mobileLine', { scope: scope(l), expiry: expiry(l), views: t('admin.shares.views', { count: l.accessCount }) })} {anon(l)}
                </p>
                {l.state === 'active' && <div className="mt-2">{revokeBtn(l)}</div>}
              </li>
            ))}
          </ul>
        </>
      )}

      <ConfirmDialog
        open={revoke !== null}
        onClose={() => setRevoke(null)}
        title={t('admin.shares.revokeTitle')}
        message={
          <p>
            {t('admin.shares.revokeBody', { label: revoke?.label, user: revoke?.userName })}
          </p>
        }
        confirmLabel={t('admin.shares.revoke')}
        danger
        busy={ctl.revokeShare.isPending}
        onConfirm={() =>
          revoke &&
          ctl.revokeShare.mutate(revoke.id, {
            onSuccess: () => {
              toast({ title: t('admin.shares.revoked'), description: `${revoke.label} · ${revoke.userName}` })
              setRevoke(null)
            },
            onError: (e) => toast({ tone: 'error', title: t('admin.shares.revokeFailed'), description: e.message }),
          })
        }
      />
    </Card>
  )
}
