import { HistoryImport } from '../../components/sync/HistoryImport'
import { Bike, CloudSun, KeyRound, Loader2, Plug, RefreshCw, ShieldAlert, Unplug, Watch } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useIntegrationMutations, useIntegrations } from '../../api/queries'
import type { Integrations } from '../../api/types'
import { GarminGuide, GarminKeyForm, garminErrorMessage } from '../../components/profile/GarminKeyForm'
import { GarminBridgeCard } from '../../components/profile/GarminBridgeCard'
import { ProviderBadge } from '../../components/profile/ProviderBadge'
import { StravaConnectButton, stravaUnavailable } from '../../components/profile/StravaConnect'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { Dialog } from '../../components/ui/Dialog'
import { KeyValue } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { fmtDateTime, fmtWhen } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { Section } from './Section'
import { lazyRecord, t } from '../../i18n'

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
  return <p className="mt-3 rounded-xl border border-warn/30 bg-warn/10 px-3 py-2 text-xs text-ink-2">{t('settings.integrations.lastError', { message })}</p>
}

export function IntegrationsSection() {
  const integ = useIntegrations()
  return (
    <Section
      id="integrations"
      title={t('settings.integrations.title')}
      icon={<Plug />}
      description={t('settings.integrations.description')}
    >
      {integ.isError ? (
        <ErrorView error={integ.error} onRetry={() => void integ.refetch()} title={t('settings.integrations.unavailable')} />
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
          {(integ.data.garmin.connected || integ.data.strava.connected) && <HistoryImport />}
          <p className="flex items-center gap-2 text-sm text-ink-3">
            <KeyRound className="size-4 flex-none" aria-hidden="true" />
            {t('settings.integrations.privacy')}
          </p>
          <p className="flex items-center gap-2 text-sm text-ink-3">
            <CloudSun className="size-4 flex-none" aria-hidden="true" />
            {integ.data.weather.enabled ? t('settings.integrations.weatherOn', { provider: integ.data.weather.provider || 'Open-Meteo' }) : t('settings.integrations.weatherOff')}
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
  // The four steps of the Personal API key live in a dialog: they used to fill the whole card.
  const [keyOpen, setKeyOpen] = useState(false)

  const test = () =>
    m.testGarmin.mutate(undefined, {
      onSuccess: (v) => {
        if (v && 'ok' in v && v.ok === false) toast({ tone: 'error', title: t('settings.integrations.garminTestFailed'), description: v.message })
        else toast({ title: t('settings.integrations.garminOk') })
      },
      onError: (e) => toast({ tone: 'error', title: t('settings.integrations.garminTestFailed'), description: garminErrorMessage(e) }),
    })

  return (
    <ProviderCard icon={<Watch />} title="Garmin Connect" badge={<ProviderBadge status={g.status} connected={g.connected} />}>
      {/* The bridge comes first: it is the path that sends sessions to the watch, and it does not
          depend on the key below. */}
      <GarminBridgeCard />

      <div className="mt-4 border-t border-line pt-3.5">
        <p className="text-sm font-semibold text-ink">{t('settings.integrations.readTitle')}</p>
        {g.connected ? (
          <>
            <KeyValue
              items={[
                [t('settings.integrations.key'), <span className="font-mono">{g.keyHint ?? '—'}</span>],
                [t('settings.integrations.tokenUntil'), fmtDateTime(g.tokenExpiresAt)],
                [t('settings.integrations.lastSync'), g.lastSyncAt ? fmtWhen(g.lastSyncAt) : t('settings.integrations.never')],
              ]}
            />
            <LastError message={g.lastError} />
            <p className="mt-3 text-xs text-ink-3">{t('settings.integrations.garminLimits')}</p>
            <div className="mt-4 flex flex-wrap gap-2">
              <button type="button" className="btn btn-ghost" onClick={test} disabled={m.testGarmin.isPending}>
                {m.testGarmin.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <RefreshCw className="size-4" aria-hidden="true" />}
                {t('settings.integrations.test')}
              </button>
              <button type="button" className="btn btn-ghost" aria-expanded={replace} onClick={() => setReplace((v) => !v)}>
                {t('settings.integrations.replaceKey')}
              </button>
              <button type="button" className="btn btn-danger" onClick={() => setConfirm(true)}>
                <Unplug className="size-4" aria-hidden="true" />
                {t('settings.integrations.disconnect')}
              </button>
            </div>
            {replace && (
              <div className="mt-4 border-t border-line pt-4">
                <GarminKeyForm
                  submitLabel={t('settings.integrations.replaceKey')}
                  autoFocus
                  onConnected={() => {
                    setReplace(false)
                    toast({ title: t('settings.integrations.newKeySaved') })
                  }}
                />
              </div>
            )}
          </>
        ) : (
          <>
            <LastError message={g.lastError} />
            <p className="mt-1 text-xs text-ink-3">{t('settings.integrations.readPitch')}</p>
            <button type="button" className="btn btn-ghost mt-3" onClick={() => setKeyOpen(true)}>
              <KeyRound className="size-4" aria-hidden="true" />
              {t('settings.integrations.setupKey')}
            </button>
          </>
        )}
      </div>

      {/* The four steps and the field only appear when asked for: the card stays readable. */}
      <Dialog open={keyOpen} onClose={() => setKeyOpen(false)} title={t('settings.integrations.setupKey')} description={t('settings.integrations.readPitch')} size="md">
        <GarminGuide />
        <div className="mt-3">
          <GarminKeyForm
            onConnected={() => {
              setKeyOpen(false)
              toast({ title: t('settings.integrations.garminConnected'), description: t('settings.integrations.syncStarting') })
            }}
          />
        </div>
      </Dialog>
      <ConfirmDialog
        open={confirm}
        onClose={() => setConfirm(false)}
        title={t('settings.integrations.garminDisconnectTitle')}
        message={<p>{t('settings.integrations.garminDisconnectBody')}</p>}
        confirmLabel={t('settings.integrations.disconnect')}
        danger
        busy={m.disconnectGarmin.isPending}
        onConfirm={() =>
          m.disconnectGarmin.mutate(undefined, {
            onSuccess: () => {
              setConfirm(false)
              toast({ title: t('settings.integrations.garminDisconnected') })
            },
            onError: (e) => toast({ tone: 'error', title: t('settings.integrations.disconnectFailed'), description: e.message }),
          })
        }
      />
    </ProviderCard>
  )
}

const SCOPE_LABEL: Record<string, string> = lazyRecord(['read', 'activity:read_all', 'profile:read_all'] as const, (k) => t(`settings.integrations.scopes.${k}`))

function StravaCard({ s }: { s: Integrations['strava'] }) {
  const { toast } = useToast()
  const m = useIntegrationMutations()
  const [confirm, setConfirm] = useState(false)
  const missing = s.missingScopes ?? []
  return (
    <ProviderCard icon={<Bike />} title="Strava" badge={s.available ? <ProviderBadge status={s.status} connected={s.connected} /> : <span className="chip border border-line bg-track text-ink-3">{t('settings.integrations.unavailableChip')}</span>}>
      {!s.available ? (
        <p className="text-sm text-ink-3">{stravaUnavailable()}</p>
      ) : s.connected ? (
        <>
          <KeyValue
            items={[
              [t('settings.integrations.athlete'), s.athleteName ?? '—'],
              [t('settings.integrations.lastSync'), s.lastSyncAt ? fmtWhen(s.lastSyncAt) : t('settings.integrations.never')],
              ...(s.scopes?.length ? ([[t('settings.integrations.permissions'), s.scopes.map((x) => SCOPE_LABEL[x] ?? x).join(', ')]] as [string, string][]) : []),
            ]}
          />
          <LastError message={s.lastError} />
          {missing.length > 0 && (
            <div role="status" className="mt-3 rounded-xl border border-warn/30 bg-warn/[0.08] p-3 text-sm text-ink-2">
              <p className="flex items-center gap-1.5 font-semibold text-ink">
                <ShieldAlert className="size-4 text-warn" aria-hidden="true" /> {t('settings.integrations.stravaScopesTitle')}
              </p>
              <p className="mt-1 text-xs">
                {t('settings.integrations.stravaScopesBody', { scopes: missing.map((x) => SCOPE_LABEL[x] ?? x).join(', ') })}
              </p>
              <div className="mt-2.5">
                <StravaConnectButton strava={s} returnPath="/settings" label={t('settings.integrations.reconnectStrava')} />
              </div>
            </div>
          )}
          <div className="mt-4 flex flex-wrap gap-2">
            <button type="button" className="btn btn-danger" onClick={() => setConfirm(true)}>
              <Unplug className="size-4" aria-hidden="true" />
              {t('settings.integrations.disconnect')}
            </button>
          </div>
        </>
      ) : (
        <div className="space-y-3">
          <LastError message={s.lastError} />
          <p className="text-sm text-ink-2">{t('settings.integrations.stravaPitch')}</p>
          <StravaConnectButton strava={s} returnPath="/settings" />
        </div>
      )}
      <ConfirmDialog
        open={confirm}
        onClose={() => setConfirm(false)}
        title={t('settings.integrations.stravaDisconnectTitle')}
        message={<p>{t('settings.integrations.stravaDisconnectBody')}</p>}
        confirmLabel={t('settings.integrations.disconnect')}
        danger
        busy={m.disconnectStrava.isPending}
        onConfirm={() =>
          m.disconnectStrava.mutate(undefined, {
            onSuccess: () => {
              setConfirm(false)
              toast({ title: t('settings.integrations.stravaDisconnected') })
            },
            onError: (e) => toast({ tone: 'error', title: t('settings.integrations.disconnectFailed'), description: e.message }),
          })
        }
      />
    </ProviderCard>
  )
}
