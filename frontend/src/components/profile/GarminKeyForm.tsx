import { Eye, EyeOff, KeyRound, Loader2 } from 'lucide-react'
import { useId, useState, type FormEvent } from 'react'
import { isApiError } from '../../api/client'
import { useIntegrationMutations } from '../../api/queries'
import type { Integrations } from '../../api/types'
import { cn } from '../../lib/cn'
import { InlineError } from '../ui/Misc'
import { t, tx } from '../../i18n'

export function GarminGuide({ className }: { className?: string }) {
  const steps = [
    tx('settings.garmin.guide1', {
      app: <strong className="font-semibold text-ink">Garmin Connect</strong>,
      link: (
        <a href="https://connect.garmin.com" target="_blank" rel="noopener noreferrer" className="text-accent underline underline-offset-2">
          connect.garmin.com
        </a>
      ),
    }),
    tx('settings.garmin.guide2', { section: <strong className="font-semibold text-ink">{t('settings.garmin.sectionName')}</strong> }),
    tx('settings.garmin.guide3', { button: <strong className="font-semibold text-ink">{t('settings.garmin.createButton')}</strong> }),
    t('settings.garmin.guide4'),
  ]
  return (
    <ol className={cn('space-y-2.5', className)}>
      {steps.map((s, i) => (
        <li key={i} className="flex gap-3 text-sm text-ink-2">
          <span className="num grid size-6 flex-none place-items-center rounded-full bg-accent/15 text-xs font-bold text-accent" aria-hidden="true">
            {i + 1}
          </span>
          <span className="min-w-0 pt-0.5">{s}</span>
        </li>
      ))}
    </ol>
  )
}

export function garminErrorMessage(err: unknown): string {
  if (isApiError(err, 'PROVIDER_UNAUTHORIZED')) return t('settings.garmin.unauthorized')
  if (isApiError(err, 'PROVIDER_UNAVAILABLE')) return t('settings.garmin.unavailable')
  if (isApiError(err, 'RATE_LIMITED')) return t('settings.garmin.rateLimited')
  return err instanceof Error ? err.message : t('settings.garmin.failed')
}

/** API key input: password field, never stored client-side, cleared after success. */
export function GarminKeyForm({ onConnected, submitLabel = t('settings.garmin.connect'), autoFocus }: { onConnected?: (v: Integrations) => void; submitLabel?: string; autoFocus?: boolean }) {
  const id = useId()
  const [key, setKey] = useState('')
  const [show, setShow] = useState(false)
  const { connectGarmin } = useIntegrationMutations()
  const error = connectGarmin.error

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const k = key.trim()
    if (!k) return
    connectGarmin.mutate(k, {
      onSuccess: (v) => {
        setKey('')
        setShow(false)
        onConnected?.(v)
      },
    })
  }

  return (
    <form onSubmit={submit} className="space-y-3" autoComplete="off">
      <div>
        <label htmlFor={id} className="mb-1.5 block text-sm font-medium text-ink-2">
          {t('settings.garmin.keyLabel')}
        </label>
        <div className="relative">
          <KeyRound className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-3" aria-hidden="true" />
          <input
            id={id}
            type={show ? 'text' : 'password'}
            className="field pr-12 pl-9 font-mono"
            value={key}
            onChange={(e) => setKey(e.target.value)}
            autoComplete="off"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            data-1p-ignore
            data-lpignore="true"
            placeholder={t('settings.garmin.keyPlaceholder')}
            aria-invalid={error ? true : undefined}
            autoFocus={autoFocus}
          />
          <button
            type="button"
            className="absolute top-1/2 right-1.5 grid size-9 -translate-y-1/2 place-items-center rounded-lg text-ink-3 hover:text-ink"
            onClick={() => setShow((v) => !v)}
            aria-label={show ? t('settings.garmin.hideKey') : t('settings.garmin.showKey')}
            aria-pressed={show}
          >
            {show ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
          </button>
        </div>
      </div>
      {error && <InlineError>{garminErrorMessage(error)}</InlineError>}
      <button type="submit" className="btn btn-primary w-full sm:w-auto" disabled={!key.trim() || connectGarmin.isPending}>
        {connectGarmin.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
        {connectGarmin.isPending ? t('settings.garmin.checking') : submitLabel}
      </button>
    </form>
  )
}
