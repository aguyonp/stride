import { Bug, Lightbulb, Loader2, Mail, MessageCircle, MessageSquareText, type LucideIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useAdminControl, useAdminFeedback } from '../../api/queries'
import type { FeedbackCategory, FeedbackItem, FeedbackStatus } from '../../api/types'
import { Card } from '../../components/ui/Card'
import { Dialog } from '../../components/ui/Dialog'
import { Field } from '../../components/ui/Field'
import { EmptyState, KeyValue } from '../../components/ui/Misc'
import { SegmentedControl } from '../../components/ui/SegmentedControl'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { cn } from '../../lib/cn'
import { fmtDateTime, fmtWhen } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { FilterChips } from './adminUtils'
import { lazyRecord, t, tx } from '../../i18n'

const CATEGORY_STYLE: Record<FeedbackCategory, { icon: LucideIcon; cls: string }> = {
  bug: { icon: Bug, cls: 'border-danger/30 bg-danger/10 text-danger' },
  idea: { icon: Lightbulb, cls: 'border-info/30 bg-info/10 text-info' },
  other: { icon: MessageCircle, cls: 'border-line bg-track/60 text-ink-2' },
}
const CATEGORY: Record<FeedbackCategory, { label: string; icon: LucideIcon; cls: string }> = lazyRecord(['bug', 'idea', 'other'] as const, (k) => ({
  ...CATEGORY_STYLE[k],
  label: t(`admin.feedback.category.${k}`),
}))
const STATUS_LABEL: Record<FeedbackStatus, string> = lazyRecord(['new', 'read', 'done'] as const, (k) => t(`admin.feedback.status.${k}`))

function CategoryChip({ c }: { c: FeedbackCategory }) {
  const m = CATEGORY[c]
  return (
    <span className={cn('chip h-6 flex-none border px-2 text-[11px]', m.cls)}>
      <m.icon className="size-3.5" aria-hidden="true" />
      {m.label}
    </span>
  )
}

function mailto(f: FeedbackItem): string {
  const quoted = f.message
    .slice(0, 600)
    .split('\n')
    .map((l) => `> ${l}`)
    .join('\n')
  return `mailto:${f.contactEmail}?subject=${encodeURIComponent('Re : ton avis sur Stride')}&body=${encodeURIComponent(`Bonjour ${f.userName},\n\n${quoted}\n\n`)}`
}

function FeedbackDetail({ item, onClose }: { item: FeedbackItem; onClose: () => void }) {
  const ctl = useAdminControl()
  const { toast } = useToast()
  const [status, setStatus] = useState<FeedbackStatus>(item.status === 'new' ? 'read' : item.status)
  const [note, setNote] = useState(item.adminNote)
  const marked = useRef(false)
  const update = ctl.updateFeedback
  const { mutate } = update

  // Opening an unread feedback marks it as read.
  useEffect(() => {
    if (item.status === 'new' && !marked.current) {
      marked.current = true
      mutate({ id: item.id, status: 'read' })
    }
  }, [item.id, item.status, mutate])

  const setAndSave = (s: FeedbackStatus) => {
    setStatus(s)
    update.mutate({ id: item.id, status: s }, { onError: (e) => toast({ tone: 'error', title: t('admin.feedback.statusFailed'), description: e.message }) })
  }
  const saveNote = () =>
    update.mutate(
      { id: item.id, adminNote: note },
      { onSuccess: () => toast({ title: t('admin.feedback.noteSaved') }), onError: (e) => toast({ tone: 'error', title: t('admin.feedback.noteFailed'), description: e.message }) },
    )

  return (
    <Dialog open onClose={onClose} variant="drawer" title={t('admin.feedback.from', { name: item.userName || `#${item.userId}` })} description={`${CATEGORY[item.category].label} · ${fmtDateTime(item.createdAt)}`}>
      <div className="space-y-5">
        <p className="rounded-2xl border border-line bg-track/40 px-4 py-3 text-sm leading-relaxed break-words whitespace-pre-wrap text-ink">{item.message}</p>

        <section aria-label={t('admin.feedback.contact')}>
          {item.contactOk && item.contactEmail ? (
            <div className="flex flex-wrap items-center justify-between gap-3">
              <span className="min-w-0 text-sm text-ink-2">
                {tx('admin.feedback.contactOk', { email: <span className="font-medium break-all text-ink">{item.contactEmail}</span> })}
              </span>
              <a className="btn btn-primary" href={mailto(item)}>
                <Mail className="size-4" aria-hidden="true" />
                {t('admin.feedback.reply')}
              </a>
            </div>
          ) : (
            <p className="text-sm text-ink-3">{t('admin.feedback.noContact')}</p>
          )}
        </section>

        <div>
          <p className="mb-1.5 text-sm font-medium text-ink-2">{t('admin.feedback.statusLabel')}</p>
          <SegmentedControl<FeedbackStatus>
            ariaLabel={t('admin.feedback.statusAria')}
            className="max-w-sm"
            value={status}
            onChange={setAndSave}
            options={(['new', 'read', 'done'] as const).map((s) => ({ value: s, label: STATUS_LABEL[s] }))}
          />
        </div>

        <Field label={t('admin.feedback.note')} hint={t('admin.feedback.noteHint')}>
          {(p) => <textarea {...p} className="field min-h-24 resize-y text-sm" maxLength={2000} value={note} onChange={(e) => setNote(e.target.value)} placeholder={t('admin.feedback.notePlaceholder')} />}
        </Field>
        <button type="button" className="btn btn-ghost" onClick={saveNote} disabled={update.isPending || note === item.adminNote}>
          {update.isPending && update.variables?.adminNote !== undefined && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
          {t('admin.feedback.saveNote')}
        </button>

        <section aria-label={t('admin.feedback.technical')} className="border-t border-line pt-4">
          <p className="label-caps mb-2">{t('admin.feedback.attached')}</p>
          <KeyValue
            items={[
              [t('admin.feedback.version'), <span className="num">{item.context.appVersion || '—'}</span>],
              [t('admin.feedback.page'), <span className="font-mono text-xs break-all">{item.context.route || '—'}</span>],
              [t('admin.feedback.screen'), <span className="num">{item.context.viewport || '—'}</span>],
              [t('admin.feedback.browser'), <span className="text-xs break-all">{item.context.userAgent || '—'}</span>],
            ]}
          />
        </section>
      </div>
    </Dialog>
  )
}

export function FeedbackPanel() {
  const [status, setStatus] = useState<FeedbackStatus | 'all'>('new')
  const [category, setCategory] = useState<FeedbackCategory | 'all'>('all')
  // Snapshot of the opened entry: it stays open even when « mark as read » moves it out of the filter.
  const [openItem, setOpenItem] = useState<FeedbackItem | null>(null)
  const list = useAdminFeedback(status, category)
  const items = list.data?.items ?? []
  const counts = list.data?.counts

  return (
    <Card
      title={t('admin.feedback.title')}
      icon={<MessageSquareText className="size-4" />}
      subtitle={counts ? t('admin.feedback.counts', { unread: t('admin.feedback.unread', { count: counts.new }), total: counts.total }) : t('admin.feedback.subtitleDefault')}
    >
      <div className="mb-4 flex flex-wrap items-center gap-x-4 gap-y-2.5">
        <FilterChips<FeedbackStatus | 'all'>
          ariaLabel={t('admin.feedback.filterStatus')}
          value={status}
          onChange={setStatus}
          options={[
            { value: 'new', label: t('admin.feedback.filterNew'), count: counts?.new },
            { value: 'read', label: t('admin.feedback.filterRead'), count: counts?.read },
            { value: 'done', label: t('admin.feedback.filterDone'), count: counts?.done },
            { value: 'all', label: t('admin.feedback.all'), count: counts?.total },
          ]}
        />
        <label className="flex items-center gap-2 text-xs text-ink-3 sm:ml-auto">
          {t('admin.feedback.filterCategory')}
          <select className="field min-h-9 w-auto py-1 text-sm" value={category} onChange={(e) => setCategory(e.target.value as FeedbackCategory | 'all')}>
            <option value="all">{t('admin.feedback.all')}</option>
            <option value="bug">{t('admin.feedback.bugs')}</option>
            <option value="idea">{t('admin.feedback.ideas')}</option>
            <option value="other">{t('admin.feedback.others')}</option>
          </select>
        </label>
      </div>

      {list.isError ? (
        <ErrorView error={list.error} onRetry={() => void list.refetch()} title={t('admin.feedback.unavailable')} />
      ) : !list.data ? (
        <div className="space-y-2">
          <Skeleton className="h-16" />
          <Skeleton className="h-16" />
        </div>
      ) : items.length === 0 ? (
        <EmptyState icon={<MessageSquareText />} title={status === 'new' ? t('admin.feedback.noUnread') : t('admin.feedback.none')}>
          {t('admin.feedback.emptyHint')}
        </EmptyState>
      ) : (
        <ul className="divide-y divide-line border-y border-line">
          {items.map((f) => (
            <li key={f.id}>
              <button type="button" onClick={() => setOpenItem(f)} className="flex w-full items-start gap-3 px-1 py-3 text-left transition-colors hover:bg-track/50 focus-visible:bg-track/50">
                <span className={cn('mt-2 size-2 flex-none rounded-full', f.status === 'new' ? 'bg-accent' : 'bg-transparent')} aria-hidden="true" />
                <span className="min-w-0 flex-1">
                  <span className="flex flex-wrap items-center gap-2">
                    <CategoryChip c={f.category} />
                    <span className={cn('text-sm', f.status === 'new' ? 'font-semibold text-ink' : 'font-medium text-ink-2')}>{f.userName || `#${f.userId}`}</span>
                    {f.contactOk && <Mail className="size-3.5 text-ink-3" aria-label={t('admin.feedback.contactAria')} />}
                    {f.status === 'done' && <span className="text-xs text-ok">{t('admin.feedback.done')}</span>}
                    <span className="num ml-auto text-xs text-ink-3">{fmtWhen(f.createdAt)}</span>
                  </span>
                  <span className="mt-1 line-clamp-2 block text-sm break-words text-ink-2">{f.message}</span>
                  {f.status === 'new' && <span className="sr-only">{t('admin.feedback.unreadSr')}</span>}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {openItem && <FeedbackDetail key={openItem.id} item={openItem} onClose={() => setOpenItem(null)} />}
    </Card>
  )
}
