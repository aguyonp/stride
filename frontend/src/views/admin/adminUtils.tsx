import type { AdminIntegration, AdminUser, AlertLevel, Invitation, JobGroup, JobState } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtNumber } from '../../lib/format'

const PROVIDER_NAME: Record<string, string> = { garmin: 'Garmin', strava: 'Strava', 'open-meteo': 'Open-Meteo', llm: 'IA (LLM)', webhook: 'Webhooks' }
export const providerName = (p: string) => PROVIDER_NAME[p] ?? p

export function userName(users: AdminUser[] | undefined, id: number): string {
  if (id === 0) return 'Système'
  return users?.find((u) => u.id === id)?.displayName ?? `#${id}`
}

export type InvitationState = 'active' | 'expired' | 'revoked' | 'exhausted'

export function invitationState(inv: Invitation, now = Date.now()): InvitationState {
  if (inv.revoked) return 'revoked'
  if (inv.expiresAt && new Date(inv.expiresAt).getTime() <= now) return 'expired'
  if (inv.uses >= inv.maxUses) return 'exhausted'
  return 'active'
}

export const INVITATION_STATE: Record<InvitationState, { label: string; cls: string }> = {
  active: { label: 'Active', cls: 'border-ok/30 bg-ok/10 text-ok' },
  expired: { label: 'Expirée', cls: 'border-line bg-track text-ink-3' },
  revoked: { label: 'Révoquée', cls: 'border-danger/30 bg-danger/10 text-danger' },
  exhausted: { label: 'Épuisée', cls: 'border-line bg-track text-ink-2' },
}

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

export function UsageBar({ label, used, limit }: { label: string; used: number; limit: number }) {
  const pct = limit > 0 ? Math.min(100, (used / limit) * 100) : 0
  const color = pct >= 90 ? 'var(--danger)' : pct >= 70 ? 'var(--warn)' : 'var(--accent)'
  return (
    <div>
      <div className="mb-1 flex items-baseline justify-between gap-2 text-xs">
        <span className="text-ink-3">{label}</span>
        <span className="num font-semibold text-ink">
          {used} / {limit}
        </span>
      </div>
      <div className="h-2 overflow-hidden rounded-full bg-track" role="img" aria-label={`${label} : ${used} sur ${limit} (${Math.round(pct)} %)`}>
        <div className="h-full rounded-full transition-[width] duration-500" style={{ width: `${pct}%`, background: color }} />
      </div>
    </div>
  )
}

/** Binary sizes in French units: « 12,4 Mo ». */
export function fmtBytes(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '0 o'
  const units = ['o', 'Ko', 'Mo', 'Go', 'To']
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
  if (d > 0) return `${d} j ${h} h`
  if (h > 0) return `${h} h ${String(m).padStart(2, '0')} min`
  if (m > 0) return `${m} min`
  return `${s} s`
}

export const JOB_GROUP_LABEL: Record<JobGroup, string> = { sync: 'Synchro & imports', llm: 'Coach IA', webhook: 'Notifications' }

export const JOB_STATE: Record<JobState, { label: string; level: AlertLevel }> = {
  queued: { label: 'En attente', level: 'info' },
  running: { label: 'En cours', level: 'info' },
  retrying: { label: 'Nouvel essai', level: 'warn' },
  completed: { label: 'Terminée', level: 'ok' },
  failed: { label: 'Échec', level: 'danger' },
}

export const INTEGRATION_LABEL: Record<AdminIntegration, string> = { garmin: 'Garmin', strava: 'Strava', llm: 'IA' }

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
