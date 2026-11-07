import { Database, Eraser, Loader2, Settings2, ShieldCheck, Sparkles, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useAdminControl, useAdminMutations } from '../../api/queries'
import type { AdminOverview, IntegrityResult, OptimizeResult, PurgeResult } from '../../api/types'
import { Badge } from '../../components/ui/Badge'
import { Card } from '../../components/ui/Card'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { KeyValue } from '../../components/ui/Misc'
import { useToast } from '../../components/ui/Toast'
import { fmtNumber } from '../../lib/format'
import { fmtBytes } from './adminUtils'

const PURGE_LABEL: Record<keyof PurgeResult['removed'], string> = {
  sessions: 'Sessions expirées',
  invitations: 'Invitations closes',
  apiEvents: 'Événements API',
  logs: 'Lignes de journal',
  jobs: 'Tâches terminées',
  shareLinks: 'Liens morts',
}

const yesNo = (b: boolean) => (b ? 'oui' : 'non')

export function MaintenancePanel({ overview }: { overview?: AdminOverview }) {
  const m = useAdminMutations()
  const ctl = useAdminControl()
  const { toast } = useToast()
  const [confirmFlush, setConfirmFlush] = useState(false)
  const [vacuum, setVacuum] = useState(false)
  const [purged, setPurged] = useState<PurgeResult | null>(null)
  const [optimized, setOptimized] = useState<OptimizeResult | null>(null)
  const [checked, setChecked] = useState<IntegrityResult | null>(null)
  const db = optimized?.after ?? overview?.database
  const cfg = overview?.config

  const purge = () =>
    ctl.purge.mutate(undefined, {
      onSuccess: (r) => {
        setPurged(r)
        const total = Object.values(r.removed).reduce((a, b) => a + b, 0)
        toast({ title: 'Purge terminée', description: `${fmtNumber(total)} ligne${total > 1 ? 's' : ''} supprimée${total > 1 ? 's' : ''}.` })
      },
      onError: (e) => toast({ tone: 'error', title: 'Purge impossible', description: e.message }),
    })
  const optimize = () =>
    ctl.optimize.mutate(vacuum, {
      onSuccess: (r) => {
        setOptimized(r)
        const gain = r.before.sizeBytes + r.before.walBytes - (r.after.sizeBytes + r.after.walBytes)
        toast({ title: 'Base optimisée', description: gain > 0 ? `${fmtBytes(gain)} libérés en ${fmtNumber(r.durationMs)} ms.` : `Terminé en ${fmtNumber(r.durationMs)} ms.` })
      },
      onError: (e) => toast({ tone: 'error', title: 'Optimisation impossible', description: e.message }),
    })
  const check = () =>
    ctl.check.mutate(undefined, {
      onSuccess: (r) => setChecked(r),
      onError: (e) => toast({ tone: 'error', title: 'Contrôle impossible', description: e.message }),
    })

  return (
    <div className="grid gap-5 lg:grid-cols-2">
      <Card title="Base SQLite" icon={<Database className="size-4" />} subtitle="Optimisation et contrôle d’intégrité (équivalent de « stride doctor »)" className="lg:col-span-2">
        {db && (
          <dl className="mb-4 grid grid-cols-3 gap-2 sm:max-w-lg">
            {(
              [
                ['Données', db.sizeBytes],
                ['Journal WAL', db.walBytes],
                ['Récupérable', db.freeBytes],
              ] as const
            ).map(([k, v]) => (
              <div key={k} className="glass-inset min-w-0 px-3 py-2">
                <dt className="truncate text-[11px] text-ink-3">{k}</dt>
                <dd className="num truncate text-base font-semibold text-ink">{fmtBytes(v)}</dd>
              </div>
            ))}
          </dl>
        )}
        <div className="flex flex-wrap items-center gap-3">
          <button type="button" className="btn btn-primary" onClick={optimize} disabled={ctl.optimize.isPending}>
            {ctl.optimize.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Sparkles className="size-4" aria-hidden="true" />}
            Optimiser la base
          </button>
          <label className="flex items-center gap-2 text-sm text-ink-2">
            <input type="checkbox" className="size-4 accent-[var(--accent)]" checked={vacuum} onChange={(e) => setVacuum(e.target.checked)} />
            Compacter aussi (VACUUM)
          </label>
          <button type="button" className="btn btn-ghost sm:ml-auto" onClick={check} disabled={ctl.check.isPending}>
            {ctl.check.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <ShieldCheck className="size-4" aria-hidden="true" />}
            Contrôler l’intégrité
          </button>
        </div>
        <p className="mt-2 text-xs text-ink-3">
          L’optimisation vide le journal WAL et met à jour les statistiques du planificateur. VACUUM réécrit tout le fichier : les écritures attendent pendant l’opération et il faut temporairement autant d’espace disque libre que la base.
        </p>
        {checked && (
          <div className="mt-4 border-t border-line pt-3" role="status">
            {checked.ok ? (
              <p className="flex flex-wrap items-center gap-2 text-sm text-ink-2">
                <Badge level="ok">Intègre</Badge>
                integrity_check et foreign_key_check sans anomalie · {fmtNumber(checked.durationMs)} ms
              </p>
            ) : (
              <>
                <p className="mb-2 flex flex-wrap items-center gap-2 text-sm text-ink-2">
                  <Badge level="danger">
                    {checked.total} problème{checked.total > 1 ? 's' : ''}
                  </Badge>
                  Sauvegarde le volume /data avant toute réparation.
                </p>
                <ul className="scroll-thin max-h-48 space-y-1 overflow-y-auto font-mono text-xs break-all text-ink-2">
                  {checked.problems.map((p, i) => (
                    <li key={i}>{p}</li>
                  ))}
                </ul>
              </>
            )}
          </div>
        )}
      </Card>

      <Card title="Données expirées" icon={<Trash2 className="size-4" />} subtitle="Lancée automatiquement chaque heure">
        <p className="text-sm text-ink-2">Supprime les sessions expirées, les invitations closes, les événements API et journaux hors rétention, les tâches terminées et les liens de partage morts depuis 14 jours.</p>
        <button type="button" className="btn btn-ghost mt-4" onClick={purge} disabled={ctl.purge.isPending}>
          {ctl.purge.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Trash2 className="size-4" aria-hidden="true" />}
          Purger maintenant
        </button>
        {purged && (
          <KeyValue
            className="mt-4 border-t border-line pt-3"
            items={(Object.keys(PURGE_LABEL) as (keyof PurgeResult['removed'])[]).map((k) => [PURGE_LABEL[k], <span className="num">{fmtNumber(purged.removed[k] ?? 0)}</span>])}
          />
        )}
      </Card>

      <Card title="Cache" icon={<Eraser className="size-4" />} subtitle={`${cfg?.cacheBackend === 'valkey' ? 'Valkey' : 'Mémoire'} · clés stride:cache:*`}>
        <p className="text-sm text-ink-2">
          Supprime les réponses fournisseurs en cache, le bien-être du jour et les analyses calculées. Les sessions sont stockées en base : personne n’est déconnecté. Les données sont rechargées à la demande (attention aux quotas Garmin / Strava).
        </p>
        <button type="button" className="btn btn-danger mt-4" onClick={() => setConfirmFlush(true)} disabled={m.flushCache.isPending}>
          {m.flushCache.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Eraser className="size-4" aria-hidden="true" />}
          Vider le cache
        </button>
      </Card>

      <Card title="Configuration du serveur" icon={<Settings2 className="size-4" />} subtitle="Lecture seule · variables STRIDE_* (clés, jetons et adresses internes jamais affichés)" className="lg:col-span-2">
        {cfg ? (
          <KeyValue
            className="sm:max-w-2xl"
            items={[
              ['URL publique', <span className="font-mono text-xs break-all">{cfg.publicUrl || '—'}</span>],
              ['Domaine WebAuthn (RP ID)', <span className="font-mono text-xs">{cfg.rpId || '—'}</span>],
              ['Inscriptions', overview?.registrationMode === 'open' ? 'ouvertes' : 'sur invitation'],
              ['Cookies sécurisés (HTTPS)', yesNo(cfg.secureCookies)],
              ['Durée de session', `${fmtNumber(cfg.sessionTtlHours / 24, 1, { trim: true })} j`],
              ['Fuseau horaire', cfg.timezone],
              ['Synchro matinale', `${String(cfg.syncHour).padStart(2, '0')}:00`],
              ['Cache', cfg.cacheBackend === 'valkey' ? 'Valkey' : 'Mémoire (instance unique)'],
              ['Métriques Prometheus', cfg.metricsEnabled ? 'activées (écoute privée)' : 'désactivées'],
              ['Strava', cfg.stravaEnabled ? 'configuré' : 'non configuré'],
              ['Météo', cfg.weatherProvider || 'désactivée'],
              ['Sortie vers réseaux privés', yesNo(cfg.allowPrivateEgress)],
              ['Journaux', `${cfg.logLevel} · ${cfg.logFormat}`],
            ]}
          />
        ) : (
          <p className="text-sm text-ink-3">Chargement de la configuration…</p>
        )}
      </Card>

      <ConfirmDialog
        open={confirmFlush}
        onClose={() => setConfirmFlush(false)}
        title="Vider le cache ?"
        message={<p>Les prochaines vues recalculeront les analyses et pourront appeler Garmin / Strava. Aucune session n’est affectée.</p>}
        confirmLabel="Vider le cache"
        danger
        busy={m.flushCache.isPending}
        onConfirm={() =>
          m.flushCache.mutate(undefined, {
            onSuccess: (r) => {
              setConfirmFlush(false)
              toast({ title: 'Cache vidé', description: `${fmtNumber(r.removed)} clé${r.removed > 1 ? 's' : ''} supprimée${r.removed > 1 ? 's' : ''}.` })
            },
            onError: (e) => toast({ tone: 'error', title: 'Échec du vidage', description: e.message }),
          })
        }
      />
    </div>
  )
}
