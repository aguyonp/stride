import { AlertTriangle, CheckCircle2, Loader2, ShieldAlert, Unplug } from 'lucide-react'
import { useState } from 'react'
import { isApiError } from '../../api/client'
import { useGarminBridge, useGarminBridgeMutations } from '../../api/queries'
import { cn } from '../../lib/cn'
import { fmtDateTime } from '../../lib/format'
import { t } from '../../i18n'
import { Field } from '../ui/Field'
import { Dialog } from '../ui/Dialog'
import { InlineError } from '../ui/Misc'
import { Skeleton } from '../ui/Skeleton'
import { useToast } from '../ui/Toast'

/**
 * Experimental Garmin Connect bridge (§23, issue #147).
 *
 * The card is only rendered when the instance allows the bridge (env switch); the switch itself is
 * the user's own consent, behind a warning nobody can miss. The sign-in happens here — Garmin's SSO,
 * then a code — and the password leaves the form for one request, never stored, never logged.
 */
export function GarminBridgeCard() {
  const bridge = useGarminBridge()
  const m = useGarminBridgeMutations()
  const { toast } = useToast()
  const [confirm, setConfirm] = useState(false)
  const [form, setForm] = useState({ email: '', password: '' })
  const [mfa, setMfa] = useState('')
  // The failure is shown in the card, not only in a toast: a wrong password must say « wrong
  // password » under the field, and a code step must be reachable the instant Garmin asks for it.
  const [error, setError] = useState('')
  const [awaitingCode, setAwaitingCode] = useState(false)

  // Nothing is rendered until the instance answers, and nothing is offered when the administrator
  // has not opened the bridge: `available` is the instance's word, not a guess from an HTTP status.
  if (!bridge.data?.available) return null
  const st = bridge.data
  // A 202 says « code required » before the poll comes back: the form must follow the answer, not
  // wait for the next GET.
  const state = awaitingCode ? 'mfa' : (st?.state ?? 'disconnected')
  const busy = m.login.isPending || m.mfa.isPending

  const login = () => {
    m.login.mutate(form, {
      onSuccess: (r) => {
        // The password is dropped as soon as the answer arrives.
        setForm({ email: form.email, password: '' })
        setError('')
        if (r.state === 'mfa') {
          setAwaitingCode(true)
          toast({ title: t('settings.bridge.codePendingTitle'), description: t('settings.bridge.codePendingBody') })
          return
        }
        setAwaitingCode(false)
        if (r.state === 'connected') toast({ title: t('settings.bridge.connected'), description: r.displayName, tone: 'success' })
      },
      onError: (e) => {
        setForm((f) => ({ ...f, password: '' }))
        // Always in the card: a 400 is the most common answer and used to say nothing at all.
        setError(bridgeErrorText(e))
        // The toast keeps the server's own wording: it names the failing step, which is what makes a
        // report actionable.
        if (!isApiError(e, 'BAD_REQUEST')) toast({ title: t('settings.bridge.failedTitle'), description: e.message, tone: 'error' })
      },
    })
  }

  const sendCode = () => {
    m.mfa.mutate(mfa, {
      onSuccess: (r) => {
        setMfa('')
        setError('')
        if (r.state === 'connected') {
          setAwaitingCode(false)
          toast({ title: t('settings.bridge.connected'), description: r.displayName, tone: 'success' })
        }
      },
      onError: (e) => {
        setMfa('')
        setError(bridgeErrorText(e))
        toast({ title: t('settings.bridge.codeRefused'), description: e.message, tone: 'error' })
      },
    })
  }

  return (
    <div role="group" aria-label={t('settings.bridge.title')} className="mt-4 rounded-2xl border border-warn/30 bg-warn/[0.06] p-3.5">
      <div className="flex items-start gap-2.5">
        <ShieldAlert className="mt-0.5 size-4 flex-none text-warn" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-ink">{t('settings.bridge.title')}</p>
          <p className="mt-1 text-xs text-ink-2">{t('settings.bridge.warning')}</p>
        </div>
      </div>

      {bridge.isError ? (
        <InlineError className="mt-3">{t('settings.bridge.statusFailed')}</InlineError>
      ) : !st ? (
        <Skeleton className="mt-3 h-9 max-w-sm" />
      ) : (
        <div className="mt-3">
          <div className="flex flex-wrap items-center gap-2" role="status">
            <StateBadge state={state} locked={st.locked} />
            {state === 'connected' && st.displayName && <span className="text-xs text-ink-2">{st.displayName}</span>}
            {state === 'connected' && st.since && (
              <span className="text-xs text-ink-3">{t('settings.bridge.since', { date: fmtDateTime(st.since) })}</span>
            )}
            {state === 'connected' && (
              <button
                type="button"
                className="btn btn-ghost ml-auto h-8 px-2.5 text-xs"
                onClick={() => m.disconnect.mutate(undefined, { onSuccess: () => toast({ title: t('settings.bridge.disconnected') }) })}
                disabled={m.disconnect.isPending}
              >
                <Unplug className="size-3.5" aria-hidden="true" /> {t('settings.bridge.disconnect')}
              </button>
            )}
          </div>

          {/* Consent first: the warning is read before anything can be typed. */}
          {state === 'disconnected' && !confirm && (
            <button type="button" className="btn btn-ghost mt-3 h-9 px-3 text-sm" onClick={() => setConfirm(true)}>
              {t('settings.bridge.enable')}
            </button>
          )}

          {state === 'disconnected' && confirm && (
            <div className="mt-3 space-y-3">
              <div role="alert" className="rounded-xl border border-warn/35 bg-warn/[0.08] p-3 text-xs text-ink-2">
                <p className="font-semibold text-ink">{t('settings.bridge.consentTitle')}</p>
                <ul className="mt-1.5 list-inside list-disc space-y-0.5">
                  <li>{t('settings.bridge.consentApi')}</li>
                  <li>{t('settings.bridge.consentRisk')}</li>
                  <li>{t('settings.bridge.consentPassword')}</li>
                  <li>{t('settings.bridge.consentFallback')}</li>
                </ul>
              </div>
              <form
                className="grid gap-2.5 sm:grid-cols-2"
                autoComplete="off"
                onSubmit={(e) => {
                  e.preventDefault()
                  if (form.email && form.password) login()
                }}
              >
                <Field label={t('settings.bridge.email')}>
                  {(p) => (
                    <input {...p} className="field" type="email" autoComplete="off" value={form.email} onChange={(e) => {
                      setError('')
                      setForm((f) => ({ ...f, email: e.target.value }))
                    }} />
                  )}
                </Field>
                <Field label={t('settings.bridge.password')} hint={t('settings.bridge.passwordHint')}>
                  {(p) => (
                    <input {...p} className="field" type="password" autoComplete="new-password" value={form.password} onChange={(e) => {
                      setError('')
                      setForm((f) => ({ ...f, password: e.target.value }))
                    }} />
                  )}
                </Field>
                {error && (
                  <p role="alert" className="rounded-xl border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger sm:col-span-2">
                    {error}
                  </p>
                )}
                <div className="flex gap-2 sm:col-span-2">
                  <button type="submit" className="btn btn-primary h-9 px-3 text-sm" disabled={busy || !form.email || !form.password}>
                    {m.login.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
                    {t('settings.bridge.signIn')}
                  </button>
                  <button type="button" className="btn btn-ghost h-9 px-3 text-sm" onClick={() => setConfirm(false)} disabled={busy}>
                    {t('common.actions.cancel')}
                  </button>
                </div>
              </form>
            </div>
          )}

          {state === 'mfa' && (
            <form
              className="mt-3 space-y-2.5"
              onSubmit={(e) => {
                e.preventDefault()
                if (mfa) sendCode()
              }}
            >
              <p className="text-xs text-ink-2">{t('settings.bridge.mfaHint')}</p>
              <Field label={t('settings.bridge.code')}>
                {(p) => (
                  <input {...p} className="field num max-w-40" inputMode="numeric" autoComplete="one-time-code" value={mfa} onChange={(e) => {
                      setError('')
                      setMfa(e.target.value.replace(/\D/g, ''))
                    }} />
                )}
              </Field>
              {error && (
                <p role="alert" className="rounded-xl border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger">
                  {error}
                </p>
              )}
              <div className="flex gap-2">
                <button type="submit" className="btn btn-primary h-9 px-3 text-sm" disabled={busy || mfa.length < 4}>
                  {m.mfa.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
                  {t('settings.bridge.verify')}
                </button>
                <button
                  type="button"
                  className="btn btn-ghost h-9 px-3 text-sm"
                  onClick={() => {
                    setConfirm(true)
                    setMfa('')
                    setError('')
                    setAwaitingCode(false)
                  }}
                  disabled={busy}
                >
                  {t('settings.bridge.restart')}
                </button>
              </div>
            </form>
          )}

          {(state === 'expired' || state === 'blocked') && (
            <div className="mt-3 space-y-2.5">
              <p role="alert" className="rounded-xl border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger">
                {state === 'expired' ? t('settings.bridge.expiredBody') : t('settings.bridge.blockedBody')}
              </p>
              <button type="button" className="btn btn-ghost h-9 px-3 text-sm" onClick={() => setConfirm(true)}>
                {t('settings.bridge.reconnect')}
              </button>
            </div>
          )}
        </div>
      )}

      <p className="mt-3 text-xs text-ink-3">{t('settings.bridge.fallback')}</p>

      <Dialog open={confirm && state === 'connected'} onClose={() => setConfirm(false)} title={t('settings.bridge.reconnectTitle')} size="md">
        <p className="text-sm text-ink-2">{t('settings.bridge.reconnectBody')}</p>
      </Dialog>
    </div>
  )
}

// bridgeErrorText translates the stable HTTP code into a sentence the user can act on. The card
// owns these words because it knows the domain: the server's message is French, names the failing
// step, and belongs in the toast and the log.
function bridgeErrorText(e: unknown): string {
  if (isApiError(e, 'BAD_REQUEST')) return t('settings.bridge.badCredentials')
  if (isApiError(e, 'RATE_LIMITED')) return t('settings.bridge.blockedBody')
  if (isApiError(e, 'PROVIDER_UNAVAILABLE')) return t('settings.bridge.flowFailed')
  return e instanceof Error ? e.message : t('settings.bridge.failedTitle')
}

function StateBadge({ state, locked }: { state: string; locked?: boolean }) {
  const tone =
    state === 'connected'
      ? 'text-ok bg-ok/12 ring-ok/25'
      : state === 'expired' || state === 'blocked'
        ? 'text-danger bg-danger/12 ring-danger/25'
        : 'text-ink-2 bg-track ring-line'
  const label = locked ? t('settings.bridge.state.locked') : t(`settings.bridge.state.${state}` as 'settings.bridge.state.disconnected')
  return (
    <span className={cn('inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ring-1 ring-inset', tone)}>
      {state === 'connected' ? <CheckCircle2 className="size-3.5" aria-hidden="true" /> : <AlertTriangle className="size-3.5" aria-hidden="true" />}
      {label}
    </span>
  )
}
