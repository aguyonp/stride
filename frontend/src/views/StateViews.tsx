import { CircleAlert, Compass, Lock, RotateCw } from 'lucide-react'
import { Link } from 'wouter'
import { Skeleton } from '../components/ui/Skeleton'
import { t } from '../i18n'

export function LoadingView({ label = t('common.actions.loading') }: { label?: string }) {
  return (
    <div className="space-y-4" role="status" aria-label={label}>
      <Skeleton className="h-24" />
      <div className="grid gap-4 md:grid-cols-2">
        <Skeleton className="h-48" />
        <Skeleton className="h-48" />
      </div>
      <Skeleton className="h-64" />
      <span className="sr-only">{label}</span>
    </div>
  )
}

export function ErrorView({ error, onRetry, title = t('layout.state.loadFailed') }: { error: unknown; onRetry?: () => void; title?: string }) {
  const message = error instanceof Error ? error.message : t('layout.state.unknownError')
  return (
    <section className="glass mx-auto max-w-xl p-6 text-center" role="alert">
      <span className="mx-auto mb-3 grid size-11 place-items-center rounded-2xl bg-danger/12 text-danger">
        <CircleAlert className="size-5" aria-hidden="true" />
      </span>
      <h2 className="font-semibold">{title}</h2>
      <p className="mt-1 text-sm text-ink-2">{message}</p>
      {onRetry && (
        <button type="button" className="btn btn-ghost mt-4" onClick={onRetry}>
          <RotateCw className="size-4" aria-hidden="true" /> {t('common.actions.retry')}
        </button>
      )}
    </section>
  )
}

export function BootErrorView({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  return (
    <div className="grid min-h-[60vh] place-items-center">
      <ErrorView error={error} onRetry={onRetry} title={t('layout.state.down')} />
    </div>
  )
}

export function NotFoundView() {
  return (
    <section className="glass mx-auto mt-6 max-w-xl p-6 text-center">
      <span className="mx-auto mb-3 grid size-11 place-items-center rounded-2xl bg-accent/12 text-accent">
        <Compass className="size-5" aria-hidden="true" />
      </span>
      <h1 className="text-lg font-semibold">{t('layout.state.notFound')}</h1>
      <p className="mt-1 text-sm text-ink-3">{t('layout.state.notFoundHint')}</p>
      <Link href="/" className="btn btn-primary mt-4">
        {t('layout.state.backHome')}
      </Link>
    </section>
  )
}

export function ForbiddenView() {
  return (
    <section className="glass mx-auto mt-6 max-w-xl p-6 text-center">
      <span className="mx-auto mb-3 grid size-11 place-items-center rounded-2xl bg-warn/12 text-warn">
        <Lock className="size-5" aria-hidden="true" />
      </span>
      <h1 className="text-lg font-semibold">{t('layout.state.forbidden')}</h1>
      <p className="mt-1 text-sm text-ink-3">{t('layout.state.forbiddenHint')}</p>
      <Link href="/" className="btn btn-ghost mt-4">
        {t('layout.state.backHome')}
      </Link>
    </section>
  )
}
