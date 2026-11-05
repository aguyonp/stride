// Synthetic activity catalogue relative to today (generic places, synthetic ids).

import type { Sport } from '../api/types'
import { addDays, todayISO } from '../lib/format'
import { elevGainLoss, hashStr, isRunKind, mean, rng, round, simulate, type Phase, type RunSpec, type Simulation } from './sim'

export interface CatalogItem {
  id: number
  garminId: number
  stravaId: number | null
  name: string
  type: string
  typeLabel: string
  sport: Sport
  isRunning: boolean
  startLocal: string
  date: string
  locationName: string | null
  outdoor: boolean
  maxHR: number | null
  spec: RunSpec
  sim: Simulation
  base: {
    distanceM: number | null
    durationS: number | null
    avgPaceSecPerKm: number | null
    avgHR: number | null
    avgPower: number | null
    avgCadenceSpm: number | null
    elevGainM: number | null
    elevLossM: number | null
    trainingLoad: number | null
  }
  strava: { title: string; description: string; privateNote: string; rpe: number | null; relativeEffort: number | null } | null
}

const TYPE_LABEL: Record<string, string> = {
  running: 'Course',
  trail_running: 'Trail',
  treadmill_running: 'Course sur tapis',
  cycling: 'Vélo',
  walking: 'Marche',
  strength_training: 'Renforcement',
}
const SPORT: Record<string, Sport> = {
  running: 'run',
  trail_running: 'run',
  treadmill_running: 'run',
  cycling: 'bike',
  walking: 'walk',
  strength_training: 'strength',
}

const runWalk = (reps: number, runS: number, walkS: number, pace: number, lastRunS = runS): Phase[] => {
  const out: Phase[] = []
  for (let i = 0; i < reps; i++) {
    out.push({ kind: 'run', durationS: i === reps - 1 ? lastRunS : runS, pace })
    if (i < reps - 1) out.push({ kind: 'walk', durationS: walkS, pace: 760 })
  }
  return out
}

const intervals = (warmS: number, reps: number, workS: number, workPace: number, restS: number, coolS: number): Phase[] => {
  const out: Phase[] = [{ kind: 'warmup', durationS: warmS, pace: 335 }]
  for (let i = 0; i < reps; i++) {
    out.push({ kind: 'active', durationS: workS, pace: workPace })
    if (i < reps - 1) out.push({ kind: 'rest', durationS: restS, pace: 420 })
  }
  out.push({ kind: 'cooldown', durationS: coolS, pace: 350 })
  return out
}

interface RunDef {
  daysAgo: number
  time: string
  name: string
  type?: string
  location?: string | null
  spec: Omit<RunSpec, 'seed'>
  strava?: CatalogItem['strava']
}

const baseSpec = {
  cadence: 177,
  vo: 8.1,
  gct: 248,
  power: 270,
  hrRef: 138,
  refPace: 346,
  drift: 0.03,
  balanceLeft: null,
  elev: { base: 182, amp: 8, cycles: 2, trend: -3 },
  hasPower: true,
  structured: false,
} satisfies Partial<RunSpec>

function runDefs(): RunDef[] {
  const defs: RunDef[] = [
    {
      daysAgo: 1,
      time: '18:30:12',
      name: 'Run/Walk 4×10′',
      location: 'Parc des berges',
      spec: { ...baseSpec, structured: true, phases: runWalk(4, 600, 60, 346, 550), cadence: 177.5, elev: { base: 182, amp: 14, cycles: 2, trend: -3 }, downhillCadenceLoss: 2.4 },
      strava: { title: 'Run/Walk 4×10′ – reprise', description: 'Blocs réguliers, métronome à 176.', privateNote: 'Gêne légère côté gauche dans la descente du pont.', rpe: 5, relativeEffort: 54 },
    },
    {
      daysAgo: 3,
      time: '18:45:03',
      name: 'Run/Walk 5×8′ (ceinture HRM)',
      location: 'Circuit vallonné',
      spec: {
        ...baseSpec,
        structured: true,
        phases: [...runWalk(4, 480, 60, 350), { kind: 'walk', durationS: 60, pace: 760 }, { kind: 'run', durationS: 480, pace: 382, hrBoost: 12 }],
        cadence: 168.5,
        vo: 9.7,
        gct: 262,
        drift: 0.075,
        balanceLeft: 49.2,
        elev: { base: 176, amp: 10, cycles: 1.5, trend: 2 },
        downhillCadenceLoss: 2,
      },
      strava: { title: 'Run/Walk 5×8′', description: 'Dernier bloc difficile, jambes lourdes.', privateNote: '', rpe: 7, relativeEffort: 71 },
    },
    {
      daysAgo: 6,
      time: '10:15:40',
      name: 'Footing facile continu',
      location: 'Parc',
      spec: { ...baseSpec, phases: [{ kind: 'run', durationS: 3000, pace: 349 }], cadence: 179, vo: 7.8, balanceLeft: 50.3, elev: { base: 170, amp: 3, cycles: 2, trend: 0 }, drift: 0.02 },
      strava: { title: 'Footing du dimanche', description: '', privateNote: '', rpe: 4, relativeEffort: 48 },
    },
    {
      daysAgo: 8,
      time: '18:20:44',
      name: 'Run/Walk 6×5′',
      location: 'Berges',
      spec: { ...baseSpec, structured: true, phases: runWalk(6, 300, 60, 352), cadence: 174, elev: { base: 182, amp: 6, cycles: 1, trend: -1 } },
      strava: { title: 'Run/Walk 6×5′', description: '', privateNote: '', rpe: 5, relativeEffort: 36 },
    },
    {
      daysAgo: 10,
      time: '09:05:00',
      name: 'Trail des collines',
      type: 'trail_running',
      location: 'Sentier des crêtes',
      spec: { ...baseSpec, phases: [{ kind: 'run', durationS: 4200, pace: 420 }], cadence: 171, vo: 8.6, gct: 275, power: 300, drift: 0.09, elev: { base: 420, amp: 34, cycles: 3, trend: -4 }, downhillCadenceLoss: 1.6 },
      strava: { title: 'Trail des collines', description: 'Beaucoup de descente technique.', privateNote: 'Genou sensible après 50 min.', rpe: 8, relativeEffort: 118 },
    },
    {
      daysAgo: 13,
      time: '07:10:22',
      name: 'Tapis - travail de cadence',
      type: 'treadmill_running',
      location: null,
      spec: { ...baseSpec, phases: [{ kind: 'run', durationS: 1800, pace: 340 }], cadence: 180.5, vo: 7.6, gct: 240, elev: null, hasPower: false, drift: 0.025 },
    },
    {
      daysAgo: 17,
      time: '18:40:10',
      name: 'Seuil 3×2 km allure 10 km',
      location: 'Piste',
      spec: { ...baseSpec, structured: true, phases: intervals(900, 3, 437, 218, 120, 600), cadence: 176, vo: 8.4, gct: 228, power: 268, hrRef: 140, drift: 0.04, elev: { base: 175, amp: 3, cycles: 2, trend: 0 } },
      strava: { title: 'Seuil 3×2 km', description: '', privateNote: '', rpe: 8, relativeEffort: 96 },
    },
    {
      daysAgo: 20,
      time: '12:15:00',
      name: 'Footing 10 km',
      location: 'Berges',
      spec: { ...baseSpec, phases: [{ kind: 'run', durationS: 3200, pace: 320 }], cadence: 178, hrRef: 146, drift: 0.045 },
      strava: { title: 'Footing 10 km', description: '', privateNote: '', rpe: 6, relativeEffort: 74 },
    },
    {
      daysAgo: 24,
      time: '09:30:00',
      name: 'Sortie longue 3×3 km allure marathon',
      location: 'Berges',
      spec: { ...baseSpec, structured: true, phases: intervals(1200, 3, 705, 235, 240, 900), cadence: 176, hrRef: 142, drift: 0.06, elev: { base: 165, amp: 4, cycles: 2, trend: 0 } },
      strava: { title: 'Sortie longue AM', description: '', privateNote: '', rpe: 7, relativeEffort: 132 },
    },
  ]
  for (const d of [27, 31, 34, 38, 41, 45, 48, 52, 55, 59, 62, 66, 69, 73, 76, 80]) {
    const r = rng(hashStr(`run-${d}`))
    const dur = 1800 + Math.round(r() * 2400)
    defs.push({
      daysAgo: d,
      time: r() > 0.5 ? '18:35:00' : '07:20:00',
      name: r() > 0.6 ? 'Footing' : 'Course à pied',
      location: 'Parc',
      spec: {
        ...baseSpec,
        phases: [{ kind: 'run', durationS: dur, pace: 300 + Math.round(r() * 40) }],
        cadence: 174 + r() * 8,
        hrRef: 140 + r() * 10,
        drift: 0.02 + r() * 0.05,
        balanceLeft: r() > 0.5 ? 49.5 + r() * 1.2 : null,
      },
      strava: { title: 'Footing', description: '', privateNote: '', rpe: 5, relativeEffort: Math.round(dur / 50) },
    })
  }
  return defs
}

export function trainingLoadFor(durationS: number, avgHR: number | null): number | null {
  if (avgHR === null) return null
  const k = 0.8 + Math.max(0, avgHR - 110) / 22
  return Math.round((durationS / 60) * k)
}

type Draft = Omit<CatalogItem, 'id' | 'garminId' | 'stravaId'> & { onStrava: boolean }

function buildRun(def: RunDef, today: string): Draft {
  const date = addDays(today, -def.daysAgo)
  const type = def.type ?? 'running'
  const spec: RunSpec = { ...def.spec, seed: hashStr(`${date}-${def.name}`) }
  const sim = simulate(spec)
  const s = sim.series
  const runIdx = sim.points.map((p, i) => (isRunKind(p.kind) ? i : -1)).filter((i) => i >= 0)
  const avgHR = round(mean(s.hr))
  const eg = elevGainLoss(s.elevationM)
  return {
    name: def.name,
    type,
    typeLabel: TYPE_LABEL[type] ?? 'Activité',
    sport: SPORT[type] ?? 'other',
    isRunning: true,
    startLocal: `${date}T${def.time}`,
    date,
    locationName: def.location ?? null,
    outdoor: type !== 'treadmill_running',
    maxHR: s.hr.reduce<number>((m, v) => (v !== null && v > m ? v : m), 0),
    spec,
    sim,
    base: {
      distanceM: Math.round(sim.distanceM),
      durationS: Math.round(sim.durationS),
      avgPaceSecPerKm: Math.round(sim.durationS / (sim.distanceM / 1000)),
      avgHR,
      avgPower: spec.hasPower ? round(mean(s.powerW)) : null,
      avgCadenceSpm: round(mean(runIdx.map((i) => s.cadenceSpm[i]))),
      elevGainM: eg.gain,
      elevLossM: eg.loss,
      trainingLoad: trainingLoadFor(sim.durationS, avgHR),
    },
    strava: def.strava ?? null,
    onStrava: def.strava !== undefined,
  }
}

function buildOther(name: string, type: string, startLocal: string, durationS: number, distanceM: number | null, avgHR: number, elevAmp: number, onStrava: boolean): Draft {
  const pace = distanceM ? durationS / (distanceM / 1000) : 600
  const spec: RunSpec = {
    phases: [{ kind: type === 'strength_training' ? 'stand' : 'walk', durationS, pace }],
    structured: false,
    cadence: 0,
    vo: 0,
    gct: 0,
    power: 0,
    hrRef: avgHR + 24,
    refPace: pace,
    drift: 0.02,
    balanceLeft: null,
    elev: elevAmp > 0 ? { base: 175, amp: elevAmp, cycles: 1, trend: 0 } : null,
    hasPower: false,
    seed: hashStr(`${startLocal}-${name}`),
  }
  const sim = simulate(spec)
  sim.series.cadenceSpm = sim.series.cadenceSpm.map(() => null)
  if (!distanceM) sim.series.paceSecPerKm = sim.series.paceSecPerKm.map(() => null)
  const eg = elevGainLoss(sim.series.elevationM)
  const hr = round(mean(sim.series.hr))
  return {
    name,
    type,
    typeLabel: TYPE_LABEL[type] ?? 'Activité',
    sport: SPORT[type] ?? 'other',
    isRunning: false,
    startLocal,
    date: startLocal.slice(0, 10),
    locationName: type === 'cycling' ? 'Trajet domicile-travail' : null,
    outdoor: type !== 'strength_training',
    maxHR: avgHR + 18,
    spec,
    sim,
    base: {
      distanceM,
      durationS,
      avgPaceSecPerKm: distanceM ? Math.round(pace) : null,
      avgHR: hr,
      avgPower: null,
      avgCadenceSpm: null,
      elevGainM: distanceM ? eg.gain : null,
      elevLossM: distanceM ? eg.loss : null,
      trainingLoad: type === 'strength_training' ? 18 : trainingLoadFor(durationS, avgHR - 8),
    },
    strava: onStrava ? { title: name, description: '', privateNote: '', rpe: null, relativeEffort: Math.round(durationS / 140) } : null,
    onStrava,
  }
}

let cache: { today: string; items: CatalogItem[] } | null = null

export function getCatalog(today = todayISO()): CatalogItem[] {
  if (cache?.today === today) return cache.items
  const drafts: Draft[] = runDefs().map((d) => buildRun(d, today))
  for (let d = 0; d <= 82; d++) {
    const date = addDays(today, -d)
    const wd = new Date(`${date}T12:00:00`).getDay()
    const r = rng(hashStr(`other-${d}`))
    if (d === 0 || (wd >= 1 && wd <= 5 && r() > 0.12)) {
      drafts.push(buildOther('Vélotaf', 'cycling', `${date}T08:${d === 0 ? '12' : '10'}:00`, 1330 + Math.round(r() * 60), 6100, 112, 4, true))
      if (d === 1 || (d > 1 && r() > 0.25)) drafts.push(buildOther('Vélotaf retour', 'cycling', `${date}T17:${d === 1 ? '20' : '45'}:00`, 1420 + Math.round(r() * 80), 6250, 118, 4, true))
    }
    if (d === 0) drafts.push(buildOther('Marche du midi', 'walking', `${date}T12:30:00`, 2100, 2800, 96, 3, false))
    else if ((wd === 2 || wd === 4) && r() > 0.3) drafts.push(buildOther('Renforcement genou', 'strength_training', `${date}T19:05:00`, 1800 + Math.round(r() * 600), null, 104, 0, false))
    else if ((wd === 0 || wd === 6) && r() > 0.45) drafts.push(buildOther('Balade', 'walking', `${date}T15:20:00`, 2700 + Math.round(r() * 1500), 3800 + Math.round(r() * 1600), 92, 10, false))
  }
  drafts.sort((a, b) => (a.startLocal < b.startLocal ? -1 : 1))
  const items: CatalogItem[] = drafts.map(({ onStrava, ...d }, i) => ({
    ...d,
    id: 101 + i,
    garminId: 90_000_000 + i * 7919,
    stravaId: onStrava ? 70_000_000 + i * 104_729 : null,
  }))
  items.reverse()
  cache = { today, items }
  return items
}

export const findItem = (id: number, today?: string) => getCatalog(today).find((it) => it.id === id) ?? null
