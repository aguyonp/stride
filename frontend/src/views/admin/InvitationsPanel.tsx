import { Ban, BellRing, Copy, DoorOpen, Download, Loader2, Mail, Plus, TriangleAlert } from 'lucide-react'
import { useId, useState } from 'react'
import { useAdminInvitations, useAdminMutations, useAdminSettings } from '../../api/queries'
import type { Invitation, InvitationCreated, RegistrationMode } from '../../api/types'
import { fmtDay } from '../../components/profile/download'
import { Card } from '../../components/ui/Card'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { Dialog } from '../../components/ui/Dialog'
import { Field } from '../../components/ui/Field'
import { EmptyState, InlineError } from '../../components/ui/Misc'
import { SegmentedControl } from '../../components/ui/SegmentedControl'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { copyText } from '../../lib/clipboard'
import { cn } from '../../lib/cn'
import { fmtDateTime } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { INVITATION_STATE, invitationState } from './adminUtils'
import { t } from '../../i18n'

const EXPIRY = [
  { value: '24', get label() { return t('admin.invitations.expiry.24') } },
  { value: '72', get label() { return t('admin.invitations.expiry.72') } },
  { value: '168', get label() { return t('admin.invitations.expiry.168') } },
  { value: '720', get label() { return t('admin.invitations.expiry.720') } },
]

export function InvitationsPanel() {
  return (
    <div className="space-y-5">
      <RegistrationCard />
      <UpdatesCard />
      <RetentionCard />
      <CreateInvitationCard />
      <InvitationList />
    </div>
  )
}

function RegistrationCard() {
  const settings = useAdminSettings()
  const m = useAdminMutations()
  const { toast } = useToast()
  const mode = settings.data?.registrationMode
  return (
    <Card title={t('admin.invitations.registration')} icon={<DoorOpen className="size-4" />} subtitle={t('admin.invitations.registrationHint')}>
      {settings.isError ? (
        <ErrorView error={settings.error} onRetry={() => void settings.refetch()} />
      ) : !mode ? (
        <Skeleton className="h-11 max-w-md" />
      ) : (
        <>
          <SegmentedControl<RegistrationMode>
            ariaLabel={t('admin.invitations.mode')}
            className="max-w-md"
            value={mode}
            onChange={(v) =>
              m.updateSettings.mutate(
                { registrationMode: v },
                {
                  onSuccess: () => toast({ title: v === 'open' ? t('admin.invitations.opened') : t('admin.invitations.inviteOnly') }),
                onError: (e) => toast({ tone: 'error', title: t('admin.invitations.changeFailed'), description: e.message }),
                },
              )
            }
            options={[
              { value: 'invite', label: t('admin.invitations.invite') },
              { value: 'open', label: t('admin.invitations.open') },
            ]}
          />
          <p className="mt-2 text-sm text-ink-3">
            {mode === 'invite'
              ? t('admin.invitations.inviteHint')
              : t('admin.invitations.openHint')}
          </p>
        </>
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
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <p className="text-sm font-medium">{t('admin.updates.check')}</p>
            <p className="text-xs text-ink-3">{on ? t('admin.updates.on') : t('admin.updates.off')}</p>
          </div>
          <button
            type="button"
            role="switch"
            aria-checked={on}
            aria-label={t('admin.updates.check')}
            disabled={m.updateSettings.isPending}
            onClick={() => m.updateSettings.mutate({ updateCheckEnabled: !on })}
            className={cn('relative h-6 w-11 flex-none rounded-full transition-colors', on ? 'bg-accent' : 'bg-line-strong')}
          >
            <span className={cn('absolute top-0.5 size-5 rounded-full bg-surface transition-all', on ? 'left-[1.375rem]' : 'left-0.5')} />
          </button>
        </div>
      )}
    </Card>
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
                <input {...p} className="field num" type="number" min={1} max={RETENTION_DAYS_MAX} step={1} inputMode="numeric"
                  value={Number.isFinite(f.days) ? f.days : ''} onChange={(e) => setF((x) => ({ ...x, days: Number(e.target.value) }))} />
              )}
            </Field>
            <Field label={t('admin.inbox.max')} hint={t('admin.inbox.maxHint')} error={ok(f.max, RETENTION_MAX_MAX) ? undefined : t('admin.inbox.maxRange')}>
              {(p) => (
                <input {...p} className="field num" type="number" min={1} max={RETENTION_MAX_MAX} step={1} inputMode="numeric"
                  value={Number.isFinite(f.max) ? f.max : ''} onChange={(e) => setF((x) => ({ ...x, max: Number(e.target.value) }))} />
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

function CreateInvitationCard() {
  const ids = useId()
  const m = useAdminMutations()
  const [label, setLabel] = useState('')
  const [maxUses, setMaxUses] = useState('1')
  const [hours, setHours] = useState('168')
  const [created, setCreated] = useState<InvitationCreated | null>(null)
  const uses = Number(maxUses)
  const usesError = !Number.isInteger(uses) || uses < 1 || uses > 100

  const submit = () => {
    if (usesError) return
    m.createInvitation.mutate(
      { label: label.trim() || undefined, maxUses: uses, expiresInHours: Number(hours) },
      {
        onSuccess: (r) => {
          setCreated(r)
          setLabel('')
          setMaxUses('1')
        },
      },
    )
  }

  return (
    <Card title={t('admin.invitations.newTitle')} icon={<Mail className="size-4" />} subtitle={t('admin.invitations.newHint')}>
      <form
        className="grid gap-3 sm:grid-cols-[minmax(0,1.6fr)_minmax(0,0.7fr)_minmax(0,1fr)_auto] sm:items-end"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="min-w-0">
          <label htmlFor={`${ids}-l`} className="mb-1.5 block text-sm font-medium text-ink-2">
            {t('admin.invitations.label')}
          </label>
          <input id={`${ids}-l`} className="field" value={label} maxLength={60} placeholder={t('admin.invitations.labelPlaceholder')} onChange={(e) => setLabel(e.target.value)} />
        </div>
        <div className="min-w-0">
          <label htmlFor={`${ids}-u`} className="mb-1.5 block text-sm font-medium text-ink-2">
            {t('admin.invitations.uses')}
          </label>
          <input id={`${ids}-u`} className="field num" type="number" inputMode="numeric" min={1} max={100} value={maxUses} aria-invalid={usesError ? true : undefined} onChange={(e) => setMaxUses(e.target.value)} />
        </div>
        <div className="min-w-0">
          <label htmlFor={`${ids}-h`} className="mb-1.5 block text-sm font-medium text-ink-2">
            {t('admin.invitations.expiresAfter')}
          </label>
          <select id={`${ids}-h`} className="field" value={hours} onChange={(e) => setHours(e.target.value)}>
            {EXPIRY.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
        <button type="submit" className="btn btn-primary min-h-11" disabled={usesError || m.createInvitation.isPending}>
          {m.createInvitation.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Plus className="size-4" aria-hidden="true" />}
          {t('admin.invitations.create')}
        </button>
      </form>
      {usesError && <p className="mt-2 text-xs text-danger">{t('admin.invitations.usesError')}</p>}
      {m.createInvitation.isError && <InlineError className="mt-3">{m.createInvitation.error.message}</InlineError>}
      <CreatedDialog created={created} onClose={() => setCreated(null)} />
    </Card>
  )
}

function CopyField({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  const id = useId()
  const { toast } = useToast()
  return (
    <div>
      <label htmlFor={id} className="mb-1.5 block text-sm font-medium text-ink-2">
        {label}
      </label>
      <div className="flex gap-2">
        <input id={id} readOnly value={value} className={cn('field min-w-0 flex-1', mono && 'font-mono')} onFocus={(e) => e.currentTarget.select()} />
        <button
          type="button"
          className="btn btn-ghost flex-none"
          onClick={async () => {
            const ok = await copyText(value)
            toast(ok ? { title: t('admin.invitations.copied', { label }) } : { tone: 'error', title: t('admin.invitations.copyFailed'), description: t('admin.invitations.copyFailedHint') })
          }}
        >
          <Copy className="size-4" aria-hidden="true" />
          <span className="hidden sm:inline">{t('admin.invitations.copy')}</span>
          <span className="sr-only sm:hidden">{t('common.extra.copyLabel', { label: label.toLowerCase() })}</span>
        </button>
      </div>
    </div>
  )
}

function CreatedDialog({ created, onClose }: { created: InvitationCreated | null; onClose: () => void }) {
  return (
    <Dialog
      open={created !== null}
      onClose={onClose}
      title={t('admin.invitations.created')}
      description={created?.invitation.label || undefined}
      footer={
        <div className="flex justify-end">
          <button type="button" className="btn btn-primary" onClick={onClose}>
            {t('admin.invitations.copiedLink')}
          </button>
        </div>
      }
    >
      {created && (
        <div className="space-y-4">
          <p className="flex items-start gap-2 rounded-xl border border-warn/35 bg-warn/10 px-3 py-2.5 text-sm text-ink">
            <TriangleAlert className="mt-0.5 size-4 flex-none text-warn" aria-hidden="true" />
            {t('admin.invitations.onceHint')}
          </p>
          <CopyField label={t('admin.invitations.link')} value={created.link} />
          <CopyField label={t('admin.invitations.code')} value={created.code} mono />
          <p className="num text-xs text-ink-3">
            {created.invitation.maxUses} utilisation{created.invitation.maxUses > 1 ? 's' : ''} · expire {created.invitation.expiresAt ? `le ${fmtDateTime(created.invitation.expiresAt)}` : 'jamais'}
          </p>
        </div>
      )}
    </Dialog>
  )
}

function InvitationList() {
  const list = useAdminInvitations()
  const m = useAdminMutations()
  const { toast } = useToast()
  const [revoke, setRevoke] = useState<Invitation | null>(null)
  const items = [...(list.data ?? [])].sort((a, b) => b.createdAt.localeCompare(a.createdAt))

  const badge = (inv: Invitation) => {
    const st = INVITATION_STATE[invitationState(inv)]
    return <span className={cn('chip border', st.cls)}>{st.label}</span>
  }
  const revokeBtn = (inv: Invitation) =>
    invitationState(inv) === 'active' ? (
      <button type="button" className="btn btn-ghost min-h-9 px-3 text-xs" onClick={() => setRevoke(inv)}>
        <Ban className="size-3.5" aria-hidden="true" /> {t('admin.invitations.revoke')}
      </button>
    ) : null

  return (
    <Card title={t('admin.invitations.title')} icon={<Mail className="size-4" />} subtitle={list.data ? t('admin.invitations.activeCount', { count: items.filter((i) => invitationState(i) === 'active').length }) : undefined}>
      {list.isError ? (
        <ErrorView error={list.error} onRetry={() => void list.refetch()} />
      ) : !list.data ? (
        <Skeleton className="h-24" />
      ) : items.length === 0 ? (
        <EmptyState icon={<Mail />} title={t('admin.invitations.none')}>
          {t('admin.invitations.noneHint')}
        </EmptyState>
      ) : (
        <>
          <div className="scroll-thin hidden overflow-x-auto md:block">
            <table className="w-full min-w-[680px] border-separate border-spacing-0 text-sm">
              <caption className="sr-only">{t('admin.invitations.caption')}</caption>
              <thead>
                <tr className="text-left text-xs text-ink-3">
                  {[...(['label', 'code', 'uses', 'created', 'expires', 'status'] as const).map((k) => t(`admin.invitations.heads.${k}`)), ''].map((h, i) => (
                    <th key={i} scope="col" className="border-b border-line px-2 py-2 font-medium first:pl-1">
                      {h || <span className="sr-only">{t('admin.invitations.actions')}</span>}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {items.map((inv) => (
                  <tr key={inv.id} className="hover:bg-track/50">
                    <th scope="row" className="max-w-[14rem] truncate border-b border-line px-2 py-2 pl-1 text-left font-semibold text-ink">
                      {inv.label || <span className="font-normal text-ink-3">{t('admin.invitations.noLabel')}</span>}
                    </th>
                    <td className="border-b border-line px-2 py-2 font-mono text-ink-2">{inv.codePrefix}…</td>
                    <td className="num border-b border-line px-2 py-2 text-ink-2">
                      {inv.uses} / {inv.maxUses}
                    </td>
                    <td className="num border-b border-line px-2 py-2 text-ink-2">{fmtDay(inv.createdAt)}</td>
                    <td className="num border-b border-line px-2 py-2 text-ink-2">{inv.expiresAt ? fmtDateTime(inv.expiresAt) : t('admin.invitations.never')}</td>
                    <td className="border-b border-line px-2 py-2">{badge(inv)}</td>
                    <td className="border-b border-line px-2 py-2 text-right">{revokeBtn(inv)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <ul className="space-y-2 md:hidden">
            {items.map((inv) => (
              <li key={inv.id} className="glass-inset p-3">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <p className="truncate font-semibold text-ink">{inv.label || t('admin.invitations.noLabel')}</p>
                    <p className="num text-xs text-ink-3">
                      <span className="font-mono">{inv.codePrefix}…</span> · {inv.uses}/{inv.maxUses}
                      {t('admin.invitations.expiresLine', { date: inv.expiresAt ? fmtDateTime(inv.expiresAt) : t('admin.invitations.never') })}
                    </p>
                  </div>
                  {badge(inv)}
                </div>
                {revokeBtn(inv) && <div className="mt-2">{revokeBtn(inv)}</div>}
              </li>
            ))}
          </ul>
        </>
      )}
      <ConfirmDialog
        open={revoke !== null}
        onClose={() => setRevoke(null)}
        title={t('admin.invitations.revokeTitle')}
        message={<p>{t('admin.invitations.revokeBody', { label: revoke?.label || revoke?.codePrefix })}</p>}
        confirmLabel={t('admin.invitations.revoke')}
        danger
        busy={m.revokeInvitation.isPending}
        onConfirm={() =>
          revoke &&
          m.revokeInvitation.mutate(revoke.id, {
            onSuccess: () => {
              setRevoke(null)
              toast({ title: t('admin.invitations.revoked') })
            },
            onError: (e) => toast({ tone: 'error', title: t('admin.invitations.revokeFailed'), description: e.message }),
          })
        }
      />
    </Card>
  )
}
