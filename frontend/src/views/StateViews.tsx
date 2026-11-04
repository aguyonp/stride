import { CloudOff, RefreshCw } from 'lucide-react'
import type { ApiError } from '../api/client'
import { Skeleton } from '../components/ui/Skeleton'

/** Squelette de chargement (shimmer) reprenant la grille. */
export function LoadingView({ label = 'Chargement de l’analyse…' }: { label?: string }) {
  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-12" role="status" aria-label={label}>
      <Skeleton className="h-20 rounded-[22px] xl:col-span-12" />
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-3 xl:col-span-12 xl:grid-cols-6">
        {Array.from({ length: 6 }, (_, i) => (
          <Skeleton key={i} className="h-[76px] rounded-2xl" />
        ))}
      </div>
      <Skeleton className="h-[420px] rounded-[22px] xl:col-span-7" />
      <div className="grid gap-4 xl:col-span-5">
        <Skeleton className="h-48 rounded-[22px]" />
        <Skeleton className="h-48 rounded-[22px]" />
      </div>
      <Skeleton className="h-80 rounded-[22px] xl:col-span-12" />
      <span className="sr-only">{label}</span>
    </div>
  )
}

export function ErrorView({ error, onRetry }: { error: ApiError | Error; onRetry: () => void }) {
  const code = 'code' in error ? error.code : null
  const title = code === 'GARMIN_UNAVAILABLE' ? 'Garmin Connect ne répond pas' : code === 'NOT_FOUND' ? 'Introuvable' : 'Impossible de charger les données'
  return (
    <section className="glass animate-in mx-auto max-w-xl p-6 text-center" role="alert">
      <span className="mx-auto mb-3 grid size-12 place-items-center rounded-2xl bg-danger/12 text-danger">
        <CloudOff className="size-6" aria-hidden="true" />
      </span>
      <h2 className="text-lg font-semibold">{title}</h2>
      <p className="mt-1 text-sm text-ink-2">{error.message}</p>
      <button type="button" className="btn btn-ghost mt-4" onClick={onRetry}>
        <RefreshCw className="size-4" aria-hidden="true" /> Réessayer
      </button>
    </section>
  )
}

export function EmptyView({ message }: { message: string }) {
  return (
    <section className="glass animate-in mx-auto max-w-xl p-6 text-center">
      <p className="text-sm text-ink-2">{message}</p>
    </section>
  )
}
