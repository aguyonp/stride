// ActivityView / DayView builders from the simulated catalogue and the mock user data.

import type {
  ActivitySummary,
  ActivityView,
  Alert,
  AthleteProfile,
  Biomechanics,
  CrossLoad,
  DayView,
  Drift,
  FatigueDrift,
  Goal,
  Performance,
  PlannedSession,
  Readiness,
  RecoveryWithReadiness,
  Segment,
  SegmentKind,
  Segments,
  Slope,
  SlopeBucket,
  Stamina,
} from '../api/types'
import { addDays, fmtNumber, fmtPaceValue, todayISO } from '../lib/format'
import type { CatalogItem } from './catalog'
import { presetFor, type UserData } from './db'
import {
  freshness,
  gapBetween,
  gapSeries,
  gapSummary,
  garminOn,
  historyRows,
  loadRatio,
  notesOf,
  plannedWithMatch,
  runIndices,
  sourceMode,
  summarize,
  visibleItems,
  type SourceMode,
} from './domain'
import { elevGainLoss, hashStr, isRunKind, max, mean, rng, round, type PhaseKind } from './sim'
import { daySummary, recoveryFor } from './wellness'

const HR_ZONE_FLOORS = [104, 124, 141, 156, 170]
const POWER_ZONE_FLOORS = [0, 230, 275, 315, 360]
const GARMIN_OFF = 'Garmin non connecté'

/* ---------------------------------- Segments ---------------------------------- */

function segmentFrom(it: CatalogItem, i0: number, i1: number, index: number, kind: SegmentKind, label: string, g: boolean): Segment {
  const { series: s, points } = it.sim
  const idx: number[] = []
  const last = i1 > i0 && i1 < points.length - 1 ? i1 - 1 : i1
  for (let i = i0; i <= last && i < points.length; i++) idx.push(i)
  const pick = (arr: (number | null)[]) => idx.map((i) => arr[i])
  const runIdx = idx.filter((i) => isRunKind(points[i].kind))
  const pickRun = (arr: (number | null)[]) => runIdx.map((i) => arr[i])
  const end = Math.min(i1, points.length - 1)
  const dist = points[end].dist - points[i0].dist
  const dur = Math.max(1, points[end].t - points[i0].t)
  const eg = elevGainLoss(pick(s.elevationM))
  const pace = dist > 20 ? Math.round(dur / (dist / 1000)) : null
  const running = runIdx.length > idx.length / 2
  return {
    index,
    kind,
    label,
    startOffsetS: Math.round(points[i0].t),
    durationS: Math.round(dur),
    distanceM: Math.round(dist),
    paceSecPerKm: pace,
    gapSecPerKm: !running ? null : it.spec.elev ? gapBetween(it, i0, end) : pace,
    avgHR: round(mean(pick(s.hr))),
    maxHR: max(pick(s.hr)),
    avgPower: g ? round(mean(pick(s.powerW))) : null,
    avgCadenceSpm: round(mean(pick(s.cadenceSpm))),
    gctMs: g ? round(mean(pickRun(s.gctMs))) : null,
    voCm: g ? round(mean(pickRun(s.voCm)), 1) : null,
    gctBalanceLeftPct: g ? round(mean(pickRun(s.gctBalanceLeftPct)), 1) : null,
    elevGainM: eg.gain,
    elevLossM: eg.loss,
  }
}

const KIND_LABEL: Record<PhaseKind, string> = { run: 'Bloc', walk: 'Pause', stand: 'Arrêt', warmup: 'Échauffement', active: 'Fraction', rest: 'Récup', cooldown: 'Retour au calme' }

function buildSegments(it: CatalogItem, g: boolean): Segments {
  const { points } = it.sim
  const blocks: Segment[] = []
  if (it.spec.structured) {
    const counters: Record<string, number> = {}
    let start = 0
    for (let i = 1; i <= points.length; i++) {
      if (i === points.length || points[i].kind !== points[start].kind) {
        const kind = points[start].kind
        counters[kind] = (counters[kind] ?? 0) + 1
        const numbered = ['run', 'walk', 'active', 'rest', 'stand'].includes(kind)
        blocks.push(segmentFrom(it, start, Math.min(i, points.length - 1), blocks.length + 1, kind, numbered ? `${KIND_LABEL[kind]} ${counters[kind]}` : KIND_LABEL[kind], g))
        start = i
      }
    }
  }
  const laps: Segment[] = []
  if (it.sim.distanceM > 0 && it.base.distanceM) {
    let start = 0
    let km = 1
    for (let i = 1; i < points.length; i++) {
      if (points[i].dist >= km * 1000 || i === points.length - 1) {
        laps.push(segmentFrom(it, start, i, laps.length + 1, 'lap', `Km ${km}`, g))
        start = i
        km++
      }
    }
  }
  return { primary: blocks.length >= 2 ? 'blocks' : 'laps', blocks, laps }
}

/* ---------------------------------- Biomechanics ---------------------------------- */

function biomechanics(it: CatalogItem, p: AthleteProfile, mode: SourceMode): Biomechanics {
  const { series: s } = it.sim
  const g = garminOn(mode)
  const runIdx = runIndices(it)
  const cad = runIdx.map((i) => s.cadenceSpm[i]).filter((v): v is number => v !== null)
  const t = p.thresholds
  const cadAvg = round(mean(cad))
  const vo = g ? round(mean(runIdx.map((i) => s.voCm[i])), 1) : null
  const speed = it.base.distanceM && it.base.durationS ? it.base.distanceM / it.base.durationS : 0
  const stride = it.isRunning && cadAvg ? round((speed * 1.04 * 60) / cadAvg, 2) : null
  const bal = g ? round(mean(runIdx.map((i) => s.gctBalanceLeftPct[i])), 1) : null
  let gctBalance: Biomechanics['gctBalance']
  if (bal === null) {
    gctBalance = {
      available: false,
      leftPct: null,
      rightPct: null,
      gapPct: null,
      weakerSide: null,
      alert: null,
      reason: !it.isRunning ? 'Non applicable : activité sans dynamique de course' : g ? 'Non mesuré : nécessite HRM-Pro / HRM 600 / Running Dynamics Pod' : `${GARMIN_OFF} : dynamique de course non fournie par Strava`,
    }
  } else {
    const right = round(100 - bal, 1) as number
    const gap = round(Math.abs(bal - right), 1) as number
    gctBalance = { available: true, leftPct: bal, rightPct: right, gapPct: gap, weakerSide: bal < right ? 'left' : bal > right ? 'right' : null, alert: gap > t.gctBalanceGapPct * 2 ? 'danger' : gap > t.gctBalanceGapPct ? 'warn' : 'ok', reason: null }
  }
  return {
    gctBalance,
    groundContactTimeMs: g ? round(mean(runIdx.map((i) => s.gctMs[i]))) : null,
    cadenceAvgSpm: cadAvg,
    cadenceMaxSpm: cad.length ? Math.max(...cad) + 4 : null,
    cadenceInTargetPct: cad.length ? Math.round((cad.filter((c) => c >= t.cadenceTargetMin && c <= t.cadenceTargetMax).length / cad.length) * 100) : null,
    cadenceBelow170Pct: cad.length ? Math.round((cad.filter((c) => c < 170).length / cad.length) * 100) : null,
    verticalOscillationCm: vo,
    verticalRatioPct: vo !== null && stride ? round((vo / (stride * 100)) * 100, 1) : null,
    strideLengthM: stride,
  }
}

function fatigueDrift(it: CatalogItem, g: boolean): FatigueDrift | null {
  const idx = runIndices(it)
  if (!it.isRunning || idx.length < 30) return null
  const third = Math.floor(idx.length / 3)
  const first = idx.slice(0, third)
  const last = idx.slice(-third)
  const s = it.sim.series
  const avg = (ids: number[], arr: (number | null)[], d = 0) => round(mean(ids.map((i) => arr[i])), d)
  const gctStart = g ? avg(first, s.gctMs) : null
  const gctEnd = g ? avg(last, s.gctMs) : null
  const gctDelta = gctStart !== null && gctEnd !== null ? gctEnd - gctStart : null
  const gctPct = gctDelta !== null && gctStart ? round((gctDelta / gctStart) * 100, 1) : null
  const voStart = g ? avg(first, s.voCm, 1) : null
  const voEnd = g ? avg(last, s.voCm, 1) : null
  const cadStart = avg(first, s.cadenceSpm)
  const cadEnd = avg(last, s.cadenceSpm)
  const cadDelta = cadStart !== null && cadEnd !== null ? cadEnd - cadStart : null
  const verdict = gctPct !== null ? (gctPct < 3 ? 'stable' : gctPct <= 5 ? 'moderate' : 'high') : cadDelta !== null && Math.abs(cadDelta) >= 4 ? 'moderate' : 'stable'
  return {
    gctStartMs: gctStart,
    gctEndMs: gctEnd,
    gctDeltaMs: gctDelta,
    gctDeltaPct: gctPct,
    voStartCm: voStart,
    voEndCm: voEnd,
    voDeltaCm: voStart !== null && voEnd !== null ? round(voEnd - voStart, 1) : null,
    cadenceStartSpm: cadStart,
    cadenceEndSpm: cadEnd,
    cadenceDeltaSpm: cadDelta,
    verdict,
  }
}

function slope(it: CatalogItem): Slope | null {
  if (!it.isRunning || !it.spec.elev) return null
  const th = 1.5
  const { points, series: s } = it.sim
  type Acc = { dur: number; dist: number; steps: number; strides: number[] }
  const acc: Record<'flat' | 'downhill' | 'uphill', Acc> = { flat: { dur: 0, dist: 0, steps: 0, strides: [] }, downhill: { dur: 0, dist: 0, steps: 0, strides: [] }, uphill: { dur: 0, dist: 0, steps: 0, strides: [] } }
  for (let i = 1; i < points.length; i++) {
    if (!isRunKind(points[i].kind)) continue
    const cad = s.cadenceSpm[i]
    if (cad === null || cad < 140) continue
    const dt = points[i].t - points[i - 1].t
    const dd = points[i].dist - points[i - 1].dist
    const gr = points[i].gradePct
    const b = gr === null ? 'flat' : gr < -th ? 'downhill' : gr > th ? 'uphill' : 'flat'
    acc[b].dur += dt
    acc[b].dist += dd
    acc[b].steps += (cad / 60) * dt
    if (dt > 0) acc[b].strides.push(dd / ((cad / 60) * dt))
  }
  const bucket = (a: Acc): SlopeBucket | null => {
    if (a.dur < 30 || a.steps === 0) return null
    const sorted = [...a.strides].sort((x, y) => x - y)
    return {
      cadenceSpm: Math.round((a.steps / a.dur) * 60),
      strideM: round(a.dist / a.steps, 2),
      strideMaxM: round(sorted[Math.floor(sorted.length * 0.95)] ?? null, 2),
      paceSecPerKm: a.dist > 0 ? Math.round(a.dur / (a.dist / 1000)) : null,
      durationS: Math.round(a.dur),
      distanceM: Math.round(a.dist),
    }
  }
  const flat = bucket(acc.flat)
  const downhill = bucket(acc.downhill)
  return { thresholdPct: th, flat, downhill, uphill: bucket(acc.uphill), downhillCadenceDropSpm: flat?.cadenceSpm != null && downhill?.cadenceSpm != null ? flat.cadenceSpm - downhill.cadenceSpm : null }
}

/* ---------------------------------- Load ---------------------------------- */

function cardiacDrift(it: CatalogItem, g: boolean): Drift | null {
  const s = it.sim.series
  const runIdx = runIndices(it)
  if (!it.isRunning || runIdx.length < 30) return null
  const usePower = g && it.spec.hasPower
  const half = Math.floor(runIdx.length / 2)
  const ef = (ids: number[]) => {
    const hr = mean(ids.map((i) => s.hr[i]))
    const out = usePower ? mean(ids.map((i) => s.powerW[i])) : mean(ids.map((i) => (s.paceSecPerKm[i] ? 1000 / (s.paceSecPerKm[i] as number) : null)))
    return hr && out ? out / hr : null
  }
  const a = runIdx.slice(0, half)
  const b = runIdx.slice(half)
  const ef1 = ef(a)
  const ef2 = ef(b)
  if (!ef1 || !ef2) return null
  const dec = round(((ef1 - ef2) / ef1) * 100, 1) as number
  const third = Math.floor(runIdx.length / 3)
  const mid = mean(runIdx.slice(third, 2 * third).map((i) => s.hr[i])) ?? 0
  const last = mean(runIdx.slice(2 * third).map((i) => s.hr[i])) ?? 0
  return {
    method: usePower ? 'pw:hr' : 'pace:hr',
    decouplingPct: dec,
    firstHalfHR: Math.round(mean(a.map((i) => s.hr[i])) ?? 0),
    secondHalfHR: Math.round(mean(b.map((i) => s.hr[i])) ?? 0),
    lastThirdHrRiseBpm: Math.round(last - mid),
    verdict: dec < 5 ? 'stable' : dec <= 10 ? 'moderate' : 'high',
  }
}

function zones(values: (number | null)[], tS: number[], floors: number[]): number[] {
  const out = [0, 0, 0, 0, 0]
  for (let i = 1; i < values.length; i++) {
    const v = values[i]
    if (v === null) continue
    let z = -1
    for (let k = floors.length - 1; k >= 0; k--) {
      if (v >= floors[k]) {
        z = k
        break
      }
    }
    if (z >= 0) out[z] += tS[i] - tS[i - 1]
  }
  return out.map(Math.round)
}

function stamina(it: CatalogItem, g: boolean): { summary: Stamina | null; series: (number | null)[] } {
  const n = it.sim.series.tS.length
  if (!g || !it.isRunning) return { summary: null, series: Array.from({ length: n }, () => null) }
  const r = rng(hashStr(`stamina-${it.id}`))
  const start = 95 + Math.round(r() * 4)
  const intensity = Math.max(0, (it.base.avgHR ?? 140) - 120) / 60
  const end = Math.max(5, Math.round(start - ((it.base.durationS ?? 0) / 60) * (0.35 + intensity)))
  const series = it.sim.series.tS.map((t) => Math.round(start - (start - end) * (t / (it.sim.durationS || 1)) + (r() * 2 - 1)))
  return { summary: { startPct: start, endPct: end, minPct: Math.min(end, ...series) }, series }
}

function bestEffort(it: CatalogItem, meters: number): number | null {
  const { points } = it.sim
  if (it.sim.distanceM < meters) return null
  let best: number | null = null
  let j = 0
  for (let i = 0; i < points.length; i++) {
    while (j < points.length && points[j].dist - points[i].dist < meters) j++
    if (j >= points.length) break
    const t = points[j].t - points[i].t
    if (best === null || t < best) best = t
  }
  return best === null ? null : Math.round(best)
}

function performance(it: CatalogItem, p: AthleteProfile, hrZonesS: number[], items: CatalogItem[], mode: SourceMode): Performance {
  const s = it.sim.series
  const g = garminOn(mode)
  const at = (lo: number, hi: number) => {
    let sec = 0
    let m = 0
    for (let i = 1; i < s.tS.length; i++) {
      const pace = s.paceSecPerKm[i]
      if (pace !== null && pace >= lo && pace <= hi) {
        const dt = s.tS[i] - s.tS[i - 1]
        sec += dt
        m += (1000 / pace) * dt
      }
    }
    return { seconds: Math.round(sec), distanceM: Math.round(m) }
  }
  const lr = loadRatio(items, it.date, mode)
  return {
    avgPower: g ? it.base.avgPower : null,
    normPower: g && it.base.avgPower ? Math.round(it.base.avgPower * 1.035) : null,
    timeInZonesS: hrZonesS,
    z4z5S: hrZonesS[3] + hrZonesS[4],
    targetPaces: p.paceTargets.map((t) => ({ label: `Allure ${t.label}`, rangeLabel: `${fmtPaceValue(t.minSecPerKm)}–${fmtPaceValue(t.maxSecPerKm)}`, ...at(t.minSecPerKm, t.maxSecPerKm) })),
    acwr: lr ? { acute7d: lr.acute, chronic28d: lr.chronic, ratio: lr.ratio, verdict: lr.verdict } : null,
    bestEfforts: [
      { label: '1 km', m: 1000 },
      { label: '1 mile', m: 1609 },
      { label: '5 km', m: 5000 },
    ]
      .map((b) => ({ label: b.label, seconds: bestEffort(it, b.m) }))
      .filter((b): b is { label: string; seconds: number } => b.seconds !== null),
  }
}

/* ---------------------------------- Weather, cross-load, recovery ---------------------------------- */

const WX: Record<number, string> = { 0: 'Ciel dégagé', 1: 'Peu nuageux', 2: 'Partiellement nuageux', 3: 'Couvert', 45: 'Brouillard', 61: 'Pluie faible' }
const DIRS = ['N', 'NE', 'E', 'SE', 'S', 'SO', 'O', 'NO']

function weather(it: CatalogItem, enabled: boolean): ActivityView['weather'] {
  if (!enabled || !it.outdoor) return null
  const r = rng(hashStr(`wx-${it.startLocal}`))
  const month = Number(it.date.slice(5, 7))
  const hour = Number(it.startLocal.slice(11, 13))
  const temp = Math.round(([4, 5, 9, 12, 16, 20, 23, 23, 19, 14, 8, 5][month - 1] + (hour >= 12 && hour <= 18 ? 3 : -1) + (r() * 4 - 2)) * 10) / 10
  const wind = Math.round(5 + r() * 20)
  const deg = Math.round(r() * 359)
  const code = [0, 1, 2, 3, 61, 45][Math.floor(r() * 6)]
  return {
    source: 'open-meteo',
    temperatureC: temp,
    apparentC: Math.round((temp - wind * 0.08) * 10) / 10,
    humidityPct: Math.round(55 + r() * 35),
    windKph: wind,
    windGustKph: wind + Math.round(6 + r() * 10),
    windDirDeg: deg,
    precipMm: code === 61 ? Math.round((0.4 + r() * 2) * 10) / 10 : 0,
    weatherCode: code,
    description: WX[code],
    observedAt: new Date(`${it.startLocal.slice(0, 13)}:00:00`).toISOString(),
    windDirLabel: DIRS[Math.round(deg / 45) % 8],
  }
}

function localNow(): string {
  const d = new Date()
  return `${todayISO()}T${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:00`
}

function crossLoad(items: CatalogItem[], d: UserData, planned: PlannedSession[], day: string, windowEnd: string, excludeId: number | null, mode: SourceMode): CrossLoad {
  const prev = addDays(day, -1)
  const windowStart = `${prev}T00:00:00`
  const inWindow = items.filter((x) => x.id !== excludeId && x.startLocal >= windowStart && x.startLocal < windowEnd)
  const g = garminOn(mode)
  const bikeAgg = (list: CatalogItem[]) => ({
    rides: list.length,
    distanceKm: round(list.reduce((s, x) => s + (x.base.distanceM ?? 0), 0) / 1000, 1) ?? 0,
    durationS: list.reduce((s, x) => s + (x.base.durationS ?? 0), 0),
    avgHR: round(mean(list.map((x) => x.base.avgHR))),
    trainingLoad: g && list.length ? list.reduce((s, x) => s + (x.base.trainingLoad ?? 0), 0) : null,
  })
  const bikes = inWindow.filter((x) => x.sport === 'bike')
  const hour = Number(windowEnd.slice(11, 13)) + Number(windowEnd.slice(14, 16)) / 60
  const frac = windowEnd.slice(0, 10) === day ? Math.min(1, hour / 24) : 1
  const sPrev = daySummary(prev)
  const sDay = daySummary(day)
  const scale = (v: number | null) => (v === null ? null : Math.round(v * frac))
  return {
    windowStart,
    windowEnd,
    bike: bikeAgg(bikes),
    bikeByDay: [prev, day].map((date) => ({ date, ...bikeAgg(bikes.filter((x) => x.date === date)) })),
    floors: g ? { prevDayUp: sPrev.floorsUp, prevDayDown: sPrev.floorsDown, dayUp: scale(sDay.floorsUp), dayDown: scale(sDay.floorsDown) } : { prevDayUp: null, prevDayDown: null, dayUp: null, dayDown: null },
    steps: g ? { prevDay: sPrev.steps, day: scale(sDay.steps) } : { prevDay: null, day: null },
    stressPrevDay: g ? sPrev.stressAvg : null,
    otherActivities: inWindow.filter((x) => x.sport !== 'bike').map((x) => summarize(x, d, planned)),
  }
}

function readiness(d: UserData, date: string): Readiness {
  const c = d.checkins[date]
  if (c && c.trainingReadiness !== null) return { score: c.trainingReadiness, status: c.readinessStatus, source: 'manual', reason: null }
  return { score: null, status: null, source: null, reason: 'Non exposé par l’API Garmin — saisie manuelle possible' }
}

function recovery(d: UserData, date: string, mode: SourceMode): RecoveryWithReadiness | null {
  if (!garminOn(mode)) return null
  return { ...recoveryFor(date), readiness: readiness(d, date) }
}

/* ---------------------------------- Alerts ---------------------------------- */

const LEVEL_ORDER = { danger: 0, warn: 1, info: 2, ok: 3 } as const

export function finalizeAlerts(alerts: Alert[]): Alert[] {
  if (!alerts.some((a) => a.level === 'warn' || a.level === 'danger')) alerts.push({ level: 'ok', code: 'ALL_GOOD', title: 'Aucun signal d’alerte', message: 'Tous les indicateurs sont dans les seuils de ton profil.' })
  return alerts.sort((a, b) => LEVEL_ORDER[a.level] - LEVEL_ORDER[b.level])
}

function recoveryAlerts(rec: RecoveryWithReadiness | null, p: AthleteProfile): Alert[] {
  const out: Alert[] = []
  if (!rec) return out
  const h = rec.hrv
  if (h?.status === 'LOW' || h?.status === 'POOR') out.push({ level: 'danger', code: 'HRV_LOW', title: 'VFC basse', message: `VFC ${h.lastNightAvgMs} ms, statut ${h.statusLabel} : récupération insuffisante, séance très légère ou repos.` })
  else if (h && (h.status === 'UNBALANCED' || (h.lastNightAvgMs !== null && h.baselineLowMs !== null && h.lastNightAvgMs < h.baselineLowMs)))
    out.push({ level: 'warn', code: 'HRV_LOW', title: 'VFC sous la baseline', message: `VFC ${h.lastNightAvgMs} ms hors baseline (${h.baselineLowMs}–${h.baselineHighMs} ms) : récupération incomplète.` })
  const r = rec.restingHR
  if (r?.delta != null && r.delta >= p.thresholds.rhrDeltaMaxBpm) out.push({ level: 'warn', code: 'RHR_HIGH', title: 'FC repos élevée', message: `FC repos ${r.bpm} bpm, +${r.delta} vs moyenne 7 j : fatigue ou début d’infection possible.` })
  const sl = rec.sleep
  if (sl?.score != null && sl.score < 60) out.push({ level: 'warn', code: 'SLEEP_LOW', title: 'Sommeil insuffisant', message: `Score sommeil ${sl.score}/100 : nuit peu réparatrice.` })
  const bb = rec.bodyBattery?.atWake
  if (bb != null && bb < 50) out.push({ level: bb < 30 ? 'danger' : 'warn', code: 'BB_LOW', title: 'Body Battery basse', message: `Body Battery au réveil ${bb}/100 : réserves ${bb < 30 ? 'très basses, privilégier le repos' : 'basses, séance légère conseillée'}.` })
  return out
}

function painAlerts(n: ActivityView['notes'], p: AthleteProfile, labels: { during: string; nextMorning: string }): Alert[] {
  const out: Alert[] = []
  if (p.focusArea === 'none') return out
  if (n.painDuring !== null && n.painDuring >= 3) out.push({ level: n.painDuring >= 5 ? 'danger' : 'warn', code: 'PAIN_HIGH', title: 'Douleur pendant l’effort', message: `${labels.during} : ${fmtNumber(n.painDuring, 1, { trim: true })}/10 (seuil 3/10) : réduire volume et intensité.` })
  if (n.painNextMorning !== null && n.painDuring !== null && n.painNextMorning > n.painDuring)
    out.push({ level: 'warn', code: 'PAIN_NEXT_MORNING', title: 'Douleur en hausse le lendemain', message: `${labels.nextMorning} ${fmtNumber(n.painNextMorning, 1, { trim: true })}/10 > pendant l’effort ${fmtNumber(n.painDuring, 1, { trim: true })}/10 : la charge n’a pas été tolérée.` })
  return out
}

function activityAlerts(it: CatalogItem, v: Omit<ActivityView, 'alerts'>, p: AthleteProfile): Alert[] {
  const out: Alert[] = []
  const t = p.thresholds
  const injury = p.focusArea !== 'none'
  const b = v.biomechanics
  if (it.isRunning) {
    if (b.cadenceAvgSpm !== null) {
      if (b.cadenceAvgSpm < t.cadenceMinSpm) out.push({ level: 'danger', code: 'CADENCE_LOW', title: 'Cadence basse', message: `Cadence ${b.cadenceAvgSpm} spm < ${t.cadenceMinSpm} : risque d’overstriding et de surcharge articulaire.` })
      else if (b.cadenceAvgSpm < t.cadenceTargetMin) out.push({ level: 'warn', code: 'CADENCE_LOW', title: 'Cadence sous la cible', message: `Cadence ${b.cadenceAvgSpm} spm sous la cible ${t.cadenceTargetMin}–${t.cadenceTargetMax} : raccourcir légèrement la foulée.` })
    }
    const drop = b.slope?.downhillCadenceDropSpm
    if (injury && drop != null && drop > t.downhillCadenceDropSpm) out.push({ level: 'warn', code: 'DOWNHILL_CADENCE_DROP', title: 'Cadence qui chute en descente', message: `−${drop} spm en descente vs plat (seuil ${t.downhillCadenceDropSpm}) : foulée allongée et freinage marqué.` })
    const fd = b.fatigueDrift
    if (fd?.gctDeltaPct != null && fd.gctDeltaPct > t.gctDriftMaxPct)
      out.push({ level: fd.gctDeltaPct > t.gctDriftMaxPct * 2 ? 'danger' : 'warn', code: 'GCT_DRIFT', title: 'Dérive du temps de contact', message: `GCT +${fmtNumber(fd.gctDeltaPct, 1)} % entre le début et la fin (seuil ${t.gctDriftMaxPct} %) : fatigue neuromusculaire.` })
    if (b.verticalOscillationCm !== null && b.verticalOscillationCm > t.verticalOscillationMaxCm)
      out.push({ level: 'warn', code: 'VO_HIGH', title: 'Oscillation verticale élevée', message: `Oscillation ${fmtNumber(b.verticalOscillationCm, 1)} cm > ${fmtNumber(t.verticalOscillationMaxCm, 1)} cm : impacts plus marqués à chaque appui.` })
    const gb = b.gctBalance
    if (!gb.available) out.push({ level: 'info', code: 'GCT_BALANCE_NA', title: 'Équilibre G/D non mesuré', message: gb.reason ?? 'Donnée non fournie.' })
    else if (gb.alert === 'warn' || gb.alert === 'danger') out.push({ level: gb.alert, code: 'GCT_BALANCE', title: 'Asymétrie d’appui', message: `Écart G/D ${fmtNumber(gb.gapPct, 1)} pt (> ${fmtNumber(t.gctBalanceGapPct, 1)}) : contact plus court à ${gb.weakerSide === 'left' ? 'gauche' : 'droite'}.` })
  }
  const loss = v.load.elevLossM
  if (injury && it.isRunning && loss !== null && loss > t.descentMaxM) out.push({ level: loss > t.descentMaxM * 2 ? 'danger' : 'warn', code: 'DESCENT_HIGH', title: 'Descente importante', message: `D− de ${loss} m > ${t.descentMaxM} m : charge excentrique accrue.` })
  const fl = v.crossLoad.floors.prevDayUp
  if (injury && fl !== null && fl > t.floorsPrevDayMax) out.push({ level: 'warn', code: 'FLOORS_HIGH', title: 'Beaucoup d’étages la veille', message: `${fl} étages montés à J-1 (> ${t.floorsPrevDayMax}) : charge cumulée avant la séance.` })
  const lr = v.load.loadRatio
  if (lr?.ratio != null && lr.ratio > t.loadRatioMax) out.push({ level: lr.ratio > 1.5 ? 'danger' : 'warn', code: 'LOAD_RATIO', title: 'Charge en hausse rapide', message: `Ratio aigu/chronique ${fmtNumber(lr.ratio, 2)} > ${fmtNumber(t.loadRatioMax, 1)} : montée de charge rapide.` })
  if (v.plan?.status === 'partial') out.push({ level: 'warn', code: 'PLAN_DEVIATION', title: 'Écart avec la séance prévue', message: v.plan.summary })
  const d = v.load.drift
  if (d && d.decouplingPct >= 5) out.push({ level: d.decouplingPct > 10 ? 'danger' : 'warn', code: 'DRIFT_HIGH', title: 'Dérive cardiaque', message: `Découplage ${fmtNumber(d.decouplingPct, 1)} % : fatigue ou chaleur, garder l’intensité basse.` })
  const runs = v.segments.blocks.filter((s) => s.kind === 'run')
  if (runs.length >= 3) {
    const last = runs[runs.length - 1]
    const pace = mean(runs.slice(0, -1).map((s) => s.paceSecPerKm))
    const hr = mean(runs.slice(0, -1).map((s) => s.avgHR))
    if (pace && hr && last.paceSecPerKm && last.avgHR && last.paceSecPerKm > pace * 1.08 && last.avgHR >= hr)
      out.push({ level: 'warn', code: 'FADE_END', title: 'Baisse en fin de séance', message: `Dernier bloc ${Math.round((last.paceSecPerKm / pace - 1) * 100)} % plus lent avec une FC égale ou supérieure : fatigue.` })
  }
  const preset = presetFor(p.focusArea)
  out.push(...painAlerts(v.notes, p, preset.painLabels))
  out.push(...recoveryAlerts(v.recovery, p))
  return finalizeAlerts(out)
}

/* ---------------------------------- Builders ---------------------------------- */

export function activeGoals(d: UserData): Goal[] {
  return d.goals.filter((g) => g.status === 'active')
}

export function upcomingAfter(planned: PlannedSession[], date: string, n = 3): PlannedSession[] {
  return planned.filter((p) => p.date > date && p.sport !== 'rest').slice(0, n)
}

export function buildActivityView(it: CatalogItem, d: UserData, today = todayISO()): ActivityView {
  const mode = sourceMode(d)
  const g = garminOn(mode)
  const items = visibleItems(d, today)
  const planned = plannedWithMatch(d, items, today)
  const p = d.profile
  const summary = summarize(it, d, planned)
  const notes = notesOf(d, it.id)
  const bio = biomechanics(it, p, mode)
  const st = stamina(it, g)
  const hrZonesS = zones(it.sim.series.hr, it.sim.series.tS, HR_ZONE_FLOORS)
  const stravaOn = mode !== 'garmin' && it.strava !== null
  const gear = d.shoes.find((s) => s.stravaGearId && notes.shoe?.id === s.id) ?? (it.isRunning ? d.shoes.find((s) => s.stravaGearId) : undefined)
  const load: ActivityView['load'] = {
    elevGainM: it.base.elevGainM,
    elevLossM: it.base.elevLossM,
    drift: cardiacDrift(it, g),
    trainingLoad: g ? it.base.trainingLoad : null,
    aerobicTE: g && it.base.trainingLoad !== null ? Math.min(5, round(1.4 + it.base.trainingLoad / 48, 1) as number) : null,
    anaerobicTE: g && it.base.trainingLoad !== null ? (it.spec.phases.some((ph) => ph.kind === 'active') ? 1.6 : 0.4) : null,
    hrZonesS,
    powerZonesS: g && it.spec.hasPower ? zones(it.sim.series.powerW, it.sim.series.tS, POWER_ZONE_FLOORS) : null,
    impactLoad: g && it.isRunning && it.base.trainingLoad !== null ? Math.round(it.base.trainingLoad * 1.08) : null,
    stamina: st.summary,
    loadRatio: loadRatio(items, it.date, mode),
    stravaRelativeEffort: stravaOn ? (it.strava?.relativeEffort ?? null) : null,
  }
  const s = it.sim.series
  const series: ActivityView['series'] = {
    ...s,
    powerW: g ? s.powerW : s.powerW.map(() => null),
    gctMs: g ? s.gctMs : s.gctMs.map(() => null),
    voCm: g ? s.voCm : s.voCm.map(() => null),
    gctBalanceLeftPct: g ? s.gctBalanceLeftPct : s.gctBalanceLeftPct.map(() => null),
    gapSecPerKm: gapSeries(it),
    staminaPct: st.series,
  }
  const match = planned.find((pl) => pl.match?.activityId === it.id)?.match ?? null
  const warnings: string[] = []
  if (mode === 'strava') {
    warnings.push(`${GARMIN_OFF} : dynamique de course (GCT, OV, équilibre G/D), stamina, Training Load et récupération indisponibles`)
    if (it.isRunning) warnings.push('GAP calculé par Stride (modèle de coût de pente standard)')
  }
  if (mode === 'garmin' && d.integrations.strava.available) warnings.push('Strava non connecté : RPE, Relative Effort et notes Strava indisponibles')
  if (!it.outdoor) warnings.push(it.isRunning ? 'Séance en intérieur : météo et pente non applicables (GAP = allure)' : 'Séance en intérieur : météo non applicable')
  if (g && it.isRunning && !it.spec.hasPower) warnings.push('Puissance non enregistrée')
  const dataSources = [
    ...(g ? ['garmin:activity', 'garmin:details', ...(it.isRunning ? ['garmin:fit'] : []), 'garmin:sleep', 'garmin:hrv', 'garmin:dailySummary'] : []),
    ...(stravaOn ? ['strava:activity', 'strava:streams'] : []),
    ...(it.outdoor && d.integrations.weather.enabled ? ['open-meteo'] : []),
  ]
  const base: Omit<ActivityView, 'alerts'> = {
    generatedAt: new Date().toISOString(),
    freshness: freshness('activity', it.date, today),
    sourceLabel: mode === 'strava' ? 'Strava' : mode === 'garmin' || it.stravaId === null ? 'Garmin' : 'Fusion',
    activity: {
      ...summary,
      locationName: it.locationName,
      movingDurationS: it.base.durationS !== null ? it.base.durationS - it.spec.phases.filter((ph) => ph.kind === 'stand').reduce((a, ph) => a + ph.durationS, 0) : null,
      maxHR: it.maxHR,
      normPower: g && it.base.avgPower ? Math.round(it.base.avgPower * 1.035) : null,
      maxCadenceSpm: bio.cadenceMaxSpm,
      aerobicTE: load.aerobicTE,
      anaerobicTE: load.anaerobicTE,
      trainingEffectLabel: g && load.anaerobicTE !== null ? (load.anaerobicTE > 1 ? 'TEMPO' : 'AEROBIC_BASE') : null,
      impactLoad: load.impactLoad,
      steps: g && it.isRunning && it.base.durationS ? Math.round((it.base.durationS / 60) * (it.base.avgCadenceSpm ?? 170)) : null,
      calories: it.base.durationS ? Math.round((it.base.durationS / 60) * 10.5) : null,
      bodyBatteryDelta: g && it.base.trainingLoad !== null ? -Math.round(it.base.trainingLoad / 6) : null,
      gapSecPerKm: gapSummary(it),
      stamina: st.summary,
      strava: stravaOn && it.strava ? { ...it.strava, gearName: gear ? `${gear.brand} ${gear.model}` : null } : null,
    },
    weather: weather(it, d.integrations.weather.enabled),
    biomechanics: { ...bio, fatigueDrift: fatigueDrift(it, g), slope: slope(it) },
    load,
    segments: buildSegments(it, g),
    series,
    crossLoad: crossLoad(items, d, planned, it.date, it.startLocal, it.id, mode),
    recovery: recovery(d, it.date, mode),
    performance: performance(it, p, hrZonesS, items, mode),
    plan: match,
    notes,
    goals: activeGoals(d),
    upcoming: upcomingAfter(planned, it.date),
    history: historyRows(d, items, it.date, mode),
    warnings,
    dataSources,
  }
  return { ...base, alerts: activityAlerts(it, base, p) }
}

export function buildDayView(date: string, d: UserData, today = todayISO()): DayView {
  const mode = sourceMode(d)
  const g = garminOn(mode)
  const items = visibleItems(d, today)
  const planned = plannedWithMatch(d, items, today)
  const rec = recovery(d, date, mode)
  const day = g ? daySummary(date) : { steps: null, stepGoal: null, floorsUp: null, floorsDown: null, intensityModerateMin: null, intensityVigorousMin: null, activeCalories: null, stressAvg: null, bodyBatteryCharged: null, bodyBatteryDrained: null }
  const windowEnd = date === today ? localNow() : `${date}T23:59:59`
  const cl = crossLoad(items, d, planned, date, windowEnd, null, mode)
  const activities: ActivitySummary[] = items
    .filter((it) => it.date === date)
    .map((it) => summarize(it, d, planned))
    .sort((a, b) => (a.startLocal < b.startLocal ? -1 : 1))
  const p = d.profile
  const t = p.thresholds
  const alerts = recoveryAlerts(rec, p)
  if (p.focusArea !== 'none' && cl.floors.prevDayUp !== null && cl.floors.prevDayUp > t.floorsPrevDayMax)
    alerts.push({ level: 'warn', code: 'FLOORS_HIGH', title: 'Beaucoup d’étages la veille', message: `${cl.floors.prevDayUp} étages montés à J-1 (> ${t.floorsPrevDayMax}).` })
  const lr = loadRatio(items, date, mode)
  if (lr?.ratio != null && lr.ratio > t.loadRatioMax) alerts.push({ level: lr.ratio > 1.5 ? 'danger' : 'warn', code: 'LOAD_RATIO', title: 'Charge en hausse rapide', message: `Ratio aigu/chronique ${fmtNumber(lr.ratio, 2)} > ${fmtNumber(t.loadRatioMax, 1)}.` })
  const lastRun = items.find((it) => it.isRunning && it.date < date && it.date >= addDays(date, -2))
  if (lastRun) alerts.push(...painAlerts(notesOf(d, lastRun.id), p, presetFor(p.focusArea).painLabels))
  const missed = planned.filter((pl) => pl.date === date && pl.match?.status === 'missed')
  for (const m of missed) alerts.push({ level: 'info', code: 'PLAN_MISSED', title: 'Séance prévue non réalisée', message: m.title })
  const warnings: string[] = []
  if (!g) warnings.push(`${GARMIN_OFF} : sommeil, VFC, Body Battery, pas et étages indisponibles`)
  if (activities.length === 0) warnings.push('Aucune activité enregistrée ce jour')
  return {
    date,
    generatedAt: new Date().toISOString(),
    freshness: freshness('day', date, today),
    recovery: rec,
    day,
    crossLoad: cl,
    activities,
    planned: planned.filter((pl) => pl.date === date),
    checkin: d.checkins[date] ?? null,
    goals: activeGoals(d),
    upcoming: upcomingAfter(planned, date),
    alerts: finalizeAlerts(alerts),
    warnings,
  }
}

