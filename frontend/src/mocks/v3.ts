// V3 mock endpoints (§11): LLM connector, plan generation/import/push, calendar, efficiency, impact, readiness, GPS track,
// notifications, rescue import, shoe merge. Synthetic data only (the GPS track is a generated loop, not a real place).
//
// Extra scenario flags: nollm (no LLM configured) · llmerr (provider call fails) · pushok (Garmin workout push accepted;
// default: unsupported) · importerr (import/parse errors) · notiferr (notification test fails) · ready-green /
// ready-orange / ready-red (force the readiness banner) · noeff (efficiency not eligible)

import type {
  ActivityEfficiency,
  ActivityImpact,
  ActivityTrack,
  ActivityView,
  ApiErrorBody,
  CalendarDay,
  DayView,
  EfficiencyTrend,
  ErrorCode,
  LLMAnalysis,
  LLMConfig,
  LLMProvider,
  NotificationInput,
  NotificationTarget,
  PlannedSessionInput,
  PlanPhase,
  PlanSport,
  ReadinessBanner,
  TrackPoint,
  WorkoutStep,
} from '../api/types'
import { addDays, diffDays, fmtNumber, fmtPace, todayISO } from '../lib/format'
import type { CatalogItem } from './catalog'
import { nextId, type MockUser, type UserData, type V3Data } from './db'
import { loadRatio, plannedWithMatch, sourceMode, summarize, visibleItems } from './domain'
import { has } from './flags'
import { v31Of } from './manual'
import { hashStr, rng } from './sim'
import { recoveryFor } from './wellness'

interface Ctx {
  url: URL
  params: string[]
  init?: RequestInit
  me: MockUser
  data: UserData
}
type RouteFn = (method: string, re: RegExp, handler: (c: Ctx) => Response | Promise<Response>) => void

const json = (body: unknown, status = 200, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json; charset=utf-8', ...headers } })
const err = (status: number, code: ErrorCode, error: string) => json({ error, code } satisfies ApiErrorBody, status)
const noContent = () => new Response(null, { status: 204 })
const isoNow = () => new Date().toISOString()
const ago = (ms: number) => new Date(Date.now() - ms).toISOString()
const H = 3_600_000
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/

function body<T>(init?: RequestInit): T | null {
  if (!init?.body || typeof init.body !== 'string') return null
  try {
    return JSON.parse(init.body) as T
  } catch {
    return null
  }
}

/* ---------------------------------- State ---------------------------------- */

const NO_LLM: LLMConfig = { configured: false, provider: null, model: null, baseUrl: null, keyHint: null, updatedAt: null }

function v3(d: UserData): V3Data {
  if (!d.v3) {
    const seeded = d.shoes.length > 0
    d.v3 = {
      llm: has('nollm') || !seeded ? NO_LLM : { configured: true, provider: 'gemini', model: 'gemini-2.5-flash', baseUrl: null, keyHint: '…7Qx2', updatedAt: ago(5 * 24 * H) },
      analyses: [],
      notifications: seeded
        ? [{ id: nextId(), kind: 'discord', label: 'Discord perso', enabled: true, hint: 'discord.com/api/webhooks/…a1b2', lastStatus: '204', lastSentAt: ago(20 * H), lastError: null }]
        : [],
      pushSupported: has('pushok') ? true : null,
    }
    const dup = d.shoes.find((s) => s.stravaGearId)
    if (dup) d.shoes.push({ ...dup, id: nextId(), name: `${dup.brand} ${dup.model} (Strava)`.trim(), stravaGearId: null, distanceKm: 12.4, note: 'Doublon importé' })
  }
  return d.v3
}

export const llmConfigured = (d: UserData) => v3(d).llm.configured

/* ---------------------------------- Readiness, efficiency, impact ---------------------------------- */

export function readinessFor(date: string, d: UserData, items: CatalogItem[]): ReadinessBanner {
  const mode = sourceMode(d)
  const rec = recoveryFor(date)
  const low = rec.hrv?.baselineLowMs ?? null
  let below = 0
  const sleeps: number[] = []
  for (let i = 0; i < 7; i++) {
    const r = recoveryFor(addDays(date, -i))
    if (low !== null && r.hrv?.lastNightAvgMs != null && r.hrv.lastNightAvgMs < low) below++
    if (r.sleep?.score != null) sleeps.push(r.sleep.score)
  }
  const sleep7dAvg = sleeps.length ? Math.round(sleeps.reduce((a, b) => a + b, 0) / sleeps.length) : null
  const ratio = loadRatio(items, date, mode)?.ratio ?? null
  const hrv7dAvg = rec.hrv?.weeklyAvgMs ?? null
  const rhrDelta = rec.restingHR?.delta ?? null
  const base = { hrv7dAvg, hrvBaselineLow: low, nightsBelowBaseline: low === null ? null : below, sleep7dAvg, loadRatio: ratio }
  if (mode === 'strava' || mode === 'none') return { level: 'unknown', title: 'Préparation non évaluée', reasons: ['Données Garmin (VFC, sommeil) indisponibles.'], ...base }

  const forced = has('ready-red') ? 'red' : has('ready-orange') ? 'orange' : has('ready-green') ? 'green' : null
  const hrvLow = hrv7dAvg !== null && low !== null && hrv7dAvg < low
  const reasons: string[] = []
  if (hrvLow || forced === 'red') reasons.push(`VFC moyenne 7 j ${hrv7dAvg ?? 38} ms sous la baseline (${low ?? 42} ms).`)
  if (below >= 3 || forced === 'red') reasons.push(`${Math.max(below, 3)} nuits sur 7 sous la baseline VFC.`)
  if ((ratio !== null && ratio > 1.3) || forced === 'red') reasons.push(`Ratio de charge ${fmtNumber(Math.max(ratio ?? 1.42, 1.31), 2)} > 1,3 : montée rapide.`)
  if (sleep7dAvg !== null && sleep7dAvg < 60) reasons.push(`Sommeil moyen 7 j ${sleep7dAvg}/100.`)
  if (rhrDelta !== null && rhrDelta >= 5) reasons.push(`FC repos +${rhrDelta} bpm vs moyenne 7 j.`)
  if (forced === 'orange' && reasons.length === 0) reasons.push('FC repos +5 bpm vs moyenne 7 j.')

  const level: ReadinessBanner['level'] = forced ?? ((hrvLow || below >= 3) && ratio !== null && ratio > 1.3 ? 'red' : reasons.length ? 'orange' : 'green')
  if (level === 'green') reasons.splice(0, reasons.length, 'VFC, sommeil, FC repos et charge dans tes repères.')
  const title = level === 'red' ? 'Récupération insuffisante : repos ou très léger' : level === 'orange' ? 'Vigilance : garde l’intensité basse' : 'Feu vert pour la séance prévue'
  return { level, title, reasons, ...base }
}

const INTERVAL_RE = /fractionn|intervall|vma|\d+\s*×\s*\d+\s*(″|s\b)|×\s*1′/i

function efOf(it: CatalogItem): { efSpeed: number | null; efPower: number | null } {
  const b = it.base
  if (!b.distanceM || !b.durationS || !b.avgHR) return { efSpeed: null, efPower: null }
  const speed = b.distanceM / (b.durationS / 60)
  return { efSpeed: Math.round((speed / b.avgHR) * 1000) / 1000, efPower: b.avgPower ? Math.round((b.avgPower / b.avgHR) * 100) / 100 : null }
}
const eligible = (it: CatalogItem) => (it.isRunning || it.sport === 'bike') && (it.base.durationS ?? 0) >= 1200 && !INTERVAL_RE.test(it.name) && !has('noeff')

export function efficiencyTrend(d: UserData, weeks: number, today = todayISO()): EfficiencyTrend {
  const from = addDays(today, -7 * weeks)
  const pts = visibleItems(d, today)
    .filter((it) => it.date > from && it.isRunning && eligible(it))
    .sort((a, b) => (a.startLocal < b.startLocal ? -1 : 1))
    .map((it) => ({ date: it.date, activityId: it.id, ...efOf(it), avgHR: it.base.avgHR, paceSecPerKm: it.base.avgPaceSecPerKm }))
  const rolling = pts.map((_, i) => {
    const w = pts.slice(Math.max(0, i - 3), i + 1).map((p) => p.efSpeed).filter((x): x is number => x !== null)
    return w.length ? Math.round((w.reduce((a, b) => a + b, 0) / w.length) * 1000) / 1000 : null
  })
  const first = rolling.find((x) => x !== null) ?? null
  const last = [...rolling].reverse().find((x) => x !== null) ?? null
  return { points: pts, rollingEfSpeed: rolling, changePct: first && last && pts.length >= 4 ? Math.round((last / first - 1) * 1000) / 10 : null }
}

function stepsOf(it: CatalogItem): number | null {
  return it.isRunning && it.base.avgCadenceSpm && it.base.durationS ? Math.round((it.base.avgCadenceSpm * it.base.durationS) / 60) : null
}

function impactOf(v: ActivityView, it: CatalogItem, d: UserData): ActivityImpact | null {
  if (!it.isRunning) return null
  const steps = stepsOf(it)
  const km = (it.base.distanceM ?? 0) / 1000
  const vo = v.biomechanics.verticalOscillationCm
  const voFactor = vo === null ? null : Math.round(Math.min(1.5, Math.max(0.75, vo / 8)) * 100) / 100
  const descentFactor = km > 0 ? Math.round((1 + 0.01 * ((it.base.elevLossM ?? 0) / km)) * 100) / 100 : null
  const ji = (s: number | null, vf: number | null, df: number | null) => (s === null ? null : Math.round(((s * (vf ?? 1) * (df ?? 1)) / 1000) * 10) / 10)
  const jointIndex = ji(steps, voFactor, descentFactor)
  const week = visibleItems(d).filter((x) => x.isRunning && x.date > addDays(it.date, -7) && x.date <= it.date)
  const j7 = week.reduce((s, x) => {
    const xkm = (x.base.distanceM ?? 0) / 1000
    return s + (ji(stepsOf(x), 1.04, xkm > 0 ? 1 + 0.01 * ((x.base.elevLossM ?? 0) / xkm) : 1) ?? 0)
  }, 0)
  return { steps, stepsPerKm: steps !== null && km > 0 ? Math.round(steps / km) : null, voFactor, descentFactor, jointIndex, jointIndex7d: Math.round(j7 * 10) / 10 }
}

export function enrichActivityView(v: ActivityView, it: CatalogItem, d: UserData): ActivityView {
  v3(d)
  const items = visibleItems(d)
  const ok = eligible(it)
  const ef = efOf(it)
  const trend = ok ? efficiencyTrend(d, 8) : null
  const efficiency: ActivityEfficiency | null =
    it.isRunning || it.sport === 'bike'
      ? {
          eligible: ok,
          reason: ok ? null : (it.base.durationS ?? 0) < 1200 ? 'Séance < 20 min : EF non calculée.' : 'Séance fractionnée : EF calculée seulement sur effort stable.',
          efSpeed: ok ? ef.efSpeed : null,
          efPower: ok ? ef.efPower : null,
          trendChangePct: trend?.changePct ?? null,
        }
      : null
  const notes = { ...v.notes }
  if (!notes.shoe && notes.shoeId === null && v.activity.sources.length === 2) {
    const auto = d.shoes.find((s) => s.stravaGearId && !s.retired)
    if (auto) Object.assign(notes, { shoe: auto, shoeId: auto.id, shoeAuto: true })
  }
  const out: ActivityView = { ...v, notes, efficiency, impact: impactOf(v, it, d), readiness: readinessFor(it.date, d, items) }
  return it.manual ? manualView(out, it) : out
}

const SERIES_ALERT = /GCT|VO_|DOWNHILL|FADE|DRIFT|SLOPE|STRIDE/

/** A manual activity has no series, splits, GPS or weather: only what the athlete typed. */
function manualView(v: ActivityView, it: CatalogItem): ActivityView {
  const empty = { tS: [], hr: [], cadenceSpm: [], powerW: [], paceSecPerKm: [], elevationM: [], gctMs: [], voCm: [], gctBalanceLeftPct: [], gapSecPerKm: [], staminaPct: [] }
  const b = v.biomechanics
  return {
    ...v,
    sourceLabel: 'Manuel',
    activity: { ...v.activity, gapSecPerKm: null, maxCadenceSpm: null, stamina: null, strava: null, steps: null, calories: null },
    weather: null,
    series: empty,
    segments: { primary: 'laps', blocks: [], laps: [] },
    biomechanics: { ...b, groundContactTimeMs: null, verticalOscillationCm: null, verticalRatioPct: null, strideLengthM: null, cadenceMaxSpm: null, cadenceInTargetPct: null, cadenceBelow170Pct: null, cadenceAvgSpm: it.base.avgCadenceSpm, gctBalance: { ...b.gctBalance, available: false, leftPct: null, rightPct: null, gapPct: null, weakerSide: null, alert: null, reason: 'Séance saisie manuellement' }, fatigueDrift: null, slope: null },
    load: { ...v.load, drift: null, hrZonesS: null, powerZonesS: null, stamina: null },
    performance: v.performance ? { ...v.performance, timeInZonesS: null, z4z5S: null, bestEfforts: [] } : null,
    alerts: v.alerts.filter((a) => !SERIES_ALERT.test(a.code)),
    warnings: ['Séance saisie manuellement : pas de courbes, de splits, de tracé GPS ni de météo.'],
    dataSources: ['manual'],
  }
}

export function enrichDayView(v: DayView, d: UserData): DayView {
  return { ...v, readiness: readinessFor(v.date, d, visibleItems(d)) }
}

/* ---------------------------------- GPS track (synthetic loop) ---------------------------------- */

export function trackFor(v: ActivityView, it: CatalogItem): ActivityTrack | null {
  const s = v.series
  const n = s.tS.length
  if (!it.outdoor || n < 2) return null
  const r = rng(hashStr(`track-${it.id}`))
  const cLat = 45.5 + r() * 0.2
  const cLon = 3.5 + r() * 0.2
  const dist = it.base.distanceM ?? 5000
  const radiusM = dist / (2 * Math.PI * 1.15)
  const lobes = 2 + Math.floor(r() * 3)
  const phase = r() * Math.PI
  const points: TrackPoint[] = []
  let prevEl: number | null = null
  for (let i = 0; i < n; i++) {
    const t = i / (n - 1)
    const th = 2 * Math.PI * t
    const rad = radiusM * (1 + 0.28 * Math.sin(lobes * th + phase) + 0.05 * Math.sin(11 * th))
    const lat = cLat + (rad * Math.cos(th)) / 111_320
    const lon = cLon + (rad * Math.sin(th)) / (111_320 * Math.cos((cLat * Math.PI) / 180))
    const el = s.elevationM[i]
    const stepM = dist / (n - 1)
    const grade = el !== null && prevEl !== null && stepM > 0 ? Math.round(((el - prevEl) / stepM) * 1000) / 10 : null
    prevEl = el
    points.push([Math.round(lat * 1e6) / 1e6, Math.round(lon * 1e6) / 1e6, s.tS[i], s.cadenceSpm[i], grade])
  }
  const lats = points.map((p) => p[0])
  const lons = points.map((p) => p[1])
  return { points, bounds: { minLat: Math.min(...lats), maxLat: Math.max(...lats), minLon: Math.min(...lons), maxLon: Math.max(...lons) } }
}

/* ---------------------------------- LLM ---------------------------------- */

function analysisMarkdown(kind: string, v: ActivityView | null, question?: string): string {
  if (!v) {
    return [
      '## Synthèse',
      '',
      'Bilan du jour **modéré** : la récupération reste correcte mais la charge des 7 derniers jours monte.',
      '',
      '## Recommandations',
      '',
      '1. Séance facile ≤ 40 min, cadence 172–178 spm.',
      '2. Coucher avant 23 h, viser > 7 h de sommeil.',
      '3. Pas d’intensité avant que la VFC revienne dans la baseline.',
      ...(question ? ['', `> Question : ${question}`] : []),
    ].join('\n')
  }
  const a = v.activity
  const cad = v.biomechanics.cadenceAvgSpm
  const alerts = v.alerts.filter((x) => x.level === 'warn' || x.level === 'danger')
  return [
    `## Synthèse — ${a.name}`,
    '',
    `Séance de **${fmtNumber((a.distanceM ?? 0) / 1000, 2)} km** à **${fmtPace(a.avgPaceSecPerKm)}**, exécution globalement ${alerts.length > 2 ? '**à surveiller**' : '**conforme**'}.`,
    '',
    '### Points forts',
    '',
    `- Cadence moyenne ${cad ?? 'N/D'} spm, stable sur les blocs.`,
    '- FC maîtrisée sur la première moitié, pas de dérive cardiaque marquée.',
    '',
    '### Points de vigilance',
    '',
    ...(alerts.length ? alerts.slice(0, 4).map((x) => `- **${x.title}** : ${x.message}`) : ['- Aucun signal majeur.']),
    '',
    '### Recommandations pour la prochaine séance',
    '',
    '1. Garder le métronome à 176 spm, surtout en descente.',
    '2. Raccourcir la foulée sur les portions < −2 %.',
    '3. Renforcement excentrique quadriceps 2×/semaine.',
    '',
    '| Indicateur | Valeur | Repère |',
    '|---|---|---|',
    `| Cadence | ${cad ?? 'N/D'} spm | 172–178 |`,
    `| D− | ${a.elevLossM ?? 'N/D'} m | ≤ 30 m |`,
    `| EF | ${v.efficiency?.efSpeed ?? 'N/D'} | tendance ${v.efficiency?.trendChangePct ?? 'N/D'} % |`,
    ...(question ? ['', `> Question posée : *${question}*`, '', 'Réponse : oui, mais en restant sous 75 % de FCmax et sur terrain plat.'] : []),
    '',
    `_Analyse ${kind} générée par le connecteur IA (données synthétiques de démonstration)._`,
  ].join('\n')
}

const PROVIDERS: LLMProvider[] = ['gemini', 'anthropic', 'openai', 'openai_compat']

/* ---------------------------------- Plan ---------------------------------- */

const PLAN_SPORTS: PlanSport[] = ['run', 'bike', 'strength', 'mobility', 'rest', 'other']
const STEP_TYPES = ['warmup', 'run', 'interval', 'recovery', 'rest', 'cooldown', 'repeat']

function validSteps(steps: unknown, path: string, errors: string[]): WorkoutStep[] {
  if (!Array.isArray(steps)) return []
  const out: WorkoutStep[] = []
  steps.forEach((raw, i) => {
    const s = raw as WorkoutStep
    if (!s || !STEP_TYPES.includes(s.type)) {
      errors.push(`${path} étape ${i + 1} : type « ${String((raw as { type?: unknown })?.type)} » inconnu.`)
      return
    }
    out.push(s.type === 'repeat' ? { ...s, steps: validSteps(s.steps, `${path} étape ${i + 1}`, errors) } : s)
  })
  return out
}

export function parsePlanText(text: string): { drafts: PlannedSessionInput[]; errors: string[]; phases: PlanPhase[] } {
  const errors: string[] = []
  const fence = /```(?:json)?\s*([\s\S]*?)```/i.exec(text)
  let raw = fence ? fence[1] : text
  if (!fence) {
    const i = text.indexOf('{')
    const j = text.lastIndexOf('}')
    if (i >= 0 && j > i) raw = text.slice(i, j + 1)
  }
  let doc: { format?: string; sessions?: unknown[]; phases?: unknown[] }
  try {
    doc = JSON.parse(raw)
  } catch {
    return { drafts: [], errors: ['Aucun JSON valide trouvé : colle la réponse complète de l’IA (bloc ```json).'], phases: [] }
  }
  if (doc.format !== 'stride-plan') errors.push('Champ « format » absent ou différent de « stride-plan » : lecture tolérante.')
  const drafts: PlannedSessionInput[] = []
  ;(Array.isArray(doc.sessions) ? doc.sessions : []).forEach((raw, i) => {
    const s = raw as Partial<PlannedSessionInput> & { steps?: unknown }
    const label = `Séance ${i + 1}`
    if (!s.date || !DATE_RE.test(s.date)) return void errors.push(`${label} : date manquante ou invalide.`)
    if (!s.title?.trim()) return void errors.push(`${label} : titre manquant.`)
    const sport = PLAN_SPORTS.includes(s.sport as PlanSport) ? (s.sport as PlanSport) : 'run'
    if (s.sport !== sport) errors.push(`${label} : sport « ${String(s.sport)} » inconnu, remplacé par course.`)
    drafts.push({
      date: s.date,
      sport,
      title: s.title.trim(),
      instructions: s.instructions ?? '',
      targetDurationS: s.targetDurationS ?? null,
      targetDistanceM: s.targetDistanceM ?? null,
      targetPaceMinSecPerKm: s.targetPaceMinSecPerKm ?? null,
      targetPaceMaxSecPerKm: s.targetPaceMaxSecPerKm ?? null,
      targetCadenceSpm: s.targetCadenceSpm ?? null,
      steps: validSteps(s.steps, label, errors),
      source: 'import',
    })
  })
  const phases: PlanPhase[] = []
  ;(Array.isArray(doc.phases) ? doc.phases : []).forEach((raw, i) => {
    const p = raw as Partial<PlanPhase>
    if (!p.name?.trim() || !DATE_RE.test(p.startDate ?? '') || !DATE_RE.test(p.endDate ?? '') || (p.endDate as string) < (p.startDate as string)) return void errors.push(`Phase ${i + 1} : nom ou dates invalides.`)
    phases.push({ id: 0, name: p.name.trim(), startDate: p.startDate as string, endDate: p.endDate as string, focus: p.focus ?? '', weeklyVolumeKm: typeof p.weeklyVolumeKm === 'number' ? p.weeklyVolumeKm : null, note: p.note ?? '' })
  })
  if (!drafts.length && !errors.length) errors.push('Aucune séance dans la réponse.')
  return { drafts, errors, phases }
}

function sampleDrafts(start: string, horizon: string): PlannedSessionInput[] {
  const pace = (min: number, max: number) => ({ kind: 'pace' as const, min, max })
  const base = { targetDistanceM: null, targetPaceMinSecPerKm: null, targetPaceMaxSecPerKm: null, source: 'llm' as const }
  const all: PlannedSessionInput[] = [
    {
      ...base,
      date: start,
      sport: 'run',
      title: 'Footing + 6×1′',
      instructions: 'Cadence 172 spm, relâché.',
      targetDurationS: 2700,
      targetCadenceSpm: 172,
      steps: [
        { type: 'warmup', durationS: 600, target: pace(330, 360) },
        { type: 'repeat', repeat: 6, steps: [{ type: 'interval', durationS: 60, target: pace(250, 260) }, { type: 'recovery', durationS: 90 }] },
        { type: 'cooldown', durationS: 600 },
      ],
    },
    { ...base, date: addDays(start, 2), sport: 'strength', title: 'Renfo excentrique 25′', instructions: 'Step-down 3×10, squats tempo.', targetDurationS: 1500, targetCadenceSpm: null, steps: [] },
    {
      ...base,
      date: addDays(start, 4),
      sport: 'run',
      title: 'Continu 40′ allure facile',
      instructions: 'Terrain plat, arrêt si gêne > 3/10.',
      targetDurationS: 2400,
      targetCadenceSpm: 175,
      steps: [{ type: 'run', durationS: 2400, target: { kind: 'hr', min: 130, max: 145 } }],
    },
    { ...base, date: addDays(start, 6), sport: 'run', title: 'Sortie longue 55′', instructions: 'Ravitaillement eau.', targetDurationS: 3300, targetCadenceSpm: 174, steps: [{ type: 'run', durationS: 3300, target: pace(335, 350) }] },
  ]
  if (horizon === 'session') return all.slice(0, 1)
  if (horizon === 'week') return all
  const weeks = horizon === '2weeks' ? 2 : 3
  const out: PlannedSessionInput[] = []
  for (let w = 0; w < weeks; w++) {
    const k = 1 + w * 0.08
    for (const s of all) out.push({ ...s, date: addDays(s.date, 7 * w), targetDurationS: s.targetDurationS ? Math.round((s.targetDurationS * k) / 60) * 60 : null, title: w > 0 && s.sport === 'run' && /longue/i.test(s.title) ? `Sortie longue ${Math.round(55 * k)}′` : s.title })
  }
  return out
}

function goalPhases(start: string, goalDate: string): PlanPhase[] {
  const days = diffDays(start, goalDate)
  if (days <= 21) return []
  const cut = (f: number) => addDays(start, Math.round(days * f))
  const mk = (name: string, a: string, b: string, focus: string, km: number): PlanPhase => ({ id: 0, name, startDate: a, endDate: b, focus, weeklyVolumeKm: km, note: '' })
  return [
    mk('Développement', start, addDays(cut(0.4), -1), 'volume aérobie, cadence', 32),
    mk('Assimilation', cut(0.4), addDays(cut(0.48), -1), 'semaine allégée', 22),
    mk('Spécifique', cut(0.48), addDays(cut(0.88), -1), 'seuil, allure course', 38),
    mk('Affûtage', cut(0.88), goalDate, 'fraîcheur, rappels d’allure', 20),
  ]
}

const PLAN_CONTRACT_V2 = '{"format":"stride-plan","version":2,"goal":{"title":"…","date":"AAAA-MM-JJ"},"phases":[{"name":"Développement","startDate":"AAAA-MM-JJ","endDate":"AAAA-MM-JJ","focus":"…","weeklyVolumeKm":35,"note":""}],"sessions":[…]}'
const PLAN_CONTRACT = '{"format":"stride-plan","version":1,"sessions":[{"date":"AAAA-MM-JJ","sport":"run","title":"…","instructions":"…","targetDurationS":2700,"targetCadenceSpm":172,"steps":[{"type":"warmup","durationS":600,"target":{"kind":"pace","min":330,"max":360}}]}]}'

/* ---------------------------------- Calendar ---------------------------------- */

export function calendar(d: UserData, from: string, to: string, today = todayISO()): CalendarDay[] {
  const items = visibleItems(d, today)
  const planned = plannedWithMatch(d, items, today)
  const days: CalendarDay[] = []
  for (let day = from; day <= to; day = addDays(day, 1)) {
    days.push({
      date: day,
      planned: planned.filter((p) => p.date === day),
      done: items.filter((it) => it.date === day).map((it) => summarize(it, d, planned)),
      events: v31Of(d).events.filter((e) => e.startDate <= day && e.endDate >= day),
    })
  }
  return days
}

/* ---------------------------------- Routes ---------------------------------- */

export function registerV3(route: RouteFn, view: (d: UserData, id: number) => { v: ActivityView; it: CatalogItem } | null) {
  route('GET', /^\/api\/llm$/, ({ data }) => json(v3(data).llm))
  route('PUT', /^\/api\/llm$/, ({ data, init }) => {
    const b = body<{ provider: LLMProvider; model: string; baseUrl?: string; apiKey?: string }>(init)
    if (!b || !PROVIDERS.includes(b.provider)) return err(400, 'BAD_REQUEST', 'Fournisseur invalide.')
    if (!b.model?.trim()) return err(400, 'BAD_REQUEST', 'Le modèle est requis.')
    if (b.provider === 'openai_compat' && !/^https?:\/\//.test(b.baseUrl ?? '')) return err(400, 'BAD_REQUEST', 'URL de base invalide (http(s)://…).')
    const s = v3(data)
    const keyHint = b.apiKey === undefined ? (s.llm.provider === b.provider ? s.llm.keyHint : null) : b.apiKey === '' ? null : `…${b.apiKey.slice(-4)}`
    if (b.provider !== 'openai_compat' && !keyHint) return err(400, 'BAD_REQUEST', 'Clé API requise pour ce fournisseur.')
    s.llm = { configured: true, provider: b.provider, model: b.model.trim(), baseUrl: b.provider === 'openai_compat' ? (b.baseUrl ?? null) : null, keyHint, updatedAt: isoNow() }
    return json(s.llm)
  })
  route('DELETE', /^\/api\/llm$/, ({ data }) => {
    v3(data).llm = NO_LLM
    return noContent()
  })
  route('POST', /^\/api\/llm\/test$/, ({ data }) => {
    const c = v3(data).llm
    if (!c.configured) return err(412, 'PROVIDER_NOT_CONNECTED', 'Aucun fournisseur IA configuré.')
    if (has('llmerr')) return json({ ok: false, latencyMs: 1204, message: 'Le fournisseur a répondu 401 : clé API refusée.' })
    return json({ ok: true, latencyMs: 640 + Math.round(Math.random() * 300), message: `${c.model} répond correctement.` })
  })
  route('POST', /^\/api\/llm\/analyze$/, async ({ data, init }) => {
    const c = v3(data).llm
    if (!c.configured) return err(412, 'PROVIDER_NOT_CONNECTED', 'Configure ton IA dans Réglages › IA pour lancer une analyse.')
    const b = body<{ kind: string; activityId?: number; date?: string; activityIds?: number[]; weeks?: number; question?: string }>(init)
    if (!b || !['activity', 'day', 'multi'].includes(b.kind)) return err(400, 'BAD_REQUEST', 'Type d’analyse invalide.')
    await new Promise((r) => setTimeout(r, 1400))
    if (has('llmerr')) return err(502, 'PROVIDER_UNAVAILABLE', 'Le fournisseur IA n’a pas répondu (délai de 120 s dépassé).')
    const ref = b.kind === 'activity' ? String(b.activityId) : b.kind === 'day' ? (b.date ?? todayISO()) : (b.activityIds ?? []).join(',') || `${b.weeks ?? 4}w`
    const target = b.kind === 'activity' && b.activityId ? view(data, b.activityId) : null
    const a: LLMAnalysis = {
      id: nextId(),
      kind: b.kind,
      ref,
      provider: c.provider ?? 'gemini',
      model: c.model ?? '',
      markdown: analysisMarkdown(b.kind, target?.v ?? null, b.question),
      promptTokensEstimate: 1300 + Math.round(Math.random() * 400),
      durationMs: 1400 + Math.round(Math.random() * 3000),
      createdAt: isoNow(),
    }
    const s = v3(data)
    s.analyses = [a, ...s.analyses].slice(0, 60)
    return json(a)
  })
  route('GET', /^\/api\/llm\/analyses$/, ({ data, url }) => {
    const kind = url.searchParams.get('kind') ?? ''
    const ref = url.searchParams.get('ref') ?? ''
    const s = v3(data)
    if (s.llm.configured && kind === 'activity' && !s.analyses.some((a) => a.kind === kind && a.ref === ref)) {
      const target = view(data, Number(ref))
      if (target && target.v.activity.isRunning) {
        s.analyses.push({ id: nextId(), kind, ref, provider: s.llm.provider ?? 'gemini', model: s.llm.model ?? '', markdown: analysisMarkdown(kind, target.v), promptTokensEstimate: 1380, durationMs: 3900, createdAt: ago(26 * H) })
      }
    }
    return json(s.analyses.filter((a) => (!kind || a.kind === kind) && (!ref || a.ref === ref)).slice(0, 20))
  })

  route('POST', /^\/api\/plan\/generate$/, async ({ data, init }) => {
    const b = body<{ mode: string; horizon: string; startDate: string; goalId?: number; note?: string }>(init)
    if (!b || !['prompt', 'llm'].includes(b.mode) || !DATE_RE.test(b.startDate ?? '') || !['session', 'week', '2weeks', 'goal'].includes(b.horizon)) return err(400, 'BAD_REQUEST', 'Paramètres de génération invalides.')
    const dated = data.goals.filter((g) => g.status === 'active' && g.targetDate && g.targetDate >= b.startDate).sort((x, y) => (x.targetDate! < y.targetDate! ? -1 : 1))
    const goal = b.horizon === 'goal' ? (b.goalId ? dated.find((g) => g.id === b.goalId) : dated[0]) : undefined
    if (b.horizon === 'goal' && !goal) return err(400, 'BAD_REQUEST', 'Aucun objectif actif avec une date : ajoute une échéance à ton objectif.')
    const period = { session: 'prochaine séance', week: '7 jours', '2weeks': '14 jours', goal: `jusqu’à l’objectif « ${goal?.title} » (${goal?.targetDate})` }[b.horizon as 'session']
    if (b.mode === 'prompt') {
      const long = b.horizon === 'goal' && diffDays(b.startDate, goal!.targetDate!) > 21
      const prompt = [
        '### DIRECTIVE',
        'Tu es un coach de course à pied objectif et sans complaisance. Base-toi uniquement sur les données ci-dessous.',
        '',
        '### CONTEXTE',
        `- Période : ${period} à partir du ${b.startDate}`,
        '- Focus : genou (syndrome fémoro-patellaire), côté gauche',
        '- Ratio de charge aigu/chronique : 1,12 · Préparation : vigilance',
        '- 14 derniers jours : 6 sorties, 41 km, cadence moyenne 174 spm',
        '- **Records personnels :** 5 km 23:50 · 10 km 49:12 · Plus longue sortie 12,4 km',
        ...(b.note ? [`- Note de l’athlète : ${b.note}`] : []),
        ...(long ? ['', '### PÉRIODISATION', 'Découpe la période en phases (développement, assimilation, spécifique, affûtage) jusqu’à la date de l’objectif, puis détaille uniquement les séances des 3 prochaines semaines.'] : []),
        '',
        '### FORMAT DE RÉPONSE OBLIGATOIRE',
        'Réponds UNIQUEMENT avec un bloc ```json respectant exactement ce schéma (allure en s/km, FC en bpm, cadence en spm) :',
        '```json',
        b.horizon === 'goal' ? PLAN_CONTRACT_V2 : PLAN_CONTRACT,
        '```',
      ].join('\n')
      return json({ prompt, tokensEstimate: Math.ceil(prompt.length / 4) })
    }
    if (!v3(data).llm.configured) return err(412, 'PROVIDER_NOT_CONNECTED', 'Aucun fournisseur IA configuré.')
    await new Promise((r) => setTimeout(r, 1200))
    if (has('llmerr')) return err(502, 'PROVIDER_UNAVAILABLE', 'Le fournisseur IA n’a pas répondu.')
    let drafts = sampleDrafts(b.startDate, b.horizon)
    let phases: PlanPhase[] = []
    if (goal?.targetDate) {
      drafts = drafts.filter((x) => x.date <= goal.targetDate!)
      phases = goalPhases(b.startDate, goal.targetDate)
    }
    return json({ drafts, phases, errors: has('importerr') ? ['Séance 5 : date manquante ou invalide.'] : [], raw: JSON.stringify({ format: 'stride-plan', version: phases.length ? 2 : 1, phases, sessions: drafts }) })
  })
  route('POST', /^\/api\/plan\/import$/, ({ init }) => {
    const b = body<{ text?: string }>(init)
    if (!b?.text?.trim()) return err(400, 'BAD_REQUEST', 'Colle la réponse de l’IA.')
    const r = parsePlanText(b.text)
    if (has('importerr')) r.errors.push('Séance 3 : étape 2 : type « sprint » inconnu.')
    return json(r)
  })
  route('POST', /^\/api\/plan\/bulk$/, ({ data, init }) => {
    const b = body<{ sessions?: PlannedSessionInput[]; phases?: PlanPhase[] }>(init)
    if (!b || (!b.sessions?.length && !b.phases?.length)) return err(400, 'BAD_REQUEST', 'Aucune séance à créer.')
    if (b.phases?.length) {
      const st = v31Of(data)
      st.phases = st.phases.filter((p) => !b.phases!.some((n) => n.startDate <= p.endDate && n.endDate >= p.startDate))
      for (const p of b.phases) st.phases.push({ ...p, id: nextId() })
      st.phases.sort((x, y) => (x.startDate < y.startDate ? -1 : 1))
    }
    b.sessions = b.sessions ?? []
    const created = b.sessions.map((s) => {
      const p = { ...s, id: nextId(), steps: s.steps ?? [], source: s.source ?? 'import', garmin: { status: 'none' as const, workoutId: null, pushedAt: null, error: null } }
      data.plan.push(p)
      return p.id
    })
    const all = plannedWithMatch(data, visibleItems(data), todayISO())
    return json(all.filter((p) => created.includes(p.id)), 201)
  })
  route('POST', /^\/api\/plan\/(\d+)\/push-garmin$/, ({ data, params }) => {
    const p = data.plan.find((x) => x.id === Number(params[0]))
    if (!p) return err(404, 'NOT_FOUND', 'Séance introuvable.')
    if (!data.integrations.garmin.connected) return err(412, 'PROVIDER_NOT_CONNECTED', 'Connecte Garmin pour envoyer la séance.')
    if (!has('pushok')) {
      p.garmin = { status: 'unsupported', workoutId: null, pushedAt: null, error: 'L’API Garmin Training n’est pas ouverte aux clés personnelles (404).' }
      return err(409, 'CONFLICT', 'Envoi direct indisponible avec ta clé Garmin : télécharge le fichier .FIT.')
    }
    p.garmin = { status: 'pushed', workoutId: 900_000 + p.id, pushedAt: isoNow(), error: null }
    return json(plannedWithMatch(data, visibleItems(data), todayISO()).find((x) => x.id === p.id))
  })
  route('GET', /^\/api\/plan\/(\d+)\/workout\.fit$/, ({ data, params }) => {
    const p = data.plan.find((x) => x.id === Number(params[0]))
    if (!p) return err(404, 'NOT_FOUND', 'Séance introuvable.')
    const bytes = new Uint8Array([14, 32, 0x84, 0x08, 64, 0, 0, 0, 0x2e, 0x46, 0x49, 0x54, 0, 0])
    return new Response(bytes, { headers: { 'Content-Type': 'application/vnd.ant.fit', 'Content-Disposition': `attachment; filename="stride-${p.date}-${p.id}.fit"` } })
  })
  route('GET', /^\/api\/calendar$/, ({ data, url }) => {
    const from = url.searchParams.get('from') ?? ''
    const to = url.searchParams.get('to') ?? ''
    if (!DATE_RE.test(from) || !DATE_RE.test(to) || to < from) return err(400, 'BAD_REQUEST', 'Période invalide.')
    if (addDays(from, 371) < to) return err(400, 'BAD_REQUEST', 'Période limitée à 371 jours.')
    return json({ days: calendar(data, from, to) })
  })

  route('GET', /^\/api\/trends\/efficiency$/, ({ data, url }) => json(efficiencyTrend(data, Math.min(52, Math.max(1, Number(url.searchParams.get('weeks')) || 12)))))
  route('GET', /^\/api\/activities\/(\d+)\/track$/, ({ data, params }) => {
    const t = view(data, Number(params[0]))
    if (!t) return err(404, 'NOT_FOUND', 'Activité introuvable.')
    const track = trackFor(t.v, t.it)
    return track ? json(track, 200, { 'Cache-Control': 'private, no-store' }) : err(404, 'NOT_FOUND', 'Pas de tracé GPS pour cette activité.')
  })

  route('GET', /^\/api\/notifications$/, ({ data }) => json(v3(data).notifications))
  route('POST', /^\/api\/notifications$/, ({ data, init }) => {
    const b = body<NotificationInput>(init)
    const e = validNotif(b)
    if (e || !b) return err(400, 'BAD_REQUEST', e ?? 'Corps invalide.')
    const n: NotificationTarget = { id: nextId(), kind: b.kind, label: b.label.trim() || b.kind, enabled: b.enabled, hint: hintOf(b), lastStatus: null, lastSentAt: null, lastError: null }
    v3(data).notifications.push(n)
    return json(n, 201)
  })
  route('PATCH', /^\/api\/notifications\/(\d+)$/, ({ data, params, init }) => {
    const n = v3(data).notifications.find((x) => x.id === Number(params[0]))
    if (!n) return err(404, 'NOT_FOUND', 'Destination introuvable.')
    const b = body<Partial<NotificationInput>>(init) ?? {}
    if (b.label !== undefined) n.label = b.label.trim() || n.label
    if (b.enabled !== undefined) n.enabled = b.enabled
    if (b.url || b.botToken || b.chatId) n.hint = hintOf({ ...b, kind: n.kind } as NotificationInput)
    return json(n)
  })
  route('DELETE', /^\/api\/notifications\/(\d+)$/, ({ data, params }) => {
    const s = v3(data)
    const before = s.notifications.length
    s.notifications = s.notifications.filter((x) => x.id !== Number(params[0]))
    return before === s.notifications.length ? err(404, 'NOT_FOUND', 'Destination introuvable.') : noContent()
  })
  route('POST', /^\/api\/notifications\/(\d+)\/test$/, async ({ data, params }) => {
    const n = v3(data).notifications.find((x) => x.id === Number(params[0]))
    if (!n) return err(404, 'NOT_FOUND', 'Destination introuvable.')
    await new Promise((r) => setTimeout(r, 500))
    const fail = has('notiferr')
    n.lastSentAt = isoNow()
    n.lastStatus = fail ? '403' : n.kind === 'telegram' ? '200' : '204'
    n.lastError = fail ? 'Webhook refusé (403) : vérifie l’URL ou le jeton.' : null
    return json({ ok: !fail, status: Number(n.lastStatus), message: fail ? n.lastError : 'Message de test envoyé.' })
  })

  route('POST', /^\/api\/imports$/, async ({ data, init }) => {
    const form = init?.body instanceof FormData ? init.body : null
    const file = form?.get('file')
    if (!(file instanceof File)) return err(400, 'BAD_REQUEST', 'Fichier manquant.')
    if (!/\.(fit|gpx|tcx)$/i.test(file.name)) return err(400, 'BAD_REQUEST', 'Format non pris en charge : .fit, .gpx ou .tcx uniquement.')
    if (file.size > 25 * 1024 * 1024) return err(400, 'BAD_REQUEST', 'Fichier trop volumineux (25 Mo max).')
    await new Promise((r) => setTimeout(r, 700))
    if (has('importerr')) return err(400, 'BAD_REQUEST', `Fichier illisible : ${file.name.toLowerCase().endsWith('.fit') ? 'en-tête FIT invalide' : 'XML mal formé'}.`)
    const latest = visibleItems(data)[0]
    if (!latest) return err(400, 'BAD_REQUEST', 'Aucune activité correspondante.')
    return json({ activityId: latest.id, merged: /\.fit$/i.test(file.name) }, 201)
  })

  route('POST', /^\/api\/shoes\/(\d+)\/merge$/, ({ data, params, init }) => {
    const id = Number(params[0])
    const intoId = body<{ intoId?: number }>(init)?.intoId
    const src = data.shoes.find((s) => s.id === id)
    const dst = data.shoes.find((s) => s.id === intoId)
    if (!src || !dst) return err(404, 'NOT_FOUND', 'Chaussure introuvable.')
    if (src.id === dst.id) return err(400, 'BAD_REQUEST', 'Choisis une autre chaussure.')
    for (const n of Object.values(data.notes)) if (n.shoeId === src.id) n.shoeId = dst.id
    dst.distanceKm = Math.round((dst.distanceKm + src.distanceKm) * 10) / 10
    dst.stravaGearId = dst.stravaGearId ?? src.stravaGearId
    data.shoes = data.shoes.filter((s) => s.id !== src.id)
    return json(dst)
  })
}

function validNotif(b: NotificationInput | null): string | null {
  if (!b || !['discord', 'telegram', 'json'].includes(b.kind)) return 'Type de destination invalide.'
  if (b.kind === 'telegram') return b.botToken?.trim() && b.chatId?.trim() ? null : 'Jeton du bot et chat ID requis.'
  if (!/^https?:\/\/\S+$/.test(b.url ?? '')) return 'URL invalide (http(s)://…).'
  if (b.kind === 'discord' && !/discord(app)?\.com\/api\/webhooks\//.test(b.url ?? '')) return 'URL de webhook Discord attendue.'
  return null
}

function hintOf(b: NotificationInput): string {
  if (b.kind === 'telegram') return `bot …${(b.botToken ?? '').slice(-4)} · chat ${b.chatId ?? ''}`
  try {
    const u = new URL(b.url ?? '')
    return `${u.host}/…${u.pathname.slice(-4)}`
  } catch {
    return '…'
  }
}
