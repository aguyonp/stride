import { useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Fingerprint, Loader2, ShieldAlert } from 'lucide-react'
import { useState } from 'react'
import { Link, useLocation, useSearch } from 'wouter'
import { setMe } from '../../api/queries'
import type { AuthState } from '../../api/types'
import { ISSUE_MESSAGE, passkeyErrorMessage, passkeyIssue, registerWithPasskey } from '../../api/webauthn'
import { Field } from '../../components/ui/Field'
import { InlineError } from '../../components/ui/Misc'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { AuthLayout } from './AuthLayout'

export function RegisterView({ state }: { state: AuthState }) {
  useDocumentTitle('Créer un compte')
  const qc = useQueryClient()
  const [, navigate] = useLocation()
  const search = useSearch()
  const fromLink = new URLSearchParams(search).get('invite') ?? ''
  const [name, setName] = useState('')
  const [code, setCode] = useState(fromLink)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [touched, setTouched] = useState(false)
  const issue = passkeyIssue()
  const needsInvite = !state.firstUser && state.registrationMode === 'invite'
  const nameError = touched && name.trim().length < 2 ? 'Indique au moins 2 caractères.' : null
  const codeError = touched && needsInvite && !code.trim() ? 'Le code d’invitation est requis.' : null

  const submit = async () => {
    setTouched(true)
    if (name.trim().length < 2 || (needsInvite && !code.trim())) return
    setBusy(true)
    setError(null)
    try {
      const me = await registerWithPasskey(name.trim(), code.trim() || undefined)
      setMe(qc, me)
      navigate('/onboarding', { replace: true })
    } catch (e) {
      setError(passkeyErrorMessage(e, 'register'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthLayout>
      <section className="glass animate-in p-5 sm:p-7" aria-labelledby="register-title">
        <Link href="/login" className="mb-3 inline-flex items-center gap-1.5 rounded-lg text-sm text-ink-3 hover:text-ink">
          <ArrowLeft className="size-4" aria-hidden="true" /> Connexion
        </Link>
        <h1 id="register-title" className="text-xl font-semibold tracking-tight">
          {state.firstUser ? 'Créer le compte administrateur' : 'Créer ton compte'}
        </h1>
        <p className="mt-1 text-sm text-ink-3">
          {state.firstUser
            ? 'Premier compte de cette instance : il aura les droits d’administration (invitations, utilisateurs, suivi des API).'
            : 'Ta passkey remplace le mot de passe : elle reste sur ton appareil ou ton gestionnaire (iCloud, Google, 1Password…), protégée par Face ID, empreinte ou code.'}
        </p>

        {issue && (
          <p className="mt-4 flex gap-2 rounded-2xl border border-warn/30 bg-warn/10 p-3 text-sm text-ink-2" role="alert">
            <ShieldAlert className="mt-0.5 size-4 flex-none text-warn" aria-hidden="true" />
            {ISSUE_MESSAGE[issue]}
          </p>
        )}

        <form
          className="mt-5 space-y-4"
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            void submit()
          }}
        >
          <Field label="Nom affiché" hint="Visible par toi et l’administrateur." error={nameError}>
            {(p) => <input {...p} className="field" value={name} onChange={(e) => setName(e.target.value)} autoComplete="nickname" maxLength={40} placeholder="Ex. Alex" />}
          </Field>
          {(needsInvite || fromLink) && (
            <Field label="Code d’invitation" hint={fromLink ? 'Pré-rempli depuis ton lien d’invitation.' : 'Reçu de l’administrateur (lien ou code).'} error={codeError}>
              {(p) => <input {...p} className="field font-mono" value={code} onChange={(e) => setCode(e.target.value)} autoComplete="off" spellCheck={false} autoCapitalize="characters" />}
            </Field>
          )}
          {error && <InlineError>{error}</InlineError>}
          <button type="submit" className="btn btn-primary h-13 w-full text-base" disabled={busy || issue !== null}>
            {busy ? <Loader2 className="size-5 animate-spin" aria-hidden="true" /> : <Fingerprint className="size-5" aria-hidden="true" />}
            {busy ? 'Création de la passkey…' : 'Créer ma passkey'}
          </button>
        </form>
      </section>
    </AuthLayout>
  )
}
