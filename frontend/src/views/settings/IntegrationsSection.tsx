import { Bike, CloudSun, Loader2, Plug, RefreshCw, Unplug, Watch } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useIntegrationMutations, useIntegrations } from '../../api/queries'
import type { Integrations } from '../../api/types'
import { GarminGuide, GarminKeyForm, garminErrorMessage } from '../../components/profile/GarminKeyForm'
import { ProviderBadge } from '../../components/profile/ProviderBadge'
import { StravaConnectButton, STRAVA_UNAVAILABLE } from '../../components/profile/StravaConnect'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { KeyValue } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { fmtDateTime, fmtWhen } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { Section } from './Section'

function ProviderCard({ icon, title, badge, children }: { icon: ReactNode; title: string; badge: ReactNode; children: ReactNode }) {
  return (
    <div className="glass-inset min-w-0 p-4">
      <div className="mb-3 flex items-center justify-between gap-3">
        <p className="flex min-w-0 items-center gap-2.5 font-semibold text-ink">
          <span className="grid size-8 flex-none place-items-center rounded-xl bg-track text-ink-2 [&>svg]:size-4" aria-hidden="true">
            {icon}
          </span>
          <span className="truncate">{title}</span>
        </p>
        {badge}
      </div>
      {children}
    </div>
  )
}

function LastError({ message }: { message: string | null }) {
  if (!message) return null
  return <p className="mt-3 rounded-xl border border-warn/30 bg-warn/10 px-3 py-2 text-xs text-ink-2">Dernière erreur : {message}</p>
}

export function IntegrationsSection() {
  const integ = useIntegrations()
  return (
    <Section id="integrations" title="Intégrations" icon={<Plug />} description="Garmin Connect (clé Personal API), Strava (OAuth) et météo. Tu peux connecter l’un, l’autre ou les deux.">
      {integ.isError ? (
        <ErrorView error={integ.error} onRetry={() => void integ.refetch()} title="Intégrations indisponibles" />
      ) : !integ.data ? (
        <div className="grid gap-3 md:grid-cols-2">
          <Skeleton className="h-48" />
          <Skeleton className="h-48" />
        </div>
      ) : (
        <div className="space-y-3">
          <div className="grid gap-3 md:grid-cols-2">
            <GarminCard g={integ.data.garmin} />
            <StravaCard s={integ.data.strava} />
          </div>
          <p className="flex items-center gap-2 text-sm text-ink-3">
            <CloudSun className="size-4 flex-none" aria-hidden="true" />
            Météo des séances : {integ.data.weather.enabled ? `${integ.data.weather.provider || 'Open-Meteo'} (activée, sans clé)` : 'désactivée sur ce serveur'}
          </p>
        </div>
      )}
    </Section>
  )
}

function GarminCard({ g }: { g: Integrations['garmin'] }) {
  const { toast } = useToast()
  const m = useIntegrationMutations()
  const [replace, setReplace] = useState(false)
  const [confirm, setConfirm] = useState(false)

  const test = () =>
    m.testGarmin.mutate(undefined, {
      onSuccess: (v) => {
        if (v && 'ok' in v && v.ok === false) toast({ tone: 'error', title: 'Test Garmin échoué', description: v.message })
        else toast({ title: 'Connexion Garmin OK' })
      },
      onError: (e) => toast({ tone: 'error', title: 'Test Garmin échoué', description: garminErrorMessage(e) }),
    })

  return (
    <ProviderCard icon={<Watch />} title="Garmin Connect" badge={<ProviderBadge status={g.status} connected={g.connected} />}>
      {g.connected ? (
        <>
          <KeyValue
            items={[
              ['Clé', <span className="font-mono">{g.keyHint ?? '—'}</span>],
              ['Jeton valide jusqu’au', fmtDateTime(g.tokenExpiresAt)],
              ['Dernière synchro', g.lastSyncAt ? fmtWhen(g.lastSyncAt) : 'jamais'],
            ]}
          />
          <LastError message={g.lastError} />
          <div className="mt-4 flex flex-wrap gap-2">
            <button type="button" className="btn btn-ghost" onClick={test} disabled={m.testGarmin.isPending}>
              {m.testGarmin.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <RefreshCw className="size-4" aria-hidden="true" />}
              Tester
            </button>
            <button type="button" className="btn btn-ghost" aria-expanded={replace} onClick={() => setReplace((v) => !v)}>
              Remplacer la clé
            </button>
            <button type="button" className="btn btn-danger" onClick={() => setConfirm(true)}>
              <Unplug className="size-4" aria-hidden="true" />
              Déconnecter
            </button>
          </div>
          {replace && (
            <div className="mt-4 border-t border-line pt-4">
              <GarminKeyForm
                submitLabel="Remplacer la clé"
                autoFocus
                onConnected={() => {
                  setReplace(false)
                  toast({ title: 'Nouvelle clé Garmin enregistrée' })
                }}
              />
            </div>
          )}
        </>
      ) : (
        <div className="space-y-4">
          <LastError message={g.lastError} />
          <GarminGuide />
          <GarminKeyForm onConnected={() => toast({ title: 'Garmin connecté', description: 'La synchronisation démarre.' })} />
        </div>
      )}
      <ConfirmDialog
        open={confirm}
        onClose={() => setConfirm(false)}
        title="Déconnecter Garmin ?"
        message={<p>La clé chiffrée et les jetons Garmin seront supprimés. Les activités déjà enregistrées restent disponibles.</p>}
        confirmLabel="Déconnecter"
        danger
        busy={m.disconnectGarmin.isPending}
        onConfirm={() =>
          m.disconnectGarmin.mutate(undefined, {
            onSuccess: () => {
              setConfirm(false)
              toast({ title: 'Garmin déconnecté' })
            },
            onError: (e) => toast({ tone: 'error', title: 'Déconnexion impossible', description: e.message }),
          })
        }
      />
    </ProviderCard>
  )
}

function StravaCard({ s }: { s: Integrations['strava'] }) {
  const { toast } = useToast()
  const m = useIntegrationMutations()
  const [confirm, setConfirm] = useState(false)
  return (
    <ProviderCard icon={<Bike />} title="Strava" badge={s.available ? <ProviderBadge status={s.status} connected={s.connected} /> : <span className="chip border border-line bg-track text-ink-3">Indisponible</span>}>
      {!s.available ? (
        <p className="text-sm text-ink-3">{STRAVA_UNAVAILABLE}</p>
      ) : s.connected ? (
        <>
          <KeyValue
            items={[
              ['Athlète', s.athleteName ?? '—'],
              ['Dernière synchro', s.lastSyncAt ? fmtWhen(s.lastSyncAt) : 'jamais'],
            ]}
          />
          <LastError message={s.lastError} />
          <div className="mt-4 flex flex-wrap gap-2">
            <button type="button" className="btn btn-danger" onClick={() => setConfirm(true)}>
              <Unplug className="size-4" aria-hidden="true" />
              Déconnecter
            </button>
          </div>
        </>
      ) : (
        <div className="space-y-3">
          <LastError message={s.lastError} />
          <p className="text-sm text-ink-2">Titre, description, RPE, Relative Effort et chaussures Strava enrichissent tes séances (fusion automatique avec Garmin).</p>
          <StravaConnectButton strava={s} returnPath="/settings" />
        </div>
      )}
      <ConfirmDialog
        open={confirm}
        onClose={() => setConfirm(false)}
        title="Déconnecter Strava ?"
        message={<p>Stride révoque son accès Strava et supprime les jetons. Les séances déjà fusionnées restent enregistrées.</p>}
        confirmLabel="Déconnecter"
        danger
        busy={m.disconnectStrava.isPending}
        onConfirm={() =>
          m.disconnectStrava.mutate(undefined, {
            onSuccess: () => {
              setConfirm(false)
              toast({ title: 'Strava déconnecté' })
            },
            onError: (e) => toast({ tone: 'error', title: 'Déconnexion impossible', description: e.message }),
          })
        }
      />
    </ProviderCard>
  )
}
