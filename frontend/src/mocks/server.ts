// Fake Stride V2/V3 backend for `npm run dev:mock`: returns real `Response` objects so the same client code runs.
//
// Scenario flags: `?mock=a,b,c` in the page URL (kept in sessionStorage across SPA navigation; a new list reseeds the data).
//   auth     admin (default: logged-in admin, onboarding done) · user (regular user) · onboarding (logged in, onboarding
//            not done, nothing connected) · out (logged out, invite mode) · open (logged out, open registration) ·
//            first (no user yet → first-user flow)
//   sources  fusion (default: Garmin + Strava) · garmin (Garmin only, Strava available) · strava (Strava only: Garmin
//            metrics null with reasons) · nostrava (Strava not configured on the server)
//   states   empty (no activities) · cooldown (provider 429 → cooldown) · syncrunning (sync never ends) ·
//            unauthorized (Garmin key refused) · fresh-db / fresh-cache / fresh-live (force freshness) · slow (latency ×4) ·
//            pkcancel (passkey ceremony cancelled) · stravaerr (Strava OAuth returns an error)
//   v3       see ./v3.ts (nollm, llmerr, pushok, importerr, notiferr, ready-*, noeff)
// Example: /?mock=user,strava,cooldown

import type {
  LogEntry,
  AdminConnection,
  AdminUser,
  ApiErrorBody,
  AthleteProfile,
  DailyCheckin,
  ErrorCode,
  FocusArea,
  Goal,
  GoalKind,
  Me,
  MeterSnapshot,
  Passkey,
  PlanSport,
  RestoreResult,
  Shoe,
  ShoeType,
  SyncStatus,
} from '../api/types'
import { todayISO } from '../lib/format'
import { CONTEXT_TAGS } from '../lib/labels'
import { getCatalog } from './catalog'
import { currentUser, db, emptyData, nextId, presetFor, PRESETS, save, userData, type MockUser, type PlanEntry, type StoredNotes, type UserData } from './db'
import { notesOf, plannedWithMatch, summarize, visibleItems, weekPlan } from './domain'
import { has } from './flags'
import { activityReport, dayReport, multiReport, parseIncludeParam } from './report'
import { buildActivityView, buildDayView } from './view'
import { enrichActivityView, enrichDayView, registerV3 } from './v3'

const SAFE = new Set(['GET', 'HEAD', 'OPTIONS'])
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))
const origin = () => (typeof window !== 'undefined' ? window.location.origin : 'http://localhost')

function json(body: unknown, status = 200, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json; charset=utf-8', ...headers } })
}
const noContent = (status = 204) => new Response(null, { status })
function err(status: number, code: ErrorCode, message: string): Response {
  const body: ApiErrorBody = { error: message, code }
  return json(body, status)
}
const markdown = (text: string) => new Response(text, { headers: { 'Content-Type': 'text/markdown; charset=utf-8' } })

function parse<T>(init?: RequestInit): T | null {
  if (!init?.body || typeof init.body !== 'string') return null
  try {
    return JSON.parse(init.body) as T
  } catch {
    return null
  }
}

const isoNow = () => new Date().toISOString()
const toMe = (u: MockUser): Me => ({ id: u.id, displayName: u.displayName, role: u.role, createdAt: u.createdAt, onboardingCompleted: u.onboardingCompleted })
const toAdmin = (u: MockUser): AdminUser => ({ id: u.id, displayName: u.displayName, role: u.role, status: u.status, createdAt: u.createdAt, lastLoginAt: u.lastLoginAt, passkeys: u.passkeys.length })
const rand = (len: number, alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789') => Array.from({ length: len }, () => alphabet[Math.floor(Math.random() * alphabet.length)]).join('')

function deviceName(): string {
  const ua = typeof navigator !== 'undefined' ? navigator.userAgent : ''
  if (/iPhone/.test(ua)) return 'iPhone'
  if (/iPad/.test(ua)) return 'iPad'
  if (/Android/.test(ua)) return 'Android'
  if (/Mac OS X/.test(ua)) return 'Mac'
  if (/Windows/.test(ua)) return 'Windows'
  return 'Navigateur'
}

function fakeCreationOptions(name: string) {
  return {
    publicKey: {
      challenge: rand(43, 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_'),
      rp: { name: 'Stride', id: typeof window !== 'undefined' ? window.location.hostname : 'localhost' },
      user: { id: rand(22, 'abcdefghijklmnopqrstuvwxyz0123456789'), name, displayName: name },
      pubKeyCredParams: [
        { type: 'public-key', alg: -7 },
        { type: 'public-key', alg: -257 },
      ],
      timeout: 60000,
      authenticatorSelection: { residentKey: 'required', requireResidentKey: true, userVerification: 'preferred' },
      attestation: 'none',
    },
  }
}

/* ---------------------------------- Sync ---------------------------------- */

function tickSync() {
  const s = db().sync
  if (s.running && !has('syncrunning') && s.startedAt !== null && Date.now() - s.startedAt > 4000) {
    const now = isoNow()
    s.running = false
    s.startedAt = null
    s.lastSyncAt = now
    s.lastLiveAt = now
    s.liveUntil = Date.now() + 60_000
    s.newGarmin = s.runs === 1 ? 2 : 0
    s.newStrava = s.runs === 1 ? 1 : 0
    for (const u of db().users) {
      const d = userData(u.id)
      if (d.integrations.garmin.connected && !has('cooldown')) d.integrations.garmin.lastSyncAt = now
      if (d.integrations.strava.connected) d.integrations.strava.lastSyncAt = now
    }
    save()
  }
}

function syncStatus(d: UserData): SyncStatus {
  const s = db().sync
  const cooldown = s.cooldownUntil && new Date(s.cooldownUntil).getTime() > Date.now() ? s.cooldownUntil : null
  const garminErr = cooldown ? 'Garmin a limité les requêtes (429) : reprise automatique après la pause.' : has('unauthorized') ? 'Clé refusée par Garmin (401) : remplace-la dans Réglages.' : null
  return {
    running: s.running,
    lastSyncAt: s.lastSyncAt,
    lastLiveAt: s.lastLiveAt,
    cooldownUntil: cooldown,
    providers: {
      garmin: d.integrations.garmin.connected ? { lastSyncAt: d.integrations.garmin.lastSyncAt, newActivities: s.newGarmin, error: garminErr } : null,
      strava: d.integrations.strava.connected ? { lastSyncAt: d.integrations.strava.lastSyncAt, newActivities: s.newStrava, error: null } : null,
    },
  }
}

/* ---------------------------------- Backup ---------------------------------- */

function backupOf(d: UserData) {
  const items = getCatalog()
  return {
    format: 'stride-backup',
    version: 1,
    exportedAt: isoNow(),
    profile: d.profile,
    goals: d.goals.map(({ id: _id, ...g }) => g),
    plan: d.plan.map(({ id: _id, ...p }) => p),
    shoes: d.shoes.map(({ id: _id, ...s }) => s),
    checkins: Object.values(d.checkins),
    activityNotes: Object.entries(d.notes).map(([aid, n]) => {
      const it = items.find((x) => x.id === Number(aid))
      const { shoeId, ...rest } = n
      return { activityRef: { garminId: it?.garminId ?? null, stravaId: it?.stravaId ?? null, startUtc: it ? new Date(it.startLocal).toISOString() : null }, ...rest, shoeRef: d.shoes.find((s) => s.id === shoeId)?.name ?? null }
    }),
  }
}

function backupResponse(d: UserData): Response {
  return json(backupOf(d), 200, { 'Content-Disposition': `attachment; filename=stride-backup-${todayISO()}.json` })
}

interface BackupFile {
  format?: string
  version?: number
  profile?: AthleteProfile
  goals?: Omit<Goal, 'id'>[]
  plan?: Omit<PlanEntry, 'id'>[]
  shoes?: Omit<Shoe, 'id'>[]
  checkins?: DailyCheckin[]
  activityNotes?: (Omit<StoredNotes, 'shoeId'> & { activityRef?: { garminId: number | null; stravaId: number | null; startUtc: string | null }; shoeRef?: string | null })[]
}

function restore(d: UserData, raw: BackupFile | null, mode: string | null): Response {
  if (mode !== 'merge' && mode !== 'replace') return err(400, 'BAD_REQUEST', 'Mode de restauration invalide (merge ou replace).')
  if (!raw || raw.format !== 'stride-backup' || raw.version !== 1) return err(400, 'BAD_REQUEST', 'Fichier invalide : sauvegarde Stride (format stride-backup v1) attendue.')
  if (mode === 'replace') {
    d.goals = []
    d.plan = []
    d.shoes = []
    d.notes = {}
    d.checkins = {}
  }
  const imported: RestoreResult['imported'] = { goals: 0, plan: 0, shoes: 0, notes: 0, checkins: 0, profile: 0 }
  if (raw.profile?.thresholds) {
    d.profile = raw.profile
    imported.profile = 1
  }
  for (const g of raw.goals ?? []) {
    if (mode === 'merge' && d.goals.some((x) => x.title === g.title)) continue
    d.goals.push({ ...g, id: nextId() })
    imported.goals++
  }
  for (const p of raw.plan ?? []) {
    if (mode === 'merge' && d.plan.some((x) => x.date === p.date && x.title === p.title)) continue
    d.plan.push({ ...p, id: nextId() })
    imported.plan++
  }
  for (const s of raw.shoes ?? []) {
    if (mode === 'merge' && d.shoes.some((x) => x.name === s.name)) continue
    d.shoes.push({ ...s, id: nextId() })
    imported.shoes++
  }
  for (const c of raw.checkins ?? []) {
    d.checkins[c.date] = c
    imported.checkins++
  }
  const items = getCatalog()
  for (const n of raw.activityNotes ?? []) {
    const ref = n.activityRef
    const it = items.find((x) => (ref?.garminId && x.garminId === ref.garminId) || (ref?.stravaId && x.stravaId === ref.stravaId) || (ref?.startUtc && new Date(x.startLocal).toISOString() === ref.startUtc))
    if (!it) continue
    const { activityRef: _r, shoeRef, ...rest } = n
    d.notes[String(it.id)] = { ...rest, shoeId: d.shoes.find((s) => s.name === shoeRef)?.id ?? null }
    imported.notes++
  }
  return json({ imported })
}

/* ---------------------------------- Validation helpers ---------------------------------- */

const isPain = (v: unknown) => v === null || (typeof v === 'number' && v >= 0 && v <= 10 && Math.round(v * 2) === v * 2)
const FOCUS: FocusArea[] = ['none', 'knee_pfps', 'achilles', 'shin_splints', 'other']
const GOAL_KINDS: GoalKind[] = ['macro', 'race', 'cadence', 'weekly_volume', 'other']
const PLAN_SPORTS: PlanSport[] = ['run', 'bike', 'strength', 'mobility', 'rest', 'other']
const SHOE_TYPES: ShoeType[] = ['soft', 'tempo', 'carbon']
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/

function meter(): MeterSnapshot {
  const now = Date.now()
  const s = db().sync
  const cool = s.cooldownUntil && new Date(s.cooldownUntil).getTime() > now ? s.cooldownUntil : null
  const ago = (ms: number) => new Date(now - ms).toISOString()
  return {
    providers: [
      { provider: 'garmin', requests1h: 42, requests24h: 486, errors1h: cool ? 4 : 1, rateLimited24h: cool ? 3 : 0, inFlight: s.running ? 3 : 0, cooldownUntil: cool, quota: null, avgLatencyMs: 412 },
      { provider: 'strava', requests1h: 14, requests24h: 171, errors1h: 0, rateLimited24h: 0, inFlight: s.running ? 1 : 0, cooldownUntil: null, quota: { limit15m: 100, usage15m: 23, limitDaily: 1000, usageDaily: 171, updatedAt: ago(40_000) }, avgLatencyMs: 296 },
      { provider: 'open-meteo', requests1h: 5, requests24h: 61, errors1h: 0, rateLimited24h: 0, inFlight: 0, cooldownUntil: null, quota: null, avgLatencyMs: 184 },
    ],
    recentErrors: [
      ...(cool ? [{ time: ago(120_000), provider: 'garmin', userId: 1, endpoint: '/activitylist-service/activities/search/activities', status: 429, message: 'Too Many Requests (Retry-After 720 s)' }] : []),
      ...(has('unauthorized') ? [{ time: ago(15 * 60_000), provider: 'garmin', userId: 1, endpoint: '/oauth-service/oauth/exchange', status: 401, message: 'Unauthorized: personal API key rejected' }] : []),
      { time: ago(47 * 60_000), provider: 'garmin', userId: 2, endpoint: '/wellness-service/wellness/dailySleepData/{date}', status: 503, message: 'Service Unavailable (succès après 2 tentatives)' },
      { time: ago(5 * 3600_000), provider: 'strava', userId: 1, endpoint: '/activities/{id}/streams', status: 404, message: 'Record Not Found' },
    ],
  }
}

/* ---------------------------------- Routes ---------------------------------- */

interface Ctx {
  url: URL
  method: string
  params: string[]
  init?: RequestInit
  me: MockUser
  data: UserData
}
type Handler = (c: Ctx) => Response | Promise<Response>
type Auth = 'public' | 'user' | 'admin'
const routes: { method: string; re: RegExp; auth: Auth; handler: Handler | ((c: Omit<Ctx, 'me' | 'data'>) => Response) }[] = []
const route = (method: string, re: RegExp, handler: Handler) => routes.push({ method, re, auth: re.source.startsWith('^\\/api\\/admin') ? 'admin' : 'user', handler })
const pub = (method: string, re: RegExp, handler: (c: Omit<Ctx, 'me' | 'data'>) => Response) => routes.push({ method, re, auth: 'public', handler })

/* --- platform: auth --- */

pub('GET', /^\/api\/auth\/state$/, () => {
  const s = db()
  const me = currentUser()
  return json({ authenticated: !!me, user: me ? toMe(me) : null, registrationMode: s.registrationMode, firstUser: s.users.length === 0, rpId: typeof window !== 'undefined' ? window.location.hostname : 'localhost' })
})

pub('POST', /^\/api\/auth\/register\/begin$/, ({ init }) => {
  const s = db()
  const b = parse<{ displayName?: string; inviteCode?: string }>(init)
  const name = b?.displayName?.trim() ?? ''
  if (name.length < 2 || name.length > 60) return err(400, 'BAD_REQUEST', 'Indique un nom d’affichage (2 à 60 caractères).')
  let invitationId: number | null = null
  if (s.users.length > 0 && s.registrationMode === 'invite') {
    const code = (b?.inviteCode ?? '').trim().toUpperCase()
    const inv = s.invitations.find((i) => i.code === code)
    const valid = inv && !inv.revoked && inv.uses < inv.maxUses && (!inv.expiresAt || new Date(inv.expiresAt).getTime() > Date.now())
    if (!valid) return err(403, 'INVITE_REQUIRED', code ? 'Code d’invitation invalide, expiré ou déjà utilisé.' : 'Inscription sur invitation uniquement : un code est requis.')
    invitationId = inv.id
  }
  s.pending = { displayName: name, invitationId }
  return json(fakeCreationOptions(name))
})

pub('POST', /^\/api\/auth\/register\/finish$/, () => {
  const s = db()
  if (!s.pending) return err(400, 'BAD_REQUEST', 'Cérémonie d’inscription expirée : recommence.')
  const id = s.users.reduce((m, u) => Math.max(m, u.id), 0) + 1
  const user: MockUser = {
    id,
    displayName: s.pending.displayName,
    role: s.users.length === 0 ? 'admin' : 'user',
    status: 'active',
    createdAt: isoNow(),
    lastLoginAt: isoNow(),
    onboardingCompleted: false,
    passkeys: [{ id: `pk_${rand(10, 'abcdefghijklmnopqrstuvwxyz0123456789')}`, name: deviceName(), createdAt: isoNow(), lastUsedAt: isoNow(), backedUp: true }],
  }
  s.users.push(user)
  s.data[String(id)] = emptyData()
  const inv = s.invitations.find((i) => i.id === s.pending?.invitationId)
  if (inv) inv.uses++
  s.pending = null
  s.sessionUserId = id
  return json(toMe(user))
})

pub('POST', /^\/api\/auth\/login\/begin$/, () =>
  json({ publicKey: { challenge: rand(43, 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_'), rpId: typeof window !== 'undefined' ? window.location.hostname : 'localhost', allowCredentials: [], userVerification: 'preferred', timeout: 60000 } }),
)

pub('POST', /^\/api\/auth\/login\/finish$/, () => {
  const s = db()
  const user = has('user') ? s.users.find((u) => u.role === 'user' && u.status === 'active') : s.users.find((u) => u.role === 'admin')
  if (!user) return err(401, 'UNAUTHORIZED', 'Aucun compte Stride ne correspond à cette passkey.')
  if (user.status === 'disabled') return err(403, 'FORBIDDEN', 'Ce compte est désactivé. Contacte l’administrateur.')
  user.lastLoginAt = isoNow()
  if (user.passkeys[0]) user.passkeys[0].lastUsedAt = isoNow()
  s.sessionUserId = user.id
  return json(toMe(user))
})

pub('POST', /^\/api\/auth\/logout$/, () => {
  db().sessionUserId = null
  return noContent()
})

/* --- platform: me & passkeys --- */

route('GET', /^\/api\/me$/, ({ me }) => json(toMe(me)))
route('PATCH', /^\/api\/me$/, ({ me, init }) => {
  const b = parse<{ displayName?: string; onboardingCompleted?: boolean }>(init) ?? {}
  if (b.displayName !== undefined) {
    const name = b.displayName.trim()
    if (name.length < 2 || name.length > 60) return err(400, 'BAD_REQUEST', 'Le nom doit faire entre 2 et 60 caractères.')
    me.displayName = name
  }
  if (typeof b.onboardingCompleted === 'boolean') me.onboardingCompleted = b.onboardingCompleted
  return json(toMe(me))
})
route('DELETE', /^\/api\/me$/, ({ me }) => {
  const s = db()
  if (me.role === 'admin' && s.users.filter((u) => u.role === 'admin' && u.status === 'active').length === 1 && s.users.length > 1)
    return err(409, 'CONFLICT', 'Tu es le dernier administrateur : promeus un autre compte avant de supprimer le tien.')
  s.users = s.users.filter((u) => u.id !== me.id)
  delete s.data[String(me.id)]
  s.sessionUserId = null
  return noContent()
})
route('GET', /^\/api\/me\/passkeys$/, ({ me }) => json(me.passkeys))
route('POST', /^\/api\/me\/passkeys\/begin$/, ({ me }) => json(fakeCreationOptions(me.displayName)))
route('POST', /^\/api\/me\/passkeys\/finish$/, ({ me }) => {
  const used = new Set(me.passkeys.map((p) => p.name))
  let name = deviceName()
  for (let i = 2; used.has(name); i++) name = `${deviceName()} ${i}`
  const pk: Passkey = { id: `pk_${rand(10, 'abcdefghijklmnopqrstuvwxyz0123456789')}`, name, createdAt: isoNow(), lastUsedAt: null, backedUp: true }
  me.passkeys.push(pk)
  return json(pk)
})
route('PATCH', /^\/api\/me\/passkeys\/([^/]+)$/, ({ me, params, init }) => {
  const pk = me.passkeys.find((p) => p.id === decodeURIComponent(params[0]))
  if (!pk) return err(404, 'NOT_FOUND', 'Passkey introuvable.')
  const name = parse<{ name?: string }>(init)?.name?.trim() ?? ''
  if (!name || name.length > 40) return err(400, 'BAD_REQUEST', 'Nom de passkey invalide (1 à 40 caractères).')
  pk.name = name
  return json(pk)
})
route('DELETE', /^\/api\/me\/passkeys\/([^/]+)$/, ({ me, params }) => {
  const id = decodeURIComponent(params[0])
  if (!me.passkeys.some((p) => p.id === id)) return err(404, 'NOT_FOUND', 'Passkey introuvable.')
  if (me.passkeys.length <= 1) return err(409, 'CONFLICT', 'Impossible de supprimer ta dernière passkey : ajoutes-en une autre d’abord.')
  me.passkeys = me.passkeys.filter((p) => p.id !== id)
  return noContent()
})

/* --- platform: admin --- */

route('GET', /^\/api\/admin\/users$/, () => json(db().users.map(toAdmin)))
route('PATCH', /^\/api\/admin\/users\/(\d+)$/, ({ me, params, init }) => {
  const u = db().users.find((x) => x.id === Number(params[0]))
  if (!u) return err(404, 'NOT_FOUND', 'Utilisateur introuvable.')
  if (u.id === me.id) return err(409, 'CONFLICT', 'Tu ne peux pas modifier ton propre compte.')
  const b = parse<{ status?: string; role?: string }>(init) ?? {}
  if (b.status !== undefined) {
    if (b.status !== 'active' && b.status !== 'disabled') return err(400, 'BAD_REQUEST', 'Statut invalide.')
    u.status = b.status
  }
  if (b.role !== undefined) {
    if (b.role !== 'user' && b.role !== 'admin') return err(400, 'BAD_REQUEST', 'Rôle invalide.')
    u.role = b.role
  }
  return json(toAdmin(u))
})
route('DELETE', /^\/api\/admin\/users\/(\d+)$/, ({ me, params }) => {
  const s = db()
  const id = Number(params[0])
  if (id === me.id) return err(409, 'CONFLICT', 'Tu ne peux pas supprimer ton propre compte ici.')
  if (!s.users.some((u) => u.id === id)) return err(404, 'NOT_FOUND', 'Utilisateur introuvable.')
  s.users = s.users.filter((u) => u.id !== id)
  delete s.data[String(id)]
  return noContent()
})
route('GET', /^\/api\/admin\/invitations$/, () => json(db().invitations.map(({ code: _c, ...i }) => i)))
route('POST', /^\/api\/admin\/invitations$/, ({ init }) => {
  const b = parse<{ label?: string; maxUses?: number; expiresInHours?: number }>(init) ?? {}
  const maxUses = b.maxUses ?? 1
  const hours = b.expiresInHours ?? 168
  if (!Number.isInteger(maxUses) || maxUses < 1 || maxUses > 100) return err(400, 'BAD_REQUEST', 'Nombre d’utilisations invalide (1 à 100).')
  if (!Number.isFinite(hours) || hours < 1 || hours > 24 * 90) return err(400, 'BAD_REQUEST', 'Durée de validité invalide (1 h à 90 jours).')
  const code = `${rand(4)}-${rand(4)}-${rand(4)}`
  const inv = { id: nextId(), label: (b.label ?? '').trim(), codePrefix: code.slice(0, 4), code, createdAt: isoNow(), expiresAt: new Date(Date.now() + hours * 3600_000).toISOString(), maxUses, uses: 0, revoked: false }
  db().invitations.unshift(inv)
  const { code: _c, ...invitation } = inv
  return json({ invitation, code, link: `${origin()}/register?invite=${code}` }, 201)
})
route('DELETE', /^\/api\/admin\/invitations\/(\d+)$/, ({ params }) => {
  const inv = db().invitations.find((i) => i.id === Number(params[0]))
  if (!inv) return err(404, 'NOT_FOUND', 'Invitation introuvable.')
  inv.revoked = true
  return noContent()
})
route('GET', /^\/api\/admin\/settings$/, () => json({ registrationMode: db().registrationMode }))
route('PUT', /^\/api\/admin\/settings$/, ({ init }) => {
  const mode = parse<{ registrationMode?: string }>(init)?.registrationMode
  if (mode !== 'open' && mode !== 'invite') return err(400, 'BAD_REQUEST', 'Mode d’inscription invalide.')
  db().registrationMode = mode
  return json({ registrationMode: mode })
})
route('GET', /^\/api\/admin\/meter$/, () => json(meter()))
route('GET', /^\/api\/admin\/logs$/, ({ url }) => {
  const q = url.searchParams
  const users = db().users
  const all: LogEntry[] = []
  const reqs = ['GET /api/activities', 'GET /api/activities/12', 'POST /api/sync', 'GET /api/days/today', 'PUT /api/profile', 'GET /api/plan/week']
  const acts = ['auth.login', 'sync.start', 'prompt.generate', 'prompt.copy', 'plan.create', 'settings.llm', 'llm.analyze']
  for (let i = 0; i < 260; i++) {
    const u = users[i % Math.max(1, users.length)]
    const at = new Date(Date.now() - i * 97_000).toISOString()
    const id = 1000 - i
    if (i % 5 === 0) {
      all.push({ id, at, category: 'user_action', level: i % 35 === 0 ? 'error' : 'info', userId: u?.id ?? null, userName: u?.displayName ?? null, method: 'POST', target: '/api/sync', provider: '', status: i % 35 === 0 ? 502 : 200, durationMs: 0, action: acts[i % acts.length], message: '' })
    } else if (i % 7 === 0) {
      all.push({ id, at, category: 'request', level: i % 21 === 0 ? 'error' : 'info', userId: u?.id ?? null, userName: u?.displayName ?? null, method: 'OUT', target: '/activity-service/activity', provider: i % 2 ? 'garmin' : 'llm', status: i % 21 === 0 ? 429 : 200, durationMs: 180 + (i % 9) * 40, action: '', message: i % 21 === 0 ? 'limite de débit atteinte' : '' })
    } else {
      const [method, target] = reqs[i % reqs.length].split(' ')
      const status = i % 23 === 0 ? 404 : 200
      all.push({ id, at, category: 'request', level: status >= 400 ? 'error' : 'info', userId: u?.id ?? null, userName: u?.displayName ?? null, method, target, provider: '', status, durationMs: 4 + (i % 13) * 3, action: '', message: '' })
    }
  }
  const cat = q.get('category'), lvl = q.get('level'), uid = Number(q.get('userId') || 0), before = Number(q.get('before') || 0)
  const rows = all.filter((e) => (!cat || cat === 'all' || e.category === cat) && (!lvl || lvl === 'all' || e.level === lvl) && (!uid || e.userId === uid) && (!before || e.id < before))
  const page = rows.slice(0, 100)
  return json({ items: page, nextBefore: rows.length > 100 ? page[page.length - 1].id : null, dropped: 0 })
})
route('GET', /^\/api\/version$/, () => json({ version: '3.0.0', commit: 'demo123', builtAt: '2026-11-05T20:00:00Z', go: 'go1.27.1', label: '3.0.0 · demo123 · 2026-11-05T20:00:00Z' }))
route('POST', /^\/api\/actions$/, () => new Response(null, { status: 204 }))
route('POST', /^\/api\/admin\/cache\/flush$/, () => json({ removed: 37 + Math.floor(Math.random() * 120) }))
route('GET', /^\/api\/admin\/connections$/, () => {
  const out: AdminConnection[] = []
  for (const u of db().users) {
    const i = userData(u.id).integrations
    out.push({ userId: u.id, provider: 'garmin', status: i.garmin.status, lastSyncAt: i.garmin.lastSyncAt, lastError: i.garmin.lastError, lastErrorAt: i.garmin.lastError ? new Date(Date.now() - 15 * 60_000).toISOString() : null })
    out.push({ userId: u.id, provider: 'strava', status: i.strava.status, lastSyncAt: i.strava.lastSyncAt, lastError: i.strava.lastError, lastErrorAt: null })
  }
  return json(out)
})
route('POST', /^\/api\/admin\/users\/(\d+)\/resync$/, ({ params }) => {
  if (!db().users.some((u) => u.id === Number(params[0]))) return err(404, 'NOT_FOUND', 'Utilisateur introuvable.')
  const s = db().sync
  if (!s.running) {
    s.running = true
    s.startedAt = Date.now()
    s.runs++
  }
  return noContent(202)
})
route('GET', /^\/api\/admin\/users\/(\d+)\/backup$/, ({ params }) => {
  if (!db().users.some((u) => u.id === Number(params[0]))) return err(404, 'NOT_FOUND', 'Utilisateur introuvable.')
  return backupResponse(userData(Number(params[0])))
})
route('POST', /^\/api\/admin\/users\/(\d+)\/restore$/, ({ params, init, url }) => {
  if (!db().users.some((u) => u.id === Number(params[0]))) return err(404, 'NOT_FOUND', 'Utilisateur introuvable.')
  return restore(userData(Number(params[0])), parse<BackupFile>(init), url.searchParams.get('mode'))
})

/* --- domain: integrations & sync --- */

route('GET', /^\/api\/integrations$/, ({ data }) => json(data.integrations))
route('PUT', /^\/api\/integrations\/garmin$/, ({ data, init }) => {
  const key = parse<{ apiKey?: string }>(init)?.apiKey?.trim() ?? ''
  if (key.length < 12 || /invalid|refus/i.test(key)) return err(400, 'PROVIDER_UNAUTHORIZED', 'Clé refusée par Garmin : vérifie qu’elle est active dans Garmin Connect (Personal API Key).')
  const g = data.integrations.garmin
  Object.assign(g, { connected: true, status: 'ok', keyHint: `••••${key.slice(-4)}`, tokenExpiresAt: new Date(Date.now() + 24 * 3600_000).toISOString(), lastSyncAt: isoNow(), lastError: null })
  const s = db().sync
  s.runs++
  s.liveUntil = Date.now() + 60_000
  s.lastLiveAt = isoNow()
  return json(data.integrations)
})
route('POST', /^\/api\/integrations\/garmin\/test$/, ({ data }) => {
  const g = data.integrations.garmin
  if (!g.connected) return err(400, 'PROVIDER_NOT_CONNECTED', 'Garmin n’est pas connecté.')
  if (g.status === 'error') return err(400, 'PROVIDER_UNAUTHORIZED', g.lastError ?? 'Clé refusée par Garmin.')
  g.tokenExpiresAt = new Date(Date.now() + 24 * 3600_000).toISOString()
  return json(data.integrations)
})
route('DELETE', /^\/api\/integrations\/garmin$/, ({ data }) => {
  Object.assign(data.integrations.garmin, { connected: false, status: 'disconnected', keyHint: null, tokenExpiresAt: null, lastError: null })
  return json(data.integrations)
})
route('GET', /^\/api\/integrations\/strava\/connect$/, ({ data, url }) => {
  const raw = url.searchParams.get('return') ?? '/settings'
  const ret = raw.startsWith('/') && !raw.startsWith('//') ? raw : '/settings'
  const sep = ret.includes('?') ? '&' : '?'
  const st = data.integrations.strava
  if (!st.available) return new Response(null, { status: 302, headers: { Location: `${ret}${sep}strava=error&reason=unavailable` } })
  if (has('stravaerr')) return new Response(null, { status: 302, headers: { Location: `${ret}${sep}strava=error&reason=access_denied` } })
  Object.assign(st, { connected: true, status: 'ok', athleteName: 'Alex D.', lastSyncAt: isoNow(), lastError: null })
  return new Response(null, { status: 302, headers: { Location: `${ret}${sep}strava=connected` } })
})
route('DELETE', /^\/api\/integrations\/strava$/, ({ data }) => {
  Object.assign(data.integrations.strava, { connected: false, status: 'disconnected', athleteName: null, lastError: null })
  return json(data.integrations)
})
pub('GET', /^\/api\/webhooks\/strava$/, ({ url }) => json({ 'hub.challenge': url.searchParams.get('hub.challenge') ?? '' }))
pub('POST', /^\/api\/webhooks\/strava$/, () => json({ ok: true }))

route('POST', /^\/api\/sync$/, ({ data }) => {
  const s = db().sync
  const i = data.integrations
  const cooling = s.cooldownUntil && new Date(s.cooldownUntil).getTime() > Date.now()
  if (!s.running && !cooling && (i.garmin.connected || i.strava.connected)) {
    s.running = true
    s.startedAt = Date.now()
    s.runs++
  }
  return json(syncStatus(data))
})
route('GET', /^\/api\/sync\/status$/, ({ data }) => json(syncStatus(data)))

/* --- domain: activities & days --- */

route('GET', /^\/api\/activities$/, ({ data, url }) => {
  const today = todayISO()
  const all = visibleItems(data, today)
  const q = url.searchParams
  const limit = Math.max(1, Math.min(200, Number(q.get('limit') ?? 30) || 30))
  let items = all
  const before = q.get('before')
  const from = q.get('from')
  const to = q.get('to')
  const sport = q.get('sport')
  if (before) items = items.filter((it) => it.startLocal < before)
  if (from) items = items.filter((it) => it.date >= from)
  if (to) items = items.filter((it) => it.date <= to)
  if (sport) items = items.filter((it) => it.sport === sport)
  const planned = plannedWithMatch(data, all, today)
  const freshness = db().sync.liveUntil > Date.now() ? { source: 'live', updatedAt: db().sync.lastLiveAt } : { source: has('fresh-db') ? 'db' : has('fresh-live') ? 'live' : 'cache', updatedAt: new Date(Date.now() - 90_000).toISOString() }
  return json({ items: items.slice(0, limit).map((it) => summarize(it, data, planned)), hasMore: items.length > limit, freshness })
})

const findVisible = (data: UserData, id: string) => visibleItems(data).find((it) => it.id === Number(id)) ?? null

route('GET', /^\/api\/activities\/(\d+)$/, ({ data, params }) => {
  const it = findVisible(data, params[0])
  return it ? json(enrichActivityView(buildActivityView(it, data), it, data)) : err(404, 'NOT_FOUND', 'Activité introuvable.')
})
route('GET', /^\/api\/activities\/(\d+)\/report$/, ({ data, params, url }) => {
  const it = findVisible(data, params[0])
  if (!it) return err(404, 'NOT_FOUND', 'Activité introuvable.')
  return markdown(activityReport(buildActivityView(it, data), data, parseIncludeParam(url.searchParams.get('include'))))
})
route('PUT', /^\/api\/activities\/(\d+)\/notes$/, ({ data, params, init }) => {
  const it = findVisible(data, params[0])
  if (!it) return err(404, 'NOT_FOUND', 'Activité introuvable.')
  const b = parse<Partial<StoredNotes>>(init)
  if (!b) return err(400, 'BAD_REQUEST', 'Corps de requête invalide.')
  for (const k of ['painDuring', 'painCold', 'painNextMorning'] as const) if (k in b && !isPain(b[k])) return err(400, 'BAD_REQUEST', 'Douleur : valeur entre 0 et 10 par pas de 0,5.')
  if ('rpe' in b && b.rpe !== null && !(Number.isInteger(b.rpe) && (b.rpe as number) >= 1 && (b.rpe as number) <= 10)) return err(400, 'BAD_REQUEST', 'RPE : entier entre 1 et 10.')
  if ('shoeId' in b && b.shoeId !== null && !data.shoes.some((s) => s.id === b.shoeId)) return err(400, 'BAD_REQUEST', 'Chaussure inconnue.')
  if ('tags' in b && (!Array.isArray(b.tags) || b.tags.some((t) => !CONTEXT_TAGS.some((c) => c.key === t)))) return err(400, 'BAD_REQUEST', 'Tag de contexte inconnu.')
  if ('note' in b && (typeof b.note !== 'string' || b.note.length > 2000)) return err(400, 'BAD_REQUEST', 'Note trop longue (2000 caractères max).')
  const cur = data.notes[String(it.id)] ?? { painDuring: null, painCold: null, painNextMorning: null, rpe: null, shoeId: null, tags: [], note: '', updatedAt: null }
  const pick = <K extends keyof StoredNotes>(k: K) => (k in b ? (b[k] as StoredNotes[K]) : cur[k])
  data.notes[String(it.id)] = {
    painDuring: pick('painDuring'),
    painCold: pick('painCold'),
    painNextMorning: pick('painNextMorning'),
    rpe: pick('rpe'),
    shoeId: pick('shoeId'),
    tags: pick('tags'),
    note: pick('note'),
    updatedAt: isoNow(),
  }
  return json(notesOf(data, it.id))
})

const dayParam = (raw: string) => (raw === 'today' ? todayISO() : raw)
route('GET', /^\/api\/days\/(today|\d{4}-\d{2}-\d{2})$/, ({ data, params }) => json(enrichDayView(buildDayView(dayParam(params[0]), data), data)))
route('GET', /^\/api\/days\/(today|\d{4}-\d{2}-\d{2})\/report$/, ({ data, params, url }) => markdown(dayReport(buildDayView(dayParam(params[0]), data), data, parseIncludeParam(url.searchParams.get('include')))))
route('PUT', /^\/api\/days\/(today|\d{4}-\d{2}-\d{2})\/checkin$/, ({ data, params, init }) => {
  const date = dayParam(params[0])
  const b = parse<{ trainingReadiness?: number | null; readinessStatus?: string | null; note?: string }>(init) ?? {}
  const tr = b.trainingReadiness
  if (tr !== undefined && tr !== null && !(Number.isInteger(tr) && tr >= 0 && tr <= 100)) return err(400, 'BAD_REQUEST', 'Training Readiness : entier entre 0 et 100.')
  const cur = data.checkins[date] ?? { date, trainingReadiness: null, readinessStatus: null, note: '', updatedAt: isoNow() }
  const next: DailyCheckin = {
    date,
    trainingReadiness: tr === undefined ? cur.trainingReadiness : tr,
    readinessStatus: b.readinessStatus === undefined ? cur.readinessStatus : b.readinessStatus,
    note: b.note ?? cur.note,
    updatedAt: isoNow(),
  }
  data.checkins[date] = next
  return json(next)
})

/* --- domain: profile, goals, plan, shoes --- */

route('GET', /^\/api\/profile$/, ({ data }) => json(data.profile))
route('PUT', /^\/api\/profile$/, ({ data, init }) => {
  const b = parse<AthleteProfile>(init)
  if (!b || !FOCUS.includes(b.focusArea) || !b.thresholds) return err(400, 'BAD_REQUEST', 'Profil invalide.')
  if (b.injuredSide !== null && !['left', 'right', 'both'].includes(b.injuredSide)) return err(400, 'BAD_REQUEST', 'Côté blessé invalide.')
  const bad = Object.values(b.thresholds).some((v) => typeof v !== 'number' || !Number.isFinite(v) || v < 0)
  if (bad) return err(400, 'BAD_REQUEST', 'Seuils invalides : nombres positifs attendus.')
  if ((b.paceTargets ?? []).some((p) => !p.label?.trim() || p.minSecPerKm <= 0 || p.maxSecPerKm < p.minSecPerKm)) return err(400, 'BAD_REQUEST', 'Allures cibles invalides (min ≤ max).')
  const label = b.focusArea === 'other' ? b.focusLabel?.trim() || 'Autre' : presetFor(b.focusArea).label
  data.profile = { ...b, focusLabel: label, paceTargets: b.paceTargets ?? [], timezone: b.timezone || 'Europe/Paris' }
  return json(data.profile)
})
route('GET', /^\/api\/profile\/presets$/, () => json(PRESETS))

route('GET', /^\/api\/goals$/, ({ data }) => json(data.goals))
route('POST', /^\/api\/goals$/, ({ data, init }) => {
  const b = parse<Omit<Goal, 'id'>>(init)
  if (!b?.title?.trim() || !GOAL_KINDS.includes(b.kind)) return err(400, 'BAD_REQUEST', 'Objectif invalide : titre et type requis.')
  const g: Goal = { title: b.title.trim(), kind: b.kind, targetValue: b.targetValue ?? null, targetUnit: b.targetUnit ?? null, targetDate: b.targetDate ?? null, status: b.status ?? 'active', note: b.note ?? '', id: nextId() }
  data.goals.push(g)
  return json(g, 201)
})
route('PATCH', /^\/api\/goals\/(\d+)$/, ({ data, params, init }) => {
  const g = data.goals.find((x) => x.id === Number(params[0]))
  if (!g) return err(404, 'NOT_FOUND', 'Objectif introuvable.')
  const b = parse<Partial<Goal>>(init) ?? {}
  if (b.kind !== undefined && !GOAL_KINDS.includes(b.kind)) return err(400, 'BAD_REQUEST', 'Type d’objectif invalide.')
  if (b.title !== undefined && !b.title.trim()) return err(400, 'BAD_REQUEST', 'Le titre est requis.')
  Object.assign(g, { ...b, id: g.id })
  return json(g)
})
route('DELETE', /^\/api\/goals\/(\d+)$/, ({ data, params }) => {
  const before = data.goals.length
  data.goals = data.goals.filter((x) => x.id !== Number(params[0]))
  return before === data.goals.length ? err(404, 'NOT_FOUND', 'Objectif introuvable.') : noContent()
})

const withMatch = (data: UserData, id: number) => plannedWithMatch(data, visibleItems(data), todayISO()).find((p) => p.id === id)
function validPlan(b: Partial<PlanEntry>, partial: boolean): string | null {
  if ((!partial || b.date !== undefined) && !DATE_RE.test(b.date ?? '')) return 'Date invalide (AAAA-MM-JJ).'
  if ((!partial || b.sport !== undefined) && !PLAN_SPORTS.includes(b.sport as PlanSport)) return 'Sport invalide.'
  if ((!partial || b.title !== undefined) && !b.title?.trim()) return 'Le titre est requis.'
  if (b.targetPaceMinSecPerKm && b.targetPaceMaxSecPerKm && b.targetPaceMinSecPerKm > b.targetPaceMaxSecPerKm) return 'Allure cible : le minimum doit être plus rapide que le maximum.'
  return null
}
route('GET', /^\/api\/plan$/, ({ data, url }) => json(plannedWithMatch(data, visibleItems(data), todayISO(), url.searchParams.get('from') ?? undefined, url.searchParams.get('to') ?? undefined)))
route('POST', /^\/api\/plan$/, ({ data, init }) => {
  const b = parse<Omit<PlanEntry, 'id'>>(init)
  const e = b ? validPlan(b, false) : 'Corps invalide.'
  if (e || !b) return err(400, 'BAD_REQUEST', e ?? 'Corps invalide.')
  const p: PlanEntry = {
    id: nextId(),
    date: b.date,
    sport: b.sport,
    title: b.title.trim(),
    targetDurationS: b.targetDurationS ?? null,
    targetDistanceM: b.targetDistanceM ?? null,
    targetPaceMinSecPerKm: b.targetPaceMinSecPerKm ?? null,
    targetPaceMaxSecPerKm: b.targetPaceMaxSecPerKm ?? null,
    targetCadenceSpm: b.targetCadenceSpm ?? null,
    instructions: b.instructions ?? '',
    steps: b.steps ?? [],
    source: b.source ?? 'manual',
    garmin: { status: 'none', workoutId: null, pushedAt: null, error: null },
  }
  data.plan.push(p)
  return json(withMatch(data, p.id), 201)
})
route('PATCH', /^\/api\/plan\/(\d+)$/, ({ data, params, init }) => {
  const p = data.plan.find((x) => x.id === Number(params[0]))
  if (!p) return err(404, 'NOT_FOUND', 'Séance introuvable.')
  const b = parse<Partial<PlanEntry>>(init) ?? {}
  const e = validPlan({ ...p, ...b }, true)
  if (e) return err(400, 'BAD_REQUEST', e)
  Object.assign(p, { ...b, id: p.id, garmin: p.garmin })
  return json(withMatch(data, p.id))
})
route('DELETE', /^\/api\/plan\/(\d+)$/, ({ data, params }) => {
  const before = data.plan.length
  data.plan = data.plan.filter((x) => x.id !== Number(params[0]))
  return before === data.plan.length ? err(404, 'NOT_FOUND', 'Séance introuvable.') : noContent()
})
route('GET', /^\/api\/plan\/week$/, ({ data, url }) => {
  const today = todayISO()
  const date = url.searchParams.get('date') || today
  if (!DATE_RE.test(date)) return err(400, 'BAD_REQUEST', 'Date invalide.')
  const items = visibleItems(data, today)
  return json(weekPlan(data, items, date, today, plannedWithMatch(data, items, today)))
})

function validShoe(b: Partial<Shoe>, partial: boolean): string | null {
  if ((!partial || b.name !== undefined) && !b.name?.trim()) return 'Le nom est requis.'
  if ((!partial || b.type !== undefined) && !SHOE_TYPES.includes(b.type as ShoeType)) return 'Type de chaussure invalide.'
  if (b.dropMm !== undefined && b.dropMm !== null && (b.dropMm < 0 || b.dropMm > 20)) return 'Drop invalide (0 à 20 mm).'
  return null
}
route('GET', /^\/api\/shoes$/, ({ data }) => json(data.shoes))
route('POST', /^\/api\/shoes$/, ({ data, init }) => {
  const b = parse<Partial<Shoe>>(init) ?? {}
  const e = validShoe(b, false)
  if (e) return err(400, 'BAD_REQUEST', e)
  const s: Shoe = { id: nextId(), name: b.name!.trim(), brand: b.brand ?? '', model: b.model ?? '', dropMm: b.dropMm ?? null, type: b.type as ShoeType, stravaGearId: b.stravaGearId ?? null, retired: !!b.retired, distanceKm: 0, note: b.note ?? '' }
  data.shoes.push(s)
  return json(s, 201)
})
route('PATCH', /^\/api\/shoes\/(\d+)$/, ({ data, params, init }) => {
  const s = data.shoes.find((x) => x.id === Number(params[0]))
  if (!s) return err(404, 'NOT_FOUND', 'Chaussure introuvable.')
  const b = parse<Partial<Shoe>>(init) ?? {}
  const e = validShoe(b, true)
  if (e) return err(400, 'BAD_REQUEST', e)
  Object.assign(s, { ...b, id: s.id, distanceKm: s.distanceKm })
  return json(s)
})
route('DELETE', /^\/api\/shoes\/(\d+)$/, ({ data, params }) => {
  const id = Number(params[0])
  const before = data.shoes.length
  data.shoes = data.shoes.filter((x) => x.id !== id)
  for (const n of Object.values(data.notes)) if (n.shoeId === id) n.shoeId = null
  return before === data.shoes.length ? err(404, 'NOT_FOUND', 'Chaussure introuvable.') : noContent()
})
route('POST', /^\/api\/shoes\/import-strava$/, ({ data }) => {
  if (!data.integrations.strava.connected) return err(400, 'PROVIDER_NOT_CONNECTED', 'Connecte Strava pour importer tes chaussures.')
  const gear = [
    { id: 'g2741001', brand: 'Vela', model: 'Cloudline 3', km: 418.7 },
    { id: 'g2741002', brand: 'Kairo', model: 'Swift 2', km: 191.4 },
    { id: 'g2741003', brand: 'Orra', model: 'Trail XT', km: 96.3 },
  ]
  for (const g of gear) {
    const existing = data.shoes.find((s) => s.stravaGearId === g.id) ?? data.shoes.find((s) => !s.stravaGearId && s.model === g.model)
    if (existing) Object.assign(existing, { stravaGearId: g.id, distanceKm: g.km })
    else data.shoes.push({ id: nextId(), name: `${g.brand} ${g.model}`, brand: g.brand, model: g.model, dropMm: null, type: 'soft', stravaGearId: g.id, retired: false, distanceKm: g.km, note: 'Importée depuis Strava' })
  }
  return json(data.shoes)
})

/* --- domain: exports & backup --- */

route('POST', /^\/api\/exports\/multi$/, ({ data, init }) => {
  const b = parse<{ activityIds?: number[]; weeks?: number; include?: string[] }>(init)
  if (!b || (!b.activityIds?.length && !b.weeks)) return err(400, 'BAD_REQUEST', 'Sélectionne des séances ou un nombre de semaines.')
  if (b.weeks !== undefined && (!Number.isInteger(b.weeks) || b.weeks < 1 || b.weeks > 52)) return err(400, 'BAD_REQUEST', 'Nombre de semaines invalide (1 à 52).')
  const include = parseIncludeParam(b.include ? b.include.join(',') || 'none' : null)
  const r = multiReport(data, { activityIds: b.activityIds, weeks: b.weeks, include })
  return json({ markdown: r.markdown, tokensEstimate: Math.ceil(r.markdown.length / 4), sessions: r.sessions })
})
route('GET', /^\/api\/backup$/, ({ data }) => backupResponse(data))
route('POST', /^\/api\/backup\/restore$/, ({ data, init, url }) => restore(data, parse<BackupFile>(init), url.searchParams.get('mode')))

registerV3(route, (data, id) => {
  const it = findVisible(data, String(id))
  return it ? { it, v: enrichActivityView(buildActivityView(it, data), it, data) } : null
})

/* ---------------------------------- Entry point ---------------------------------- */

function latency(path: string): number {
  const heavy = /^\/api\/(activities\/\d+(\/report)?|days\/[^/]+(\/report)?|exports\/multi)$/.test(path)
  return heavy ? 380 + Math.random() * 420 : 110 + Math.random() * 190
}

export async function mockFetch(path: string, init?: RequestInit): Promise<Response> {
  const url = new URL(path, origin())
  const method = (init?.method ?? 'GET').toUpperCase()
  await sleep(latency(url.pathname) * (has('slow') ? 4 : 1))
  tickSync()
  if (!SAFE.has(method) && !url.pathname.startsWith('/api/webhooks/') && new Headers(init?.headers).get('X-Stride-CSRF') !== '1') {
    return err(403, 'FORBIDDEN', 'Requête refusée : en-tête anti-CSRF manquant.')
  }
  for (const r of routes) {
    if (r.method !== method) continue
    const m = r.re.exec(url.pathname)
    if (!m) continue
    let res: Response
    if (r.auth === 'public') {
      res = (r.handler as (c: Omit<Ctx, 'me' | 'data'>) => Response)({ url, method, params: m.slice(1), init })
    } else {
      const me = currentUser()
      if (!me) return err(401, 'UNAUTHORIZED', 'Session expirée : reconnecte-toi.')
      if (r.auth === 'admin' && me.role !== 'admin') return err(403, 'FORBIDDEN', 'Réservé aux administrateurs.')
      res = await (r.handler as Handler)({ url, method, params: m.slice(1), init, me, data: userData(me.id) })
    }
    save()
    return res
  }
  return err(404, 'NOT_FOUND', `Route mock inconnue : ${method} ${url.pathname}`)
}


