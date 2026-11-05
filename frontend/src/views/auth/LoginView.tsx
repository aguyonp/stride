import { useQueryClient } from '@tanstack/react-query'
import { Fingerprint, KeyRound, Loader2, ShieldAlert, UserPlus } from 'lucide-react'
import { useState } from 'react'
import { Link, useLocation, useSearch } from 'wouter'
import { setMe } from '../../api/queries'
import type { AuthState } from '../../api/types'
import { ISSUE_MESSAGE, loginWithPasskey, passkeyErrorMessage, passkeyIssue } from '../../api/webauthn'
import { InlineError } from '../../components/ui/Misc'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { AuthLayout } from './AuthLayout'

export function LoginView({ state }: { state: AuthState }) {
  useDocumentTitle('Connexion')
  const qc = useQueryClient()
  const [, navigate] = useLocation()
  const search = useSearch()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const issue = passkeyIssue()

  const login = async () => {
    setBusy(true)
    setError(null)
    try {
      const me = await loginWithPasskey()
      setMe(qc, me)
      const next = new URLSearchParams(search).get('next')
      navigate(!me.onboardingCompleted ? '/onboarding' : next && next.startsWith('/') && !next.startsWith('//') ? next : '/', { replace: true })
    } catch (e) {
      setError(passkeyErrorMessage(e, 'login'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthLayout>
      <section className="glass animate-in p-5 sm:p-7" aria-labelledby="login-title">
        <h1 id="login-title" className="text-xl font-semibold tracking-tight">
          Connexion
        </h1>
        <p className="mt-1 text-sm text-ink-3">Sans mot de passe : utilise la passkey enregistrée sur ton téléphone, ton ordinateur ou ton gestionnaire.</p>

        {state.firstUser && (
          <div className="mt-4 rounded-2xl border border-accent/35 bg-accent/10 p-3.5 text-sm">
            <p className="font-semibold text-ink">Premier lancement</p>
            <p className="mt-0.5 text-ink-2">Aucun compte n’existe encore. Le premier compte créé devient administrateur, sans invitation.</p>
            <Link href="/register" className="btn btn-primary mt-3 w-full">
              <UserPlus className="size-4" aria-hidden="true" /> Créer le compte administrateur
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
          <button type="button" className="btn btn-primary mt-5 h-13 w-full text-base" onClick={() => void login()} disabled={busy || issue !== null}>
            {busy ? <Loader2 className="size-5 animate-spin" aria-hidden="true" /> : <Fingerprint className="size-5" aria-hidden="true" />}
            {busy ? 'En attente de ta passkey…' : 'Se connecter avec une passkey'}
          </button>
        )}
        {error && <InlineError className="mt-3">{error}</InlineError>}

        {!state.firstUser && (
          <div className="mt-6 border-t border-line pt-4 text-sm">
            {state.registrationMode === 'open' ? (
              <p className="text-ink-3">
                Pas encore de compte ?{' '}
                <Link href="/register" className="font-semibold text-accent underline-offset-2 hover:underline">
                  Créer un compte
                </Link>
              </p>
            ) : (
              <p className="flex gap-2 text-ink-3">
                <KeyRound className="mt-0.5 size-4 flex-none" aria-hidden="true" />
                <span>
                  Inscription sur invitation : demande un lien à l’administrateur.{' '}
                  <Link href="/register" className="font-semibold text-accent underline-offset-2 hover:underline">
                    J’ai un code d’invitation
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
