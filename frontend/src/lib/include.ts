import type { IncludeKey } from '../api/types'

export interface IncludeChip {
  key: IncludeKey
  label: string
  hint: string
  scope: 'session' | 'all'
}

export const INCLUDE_CHIPS: IncludeChip[] = [
  { key: 'pain', label: 'Suivi douleur', hint: 'Ligne « Ressenti & Douleur » et colonnes de gêne de l’historique', scope: 'all' },
  { key: 'biomech', label: 'Biomécanique', hint: 'Cadence, GCT, OV, foulée, dérive de fatigue, pente', scope: 'session' },
  { key: 'health', label: 'Santé / VFC', hint: 'Nuit précédente : VFC, sommeil, FC repos, Body Battery, readiness', scope: 'all' },
  { key: 'plan', label: 'Planning', hint: 'Profil, objectifs, séance prévue et prochaines séances', scope: 'all' },
  { key: 'load', label: 'Charge 48 h', hint: 'Dérive cardiaque, stamina, charge, ratio, vélo / étages / pas', scope: 'session' },
  { key: 'splits', label: 'Intervalles', hint: 'Tableau des blocs / laps', scope: 'session' },
  { key: 'history', label: 'Historique 14 j', hint: 'Tableau des 14 derniers jours', scope: 'all' },
]

export const ALL_INCLUDE: IncludeKey[] = INCLUDE_CHIPS.map((c) => c.key)

export function parseInclude(raw: string | null): IncludeKey[] | undefined {
  if (raw === null) return undefined
  if (raw === '') return []
  const keys = raw.split(',').filter((k): k is IncludeKey => (ALL_INCLUDE as string[]).includes(k))
  return ALL_INCLUDE.filter((k) => keys.includes(k))
}

export const serializeInclude = (keys: IncludeKey[]) => ALL_INCLUDE.filter((k) => keys.includes(k)).join(',')

/** Keys sent for a subject: session-only chips are dropped for the day report. */
export function effectiveInclude(keys: IncludeKey[], subject: 'activity' | 'day' | 'multi'): IncludeKey[] {
  return keys.filter((k) => {
    const chip = INCLUDE_CHIPS.find((c) => c.key === k)
    if (!chip) return false
    if (subject === 'day') return chip.scope === 'all'
    if (subject === 'multi') return k !== 'splits'
    return true
  })
}
