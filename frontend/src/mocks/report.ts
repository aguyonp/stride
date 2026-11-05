// Markdown prompts following docs/SPEC.md §6 (directive first, sections gated by include chips).

import type { ActivityView, Alert, AthleteProfile, DayView, Goal, HistoryRow, IncludeKey, PlannedSession, Segment } from '../api/types'
import { addDays, diffDays, fmtDuration, fmtHM, fmtNumber, fmtPaceValue, fmtSigned, isNum, mondayOf, ND, parseLocal, todayISO } from '../lib/format'
import { tagLabel } from '../lib/labels'
import type { CatalogItem } from './catalog'
import type { UserData } from './db'
import { avgGct, garminOn, loadRatio, notesOf, plannedWithMatch, sourceMode, visibleItems } from './domain'
import { mean } from './sim'
import { buildActivityView, upcomingAfter } from './view'
import { recoveryFor } from './wellness'

export const DIRECTIVE =
  "> **DIRECTIVE SYSTÈME POUR L'IA :** Agis comme un ingénieur biomécanicien et entraîneur de course à pied strictement objectif, analytique et rigoureux. Ne cherche JAMAIS à flatter l'utilisateur, à minimiser ses erreurs ou à lui répondre pour lui faire plaisir. Analyse les données froidement : si une métrique (cadence, dérive, charge, allure, douleur) indique un risque de blessure, une compensation ou une erreur d'entraînement, dis-le-lui directement et ajuste le plan vers le bas s'il le faut. Dis uniquement ce qui est physiologiquement et mécaniquement juste pour lui."

const ALL_KEYS: IncludeKey[] = ['pain', 'biomech', 'health', 'plan', 'load', 'splits', 'history']

export function parseIncludeParam(raw: string | null): IncludeKey[] {
  if (raw === null) return ALL_KEYS
  if (raw === '' || raw === 'none') return []
  return raw.split(',').filter((k): k is IncludeKey => (ALL_KEYS as string[]).includes(k))
}

const n = (v: number | null | undefined, d = 0) => (isNum(v) ? fmtNumber(v, d) : ND)
const u = (v: number | null | undefined, unit: string, d = 0) => (isNum(v) ? `${fmtNumber(v, d)} ${unit}` : ND)
const pace = (v: number | null | undefined) => (isNum(v) && v > 0 ? `${fmtPaceValue(v)}/km` : ND)
const signed = (v: number | null | undefined, d = 0, unit = '') => (isNum(v) ? `${fmtSigned(v, d)}${unit}` : ND)
const pain = (v: number | null | undefined) => (isNum(v) ? `${fmtNumber(v, 1, { trim: true })}/10` : ND)
const km = (m: number | null | undefined, d = 2) => (isNum(m) ? `${fmtNumber(m / 1000, d)} km` : ND)
const why = (v: string, reason: string) => (v === ND ? `${ND} (${reason})` : v)

function frDate(iso: string): string {
  const p = parseLocal(iso)
  return p ? `${String(p.d).padStart(2, '0')}/${String(p.mo).padStart(2, '0')}/${p.y}` : ND
}
function frDay(iso: string): string {
  const p = parseLocal(iso)
  return p ? `${String(p.d).padStart(2, '0')}/${String(p.mo).padStart(2, '0')}` : ND
}
const WD = ['dim.', 'lun.', 'mar.', 'mer.', 'jeu.', 'ven.', 'sam.']
const weekday = (iso: string) => WD[new Date(`${iso}T12:00:00`).getDay()]

const ICON: Record<Alert['level'], string> = { danger: '🛑', warn: '⚠️', info: 'ℹ️', ok: '✅' }
const SHOE_TYPE = { soft: 'Souple', tempo: 'Tempo', carbon: 'Carbone' } as const
const MATCH = { done: 'Réalisée', partial: 'Partielle', missed: 'Manquée', upcoming: 'À venir', extra: 'Hors plan' } as const

function profileLabel(p: AthleteProfile): string {
  if (p.focusArea === 'none') return 'Performance'
  const side = p.injuredSide ? `, côté ${p.injuredSide === 'left' ? 'gauche' : p.injuredSide === 'right' ? 'droit' : 'bilatéral'}` : ''
  return `Suivi Pathologie (${p.focusLabel}${side})`
}

function goalLine(goals: Goal[], today: string): { macro: string; others: string } {
  const label = (g: Goal) => `${g.title}${g.targetDate ? ` (échéance ${frDay(g.targetDate)}, J-${Math.max(0, diffDays(today, g.targetDate))})` : ''}`
  const macro = goals.find((g) => g.kind === 'macro')
  const others = goals.filter((g) => g !== macro)
  return { macro: macro ? label(macro) : `${ND} (aucun objectif macro défini)`, others: others.length ? others.map(label).join(' ; ') : 'aucun' }
}

function plannedLabel(p: PlannedSession): string {
  if (p.sport === 'rest') return 'Repos'
  const bits: string[] = []
  if (p.targetDurationS) bits.push(`${Math.round(p.targetDurationS / 60)} min`)
  if (p.targetDistanceM) bits.push(km(p.targetDistanceM, 1))
  let s = `${p.title}${bits.length ? ` (${bits.join(', ')})` : ''}`
  if (p.targetPaceMinSecPerKm || p.targetPaceMaxSecPerKm) {
    const lo = p.targetPaceMinSecPerKm ?? p.targetPaceMaxSecPerKm
    const hi = p.targetPaceMaxSecPerKm ?? p.targetPaceMinSecPerKm
    s += ` @ ${fmtPaceValue(lo)}${hi !== lo ? `–${fmtPaceValue(hi)}` : ''}/km`
  }
  if (p.targetCadenceSpm) s += ` | Cible cadence: ${p.targetCadenceSpm} spm`
  return s
}

function contextSection(p: AthleteProfile, goals: Goal[], planned: PlannedSession[], upcoming: PlannedSession[], today: string, plannedTitle = "Séance prévue aujourd'hui"): string[] {
  const g = goalLine(goals, today)
  const pl = planned.filter((x) => x.sport !== 'rest' || planned.length === 1)
  const plannedText = pl.length
    ? pl.map((x) => `${plannedLabel(x)}${x.match && x.match.status !== 'upcoming' ? ` → ${MATCH[x.match.status]} : ${x.match.summary}` : ''}`).join(' ; ')
    : 'Aucune séance prévue'
  return [
    "### CONTEXTE & OBJECTIFS DE L'ATHLÈTE",
    `- **Profil / Mode actif :** ${profileLabel(p)}`,
    `- **Objectif Macro :** ${g.macro}${g.others !== 'aucun' ? ` | Autres : ${g.others}` : ''}`,
    `- **${plannedTitle} :** ${plannedText}`,
    `- **Prochaines séances prévues :** ${upcoming.length ? upcoming.map((x) => `${weekday(x.date)} ${frDay(x.date)} : ${plannedLabel(x)}`).join(' ; ') : 'aucune'}`,
  ]
}

function alertsSection(alerts: Alert[], title = '### ALERTES AUTOMATIQUES'): string[] {
  return [title, ...(alerts.length ? alerts.map((a) => `- ${ICON[a.level]} **${a.title} :** ${a.message}`) : ['- ✅ Aucun signal d’alerte.'])]
}

function healthSection(rec: ActivityView['recovery'], reason: string): string[] {
  const r = rec?.readiness
  const readiness = r && r.score !== null ? `${r.score}/100 (${r.status ?? 'statut N/D'}, saisie manuelle)` : `${ND} (non exposé par l'API Garmin, saisie manuelle possible)`
  if (!rec) {
    return [
      '### RÉCUPÉRATION & PRÉPARATION (NUIT PRÉCÉDENTE)',
      `- **Training Readiness :** ${readiness} | **Body Battery réveil :** ${ND} (${reason})`,
      `- **VFC nocturne :** ${ND} (${reason})`,
      `- **Sommeil :** ${ND} (${reason}) | **FC repos :** ${ND}`,
    ]
  }
  const h = rec.hrv
  const sl = rec.sleep
  return [
    `### RÉCUPÉRATION & PRÉPARATION (NUIT DU ${frDay(addDays(rec.date, -1))} AU ${frDay(rec.date)})`,
    `- **Training Readiness :** ${readiness} | **Body Battery réveil :** ${isNum(rec.bodyBattery?.atWake) ? `${rec.bodyBattery.atWake}/100` : ND}`,
    `- **VFC nocturne :** ${u(h?.lastNightAvgMs, 'ms')} (Statut: ${h?.statusLabel ?? ND} | Baseline: ${n(h?.baselineLowMs)}-${n(h?.baselineHighMs)} ms | Moy 7j: ${u(h?.weeklyAvgMs, 'ms')})`,
    `- **Sommeil :** ${sl ? `Score ${isNum(sl.score) ? `${sl.score}/100` : ND} (Durée: ${fmtHM(sl.durationS)} dont Profond: ${fmtHM(sl.deepS)})` : `${ND} (sommeil non enregistré)`} | **FC repos :** ${u(rec.restingHR?.bpm, 'bpm')}`,
  ]
}

function historySection(rows: HistoryRow[], withPain: boolean): string[] {
  const head = ['Date', 'Dist.', 'Durée', 'Allure', 'Cadence', 'GCT', 'D-', 'Load', 'Chaussures', ...(withPain ? ['Gêne Effort', 'Gêne Froid'] : [])]
  const out = ['### HISTORIQUE 14 JOURS (COURSE & RESSENTI)']
  if (!rows.length) return [...out, '- Aucune course sur les 14 derniers jours.']
  out.push(`| ${head.join(' | ')} |`, `|${head.map(() => '---').join('|')}|`)
  for (const r of rows) {
    const cells = [frDay(r.date), km(r.distanceM), fmtDuration(r.durationS), pace(r.paceSecPerKm), u(r.avgCadenceSpm, 'spm'), u(r.gctMs, 'ms'), isNum(r.elevLossM) ? `${r.elevLossM} m` : ND, n(r.trainingLoad), r.shoeName ?? ND]
    if (withPain) cells.push(pain(r.painDuring), pain(r.painCold))
    out.push(`| ${cells.join(' | ')} |`)
  }
  return out
}

function splitsSection(v: ActivityView): string[] {
  const rows: Segment[] = v.segments.primary === 'blocks' ? v.segments.blocks : v.segments.laps
  const out = ['### DÉTAIL DES INTERVALLES / SPLITS']
  if (!rows.length) return [...out, '- Aucun segment disponible pour cette activité.']
  out.push('| Bloc | Durée | Dist. | Allure | GAP | FC moy | Watts | Cadence | GCT | D+/D- |', '|---|---|---|---|---|---|---|---|---|---|')
  for (const s of rows) {
    const walk = s.kind === 'walk' || s.kind === 'stand'
    out.push(
      `| ${s.label} | ${fmtDuration(s.durationS)} | ${km(s.distanceM)} | ${walk ? 'Marche' : pace(s.paceSecPerKm)} | ${walk ? '—' : pace(s.gapSecPerKm)} | ${u(s.avgHR, 'bpm')} | ${walk ? '—' : u(s.avgPower, 'W')} | ${walk ? '—' : u(s.avgCadenceSpm, 'spm')} | ${walk ? '—' : u(s.gctMs, 'ms')} | +${n(s.elevGainM)}/-${n(s.elevLossM)} m |`,
    )
  }
  return out
}

function frDateTime(startLocal: string): string {
  return `${frDate(startLocal)} ${startLocal.slice(11, 16)}`
}

/* ---------------------------------- §6.2 single session ---------------------------------- */

export function activityReport(v: ActivityView, d: UserData, include: IncludeKey[], today = todayISO()): string {
  const on = (k: IncludeKey) => include.includes(k)
  const a = v.activity
  const notes = v.notes
  const g = garminOn(sourceMode(d))
  const L: string[] = [DIRECTIVE, '']
  if (on('plan')) {
    const planned = plannedWithMatch(d, visibleItems(d, today), today).filter((p) => p.date === a.date)
    L.push(...contextSection(d.profile, v.goals, planned, v.upcoming, today), '')
  }
  const w = v.weather
  const meteo = w ? `${n(w.temperatureC)}°C, Hum: ${n(w.humidityPct)}%, Vent: ${n(w.windKph)} km/h${w.windDirLabel ? ` ${w.windDirLabel}` : ''}` : `${ND} (${v.warnings.some((x) => x.startsWith('Séance en intérieur')) ? 'séance en intérieur' : 'météo indisponible'})`
  const rpe = notes.rpe ?? a.strava?.rpe ?? null
  const noteText = [notes.note, a.strava?.description, a.strava?.privateNote].filter((x) => x && x.trim()).join(' — ')
  const shoe = notes.shoe
  L.push(
    `### RAPPORT SÉANCE - ${frDateTime(a.startLocal)}`,
    `- **Source :** ${v.sourceLabel} | **Météo :** ${meteo}`,
    `- **Réalisé :** ${km(a.distanceM)} en ${fmtDuration(a.durationS)} | Allure: ${pace(a.avgPaceSecPerKm)} (GAP: ${pace(a.gapSecPerKm)}) | Puissance: ${why(u(a.avgPower, 'W'), g ? 'non enregistrée' : 'Garmin non connecté')}`,
    `- **Dénivelé :** +${n(a.elevGainM)} m / -${n(a.elevLossM)} m | **RPE / Notes :** ${rpe !== null ? `RPE ${rpe}/10` : `RPE ${ND}`}${noteText ? ` — ${noteText}` : ''}`,
    `- **Chaussures :** ${shoe ? `${shoe.brand} ${shoe.model} (Drop: ${u(shoe.dropMm, 'mm')}, Type: ${SHOE_TYPE[shoe.type]})` : a.strava?.gearName ? `${a.strava.gearName} (Strava)` : `${ND} (non renseignées)`}`,
  )
  if (on('pain')) {
    L.push(`- **Ressenti & Douleur :** Effort: ${pain(notes.painDuring)} | À froid: ${pain(notes.painCold)} | Lendemain: ${pain(notes.painNextMorning)} | Contexte: ${notes.tags.length ? notes.tags.map(tagLabel).join(', ') : 'aucun'}`)
  }
  L.push('')
  if (on('biomech')) {
    const b = v.biomechanics
    const t = d.profile.thresholds
    const fd = b.fatigueDrift
    const sl = b.slope
    const gb = b.gctBalance
    L.push(
      '### BIOMÉCANIQUE & DÉRIVE DE FATIGUE',
      `- **Cadence moy :** ${u(b.cadenceAvgSpm, 'spm')} (Max: ${n(b.cadenceMaxSpm)}) | % dans la cible (${t.cadenceTargetMin}-${t.cadenceTargetMax}): ${u(b.cadenceInTargetPct, '%')} | **Dérive Cadence :** ${signed(fd?.cadenceDeltaSpm, 0, ' spm')}`,
      `- **Cadence selon pente :** Plat: ${u(sl?.flat?.cadenceSpm, 'spm')} | Descente (<-${fmtNumber(sl?.thresholdPct ?? 1.5, 1)}%): ${why(u(sl?.downhill?.cadenceSpm, 'spm'), sl ? 'pas de descente significative' : 'altitude indisponible')}`,
      `- **Temps de contact sol (GCT) :** ${why(u(b.groundContactTimeMs, 'ms'), g ? 'capteur absent' : 'Garmin non connecté')} | **Dérive GCT (Début vs Fin) :** ${signed(fd?.gctDeltaMs, 0, ' ms')} (${signed(fd?.gctDeltaPct, 1, ' %')})`,
      `- **Oscillation verticale :** ${why(u(b.verticalOscillationCm, 'cm', 1), g ? 'capteur absent' : 'Garmin non connecté')} (Ratio: ${u(b.verticalRatioPct, '%', 1)}) | **Dérive OV :** ${signed(fd?.voDeltaCm, 1, ' cm')}`,
      `- **Longueur de foulée :** Moy: ${u(b.strideLengthM, 'm', 2)} (En descente: ${u(sl?.downhill?.strideM, 'm', 2)}, max ${u(sl?.downhill?.strideMaxM, 'm', 2)})`,
      `- **Équilibre G/D :** ${gb.available ? `${n(gb.leftPct, 1)} % / ${n(gb.rightPct, 1)} %` : `${ND}${gb.reason ? ` (${gb.reason})` : ''}`}`,
      '',
    )
  }
  if (on('splits')) L.push(...splitsSection(v), '')
  if (on('load')) {
    const l = v.load
    const st = l.stamina
    const cl = v.crossLoad
    const lr = l.loadRatio
    const why2 = g ? 'non disponible' : 'Garmin non connecté'
    L.push(
      '### CHARGE, STAMINA & IMPACT QUOTIDIEN (48H)',
      `- **Dérive cardiaque (${l.drift?.method === 'pace:hr' ? 'Allure:FC' : 'Pw:Hr'}) :** ${l.drift ? signed(l.drift.decouplingPct, 1, ' %') : `${ND} (séance trop courte)`} | **Stamina Fenix 8 :** ${st ? `Début ${u(st.startPct, '%')} -> Fin ${u(st.endPct, '%')} (Min: ${u(st.minPct, '%')})` : `${ND} (${why2})`}`,
      `- **Charge séance :** Garmin Load ${why(n(l.trainingLoad), why2)} | Impact Load: ${n(l.impactLoad)} | TE Aérobie: ${n(l.aerobicTE, 1)} | Strava Relative Effort: ${why(n(l.stravaRelativeEffort), 'Strava non connecté')}`,
      `- **Ratio de charge global :** ${lr ? `${n(lr.ratio, 2)} (Aiguë: ${n(lr.acute)} / Chronique: ${n(lr.chronic)})` : `${ND} (${why2})`}`,
      `- **Charge croisée (48h) :** Vélo: ${fmtNumber(cl.bike.distanceKm, 1)} km / ${Math.round(cl.bike.durationS / 60)} min (Load: ${n(cl.bike.trainingLoad)}) | Escaliers (J-1/J): ${n(cl.floors.prevDayUp)} / ${n(cl.floors.dayUp)} étages | Pas (J-1): ${n(cl.steps.prevDay)} | Stress J-1: ${isNum(cl.stressPrevDay) ? `${cl.stressPrevDay}/100` : ND}`,
      '',
    )
  }
  if (on('health')) L.push(...healthSection(v.recovery, 'Garmin non connecté'), '')
  if (on('history')) L.push(...historySection(v.history, on('pain')), '')
  L.push(...alertsSection(v.alerts))
  return L.join('\n')
}

/* ---------------------------------- §6.3 day report ---------------------------------- */

export function dayReport(v: DayView, d: UserData, include: IncludeKey[], today = todayISO()): string {
  const on = (k: IncludeKey) => include.includes(k)
  const day = v.day
  const L: string[] = [DIRECTIVE, '']
  if (on('plan')) L.push(...contextSection(d.profile, v.goals, v.planned, v.upcoming, today, `Séance prévue le ${frDay(v.date)}`), '')
  const acts = v.activities.length
    ? v.activities.map((a) => `${a.name} ${a.distanceM ? `${km(a.distanceM, 1)} / ` : ''}${fmtHM(a.durationS)}${a.avgHR ? ` (FC ${a.avgHR})` : ''}`).join(' ; ')
    : 'aucune'
  const planned = v.planned.filter((p) => p.sport !== 'rest')
  L.push(
    `### BILAN SANTÉ & CHARGE QUOTIDIENNE - ${frDate(v.date)}`,
    `- **Pas :** ${n(day.steps)} / ${n(day.stepGoal)}${isNum(day.steps) && isNum(day.stepGoal) && day.stepGoal > 0 ? ` (${Math.round((day.steps / day.stepGoal) * 100)} %)` : ''} | **Étages :** +${n(day.floorsUp)} / -${n(day.floorsDown)}`,
    `- **Minutes d'intensité :** modérées ${u(day.intensityModerateMin, 'min')} | vigoureuses ${u(day.intensityVigorousMin, 'min')} | **Stress moyen :** ${isNum(day.stressAvg) ? `${day.stressAvg}/100` : ND}`,
    `- **Body Battery :** rechargé ${signed(day.bodyBatteryCharged)} | dépensé ${isNum(day.bodyBatteryDrained) ? `-${day.bodyBatteryDrained}` : ND}`,
    `- **Activités du jour :** ${acts} | **Vélo 48 h :** ${fmtNumber(v.crossLoad.bike.distanceKm, 1)} km / ${Math.round(v.crossLoad.bike.durationS / 60)} min`,
    `- **Séance prévue :** ${planned.length ? planned.map((p) => `${plannedLabel(p)} — ${p.match ? `${MATCH[p.match.status]} (${p.match.summary})` : 'statut N/D'}`).join(' ; ') : 'aucune'}`,
    '',
  )
  if (on('health')) {
    L.push(...healthSection(v.recovery, 'Garmin non connecté'), '')
    const trend = v.recovery?.trend ?? []
    L.push('### TENDANCE 7 JOURS')
    if (!trend.length) L.push(`- ${ND} (Garmin non connecté)`)
    else {
      L.push('| Jour | VFC | Statut | Sommeil | Profond | FC repos | Pas | Étages | Load | Gêne lendemain |', '|---|---|---|---|---|---|---|---|---|---|')
      const items = visibleItems(d, today)
      for (const t of trend) {
        const run = items.find((it) => it.isRunning && it.date === t.date)
        const next = run ? notesOf(d, run.id).painNextMorning : null
        L.push(`| ${weekday(t.date)} ${frDay(t.date)} | ${u(t.hrvMs, 'ms')} | ${t.hrvStatus ?? ND} | ${isNum(t.sleepScore) ? `${t.sleepScore}/100` : ND} | ${fmtHM(t.deepS)} | ${u(t.restingHR, 'bpm')} | ${n(t.steps)} | ${n(t.floorsUp)} | ${n(t.trainingLoad)} | ${pain(next)} |`)
      }
    }
    L.push('')
  }
  if (on('history')) {
    const mode = sourceMode(d)
    const items = visibleItems(d, today)
    const rows = items
      .filter((it) => it.isRunning && it.date >= addDays(v.date, -13) && it.date <= v.date)
      .sort((a, b) => (a.startLocal < b.startLocal ? -1 : 1))
      .map((it) => {
        const nt = notesOf(d, it.id)
        return { activityId: it.id, date: it.date, type: it.type, typeLabel: it.typeLabel, distanceM: it.base.distanceM, durationS: it.base.durationS, paceSecPerKm: it.base.avgPaceSecPerKm, avgCadenceSpm: it.base.avgCadenceSpm, gctMs: garminOn(mode) ? avgGct(it) : null, elevLossM: it.base.elevLossM, trainingLoad: garminOn(mode) ? it.base.trainingLoad : null, shoeName: nt.shoe?.name ?? null, painDuring: nt.painDuring, painCold: nt.painCold, painNextMorning: nt.painNextMorning }
      })
    L.push(...historySection(rows, on('pain')), '')
  }
  L.push(...alertsSection(v.alerts))
  return L.join('\n')
}

/* ---------------------------------- §6.4 multi-session ---------------------------------- */

export function multiReport(d: UserData, opts: { activityIds?: number[]; weeks?: number; include: IncludeKey[] }, today = todayISO()): { markdown: string; sessions: number } {
  const on = (k: IncludeKey) => opts.include.includes(k)
  const items = visibleItems(d, today)
  const mode = sourceMode(d)
  const g = garminOn(mode)
  let selected: CatalogItem[]
  let from: string
  let to: string
  if (opts.activityIds?.length) {
    const ids = new Set(opts.activityIds)
    selected = items.filter((it) => ids.has(it.id))
    from = selected.reduce((m, it) => (it.date < m ? it.date : m), today)
    to = selected.reduce((m, it) => (it.date > m ? it.date : m), selected[0]?.date ?? today)
  } else {
    const weeks = Math.max(1, Math.min(52, opts.weeks ?? 4))
    from = addDays(mondayOf(today), -7 * (weeks - 1))
    to = today
    selected = items.filter((it) => it.isRunning && it.date >= from && it.date <= to)
  }
  selected = [...selected].sort((a, b) => (a.startLocal < b.startLocal ? -1 : 1))
  const weeksCount = Math.max(1, Math.ceil((diffDays(mondayOf(from), to) + 1) / 7))
  const views = selected.map((it) => buildActivityView(it, d, today))
  const planned = plannedWithMatch(d, items, today)
  const L: string[] = [DIRECTIVE, '', "### CONTEXTE & OBJECTIFS DE L'ATHLÈTE"]
  if (on('plan')) {
    const gl = goalLine(d.goals.filter((x) => x.status === 'active'), today)
    L.push(`- **Profil / Mode actif :** ${profileLabel(d.profile)} | **Objectif Macro :** ${gl.macro} | **Autres objectifs :** ${gl.others}`)
  }
  L.push(`- **Période analysée :** ${frDate(from)} → ${frDate(to)} (${weeksCount} semaine${weeksCount > 1 ? 's' : ''}, ${selected.length} séance${selected.length > 1 ? 's' : ''})`)
  if (on('plan')) {
    const up = upcomingAfter(planned, today)
    L.push(`- **Prochaines séances prévues :** ${up.length ? up.map((x) => `${weekday(x.date)} ${frDay(x.date)} : ${plannedLabel(x)}`).join(' ; ') : 'aucune'}`)
  }
  L.push('', '### RÉCAPITULATIF DES SÉANCES')
  const head = ['Date', 'Type', 'Dist.', 'Durée', 'Allure', 'GAP', 'FC moy', 'Cadence', 'GCT', 'Dérive GCT', 'D-', 'Load', 'Chaussures', ...(on('pain') ? ['Gêne E/F/L'] : [])]
  if (!views.length) L.push('- Aucune séance sur la période.')
  else {
    L.push(`| ${head.join(' | ')} |`, `|${head.map(() => '---').join('|')}|`)
    for (const v of views) {
      const a = v.activity
      const nt = v.notes
      const cells = [frDay(a.date), a.typeLabel, km(a.distanceM), fmtDuration(a.durationS), pace(a.avgPaceSecPerKm), pace(a.gapSecPerKm), u(a.avgHR, 'bpm'), u(v.biomechanics.cadenceAvgSpm, 'spm'), u(v.biomechanics.groundContactTimeMs, 'ms'), signed(v.biomechanics.fatigueDrift?.gctDeltaPct, 1, ' %'), isNum(a.elevLossM) ? `${a.elevLossM} m` : ND, n(a.trainingLoad), nt.shoe?.name ?? ND]
      if (on('pain')) cells.push(`${pain(nt.painDuring)} / ${pain(nt.painCold)} / ${pain(nt.painNextMorning)}`)
      L.push(`| ${cells.join(' | ')} |`)
    }
  }
  L.push('', '### TENDANCES HEBDOMADAIRES')
  const wHead = ['Semaine', 'Séances', 'Volume', 'Durée', 'Load', 'Ratio charge', 'Cadence moy', 'GCT moy', ...(on('health') ? ['VFC moy', 'Sommeil moy'] : []), ...(on('pain') ? ['Gêne effort', 'Gêne froid', 'Gêne lendemain'] : [])]
  L.push(`| ${wHead.join(' | ')} |`, `|${wHead.map(() => '---').join('|')}|`)
  for (let w = mondayOf(from); w <= to; w = addDays(w, 7)) {
    const end = addDays(w, 6)
    const vs = views.filter((v) => v.activity.date >= w && v.activity.date <= end)
    const lr = loadRatio(items, end < today ? end : today, mode)
    const days = Array.from({ length: 7 }, (_, i) => addDays(w, i)).filter((x) => x <= today)
    const avg = (xs: (number | null)[], dd = 0) => {
      const m = mean(xs)
      return m === null ? ND : fmtNumber(m, dd)
    }
    const cells = [
      `${frDay(w)}–${frDay(end)}`,
      String(vs.length),
      km(vs.reduce((s, v) => s + (v.activity.distanceM ?? 0), 0), 1),
      fmtHM(vs.reduce((s, v) => s + (v.activity.durationS ?? 0), 0)),
      g ? n(vs.reduce((s, v) => s + (v.activity.trainingLoad ?? 0), 0)) : ND,
      lr ? n(lr.ratio, 2) : ND,
      avg(vs.map((v) => v.biomechanics.cadenceAvgSpm)),
      avg(vs.map((v) => v.biomechanics.groundContactTimeMs)),
    ]
    if (on('health')) {
      const rec = g ? days.map((x) => recoveryFor(x)) : []
      cells.push(g ? `${avg(rec.map((r) => r.hrv?.lastNightAvgMs ?? null))} ms` : ND, g ? `${avg(rec.map((r) => r.sleep?.score ?? null))}/100` : ND)
    }
    if (on('pain')) cells.push(avg(vs.map((v) => v.notes.painDuring), 1), avg(vs.map((v) => v.notes.painCold), 1), avg(vs.map((v) => v.notes.painNextMorning), 1))
    L.push(`| ${cells.join(' | ')} |`)
  }
  if (on('plan')) {
    const inRange = planned.filter((p) => p.date >= from && p.date <= to && p.sport !== 'rest')
    const count = (s: string) => inRange.filter((p) => p.match?.status === s).length
    const plannedKm = inRange.reduce((s, p) => s + (p.targetDistanceM ?? 0), 0)
    const doneKm = inRange.reduce((s, p) => {
      const it = items.find((x) => x.id === p.match?.activityId)
      return s + (it?.base.distanceM ?? 0)
    }, 0)
    L.push('', '### PRÉVU vs RÉALISÉ', `- Séances prévues : ${inRange.length} | réalisées : ${count('done')} | partielles : ${count('partial')} | manquées : ${count('missed')} | écart volume : ${plannedKm > 0 ? signed(((doneKm - plannedKm) / plannedKm) * 100, 0, ' %') : ND}`)
  }
  const agg = new Map<string, { alert: Alert; count: number }>()
  for (const v of views) for (const a of v.alerts) if (a.level === 'warn' || a.level === 'danger') {
    const cur = agg.get(a.code)
    if (cur) {
      cur.count++
      if (a.level === 'danger') cur.alert = a
    } else agg.set(a.code, { alert: a, count: 1 })
  }
  L.push('', '### ALERTES & SIGNAUX RÉCURRENTS')
  if (!agg.size) L.push('- ✅ Aucun signal récurrent sur la période.')
  else for (const { alert, count } of [...agg.values()].sort((a, b) => b.count - a.count)) L.push(`- ${ICON[alert.level]} **${alert.title}** ×${count} (dernière : ${alert.message})`)
  const next = Math.max(2, Math.min(4, weeksCount))
  L.push('', '### DEMANDE', `- À partir de ces données, construis ou réadapte un plan d'entraînement pour les ${next} prochaines semaines, cohérent avec les objectifs et la zone surveillée. Justifie chaque ajustement par les données ci-dessus.`)
  return { markdown: L.join('\n'), sessions: selected.length }
}
