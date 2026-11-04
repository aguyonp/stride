import type { Mode, Profile } from './types'

/** Profil par défaut (valeurs du contrat) — utilisé tant que /api/settings n'a pas répondu. */
export const DEFAULT_PROFILE: Profile = {
  injuredSide: 'left',
  cadenceTargetMin: 175,
  cadenceTargetMax: 182,
  gctBalanceAlertPct: 1.5,
  verticalOscillationAlertCm: 9,
  descentAlertM: 30,
  rhrAlertDeltaBpm: 5,
  pace10kMinSecPerKm: 215,
  pace10kMaxSecPerKm: 220,
  paceMarathonMinSecPerKm: 230,
  paceMarathonMaxSecPerKm: 240,
}

export const MODES: Mode[] = ['sfp', 'recovery', 'performance']
export const isMode = (v: unknown): v is Mode => typeof v === 'string' && (MODES as string[]).includes(v)
