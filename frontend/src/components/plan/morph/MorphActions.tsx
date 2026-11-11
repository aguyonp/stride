import { Loader2, Undo2, WandSparkles } from 'lucide-react'
import { useState } from 'react'
import { useMorphActions, useMorphPreview } from '../../../api/queriesV42'
import { usePlanRange } from '../../../api/queriesV31'
import type { MorphPreview, PlannedSession } from '../../../api/types'
import { cn } from '../../../lib/cn'
import { addDays, fmtDateShort } from '../../../lib/format'
import { useToast } from '../../ui/Toast'
import { ConstraintChip, MorphDialog } from './MorphDialog'
import { isMorphCandidate } from './morphFormat'
import { t } from '../../../i18n'

/** Small « Adaptée » marker on a morphed session. */
export function MorphedTag({ s, className }: { s: PlannedSession; className?: string }) {
  if (!s.morph) return null
  return (
    <span className={cn('chip h-5 border border-violet/30 bg-violet/10 px-1.5 text-[11px] text-violet', className)} title={s.morph.reasons.length ? t('plan.morph.adaptedReasons', { reasons: s.morph.reasons.join(', ') }) : t('plan.morph.adapted')}>
      <WandSparkles className="size-3" aria-hidden="true" />
      {t('plan.morph.adapted')}
    </span>
  )
}

/** « Adapter la séance », shown only for today's/tomorrow's session when the server reports constraints. */
export function MorphTrigger({ session, today, className, label = t('plan.morph.adapt') }: { session: PlannedSession; today: string; className?: string; label?: string }) {
  const eligible = isMorphCandidate(session, today) && !session.morph
  const q = useMorphPreview(session.id, eligible)
  // The dialog outlives the trigger: once applied, the session is morphed and the button goes away.
  const [dialog, setDialog] = useState<MorphPreview | null>(null)
  const p = q.data
  const dlg = dialog && <MorphDialog open session={session} preview={dialog} onClose={() => setDialog(null)} />
  const show = eligible && p?.proposal && p.constraints.length > 0
  // Stable tree position: the dialog keeps its state (applied → .FIT) when the button disappears.
  return (
    <>
      {show && (
        <button type="button" className={cn('btn btn-ghost h-8 min-h-8 px-3 text-xs', className)} onClick={() => setDialog(p)}>
          <WandSparkles className="size-3.5" aria-hidden="true" />
          {label}
        </button>
      )}
      {dlg}
    </>
  )
}

/** Back to the planned version of a morphed session. */
function RevertButton({ s, onReverted }: { s: PlannedSession; onReverted?: (p: PlannedSession) => void }) {
  const { revert } = useMorphActions()
  const { toast } = useToast()
  const undo = () =>
    revert.mutate(s.id, {
      onSuccess: (p) => {
        toast({ title: t('plan.morph.restored'), description: p.title })
        onReverted?.(p)
      },
      onError: (e) => toast({ tone: 'error', title: t('plan.morph.revertFailed'), description: e.message }),
    })
  return (
    <button type="button" className="btn btn-ghost h-8 min-h-8 px-3 text-xs" onClick={undo} disabled={revert.isPending}>
      {revert.isPending ? <Loader2 className="size-3.5 animate-spin" aria-hidden="true" /> : <Undo2 className="size-3.5" aria-hidden="true" />}
      {t('plan.morph.revert')}
    </button>
  )
}

/** Morphed session: why it changed and the way back to the planned version. */
export function MorphedNote({ s, onReverted }: { s: PlannedSession; onReverted?: (p: PlannedSession) => void }) {
  if (!s.morph) return null
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 rounded-2xl border border-violet/25 bg-violet/[0.06] px-3 py-2.5">
      <div className="min-w-0 text-xs text-ink-2">
        <p className="font-semibold text-violet">{t('plan.morph.adaptedOn', { date: fmtDateShort(s.morph.at.slice(0, 10)) })}</p>
        {s.morph.reasons.length > 0 && <p className="mt-0.5">{s.morph.reasons.join(' · ')}</p>}
        <p className="mt-0.5 text-ink-3">{t('plan.morph.original', { title: s.morph.original.title })}</p>
      </div>
      <RevertButton s={s} onReverted={onReverted} />
    </div>
  )
}

function AdaptRow({ session, today }: { session: PlannedSession; today: string }) {
  const q = useMorphPreview(session.id, !session.morph)
  const [dialog, setDialog] = useState<MorphPreview | null>(null)
  const when = session.date === today ? t('plan.morph.today') : t('plan.morph.tomorrow')
  const dlg = dialog && <MorphDialog open session={session} preview={dialog} onClose={() => setDialog(null)} />
  const p = q.data
  let content = null
  if (session.morph)
    content = (
      <div className="glass-inset flex flex-wrap items-center gap-x-3 gap-y-1.5 px-3 py-2.5 sm:px-4">
        <MorphedTag s={session} />
        <p className="min-w-0 flex-1 truncate text-sm text-ink-2">
          <span className="font-semibold text-ink">{when}</span> · {session.title}
        </p>
        <RevertButton s={session} />
      </div>
    )
  else if (p?.proposal && p.constraints.length > 0)
    content = (
    <section className="glass flex flex-wrap items-center gap-x-4 gap-y-2.5 p-3 sm:px-4" aria-label={t('plan.morph.sessionToAdapt', { when: when.toLowerCase() })}>
      <div className="min-w-0 flex-1 basis-60">
        <p className="truncate text-sm text-ink-2">
          <span className="font-semibold text-ink">{when}</span> · {session.title}
        </p>
        <ul className="mt-1.5 flex flex-wrap gap-1" aria-label={t('plan.morph.constraints')}>
          {p.constraints.map((c, i) => (
            <ConstraintChip key={`${c.kind}-${i}`} c={c} compact />
          ))}
        </ul>
      </div>
      <button type="button" className="btn btn-primary h-9 min-h-9 flex-none px-3.5 text-sm" onClick={() => setDialog(p)}>
        <WandSparkles className="size-4" aria-hidden="true" />
        {t('plan.morph.adapt')}
      </button>
    </section>
    )
  return (
    <>
      {content}
      {dlg}
    </>
  )
}

/** Dashboard hero strip: today's and tomorrow's sessions facing constraints (nothing rendered otherwise). */
export function SessionAdaptStrip({ today }: { today: string }) {
  const plan = usePlanRange(today, addDays(today, 1))
  const sessions = (plan.data ?? []).filter((s) => isMorphCandidate(s, today))
  if (sessions.length === 0) return null
  return (
    <div className="space-y-2">
      {sessions.map((s) => (
        <AdaptRow key={s.id} session={s} today={today} />
      ))}
    </div>
  )
}
