// 3.1.0 mock state (§12): manual activities, calendar events, plan phases, manual records. Synthetic data only.
// Kept free of ./domain imports so domain.visibleItems can include manual activities.

import type { CalendarEvent, ManualActivityInput, PlanPhase, RecordKey, Sport } from '../api/types'
import { addDays, mondayOf, todayISO } from '../lib/format'
import type { CatalogItem } from './catalog'
import { nextId, type UserData } from './db'
import { has } from './flags'
import { hashStr, simulate, type RunSpec } from './sim'

export interface ManualEntry extends ManualActivityInput {
  id: number
}

export interface V31Data {
  manual: ManualEntry[]
  events: CalendarEvent[]
  phases: PlanPhase[]
  records: Partial<Record<RecordKey, { timeS: number | null; distanceM?: number; date: string | null }>>
}

const TYPE: Record<Sport, [string, string]> = {
  run: ['running', 'Course'],
  bike: ['cycling', 'Vélo'],
  walk: ['walking', 'Marche'],
  hike: ['hiking', 'Randonnée'],
  swim: ['swimming', 'Natation'],
  strength: ['strength_training', 'Renforcement'],
  other: ['other', 'Activité'],
}

function seedManual(today: string): ManualEntry[] {
  const run = (daysAgo: number, time: string, title: string, km: number, paceS: number, hr: number, cad: number, gain = 20): ManualEntry => ({
    id: nextId(),
    date: addDays(today, -daysAgo),
    startTime: time,
    sport: 'run',
    title,
    distanceM: Math.round(km * 1000),
    durationS: Math.round(km * paceS),
    avgHR: hr,
    maxHR: hr + 18,
    avgCadenceSpm: cad,
    elevGainM: gain,
    elevLossM: gain,
  })
  return [
    run(1, '18:20', 'Footing du soir', 7.2, 352, 142, 174),
    run(3, '07:15', 'Fractionné 6×1′', 6.8, 330, 151, 178, 12),
    { id: nextId(), date: addDays(today, -4), startTime: '19:00', sport: 'strength', title: 'Renfo genou', durationS: 1800, avgHR: 104 },
    run(6, '18:05', 'Sortie longue', 12.4, 360, 139, 172, 85),
    run(9, '12:30', 'Footing pause déj', 5.1, 348, 140, 175),
    run(12, '18:40', 'Test 5 km', 5.0, 286, 168, 181, 8),
  ]
}

function seedPhases(goalDate: string, today: string): PlanPhase[] {
  const start = addDays(mondayOf(today), 7)
  const weeks = Math.max(6, Math.floor((Date.parse(goalDate) - Date.parse(start)) / (7 * 86_400_000)))
  const cut = (w: number) => addDays(start, w * 7)
  const b1 = Math.round(weeks * 0.35)
  const b2 = b1 + 1
  const b3 = Math.max(b2 + 2, weeks - 2)
  const mk = (name: string, s: string, e: string, focus: string, km: number | null, note = ''): PlanPhase => ({ id: nextId(), name, startDate: s, endDate: e, focus, weeklyVolumeKm: km, note })
  return [
    mk('Développement', start, addDays(cut(b1), -1), 'volume aérobie, cadence 175 spm', 32, 'Montée de +8 %/sem max.'),
    mk('Assimilation', cut(b1), addDays(cut(b2), -1), 'semaine allégée', 22),
    mk('Spécifique 10 km', cut(b2), addDays(cut(b3), -1), 'seuil, VMA courte, allure 10 km', 38),
    mk('Affûtage', cut(b3), goalDate, 'fraîcheur, rappels d’allure', 20, 'Volume −40 %, intensité conservée.'),
  ]
}

/** Lazily seeded per user (reseeded with the whole mock DB when flags change). */
export function v31Of(d: UserData): V31Data {
  if (!d.v31) {
    const today = todayISO()
    const rich = d.goals.length > 0
    const race = d.goals.find((g) => g.kind === 'race' && g.targetDate)
    d.v31 = {
      manual: has('standalone') && rich ? seedManual(today) : [],
      events: rich ? [{ id: nextId(), kind: 'fatigue', startDate: addDays(today, -2), endDate: addDays(today, -2), severity: 1, note: 'Grosse journée de travail, jambes lourdes.' }] : [],
      phases: has('longgoal') && race?.targetDate ? seedPhases(race.targetDate, today) : [],
      records: rich ? { half: { timeS: 6080, date: '2025-10-19' } } : {},
    }
  }
  return d.v31
}

/* ---------------------------------- Manual entry → catalog item ---------------------------------- */

const cache = new Map<string, CatalogItem>()

export function manualItem(e: ManualEntry): CatalogItem {
  const key = JSON.stringify(e)
  const hit = cache.get(key)
  if (hit) return hit
  const isRun = e.sport === 'run'
  const dist = e.distanceM ?? null
  const pace = dist ? e.durationS / (dist / 1000) : 600
  const spec: RunSpec = {
    phases: [{ kind: isRun ? 'run' : e.sport === 'strength' ? 'stand' : 'walk', durationS: Math.max(60, e.durationS), pace }],
    structured: false,
    cadence: e.avgCadenceSpm ?? (isRun ? 170 : 0),
    vo: 0,
    gct: 0,
    power: 0,
    hrRef: (e.avgHR ?? 135) + 20,
    refPace: pace,
    drift: 0.02,
    balanceLeft: null,
    elev: null,
    hasPower: false,
    seed: hashStr(`manual-${e.id}`),
  }
  const [type, typeLabel] = TYPE[e.sport] ?? TYPE.other
  const item: CatalogItem = {
    id: e.id,
    garminId: 0,
    stravaId: null,
    manual: true,
    name: e.title?.trim() || `${typeLabel} (saisie manuelle)`,
    type,
    typeLabel,
    sport: e.sport,
    isRunning: isRun,
    startLocal: `${e.date}T${e.startTime ?? '12:00'}:00`,
    date: e.date,
    locationName: null,
    outdoor: false,
    maxHR: e.maxHR ?? null,
    spec,
    sim: simulate(spec),
    base: {
      distanceM: dist,
      durationS: e.durationS,
      avgPaceSecPerKm: dist ? Math.round(pace) : null,
      avgHR: e.avgHR ?? null,
      avgPower: null,
      avgCadenceSpm: e.avgCadenceSpm ?? null,
      elevGainM: e.elevGainM ?? null,
      elevLossM: e.elevLossM ?? null,
      trainingLoad: null,
    },
    strava: null,
  }
  cache.set(key, item)
  return item
}

export const manualItems = (d: UserData): CatalogItem[] => v31Of(d).manual.map(manualItem)
