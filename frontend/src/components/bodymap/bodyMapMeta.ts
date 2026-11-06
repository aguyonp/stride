import type { BodyCorrelationFactor, BodySide, BodyZone, BodyZoneSummary } from '../../api/types'

export const BODY_ZONE_META: Record<
  BodyZone,
  { label: string; short: string; view: 'Antérieure' | 'Postérieure' | 'Plantaire'; hint: string }
> = {
  knee_pfps: {
    label: 'Rotule / Genou (SFP)',
    short: 'Rotule / Genou',
    view: 'Antérieure',
    hint: 'Syndrome fémoro-patellaire, descentes, escaliers, flexion prolongée.',
  },
  shin_splints: {
    label: 'Périoste tibial (Périostite)',
    short: 'Périoste tibial',
    view: 'Antérieure',
    hint: 'Bord postéro-médial du tibia, chocs répétés et hausses brusques de volume.',
  },
  iliotibial_band: {
    label: 'Bandelette ilio-tibiale (TFL)',
    short: 'Bandelette TFL',
    view: 'Antérieure',
    hint: 'Face externe du genou et de la cuisse, dévers et allongement de foulée.',
  },
  achilles: {
    label: 'Tendon d’Achille',
    short: 'Tendon d’Achille',
    view: 'Postérieure',
    hint: 'Raideur matinale à froid J+1, travail de pied, drop faible ou D+.',
  },
  plantar_fascia: {
    label: 'Aponévrose plantaire',
    short: 'Aponévrose',
    view: 'Plantaire',
    hint: 'Insertion calcanéenne et voûte plantaire, premiers pas au lever.',
  },
  hamstrings_calves: {
    label: 'Ischio-jambiers / Mollets',
    short: 'Ischios / Mollets',
    view: 'Postérieure',
    hint: 'Chaîne postérieure, vitesse haute, fatigue neuromusculaire.',
  },
}

export const ALL_BODY_ZONES: BodyZone[] = [
  'knee_pfps',
  'shin_splints',
  'iliotibial_band',
  'achilles',
  'plantar_fascia',
  'hamstrings_calves',
]

export const BODY_SIDE_LABEL: Record<BodySide, string> = {
  left: 'Gauche',
  right: 'Droite',
  both: 'Bilatéral',
}

export const TREND_META: Record<BodyZoneSummary['trend'], { label: string; cls: string }> = {
  improving: { label: 'En amélioration', cls: 'border-ok/30 bg-ok/12 text-ok' },
  stable: { label: 'Stable', cls: 'border-line-strong bg-track text-ink-2' },
  worsening: { label: 'En hausse', cls: 'border-danger/30 bg-danger/12 text-danger' },
  none: { label: 'Aucun signal', cls: 'border-line bg-track/60 text-ink-3' },
}

export const CONFIDENCE_META: Record<BodyCorrelationFactor['confidence'], { label: string; cls: string }> = {
  high: { label: 'Fiabilité élevée', cls: 'border-ok/30 bg-ok/12 text-ok' },
  moderate: { label: 'Fiabilité modérée', cls: 'border-info/30 bg-info/12 text-info' },
  low: { label: 'Fiabilité faible', cls: 'border-warn/30 bg-warn/12 text-warn' },
  insufficient: { label: 'Données insuffisantes', cls: 'border-line bg-track text-ink-3' },
}

export function effectiveZoneScore(z: BodyZoneSummary | undefined): number {
  if (!z) return 0
  const candidates = [z.latestScore, z.latestColdNextDayScore, z.maxScore30d].filter(
    (v): v is number => typeof v === 'number' && Number.isFinite(v),
  )
  if (!candidates.length) return 0
  return Math.max(...candidates)
}

export function painScoreTone(score: number | null | undefined): {
  level: 'none' | 'low' | 'warn' | 'danger'
  textCls: string
  badgeCls: string
  svgFill: string
  svgStroke: string
} {
  const s = score ?? 0
  if (s >= 7) {
    return {
      level: 'danger',
      textCls: 'text-danger',
      badgeCls: 'border-danger/35 bg-danger/14 text-danger',
      svgFill: 'color-mix(in oklab, var(--danger) 38%, transparent)',
      svgStroke: 'var(--danger)',
    }
  }
  if (s >= 4) {
    return {
      level: 'warn',
      textCls: 'text-warn',
      badgeCls: 'border-warn/35 bg-warn/14 text-warn',
      svgFill: 'color-mix(in oklab, var(--warn) 34%, transparent)',
      svgStroke: 'var(--warn)',
    }
  }
  if (s > 0) {
    return {
      level: 'low',
      textCls: 'text-ok',
      badgeCls: 'border-ok/35 bg-ok/14 text-ok',
      svgFill: 'color-mix(in oklab, var(--ok) 28%, transparent)',
      svgStroke: 'var(--ok)',
    }
  }
  return {
    level: 'none',
    textCls: 'text-ink-3',
    badgeCls: 'border-line bg-track text-ink-3',
    svgFill: 'color-mix(in oklab, var(--text-1) 6%, transparent)',
    svgStroke: 'var(--line-strong)',
  }
}
