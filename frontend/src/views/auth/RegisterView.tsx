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
import { t } from '../../i18n'

export function RegisterView({ state }: { state: AuthState }) {
  useDocumentTitle(t('auth.register.title'))
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
  const nameError = touched && name.trim().length < 2 ? t('auth.register.nameTooShort') : null
  const codeError = touched && needsInvite && !code.trim() ? t('auth.register.codeRequired') : null

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
          <ArrowLeft className="size-4" aria-hidden="true" /> {t('auth.register.back')}
        </Link>
        <h1 id="register-title" className="text-xl font-semibold tracking-tight">
          {state.firstUser ? t('auth.register.adminTitle') : t('auth.register.userTitle')}
        </h1>
        <p className="mt-1 text-sm text-ink-3">
          {state.firstUser
            ? t('auth.register.adminIntro')
            : t('auth.register.userIntro')}
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
          <Field label={t('auth.register.name')} hint={t('auth.register.nameHint')} error={nameError}>
            {(p) => <input {...p} className="field" value={name} onChange={(e) => setName(e.target.value)} autoComplete="nickname" maxLength={40} placeholder={t('auth.register.namePlaceholder')} />}
          </Field>
          {(needsInvite || fromLink) && (
            <Field label={t('auth.register.code')} hint={fromLink ? t('auth.register.codeFromLink') : t('auth.register.codeHint')} error={codeError}>
              {(p) => <input {...p} className="field font-mono" value={code} onChange={(e) => setCode(e.target.value)} autoComplete="off" spellCheck={false} autoCapitalize="characters" />}
            </Field>
          )}
          {error && <InlineError>{error}</InlineError>}
          <button type="submit" className="btn btn-primary h-13 w-full text-base" disabled={busy || issue !== null}>
            {busy ? <Loader2 className="size-5 animate-spin" aria-hidden="true" /> : <Fingerprint className="size-5" aria-hidden="true" />}
            {busy ? t('auth.register.creating') : t('auth.register.submit')}
          </button>
        </form>
      </section>
    </AuthLayout>
  )
}
