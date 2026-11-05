import { CircleAlert, Compass, Lock, RotateCw } from 'lucide-react'
import { Link } from 'wouter'
import { Skeleton } from '../components/ui/Skeleton'

export function LoadingView({ label = 'Chargement…' }: { label?: string }) {
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

export function ErrorView({ error, onRetry, title = 'Impossible de charger les données' }: { error: unknown; onRetry?: () => void; title?: string }) {
  const message = error instanceof Error ? error.message : 'Erreur inconnue.'
  return (
    <section className="glass mx-auto max-w-xl p-6 text-center" role="alert">
      <span className="mx-auto mb-3 grid size-11 place-items-center rounded-2xl bg-danger/12 text-danger">
        <CircleAlert className="size-5" aria-hidden="true" />
      </span>
      <h2 className="font-semibold">{title}</h2>
      <p className="mt-1 text-sm text-ink-2">{message}</p>
      {onRetry && (
        <button type="button" className="btn btn-ghost mt-4" onClick={onRetry}>
          <RotateCw className="size-4" aria-hidden="true" /> Réessayer
        </button>
      )}
    </section>
  )
}

export function BootErrorView({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  return (
    <div className="grid min-h-[60vh] place-items-center">
      <ErrorView error={error} onRetry={onRetry} title="Stride ne répond pas" />
    </div>
  )
}

export function NotFoundView() {
  return (
    <section className="glass mx-auto mt-6 max-w-xl p-6 text-center">
      <span className="mx-auto mb-3 grid size-11 place-items-center rounded-2xl bg-accent/12 text-accent">
        <Compass className="size-5" aria-hidden="true" />
      </span>
      <h1 className="text-lg font-semibold">Page introuvable</h1>
      <p className="mt-1 text-sm text-ink-3">Cette adresse ne correspond à aucune page de Stride.</p>
      <Link href="/" className="btn btn-primary mt-4">
        Retour au tableau de bord
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
      <h1 className="text-lg font-semibold">Accès réservé aux administrateurs</h1>
      <p className="mt-1 text-sm text-ink-3">Ton compte n’a pas les droits nécessaires pour cette page.</p>
      <Link href="/" className="btn btn-ghost mt-4">
        Retour au tableau de bord
      </Link>
    </section>
  )
}
