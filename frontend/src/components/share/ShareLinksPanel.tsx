import { Check, Copy, Link2Off, Loader2, Pencil, Plus, Share2 } from 'lucide-react'
import { useState } from 'react'
import { useShareLinks, useShareMutations } from '../../api/queries'
import type { ShareLink } from '../../api/types'
import { copyText } from '../../lib/clipboard'
import { cn } from '../../lib/cn'
import { fmtDateShort, fmtNumber } from '../../lib/format'
import { Card } from '../ui/Card'
import { InlineError } from '../ui/Misc'
import { Skeleton } from '../ui/Skeleton'
import { useToast } from '../ui/Toast'
import { ShareLinkDialog } from './ShareLinkDialog'
import { linkStatus, permissionsSummary, scopeSummary } from './shareScope'
import { t } from '../../i18n'

const fullUrl = (path: string) => (typeof window === 'undefined' ? path : new URL(path, window.location.origin).toString())

/** Physio & coach share links: create, edit scope, copy, revoke. */
export function ShareLinksPanel({ className }: { className?: string }) {
  const links = useShareLinks()
  const m = useShareMutations()
  const { toast } = useToast()
  const [dialog, setDialog] = useState<{ link: ShareLink | null } | null>(null)
  const [copiedId, setCopiedId] = useState<number | null>(null)
  const nowMs = Date.now()

  const copy = async (link: ShareLink) => {
    const ok = await copyText(fullUrl(link.url))
    if (!ok) return toast({ tone: 'error', title: t('share.panel.copyFailed'), description: fullUrl(link.url) })
    setCopiedId(link.id)
    window.setTimeout(() => setCopiedId((c) => (c === link.id ? null : c)), 2000)
    toast({ title: t('share.panel.copied'), description: link.label })
  }

  const revoke = (link: ShareLink) =>
    m.revoke.mutate(link.id, {
      onSuccess: () => toast({ title: t('share.panel.revokedToast', { label: link.label }) }),
      onError: (e) => toast({ tone: 'error', title: t('share.panel.revokeFailed'), description: e.message }),
    })

  const items = (links.data ?? []).filter((l) => !l.revoked)

  return (
    <Card
      id="partage"
      className={className}
      title={t('share.panel.title')}
      icon={<Share2 className="size-4" />}
      action={
        <button type="button" className="btn btn-ghost h-9 min-h-9 px-3 text-xs" onClick={() => setDialog({ link: null })}>
          <Plus className="size-4" aria-hidden="true" /> Nouveau lien
        </button>
      }
    >
      {links.isError ? (
        <InlineError>{links.error.message}</InlineError>
      ) : !links.data ? (
        <Skeleton className="h-16" />
      ) : items.length === 0 ? (
        <p className="text-sm text-ink-3">{t('share.panel.empty')}</p>
      ) : (
        <ul className="divide-y divide-line" aria-label={t('share.panel.links')}>
          {items.map((link) => {
            const status = linkStatus(link, nowMs)
            return (
              <li key={link.id} className="flex flex-col gap-2 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0">
                  <p className="flex flex-wrap items-center gap-2">
                    <span className="truncate font-semibold text-ink">{link.label}</span>
                    <span className={cn('chip h-5 border px-2 text-[11px]', status.cls)}>{status.label}</span>
                  </p>
                  <p className="num mt-0.5 text-xs text-ink-3">
                    {scopeSummary(link)} · {t('share.panel.views', { count: fmtNumber(link.accessCount) })}
                    {link.expiresAt ? t('share.panel.until', { date: fmtDateShort(link.expiresAt.slice(0, 10)) }) : ''}
                  </p>
                  {permissionsSummary(link.permissions) && <p className="mt-0.5 text-xs text-ink-2">{t('share.panel.allowed', { list: permissionsSummary(link.permissions) })}</p>}
                </div>
                <div className="flex flex-none items-center gap-1.5">
                  {status.active && (
                    <button type="button" className="btn btn-ghost h-9 px-3 text-xs" onClick={() => void copy(link)} aria-label={t('share.labels.copyLink', { label: link.label })}>
                      {copiedId === link.id ? <Check className="size-3.5 text-ok" aria-hidden="true" /> : <Copy className="size-3.5" aria-hidden="true" />}
                      {copiedId === link.id ? t('share.panel.copiedShort') : t('share.panel.copy')}
                    </button>
                  )}
                  <button type="button" className="icon-btn size-9" onClick={() => setDialog({ link })} aria-label={t('share.labels.editLink', { label: link.label })}>
                    <Pencil className="size-4" />
                  </button>
                  <button
                    type="button"
                    className="icon-btn size-9 text-danger"
                    onClick={() => revoke(link)}
                    disabled={m.revoke.isPending && m.revoke.variables === link.id}
                    aria-label={t('share.panel.revoke', { label: link.label })}
                  >
                    {m.revoke.isPending && m.revoke.variables === link.id ? <Loader2 className="size-4 animate-spin" /> : <Link2Off className="size-4" />}
                  </button>
                </div>
              </li>
            )
          })}
        </ul>
      )}
      {dialog && (
        <ShareLinkDialog
          key={dialog.link?.id ?? 'new'}
          link={dialog.link}
          onClose={() => setDialog(null)}
          onCreated={(l) => {
            setDialog(null)
            void copy(l)
          }}
        />
      )}
    </Card>
  )
}
