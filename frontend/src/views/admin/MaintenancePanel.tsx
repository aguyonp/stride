import { Eraser, Loader2, Wrench } from 'lucide-react'
import { useState } from 'react'
import { useAdminMutations } from '../../api/queries'
import { Card } from '../../components/ui/Card'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { useToast } from '../../components/ui/Toast'
import { fmtNumber } from '../../lib/format'

export function MaintenancePanel() {
  const m = useAdminMutations()
  const { toast } = useToast()
  const [confirm, setConfirm] = useState(false)
  return (
    <div className="space-y-5">
      <Card title="Cache" icon={<Eraser className="size-4" />} subtitle="Valkey · clés stride:cache:*">
        <p className="text-sm text-ink-2">
          Supprime les réponses fournisseurs en cache, le bien-être du jour et les analyses calculées. Les sessions sont stockées en base : personne n’est déconnecté. Les données sont rechargées à la demande (attention aux quotas Garmin / Strava).
        </p>
        <button type="button" className="btn btn-danger mt-4" onClick={() => setConfirm(true)} disabled={m.flushCache.isPending}>
          {m.flushCache.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Eraser className="size-4" aria-hidden="true" />}
          Vider le cache
        </button>
      </Card>
      <Card title="Resynchroniser un utilisateur" icon={<Wrench className="size-4" />}>
        <p className="text-sm text-ink-2">Depuis l’onglet Utilisateurs, le menu « Resynchroniser » vide le cache de la personne, supprime ses données non définitives et relance une synchronisation.</p>
      </Card>
      <ConfirmDialog
        open={confirm}
        onClose={() => setConfirm(false)}
        title="Vider le cache ?"
        message={<p>Les prochaines vues recalculeront les analyses et pourront appeler Garmin / Strava. Aucune session n’est affectée.</p>}
        confirmLabel="Vider le cache"
        danger
        busy={m.flushCache.isPending}
        onConfirm={() =>
          m.flushCache.mutate(undefined, {
            onSuccess: (r) => {
              setConfirm(false)
              toast({ title: 'Cache vidé', description: `${fmtNumber(r.removed)} clé${r.removed > 1 ? 's' : ''} supprimée${r.removed > 1 ? 's' : ''}.` })
            },
            onError: (e) => toast({ tone: 'error', title: 'Échec du vidage', description: e.message }),
          })
        }
      />
    </div>
  )
}
