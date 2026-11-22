import type { AdminIntegration, AdminUser, AlertLevel, Invitation, JobGroup, JobState } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtNumber, fmtPct } from '../../lib/format'
import { lazyRecord, t } from '../../i18n'

const PROVIDER_NAME: Record<string, string> = lazyRecord(['garmin', 'strava', 'open-meteo', 'llm', 'webhook'] as const, (k) =>
  k === 'llm' ? t('admin.common.llm') : { garmin: 'Garmin', strava: 'Strava', 'open-meteo': 'Open-Meteo', webhook: 'Webhooks' }[k],
)
export const providerName = (p: string) => PROVIDER_NAME[p] ?? p

export function userName(users: AdminUser[] | undefined, id: number): string {
  if (id === 0) return t('admin.common.system')
  return users?.find((u) => u.id === id)?.displayName ?? `#${id}`
}

export type InvitationState = 'active' | 'expired' | 'revoked' | 'exhausted'

export function invitationState(inv: Invitation, now = Date.now()): InvitationState {
  if (inv.revoked) return 'revoked'
  if (inv.expiresAt && new Date(inv.expiresAt).getTime() <= now) return 'expired'
  if (inv.uses >= inv.maxUses) return 'exhausted'
  return 'active'
}

const INVITATION_CLS: Record<InvitationState, string> = {
  active: 'border-ok/30 bg-ok/10 text-ok',
  expired: 'border-line bg-track text-ink-3',
  revoked: 'border-danger/30 bg-danger/10 text-danger',
  exhausted: 'border-line bg-track text-ink-2',
}
export const INVITATION_STATE: Record<InvitationState, { label: string; cls: string }> = lazyRecord(['active', 'expired', 'revoked', 'exhausted'] as const, (k) => ({
  label: t(`admin.common.invitation.${k}`),
  cls: INVITATION_CLS[k],
}))

/** Compact switch for table cells (accessible name required). */
export function MiniSwitch({ checked, onChange, label, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: string; disabled?: boolean }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cn('relative h-6 w-11 flex-none rounded-full border transition-colors duration-200 disabled:opacity-45', checked ? 'border-transparent bg-accent' : 'border-line-strong bg-track')}
    >
      <span
        aria-hidden="true"
        className={cn('absolute top-1/2 left-0.5 size-[1.125rem] -translate-y-1/2 rounded-full shadow-sm transition-transform duration-200', checked ? 'translate-x-5 bg-on-accent' : 'translate-x-0 bg-ink-2')}
      />
    </button>
  )
}

/** Vertical gauge of quota usage: a track filled from the bottom, colored by load (accent / warn / danger). */
export function QuotaGauge({ label, used, limit }: { label: string; used: number; limit: number }) {
  const pct = limit > 0 ? Math.min(100, (used / limit) * 100) : 0
  const color = pct >= 90 ? 'var(--danger)' : pct >= 70 ? 'var(--warn)' : 'var(--accent)'
  return (
    <div className="flex flex-col items-center gap-1">
      <div
        className="flex h-24 w-3.5 flex-col justify-end overflow-hidden rounded-full bg-track"
        role="img"
        aria-label={t('admin.common.quotaAria', { label, used, limit, pct: fmtPct(Math.round(pct), 0) })}
      >
        <div className="w-full transition-[height] duration-500" style={{ height: `${pct}%`, background: color }} />
      </div>
      <span className="num text-[11px] font-semibold text-ink-2">
        {used}/{limit}
      </span>
      <span className="text-[11px] text-ink-3">{label}</span>
    </div>
  )
}

/** Binary sizes in the active language's units: « 12,4 Mo » / "12.4 MB". */
export function fmtBytes(n: number): string {
  const units = t('admin.common.bytes').split(' ')
  if (!Number.isFinite(n) || n <= 0) return `0 ${units[0]}`
  const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)))
  const v = n / 1024 ** i
  return `${fmtNumber(v, i === 0 || v >= 100 ? 0 : 1, { trim: true })} ${units[i]}`
}

/** Uptime: « 3 j 4 h », « 5 h 12 min », « 42 s ». */
export function fmtUptime(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds))
  const d = Math.floor(s / 86_400)
  const h = Math.floor((s % 86_400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (d > 0) return t('admin.common.uptimeDays', { d, h })
  if (h > 0) return t('admin.common.uptimeHours', { h, m: String(m).padStart(2, '0') })
  if (m > 0) return t('admin.common.uptimeMinutes', { m })
  return t('admin.common.uptimeSeconds', { s })
}

export const JOB_GROUP_LABEL: Record<JobGroup, string> = lazyRecord(['sync', 'llm', 'webhook'] as const, (k) => t(`admin.common.jobGroup.${k}`))

export const JOB_STATE: Record<JobState, { label: string; level: AlertLevel }> = {
  queued: { level: 'info', get label() { return t('admin.common.jobState.queued') } },
  running: { level: 'info', get label() { return t('admin.common.jobState.running') } },
  retrying: { level: 'warn', get label() { return t('admin.common.jobState.retrying') } },
  completed: { level: 'ok', get label() { return t('admin.common.jobState.completed') } },
  failed: { level: 'danger', get label() { return t('admin.common.jobState.failed') } },
}

export const INTEGRATION_LABEL: Record<AdminIntegration, string> = lazyRecord(['garmin', 'strava', 'llm'] as const, (k) => (k === 'llm' ? t('admin.common.ai') : k === 'garmin' ? 'Garmin' : 'Strava'))

/** Active administrators other than `id` (guards: never leave the instance without an admin). */
export function isLastActiveAdmin(users: AdminUser[], u: AdminUser): boolean {
  return u.role === 'admin' && u.status === 'active' && !users.some((x) => x.id !== u.id && x.role === 'admin' && x.status === 'active')
}

/** Compact single-choice filter for panel toolbars (toggle buttons, current one pressed). */
export function FilterChips<T extends string>({ value, onChange, options, ariaLabel }: { value: T; onChange: (v: T) => void; options: { value: T; label: string; count?: number }[]; ariaLabel: string }) {
  return (
    <div role="group" aria-label={ariaLabel} className="flex flex-wrap gap-1.5">
      {options.map((o) => {
        const on = o.value === value
        return (
          <button
            key={o.value}
            type="button"
            aria-pressed={on}
            onClick={() => onChange(o.value)}
            className={cn('chip h-8 border px-3 transition-colors', on ? 'border-accent/50 bg-accent/15 text-ink' : 'border-line bg-track/60 text-ink-2 hover:text-ink')}
          >
            {o.label}
            {o.count !== undefined && <span className={cn('num text-[11px]', on ? 'text-ink-2' : 'text-ink-3')}>{o.count}</span>}
          </button>
        )
      })}
    </div>
  )
}
