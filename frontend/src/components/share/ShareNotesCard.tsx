import { MessageSquareQuote, Stethoscope, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Link } from 'wouter'
import { useDeleteShareNote, useShareNotes } from '../../api/queriesV42'
import type { ShareNote } from '../../api/types'
import { fmtDateShort } from '../../lib/format'
import { Card } from '../ui/Card'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { InlineError } from '../ui/Misc'
import { useToast } from '../ui/Toast'
import { authorLine, isDirectiveActive, restrictionChips, validityLine } from './shareNotes'
import { t } from '../../i18n'

const RECENT_REMARKS = 8

/** Santé & forme: active physio/coach directives and recent session remarks, each removable by the athlete. */
export function ShareNotesCard({ today, className }: { today: string; className?: string }) {
  const q = useShareNotes(true)
  const del = useDeleteShareNote()
  const { toast } = useToast()
  const [target, setTarget] = useState<ShareNote | null>(null)
  const notes = q.data ?? []
  const directives = notes.filter((n) => isDirectiveActive(n, today))
  const remarks = notes.filter((n) => n.kind === 'remark').slice(0, RECENT_REMARKS)
  const hasNotes = directives.length + remarks.length > 0

  useEffect(() => {
    if (hasNotes && window.location.hash === '#consignes') document.getElementById('consignes')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }, [hasNotes])

  if (q.isError) return <InlineError>{q.error.message}</InlineError>
  if (!hasNotes) return null

  const remove = () =>
    target &&
    del.mutate(target.id, {
      onSuccess: () => {
        toast({ title: target.kind === 'directive' ? t('share.panel.notesDeletedDirective') : t('share.panel.notesDeletedRemark') })
        setTarget(null)
      },
      onError: (e) => toast({ tone: 'error', title: t('share.panel.deleteFailed'), description: e.message }),
    })

  return (
    <section id="consignes" className="scroll-mt-24">
      <Card className={className} icon={<Stethoscope className="size-4" />} title={t('share.panel.notesTitle')} subtitle={t('share.panel.notesSubtitle')}>
        <div className="space-y-4">
          {directives.length > 0 && (
            <ul className="space-y-2.5" aria-label={t('share.panel.activeDirectives')}>
              {directives.map((n) => (
                <li key={n.id} className="glass-inset p-3.5">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="text-[11px] font-semibold text-info">{t('common.extra.activeDirective', { label: n.linkLabel })}</p>
                      <p className="mt-1 text-sm break-words whitespace-pre-line text-ink">{n.body}</p>
                    </div>
                    <button type="button" className="icon-btn size-8 flex-none" onClick={() => setTarget(n)} aria-label={t('share.labels.deleteDirectiveOf', { label: n.linkLabel })}>
                      <Trash2 className="size-3.5" />
                    </button>
                  </div>
                  {restrictionChips(n.restrictions).length > 0 && (
                    <ul className="mt-2 flex flex-wrap gap-1" aria-label={t('share.panel.restrictions')}>
                      {restrictionChips(n.restrictions).map((r) => (
                        <li key={r} className="rounded-lg bg-track px-1.5 py-0.5 text-[11px] font-semibold text-ink-2">
                          {r}
                        </li>
                      ))}
                    </ul>
                  )}
                  <p className="num mt-2 text-xs text-ink-3">
                    {authorLine(n)} · {fmtDateShort(n.updatedAt.slice(0, 10))} · {validityLine(n)}
                  </p>
                </li>
              ))}
            </ul>
          )}

          {remarks.length > 0 && (
            <div>
              <h3 className="mb-1.5 text-xs font-semibold text-ink-2">{t('share.panel.recentRemarks')}</h3>
        <ul className="divide-y divide-line" aria-label={t('share.panel.recentRemarksAria')}>
                {remarks.map((n) => (
                  <li key={n.id} className="flex items-start gap-3 py-2.5 first:pt-0 last:pb-0">
                    <MessageSquareQuote className="mt-0.5 size-4 flex-none text-info" aria-hidden="true" />
                    <div className="min-w-0 flex-1">
                      <p className="text-sm break-words whitespace-pre-line text-ink-2">{n.body}</p>
                      <p className="num mt-0.5 text-xs text-ink-3">
                        {authorLine(n)} · {fmtDateShort(n.updatedAt.slice(0, 10))}
                        {n.activityId !== null && (
                          <>
                            {' · '}
                            <Link href={`/activities/${n.activityId}`} className="text-accent underline-offset-2 hover:underline">
                              {t('share.panel.viewSession')}
                            </Link>
                          </>
                        )}
                      </p>
                    </div>
                    <button type="button" className="icon-btn size-8 flex-none" onClick={() => setTarget(n)} aria-label={t('share.labels.deleteRemarkOf', { label: n.linkLabel })}>
                      <Trash2 className="size-3.5" />
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      </Card>
      <ConfirmDialog
        open={target !== null}
        onClose={() => setTarget(null)}
        title={target?.kind === 'directive' ? t('share.panel.deleteDirective') : t('share.panel.deleteRemark')}
        message={<p>{t('share.panel.deleteBody')}</p>}
        confirmLabel={t('common.actions.delete')}
        danger
        busy={del.isPending}
        onConfirm={remove}
      />
    </section>
  )
}
