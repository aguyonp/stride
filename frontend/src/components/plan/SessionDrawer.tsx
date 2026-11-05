import { CalendarClock, Download, Loader2, Pencil, Trash2, Watch } from 'lucide-react'
import { useState } from 'react'
import { api, isApiError } from '../../api/client'
import { useIntegrations, usePlanMutations } from '../../api/queries'
import { usePlanV3 } from '../../api/queriesV3'
import { usePlanV31 } from '../../api/queriesV31'
import type { GarminPush, PlannedSession } from '../../api/types'
import { cn } from '../../lib/cn'
import { addDays, capitalize, fmtDateLong } from '../../lib/format'
import { PLAN_SPORT_LABEL } from '../../lib/labels'
import { downloadBlob } from '../profile/download'
import { Dialog } from '../ui/Dialog'
import { useToast } from '../ui/Toast'
import { GarminStatus } from './GarminStatus'
import { MatchBadge } from './MatchBadge'
import { targetParts } from './planFormat'
import { useSessionEditor } from './PlannedSessionDialog'
import { StepsView } from './StepsView'

const SOURCE_LABEL = { manual: 'Saisie manuelle', llm: 'Générée par ton IA', import: 'Importée (copier-coller IA)' } as const
const UNSUPPORTED = 'L’API personnelle Garmin ne permet pas de créer ni de planifier des séances (réservé aux partenaires Garmin). Le fichier .FIT est la voie fiable : il contient toute la structure.'

function WatchSection({ s, onGarmin }: { s: PlannedSession; onGarmin: (g: GarminPush) => void }) {
  const { push } = usePlanV3()
  const integ = useIntegrations()
  const { toast } = useToast()
  const [downloading, setDownloading] = useState(false)
  const garminOn = integ.data?.garmin.connected === true
  const unsupported = s.garmin?.status === 'unsupported'
  const send = () =>
    push.mutate(s.id, {
      onSuccess: (r) => {
        onGarmin(r.garmin)
        toast({ title: 'Séance envoyée sur Garmin Connect', description: 'Synchronise ta montre pour la retrouver dans Entraînement › Mes séances.', tone: 'success' })
      },
      onError: (e) => {
        if (isApiError(e, 'CONFLICT')) onGarmin({ status: 'unsupported', workoutId: null, pushedAt: null, error: s.garmin?.error ?? null })
        else toast({ title: 'Envoi impossible', description: e.message, tone: 'error' })
      },
    })
  const fit = async () => {
    setDownloading(true)
    try {
      const { blob, filename } = await api.workoutFit(s.id)
      downloadBlob(blob, filename)
      toast({ title: 'Fichier .FIT téléchargé', description: 'Copie-le dans GARMIN/NewFiles de ta montre (USB).' })
    } catch (e) {
      toast({ title: 'Téléchargement impossible', description: e instanceof Error ? e.message : undefined, tone: 'error' })
    } finally {
      setDownloading(false)
    }
  }
  const fitFirst = unsupported || !garminOn
  return (
    <section aria-labelledby="watch-title" className="rounded-2xl bg-track/50 p-4">
      <h3 id="watch-title" className="flex items-center gap-2 text-sm font-semibold text-ink">
        <Watch className="size-4 text-accent" aria-hidden="true" /> Sur ma montre
      </h3>
      {unsupported && (
        <div role="status" className="mt-3 rounded-xl border border-info/25 bg-info/[0.08] p-3 text-xs text-ink-2">
          <p className="font-semibold text-ink">Envoi direct vers Garmin indisponible</p>
          <p className="mt-1">{UNSUPPORTED}</p>
          {s.garmin.error && <p className="num mt-1.5 text-ink-3">Réponse Garmin : {s.garmin.error}</p>}
        </div>
      )}
      {!garminOn && integ.isSuccess && !unsupported && <p className="mt-2 text-xs text-ink-3">Garmin n’est pas connecté : télécharge le fichier .FIT pour l’installer sur ta montre.</p>}
      <div className="mt-3 flex flex-wrap gap-2">
        <button type="button" className={cn('btn', fitFirst ? 'btn-primary' : 'btn-ghost')} onClick={() => void fit()} disabled={downloading}>
          {downloading ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Download className="size-4" aria-hidden="true" />}
          Télécharger .FIT
        </button>
        <button
          type="button"
          className={cn('btn', fitFirst ? 'btn-ghost' : 'btn-primary')}
          onClick={send}
          disabled={push.isPending || !garminOn || unsupported}
          title={unsupported ? 'Indisponible avec l’API personnelle Garmin' : !garminOn ? 'Connecte Garmin dans Réglages' : undefined}
        >
          {push.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Watch className="size-4" aria-hidden="true" />}
          Envoyer vers Garmin
        </button>
      </div>
      <div className="mt-3 space-y-1.5">
        <GarminStatus s={s} />
        <p className="text-xs text-ink-3">
          .FIT : branche la montre en USB et copie le fichier dans <code className="rounded bg-track px-1">GARMIN/NewFiles</code> ; la séance apparaît dans Entraînement › Mes séances.
        </p>
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
          toast({ title: 'Séance décalée', description: capitalize(fmtDateLong(p.date)) })
        },
        onError: (e) => toast({ tone: 'error', title: 'Décalage impossible', description: e.message }),
      },
    )
  return (
    <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label="Décaler la séance">
      <span className="mr-1 flex items-center gap-1.5 text-xs font-medium text-ink-3">
        <CalendarClock className="size-3.5" aria-hidden="true" /> Décaler
      </span>
      <button type="button" className="chip h-8 border border-line bg-transparent px-3 text-ink-2 hover:bg-track" onClick={() => run({ days: 1 })} disabled={shift.isPending}>
        +1 j
      </button>
      <button type="button" className="chip h-8 border border-line bg-transparent px-3 text-ink-2 hover:bg-track" onClick={() => run({ days: 2 })} disabled={shift.isPending}>
        +2 j
      </button>
      {picking ? (
        <span className="flex items-center gap-1.5">
          <input type="date" className="field h-8 min-h-8 w-auto py-0 text-xs" value={to} onChange={(e) => setTo(e.target.value)} aria-label="Nouvelle date" />
          <button type="button" className="btn btn-primary h-8 min-h-8 px-3 text-xs" onClick={() => run({ toDate: to })} disabled={!to || to === s.date || shift.isPending}>
            Déplacer
          </button>
        </span>
      ) : (
        <button type="button" className="chip h-8 border border-line bg-transparent px-3 text-ink-2 hover:bg-track" onClick={() => setPicking(true)}>
          Choisir une date…
        </button>
      )}
      {shift.isPending && <Loader2 className="size-4 animate-spin text-ink-3" aria-label="Décalage en cours" />}
    </div>
  )
}

/** Side drawer opened from any planned session: details, quick shift, watch delivery, inline full edit and delete. */
export function SessionDrawer({ session, startInEdit = false, onClose }: { session: PlannedSession | null; startInEdit?: boolean; onClose: () => void }) {
  const { toast } = useToast()
  const { remove } = usePlanMutations()
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
        toast({ title: 'Séance supprimée', description: s.title })
        onClose()
      },
      onError: (e) => toast({ tone: 'error', title: 'Suppression impossible', description: e.message }),
    })

  const footer =
    s &&
    (editing ? (
      <div key="edit" className="flex justify-end gap-2 pb-3">
        <button type="button" className="btn btn-ghost" onClick={() => setMode((m) => ({ ...m, edit: false }))}>
          Annuler
        </button>
        <button type="submit" form={ed.formId} className="btn btn-primary" disabled={ed.pending}>
          {ed.pending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
          Enregistrer
        </button>
      </div>
    ) : (
      <div key="view" className="flex flex-wrap items-center justify-between gap-2 pb-3">
        {mode.confirm ? (
          <span className="flex items-center gap-2">
            <button type="button" className="btn btn-danger" onClick={del} disabled={remove.isPending}>
              {remove.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Trash2 className="size-4" aria-hidden="true" />}
              Confirmer
            </button>
            <button type="button" className="btn btn-ghost" onClick={() => setMode((m) => ({ ...m, confirm: false }))}>
              Garder
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
      title={editing ? 'Modifier la séance' : (s?.title ?? 'Séance')}
      description={s ? `${capitalize(fmtDateLong(editing ? ed.date || s.date : s.date))} · ${PLAN_SPORT_LABEL[s.sport]} · ${SOURCE_LABEL[s.source ?? 'manual']}` : undefined}
      footer={footer}
    >
      {s &&
        (editing ? (
          ed.form
        ) : (
          <div className="space-y-5">
            {s.match && s.match.status !== 'upcoming' && (
              <div className="flex flex-wrap items-center gap-2 text-sm text-ink-2">
                <MatchBadge status={s.match.status} /> {s.match.summary}
              </div>
            )}
            {targetParts(s).length > 0 && (
              <ul className="flex flex-wrap gap-1.5" aria-label="Cibles">
                {targetParts(s).map((t) => (
                  <li key={t} className="num chip bg-track text-ink-2">
                    {t}
                  </li>
                ))}
              </ul>
            )}
            {s.instructions && <p className="text-sm text-ink-2">{s.instructions}</p>}
            {(!s.match || s.match.status === 'upcoming' || s.match.status === 'missed') && <QuickShift key={s.id} s={s} onShifted={setOverride} />}
            {s.steps?.length > 0 ? <StepsView steps={s.steps} /> : s.sport !== 'rest' && <p className="text-sm text-ink-3">Pas de structure détaillée : « Modifier » pour ajouter des étapes.</p>}
            {workout && <WatchSection s={s} onGarmin={(g) => setOverride({ ...s, garmin: g })} />}
          </div>
        ))}
    </Dialog>
  )
}
