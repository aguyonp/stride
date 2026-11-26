import { AlertTriangle, CheckCircle2, Loader2, ShieldAlert, Unplug } from 'lucide-react'
import { useEffect, useState } from 'react'
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
 * The card stays a summary: a state, one button, the fallback line. Everything that needs a keyboard
 * — consent, credentials, the code — happens in a modal, so the settings page keeps its rhythm
 * instead of growing a module taller than the screen.
 *
 * The password leaves the form for one request, is never stored and never logged. The fields opt out
 * of the password managers: this credential belongs to Garmin, not to Stride, and a « save this
 * login? » prompt over the card is noise — and a bad idea for a one-shot password.
 */
export function GarminBridgeCard() {
  const bridge = useGarminBridge()
  const m = useGarminBridgeMutations()
  const { toast } = useToast()
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState({ email: '', password: '' })
  const [mfa, setMfa] = useState('')
  // The failure is shown where the field is, not only in a toast: a wrong password must say « wrong
  // password », and a code step must be reachable the instant Garmin asks for it.
  const [error, setError] = useState('')
  const [awaitingCode, setAwaitingCode] = useState(false)

  const st = bridge.data
  // A 202 says « code required » before the poll comes back: the flow must follow the answer, not
  // wait for the next GET.
  const state = awaitingCode ? 'mfa' : (st?.state ?? 'disconnected')

  // Two unfinished actions reopen the flow by themselves: a code Garmin already asked for, and an
  // expired session reached from the notification centre (whose link targets #integrations).
  const pending = st?.state === 'mfa'
  const expiredFromLink = st?.state === 'expired' && typeof window !== 'undefined' && window.location.hash === '#integrations'
  useEffect(() => {
    if (pending || expiredFromLink) setOpen(true)
  }, [pending, expiredFromLink])

  // Nothing is rendered until the instance answers, and nothing is offered when the administrator
  // has not opened the bridge: `available` is the instance's word, not a guess from an HTTP status.
  if (!st?.available) return null
  const busy = m.login.isPending || m.mfa.isPending

  const close = () => {
    setOpen(false)
    setError('')
    setMfa('')
  }

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
        if (r.state === 'connected') {
          close()
          toast({ title: t('settings.bridge.connected'), description: r.displayName, tone: 'success' })
        }
      },
      onError: (e) => {
        setForm((f) => ({ ...f, password: '' }))
        // Always in the flow: a 400 is the most common answer and used to say nothing at all.
        setError(bridgeErrorText(e, 'password'))
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
          close()
          toast({ title: t('settings.bridge.connected'), description: r.displayName, tone: 'success' })
        }
      },
      onError: (e) => {
        setMfa('')
        setError(bridgeErrorText(e, 'code'))
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
        <div className="mt-2.5 flex flex-wrap items-center gap-2">
          <StateBadge state={state} locked={st.locked} />
          {state === 'connected' && st.displayName && <span className="truncate text-xs text-ink-2">{st.displayName}</span>}
          {state === 'connected' && st.since && (
            <span className="hidden text-xs text-ink-3 sm:inline">{t('settings.bridge.since', { date: fmtDateTime(st.since) })}</span>
          )}
          {state === 'connected' ? (
            <button
              type="button"
              className="btn btn-ghost ml-auto h-8 px-2.5 text-xs"
              onClick={() => m.disconnect.mutate(undefined, { onSuccess: () => toast({ title: t('settings.bridge.disconnected') }) })}
              disabled={m.disconnect.isPending}
            >
              <Unplug className="size-3.5" aria-hidden="true" /> {t('settings.bridge.disconnect')}
            </button>
          ) : (
            <button type="button" className="btn btn-ghost ml-auto h-8 px-2.5 text-xs" onClick={() => setOpen(true)}>
              {state === 'mfa'
                ? t('settings.bridge.enterCode')
                : state === 'expired' || state === 'blocked'
                  ? t('settings.bridge.reconnect')
                  : t('settings.bridge.enable')}
            </button>
          )}
        </div>
      )}

      <p className="mt-2 text-xs text-ink-3">{t('settings.bridge.fallbackShort')}</p>

      {(state === 'expired' || state === 'blocked') && (
        <p role="alert" className="mt-2 rounded-xl border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger">
          {state === 'expired' ? t('settings.bridge.expiredBody') : t('settings.bridge.blockedBody')}
        </p>
      )}

      <Dialog
        open={open}
        onClose={close}
        title={t('settings.bridge.title')}
        description={t('settings.bridge.warning')}
        size="md"
        // The Field owns the ids: target the control by its type, not by an id we would have to
        // override (which breaks the label association).
        initialFocus={state === 'mfa' ? 'input[inputmode="numeric"]' : 'input[type="email"]'}
      >
        <div className="space-y-3">
          {/* An expired session is not a first sign-in: the consent was given once, say why the
              fields are back instead of repeating the warning. */}
          {state === 'expired' && (
            <p role="alert" className="rounded-xl border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger">
              {t('settings.bridge.expiredBody')}
            </p>
          )}

          {/* Consent first: the warning is read before anything can be typed. */}
          {state !== 'mfa' && state !== 'expired' && (
            <div role="alert" className="rounded-xl border border-warn/35 bg-warn/[0.08] p-3 text-xs text-ink-2">
              <p className="font-semibold text-ink">{t('settings.bridge.consentTitle')}</p>
              <ul className="mt-1.5 list-inside list-disc space-y-0.5">
                <li>{t('settings.bridge.consentApi')}</li>
                <li>{t('settings.bridge.consentRisk')}</li>
                <li>{t('settings.bridge.consentPassword')}</li>
                <li>{t('settings.bridge.consentFallback')}</li>
              </ul>
              <p className="mt-2 text-ink-3">{t('settings.bridge.fallback')}</p>
            </div>
          )}

          {state === 'mfa' ? (
            <form
              className="space-y-2.5"
              autoComplete="off"
              onSubmit={(e) => {
                e.preventDefault()
                if (mfa) sendCode()
              }}
            >
              <p className="text-xs text-ink-2">{t('settings.bridge.mfaHint')}</p>
              <Field label={t('settings.bridge.code')}>
                {(p) => (
                  <input
                    {...p}
                    className="field num max-w-40"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    value={mfa}
                    onChange={(e) => {
                      setError('')
                      setMfa(e.target.value.replace(/\D/g, ''))
                    }}
                  />
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
          ) : (
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
                  <input
                    {...p}
                    className="field"
                    type="email"
                    autoComplete="off"
                    data-1p-ignore
                    data-lpignore="true"
                    data-bwignore
                    value={form.email}
                    onChange={(e) => {
                      setError('')
                      setForm((f) => ({ ...f, email: e.target.value }))
                    }}
                  />
                )}
              </Field>
              <Field label={t('settings.bridge.password')} hint={t('settings.bridge.passwordHint')}>
                {(p) => (
                  <input
                    {...p}
                    className="field"
                    type="password"
                    autoComplete="new-password"
                    data-1p-ignore
                    data-lpignore="true"
                    data-bwignore
                    value={form.password}
                    onChange={(e) => {
                      setError('')
                      setForm((f) => ({ ...f, password: e.target.value }))
                    }}
                  />
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
                <button type="button" className="btn btn-ghost h-9 px-3 text-sm" onClick={close} disabled={busy}>
                  {t('common.actions.cancel')}
                </button>
              </div>
            </form>
          )}
        </div>
      </Dialog>
    </div>
  )
}

/**
 * bridgeErrorText translates the stable HTTP code into a sentence the user can act on, at the step
 * where they stand: a 400 means « wrong password » before the code and « wrong code » after it — the
 * second field report showed the password sentence under the code field.
 */
function bridgeErrorText(e: unknown, step: 'password' | 'code'): string {
  if (isApiError(e, 'BAD_REQUEST')) return step === 'code' ? t('settings.bridge.codeRefusedBody') : t('settings.bridge.badCredentials')
  if (isApiError(e, 'RATE_LIMITED')) return t('settings.bridge.blockedBody')
  if (isApiError(e, 'PROVIDER_UNAVAILABLE')) return t('settings.bridge.flowFailed')
  if (isApiError(e, 'UNAUTHORIZED')) return t('settings.bridge.expiredBody')
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
