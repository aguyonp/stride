import { CalendarClock, Download, ExternalLink, Loader2, Pencil, Trash2, Watch } from 'lucide-react'
import { useState } from 'react'
import { api, isApiError } from '../../api/client'
import { useGarminBridge, useIntegrations, usePlanMutations, useProfile } from '../../api/queries'
import { usePlanV3 } from '../../api/queriesV3'
import { usePlanV31 } from '../../api/queriesV31'
import type { GarminPush, PlannedSession } from '../../api/types'
import { cn } from '../../lib/cn'
import { addDays, capitalize, fmtDateLong, todayISO } from '../../lib/format'
import { PLAN_SPORT_LABEL } from '../../lib/labels'
import { downloadBlob } from '../profile/download'
import { Dialog } from '../ui/Dialog'
import { useToast } from '../ui/Toast'
import { GarminStatus } from './GarminStatus'
import { MatchBadge } from './MatchBadge'
import { targetParts } from './planFormat'
import { useSessionEditor } from './PlannedSessionDialog'
import { StepsView } from './StepsView'
import { MorphedNote, MorphTrigger } from './morph/MorphActions'
import { PrescribedByTag } from '../share/ShareMarkers'
import { lazyRecord, t } from '../../i18n'

const SOURCE_LABEL = lazyRecord(['manual', 'llm', 'import', 'share'] as const, (k) => t(`plan.drawer.source.${k}`))

function WatchSection({ s, onGarmin }: { s: PlannedSession; onGarmin: (g: GarminPush) => void }) {
  const { push } = usePlanV3()
  const integ = useIntegrations()
  const { toast } = useToast()
  const [downloading, setDownloading] = useState(false)
  // Two ways to reach the watch: the Personal API key, and the experimental bridge. The button must
  // follow both — a connected bridge that leaves « Envoyer vers Garmin » greyed out is a dead end.
  const bridge = useGarminBridge()
  const garminOn = integ.data?.garmin.connected === true || bridge.data?.state === 'connected'
  const unsupported = s.garmin?.status === 'unsupported'
  const send = () =>
    push.mutate(s.id, {
      onSuccess: (r) => {
        onGarmin(r.garmin)
        toast({ title: t('plan.drawer.sent'), description: t('plan.drawer.sentHint'), tone: 'success' })
      },
      onError: (e) => {
        if (isApiError(e, 'CONFLICT')) onGarmin({ status: 'unsupported', workoutId: null, pushedAt: null, error: s.garmin?.error ?? null })
        else toast({ title: t('plan.drawer.sendFailed'), description: e.message, tone: 'error' })
      },
    })
  const fit = async () => {
    setDownloading(true)
    try {
      const { blob, filename } = await api.workoutFit(s.id)
      downloadBlob(blob, filename)
      toast({ title: t('plan.drawer.fitDownloaded'), description: t('plan.drawer.fitHint') })
    } catch (e) {
      toast({ title: t('plan.drawer.downloadFailed'), description: e instanceof Error ? e.message : undefined, tone: 'error' })
    } finally {
      setDownloading(false)
    }
  }
  const fitFirst = unsupported || !garminOn
  return (
    <section aria-labelledby="watch-title" className="rounded-2xl bg-track/50 p-4">
      <h3 id="watch-title" className="flex items-center gap-2 text-sm font-semibold text-ink">
        <Watch className="size-4 text-accent" aria-hidden="true" /> {t('plan.drawer.watch')}
      </h3>
      {unsupported && (
        <div role="status" className="mt-3 rounded-xl border border-info/25 bg-info/[0.08] p-3 text-xs text-ink-2">
          <p className="font-semibold text-ink">{t('plan.drawer.directUnavailable')}</p>
          <p className="mt-1">{t('plan.drawer.unsupported')}</p>
          {s.garmin.error && <p className="num mt-1.5 text-ink-3">{t('plan.drawer.garminReply', { error: s.garmin.error })}</p>}
        </div>
      )}
      {!garminOn && integ.isSuccess && !unsupported && <p className="mt-2 text-xs text-ink-3">{t('plan.drawer.garminOff')}</p>}
      <div className="mt-3 flex flex-wrap gap-2">
        <button type="button" className={cn('btn', fitFirst ? 'btn-primary' : 'btn-ghost')} onClick={() => void fit()} disabled={downloading}>
          {downloading ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Download className="size-4" aria-hidden="true" />}
          {t('plan.drawer.downloadFit')}
        </button>
        <button
          type="button"
          className={cn('btn', fitFirst ? 'btn-ghost' : 'btn-primary')}
          onClick={send}
          disabled={push.isPending || !garminOn || unsupported}
          title={unsupported ? t('plan.drawer.unsupportedShort') : !garminOn ? t('plan.drawer.connectGarmin') : undefined}
        >
          {push.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Watch className="size-4" aria-hidden="true" />}
          {t('plan.drawer.sendGarmin')}
        </button>
      </div>
      <div className="mt-3 space-y-2">
        <GarminStatus s={s} />
        {/* The cable is gone: the .FIT goes through Garmin Connect Web (§23, issue #147). */}
        <div className="rounded-xl bg-track/50 p-3">
          <p className="text-xs text-ink-2">{t('plan.drawer.fitHelp')}</p>
          <ol className="mt-1.5 space-y-0.5 text-xs text-ink-3">
            <li>{t('plan.drawer.fitStep1')}</li>
            <li>{t('plan.drawer.fitStep2')}</li>
            <li>{t('plan.drawer.fitStep3')}</li>
          </ol>
          <a className="btn btn-ghost mt-2 h-8 px-2.5 text-xs" href="https://connect.garmin.com/modern/workouts" target="_blank" rel="noopener noreferrer">
            <ExternalLink className="size-3.5" aria-hidden="true" /> {t('plan.drawer.openConnect')}
          </a>
        </div>
      </div>
    </section>
  )
}

function QuickShift({ s, onShifted }: { s: PlannedSession; onShifted: (p: PlannedSession) => void }) {
  const { shift } = usePlanV31()
  const { toast } = useToast()
  const [picking, setPicking] = useState(false)
  const [to, setTo] = useState(addDays(s.date, 1))
  const run = (body: { days?: number; toDate?: string }) =>
    shift.mutate(
      { id: s.id, ...body },
      {
        onSuccess: (p) => {
          onShifted(p)
          setPicking(false)
          toast({ title: t('plan.drawer.shifted'), description: capitalize(fmtDateLong(p.date)) })
        },
        onError: (e) => toast({ tone: 'error', title: t('plan.drawer.shiftFailed'), description: e.message }),
      },
    )
  return (
    <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label={t('plan.drawer.shiftGroup')}>
      <span className="mr-1 flex items-center gap-1.5 text-xs font-medium text-ink-3">
        <CalendarClock className="size-3.5" aria-hidden="true" /> {t('plan.drawer.shift')}
      </span>
      <button type="button" className="chip h-8 border border-line bg-transparent px-3 text-ink-2 hover:bg-track" onClick={() => run({ days: 1 })} disabled={shift.isPending}>
        {t('plan.drawer.plusDays', { n: 1 })}
      </button>
      <button type="button" className="chip h-8 border border-line bg-transparent px-3 text-ink-2 hover:bg-track" onClick={() => run({ days: 2 })} disabled={shift.isPending}>
        {t('plan.drawer.plusDays', { n: 2 })}
      </button>
      {picking ? (
        <span className="flex items-center gap-1.5">
          <input type="date" className="field h-8 min-h-8 w-auto py-0 text-xs" value={to} onChange={(e) => setTo(e.target.value)} aria-label={t('plan.drawer.newDate')} />
          <button type="button" className="btn btn-primary h-8 min-h-8 px-3 text-xs" onClick={() => run({ toDate: to })} disabled={!to || to === s.date || shift.isPending}>
            {t('plan.drawer.move')}
          </button>
        </span>
      ) : (
        <button type="button" className="chip h-8 border border-line bg-transparent px-3 text-ink-2 hover:bg-track" onClick={() => setPicking(true)}>
          {t('plan.drawer.pickDate')}
        </button>
      )}
      {shift.isPending && <Loader2 className="size-4 animate-spin text-ink-3" aria-label={t('plan.drawer.shifting')} />}
    </div>
  )
}

/** Side drawer opened from any planned session: details, quick shift, watch delivery, inline full edit and delete. */
export function SessionDrawer({ session, startInEdit = false, onClose }: { session: PlannedSession | null; startInEdit?: boolean; onClose: () => void }) {
  const { toast } = useToast()
  const { remove } = usePlanMutations()
  const today = todayISO(useProfile().data?.timezone)
  const [override, setOverride] = useState<PlannedSession | null>(null)
  const [mode, setMode] = useState<{ id: number | null; edit: boolean; confirm: boolean }>({ id: null, edit: false, confirm: false })
  const id = session?.id ?? null
  if (mode.id !== id) setMode({ id, edit: startInEdit, confirm: false })
  const s = session && override?.id === session.id ? override : session
  const editing = mode.id === id ? mode.edit : startInEdit
  const ed = useSessionEditor(s, s?.date ?? '', s !== null && editing, (saved) => {
    setOverride(saved)
    setMode((m) => ({ ...m, edit: false }))
  })
  const workout = s && s.sport !== 'rest' && s.sport !== 'strength' && s.sport !== 'mobility'
  const del = () =>
    s &&
    remove.mutate(s.id, {
      onSuccess: () => {
        toast({ title: t('plan.drawer.deleted'), description: s.title })
        onClose()
      },
      onError: (e) => toast({ tone: 'error', title: t('plan.drawer.deleteFailed'), description: e.message }),
    })

  const footer =
    s &&
    (editing ? (
      <div key="edit" className="flex justify-end gap-2 pb-3">
        <button type="button" className="btn btn-ghost" onClick={() => setMode((m) => ({ ...m, edit: false }))}>
          {t('common.actions.cancel')}
        </button>
        <button type="submit" form={ed.formId} className="btn btn-primary" disabled={ed.pending}>
          {ed.pending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
          {t('common.actions.save')}
        </button>
      </div>
    ) : (
      <div key="view" className="flex flex-wrap items-center justify-between gap-2 pb-3">
        {mode.confirm ? (
          <span className="flex items-center gap-2">
            <button type="button" className="btn btn-danger" onClick={del} disabled={remove.isPending}>
              {remove.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Trash2 className="size-4" aria-hidden="true" />}
              {t('common.actions.confirm')}
            </button>
            <button type="button" className="btn btn-ghost" onClick={() => setMode((m) => ({ ...m, confirm: false }))}>
              {t('common.misc.keep')}
            </button>
          </span>
        ) : (
          <button type="button" className="btn btn-ghost text-danger" onClick={() => setMode((m) => ({ ...m, confirm: true }))}>
            <Trash2 className="size-4" aria-hidden="true" /> Supprimer
          </button>
        )}
        <button type="button" className="btn btn-primary" onClick={() => setMode((m) => ({ ...m, edit: true, confirm: false }))}>
          <Pencil className="size-4" aria-hidden="true" /> Modifier
        </button>
      </div>
    ))

  return (
    <Dialog
      open={s !== null}
      onClose={onClose}
      variant="drawer"
      title={editing ? t('plan.drawer.editTitle') : (s?.title ?? t('plan.drawer.session'))}
      description={s ? `${capitalize(fmtDateLong(editing ? ed.date || s.date : s.date))} · ${PLAN_SPORT_LABEL[s.sport]} · ${SOURCE_LABEL[s.source ?? 'manual']}` : undefined}
      footer={footer}
    >
      {s &&
        (editing ? (
          ed.form
        ) : (
          <div className="space-y-5">
            {s.prescribedBy && <PrescribedByTag by={s.prescribedBy} className="text-xs" />}
            {s.morph ? <MorphedNote s={s} onReverted={setOverride} /> : <MorphTrigger session={s} today={today} className="-mt-1" />}
            {s.match && s.match.status !== 'upcoming' && (
              <div className="flex flex-wrap items-center gap-2 text-sm text-ink-2">
                <MatchBadge status={s.match.status} /> {s.match.summary}
              </div>
            )}
            {targetParts(s).length > 0 && (
              <ul className="flex flex-wrap gap-1.5" aria-label={t('plan.drawer.targets')}>
                {targetParts(s).map((t) => (
                  <li key={t} className="num chip bg-track text-ink-2">
                    {t}
                  </li>
                ))}
              </ul>
            )}
            {s.instructions && <p className="text-sm text-ink-2">{s.instructions}</p>}
            {(!s.match || s.match.status === 'upcoming' || s.match.status === 'missed') && <QuickShift key={s.id} s={s} onShifted={setOverride} />}
            {s.steps?.length > 0 ? <StepsView steps={s.steps} /> : s.sport !== 'rest' && <p className="text-sm text-ink-3">{t('plan.drawer.noStructure')}</p>}
            {workout && <WatchSection s={s} onGarmin={(g) => setOverride({ ...s, garmin: g })} />}
          </div>
        ))}
    </Dialog>
  )
}
