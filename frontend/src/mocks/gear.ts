// Strava "My Gear" of the demo athlete (§14.5): 7 active pairs (6 on Strava + 1 kept in Stride only)
// and 3 retired ones (two retired on Strava, one deleted there). Strava's total is the real mileage;
// `analyzedKm` is the part of it Stride holds activities for.
import type { Shoe, ShoeStravaState, ShoeType } from '../api/types'

interface Pair {
  gearId: string | null
  name: string
  brand: string
  model: string
  type: ShoeType
  dropMm: number | null
  stravaKm: number | null
  analyzedKm: number
  primary?: boolean
  state: ShoeStravaState | null
  note?: string
}

// Order matters: the first three are referenced by the seeded notes (daily, tempo, carbon).
const PAIRS: Pair[] = [
  { gearId: 'g2741001', name: 'Souple du quotidien', brand: 'Vela', model: 'Cloudline 3', type: 'soft', dropMm: 10, stravaKm: 1065.5, analyzedKm: 412.6, primary: true, state: 'active', note: 'Footings et run/walk' },
  { gearId: 'g2741002', name: 'Tempo', brand: 'Kairo', model: 'Swift 2', type: 'tempo', dropMm: 8, stravaKm: 486.2, analyzedKm: 186.2, state: 'active' },
  { gearId: 'g2741005', name: 'Carbone compétition', brand: 'Kairo', model: 'Apex Elite', type: 'carbon', dropMm: 6, stravaKm: 164, analyzedKm: 64, state: 'active', note: 'Courses et séances clés uniquement' },
  { gearId: 'g2741003', name: 'Orra Trail XT', brand: 'Orra', model: 'Trail XT', type: 'soft', dropMm: 4, stravaKm: 296.3, analyzedKm: 96.3, state: 'active' },
  { gearId: 'g2741007', name: 'Fractionné piste', brand: 'Norde', model: 'Pace 5', type: 'tempo', dropMm: 6, stravaKm: 221.7, analyzedKm: 121.7, state: 'active' },
  { gearId: 'g2741006', name: 'Vela Drift 2', brand: 'Vela', model: 'Drift 2', type: 'soft', dropMm: 8, stravaKm: 38.4, analyzedKm: 38.4, state: 'active' },
  { gearId: null, name: 'Pointes cross', brand: 'Lumo', model: 'XC Spike', type: 'carbon', dropMm: 4, stravaKm: null, analyzedKm: 18.2, state: null, note: 'Hors Strava' },
  { gearId: 'g2740991', name: 'Vela Cloudline 2', brand: 'Vela', model: 'Cloudline 2', type: 'soft', dropMm: 10, stravaKm: 1012.8, analyzedKm: 0, state: 'retired' },
  { gearId: 'g2740992', name: 'Kairo Swift 1', brand: 'Kairo', model: 'Swift 1', type: 'tempo', dropMm: 8, stravaKm: 684, analyzedKm: 22.5, state: 'retired' },
  { gearId: 'g2740993', name: 'Orra Ridge 4', brand: 'Orra', model: 'Ridge 4', type: 'soft', dropMm: 6, stravaKm: 405.9, analyzedKm: 0, state: 'deleted' },
]

function toShoe(p: Pair, id: number, syncedAt: string | null): Shoe {
  const linked = p.gearId !== null && p.stravaKm !== null
  return {
    id,
    name: p.name,
    brand: p.brand,
    model: p.model,
    dropMm: p.dropMm,
    type: p.type,
    stravaGearId: p.gearId,
    retired: p.state === 'retired' || p.state === 'deleted',
    distanceKm: linked ? p.stravaKm! : p.analyzedKm,
    analyzedKm: p.analyzedKm,
    distanceSource: linked ? 'strava' : 'stride',
    stravaDistanceKm: linked ? p.stravaKm : null,
    stravaPrimary: !!p.primary,
    stravaState: p.state,
    stravaSyncedAt: linked ? syncedAt : null,
    note: p.note ?? '',
  }
}

/** The seeded pairs, as after a Strava gear sync at `syncedAt`. */
export function seedShoes(next: () => number, syncedAt: string): Shoe[] {
  return PAIRS.map((p) => toShoe(p, next(), syncedAt))
}

/** Local pair without any Strava data (created by hand). */
export function localShoe(s: Omit<Shoe, 'distanceKm' | 'analyzedKm' | 'distanceSource' | 'stravaDistanceKm' | 'stravaPrimary' | 'stravaState' | 'stravaSyncedAt'>): Shoe {
  return { ...s, distanceKm: 0, analyzedKm: 0, distanceSource: 'stride', stravaDistanceKm: null, stravaPrimary: false, stravaState: null, stravaSyncedAt: null }
}

/**
 * Mirrors POST /api/shoes/import-strava: Strava lists the active pairs, GET /gear/{id} tells the
 * retired ones, and a 404 marks a pair deleted on Strava (kept, retired, last total). The Strava
 * retired flag only applies when the Strava state changes, like the backend.
 */
export function syncStravaGear(shoes: Shoe[], nextId: () => number, now: string): Shoe[] {
  for (const p of PAIRS) {
    if (p.gearId === null || p.state === 'deleted') continue
    const cur = shoes.find((s) => s.stravaGearId === p.gearId) ?? shoes.find((s) => !s.stravaGearId && s.brand === p.brand && s.model === p.model)
    if (!cur) {
      shoes.push(toShoe(p, nextId(), now))
      continue
    }
    const changed = cur.stravaState !== p.state
    Object.assign(cur, {
      stravaGearId: p.gearId,
      brand: p.brand,
      model: p.model,
      distanceKm: p.stravaKm,
      distanceSource: 'strava',
      stravaDistanceKm: p.stravaKm,
      stravaPrimary: !!p.primary,
      stravaState: p.state,
      stravaSyncedAt: now,
      retired: changed ? p.state !== 'active' : cur.retired,
    })
  }
  const listed = new Set(PAIRS.filter((p) => p.state !== 'deleted').map((p) => p.gearId))
  for (const s of shoes) {
    if (!s.stravaGearId || listed.has(s.stravaGearId)) continue
    if (s.stravaState !== 'deleted') s.retired = true
    Object.assign(s, { stravaState: 'deleted', stravaPrimary: false, stravaSyncedAt: now })
  }
  return shoes
}
