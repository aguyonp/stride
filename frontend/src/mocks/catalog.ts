// Catalogue d'activités fictives, relatif à « aujourd'hui » (dates relatives réalistes).

import type { ActivitySummary } from '../api/types'
import { addDays, todayISO } from '../lib/format'
import { elevGainLoss, hashStr, isRunKind, mean, rng, round, simulate, type Phase, type RunSpec, type Simulation } from './sim'

export interface CatalogItem {
  summary: ActivitySummary
  spec: RunSpec
  sim: Simulation
  locationName: string | null
  maxHR: number | null
}

const TYPE_LABEL: Record<string, string> = {
  running: 'Course',
  trail_running: 'Trail',
  treadmill_running: 'Course sur tapis',
  cycling: 'Vélo',
  walking: 'Marche',
  strength_training: 'Renforcement',
}
const RUNNING_TYPES = new Set(['running', 'trail_running', 'treadmill_running', 'track_running'])

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
      // Séance de référence : run/walk, équilibre G/D NON mesuré (cas réel actuel).
      daysAgo: 1,
      time: '18:30:12',
      name: 'Lyon - Run/Walk 4×10′',
      location: 'Lyon',
      spec: { ...baseSpec, structured: true, phases: runWalk(4, 600, 60, 346, 550), cadence: 177.5 },
    },
    {
      // Variante : équilibre G/D disponible (49,2 / 50,8) + plusieurs alertes.
      daysAgo: 3,
      time: '18:45:03',
      name: 'Lyon - Run/Walk 5×8′ (HRM-Pro)',
      location: 'Lyon',
      spec: {
        ...baseSpec,
        structured: true,
        phases: [...runWalk(4, 480, 60, 350), { kind: 'walk', durationS: 60, pace: 760 }, { kind: 'run', durationS: 480, pace: 382, hrBoost: 12 }],
        cadence: 168.5,
        vo: 9.5,
        gct: 262,
        drift: 0.075,
        balanceLeft: 49.2,
        elev: { base: 176, amp: 5, cycles: 1.5, trend: 2 },
      },
    },
    {
      daysAgo: 6,
      time: '10:15:40',
      name: 'Footing facile continu',
      location: 'Parc de la Tête d’Or',
      spec: { ...baseSpec, phases: [{ kind: 'run', durationS: 3000, pace: 349 }], cadence: 179, vo: 7.8, balanceLeft: 50.3, elev: { base: 170, amp: 3, cycles: 2, trend: 0 }, drift: 0.02 },
    },
    {
      daysAgo: 8,
      time: '18:20:44',
      name: 'Lyon - Run/Walk 6×5′',
      location: 'Lyon',
      spec: { ...baseSpec, structured: true, phases: runWalk(6, 300, 60, 352), cadence: 174, elev: { base: 182, amp: 6, cycles: 1, trend: -1 } },
    },
    {
      daysAgo: 10,
      time: '09:05:00',
      name: 'Trail des Monts d’Or',
      type: 'trail_running',
      location: 'Monts d’Or',
      spec: { ...baseSpec, phases: [{ kind: 'run', durationS: 4200, pace: 420 }], cadence: 171, vo: 8.6, gct: 275, power: 300, drift: 0.09, elev: { base: 420, amp: 34, cycles: 3, trend: -4 } },
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
      location: 'Lyon',
      spec: { ...baseSpec, structured: true, phases: intervals(900, 3, 437, 218, 120, 600), cadence: 176, vo: 8.4, gct: 228, power: 268, hrRef: 140, drift: 0.04, elev: { base: 175, amp: 3, cycles: 2, trend: 0 } },
    },
    {
      daysAgo: 20,
      time: '12:15:00',
      name: 'Footing 10 km',
      location: 'Lyon',
      spec: { ...baseSpec, phases: [{ kind: 'run', durationS: 3200, pace: 320 }], cadence: 178, hrRef: 146, drift: 0.045 },
    },
    {
      daysAgo: 24,
      time: '09:30:00',
      name: 'Sortie longue 3×3 km allure marathon',
      location: 'Berges du Rhône',
      spec: { ...baseSpec, structured: true, phases: intervals(1200, 3, 705, 235, 240, 900), cadence: 176, hrRef: 142, drift: 0.06, elev: { base: 165, amp: 4, cycles: 2, trend: 0 } },
    },
  ]
  // Footings plus anciens générés.
  const extra = [27, 31, 34, 38, 41, 45, 48, 52, 55, 59, 62, 66, 69, 73, 76, 80]
  for (const d of extra) {
    const r = rng(hashStr(`run-${d}`))
    const dur = 1800 + Math.round(r() * 2400)
    defs.push({
      daysAgo: d,
      time: r() > 0.5 ? '18:35:00' : '07:20:00',
      name: r() > 0.6 ? 'Footing' : 'Course à pied',
      location: 'Lyon',
      spec: {
        ...baseSpec,
        phases: [{ kind: 'run', durationS: dur, pace: 300 + Math.round(r() * 40) }],
        cadence: 174 + r() * 8,
        hrRef: 140 + r() * 10,
        drift: 0.02 + r() * 0.05,
        balanceLeft: r() > 0.5 ? 49.5 + r() * 1.2 : null,
      },
    })
  }
  return defs
}

let cache: { today: string; items: CatalogItem[] } | null = null

export function getCatalog(): CatalogItem[] {
  const today = todayISO()
  if (cache?.today === today) return cache.items
  const items: CatalogItem[] = []
  let idSeq = 0
  const mkId = (daysAgo: number) => 21_400_000_000 + (200 - daysAgo) * 100 + idSeq++

  for (const def of runDefs()) {
    const date = addDays(today, -def.daysAgo)
    const type = def.type ?? 'running'
    const id = mkId(def.daysAgo)
    const spec: RunSpec = { ...def.spec, seed: hashStr(`${id}`) }
    const sim = simulate(spec)
    items.push(buildRunItem(id, def.name, type, `${date}T${def.time}`, def.location ?? null, spec, sim))
  }

  // Activités non-course : vélotaf (jours ouvrés), renfo, marche.
  for (let d = 0; d <= 82; d++) {
    const date = addDays(today, -d)
    const wd = new Date(`${date}T12:00:00`).getDay() // 0 = dimanche
    const r = rng(hashStr(`other-${d}`))
    if (d === 0 || (wd >= 1 && wd <= 5 && r() > 0.12)) {
      items.push(other(mkId(d), 'Vélotaf', 'cycling', `${date}T08:${d === 0 ? '12' : '10'}:00`, 1330 + Math.round(r() * 60), 6100, 112, 4))
    }
    if (d === 0) items.push(other(mkId(d), 'Marche du midi', 'walking', `${date}T12:30:00`, 2100, 2800, 96, 3))
    else if ((wd === 2 || wd === 4) && r() > 0.3) items.push(other(mkId(d), 'Renforcement genou', 'strength_training', `${date}T19:05:00`, 1800 + Math.round(r() * 600), null, 104, 0))
    else if ((wd === 0 || wd === 6) && r() > 0.45) items.push(other(mkId(d), 'Balade', 'walking', `${date}T15:20:00`, 2700 + Math.round(r() * 1500), 3800 + Math.round(r() * 1600), 92, 10))
  }

  items.sort((a, b) => (a.summary.startLocal < b.summary.startLocal ? 1 : -1))
  cache = { today, items }
  return items
}

function trainingLoadFor(durationS: number, avgHR: number | null): number | null {
  if (avgHR === null) return null
  const k = 0.8 + Math.max(0, avgHR - 110) / 22
  return Math.round((durationS / 60) * k)
}

function buildRunItem(id: number, name: string, type: string, startLocal: string, location: string | null, spec: RunSpec, sim: Simulation): CatalogItem {
  const s = sim.series
  const runIdx = sim.points.map((p, i) => (isRunKind(p.kind) ? i : -1)).filter((i) => i >= 0)
  const pick = (arr: (number | null)[]) => runIdx.map((i) => arr[i])
  const avgHR = round(mean(s.hr))
  const eg = elevGainLoss(s.elevationM)
  const summary: ActivitySummary = {
    id,
    name,
    type,
    typeLabel: TYPE_LABEL[type] ?? 'Activité',
    isRunning: RUNNING_TYPES.has(type),
    startLocal,
    date: startLocal.slice(0, 10),
    distanceM: Math.round(sim.distanceM),
    durationS: Math.round(sim.durationS),
    avgPaceSecPerKm: Math.round(sim.durationS / (sim.distanceM / 1000)),
    avgHR,
    avgPower: spec.hasPower ? round(mean(s.powerW)) : null,
    avgCadenceSpm: round(mean(pick(s.cadenceSpm))),
    elevGainM: eg.gain,
    elevLossM: eg.loss,
    trainingLoad: trainingLoadFor(sim.durationS, avgHR),
  }
  const maxHR = s.hr.reduce<number>((m, v) => (v !== null && v > m ? v : m), 0)
  return { summary, spec, sim, locationName: location, maxHR }
}

function other(id: number, name: string, type: string, startLocal: string, durationS: number, distanceM: number | null, avgHR: number, elevAmp: number): CatalogItem {
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
    seed: hashStr(`${id}`),
  }
  const sim = simulate(spec)
  // Pas de dynamique de course hors course à pied.
  sim.series.cadenceSpm = sim.series.cadenceSpm.map(() => null)
  if (!distanceM) sim.series.paceSecPerKm = sim.series.paceSecPerKm.map(() => null)
  const eg = elevGainLoss(sim.series.elevationM)
  const summary: ActivitySummary = {
    id,
    name,
    type,
    typeLabel: TYPE_LABEL[type] ?? 'Activité',
    isRunning: false,
    startLocal,
    date: startLocal.slice(0, 10),
    distanceM,
    durationS,
    avgPaceSecPerKm: distanceM ? Math.round(pace) : null,
    avgHR: round(mean(sim.series.hr)),
    avgPower: null,
    avgCadenceSpm: null,
    elevGainM: distanceM ? eg.gain : null,
    elevLossM: distanceM ? eg.loss : null,
    trainingLoad: type === 'strength_training' ? 18 : trainingLoadFor(durationS, avgHR - 8),
  }
  return { summary, spec, sim, locationName: type === 'cycling' ? 'Lyon' : null, maxHR: avgHR + 18 }
}
