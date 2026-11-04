import type { AcwrVerdict, AlertLevel, DriftVerdict, Mode } from '../api/types'

export const MODE_LABEL: Record<Mode, string> = {
  sfp: 'Rééducation SFP',
  recovery: 'Santé & Récup',
  performance: 'Performance',
}

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

export const SIDE_LABEL = { left: 'gauche', right: 'droite' } as const

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
