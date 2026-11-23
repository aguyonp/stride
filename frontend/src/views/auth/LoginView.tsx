import { useQueryClient } from '@tanstack/react-query'
import { Fingerprint, KeyRound, Loader2, ShieldAlert, ShieldCheck, UserPlus } from 'lucide-react'
import { useState } from 'react'
import { Link, useLocation, useSearch } from 'wouter'
import { api, ApiError } from '../../api/client'
import { setMe } from '../../api/queries'
import type { AuthState, Me } from '../../api/types'
import { ISSUE_MESSAGE, loginWithPasskey, passkeyErrorMessage, passkeyIssue } from '../../api/webauthn'
import { InlineError } from '../../components/ui/Misc'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { AuthLayout } from './AuthLayout'
import { t } from '../../i18n'

export function LoginView({ state }: { state: AuthState }) {
  useDocumentTitle(t('auth.login.title'))
  const qc = useQueryClient()
  const [, navigate] = useLocation()
  const search = useSearch()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [recoveryOpen, setRecoveryOpen] = useState(false)
  const [recoveryBusy, setRecoveryBusy] = useState(false)
  const [code, setCode] = useState('')
  const [recoveryError, setRecoveryError] = useState<string | null>(null)
  const issue = passkeyIssue()

  const finishLogin = (me: Me) => {
    setMe(qc, me)
    const next = new URLSearchParams(search).get('next')
    navigate(!me.onboardingCompleted ? '/onboarding' : next && next.startsWith('/') && !next.startsWith('//') ? next : '/', { replace: true })
  }

  const login = async () => {
    setBusy(true)
    setError(null)
    try {
      finishLogin(await loginWithPasskey())
    } catch (e) {
      setError(passkeyErrorMessage(e, 'login'))
    } finally {
      setBusy(false)
    }
  }

  const loginRecovery = async () => {
    setRecoveryBusy(true)
    setRecoveryError(null)
    try {
      finishLogin(await api.loginRecovery(code))
    } catch (e) {
      setRecoveryError(e instanceof ApiError ? e.message : t('auth.login.recoveryFailed'))
    } finally {
      setRecoveryBusy(false)
    }
  }

  return (
    <AuthLayout>
      <section className="glass animate-in p-5 sm:p-7" aria-labelledby="login-title">
        <h1 id="login-title" className="text-xl font-semibold tracking-tight">
          {t('auth.login.title')}
        </h1>
        <p className="mt-1 text-sm text-ink-3">{t('auth.login.intro')}</p>

        {state.firstUser && (
          <div className="mt-4 rounded-2xl border border-accent/35 bg-accent/10 p-3.5 text-sm">
            <p className="font-semibold text-ink">{t('auth.login.firstRun')}</p>
            <p className="mt-0.5 text-ink-2">{t('auth.login.firstRunHint')}</p>
            <Link href="/register" className="btn btn-primary mt-3 w-full">
              <UserPlus className="size-4" aria-hidden="true" /> {t('auth.login.createAdmin')}
            </Link>
          </div>
        )}

        {issue && (
          <p className="mt-4 flex gap-2 rounded-2xl border border-warn/30 bg-warn/10 p-3 text-sm text-ink-2" role="alert">
            <ShieldAlert className="mt-0.5 size-4 flex-none text-warn" aria-hidden="true" />
            {ISSUE_MESSAGE[issue]}
          </p>
        )}

        {!state.firstUser && (
          <>
            <button type="button" className="btn btn-primary mt-5 h-13 w-full text-base" onClick={() => void login()} disabled={busy || issue !== null}>
              {busy ? <Loader2 className="size-5 animate-spin" aria-hidden="true" /> : <Fingerprint className="size-5" aria-hidden="true" />}
              {busy ? t('auth.login.waiting') : t('auth.login.submit')}
            </button>
            {error && <InlineError className="mt-3">{error}</InlineError>}

            <div className="mt-3">
              <button type="button" className="text-sm text-ink-3 underline-offset-2 hover:text-ink hover:underline" onClick={() => { setRecoveryOpen((v) => !v); setRecoveryError(null) }} aria-expanded={recoveryOpen}>
                {t('auth.login.recoveryToggle')}
              </button>
              {recoveryOpen && (
                <form className="mt-3 space-y-2" onSubmit={(e) => { e.preventDefault(); if (code.trim()) void loginRecovery() }}>
                  <p className="text-xs text-ink-3">{t('auth.login.recoveryHint')}</p>
                  <div className="flex gap-2">
                    <input
                      className="field min-h-11 flex-1 font-mono uppercase tracking-wider"
                      placeholder={t('auth.login.recoveryPlaceholder')}
                      value={code}
                      onChange={(e) => setCode(e.target.value)}
                      maxLength={20}
                      aria-label={t('auth.login.recoveryLabel')}
                      autoFocus
                    />
                    <button type="submit" className="btn h-11" disabled={recoveryBusy || !code.trim()}>
                      {recoveryBusy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <ShieldCheck className="size-4" aria-hidden="true" />}
                      {t('auth.login.recoverySubmit')}
                    </button>
                  </div>
                  {recoveryError && <InlineError>{recoveryError}</InlineError>}
                </form>
              )}
            </div>
          </>
        )}

        {!state.firstUser && (
          <div className="mt-6 border-t border-line pt-4 text-sm">
            {state.registrationMode === 'open' ? (
              <p className="text-ink-3">
                {t('auth.login.noAccount')}{' '}
                <Link href="/register" className="font-semibold text-accent underline-offset-2 hover:underline">
                  {t('auth.login.createAccount')}
                </Link>
              </p>
            ) : (
              <p className="flex gap-2 text-ink-3">
                <KeyRound className="mt-0.5 size-4 flex-none" aria-hidden="true" />
                <span>
                  {t('auth.login.inviteOnly')}{' '}
                  <Link href="/register" className="font-semibold text-accent underline-offset-2 hover:underline">
                    {t('auth.login.haveCode')}
                  </Link>
                </span>
              </p>
            )}
          </div>
        )}
      </section>
    </AuthLayout>
  )
}
