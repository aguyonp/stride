import { CircleCheck, Download, Info, Loader2, Pencil, Trash2, Watch } from 'lucide-react'
import { useState } from 'react'
import { api, isApiError } from '../../api/client'
import { usePlanV3 } from '../../api/queriesV3'
import type { GarminPush, PlannedSession } from '../../api/types'
import { capitalize, fmtDateLong, fmtDateTime } from '../../lib/format'
import { PLAN_SPORT_LABEL } from '../../lib/labels'
import { downloadBlob } from '../profile/download'
import { Dialog } from '../ui/Dialog'
import { useToast } from '../ui/Toast'
import { MatchBadge } from './MatchBadge'
import { targetParts } from './planFormat'
import { StepsView } from './StepsView'

const SOURCE_LABEL = { manual: 'Saisie manuelle', llm: 'Générée par ton IA', import: 'Importée (copier-coller IA)' } as const

/** Push to Garmin (degrades to « unsupported ») and FIT download, with toasts. */
export function useWorkoutActions() {
  const { push } = usePlanV3()
  const { toast } = useToast()
  const [downloading, setDownloading] = useState<number | null>(null)
  const sendToGarmin = (s: PlannedSession, onStatus?: (g: GarminPush) => void) =>
    push.mutate(s.id, {
      onSuccess: (r) => {
        onStatus?.(r.garmin)
        toast({ title: 'Séance envoyée sur Garmin Connect', description: 'Synchronise ta montre pour la retrouver dans Entraînement › Mes séances.', tone: 'success' })
      },
      onError: (e) => {
        if (isApiError(e, 'CONFLICT')) onStatus?.({ status: 'unsupported', workoutId: null, pushedAt: null, error: e.message })
        return isApiError(e, 'CONFLICT')
          ? toast({ title: 'Envoi direct indisponible', description: 'Ta clé Garmin ne donne pas accès aux séances : télécharge le fichier .FIT.', tone: 'info' })
          : toast({ title: 'Envoi impossible', description: e.message, tone: 'error' })
      },
    })
  const downloadFit = async (s: PlannedSession) => {
    setDownloading(s.id)
    try {
      const { blob, filename } = await api.workoutFit(s.id)
      downloadBlob(blob, filename)
      toast({ title: 'Fichier .FIT téléchargé', description: 'Copie-le dans le dossier GARMIN/NewFiles de ta montre (USB).' })
    } catch (e) {
      toast({ title: 'Téléchargement impossible', description: e instanceof Error ? e.message : undefined, tone: 'error' })
    } finally {
      setDownloading(null)
    }
  }
  return { sendToGarmin, downloadFit, pushing: push.isPending ? push.variables : null, downloading }
}

export function GarminStatus({ s, compact }: { s: PlannedSession; compact?: boolean }) {
  const g = s.garmin
  if (!g || g.status === 'none') return null
  if (g.status === 'pushed')
    return (
      <span className="flex items-center gap-1 text-xs text-ok">
        <CircleCheck className="size-3.5" aria-hidden="true" /> {compact ? 'Sur Garmin' : `Envoyée sur Garmin${g.pushedAt ? ` le ${fmtDateTime(g.pushedAt)}` : ''}`}
      </span>
    )
  if (compact) return null
  return (
    <p className="flex items-start gap-1.5 text-xs text-ink-3">
      <Info className="mt-px size-3.5 flex-none text-info" aria-hidden="true" />
      {g.status === 'unsupported' ? 'Envoi direct indisponible avec ta clé Garmin (API réservée aux comptes business) : utilise le fichier .FIT.' : `Dernier envoi échoué${g.error ? ` : ${g.error}` : '.'}`}
    </p>
  )
}

export function PlannedSessionDetail({ session, onClose, onEdit, onDelete }: { session: PlannedSession | null; onClose: () => void; onEdit: (s: PlannedSession) => void; onDelete: (s: PlannedSession) => void }) {
  const w = useWorkoutActions()
  const [override, setOverride] = useState<{ id: number; g: GarminPush } | null>(null)
  const s = session && override?.id === session.id ? { ...session, garmin: override.g } : session
  const unsupported = s?.garmin?.status === 'unsupported'
  const workout = s && s.sport !== 'rest' && s.sport !== 'strength' && s.sport !== 'mobility'
  return (
    <Dialog
      open={s !== null}
      onClose={onClose}
      size="lg"
      title={s?.title ?? 'Séance'}
      description={s ? `${capitalize(fmtDateLong(s.date))} · ${PLAN_SPORT_LABEL[s.sport]} · ${SOURCE_LABEL[s.source ?? 'manual']}` : undefined}
      footer={
        s && (
          <div className="flex flex-wrap justify-end gap-2 pb-3">
            <button type="button" className="btn btn-ghost" onClick={() => (onClose(), onDelete(s))}>
              <Trash2 className="size-4" aria-hidden="true" /> Supprimer
            </button>
            <button type="button" className="btn btn-ghost" onClick={() => (onClose(), onEdit(s))}>
              <Pencil className="size-4" aria-hidden="true" /> Modifier
            </button>
          </div>
        )
      }
    >
      {s && (
        <div className="space-y-5">
          {targetParts(s).length > 0 && (
            <ul className="flex flex-wrap gap-1.5" aria-label="Cibles">
              {targetParts(s).map((t) => (
                <li key={t} className="num chip border border-line bg-track text-ink-2">
                  {t}
                </li>
              ))}
            </ul>
          )}
          {s.instructions && <p className="text-sm text-ink-2">{s.instructions}</p>}
          {s.steps?.length > 0 ? <StepsView steps={s.steps} /> : <p className="text-sm text-ink-3">Pas de structure détaillée : modifie la séance pour ajouter des étapes.</p>}
          {s.match && s.match.status !== 'upcoming' && (
            <div className="flex flex-wrap items-center gap-2 text-sm text-ink-2">
              <MatchBadge status={s.match.status} /> {s.match.summary}
            </div>
          )}
          {workout && (
            <section aria-labelledby="watch-title" className="rounded-2xl border border-line p-4">
              <h3 id="watch-title" className="flex items-center gap-2 font-semibold text-ink">
                <Watch className="size-4 text-accent" aria-hidden="true" /> Sur ma montre
              </h3>
              <div className="mt-3 flex flex-wrap gap-2">
                <button type="button" className="btn btn-primary" onClick={() => w.sendToGarmin(s, (g) => setOverride({ id: s.id, g }))} disabled={w.pushing === s.id || unsupported} title={unsupported ? 'Indisponible avec ta clé Garmin' : undefined}>
                  {w.pushing === s.id ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Watch className="size-4" aria-hidden="true" />}
                  Envoyer sur Garmin
                </button>
                <button type="button" className="btn btn-ghost" onClick={() => void w.downloadFit(s)} disabled={w.downloading === s.id}>
                  {w.downloading === s.id ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Download className="size-4" aria-hidden="true" />}
                  Télécharger .FIT
                </button>
              </div>
              <div className="mt-3 space-y-1.5">
                <GarminStatus s={s} />
                <p className="text-xs text-ink-3">
                  .FIT : branche la montre en USB et copie le fichier dans <code className="rounded bg-track px-1">GARMIN/NewFiles</code> ; la séance apparaît dans Entraînement › Mes séances.
                </p>
              </div>
            </section>
          )}
        </div>
      )}
    </Dialog>
  )
}
