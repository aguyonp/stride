import type { AdminUser, Invitation } from '../../api/types'
import { cn } from '../../lib/cn'

const PROVIDER_NAME: Record<string, string> = { garmin: 'Garmin', strava: 'Strava', 'open-meteo': 'Open-Meteo' }
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
