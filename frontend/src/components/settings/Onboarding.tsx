import { KeyRound, RefreshCw, ShieldCheck } from 'lucide-react'
import type { ApiError } from '../../api/client'
import { StrideMark } from '../ui/icons'

interface OnboardingProps {
  error: ApiError
  onOpenSettings: () => void
  onRetry: () => void
}

/** Écran d'accueil quand aucune clé n'est configurée (NO_API_KEY) ou qu'elle est refusée. */
export function Onboarding({ error, onOpenSettings, onRetry }: OnboardingProps) {
  const rejected = error.code === 'GARMIN_UNAUTHORIZED'
  const steps = [
    { t: 'Ouvre Garmin Connect sur le web', d: 'connect.garmin.com, depuis un ordinateur.' },
    { t: 'Va dans « Personal API Key »', d: 'Menu du profil → Paramètres → Personal API Key.' },
    { t: 'Clique « Créer » et copie la clé', d: 'Colle-la ensuite dans les paramètres de Stride.' },
  ]
  return (
    <section className="glass animate-in mx-auto max-w-2xl overflow-hidden p-0" aria-labelledby="onboarding-title">
      <div className="relative px-5 pt-7 pb-5 text-center sm:px-8">
        <div className="mx-auto mb-4 grid size-16 place-items-center rounded-[22px] bg-gradient-to-br from-accent/25 to-accent-2/10 ring-1 ring-accent/30 ring-inset">
          <StrideMark className="size-10" />
        </div>
        <h2 id="onboarding-title" className="text-xl font-semibold tracking-tight sm:text-2xl">
          {rejected ? 'Ta clé Garmin a été refusée' : 'Connecte tes données Garmin'}
        </h2>
        <p className="mx-auto mt-2 max-w-md text-sm text-ink-2">
          {rejected
            ? 'Garmin n’accepte plus la clé actuelle (révoquée ou expirée). Crée une nouvelle clé personnelle pour reprendre le suivi.'
            : 'Stride lit tes séances, ton sommeil et ta VFC via l’API personnelle de Garmin Connect. Il suffit d’une clé, créée en 30 secondes.'}
        </p>
      </div>
      <ol className="space-y-2 px-4 sm:px-8">
        {steps.map((s, i) => (
          <li key={s.t} className="glass-inset flex items-start gap-3 px-3.5 py-3">
            <span className="num grid size-7 flex-none place-items-center rounded-full bg-accent/15 text-sm font-bold text-accent">{i + 1}</span>
            <div className="min-w-0">
              <p className="text-sm font-semibold text-ink">{s.t}</p>
              <p className="text-xs text-ink-3">{s.d}</p>
            </div>
          </li>
        ))}
      </ol>
      <div className="flex flex-col gap-2 px-4 pt-5 pb-4 sm:flex-row sm:px-8">
        <button type="button" className="btn btn-primary h-12 flex-1 text-[0.95rem]" onClick={onOpenSettings}>
          <KeyRound className="size-[1.125rem]" aria-hidden="true" />
          Saisir ma clé API
        </button>
        <button type="button" className="btn btn-ghost h-12" onClick={onRetry}>
          <RefreshCw className="size-4" aria-hidden="true" />
          Réessayer
        </button>
      </div>
      <p className="flex items-center justify-center gap-1.5 border-t border-line px-4 py-3 text-center text-xs text-ink-3">
        <ShieldCheck className="size-3.5 flex-none" aria-hidden="true" />
        La clé est envoyée une seule fois au serveur Stride ; elle n’est jamais stockée dans le navigateur.
      </p>
      <p className="sr-only" role="status">
        {error.message}
      </p>
    </section>
  )
}
