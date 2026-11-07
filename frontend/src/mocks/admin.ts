// 4.1 admin control center mock endpoints (§14.6): overview, cross-user jobs and share links, sessions,
// on-demand sync and database maintenance. Flags: `dbcorrupt` (integrity check reports problems),
// `adminbusy` (more running / failed jobs).

import type { AdminJob, AdminOverview, AdminShareLink, AdminUser, ErrorCode, FeedbackItem, FeedbackStatus, JobGroup, JobGroupStats, JobState, JobStatus, MeterSnapshot, ShareLink } from '../api/types'

type JobKind = JobStatus['kind']
import { db, userData, type MockUser } from './db'
import { visibleItems } from './domain'
import { has } from './flags'
import { llmConfigured } from './v3'
import { v4Of } from './v4'

type Ctx = { url: URL; params: string[]; init?: RequestInit; me: MockUser }
type RouteFn = (method: string, re: RegExp, handler: (c: Ctx) => Response | Promise<Response>) => void

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const err = (status: number, code: ErrorCode, error: string) => json({ error, code }, status)
const ago = (ms: number) => new Date(Date.now() - ms).toISOString()
const MIN = 60_000
const H = 60 * MIN

const startedAt = ago(3 * 24 * H + 5 * H + 12 * MIN)
const REPLAYABLE: JobKind[] = ['sync.provider', 'sync.resync', 'sync.history', 'sync.strava_webhook']
const WORKERS: Record<JobGroup, number> = { sync: 2, llm: 2, webhook: 4 }
const groupOf = (k: JobKind): JobGroup => (k.startsWith('llm.') ? 'llm' : k.startsWith('webhook.') ? 'webhook' : 'sync')

/* ---------------------------------- state (memory only) ---------------------------------- */

interface AdminMockState {
  jobs: (Omit<AdminJob, 'userName' | 'retryable' | 'cancellable'>)[]
  sessions: Record<number, number>
  seq: number
}
let state: AdminMockState | null = null

function job(p: Partial<AdminMockState['jobs'][number]> & Pick<AdminMockState['jobs'][number], 'id' | 'userId' | 'kind' | 'state' | 'label'>): AdminMockState['jobs'][number] {
  return {
    queue: p.kind.startsWith('llm.') ? 'critical' : p.kind === 'sync.history' ? 'low' : 'default',
    progressPct: null,
    processed: null,
    total: null,
    attempts: 1,
    maxAttempts: 1,
    error: null,
    result: null,
    createdAt: ago(10 * MIN),
    updatedAt: ago(5 * MIN),
    ...p,
  }
}

function st(): AdminMockState {
  if (state) return state
  const busy = has('adminbusy')
  state = {
    seq: 1,
    sessions: Object.fromEntries(db().users.map((u) => [u.id, u.status === 'active' ? u.passkeys.length : 0])),
    jobs: [
      job({ id: 'job_adm_hist', userId: 2, kind: 'sync.history', state: 'running', label: 'Import de l’historique complet', progressPct: 38, processed: 6, total: 16, maxAttempts: 5, createdAt: ago(4 * MIN), updatedAt: ago(10_000) }),
      job({ id: 'job_adm_llmq', userId: 1, kind: 'llm.analyze', state: 'queued', label: 'Analyse coach IA (activity)', createdAt: ago(20_000), updatedAt: ago(20_000) }),
      job({ id: 'job_adm_syncfail', userId: 2, kind: 'sync.provider', state: 'failed', label: 'Synchronisation des activités', error: 'Garmin indisponible (503) : réessaie dans quelques minutes.', createdAt: ago(3 * H), updatedAt: ago(3 * H) }),
      job({ id: 'job_adm_llmfail', userId: 1, kind: 'llm.analyze', state: 'failed', label: 'Analyse coach IA (day)', error: 'Le fournisseur IA a refusé la clé : vérifie-la dans Réglages › IA.', createdAt: ago(7 * H), updatedAt: ago(7 * H) }),
      job({ id: 'job_adm_hook', userId: 2, kind: 'webhook.deliver', state: 'retrying', label: 'Envoi de notification externe', attempts: 2, maxAttempts: 4, error: 'Destinataire injoignable, nouvel essai programmé.', createdAt: ago(9 * MIN), updatedAt: ago(MIN) }),
      job({ id: 'job_adm_done', userId: 1, kind: 'sync.provider', state: 'completed', label: 'Synchronisation des activités', progressPct: 100, createdAt: ago(26 * H), updatedAt: ago(26 * H) }),
      ...(busy
        ? [
            job({ id: 'job_adm_imp', userId: 1, kind: 'import.activity', state: 'running', label: 'Import de fichier d’activité', progressPct: 72, createdAt: ago(MIN), updatedAt: ago(5000) }),
            job({ id: 'job_adm_hist2', userId: 2, kind: 'sync.history', state: 'failed', label: 'Import de l’historique complet', maxAttempts: 5, attempts: 5, error: 'Limite Garmin atteinte (429) : reprise impossible après 5 essais.', createdAt: ago(12 * H), updatedAt: ago(11 * H) }),
          ]
        : []),
    ],
  }
  return state
}

/* ---------------------------------- users ---------------------------------- */

const userName = (id: number) => db().users.find((u) => u.id === id)?.displayName ?? `#${id}`

/** Fields the 4.1 control center adds to AdminUser (deterministic per user). */
export function adminUserExtras(u: MockUser): Pick<AdminUser, 'lastSeenAt' | 'sessions' | 'activities' | 'storageBytes' | 'integrations'> {
  const d = userData(u.id)
  const activities = visibleItems(d).length
  const integrations: AdminUser['integrations'] = []
  if (d.integrations.garmin.connected) integrations.push('garmin')
  if (d.integrations.strava.connected) integrations.push('strava')
  if (llmConfigured(d)) integrations.push('llm')
  const sessions = u.status === 'active' ? (st().sessions[u.id] ?? 0) : 0
  const seen = u.lastLoginAt ? new Date(new Date(u.lastLoginAt).getTime() + (u.id === 1 ? 2 * H - 3 * MIN : 40 * MIN)).toISOString() : null
  return { lastSeenAt: sessions > 0 ? seen : u.lastLoginAt, sessions, activities, storageBytes: activities * 184_320 + (u.id === 1 ? 2_400_000 : 0), integrations }
}

/** Mirrors the store guard: the last active admin can never be demoted, disabled or deleted. */
export function isLastAdmin(u: MockUser): boolean {
  return u.role === 'admin' && u.status === 'active' && !db().users.some((x) => x.id !== u.id && x.role === 'admin' && x.status === 'active')
}

export function revokeMockSessions(id: number): number {
  const n = st().sessions[id] ?? 0
  st().sessions[id] = 0
  return n
}

/* ---------------------------------- jobs ---------------------------------- */

function allJobs(): AdminJob[] {
  const own = db()
    .users.filter((u) => u.id <= 2)
    .flatMap((u) => (v4Of(userData(u.id), u.id).jobs ?? []).map((j) => ({ ...j, userId: u.id })))
  const list = [...st().jobs, ...own.filter((j) => !st().jobs.some((x) => x.id === j.id))]
  for (const j of list) {
    if (j.state === 'running' && j.processed !== null && j.total !== null && j.processed < j.total) {
      j.processed++
      j.progressPct = Math.round((j.processed / j.total) * 1000) / 10
      j.updatedAt = new Date().toISOString()
    }
  }
  return list
    .map((j) => ({
      ...j,
      result: null,
      userName: userName(j.userId),
      retryable: j.state === 'failed' && REPLAYABLE.includes(j.kind),
      cancellable: j.state === 'queued' || j.state === 'running' || j.state === 'retrying',
    }))
    .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
}

const ACTIVE: JobState[] = ['queued', 'running', 'retrying']

function groups(): JobGroupStats[] {
  const jobs = allJobs()
  return (Object.keys(WORKERS) as JobGroup[]).map((g) => {
    const of = jobs.filter((j) => groupOf(j.kind) === g)
    return { group: g, workers: WORKERS[g], running: of.filter((j) => j.state === 'running').length, queued: of.filter((j) => j.state === 'queued').length, delayed: of.filter((j) => j.state === 'retrying').length }
  })
}

function findJob(id: string) {
  return st().jobs.find((j) => j.id === id) ?? db().users.flatMap((u) => (u.id <= 2 ? v4Of(userData(u.id), u.id).jobs : [])).find((j) => j.id === id)
}

const view = (id: string) => allJobs().find((j) => j.id === id)!

/* ---------------------------------- shares ---------------------------------- */

function allShares(): (ShareLink & { token: string; userId: number })[] {
  return db()
    .users.filter((u) => u.id <= 2)
    .flatMap((u) => (v4Of(userData(u.id), u.id).shares as (ShareLink & { token: string })[]).map((s) => Object.assign(s, { userId: u.id })))
}

function toAdminShare(s: ShareLink & { userId: number }): AdminShareLink {
  const state = s.revoked ? 'revoked' : s.expiresAt && new Date(s.expiresAt).getTime() <= Date.now() ? 'expired' : 'active'
  return {
    id: s.id,
    userId: s.userId,
    userName: userName(s.userId),
    label: s.label,
    tokenPrefix: s.tokenPrefix,
    createdAt: s.createdAt,
    expiresAt: s.expiresAt,
    revokedAt: s.revokedAt,
    accessCount: s.accessCount,
    lastAccessedAt: s.lastAccessedAt,
    state,
    periodWeeks: s.periodWeeks,
    sections: s.sections,
    anonymize: s.anonymize,
  }
}

/* ---------------------------------- feedback ---------------------------------- */

const UA = 'Mozilla/5.0 (iPhone; CPU iPhone OS 19_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.1 Mobile/15E148 Safari/604.1'
let feedback: FeedbackItem[] | null = null

function feedbackItems(): FeedbackItem[] {
  if (feedback) return feedback
  const fb = (p: Partial<FeedbackItem> & Pick<FeedbackItem, 'id' | 'userId' | 'category' | 'message'>, ageMs: number): FeedbackItem => ({
    userName: userName(p.userId),
    contactOk: false,
    contactEmail: null,
    context: { appVersion: '4.1.0', route: '/plan', viewport: '390×844', userAgent: UA },
    status: 'new',
    adminNote: '',
    createdAt: ago(ageMs),
    updatedAt: ago(ageMs),
    ...p,
  })
  feedback = [
    fb({ id: 3, userId: 2, category: 'bug', message: 'Sur Planning, quand je glisse une séance au dimanche elle revient au samedi après rechargement.\niPhone, mode sombre.', contactOk: true, contactEmail: 'sam@exemple.fr' }, 40 * MIN),
    fb({ id: 2, userId: 2, category: 'idea', message: 'Pouvoir comparer deux blocs d’entraînement côte à côte (volume, charge, ressenti).', context: { appVersion: '4.1.0', route: '/coach', viewport: '1440×900', userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 15_6) AppleWebKit/605.1.15 Safari/605.1.15' } }, 26 * H),
    fb({ id: 1, userId: 1, category: 'other', message: 'Merci pour la cartographie corporelle, très utile avec ma kiné.', status: 'done', adminNote: 'Partagé avec l’équipe.', context: { appVersion: '4.0.2', route: '/health', viewport: '390×844', userAgent: UA } }, 6 * 24 * H),
  ]
  return feedback
}

/** Deleting a user deletes their feedback (ON DELETE CASCADE on the server). */
export function dropUserFeedback(userId: number) {
  feedback = feedbackItems().filter((f) => f.userId !== userId)
}

/* ---------------------------------- routes ---------------------------------- */

export function registerAdmin(route: RouteFn, meter: () => MeterSnapshot) {
  route('GET', /^\/api\/admin\/overview$/, () => {
    const s = db()
    const users = s.users
    const jobs = allJobs()
    const shares = allShares().map(toAdminShare)
    const m = meter()
    const ov: AdminOverview = {
      generatedAt: new Date().toISOString(),
      build: { version: '4.1.0', commit: 'v41mock1', builtAt: '2026-11-07T20:00:00Z', go: 'go1.27.1', label: '4.1.0 · v41mock1 · 2026-11-07T20:00:00Z' },
      startedAt,
      uptimeS: Math.floor((Date.now() - new Date(startedAt).getTime()) / 1000),
      registrationMode: s.registrationMode,
      database: { ok: true, sizeBytes: 48_234_496, walBytes: has('adminbusy') ? 92_000_000 : 3_276_800, freeBytes: 1_048_576 },
      cache: { backend: 'valkey', ok: true, latencyMs: 0.4 },
      jobs: { groups: groups(), active: jobs.filter((j) => ACTIVE.includes(j.state)).length, failed24h: jobs.filter((j) => j.state === 'failed' && Date.now() - new Date(j.updatedAt).getTime() < 24 * H).length },
      streams: { clients: 3, users: 2 },
      errors24h: 17,
      logsDropped: 0,
      providers: [
        ...m.providers,
        { provider: 'llm', requests1h: 3, requests24h: 22, errors1h: 0, rateLimited24h: 0, inFlight: 0, cooldownUntil: null, quota: null, avgLatencyMs: 8400 },
      ],
      counts: {
        users: users.length,
        activeUsers: users.filter((u) => u.status === 'active').length,
        admins: users.filter((u) => u.role === 'admin' && u.status === 'active').length,
        connectedUsers: users.filter((u) => adminUserExtras(u).integrations.some((i) => i !== 'llm')).length,
        llmUsers: users.filter((u) => adminUserExtras(u).integrations.includes('llm')).length,
        activities: users.reduce((n, u) => n + adminUserExtras(u).activities, 0),
        activeShareLinks: shares.filter((l) => l.state === 'active').length,
        sessions: users.reduce((n, u) => n + adminUserExtras(u).sessions, 0),
        feedbackNew: feedbackItems().filter((f) => f.status === 'new').length,
      },
      config: {
        publicUrl: typeof window !== 'undefined' ? window.location.origin : 'http://localhost:5173',
        rpId: typeof window !== 'undefined' ? window.location.hostname : 'localhost',
        secureCookies: false,
        sessionTtlHours: 720,
        timezone: 'Europe/Paris',
        logLevel: 'info',
        logFormat: 'text',
        cacheBackend: 'valkey',
        metricsEnabled: true,
        stravaEnabled: !has('nostrava'),
        weatherProvider: 'open-meteo',
        allowPrivateEgress: false,
        syncHour: 5,
      },
    }
    return json(ov)
  })

  route('DELETE', /^\/api\/admin\/users\/(\d+)\/sessions$/, ({ me, params }) => {
    const id = Number(params[0])
    if (id === me.id) return err(409, 'CONFLICT', 'Pour fermer tes propres sessions, utilise la déconnexion.')
    if (!db().users.some((u) => u.id === id)) return err(404, 'NOT_FOUND', 'Utilisateur introuvable.')
    return json({ revoked: revokeMockSessions(id) })
  })

  route('POST', /^\/api\/admin\/users\/(\d+)\/sync$/, ({ params }) => {
    const u = db().users.find((x) => x.id === Number(params[0]))
    if (!u) return err(404, 'NOT_FOUND', 'Ressource introuvable.')
    if (u.status !== 'active') return err(409, 'CONFLICT', 'Compte désactivé : réactivez-le avant de lancer une synchronisation.')
    if (!adminUserExtras(u).integrations.some((i) => i !== 'llm')) return err(412, 'PROVIDER_NOT_CONNECTED', 'Aucun fournisseur connecté pour ce compte.')
    const s = st()
    const running = s.jobs.find((j) => j.userId === u.id && j.kind === 'sync.provider' && ACTIVE.includes(j.state))
    if (running) return json(view(running.id), 202)
    const j = job({ id: `job_adm_sync_${s.seq++}`, userId: u.id, kind: 'sync.provider', state: 'running', label: 'Synchronisation des activités', processed: 0, total: 6, progressPct: 0, createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() })
    s.jobs.unshift(j)
    return json(view(j.id), 202)
  })

  route('GET', /^\/api\/admin\/jobs$/, ({ url }) => {
    const f = url.searchParams.get('state') ?? 'all'
    if (!['active', 'failed', 'all'].includes(f)) return err(400, 'BAD_REQUEST', 'Filtre inconnu (active, failed ou all).')
    const items = allJobs().filter((j) => (f === 'active' ? ACTIVE.includes(j.state) : f === 'failed' ? j.state === 'failed' : true))
    return json({ items: items.slice(0, 100), groups: groups() })
  })

  route('POST', /^\/api\/admin\/jobs\/([^/]+)\/retry$/, ({ params }) => {
    const j = findJob(decodeURIComponent(params[0]))
    if (!j) return err(404, 'NOT_FOUND', 'Ressource introuvable.')
    if (j.state !== 'failed' || !REPLAYABLE.includes(j.kind)) return err(409, 'CONFLICT', 'Seules les synchronisations en échec peuvent être relancées d’ici.')
    const s = st()
    const now = new Date().toISOString()
    const n = job({ ...j, id: `job_adm_retry_${s.seq++}`, state: 'queued', attempts: 0, error: null, progressPct: null, createdAt: now, updatedAt: now })
    s.jobs.unshift(n)
    setTimeout(() => {
      n.state = 'running'
      n.processed = 0
      n.total = 8
      n.attempts = 1
      n.updatedAt = new Date().toISOString()
    }, 1500)
    return json(view(n.id), 202)
  })

  route('POST', /^\/api\/admin\/jobs\/([^/]+)\/cancel$/, ({ params }) => {
    const j = findJob(decodeURIComponent(params[0]))
    if (!j) return err(404, 'NOT_FOUND', 'Ressource introuvable.')
    if (!ACTIVE.includes(j.state)) return err(409, 'CONFLICT', 'Cette tâche est déjà terminée.')
    j.state = 'failed'
    j.error = 'Tâche annulée par un administrateur.'
    j.updatedAt = new Date().toISOString()
    return json(view(j.id))
  })

  route('GET', /^\/api\/admin\/shares$/, ({ url }) => {
    const all = url.searchParams.get('state') === 'all'
    const links = allShares()
      .map(toAdminShare)
      .filter((l) => all || l.state === 'active')
      .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
    return json(links)
  })

  route('DELETE', /^\/api\/admin\/shares\/(\d+)$/, ({ params }) => {
    const s = allShares().find((x) => x.id === Number(params[0]))
    if (!s) return err(404, 'NOT_FOUND', 'Ressource introuvable.')
    if (!s.revoked) {
      s.revoked = true
      s.revokedAt = new Date().toISOString()
    }
    return new Response(null, { status: 204 })
  })

  route('POST', /^\/api\/feedback$/, ({ me, init }) => {
    const b = (typeof init?.body === 'string' ? JSON.parse(init.body) : {}) as Partial<FeedbackItem> & { contactEmail?: string }
    const message = (b.message ?? '').trim()
    if (!b.category || !['bug', 'idea', 'other'].includes(b.category)) return err(400, 'BAD_REQUEST', 'Catégorie invalide (bug, idea ou other).')
    if (message.length < 5 || message.length > 4000) return err(400, 'BAD_REQUEST', 'Message requis : entre 5 et 4000 caractères.')
    if (b.contactOk && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(b.contactEmail ?? '')) return err(400, 'BAD_REQUEST', 'Adresse e-mail invalide : elle est nécessaire pour être recontacté.')
    const list = feedbackItems()
    if (list.filter((f) => f.userId === me.id && Date.now() - new Date(f.createdAt).getTime() < H).length >= 5) return err(429, 'RATE_LIMITED', 'Merci ! Tu as déjà envoyé plusieurs avis : réessaie dans une heure.')
    const now = new Date().toISOString()
    const item: FeedbackItem = {
      id: list.reduce((m, f) => Math.max(m, f.id), 0) + 1,
      userId: me.id,
      userName: me.displayName,
      category: b.category,
      message,
      contactOk: !!b.contactOk,
      contactEmail: b.contactOk ? (b.contactEmail ?? null) : null,
      context: { appVersion: '', route: '', viewport: '', userAgent: '', ...b.context },
      status: 'new',
      adminNote: '',
      createdAt: now,
      updatedAt: now,
    }
    list.unshift(item)
    return json({ id: item.id, createdAt: now }, 201)
  })

  route('GET', /^\/api\/admin\/feedback$/, ({ url }) => {
    const status = url.searchParams.get('status') ?? 'all'
    const category = url.searchParams.get('category') ?? 'all'
    const all = feedbackItems()
    const items = all.filter((f) => (status === 'all' || f.status === status) && (category === 'all' || f.category === category))
    const n = (s: FeedbackStatus) => all.filter((f) => f.status === s).length
    return json({ items, counts: { new: n('new'), read: n('read'), done: n('done'), total: all.length } })
  })

  route('PATCH', /^\/api\/admin\/feedback\/(\d+)$/, ({ params, init }) => {
    const f = feedbackItems().find((x) => x.id === Number(params[0]))
    if (!f) return err(404, 'NOT_FOUND', 'Avis introuvable.')
    const b = (typeof init?.body === 'string' ? JSON.parse(init.body) : {}) as { status?: FeedbackStatus; adminNote?: string }
    if (b.status !== undefined && !['new', 'read', 'done'].includes(b.status)) return err(400, 'BAD_REQUEST', 'Statut invalide (new, read ou done).')
    if (b.status) f.status = b.status
    if (b.adminNote !== undefined) f.adminNote = b.adminNote.trim().slice(0, 2000)
    f.updatedAt = new Date().toISOString()
    return json(f)
  })

  route('POST', /^\/api\/admin\/maintenance\/purge$/, () => json({ removed: { sessions: 4, invitations: 1, apiEvents: 37, logs: 1180, jobs: 12, shareLinks: 1 } }))

  route('POST', /^\/api\/admin\/maintenance\/optimize$/, ({ init }) => {
    const vacuum = typeof init?.body === 'string' && (JSON.parse(init.body) as { vacuum?: boolean }).vacuum === true
    const before = { ok: true, sizeBytes: 48_234_496, walBytes: 3_276_800, freeBytes: 1_048_576 }
    const after = { ok: true, sizeBytes: vacuum ? 47_185_920 : before.sizeBytes, walBytes: 0, freeBytes: vacuum ? 0 : before.freeBytes }
    return json({ vacuum, before, after, durationMs: vacuum ? 1840 : 62 })
  })

  route('POST', /^\/api\/admin\/maintenance\/check$/, () =>
    has('dbcorrupt')
      ? json({ ok: false, problems: ['foreign_key_check: activity_notes rowid 412 → activities (parent manquant)', 'foreign_key_check: activity_notes rowid 413 → activities (parent manquant)'], total: 2, durationMs: 940 })
      : json({ ok: true, problems: [], total: 0, durationMs: 870 }),
  )
}
