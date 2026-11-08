// In-memory mock database, persisted to sessionStorage and reseeded when the scenario flags or the day change.

import type {
  HealthStatus,
  LLMAnalysis,
  LLMConfig,
  NotificationTarget,
  WorkoutStep,
  AthleteProfile,
  DailyCheckin,
  FocusArea,
  FocusPreset,
  Goal,
  Integrations,
  Invitation,
  PlannedSession,
  Passkey,
  RegistrationMode,
  Role,
  Shoe,
  Thresholds,
} from '../api/types'
import { addDays, todayISO } from '../lib/format'
import { getCatalog } from './catalog'
import { seedShoes } from './gear'
import { flagsKey, has } from './flags'
import type { V31Data } from './manual'

export interface MockUser {
  id: number
  displayName: string
  role: Role
  status: 'active' | 'disabled'
  createdAt: string
  lastLoginAt: string | null
  onboardingCompleted: boolean
  passkeys: Passkey[]
}

export interface StoredNotes {
  painDuring: number | null
  painCold: number | null
  painNextMorning: number | null
  rpe: number | null
  shoeId: number | null
  tags: string[]
  note: string
  updatedAt: string | null
}

export type PlanEntry = Omit<PlannedSession, 'match'>

export interface UserData {
  profile: AthleteProfile
  goals: Goal[]
  plan: PlanEntry[]
  shoes: Shoe[]
  notes: Record<string, StoredNotes>
  checkins: Record<string, DailyCheckin>
  integrations: Integrations
  v3?: V3Data
  v31?: V31Data
  v4?: unknown
  /** 4.2 share notes (see ./v42.ts). */
  v42?: unknown
  health?: HealthStatus
}

export interface V3Data {
  llm: LLMConfig
  analyses: LLMAnalysis[]
  notifications: NotificationTarget[]
  pushSupported: boolean | null
}

export interface MockState {
  version: 2
  flags: string
  day: string
  seq: number
  users: MockUser[]
  sessionUserId: number | null
  registrationMode: RegistrationMode
  invitations: (Invitation & { code: string })[]
  data: Record<string, UserData>
  sync: { running: boolean; startedAt: number | null; lastSyncAt: string | null; lastLiveAt: string | null; liveUntil: number; runs: number; newGarmin: number; newStrava: number; cooldownUntil: string | null }
  pending: { displayName: string; invitationId: number | null } | null
  /** 4.2 instance backups and S3 target (see ./v42.ts). */
  v42?: unknown
}

const KEY = 'stride.mock.v2'

/* ---------------------------------- Presets ---------------------------------- */

const BASE: Thresholds = {
  cadenceMinSpm: 168,
  cadenceTargetMin: 170,
  cadenceTargetMax: 180,
  gctBalanceGapPct: 1.5,
  verticalOscillationMaxCm: 9.5,
  gctDriftMaxPct: 5,
  descentMaxM: 30,
  floorsPrevDayMax: 15,
  loadRatioMax: 1.3,
  rhrDeltaMaxBpm: 5,
  planDeviationPct: 20,
  downhillCadenceDropSpm: 5,
}

export const PRESETS: FocusPreset[] = [
  {
    focusArea: 'none',
    label: '100 % Performance – Aucune blessure',
    thresholds: { ...BASE, descentMaxM: 150, floorsPrevDayMax: 40, downhillCadenceDropSpm: 8 },
    painLabels: { during: 'Douleur pendant l’effort', cold: 'Douleur à froid (+2 h)', nextMorning: 'Douleur le lendemain matin' },
    painEnabled: false,
    emphasis: ['Allures cibles', 'Charge et ratio aigu/chronique', 'Récupération (VFC, sommeil)'],
  },
  {
    focusArea: 'knee_pfps',
    label: 'Genou / Fémoro-patellaire',
    thresholds: { ...BASE },
    painLabels: { during: 'Gêne genou pendant l’effort', cold: 'Gêne genou à froid (+2 h)', nextMorning: 'Gêne genou au réveil (lendemain)' },
    painEnabled: true,
    emphasis: ['Cadence', 'Descentes (D−)', 'Oscillation verticale', 'Étages J-1'],
  },
  {
    focusArea: 'achilles',
    label: 'Tendon d’Achille',
    thresholds: { ...BASE, gctDriftMaxPct: 4, descentMaxM: 60, floorsPrevDayMax: 25 },
    painLabels: { during: 'Tendon pendant l’effort', cold: 'Tendon à froid (+2 h)', nextMorning: 'Raideur du tendon au réveil' },
    painEnabled: true,
    emphasis: ['Cadence', 'Dérive GCT', 'Montées / D+', 'Charge mollet'],
  },
  {
    focusArea: 'shin_splints',
    label: 'Périostite tibiale',
    thresholds: { ...BASE, loadRatioMax: 1.2, descentMaxM: 60 },
    painLabels: { during: 'Tibia pendant l’effort', cold: 'Tibia à froid (+2 h)', nextMorning: 'Tibia au réveil (lendemain)' },
    painEnabled: true,
    emphasis: ['Cadence', 'Impact Load', 'Sauts de volume', 'Surfaces dures'],
  },
  {
    focusArea: 'other',
    label: 'Autre',
    thresholds: { ...BASE },
    painLabels: { during: 'Douleur pendant l’effort', cold: 'Douleur à froid (+2 h)', nextMorning: 'Douleur au réveil (lendemain)' },
    painEnabled: true,
    emphasis: ['Ressenti', 'Charge', 'Biomécanique'],
  },
]

export const presetFor = (f: FocusArea) => PRESETS.find((p) => p.focusArea === f) ?? PRESETS[0]

/* ---------------------------------- Seeding ---------------------------------- */

const iso = (msAgo: number) => new Date(Date.now() - msAgo).toISOString()
const H = 3600_000
const D = 24 * H

const STRAVA_SCOPES = ['read', 'activity:read_all', 'profile:read_all']

function integrationsFor(rich: boolean): Integrations {
  const stravaOnly = has('strava')
  const garminOnly = has('garmin') || has('nostrava')
  const standalone = has('standalone')
  const garmin = rich && !stravaOnly && !standalone
  const strava = rich && (stravaOnly || !garminOnly) && !standalone
  const scoped = strava && has('stravascope')
  const unauthorized = has('unauthorized')
  return {
    garmin: {
      connected: garmin,
      status: garmin ? (unauthorized ? 'error' : 'ok') : 'disconnected',
      keyHint: garmin ? '••••7f3c' : null,
      tokenExpiresAt: garmin && !unauthorized ? new Date(Date.now() + 20 * H).toISOString() : null,
      lastSyncAt: garmin ? iso(3 * H) : null,
      lastError: garmin && unauthorized ? 'Clé refusée par Garmin (révoquée ou expirée). Remplace-la dans Réglages › Intégrations.' : null,
    },
    strava: {
      available: !has('nostrava'),
      connected: strava,
      status: strava ? 'ok' : 'disconnected',
      athleteName: strava ? 'Alex D.' : null,
      lastSyncAt: strava ? iso(3 * H + 120_000) : null,
      lastError: null,
      scopes: strava ? (scoped ? ['read'] : STRAVA_SCOPES) : [],
      missingScopes: scoped ? STRAVA_SCOPES.slice(1) : [],
    },
    weather: { enabled: true, provider: 'open-meteo' },
  }
}

function defaultProfile(): AthleteProfile {
  const p = presetFor('none')
  return { focusArea: 'none', focusLabel: p.label, injuredSide: null, timezone: 'Europe/Paris', thresholds: { ...p.thresholds }, paceTargets: [], language: null }
}

export function emptyData(): UserData {
  return { profile: defaultProfile(), goals: [], plan: [], shoes: [], notes: {}, checkins: {}, integrations: integrationsFor(false) }
}

function richData(st: { seq: number }, today: string): UserData {
  const next = () => ++st.seq
  const knee = presetFor('knee_pfps')
  const profile: AthleteProfile = {
    focusArea: 'knee_pfps',
    focusLabel: knee.label,
    injuredSide: 'left',
    timezone: 'Europe/Paris',
    thresholds: { ...knee.thresholds, cadenceTargetMin: 172, cadenceTargetMax: 178 },
    paceTargets: [
      { label: '10 km', minSecPerKm: 215, maxSecPerKm: 220 },
      { label: 'Marathon', minSecPerKm: 230, maxSecPerKm: 240 },
    ],
    language: null,
  }
  const goals: Goal[] = [
    { id: next(), title: 'Courir 45 min continu sans gêne mi-novembre → reprise perf en décembre', kind: 'macro', targetValue: 45, targetUnit: 'min', targetDate: addDays(today, 12), status: 'active', note: 'Priorité : zéro gêne le lendemain.' },
    { id: next(), title: '10 km de printemps', kind: 'race', targetValue: 37, targetUnit: 'min', targetDate: addDays(today, 150), status: 'active', note: 'Allure cible 3:35–3:40/km.' },
    { id: next(), title: 'Cadence 172–178 spm sur toutes les sorties', kind: 'cadence', targetValue: 175, targetUnit: 'spm', targetDate: null, status: 'active', note: '' },
    { id: next(), title: 'Volume 25 km / semaine', kind: 'weekly_volume', targetValue: 25, targetUnit: 'km', targetDate: null, status: 'done', note: '' },
  ]
  const shoes: Shoe[] = seedShoes(next, new Date(Date.now() - 2 * 3600_000).toISOString())
  const run = (daysAgo: number) => getCatalog(today).find((it) => it.isRunning && it.date === addDays(today, -daysAgo))
  const notes: Record<string, StoredNotes> = {}
  const addNote = (daysAgo: number, n: Omit<StoredNotes, 'updatedAt'>) => {
    const it = run(daysAgo)
    if (it) notes[String(it.id)] = { ...n, updatedAt: new Date(new Date(`${it.date}T21:30:00`).getTime()).toISOString() }
  }
  addNote(1, { painDuring: 2, painCold: 3, painNextMorning: 3.5, rpe: 5, shoeId: shoes[0].id, tags: ['metronome', 'warmup_iso'], note: 'Gêne externe genou gauche dans la descente du pont.' })
  addNote(3, { painDuring: 3.5, painCold: 3, painNextMorning: 2, rpe: 7, shoeId: shoes[1].id, tags: ['desk_sitting'], note: 'Dernier bloc subi.' })
  addNote(6, { painDuring: 1, painCold: 1, painNextMorning: 0.5, rpe: 4, shoeId: shoes[0].id, tags: [], note: '' })
  addNote(8, { painDuring: 2, painCold: 2.5, painNextMorning: 2, rpe: 5, shoeId: shoes[0].id, tags: ['squatting_48h'], note: 'Travaux dans l’appartement la veille.' })
  addNote(10, { painDuring: 4, painCold: 5, painNextMorning: 4.5, rpe: 8, shoeId: shoes[0].id, tags: [], note: 'Trop de descente technique.' })
  addNote(13, { painDuring: 0.5, painCold: 0, painNextMorning: 0, rpe: 4, shoeId: shoes[1].id, tags: ['metronome'], note: '' })

  const plan: PlanEntry[] = []
  const add = (daysFromToday: number, p: Omit<PlanEntry, 'id' | 'date' | 'steps' | 'source' | 'garmin'>, steps: WorkoutStep[] = []) =>
    plan.push({ id: next(), date: addDays(today, daysFromToday), steps, source: 'manual', garmin: { status: 'none', workoutId: null, pushedAt: null, error: null }, ...p })
  const pace = (min: number, max: number) => ({ kind: 'pace' as const, min, max })
  const runP = (title: string, min: number, km: number | null, pMin: number | null, pMax: number | null, cad: number | null, instructions: string) => ({
    sport: 'run' as const,
    title,
    targetDurationS: min * 60,
    targetDistanceM: km === null ? null : km * 1000,
    targetPaceMinSecPerKm: pMin,
    targetPaceMaxSecPerKm: pMax,
    targetCadenceSpm: cad,
    instructions,
  })
  const none = { targetDurationS: null, targetDistanceM: null, targetPaceMinSecPerKm: null, targetPaceMaxSecPerKm: null, targetCadenceSpm: null }
  add(-8, runP('Run/Walk 6×5′', 35, 5.5, 370, 390, 175, '1′ de marche entre les blocs (allure moyenne marche incluse).'))
  add(-6, runP('Footing facile 50′', 50, 8.5, 340, 355, 176, 'Aisance respiratoire, terrain plat.'))
  add(-4, { sport: 'mobility', title: 'Mobilité hanches 15′', ...none, targetDurationS: 900, instructions: 'Routine hanches + chaîne postérieure.' })
  add(-3, runP('Run/Walk 5×8′', 45, 7.5, 340, 355, 175, 'Garder la cadence même sur le dernier bloc.'))
  add(-2, { sport: 'strength', title: 'Renforcement excentrique', ...none, targetDurationS: 1800, instructions: 'Squats lents tempo 3-1-1, step-down 3×10.' })
  add(-1, runP('Run/Walk 4×10′', 42, 7, 355, 365, 176, 'Métronome 176, éviter les descentes raides (allure moyenne marche incluse).'), [
    { type: 'warmup', durationS: 300, target: { kind: 'cadence', min: 172, max: 178 } },
    { type: 'repeat', repeat: 4, steps: [{ type: 'run', durationS: 600, target: pace(355, 365) }, { type: 'recovery', durationS: 60, note: 'Marche active' }] },
    { type: 'cooldown', durationS: 180 },
  ])
  add(0, runP('38 min continu', 38, 7.1, 315, 325, 170, 'Continu @ 5:20/km, arrêt si gêne > 3/10.'))
  add(1, { sport: 'rest', title: 'Repos', ...none, instructions: '' })
  add(2, runP('Fractionné court 6×1′', 40, 7, 290, 300, 178, '6×1′ vite / 1′30 trot, plat.'), [
    { type: 'warmup', durationS: 900, target: pace(330, 360) },
    { type: 'repeat', repeat: 6, steps: [{ type: 'interval', durationS: 60, target: pace(250, 260) }, { type: 'recovery', durationS: 90 }] },
    { type: 'cooldown', durationS: 600 },
  ])
  add(3, { sport: 'strength', title: 'Renfo isométrique quadriceps', ...none, targetDurationS: 1500, instructions: 'Wall-sit 5×45″, extension isométrique 5×30″.' })
  add(5, runP('Sortie longue 55′', 55, 9.5, 335, 350, 175, 'Terrain plat, ravitaillement eau.'))
  add(7, runP('Test 45′ continu', 45, 8.2, 320, 335, 175, 'Objectif macro : 45′ sans gêne.'))
  add(9, { sport: 'bike', title: 'Vélo souple 45′', ...none, targetDurationS: 2700, instructions: 'Récupération active.' })

  const checkins: Record<string, DailyCheckin> = {
    [today]: { date: today, trainingReadiness: 62, readinessStatus: 'Modérée', note: 'Jambes un peu lourdes.', updatedAt: iso(2 * H) },
    [addDays(today, -3)]: { date: addDays(today, -3), trainingReadiness: 41, readinessStatus: 'Faible', note: 'Mauvaise nuit.', updatedAt: iso(3 * D) },
  }

  return { profile, goals, plan, shoes, notes, checkins, integrations: integrationsFor(true) }
}

function passkey(name: string, created: number, used: number | null, backedUp = true): Passkey {
  return { id: `pk_${Math.abs(Math.round(created / 1000)).toString(36)}_${name.length}`, name, createdAt: iso(created), lastUsedAt: used === null ? null : iso(used), backedUp }
}

function seed(): MockState {
  const today = todayISO()
  const st = { seq: 1000 }
  const first = has('first')
  const onboarding = has('onboarding')
  const users: MockUser[] = first
    ? []
    : [
        { id: 1, displayName: 'Alex Démo', role: 'admin', status: 'active', createdAt: iso(92 * D), lastLoginAt: iso(2 * H), onboardingCompleted: !onboarding, passkeys: [passkey('iPhone', 92 * D, 2 * H), passkey('MacBook', 60 * D, 5 * D)] },
        { id: 2, displayName: 'Sam Testeur', role: 'user', status: 'active', createdAt: iso(40 * D), lastLoginAt: iso(1 * D), onboardingCompleted: true, passkeys: [passkey('Pixel', 40 * D, 1 * D)] },
        { id: 3, displayName: 'Lou Essai', role: 'user', status: 'disabled', createdAt: iso(20 * D), lastLoginAt: iso(15 * D), onboardingCompleted: true, passkeys: [passkey('Clé USB', 20 * D, 15 * D, false)] },
      ]
  const loggedOut = first || has('out') || has('open')
  const data: Record<string, UserData> = {}
  for (const u of users) data[String(u.id)] = u.id === 1 && onboarding ? emptyData() : u.id <= 2 ? richData(st, today) : emptyData()
  const invitations: MockState['invitations'] = first
    ? []
    : [
        { id: 1, label: 'Club du jeudi', codePrefix: 'K7Q2', code: 'K7Q2-M9XW-4TBE', createdAt: iso(3 * D), expiresAt: new Date(Date.now() + 4 * D).toISOString(), maxUses: 5, uses: 2, revoked: false },
        { id: 2, label: 'Kiné', codePrefix: 'P3VD', code: 'P3VD-8HJK-2LQA', createdAt: iso(12 * D), expiresAt: iso(5 * D), maxUses: 1, uses: 0, revoked: false },
        { id: 3, label: 'Ancien lien', codePrefix: 'Z1RC', code: 'Z1RC-5NNA-7YQE', createdAt: iso(30 * D), expiresAt: null, maxUses: 1, uses: 0, revoked: true },
      ]
  return {
    version: 2,
    flags: flagsKey(),
    day: today,
    seq: st.seq,
    users,
    sessionUserId: loggedOut ? null : has('user') ? 2 : 1,
    registrationMode: has('open') ? 'open' : 'invite',
    invitations,
    data,
    sync: {
      running: has('syncrunning'),
      startedAt: has('syncrunning') ? Date.now() : null,
      lastSyncAt: users.length ? iso(3 * H) : null,
      lastLiveAt: users.length ? iso(3 * H) : null,
      liveUntil: 0,
      runs: 0,
      newGarmin: 0,
      newStrava: 0,
      cooldownUntil: has('cooldown') ? new Date(Date.now() + 12 * 60_000).toISOString() : null,
    },
    pending: null,
  }
}

/* ---------------------------------- Persistence ---------------------------------- */

let state: MockState | null = null

function storage(): Storage | null {
  try {
    return typeof window !== 'undefined' ? window.sessionStorage : null
  } catch {
    return null
  }
}

export function db(): MockState {
  if (state) return state
  const raw = storage()?.getItem(KEY)
  if (raw) {
    try {
      const parsed = JSON.parse(raw) as MockState
      if (parsed.version === 2 && parsed.flags === flagsKey() && parsed.day === todayISO()) {
        state = parsed
        return state
      }
    } catch {
      /* reseed */
    }
  }
  state = seed()
  save()
  return state
}

export function save() {
  if (state) storage()?.setItem(KEY, JSON.stringify(state))
}

export function resetDb() {
  state = null
  storage()?.removeItem(KEY)
}

export const nextId = () => ++db().seq

export function currentUser(): MockUser | null {
  const s = db()
  return s.users.find((u) => u.id === s.sessionUserId && u.status === 'active') ?? null
}

export function userData(userId: number): UserData {
  const s = db()
  const key = String(userId)
  if (!s.data[key]) s.data[key] = emptyData()
  return s.data[key]
}
