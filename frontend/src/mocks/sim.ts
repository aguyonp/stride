// Deterministic session simulator (≤ 400 points per series). Mock mode only.

import type { Series } from '../api/types'

export function hashStr(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

/** mulberry32 PRNG. */
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
  pace: number
  hrBoost?: number
}

export interface RunSpec {
  phases: Phase[]
  structured: boolean
  cadence: number
  vo: number
  gct: number
  power: number
  hrRef: number
  refPace: number
  drift: number
  balanceLeft: number | null
  elev: { base: number; amp: number; cycles: number; trend: number } | null
  hasPower: boolean
  /** spm lost per % of downhill grade (overstriding when braking). */
  downhillCadenceLoss?: number
  seed: number
}

interface SimPoint {
  t: number
  kind: PhaseKind
  dist: number
  gradePct: number | null
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

  const bounds: { start: number; end: number; phase: Phase }[] = []
  let acc = 0
  for (const p of spec.phases) {
    bounds.push({ start: acc, end: acc + p.durationS, phase: p })
    acc += p.durationS
  }
  const phaseAt = (t: number) => bounds.find((b) => t >= b.start && t < b.end)?.phase ?? spec.phases[spec.phases.length - 1]

  const series: Series = { tS: [], hr: [], cadenceSpm: [], powerW: [], paceSecPerKm: [], elevationM: [], gctMs: [], voCm: [], gctBalanceLeftPct: [] }
  const points: SimPoint[] = []

  let hr = spec.hrRef - 38
  let dist = 0
  let smoothPace = spec.phases[0].pace
  let prevElev: number | null = null
  let prevDist = 0
  let grade: number | null = null
  for (let i = 0; i < n; i++) {
    const t = i * dt
    const ph = phaseAt(t)
    const frac = t / durationS
    const running = isRunKind(ph.kind)

    const targetPace = ph.kind === 'stand' ? 0 : ph.pace * (1 + noise(0.025))
    if (ph.kind !== 'stand') smoothPace = smoothPace + (targetPace - smoothPace) * Math.min(1, dt / 12)
    const speed = ph.kind === 'stand' ? 0 : 1000 / smoothPace
    if (i > 0) dist += speed * dt

    let elev: number | null = null
    if (spec.elev) {
      const e = spec.elev
      elev = Math.round((e.base + e.amp * Math.sin(2 * Math.PI * e.cycles * frac) + e.amp * 0.25 * Math.sin(2 * Math.PI * e.cycles * 3.1 * frac + 1) + e.trend * frac) * 10) / 10
      if (prevElev !== null && dist - prevDist > 5) grade = ((elev - prevElev) / (dist - prevDist)) * 100
      if (prevElev === null || dist - prevDist > 5) {
        prevElev = elev
        prevDist = dist
      }
    }
    const down = grade !== null && grade < 0 ? Math.min(10, -grade) : 0

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
      cad = Math.round(spec.cadence + (spec.refPace - ph.pace) * 0.09 + noise(2.6) - frac * 1.2 - down * (spec.downhillCadenceLoss ?? 1.2))
      pw = spec.hasPower ? Math.round(spec.power * (spec.refPace / smoothPace) + noise(9)) : null
      gct = Math.round(spec.gct + (ph.pace - spec.refPace) * 0.22 + frac * 6 + noise(5))
      vo = Math.round((spec.vo + frac * 0.35 + noise(0.25)) * 10) / 10
      bal = spec.balanceLeft === null ? null : Math.round((spec.balanceLeft + noise(0.45)) * 10) / 10
    } else if (ph.kind === 'walk') {
      cad = Math.round(112 + noise(3))
      pw = spec.hasPower ? Math.round(138 + noise(10)) : null
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
    points.push({ t, kind: ph.kind, dist, gradePct: grade === null ? null : Math.round(grade * 10) / 10 })
  }

  return { series, points, durationS, distanceM: dist }
}

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
  if (v === null || !Number.isFinite(v)) return null
  const k = 10 ** d
  return Math.round(v * k) / k
}

/** Grade cost factor (simplified Minetti): > 1 uphill, < 1 downhill. */
export function gradeFactor(gradePct: number | null): number {
  if (gradePct === null) return 1
  const g = Math.max(-15, Math.min(15, gradePct))
  return g >= 0 ? 1 + 0.035 * g : Math.max(0.8, 1 + 0.018 * g)
}
