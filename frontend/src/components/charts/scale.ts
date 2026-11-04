export interface Linear {
  (v: number): number
  domain: [number, number]
  range: [number, number]
}

export function linear(domain: [number, number], range: [number, number]): Linear {
  const [d0, d1] = domain
  const [r0, r1] = range
  const k = d1 === d0 ? 0 : (r1 - r0) / (d1 - d0)
  const f = ((v: number) => r0 + (v - d0) * k) as Linear
  f.domain = domain
  f.range = range
  return f
}

/** Pas « rond » (1, 2, 2.5, 5 × 10^n). */
function niceStep(span: number, count: number): number {
  const raw = span / Math.max(1, count)
  const pow = 10 ** Math.floor(Math.log10(raw))
  const n = raw / pow
  const m = n <= 1 ? 1 : n <= 2 ? 2 : n <= 2.5 ? 2.5 : n <= 5 ? 5 : 10
  return m * pow
}

/** Domaine arrondi + graduations propres. */
export function niceDomain(min: number, max: number, count = 3): { domain: [number, number]; ticks: number[] } {
  if (!Number.isFinite(min) || !Number.isFinite(max)) return { domain: [0, 1], ticks: [0, 1] }
  if (min === max) {
    const pad = Math.abs(min) * 0.05 || 1
    min -= pad
    max += pad
  }
  const step = niceStep(max - min, count)
  const lo = Math.floor(min / step) * step
  const hi = Math.ceil(max / step) * step
  const ticks: number[] = []
  for (let v = lo; v <= hi + step / 2; v += step) ticks.push(Math.round(v * 1e6) / 1e6)
  return { domain: [lo, hi], ticks }
}

export function extent(values: (number | null)[], extra: number[] = []): [number, number] | null {
  let lo = Infinity
  let hi = -Infinity
  for (const v of values) {
    if (v === null || !Number.isFinite(v)) continue
    if (v < lo) lo = v
    if (v > hi) hi = v
  }
  for (const v of extra) {
    if (v < lo) lo = v
    if (v > hi) hi = v
  }
  return lo === Infinity ? null : [lo, hi]
}

/** Chemin SVG d'une ligne avec trous (null = coupure). */
export function linePath(xs: number[], ys: (number | null)[], x: (v: number) => number, y: (v: number) => number): string {
  let d = ''
  let pen = false
  for (let i = 0; i < xs.length; i++) {
    const v = ys[i]
    if (v === null || !Number.isFinite(v)) {
      pen = false
      continue
    }
    d += `${pen ? 'L' : 'M'}${x(xs[i]).toFixed(1)},${y(v).toFixed(1)}`
    pen = true
  }
  return d
}

/** Aire sous la courbe par morceaux continus. */
export function areaPath(xs: number[], ys: (number | null)[], x: (v: number) => number, y: (v: number) => number, y0: number): string {
  let d = ''
  let start = -1
  const flush = (end: number) => {
    if (start < 0) return
    d += `M${x(xs[start]).toFixed(1)},${y0}`
    for (let i = start; i <= end; i++) d += `L${x(xs[i]).toFixed(1)},${y(ys[i] as number).toFixed(1)}`
    d += `L${x(xs[end]).toFixed(1)},${y0}Z`
    start = -1
  }
  for (let i = 0; i < xs.length; i++) {
    const v = ys[i]
    if (v === null || !Number.isFinite(v)) flush(i - 1)
    else if (start < 0) start = i
  }
  flush(xs.length - 1)
  return d
}

/** Index du point le plus proche de t (xs trié). */
export function nearestIndex(xs: number[], t: number): number {
  let lo = 0
  let hi = xs.length - 1
  while (hi - lo > 1) {
    const mid = (lo + hi) >> 1
    if (xs[mid] < t) lo = mid
    else hi = mid
  }
  return Math.abs(xs[lo] - t) <= Math.abs(xs[hi] - t) ? lo : hi
}

/** Pas de graduation temporelle (s) pour ~n graduations. */
export function timeStep(totalS: number, maxTicks: number): number {
  const steps = [60, 120, 300, 600, 900, 1200, 1800, 3600, 5400, 7200]
  return steps.find((s) => totalS / s <= maxTicks) ?? 7200
}
