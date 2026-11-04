// Construction des réponses ActivityAnalysis / DayAnalysis à partir du catalogue simulé.
// Reproduit la logique décrite dans docs/API_CONTRACT.md (§3 et §5) de façon simplifiée.

import type {
  ActivityAnalysis,
  ActivityDetail,
  Alert,
  Biomechanics,
  DayAnalysis,
  Drift,
  HistoryRow,
  Load,
  Mode,
  Performance,
  Profile,
  RecoveryContext,
  Segment,
  SegmentKind,
  Segments,
} from '../api/types'
import { addDays, fmtNumber, fmtPace } from '../lib/format'
import { getCatalog, type CatalogItem } from './catalog'
import { elevGainLoss, isRunKind, max, mean, round, type PhaseKind } from './sim'
import { buildDayReport, buildActivityReport } from './report'
import { daySummary, recoveryFor } from './wellness'

const HR_ZONE_FLOORS = [104, 124, 141, 156, 170]
const POWER_ZONE_FLOORS = [0, 230, 275, 315, 360]

/* ------------------------------------------------------------------ */
/* Segments                                                           */
/* ------------------------------------------------------------------ */

function segmentFrom(item: CatalogItem, i0: number, i1: number, index: number, kind: SegmentKind, label: string): Segment {
  const { series: s, points } = item.sim
  // Métriques sur [i0, i1[ (i1 = premier point du segment suivant), bornes temporelles sur [i0, i1].
  const idx: number[] = []
  const last = i1 > i0 && i1 < points.length - 1 ? i1 - 1 : i1
  for (let i = i0; i <= last && i < points.length; i++) idx.push(i)
  const pick = (arr: (number | null)[]) => idx.map((i) => arr[i])
  const runIdx = idx.filter((i) => isRunKind(points[i].kind))
  const pickRun = (arr: (number | null)[]) => runIdx.map((i) => arr[i])
  const t0 = points[i0].t
  const t1 = points[Math.min(i1, points.length - 1)].t
  const dist = points[Math.min(i1, points.length - 1)].dist - points[i0].dist
  const eg = elevGainLoss(pick(s.elevationM))
  const dur = Math.max(1, t1 - t0)
  return {
    index,
    kind,
    label,
    startOffsetS: Math.round(t0),
    durationS: Math.round(dur),
    distanceM: Math.round(dist),
    paceSecPerKm: dist > 20 ? Math.round(dur / (dist / 1000)) : null,
    avgHR: round(mean(pick(s.hr))),
    maxHR: max(pick(s.hr)),
    avgPower: round(mean(pick(s.powerW))),
    avgCadenceSpm: round(mean(pick(s.cadenceSpm))),
    gctMs: round(mean(pickRun(s.gctMs))),
    voCm: round(mean(pickRun(s.voCm)), 1),
    gctBalanceLeftPct: round(mean(pickRun(s.gctBalanceLeftPct)), 1),
    elevGainM: eg.gain,
    elevLossM: eg.loss,
  }
}

const KIND_LABEL: Record<PhaseKind, string> = {
  run: 'Bloc',
  walk: 'Pause',
  stand: 'Arrêt',
  warmup: 'Échauffement',
  active: 'Fraction',
  rest: 'Récup',
  cooldown: 'Retour au calme',
}

function buildSegments(item: CatalogItem): Segments {
  const { points } = item.sim
  const blocks: Segment[] = []
  if (item.spec.structured) {
    const counters: Record<string, number> = {}
    let start = 0
    for (let i = 1; i <= points.length; i++) {
      if (i === points.length || points[i].kind !== points[start].kind) {
        const kind = points[start].kind
        counters[kind] = (counters[kind] ?? 0) + 1
        const numbered = kind === 'run' || kind === 'walk' || kind === 'active' || kind === 'rest' || kind === 'stand'
        const label = numbered ? `${KIND_LABEL[kind]} ${counters[kind]}` : KIND_LABEL[kind]
        blocks.push(segmentFrom(item, start, Math.min(i, points.length - 1), blocks.length + 1, kind, label))
        start = i
      }
    }
  }
  const laps: Segment[] = []
  if (item.sim.distanceM > 0) {
    let start = 0
    let km = 1
    for (let i = 1; i < points.length; i++) {
      if (points[i].dist >= km * 1000 || i === points.length - 1) {
        laps.push(segmentFrom(item, start, i, laps.length + 1, 'lap', `Km ${km}`))
        start = i
        km++
      }
    }
  }
  const runWalk = blocks.filter((b) => b.kind !== 'lap').length
  return { primary: runWalk >= 2 ? 'blocks' : 'laps', blocks, laps }
}

/* ------------------------------------------------------------------ */
/* Biomécanique, charge, performance                                  */
/* ------------------------------------------------------------------ */

function biomechanics(item: CatalogItem, profile: Profile): Biomechanics {
  const { series: s, points } = item.sim
  const runIdx = points.map((p, i) => (isRunKind(p.kind) ? i : -1)).filter((i) => i >= 0)
  const isRunning = item.summary.isRunning
  const cad = runIdx.map((i) => s.cadenceSpm[i]).filter((v): v is number => v !== null)
  const inTarget = cad.filter((c) => c >= profile.cadenceTargetMin && c <= profile.cadenceTargetMax).length
  const below = cad.filter((c) => c < 170).length
  const cadAvg = round(mean(cad))
  const vo = round(mean(runIdx.map((i) => s.voCm[i])), 1)
  const avgSpeed = item.sim.distanceM / item.sim.durationS
  const stride = isRunning && cadAvg ? round((avgSpeed * 1.04) / (cadAvg / 60), 2) : null
  const bal = round(mean(runIdx.map((i) => s.gctBalanceLeftPct[i])), 1)

  const gctBalance: Biomechanics['gctBalance'] =
    bal === null
      ? {
          available: false,
          leftPct: null,
          rightPct: null,
          gapPct: null,
          weakerSide: null,
          alert: null,
          reason: isRunning
            ? 'Non mesuré : nécessite HRM-Pro / HRM 600 / Running Dynamics Pod'
            : 'Non applicable : activité sans dynamique de course',
        }
      : (() => {
          const right = round(100 - bal, 1) as number
          const gap = round(Math.abs(bal - right), 1) as number
          return {
            available: true,
            leftPct: bal,
            rightPct: right,
            gapPct: gap,
            weakerSide: bal < right ? 'left' : bal > right ? 'right' : null,
            alert: gap > profile.gctBalanceAlertPct * 2 ? 'danger' : gap > profile.gctBalanceAlertPct ? 'warn' : 'ok',
            reason: null,
          }
        })()

  return {
    gctBalance,
    groundContactTimeMs: round(mean(runIdx.map((i) => s.gctMs[i]))),
    cadenceAvgSpm: cadAvg,
    cadenceMaxSpm: cad.length ? Math.max(...cad) + 4 : null,
    cadenceInTargetPct: cad.length ? Math.round((inTarget / cad.length) * 100) : null,
    cadenceBelow170Pct: cad.length ? Math.round((below / cad.length) * 100) : null,
    verticalOscillationCm: vo,
    verticalRatioPct: vo !== null && stride ? round((vo / (stride * 100)) * 100, 1) : null,
    strideLengthM: stride,
  }
}

function drift(item: CatalogItem): Drift | null {
  const { series: s, points } = item.sim
  const runIdx = points.map((p, i) => (isRunKind(p.kind) ? i : -1)).filter((i) => i >= 0)
  if (runIdx.length < 30) return null
  const usePower = item.spec.hasPower
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

function bestEffort(item: CatalogItem, meters: number): number | null {
  const { points } = item.sim
  if (item.sim.distanceM < meters) return null
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

function acwr(date: string) {
  const items = getCatalog()
  const sum = (from: string, to: string) =>
    items.filter((it) => it.summary.date > from && it.summary.date <= to).reduce((s, it) => s + (it.summary.trainingLoad ?? 0), 0)
  const acute = sum(addDays(date, -7), date)
  const chronic = Math.round(sum(addDays(date, -28), date) / 4)
  const ratio = chronic > 0 ? Math.round((acute / chronic) * 100) / 100 : null
  const verdict = ratio === null ? 'low' : ratio < 0.8 ? 'low' : ratio <= 1.3 ? 'optimal' : ratio <= 1.5 ? 'high' : 'danger'
  return { acute7d: acute, chronic28d: chronic, ratio, verdict } as Performance['acwr']
}

function performance(item: CatalogItem, profile: Profile, hrZonesS: number[]): Performance {
  const { series: s } = item.sim
  const at = (lo: number, hi: number) => {
    let sec = 0
    let m = 0
    for (let i = 1; i < s.tS.length; i++) {
      const p = s.paceSecPerKm[i]
      if (p !== null && p >= lo && p <= hi) {
        const dt = s.tS[i] - s.tS[i - 1]
        sec += dt
        m += (1000 / p) * dt
      }
    }
    return { seconds: Math.round(sec), distanceM: Math.round(m) }
  }
  const range = (a: number, b: number) => `${fmtPace(a).replace('/km', '')}–${fmtPace(b).replace('/km', '')}`
  const best = [
    { label: '1 km', m: 1000 },
    { label: '1 mile', m: 1609 },
    { label: '5 km', m: 5000 },
  ]
    .map((b) => ({ label: b.label, seconds: bestEffort(item, b.m) }))
    .filter((b): b is { label: string; seconds: number } => b.seconds !== null)
  return {
    avgPower: item.summary.avgPower,
    normPower: item.summary.avgPower ? Math.round(item.summary.avgPower * 1.035) : null,
    timeInZonesS: hrZonesS,
    z4z5S: hrZonesS[3] + hrZonesS[4],
    targetPaces: [
      { label: 'Allure 10 km', rangeLabel: range(profile.pace10kMinSecPerKm, profile.pace10kMaxSecPerKm), ...at(profile.pace10kMinSecPerKm, profile.pace10kMaxSecPerKm) },
      { label: 'Allure marathon', rangeLabel: range(profile.paceMarathonMinSecPerKm, profile.paceMarathonMaxSecPerKm), ...at(profile.paceMarathonMinSecPerKm, profile.paceMarathonMaxSecPerKm) },
    ],
    acwr: acwr(item.summary.date),
    bestEfforts: best,
  }
}

function history(date: string, days: number): HistoryRow[] | null {
  if (days <= 0) return null
  const from = addDays(date, -days)
  return getCatalog()
    .filter((it) => it.summary.date >= from && it.summary.date <= date && it.summary.type !== 'cycling')
    .map((it) => ({
      date: it.summary.date,
      type: it.summary.typeLabel,
      distanceM: it.summary.distanceM,
      durationS: it.summary.durationS,
      paceSecPerKm: it.summary.avgPaceSecPerKm,
      avgHR: it.summary.avgHR,
      avgCadenceSpm: it.summary.avgCadenceSpm,
      elevLossM: it.summary.elevLossM,
      trainingLoad: it.summary.trainingLoad,
    }))
}

/* ------------------------------------------------------------------ */
/* Alertes (§5)                                                       */
/* ------------------------------------------------------------------ */

const LEVEL_ORDER = { danger: 0, warn: 1, info: 2, ok: 3 } as const

export function finalizeAlerts(alerts: Alert[]): Alert[] {
  if (!alerts.some((a) => a.level === 'warn' || a.level === 'danger')) {
    alerts.push({ level: 'ok', code: 'ALL_GOOD', title: 'Aucun signal d’alerte', message: 'Tous les indicateurs sont dans les seuils de ton profil.' })
  }
  return alerts.sort((a, b) => LEVEL_ORDER[a.level] - LEVEL_ORDER[b.level])
}

function recoveryAlerts(rec: RecoveryContext | null, profile: Profile): Alert[] {
  const out: Alert[] = []
  if (!rec) return out
  const h = rec.hrv
  if (h?.status) {
    if (h.status === 'LOW' || h.status === 'POOR') out.push({ level: 'danger', code: 'HRV_LOW', title: 'VFC basse', message: `VFC ${h.lastNightAvgMs} ms, statut ${h.statusLabel} : récupération insuffisante, séance très légère ou repos.` })
    else if (h.status === 'UNBALANCED' || (h.lastNightAvgMs !== null && h.baselineLowMs !== null && h.lastNightAvgMs < h.baselineLowMs))
      out.push({ level: 'warn', code: 'HRV_LOW', title: 'VFC sous la baseline', message: `VFC ${h.lastNightAvgMs} ms sous la baseline (${h.baselineLowMs}–${h.baselineHighMs} ms) : récupération incomplète.` })
  }
  const r = rec.restingHR
  if (r?.delta !== null && r?.delta !== undefined && r.delta >= profile.rhrAlertDeltaBpm)
    out.push({ level: 'warn', code: 'RHR_HIGH', title: 'FC repos élevée', message: `FC repos ${r.bpm} bpm, +${r.delta} vs moyenne 7 j : fatigue ou début d’infection possible.` })
  const sl = rec.sleep
  if (sl?.score !== null && sl?.score !== undefined && sl.score < 60) out.push({ level: 'warn', code: 'SLEEP_LOW', title: 'Sommeil insuffisant', message: `Score sommeil ${sl.score}/100 : nuit peu réparatrice.` })
  if (sl?.deepS !== null && sl?.deepS !== undefined && sl.deepS < 3600) out.push({ level: 'info', code: 'SLEEP_LOW', title: 'Sommeil profond court', message: `Sommeil profond ${Math.round(sl.deepS / 60)} min (< 1 h).` })
  const bb = rec.bodyBattery?.atWake
  if (bb !== null && bb !== undefined) {
    if (bb < 30) out.push({ level: 'danger', code: 'BB_LOW', title: 'Body Battery très basse', message: `Body Battery au réveil ${bb}/100 : réserves très basses, privilégier le repos.` })
    else if (bb < 50) out.push({ level: 'warn', code: 'BB_LOW', title: 'Body Battery basse', message: `Body Battery au réveil ${bb}/100 : réserves basses, séance légère conseillée.` })
  }
  return out
}

function activityAlerts(item: CatalogItem, a: Omit<ActivityAnalysis, 'alerts' | 'report' | 'reportTokensEstimate'>, profile: Profile, mode: Mode): Alert[] {
  const out: Alert[] = []
  const b = a.biomechanics
  if (item.summary.isRunning && mode !== 'recovery') {
    if (b.cadenceAvgSpm !== null) {
      if (b.cadenceAvgSpm < 170) out.push({ level: 'danger', code: 'CADENCE_LOW', title: 'Cadence basse', message: `Cadence ${b.cadenceAvgSpm} spm < 170 : risque d’overstriding et de surcharge rotulienne.` })
      else if (b.cadenceAvgSpm < profile.cadenceTargetMin)
        out.push({ level: 'warn', code: 'CADENCE_LOW', title: 'Cadence sous la cible', message: `Cadence ${b.cadenceAvgSpm} spm sous la cible ${profile.cadenceTargetMin}–${profile.cadenceTargetMax} : raccourcir légèrement la foulée.` })
    }
    if (b.verticalOscillationCm !== null && b.verticalOscillationCm > profile.verticalOscillationAlertCm) {
      const danger = b.verticalOscillationCm > profile.verticalOscillationAlertCm + 1
      out.push({ level: danger ? 'danger' : 'warn', code: 'VO_HIGH', title: 'Oscillation verticale élevée', message: `Oscillation ${fmtNumber(b.verticalOscillationCm, 1)} cm > ${profile.verticalOscillationAlertCm} cm : impacts plus marqués à chaque appui.` })
    }
    const g = b.gctBalance
    if (!g.available) out.push({ level: 'info', code: 'GCT_BALANCE_NA', title: 'Équilibre G/D non mesuré', message: 'Nécessite un HRM-Pro, HRM 600 ou Running Dynamics Pod.' })
    else if (g.alert === 'warn' || g.alert === 'danger') {
      const side = g.weakerSide === 'left' ? 'gauche' : 'droite'
      const injured = g.weakerSide === profile.injuredSide ? ', côté blessé (protection probable)' : ''
      out.push({ level: g.alert, code: 'GCT_BALANCE', title: 'Asymétrie d’appui', message: `Écart G/D ${fmtNumber(g.gapPct, 1)} pt (> ${fmtNumber(profile.gctBalanceAlertPct, 1)}) : contact plus court à ${side}${injured}.` })
    }
  }
  const loss = a.load.elevLossM
  if (loss !== null && loss > profile.descentAlertM) {
    out.push({ level: loss > profile.descentAlertM * 2 ? 'danger' : 'warn', code: 'DESCENT_HIGH', title: 'Descente importante', message: `D− de ${loss} m > ${profile.descentAlertM} m : les descentes augmentent la charge fémoro-patellaire.` })
  }
  const d = a.load.drift
  if (d && d.decouplingPct >= 5) {
    out.push({ level: d.decouplingPct > 10 ? 'danger' : 'warn', code: 'DRIFT_HIGH', title: 'Dérive cardiaque', message: `Découplage ${fmtNumber(d.decouplingPct, 1)} % : fatigue ou chaleur, garder l’intensité basse.` })
  }
  const runs = a.segments.blocks.filter((s) => s.kind === 'run')
  if (runs.length >= 3) {
    const last = runs[runs.length - 1]
    const prev = runs.slice(0, -1)
    const pace = mean(prev.map((s) => s.paceSecPerKm))
    const hr = mean(prev.map((s) => s.avgHR))
    if (pace && hr && last.paceSecPerKm && last.avgHR && last.paceSecPerKm > pace * 1.08 && last.avgHR >= hr) {
      out.push({ level: 'warn', code: 'FADE_END', title: 'Baisse en fin de séance', message: `Dernier bloc ${Math.round((last.paceSecPerKm / pace - 1) * 100)} % plus lent avec une FC égale ou supérieure : fatigue.` })
    }
  }
  if (mode === 'performance' && a.performance?.acwr?.ratio) {
    const r = a.performance.acwr.ratio
    if (r > 1.3) out.push({ level: r > 1.5 ? 'danger' : 'warn', code: 'ACWR', title: 'Charge en hausse rapide', message: `Ratio charge aiguë/chronique ${fmtNumber(r, 2)} : montée de charge rapide, risque de blessure accru.` })
  }
  out.push(...recoveryAlerts(a.recovery, profile))
  return finalizeAlerts(out)
}

/* ------------------------------------------------------------------ */
/* Points d'entrée                                                    */
/* ------------------------------------------------------------------ */

export function buildActivityAnalysis(item: CatalogItem, mode: Mode, historyDays: number, profile: Profile, opts: { now: Date }): ActivityAnalysis {
  const sm = item.summary
  const segments = buildSegments(item)
  const bio = biomechanics(item, profile)
  const hrZonesS = zones(item.sim.series.hr, item.sim.series.tS, HR_ZONE_FLOORS)
  const load: Load = {
    elevGainM: sm.elevGainM,
    elevLossM: sm.elevLossM,
    drift: drift(item),
    trainingLoad: sm.trainingLoad,
    aerobicTE: sm.trainingLoad !== null ? Math.min(5, round(1.4 + sm.trainingLoad / 48, 1) as number) : null,
    anaerobicTE: sm.trainingLoad !== null ? (item.spec.phases.some((p) => p.kind === 'active') ? 1.6 : 0.4) : null,
    hrZonesS,
    powerZonesS: item.spec.hasPower ? zones(item.sim.series.powerW, item.sim.series.tS, POWER_ZONE_FLOORS) : null,
  }
  const detail: ActivityDetail = {
    ...sm,
    locationName: item.locationName,
    movingDurationS: sm.durationS !== null ? sm.durationS - item.spec.phases.filter((p) => p.kind === 'stand').reduce((s, p) => s + p.durationS, 0) : null,
    maxHR: item.maxHR,
    normPower: sm.avgPower ? Math.round(sm.avgPower * 1.035) : null,
    maxCadenceSpm: bio.cadenceMaxSpm,
    aerobicTE: load.aerobicTE,
    anaerobicTE: load.anaerobicTE,
    trainingEffectLabel: load.anaerobicTE && load.anaerobicTE > 1 ? 'TEMPO' : 'AEROBIC_BASE',
    impactLoad: sm.isRunning && sm.trainingLoad !== null ? Math.round(sm.trainingLoad * 1.08) : null,
    steps: sm.isRunning && sm.durationS ? Math.round((sm.durationS / 60) * (sm.avgCadenceSpm ?? 170)) : null,
    calories: sm.durationS ? Math.round((sm.durationS / 60) * 10.5) : null,
    bodyBatteryDelta: sm.trainingLoad !== null ? -Math.round(sm.trainingLoad / 6) : null,
  }

  // Récupération : la nuit se terminant le jour de l'activité.
  const missingSleep = item.summary.name.includes('6×5')
  const recovery = recoveryFor(sm.date, { missingSleep })
  const warnings: string[] = missingSleep ? ['Sommeil indisponible'] : []
  const dataSources = ['activity', 'details', 'sleep', 'hrv', 'bodyBattery', 'dailyHeartRate']
  if (sm.isRunning) dataSources.splice(1, 0, 'fit')
  if (item.spec.elev === null && sm.type === 'treadmill_running') warnings.push('Altitude indisponible (tapis)')
  if (!item.spec.hasPower && sm.isRunning) warnings.push('Puissance non enregistrée')

  const base = {
    mode,
    generatedAt: opts.now.toISOString(),
    activity: detail,
    biomechanics: bio,
    load,
    segments,
    series: item.sim.series,
    recovery,
    performance: mode === 'performance' ? performance(item, profile, hrZonesS) : null,
    history: history(sm.date, historyDays),
    dataSources,
    warnings,
  }
  const alerts = activityAlerts(item, base, profile, mode)
  const report = buildActivityReport({ ...base, alerts }, profile)
  return { ...base, alerts, report, reportTokensEstimate: Math.round(report.length / 4) }
}

export function buildDayAnalysis(date: string, historyDays: number, profile: Profile, opts: { now: Date }): DayAnalysis {
  const recovery = recoveryFor(date)
  const day = daySummary(date)
  const activities = getCatalog()
    .filter((it) => it.summary.date === date)
    .map((it) => it.summary)
    .sort((a, b) => (a.startLocal < b.startLocal ? -1 : 1))
  const alerts = recoveryAlerts(recovery, profile)
  if (day.floorsUp !== null && day.floorsUp > 20)
    alerts.push({ level: 'warn', code: 'FLOORS_HIGH', title: 'Beaucoup d’étages', message: `${day.floorsUp} étages montés (> 20) : charge rotulienne cumulée importante.` })
  const finalAlerts = finalizeAlerts(alerts)
  const base = {
    mode: 'recovery' as const,
    date,
    generatedAt: opts.now.toISOString(),
    recovery,
    day,
    activities,
    history: history(date, historyDays),
    alerts: finalAlerts,
    warnings: activities.length === 0 ? ['Aucune activité enregistrée ce jour'] : [],
  }
  const report = buildDayReport(base)
  return { ...base, report, reportTokensEstimate: Math.round(report.length / 4) }
}

