// Simulation déterministe d'une séance (séries temporelles ≤ 400 points).
// Utilisé uniquement en mode mock (VITE_MOCK=1).

import type { Series } from '../api/types'

export function hashStr(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

/** PRNG mulberry32. */
export function rng(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

export type PhaseKind = 'run' | 'walk' | 'stand' | 'warmup' | 'active' | 'rest' | 'cooldown'

export interface Phase {
  kind: PhaseKind
  durationS: number
  pace: number // s/km (ignoré si stand)
  hrBoost?: number // bpm ajoutés à la cible FC (fatigue simulée)
}

export interface RunSpec {
  phases: Phase[]
  structured: boolean // true → blocs (course/marche ou fractionné)
  cadence: number // cadence de course de référence (spm)
  vo: number // oscillation verticale (cm)
  gct: number // temps de contact (ms)
  power: number // puissance à l'allure de référence (W)
  hrRef: number // FC à l'allure de référence
  refPace: number
  drift: number // hausse relative de FC sur la séance (0.03 = +3 %)
  balanceLeft: number | null // null = non mesuré
  elev: { base: number; amp: number; cycles: number; trend: number } | null
  hasPower: boolean
  seed: number
}

export interface SimPoint {
  t: number
  kind: PhaseKind
  dist: number // m cumulés
}

export interface Simulation {
  series: Series
  points: SimPoint[]
  durationS: number
  distanceM: number
}

const RUNNING_KINDS: PhaseKind[] = ['run', 'warmup', 'active', 'cooldown']
export const isRunKind = (k: PhaseKind) => RUNNING_KINDS.includes(k)

export function simulate(spec: RunSpec): Simulation {
  const r = rng(spec.seed)
  const noise = (amp: number) => (r() * 2 - 1) * amp
  const durationS = spec.phases.reduce((s, p) => s + p.durationS, 0)
  const n = Math.min(400, Math.max(60, Math.ceil(durationS / 5)))
  const dt = durationS / (n - 1)

  // bornes des phases
  const bounds: { start: number; end: number; phase: Phase }[] = []
  let acc = 0
  for (const p of spec.phases) {
    bounds.push({ start: acc, end: acc + p.durationS, phase: p })
    acc += p.durationS
  }
  const phaseAt = (t: number) => bounds.find((b) => t >= b.start && t < b.end)?.phase ?? spec.phases[spec.phases.length - 1]

  const series: Series = {
    tS: [],
    hr: [],
    cadenceSpm: [],
    powerW: [],
    paceSecPerKm: [],
    elevationM: [],
    gctMs: [],
    voCm: [],
    gctBalanceLeftPct: [],
  }
  const points: SimPoint[] = []

  let hr = spec.hrRef - 38
  let dist = 0
  let smoothPace = spec.phases[0].pace
  for (let i = 0; i < n; i++) {
    const t = i * dt
    const ph = phaseAt(t)
    const frac = t / durationS
    const running = isRunKind(ph.kind)

    // allure (lissée pour éviter les marches d'escalier)
    const targetPace = ph.kind === 'stand' ? 0 : ph.pace * (1 + noise(0.025))
    if (ph.kind !== 'stand') smoothPace = smoothPace + (targetPace - smoothPace) * Math.min(1, dt / 12)
    const speed = ph.kind === 'stand' ? 0 : 1000 / smoothPace
    if (i > 0) dist += speed * dt

    // FC : réponse du 1er ordre vers la cible
    const intensity = running ? (spec.refPace - ph.pace) * 0.22 : -24
    const target = spec.hrRef + intensity + spec.hrRef * spec.drift * frac + (ph.kind === 'stand' ? -10 : 0) + (ph.hrBoost ?? 0)
    hr += (target - hr) * (1 - Math.exp(-dt / 22))
    const hrVal = Math.round(hr + noise(1.5))

    let cad: number | null = null
    let pw: number | null = null
    let gct: number | null = null
    let vo: number | null = null
    let bal: number | null = null
    if (running) {
      cad = Math.round(spec.cadence + (spec.refPace - ph.pace) * 0.09 + noise(2.6) - frac * 1.2)
      pw = spec.hasPower ? Math.round(spec.power * (spec.refPace / smoothPace) + noise(9)) : null
      gct = Math.round(spec.gct + (ph.pace - spec.refPace) * 0.22 + frac * 6 + noise(5))
      vo = Math.round((spec.vo + frac * 0.35 + noise(0.25)) * 10) / 10
      bal = spec.balanceLeft === null ? null : Math.round((spec.balanceLeft + noise(0.45)) * 10) / 10
    } else if (ph.kind === 'walk') {
      cad = Math.round(112 + noise(3))
      pw = spec.hasPower ? Math.round(138 + noise(10)) : null
    }

    let elev: number | null = null
    if (spec.elev) {
      const e = spec.elev
      elev =
        Math.round(
          (e.base + e.amp * Math.sin(2 * Math.PI * e.cycles * frac) + e.amp * 0.25 * Math.sin(2 * Math.PI * e.cycles * 3.1 * frac + 1) + e.trend * frac) *
            10,
        ) / 10
    }

    series.tS.push(Math.round(t))
    series.hr.push(hrVal)
    series.cadenceSpm.push(cad)
    series.powerW.push(pw)
    series.paceSecPerKm.push(speed < 0.5 ? null : Math.round(smoothPace))
    series.elevationM.push(elev)
    series.gctMs.push(gct)
    series.voCm.push(vo)
    series.gctBalanceLeftPct.push(bal)
    points.push({ t, kind: ph.kind, dist })
  }

  return { series, points, durationS, distanceM: dist }
}

/* ------------------------------------------------------------------ */
/* Agrégats                                                           */
/* ------------------------------------------------------------------ */

export function mean(xs: (number | null)[]): number | null {
  let s = 0
  let c = 0
  for (const x of xs) {
    if (x !== null && Number.isFinite(x)) {
      s += x
      c++
    }
  }
  return c ? s / c : null
}

export function max(xs: (number | null)[]): number | null {
  let m: number | null = null
  for (const x of xs) if (x !== null && (m === null || x > m)) m = x
  return m
}

export function elevGainLoss(xs: (number | null)[]): { gain: number | null; loss: number | null } {
  const vals = xs.filter((x): x is number => x !== null)
  if (vals.length < 2) return { gain: null, loss: null }
  let gain = 0
  let loss = 0
  let ref = vals[0]
  // hystérésis 1 m pour ignorer le bruit
  for (const v of vals) {
    const d = v - ref
    if (d >= 1) {
      gain += d
      ref = v
    } else if (d <= -1) {
      loss -= d
      ref = v
    }
  }
  return { gain: Math.round(gain), loss: Math.round(loss) }
}

export function round(v: number | null, d = 0): number | null {
  if (v === null) return null
  const k = 10 ** d
  return Math.round(v * k) / k
}
