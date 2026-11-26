import { AlertTriangle, Bell, CheckCheck, CircleAlert, Info, Sparkles, Stethoscope } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useLocation } from 'wouter'
import { useInbox, useInboxMutations } from '../../api/queries'
import type { InboxEntry } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtDateTime, fmtDateLong, toISODate, todayISO } from '../../lib/format'
import { Dialog } from '../ui/Dialog'
import { InlineError, Spinner } from '../ui/Misc'
import { t } from '../../i18n'

/** Unread badge cap: past it the exact count stops being useful. */
const BADGE_MAX = 99

/** Icon and tone of an entry, by level first (the severity is what the eye must catch). */
function entryIcon(e: InboxEntry) {
  if (e.kind === 'share') return Stethoscope
  if (e.kind === 'update') return Sparkles
  if (e.level === 'error') return CircleAlert
  if (e.level === 'warn') return AlertTriangle
  return Info
}

const LEVEL_TONE: Record<string, string> = {
  error: 'text-danger bg-danger/12 ring-danger/25',
  warn: 'text-warn bg-warn/12 ring-warn/25',
  success: 'text-ok bg-ok/12 ring-ok/25',
  info: 'text-accent bg-accent/12 ring-accent/25',
}

/**
 * Title of an entry. Alerts arrive already written by the analysis, in the user's language; share
 * writes and release notes are localized here from their stable code (§21), like the toasts.
 */
function entryTitle(e: InboxEntry): string {
  if (e.kind === 'share' && e.code) {
    switch (e.code) {
      case 'directive':
        return t('layout.inbox.share.directive')
      case 'remark':
        return t('layout.inbox.share.remark')
      case 'plan':
        return t('layout.inbox.share.plan')
      case 'event':
        return t('layout.inbox.share.event')
      default:
        return t('layout.inbox.share.other')
    }
  }
  if (e.kind === 'update') return t('layout.inbox.update.title')
  return e.title
}

/** Detail line: the share label then the session it comments, or the release version. */
function entryDetail(e: InboxEntry): string {
  if (e.kind === 'share') {
    const date = e.date ? t('layout.inbox.sessionOf', { date: fmtDateLong(e.date) }) : ''
    return [e.detail, date].filter(Boolean).join(' · ')
  }
  if (e.kind === 'update') return t('layout.inbox.update.detail', { version: e.code ?? '' })
  return e.detail
}

/** Where an entry leads: the session it names, the health page for a portal write, the release. */
function targetOf(e: InboxEntry): { path: string } | { external: string } {
  if (e.kind === 'update' && e.url) return { external: e.url }
  if (e.activityId) return { path: `/activities/${e.activityId}` }
  if (e.kind === 'share') return { path: '/health' }
  return { path: '/' }
}

function groupByDay(items: InboxEntry[]): Array<{ day: string; items: InboxEntry[] }> {
  const out: Array<{ day: string; items: InboxEntry[] }> = []
  for (const e of items) {
    const day = toISODate(new Date(e.createdAt))
    const last = out[out.length - 1]
    if (last && last.day === day) last.items.push(e)
    else out.push({ day, items: [e] })
  }
  return out
}

function dayLabel(day: string, today: string): string {
  if (day === today) return t('layout.inbox.today')
  if (day === toISODate(new Date(Date.now() - 86_400_000))) return t('layout.inbox.yesterday')
  return fmtDateLong(day)
}

/**
 * Notification centre (§21, issue #142): the bell and its badge in the header, the panel with the
 * history grouped by day. The toast stays the immediate signal; this is what survives it.
 */
export function NotificationBell() {
  const inbox = useInbox()
  const m = useInboxMutations()
  const [, navigate] = useLocation()
  const [open, setOpen] = useState(false)
  const unread = inbox.data?.unread ?? 0
  const today = todayISO()
  const days = useMemo(() => groupByDay(inbox.data?.items ?? []), [inbox.data])

  const openEntry = (e: InboxEntry) => {
    if (!e.readAt) m.markRead.mutate([e.id])
    const target = targetOf(e)
    setOpen(false)
    if ('external' in target) window.open(target.external, '_blank', 'noopener,noreferrer')
    else navigate(target.path)
  }

  return (
    <>
      <button
        type="button"
        className="icon-btn relative"
        onClick={() => setOpen(true)}
        aria-label={unread > 0 ? t('layout.inbox.bellUnread', { count: unread }) : t('layout.inbox.bell')}
        title={t('layout.inbox.bell')}
      >
        <Bell className="size-[1.125rem]" />
        {unread > 0 && (
          <span className="absolute -top-1 -right-1 grid h-4 min-w-4 place-items-center rounded-full bg-accent px-1 text-[10px] leading-none font-semibold text-on-accent">
            {unread > BADGE_MAX ? `${BADGE_MAX}+` : unread}
          </span>
        )}
      </button>

      <Dialog
        open={open}
        onClose={() => setOpen(false)}
        title={t('layout.inbox.title')}
        description={unread > 0 ? t('layout.inbox.unread', { count: unread }) : undefined}
        variant="drawer"
        size="lg"
        headerExtra={
          <button
            type="button"
            className="btn btn-ghost h-9 px-3 text-sm"
            onClick={() => m.markAll.mutate()}
            disabled={unread === 0 || m.markAll.isPending}
          >
            <CheckCheck className="size-4" aria-hidden="true" /> {t('layout.inbox.markAll')}
          </button>
        }
      >
        {inbox.isError ? (
          <InlineError>{t('layout.inbox.error')}</InlineError>
        ) : inbox.isPending ? (
          <Spinner label={t('common.actions.loading')} />
        ) : days.length === 0 ? (
          <div className="py-10 text-center">
            <Bell className="mx-auto mb-3 size-8 text-ink-3" aria-hidden="true" />
            <p className="font-medium">{t('layout.inbox.empty')}</p>
            <p className="mt-1 text-sm text-ink-3">{t('layout.inbox.emptyHint')}</p>
          </div>
        ) : (
          <div className="space-y-5">
            {days.map((g) => (
              <section key={g.day} aria-label={dayLabel(g.day, today)}>
                <p className="label-caps mb-2">{dayLabel(g.day, today)}</p>
                <ul className="space-y-1.5">
                  {g.items.map((e) => {
                    const Icon = entryIcon(e)
                    return (
                      <li key={e.id}>
                        <button
                          type="button"
                          onClick={() => openEntry(e)}
                          className={cn(
                            'flex w-full items-start gap-3 rounded-2xl border border-line/70 bg-card/60 p-3 text-left transition-colors hover:bg-track',
                            !e.readAt && 'border-accent/35 bg-accent/6',
                          )}
                        >
                          <span className={cn('mt-0.5 grid size-8 flex-none place-items-center rounded-xl ring-1 ring-inset', LEVEL_TONE[e.level] ?? LEVEL_TONE.info)}>
                            <Icon className="size-4" aria-hidden="true" />
                          </span>
                          <span className="min-w-0 flex-1">
                            <span className="flex items-baseline gap-2">
                              <span className={cn('min-w-0 flex-1 truncate text-sm', !e.readAt ? 'font-semibold' : 'font-medium')}>{entryTitle(e)}</span>
                              <span className="flex-none text-xs text-ink-3">{fmtDateTime(e.createdAt)}</span>
                            </span>
                            {entryDetail(e) && <span className="mt-0.5 line-clamp-2 block text-sm text-ink-2">{entryDetail(e)}</span>}
                          </span>
                          {!e.readAt && <span className="mt-2 size-2 flex-none rounded-full bg-accent" aria-label={t('layout.inbox.unreadOne')} />}
                        </button>
                      </li>
                    )
                  })}
                </ul>
              </section>
            ))}
            {inbox.data?.hasMore && <p className="text-center text-xs text-ink-3">{t('layout.inbox.more')}</p>}
          </div>
        )}
      </Dialog>
    </>
  )
}
