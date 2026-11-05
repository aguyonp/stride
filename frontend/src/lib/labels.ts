import type { AcwrVerdict, AlertLevel, DriftVerdict, FocusArea, Freshness, GoalKind, MatchStatus, PlanSport, ShoeType, Sport } from '../api/types'

export const HRV_STATUS: Record<string, { label: string; level: AlertLevel }> = {
  BALANCED: { label: 'Équilibré', level: 'ok' },
  UNBALANCED: { label: 'Déséquilibré', level: 'warn' },
  LOW: { label: 'Bas', level: 'danger' },
  POOR: { label: 'Faible', level: 'danger' },
}

export function hrvStatus(status: string | null, label?: string | null): { label: string; level: AlertLevel } | null {
  if (!status) return null
  return HRV_STATUS[status] ?? { label: label ?? status, level: 'info' }
}

export const SLEEP_QUALIFIER: Record<string, { label: string; level: AlertLevel }> = {
  EXCELLENT: { label: 'Excellent', level: 'ok' },
  GOOD: { label: 'Bon', level: 'ok' },
  FAIR: { label: 'Correct', level: 'warn' },
  POOR: { label: 'Médiocre', level: 'danger' },
}

export const DRIFT_VERDICT: Record<DriftVerdict, { label: string; level: AlertLevel }> = {
  stable: { label: 'Stable', level: 'ok' },
  moderate: { label: 'Modérée', level: 'warn' },
  high: { label: 'Élevée', level: 'danger' },
}

export const ACWR_VERDICT: Record<AcwrVerdict, { label: string; level: AlertLevel }> = {
  low: { label: 'Faible', level: 'info' },
  optimal: { label: 'Optimal', level: 'ok' },
  high: { label: 'Élevé', level: 'warn' },
  danger: { label: 'Danger', level: 'danger' },
}

export const SIDE_LABEL = { left: 'gauche', right: 'droite', both: 'les deux' } as const

const TE_LABEL: Record<string, string> = {
  RECOVERY: 'récupération',
  AEROBIC_BASE: 'base aérobie',
  TEMPO: 'tempo',
  LACTATE_THRESHOLD: 'seuil lactique',
  VO2MAX: 'VO2max',
  ANAEROBIC_CAPACITY: 'capacité anaérobie',
  SPEED: 'vitesse',
}

export function teLabel(raw: string | null): string | null {
  if (!raw) return null
  return TE_LABEL[raw] ?? raw.replace(/_/g, ' ').toLowerCase()
}

export const FOCUS_FALLBACK: Record<FocusArea, string> = {
  none: '100 % Performance – Aucune blessure',
  knee_pfps: 'Genou / Fémoro-patellaire',
  achilles: 'Tendon d’Achille',
  shin_splints: 'Périostite tibiale',
  other: 'Autre',
}

export const CONTEXT_TAGS: { key: string; label: string }[] = [
  { key: 'metronome', label: 'Métronome activé' },
  { key: 'warmup_iso', label: 'Échauffement / Activation isométrique pré-course' },
  { key: 'squatting_48h', label: 'Travaux / Positions accroupies (48h)' },
  { key: 'desk_sitting', label: 'Position assise bureau prolongée' },
]
export const tagLabel = (key: string) => CONTEXT_TAGS.find((t) => t.key === key)?.label ?? key

export const SPORT_LABEL: Record<Sport, string> = {
  run: 'Course',
  bike: 'Vélo',
  walk: 'Marche',
  hike: 'Randonnée',
  swim: 'Natation',
  strength: 'Renforcement',
  other: 'Autre',
}

export const PLAN_SPORT_LABEL: Record<PlanSport, string> = {
  run: 'Course',
  bike: 'Vélo',
  strength: 'Renforcement',
  mobility: 'Mobilité',
  rest: 'Repos',
  other: 'Autre',
}

export const MATCH_STATUS: Record<MatchStatus, { label: string; level: AlertLevel }> = {
  done: { label: 'Réalisée', level: 'ok' },
  partial: { label: 'Partielle', level: 'warn' },
  missed: { label: 'Manquée', level: 'danger' },
  upcoming: { label: 'À venir', level: 'info' },
  extra: { label: 'Hors plan', level: 'info' },
}

export const GOAL_KIND_LABEL: Record<GoalKind, string> = {
  macro: 'Objectif macro',
  race: 'Course cible',
  cadence: 'Cadence',
  weekly_volume: 'Volume hebdo',
  other: 'Autre',
}

export const SHOE_TYPE_LABEL: Record<ShoeType, string> = {
  soft: 'Souple',
  tempo: 'Tempo',
  carbon: 'Carbone',
}

export const FRESHNESS_LABEL: Record<Freshness['source'], { label: string; hint: string }> = {
  db: { label: 'Base locale', hint: 'Données lues depuis la base Stride (aucun appel fournisseur).' },
  cache: { label: 'Cache', hint: 'Données servies depuis le cache (moins de quelques minutes).' },
  live: { label: 'Synchro live', hint: 'Un appel Garmin / Strava a été fait pour cette réponse.' },
}

export const SOURCE_LABEL: Record<string, string> = { garmin: 'Garmin', strava: 'Strava' }
