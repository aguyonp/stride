import { ExternalLink, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { goToStravaConnect } from '../../api/client'
import type { Integrations } from '../../api/types'
import { t } from '../../i18n'

export const stravaUnavailable = () => t('settings.providers.stravaUnavailable')

export function StravaConnectButton({ strava, returnPath, label = t('settings.providers.stravaConnect') }: { strava: Integrations['strava'] | undefined; returnPath: string; label?: string }) {
  const [busy, setBusy] = useState(false)
  const disabled = !strava?.available || busy
  return (
    <div>
      <button
        type="button"
        className="btn w-full text-white sm:w-auto"
        style={{ background: disabled ? 'var(--track)' : '#cc3d00', color: disabled ? 'var(--text-3)' : '#fff' }}
        disabled={disabled}
        aria-describedby={strava && !strava.available ? 'strava-unavailable' : undefined}
        onClick={() => {
          setBusy(true)
          void goToStravaConnect(returnPath).catch(() => setBusy(false))
        }}
      >
        {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <ExternalLink className="size-4" aria-hidden="true" />}
        {label}
      </button>
      {strava && !strava.available && (
        <p id="strava-unavailable" className="mt-2 text-xs text-ink-3">
          {stravaUnavailable()}
        </p>
      )}
    </div>
  )
}

export type StravaResult = { status: 'connected' | 'error'; reason: string | null }

/** Reads the `strava` result parameters added by the OAuth callback (no side effect). */
export function readStravaResult(): StravaResult | null {
  const sp = new URLSearchParams(window.location.search)
  const status = sp.get('strava')
  if (status !== 'connected' && status !== 'error') return null
  return { status, reason: sp.get('reason') }
}

export function clearStravaResult() {
  const sp = new URLSearchParams(window.location.search)
  if (!sp.has('strava') && !sp.has('reason')) return
  sp.delete('strava')
  sp.delete('reason')
  const q = sp.toString()
  window.history.replaceState(window.history.state, '', `${window.location.pathname}${q ? `?${q}` : ''}${window.location.hash}`)
}

export function consumeStravaResult(): StravaResult | null {
  const r = readStravaResult()
  if (r) clearStravaResult()
  return r
}
