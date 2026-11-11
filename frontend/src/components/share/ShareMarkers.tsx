import { MessageSquareQuote, Stethoscope, UserRound } from 'lucide-react'
import { Link } from 'wouter'
import { useShareNotes } from '../../api/queriesV42'
import type { PrescribedBy } from '../../api/types'
import { cn } from '../../lib/cn'
import { isDirectiveActive, prescribedByLine } from './shareNotes'
import { t } from '../../i18n'

/** Sober dashboard badge: an active physio/coach directive exists → Santé & forme. */
export function DirectiveBadge({ today, className }: { today: string; className?: string }) {
  const notes = useShareNotes(true)
  const active = (notes.data ?? []).filter((n) => isDirectiveActive(n, today))
  if (active.length === 0) return null
  return (
    <Link href="/health#consignes" className={cn('chip h-8 border border-info/30 bg-info/10 px-3 text-info transition-colors hover:bg-info/15', className)}>
      <Stethoscope className="size-3.5" aria-hidden="true" />
      {t('share.panel.directiveActive')}
      {active.length > 1 && <span className="num grid h-5 min-w-5 place-items-center rounded-full bg-info/15 px-1.5 text-[11px]">{active.length}</span>}
    </Link>
  )
}

/** Marker on a session that received a physio/coach remark. */
export function RemarkMarker({ count, link = true, className }: { count: number | undefined; link?: boolean; className?: string }) {
  if (!count) return null
  const label = t('share.panel.remarks', { count })
  const cls = cn('chip h-6 border border-info/30 bg-info/10 px-2 text-[11px] text-info', className)
  const body = (
    <>
      <MessageSquareQuote className="size-3" aria-hidden="true" />
      {label}
    </>
  )
  return link ? (
    <Link href="/health#consignes" className={cls}>
      {body}
    </Link>
  ) : (
    <span className={cls}>{body}</span>
  )
}

/** « Prescrit par <label> » on a session or constraint created through a share link. */
export function PrescribedByTag({ by, className }: { by: PrescribedBy | null | undefined; className?: string }) {
  if (!by) return null
  return (
    <span className={cn('inline-flex max-w-full min-w-0 items-start gap-1 text-[11px] font-medium text-info', className)}>
      <UserRound className="mt-px size-3 flex-none" aria-hidden="true" />
      <span className="min-w-0 break-words">{prescribedByLine(by)}</span>
    </span>
  )
}
