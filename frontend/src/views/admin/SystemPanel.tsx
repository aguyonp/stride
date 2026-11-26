import { BellRing, Download, Loader2, Watch } from 'lucide-react'
import { useState } from 'react'
import { useAdminMutations, useAdminSettings } from '../../api/queries'
import { Card } from '../../components/ui/Card'
import { Field } from '../../components/ui/Field'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { cn } from '../../lib/cn'
import { ErrorView } from '../StateViews'
import { t } from '../../i18n'

/**
 * Admin › Instance (§21, §22, §23): the settings that belong to the installation itself, not to the
 * accounts. They used to sit in the invitations tab, which read like a mistake — retention, release
 * check and the experimental Garmin bridge have nothing to do with who may join.
 */
export function SystemPanel() {
  return (
    <div className="space-y-5">
      <RetentionCard />
      <UpdatesCard />
      <BridgeCard />
    </div>
  )
}

const RETENTION_DAYS_MAX = 365
const RETENTION_MAX_MAX = 200

/**
 * Notification-centre retention (issue #142): how much history a user keeps. The server clamps the
 * values and purges the surplus as soon as they change.
 */
function RetentionCard() {
  const settings = useAdminSettings()
  const m = useAdminMutations()
  const { toast } = useToast()
  const cur = settings.data
  const [f, setF] = useState({ days: cur?.inboxRetentionDays ?? 0, max: cur?.inboxRetentionMax ?? 0 })
  const [seed, setSeed] = useState('')
  const stamp = cur ? `${cur.inboxRetentionDays}/${cur.inboxRetentionMax}` : ''
  if (cur && seed !== stamp) {
    // The panel follows the server when the stored values change (another admin, a clamp).
    setSeed(stamp)
    setF({ days: cur.inboxRetentionDays, max: cur.inboxRetentionMax })
  }
  const ok = (v: number, max: number) => Number.isInteger(v) && v >= 1 && v <= max
  const valid = ok(f.days, RETENTION_DAYS_MAX) && ok(f.max, RETENTION_MAX_MAX)
  const dirty = cur !== undefined && (f.days !== cur.inboxRetentionDays || f.max !== cur.inboxRetentionMax)
  return (
    <Card title={t('admin.inbox.retention')} icon={<BellRing className="size-4" />} subtitle={t('admin.inbox.retentionHint')}>
      {settings.isError ? (
        <ErrorView error={settings.error} onRetry={() => void settings.refetch()} />
      ) : !cur ? (
        <Skeleton className="h-28 max-w-md" />
      ) : (
        <form
          className="space-y-3.5"
          autoComplete="off"
          onSubmit={(e) => {
            e.preventDefault()
            m.updateSettings.mutate(
              { inboxRetentionDays: f.days, inboxRetentionMax: f.max },
              {
                onSuccess: (s) => toast({ title: t('admin.inbox.saved'), description: t('admin.inbox.savedBody', { days: s.inboxRetentionDays, max: s.inboxRetentionMax }) }),
                onError: (e) => toast({ tone: 'error', title: t('admin.inbox.saveFailed'), description: e.message }),
              },
            )
          }}
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label={t('admin.inbox.days')} hint={t('admin.inbox.daysHint')} error={ok(f.days, RETENTION_DAYS_MAX) ? undefined : t('admin.inbox.daysRange')}>
              {(p) => (
                <input
                  {...p}
                  className="field num"
                  type="number"
                  min={1}
                  max={RETENTION_DAYS_MAX}
                  step={1}
                  inputMode="numeric"
                  value={Number.isFinite(f.days) ? f.days : ''}
                  onChange={(e) => setF((x) => ({ ...x, days: Number(e.target.value) }))}
                />
              )}
            </Field>
            <Field label={t('admin.inbox.max')} hint={t('admin.inbox.maxHint')} error={ok(f.max, RETENTION_MAX_MAX) ? undefined : t('admin.inbox.maxRange')}>
              {(p) => (
                <input
                  {...p}
                  className="field num"
                  type="number"
                  min={1}
                  max={RETENTION_MAX_MAX}
                  step={1}
                  inputMode="numeric"
                  value={Number.isFinite(f.max) ? f.max : ''}
                  onChange={(e) => setF((x) => ({ ...x, max: Number(e.target.value) }))}
                />
              )}
            </Field>
          </div>
          <div className="flex justify-end">
            <button type="submit" className="btn btn-primary" disabled={m.updateSettings.isPending || !dirty || !valid}>
              {m.updateSettings.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
              {t('common.actions.save')}
            </button>
          </div>
        </form>
      )}
    </Card>
  )
}

/**
 * Release check (issue #143): the switch that stops every GitHub call, banner and notice. On by
 * default; an air-gapped instance turns it off and nothing leaves the server.
 */
function UpdatesCard() {
  const settings = useAdminSettings()
  const m = useAdminMutations()
  const cur = settings.data
  const on = cur?.updateCheckEnabled ?? true
  return (
    <Card title={t('admin.updates.card')} icon={<Download className="size-4" />} subtitle={t('admin.updates.hint')}>
      {settings.isError ? (
        <ErrorView error={settings.error} onRetry={() => void settings.refetch()} />
      ) : !cur ? (
        <Skeleton className="h-16 max-w-md" />
      ) : (
        <Toggle
          label={t('admin.updates.check')}
          hint={on ? t('admin.updates.on') : t('admin.updates.off')}
          checked={on}
          disabled={m.updateSettings.isPending}
          onChange={() => m.updateSettings.mutate({ updateCheckEnabled: !on })}
        />
      )}
    </Card>
  )
}

/**
 * Experimental Garmin bridge (issue #147): the admin half of the kill switch. It only opens the
 * door — each user still has to activate the bridge and sign in for their own account.
 */
function BridgeCard() {
  const settings = useAdminSettings()
  const m = useAdminMutations()
  const cur = settings.data
  const on = cur?.garminBridgeEnabled ?? false
  return (
    <Card title={t('admin.bridge.card')} icon={<Watch className="size-4" />} subtitle={t('admin.bridge.hint')}>
      {settings.isError ? (
        <ErrorView error={settings.error} onRetry={() => void settings.refetch()} />
      ) : !cur ? (
        <Skeleton className="h-16 max-w-md" />
      ) : (
        <Toggle
          label={t('admin.bridge.toggle')}
          hint={on ? t('admin.bridge.on') : t('admin.bridge.off')}
          checked={on}
          disabled={m.updateSettings.isPending}
          onChange={() => m.updateSettings.mutate({ garminBridgeEnabled: !on })}
        />
      )}
    </Card>
  )
}

/** One labelled switch, shared by the cards above. */
function Toggle({ label, hint, checked, disabled, onChange }: { label: string; hint: string; checked: boolean; disabled: boolean; onChange: () => void }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div className="min-w-0">
        <p className="text-sm font-medium">{label}</p>
        <p className="text-xs text-ink-3">{hint}</p>
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label}
        disabled={disabled}
        onClick={onChange}
        className={cn('relative h-6 w-11 flex-none rounded-full transition-colors', checked ? 'bg-accent' : 'bg-line-strong')}
      >
        <span className={cn('absolute top-0.5 size-5 rounded-full bg-surface transition-all', checked ? 'left-[1.375rem]' : 'left-0.5')} />
      </button>
    </div>
  )
}
