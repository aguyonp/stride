// Génération du rapport markdown compact (format §4 du contrat) — version mock.
// Le vrai rapport est produit par le backend Go ; ceci sert à avoir un export réaliste en dev.

import type { ActivityAnalysis, Alert, DayAnalysis, HistoryRow, Profile, RecoveryContext, Segment } from '../api/types'
import { addDays, fmtDayMonth, fmtDuration, fmtHM, fmtKm, fmtNumber, fmtPace, isNum, ND, parseLocal } from '../lib/format'

// Espaces normaux dans le rapport (plus robuste pour un copier-coller dans un chat IA).
const plain = (s: string) => s.replace(/[  ]/g, ' ').replace(/−/g, '-')
const n = (v: number | null | undefined, d = 0) => plain(fmtNumber(v, d))
const u = (v: number | null | undefined, unit: string, d = 0) => (isNum(v) ? `${n(v, d)} ${unit}` : ND)
const km = (m: number | null | undefined) => plain(fmtKm(m))
const MODE_LABEL = { sfp: 'Rééducation SFP', recovery: 'Santé & Récupération', performance: 'Performance' } as const
const EMOJI: Record<Alert['level'], string> = { warn: '⚠️', danger: '🛑', info: 'ℹ️', ok: '✅' }

function dateTimeFR(startLocal: string): string {
  const p = parseLocal(startLocal)
  if (!p) return ND
  return `${String(p.d).padStart(2, '0')}/${String(p.mo).padStart(2, '0')}/${p.y} ${String(p.h).padStart(2, '0')}:${String(p.mi).padStart(2, '0')}`
}

function header(a: Omit<ActivityAnalysis, 'report' | 'reportTokensEstimate'>): string[] {
  const x = a.activity
  return [
    `### RAPPORT SÉANCE GARMIN - ${dateTimeFR(x.startLocal)}`,
    `- **Mode :** ${MODE_LABEL[a.mode]}`,
    `- **Séance :** ${km(x.distanceM)} en ${fmtDuration(x.durationS)} | Allure moy: ${fmtPace(x.avgPaceSecPerKm)} | Puissance: ${u(x.avgPower, 'W')}`,
    `- **Dénivelé :** ${isNum(a.load.elevGainM) ? `+${a.load.elevGainM} m` : ND} / ${isNum(a.load.elevLossM) ? `-${a.load.elevLossM} m` : ND}`,
  ]
}

function balanceLine(a: Omit<ActivityAnalysis, 'report' | 'reportTokensEstimate'>): string {
  const g = a.biomechanics.gctBalance
  if (!g.available) return 'N/D (non mesuré par le capteur)'
  return `${n(g.leftPct, 1)} % G / ${n(g.rightPct, 1)} % D (écart ${n(g.gapPct, 1)} pt)`
}

function biomech(a: Omit<ActivityAnalysis, 'report' | 'reportTokensEstimate'>, p: Profile): string[] {
  const b = a.biomechanics
  return [
    '### BIOMÉCANIQUE ROTULE & IMPACTS',
    `- **Équilibre contact sol (G/D) :** ${balanceLine(a)}`,
    `- **Temps de contact sol :** ${u(b.groundContactTimeMs, 'ms')}`,
    `- **Cadence moyenne :** ${u(b.cadenceAvgSpm, 'spm')} (Max : ${u(b.cadenceMaxSpm, 'spm')}) | Dans la cible ${p.cadenceTargetMin}-${p.cadenceTargetMax} : ${isNum(b.cadenceInTargetPct) ? `${b.cadenceInTargetPct} %` : ND}`,
    `- **Oscillation verticale :** ${u(b.verticalOscillationCm, 'cm', 1)} | **Ratio vertical :** ${u(b.verticalRatioPct, '%', 1)}`,
    `- **Longueur de foulée moyenne :** ${u(b.strideLengthM, 'm', 2)}`,
  ]
}

function biomechShort(a: Omit<ActivityAnalysis, 'report' | 'reportTokensEstimate'>): string {
  const b = a.biomechanics
  const g = b.gctBalance
  return `- Cadence ${u(b.cadenceAvgSpm, 'spm')} | GCT ${u(b.groundContactTimeMs, 'ms')} | OV ${u(b.verticalOscillationCm, 'cm', 1)} | RV ${u(b.verticalRatioPct, '%', 1)} | Foulée ${u(b.strideLengthM, 'm', 2)} | G/D ${g.available ? `${n(g.leftPct, 1)}/${n(g.rightPct, 1)}` : ND}`
}

function segRow(s: Segment): string {
  const pause = s.kind === 'walk' || s.kind === 'stand' || s.kind === 'rest'
  const allure = s.kind === 'walk' ? 'Marche' : s.kind === 'stand' ? 'Arrêt' : fmtPace(s.paceSecPerKm)
  return `| ${s.label} | ${fmtDuration(s.durationS)} | ${km(s.distanceM)} | ${allure} | ${u(s.avgHR, 'bpm')} | ${pause ? '—' : u(s.avgPower, 'W')} | ${pause ? '—' : u(s.avgCadenceSpm, 'spm')} |`
}

function splits(a: Omit<ActivityAnalysis, 'report' | 'reportTokensEstimate'>): string[] {
  const segs = a.segments.primary === 'blocks' ? a.segments.blocks : a.segments.laps
  if (!segs.length) return []
  return ['### DÉTAIL DES INTERVALLES / SPLITS', '| Bloc | Durée | Distance | Allure | FC moy | Watts | Cadence |', '|---|---|---|---|---|---|---|', ...segs.map(segRow)]
}

function driftLine(a: Omit<ActivityAnalysis, 'report' | 'reportTokensEstimate'>, withRise: boolean): string {
  const d = a.load.drift
  if (!d) return `- **Dérive cardiaque :** ${ND}`
  const verdict = { stable: 'stable', moderate: 'modérée', high: 'élevée' }[d.verdict]
  const sign = d.decouplingPct >= 0 ? '+' : '-'
  const base = `- **Dérive cardiaque (${d.method === 'pw:hr' ? 'Pw:Hr' : 'Allure:FC'}) :** ${sign}${n(Math.abs(d.decouplingPct), 1)} % (${verdict})`
  return withRise ? `${base} | FC dernier tiers : ${d.lastThirdHrRiseBpm >= 0 ? '+' : '-'}${Math.abs(d.lastThirdHrRiseBpm)} bpm` : base
}

function charge(a: Omit<ActivityAnalysis, 'report' | 'reportTokensEstimate'>): string[] {
  return [
    '### CHARGE & FATIGUE',
    driftLine(a, true),
    `- **Training Load :** ${n(a.load.trainingLoad)} | **TE aérobie :** ${n(a.load.aerobicTE, 1)} | **Impact load :** ${n(a.activity.impactLoad)}`,
  ]
}

export function recoverySection(r: RecoveryContext | null): string[] {
  if (!r) return []
  const prev = addDays(r.date, -1)
  const h = r.hrv
  const s = r.sleep
  const rhr = r.restingHR
  return [
    `### RÉCUPÉRATION & PHYSIOLOGIE (NUIT DU ${fmtDayMonth(prev)} AU ${fmtDayMonth(r.date)})`,
    h
      ? `- **VFC nocturne :** ${u(h.lastNightAvgMs, 'ms')} (Statut : ${h.statusLabel ?? ND} vs baseline ${h.baselineLowMs ?? ND}-${h.baselineHighMs ?? ND} ms | moy. 7 j : ${u(h.weeklyAvgMs, 'ms')})`
      : `- **VFC nocturne :** ${ND}`,
    s ? `- **Score Sommeil :** ${isNum(s.score) ? `${s.score}/100` : ND} (Dont profond : ${fmtHM(s.deepS).replace(/ /g, ' ')}) | Durée : ${fmtHM(s.durationS)}` : `- **Score Sommeil :** ${ND}`,
    `- **FC repos nocturne :** ${u(rhr?.bpm, 'bpm')} (moy. 7 j : ${u(rhr?.avg7d, 'bpm')}) | **Body Battery réveil :** ${isNum(r.bodyBattery?.atWake) ? `${r.bodyBattery?.atWake}/100` : ND}`,
  ]
}

function trendSection(r: RecoveryContext): string[] {
  const label: Record<string, string> = { BALANCED: 'Équilibré', UNBALANCED: 'Déséquilibré', LOW: 'Bas', POOR: 'Faible' }
  return [
    '### TENDANCE 7 JOURS',
    '| Jour | VFC | Statut | Sommeil | Profond | FC repos | Pas | Étages |',
    '|---|---|---|---|---|---|---|---|',
    ...r.trend.map(
      (t) =>
        `| ${fmtDayMonth(t.date)} | ${u(t.hrvMs, 'ms')} | ${t.hrvStatus ? (label[t.hrvStatus] ?? t.hrvStatus) : ND} | ${n(t.sleepScore)} | ${plain(fmtHM(t.deepS))} | ${n(t.restingHR)} | ${n(t.steps)} | ${n(t.floorsUp)} |`,
    ),
  ]
}

function historySection(rows: HistoryRow[] | null, days: number): string[] {
  if (!rows) return []
  return [
    `### HISTORIQUE ${days} JOURS`,
    '| Date | Type | Distance | Durée | Allure | FC | Cadence | D- | Load |',
    '|---|---|---|---|---|---|---|---|---|',
    ...rows.map(
      (h) =>
        `| ${fmtDayMonth(h.date)} | ${h.type} | ${km(h.distanceM)} | ${fmtDuration(h.durationS)} | ${fmtPace(h.paceSecPerKm)} | ${n(h.avgHR)} | ${n(h.avgCadenceSpm)} | ${isNum(h.elevLossM) ? `${h.elevLossM} m` : ND} | ${n(h.trainingLoad)} |`,
    ),
  ]
}

function alertsSection(alerts: Alert[]): string[] {
  return ['### ALERTES', ...alerts.map((a) => `- ${EMOJI[a.level]} ${a.title} : ${plain(a.message)}`)]
}

function perfSection(a: Omit<ActivityAnalysis, 'report' | 'reportTokensEstimate'>): string[] {
  const p = a.performance
  if (!p) return []
  const z = p.timeInZonesS
  const zones = z ? `${z.map((s, i) => `Z${i + 1} ${fmtDuration(s)}`).join(' | ')} (Z4+Z5 : ${fmtDuration(p.z4z5S)})` : ND
  const paces = p.targetPaces.map((t) => `**${t.label} (${t.rangeLabel.replace('–', '-')}) :** ${fmtDuration(t.seconds)}${t.distanceM > 0 ? ` sur ${km(t.distanceM)}` : ''}`).join(' | ')
  const ac = p.acwr
  const verdict = ac ? { low: 'faible', optimal: 'optimal', high: 'élevé', danger: 'danger' }[ac.verdict] : ''
  return [
    '### PERFORMANCE',
    `- **Puissance :** moy ${u(p.avgPower, 'W')} | normalisée ${u(p.normPower, 'W')}`,
    `- **Zones FC :** ${zones}`,
    `- ${paces}`,
    driftLine(a, false),
    `- **Charge aiguë/chronique :** ${ac ? `${ac.acute7d} / ${ac.chronic28d} (ACWR ${n(ac.ratio, 2)} – ${verdict})` : ND}`,
    `- **Training Load séance :** ${n(a.load.trainingLoad)} | **TE :** ${n(a.load.aerobicTE, 1)} aéro / ${n(a.load.anaerobicTE, 1)} anaéro`,
    ...(p.bestEfforts.length ? [`- **Meilleurs efforts :** ${p.bestEfforts.map((b) => `${b.label} ${fmtDuration(b.seconds)}`).join(' | ')}`] : []),
  ]
}

export function buildActivityReport(a: Omit<ActivityAnalysis, 'report' | 'reportTokensEstimate'>, p: Profile): string {
  const historyDays = a.history ? 14 : 0
  let lines: string[]
  if (a.mode === 'performance') {
    lines = [...header(a), '', ...perfSection(a), '', '### BIOMÉCANIQUE (RÉSUMÉ)', biomechShort(a), '', ...splits(a), '', ...recoverySection(a.recovery)]
  } else if (a.mode === 'recovery') {
    lines = [...header(a), biomechShort(a), '', ...recoverySection(a.recovery), '', ...(a.recovery ? trendSection(a.recovery) : [])]
  } else {
    lines = [...header(a), '', ...biomech(a, p), '', ...splits(a), '', ...charge(a), '', ...recoverySection(a.recovery)]
  }
  lines.push('', ...historySection(a.history, historyDays), '', ...alertsSection(a.alerts))
  return tidy(lines)
}

export function buildDayReport(d: Omit<DayAnalysis, 'report' | 'reportTokensEstimate'>): string {
  const p = parseLocal(d.date)
  const dateFR = p ? `${String(p.d).padStart(2, '0')}/${String(p.mo).padStart(2, '0')}/${p.y}` : d.date
  const x = d.day
  const acts = d.activities.length
    ? d.activities.map((a) => `${a.typeLabel} ${fmtDuration(a.durationS)}${isNum(a.distanceM) ? ` (${km(a.distanceM)})` : ''}`).join(' ; ')
    : 'aucune'
  const lines = [
    `### BILAN RÉCUPÉRATION GARMIN - ${dateFR}`,
    '- **Mode :** Santé & Récupération (hors séance)',
    `- **Activité du jour :** ${n(x.steps)} pas (objectif ${n(x.stepGoal)}) | Étages : ${n(x.floorsUp)} ↑ / ${n(x.floorsDown)} ↓ | Intensité : ${n(x.intensityModerateMin)} min mod. / ${n(x.intensityVigorousMin)} min vig.`,
    `- **Activités enregistrées :** ${acts}`,
    `- **Stress moyen :** ${n(x.stressAvg)} | **Body Battery :** ${isNum(x.bodyBatteryCharged) ? `+${x.bodyBatteryCharged}` : ND} / ${isNum(x.bodyBatteryDrained) ? `-${x.bodyBatteryDrained}` : ND}`,
    '',
    ...recoverySection(d.recovery),
    '',
    ...trendSection(d.recovery),
    '',
    ...historySection(d.history, d.history ? 14 : 0),
    '',
    ...alertsSection(d.alerts),
  ]
  return tidy(lines)
}

/** Supprime les lignes vides en double / en fin. */
function tidy(lines: string[]): string {
  const out: string[] = []
  for (const l of lines) {
    if (l === '' && (out.length === 0 || out[out.length - 1] === '')) continue
    out.push(plain(l))
  }
  while (out.length && out[out.length - 1] === '') out.pop()
  return out.join('\n')
}
