// 4.2.0 mock endpoints (§15): share portal permissions & notes, public portal writes, atomic backups + S3,
// Smart Workout Morphing. Synthetic, deterministic data only.
// Flags: nos3 (no S3 target configured) · s3err (S3 test / upload refused) · calm (no morphing constraint)

import type {
  BackupItem,
  BackupList,
  CalendarEvent,
  CalendarEventInput,
  DirectiveRestrictions,
  ErrorCode,
  EventKind,
  JobStatus,
  MorphChange,
  MorphConstraint,
  MorphPreview,
  PlannedSession,
  PlannedSessionMorph,
  PlanSport,
  PublicActivityDetail,
  S3Config,
  S3ConfigInput,
  ShareAuthorRole,
  ShareDirectiveInput,
  ShareNote,
  SharePermissions,
  ShareRemarkInput,
  WorkoutStep,
} from '../api/types'
import { BODY_SIDE_LABEL } from '../components/bodymap/bodyMapMeta'
import { stepsSummary, totalDurationS } from '../components/plan/steps'
import { addDays, fmtHM, fmtKm, fmtNumber, fmtPaceValue, isNum, todayISO } from '../lib/format'
import { emitStreamEvent } from '../state/events'
import type { CatalogItem } from './catalog'
import { db, nextId, type MockUser, type PlanEntry, type UserData } from './db'
import { loadRatio, notesOf, plannedWithMatch, sourceMode, visibleItems } from './domain'
import { has } from './flags'
import { v31Of } from './manual'
import { enrichActivityView, llmConfigured } from './v3'
import { addPublicShareHook, findShareByToken, v4Of, type StoredShare } from './v4'
import { buildActivityView } from './view'

type Ctx = { url: URL; params: string[]; init?: RequestInit; me: MockUser; data: UserData }
type PublicCtx = { url: URL; params: string[]; init?: RequestInit }
type RouteFn = (method: string, re: RegExp, handler: (c: Ctx) => Response | Promise<Response>) => void
type PubFn = (method: string, re: RegExp, handler: (c: PublicCtx) => Response) => void

interface V42Data {
  notes: ShareNote[]
}

interface V42Global {
  backups: BackupItem[]
  s3: S3Config & { secretKey: string }
}

const json = (body: unknown, status = 200, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json; charset=utf-8', ...headers } })
const noContent = () => new Response(null, { status: 204 })
const err = (status: number, code: ErrorCode, error: string) => json({ error, code }, status)
const isoNow = () => new Date().toISOString()
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/
const NO_PERMISSIONS: SharePermissions = { comments: false, planning: false, activityDetails: false }
const ROLES: ShareAuthorRole[] = ['physio', 'coach', 'other']
const PLAN_SPORTS: PlanSport[] = ['run', 'bike', 'strength', 'mobility', 'rest', 'other']
const EVENT_KINDS: EventKind[] = ['fatigue', 'pain', 'illness', 'unavailable', 'forced_rest']
const DEMO_AUTHOR = 'Claire Martin'

function parseBody<T>(init?: RequestInit): T | null {
  try {
    return typeof init?.body === 'string' ? (JSON.parse(init.body) as T) : null
  } catch {
    return null
  }
}

/** Same sanitising as feedback: control characters removed, line endings normalised, trimmed. */
function clean(v: unknown, max: number): string | null {
  if (typeof v !== 'string') return null
  // eslint-disable-next-line no-control-regex
  const s = v.replace(/\r\n?/g, '\n').replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F​-‏‪-‮⁦-⁩]/g, '').trim()
  return s.length >= 1 && s.length <= max ? s : null
}

/* ---------------------------------- State & seed ---------------------------------- */

export function v42Of(data: UserData): V42Data {
  if (!data.v42) data.v42 = { notes: [] } satisfies V42Data
  return data.v42 as V42Data
}

/** Seeds the demo physio link once (directive, a remark and one prescribed session); idempotent. */
export function ensureV42(data: UserData, userId: number) {
  if (data.v42) return
  const st = v42Of(data)
  const today = todayISO()
  const demo = v4Of(data, userId).shares.find((s) => s.token === 'demo-kine')
  if (!demo || data.plan.length === 0) return
  const at = (daysAgo: number) => new Date(Date.now() - daysAgo * 86_400_000).toISOString()
  st.notes.push({
    id: nextId(),
    kind: 'directive',
    linkId: demo.id,
    linkLabel: demo.label,
    authorName: DEMO_AUTHOR,
    authorRole: 'physio',
    body: 'Pas de fractionné ni de descente technique jusqu’au prochain contrôle. Footings de 40 min maximum sur le plat, cadence 176 au métronome.',
    activityId: null,
    validUntil: addDays(today, 14),
    restrictions: { noIntervals: true, maxIntensity: 'easy', maxElevLossM: 150, cadenceTargetSpm: 176 },
    createdAt: at(3),
    updatedAt: at(3),
  })
  const lastRun = visibleItems(data, today).find((it) => it.isRunning)
  if (lastRun) {
    st.notes.push({
      id: nextId(),
      kind: 'remark',
      linkId: demo.id,
      linkLabel: demo.label,
      authorName: DEMO_AUTHOR,
      authorRole: 'physio',
      body: 'Bonne gestion de la descente, la cadence tient. On garde ce volume cette semaine.',
      activityId: lastRun.id,
      validUntil: null,
      restrictions: null,
      createdAt: at(1),
      updatedAt: at(1),
    })
  }
  data.plan.push({
    id: nextId(),
    date: addDays(today, 4),
    sport: 'strength',
    title: 'Renfo excentrique (kiné)',
    targetDurationS: 1500,
    targetDistanceM: null,
    targetPaceMinSecPerKm: null,
    targetPaceMaxSecPerKm: null,
    targetCadenceSpm: null,
    instructions: 'Step-down lent 3×12 par jambe, squat espagnol 3×45″. Douleur ≤ 3/10 pendant l’exercice, sinon on arrête.',
    steps: [],
    source: 'share',
    garmin: { status: 'none', workoutId: null, pushedAt: null, error: null },
    prescribedBy: { linkId: demo.id, label: demo.label, authorName: DEMO_AUTHOR },
    morph: null,
  })
}

function linkActive(share: StoredShare | undefined): boolean {
  return !!share && !share.revoked && !(share.expiresAt && new Date(share.expiresAt).getTime() <= Date.now())
}

function activeDirectives(data: UserData, userId: number): ShareNote[] {
  const today = todayISO()
  const shares = v4Of(data, userId).shares
  return v42Of(data).notes.filter((n) => n.kind === 'directive' && linkActive(shares.find((s) => s.id === n.linkId)) && (!n.validUntil || n.validUntil >= today))
}

function globalState(): V42Global {
  const s = db()
  if (!s.v42) {
    const today = todayISO()
    const noS3 = has('nos3')
    const backups: BackupItem[] = []
    const mk = (daysAgo: number, time: string, trigger: BackupItem['trigger'], s3: BackupItem['s3']['status'], size: number, version = '4.2.0') => {
      const date = addDays(today, -daysAgo)
      const stamp = `${date.replace(/-/g, '')}T${time.replace(':', '')}00Z`
      const name = `stride-${stamp}-${version}.sqlite.gz`
      const createdAt = `${date}T${time}:00Z`
      backups.push({
        name,
        sizeBytes: size,
        sha256: sha256Of(name),
        createdAt,
        trigger,
        s3: {
          status: noS3 ? 'none' : s3,
          key: !noS3 && s3 !== 'none' ? `prod/${name}` : null,
          syncedAt: !noS3 && s3 === 'synced' ? new Date(Date.parse(createdAt) + 42_000).toISOString() : null,
          error: !noS3 && s3 === 'error' ? 'Envoi refusé par le stockage S3 (403 SignatureDoesNotMatch). Vérifie la clé secrète.' : null,
        },
      })
    }
    mk(0, '03:00', 'daily', 'synced', 18_903_552)
    mk(1, '03:00', 'daily', 'synced', 18_871_808)
    mk(2, '03:00', 'daily', 'synced', 18_822_144)
    mk(3, '09:12', 'pre_migration', 'none', 18_790_400, '4.1.0')
    mk(5, '21:47', 'manual', 'error', 18_655_232, '4.1.0')
    s.v42 = {
      backups,
      s3: noS3
        ? { endpoint: '', region: '', bucket: '', prefix: '', accessKeyId: '', secretKey: '', secretKeySet: false, pathStyle: false, autoUpload: false, retentionDays: 30 }
        : { endpoint: 'https://s3.fr-par.scw.cloud', region: 'fr-par', bucket: 'stride-backups', prefix: 'prod/', accessKeyId: 'SCWDEMOACCESSKEY0000', secretKey: 'mock-secret', secretKeySet: true, pathStyle: false, autoUpload: true, retentionDays: 30 },
    } satisfies V42Global
  }
  return s.v42 as V42Global
}

/** Deterministic 64-hex digest (FNV-1a rounds) standing in for a SHA-256. */
function sha256Of(seed: string): string {
  let out = ''
  for (let round = 0; out.length < 64; round++) {
    let h = 0x811c9dc5 ^ round
    for (let i = 0; i < seed.length; i++) h = Math.imul(h ^ seed.charCodeAt(i), 0x01000193)
    out += (h >>> 0).toString(16).padStart(8, '0')
  }
  return out.slice(0, 64)
}

const publicS3 = ({ secretKey: _s, ...cfg }: V42Global['s3']): S3Config => ({ ...cfg, secretKeySet: _s.length > 0 })
const s3Ready = (c: V42Global['s3']) => Boolean(c.endpoint && c.bucket && c.accessKeyId && c.secretKey)

/* ---------------------------------- Public portal enrichment ---------------------------------- */

addPublicShareHook((view, owner, data, share) => {
  ensureV42(data, owner.id)
  const permissions = share.permissions ?? NO_PERMISSIONS
  const mine = v42Of(data).notes.filter((n) => n.linkId === share.id)
  const out = { ...view, permissions, directive: mine.find((n) => n.kind === 'directive') ?? null }
  if (out.recentSessions) out.recentSessions = out.recentSessions.map((r) => ({ ...r, remark: mine.find((n) => n.kind === 'remark' && n.activityId === r.id) ?? null }))
  if (permissions.planning) {
    const today = todayISO()
    const from = addDays(today, -7)
    const to = addDays(today, 42)
    out.calendar = {
      planned: plannedWithMatch(data, visibleItems(data, today), today, from, to),
      events: v31Of(data).events.filter((e) => e.endDate >= from && e.startDate <= to).sort((a, b) => a.startDate.localeCompare(b.startDate)),
    }
  }
  return out
})

/* ---------------------------------- Morphing rules ---------------------------------- */

const MORPH_FIELDS = ['title', 'targetDurationS', 'targetDistanceM', 'targetPaceMinSecPerKm', 'targetPaceMaxSecPerKm', 'targetCadenceSpm', 'instructions', 'steps'] as const

function originalOf(p: PlanEntry): PlannedSessionMorph['original'] {
  return {
    title: p.title,
    targetDurationS: p.targetDurationS,
    targetDistanceM: p.targetDistanceM,
    targetPaceMinSecPerKm: p.targetPaceMinSecPerKm,
    targetPaceMaxSecPerKm: p.targetPaceMaxSecPerKm,
    targetCadenceSpm: p.targetCadenceSpm,
    instructions: p.instructions,
    steps: p.steps,
  }
}

function restrictionsText(r: DirectiveRestrictions | null): string {
  if (!r) return ''
  return [
    r.noIntervals && 'pas de fractionné',
    r.maxIntensity === 'easy' ? 'intensité facile' : r.maxIntensity === 'moderate' ? 'intensité modérée au plus' : null,
    isNum(r.maxElevLossM) && `D− ≤ ${r.maxElevLossM} m`,
    isNum(r.cadenceTargetSpm) && `cadence ${r.cadenceTargetSpm} spm`,
  ]
    .filter(Boolean)
    .join(', ')
}

function constraintsFor(data: UserData, userId: number, p: PlanEntry): MorphConstraint[] {
  const today = todayISO()
  if (has('calm') || p.sport === 'rest' || (p.date !== today && p.date !== addDays(today, 1))) return []
  const out: MorphConstraint[] = []
  for (const d of activeDirectives(data, userId)) {
    const who = d.authorRole === 'coach' ? 'coach' : 'kiné'
    out.push({ kind: 'directive', label: `Consigne ${who}`, detail: restrictionsText(d.restrictions) || d.body.slice(0, 120), severity: 2 })
  }
  const since = addDays(today, -2)
  const pains = v4Of(data, userId).bodyEntries.filter((e) => e.date >= since && Math.max(e.painScore, e.coldNextDayScore ?? 0) >= 3)
  if (pains.length) {
    const worst = pains.reduce((a, b) => (Math.max(b.painScore, b.coldNextDayScore ?? 0) > Math.max(a.painScore, a.coldNextDayScore ?? 0) ? b : a))
    const score = Math.max(worst.painScore, worst.coldNextDayScore ?? 0)
    out.push({
      kind: 'pain',
      label: `Douleur ${fmtNumber(score, 1, { trim: true })}/10`,
      detail: `${worst.zoneLabel}, ${BODY_SIDE_LABEL[worst.side].toLowerCase()} · ${worst.date === today ? 'aujourd’hui' : worst.date === addDays(today, -1) ? 'hier' : 'avant-hier'}`,
      severity: score >= 6 ? 3 : score >= 4 ? 2 : 1,
    })
  }
  const lr = loadRatio(visibleItems(data, today), today, sourceMode(data))
  if (lr?.ratio && lr.ratio > 1.3) out.push({ kind: 'load', label: `Charge ACWR ${fmtNumber(lr.ratio, 2)}`, detail: 'Charge des 7 derniers jours nettement au-dessus de la moyenne sur 28 jours.', severity: lr.ratio > 1.5 ? 3 : 2 })
  if (p.sport === 'run' && p.date === today) out.push({ kind: 'heat', label: 'Chaleur 24 °C', detail: 'Prévision Open-Meteo au créneau de 18 h.', severity: 1 })
  if (p.sport === 'run' && p.date === addDays(today, 1)) out.push({ kind: 'wind', label: 'Vent 34 km/h', detail: 'Rafales prévues au créneau habituel.', severity: 1 })
  return out
}

const hasIntervals = (steps: WorkoutStep[]): boolean => steps.some((s) => s.type === 'interval' || (s.type === 'repeat' && hasIntervals(s.steps ?? [])))
const scaleSteps = (steps: WorkoutStep[], k: number): WorkoutStep[] =>
  steps.map((s) => ({ ...s, durationS: isNum(s.durationS) ? Math.round((s.durationS * k) / 30) * 30 : s.durationS, steps: s.steps ? scaleSteps(s.steps, k) : s.steps }))
const paceRange = (a: number | null, b: number | null) => (isNum(a) || isNum(b) ? `${fmtPaceValue(a ?? b)}–${fmtPaceValue(b ?? a)}/km` : '—')

function proposalFor(data: UserData, userId: number, p: PlanEntry, cs: MorphConstraint[]): { proposal: PlannedSession | null; changes: MorphChange[] } {
  if (cs.length === 0) return { proposal: null, changes: [] }
  const base = plannedWithMatch(data, visibleItems(data), todayISO()).find((x) => x.id === p.id)
  if (!base) return { proposal: null, changes: [] }
  const q: PlannedSession = JSON.parse(JSON.stringify(base)) as PlannedSession
  const changes: MorphChange[] = []
  const dir = activeDirectives(data, userId)[0]
  const r = dir?.restrictions ?? null
  const pain = Math.max(0, ...cs.filter((c) => c.kind === 'pain').map((c) => c.severity))
  const load = Math.max(0, ...cs.filter((c) => c.kind === 'load').map((c) => c.severity))
  const heat = cs.some((c) => c.kind === 'heat')
  const wind = cs.some((c) => c.kind === 'wind')

  if ((r?.noIntervals || pain >= 2) && hasIntervals(q.steps)) {
    const total = totalDurationS(q.steps) ?? q.targetDurationS ?? 2400
    const easy = { kind: 'pace' as const, min: 335, max: 355 }
    const before = stepsSummary(q.steps)
    q.steps = [
      { type: 'warmup', durationS: 600 },
      { type: 'run', durationS: Math.max(600, total - 900), target: easy },
      { type: 'cooldown', durationS: 300 },
    ]
    q.title = `${p.title.replace(/\s*\(.*\)$/, '')} → footing continu`
    changes.push({ field: 'steps', stepIndex: null, before, after: stepsSummary(q.steps), reason: r?.noIntervals ? 'Consigne kiné : pas de fractionné.' : 'Douleur récente : pas d’intensité.' })
    changes.push({ field: 'title', stepIndex: null, before: p.title, after: q.title, reason: 'Le titre suit la nouvelle structure.' })
  }
  const cut = Math.max(pain, load) >= 3 ? 0.3 : Math.max(pain, load) === 2 ? 0.2 : Math.max(pain, load) === 1 ? 0.15 : 0
  if (cut > 0) {
    const why = pain >= load ? `Douleur récente : volume −${Math.round(cut * 100)} %.` : `Charge élevée : volume −${Math.round(cut * 100)} %.`
    if (isNum(q.targetDurationS)) {
      const next = Math.round((q.targetDurationS * (1 - cut)) / 60) * 60
      changes.push({ field: 'targetDurationS', stepIndex: null, before: fmtHM(q.targetDurationS), after: fmtHM(next), reason: why })
      q.targetDurationS = next
    }
    if (isNum(q.targetDistanceM)) {
      const next = Math.round((q.targetDistanceM * (1 - cut)) / 100) * 100
      changes.push({ field: 'targetDistanceM', stepIndex: null, before: fmtKm(q.targetDistanceM, 1), after: fmtKm(next, 1), reason: why })
      q.targetDistanceM = next
    }
    if (q.steps.length) q.steps = scaleSteps(q.steps, 1 - cut)
  }
  if (heat && (isNum(q.targetPaceMinSecPerKm) || isNum(q.targetPaceMaxSecPerKm))) {
    const before = paceRange(q.targetPaceMinSecPerKm, q.targetPaceMaxSecPerKm)
    q.targetPaceMinSecPerKm = isNum(q.targetPaceMinSecPerKm) ? q.targetPaceMinSecPerKm + 10 : null
    q.targetPaceMaxSecPerKm = isNum(q.targetPaceMaxSecPerKm) ? q.targetPaceMaxSecPerKm + 15 : null
    changes.push({ field: 'pace', stepIndex: null, before, after: paceRange(q.targetPaceMinSecPerKm, q.targetPaceMaxSecPerKm), reason: 'Chaleur au-dessus de 22 °C : allure ralentie de 10 à 15 s/km.' })
  }
  if (isNum(r?.cadenceTargetSpm) && q.sport === 'run') {
    const spm = r.cadenceTargetSpm
    if (q.targetCadenceSpm !== spm) {
      changes.push({ field: 'targetCadenceSpm', stepIndex: null, before: isNum(q.targetCadenceSpm) ? `${q.targetCadenceSpm} spm` : '—', after: `${spm} spm`, reason: 'Cadence demandée par le kiné.' })
      q.targetCadenceSpm = spm
    }
    q.steps = q.steps.map((s, i) => {
      if (s.type !== 'warmup' && s.type !== 'cooldown') return s
      if (s.target && s.target.kind !== 'none') return s
      const next: WorkoutStep = { ...s, target: { kind: 'cadence', min: spm - 2, max: spm + 2 } }
      changes.push({ field: 'steps', stepIndex: i, before: stepsSummary([s]), after: stepsSummary([next]), reason: 'Cadence du kiné injectée dans l’étape.' })
      return next
    })
  }
  if (wind) {
    const add = 'Vent fort : cours au ressenti et à la FC, pas à l’allure.'
    changes.push({ field: 'instructions', stepIndex: null, before: q.instructions || '—', after: [q.instructions, add].filter(Boolean).join(' '), reason: 'Vent au-dessus de 30 km/h.' })
    q.instructions = [q.instructions, add].filter(Boolean).join(' ')
  }
  return { proposal: changes.length ? q : null, changes }
}

function previewFor(data: UserData, userId: number, p: PlanEntry): MorphPreview {
  const original = plannedWithMatch(data, visibleItems(data), todayISO()).find((x) => x.id === p.id) as PlannedSession
  if (p.morph) return { sessionId: p.id, constraints: constraintsFor(data, userId, p), original, proposal: null, changes: [] }
  const constraints = constraintsFor(data, userId, p)
  const { proposal, changes } = proposalFor(data, userId, p, constraints)
  return { sessionId: p.id, constraints, original, proposal, changes }
}

/* ---------------------------------- Jobs ---------------------------------- */

function newJob(userId: number, kind: JobStatus['kind'], label: string): JobStatus {
  return { id: `job_${kind.replace('.', '_')}_${nextId()}`, userId, kind, queue: kind.startsWith('backup.') ? 'low' : 'critical', state: 'running', label, progressPct: 5, processed: null, total: null, attempts: 1, maxAttempts: 4, error: null, result: null, createdAt: isoNow(), updatedAt: isoNow() }
}

/** Plays a job's progress over the synthetic event stream, then runs `finish` (which may fail it). */
function playJob(job: JobStatus, steps: number[], finish: () => string | null, stepMs = 450) {
  steps.forEach((pct, i) =>
    setTimeout(() => {
      job.progressPct = pct
      job.updatedAt = isoNow()
      emitStreamEvent('job.updated', { ...job })
    }, stepMs * (i + 1)),
  )
  setTimeout(() => {
    const failure = finish()
    job.state = failure ? 'failed' : 'completed'
    job.error = failure
    job.progressPct = failure ? job.progressPct : 100
    job.updatedAt = isoNow()
    emitStreamEvent('job.updated', { ...job })
  }, stepMs * (steps.length + 1))
}

/* ---------------------------------- Registration ---------------------------------- */

export function registerV42(route: RouteFn, pub: PubFn) {
  /* --- public portal (token-scoped, no session) --- */
  const linkFor = (tokenRaw: string, need: keyof SharePermissions | null) => {
    const hit = findShareByToken(decodeURIComponent(tokenRaw))
    if ('status' in hit) return err(404, 'NOT_FOUND', 'Lien de partage introuvable, expiré ou révoqué.')
    ensureV42(hit.data, hit.owner.id)
    if (need && !(hit.share.permissions ?? NO_PERMISSIONS)[need]) return err(403, 'SHARE_FORBIDDEN', 'Ce lien ne permet pas cette action.')
    return hit
  }
  const inWindow = (data: UserData, share: StoredShare, id: number): CatalogItem | null => {
    const today = todayISO()
    const it = visibleItems(data, today).find((x) => x.id === id) ?? null
    return it && it.date >= addDays(today, -share.periodWeeks * 7) ? it : null
  }
  const author = (b: { authorName?: unknown; authorRole?: unknown } | null) => ({
    authorName: b?.authorName == null || b.authorName === '' ? null : clean(b.authorName, 80),
    authorRole: ROLES.includes(b?.authorRole as ShareAuthorRole) ? (b?.authorRole as ShareAuthorRole) : ('other' as const),
  })
  const notify = (kind: 'directive' | 'remark' | 'plan' | 'event', linkId: number) => emitStreamEvent('share.updated', { kind, linkId })

  pub('PUT', /^\/api\/public\/share\/([^/]+)\/directive$/, ({ params, init }) => {
    const hit = linkFor(params[0], 'comments')
    if (hit instanceof Response) return hit
    const b = parseBody<ShareDirectiveInput>(init)
    const body = clean(b?.body, 2000)
    if (!body) return err(400, 'BAD_REQUEST', 'La consigne doit contenir entre 1 et 2000 caractères.')
    const validUntil = b?.validUntil ?? null
    if (validUntil !== null && (!DATE_RE.test(validUntil) || validUntil < todayISO())) return err(400, 'BAD_REQUEST', 'Date de validité invalide (aujourd’hui ou plus tard).')
    const r = b?.restrictions ?? null
    if (r && (typeof r.noIntervals !== 'boolean' || ![null, 'easy', 'moderate'].includes(r.maxIntensity) || (r.maxElevLossM !== null && !(r.maxElevLossM >= 0 && r.maxElevLossM <= 5000)) || (r.cadenceTargetSpm !== null && !(r.cadenceTargetSpm >= 120 && r.cadenceTargetSpm <= 220))))
      return err(400, 'BAD_REQUEST', 'Restrictions invalides.')
    const a = author(b)
    const notes = v42Of(hit.data).notes
    const cur = notes.find((n) => n.kind === 'directive' && n.linkId === hit.share.id)
    const note: ShareNote = { id: cur?.id ?? nextId(), kind: 'directive', linkId: hit.share.id, linkLabel: hit.share.label, ...a, body, activityId: null, validUntil, restrictions: r, createdAt: cur?.createdAt ?? isoNow(), updatedAt: isoNow() }
    if (cur) Object.assign(cur, note)
    else notes.unshift(note)
    notify('directive', hit.share.id)
    return json(note)
  })
  pub('DELETE', /^\/api\/public\/share\/([^/]+)\/directive$/, ({ params }) => {
    const hit = linkFor(params[0], 'comments')
    if (hit instanceof Response) return hit
    const st = v42Of(hit.data)
    const before = st.notes.length
    st.notes = st.notes.filter((n) => !(n.kind === 'directive' && n.linkId === hit.share.id))
    if (before === st.notes.length) return err(404, 'NOT_FOUND', 'Aucune consigne à supprimer.')
    notify('directive', hit.share.id)
    return noContent()
  })
  pub('PUT', /^\/api\/public\/share\/([^/]+)\/activities\/(\d+)\/remark$/, ({ params, init }) => {
    const hit = linkFor(params[0], 'comments')
    if (hit instanceof Response) return hit
    const it = inWindow(hit.data, hit.share, Number(params[1]))
    if (!it) return err(404, 'NOT_FOUND', 'Séance introuvable sur la période partagée.')
    const b = parseBody<ShareRemarkInput>(init)
    const body = clean(b?.body, 1000)
    if (!body) return err(400, 'BAD_REQUEST', 'La remarque doit contenir entre 1 et 1000 caractères.')
    const notes = v42Of(hit.data).notes
    const cur = notes.find((n) => n.kind === 'remark' && n.linkId === hit.share.id && n.activityId === it.id)
    const note: ShareNote = { id: cur?.id ?? nextId(), kind: 'remark', linkId: hit.share.id, linkLabel: hit.share.label, ...author(b), body, activityId: it.id, validUntil: null, restrictions: null, createdAt: cur?.createdAt ?? isoNow(), updatedAt: isoNow() }
    if (cur) Object.assign(cur, note)
    else notes.unshift(note)
    notify('remark', hit.share.id)
    return json(note)
  })
  pub('DELETE', /^\/api\/public\/share\/([^/]+)\/activities\/(\d+)\/remark$/, ({ params }) => {
    const hit = linkFor(params[0], 'comments')
    if (hit instanceof Response) return hit
    const st = v42Of(hit.data)
    const before = st.notes.length
    st.notes = st.notes.filter((n) => !(n.kind === 'remark' && n.linkId === hit.share.id && n.activityId === Number(params[1])))
    if (before === st.notes.length) return err(404, 'NOT_FOUND', 'Aucune remarque à supprimer.')
    notify('remark', hit.share.id)
    return noContent()
  })
  pub('GET', /^\/api\/public\/share\/([^/]+)\/activities\/(\d+)$/, ({ params }) => {
    const hit = linkFor(params[0], 'activityDetails')
    if (hit instanceof Response) return hit
    const it = inWindow(hit.data, hit.share, Number(params[1]))
    if (!it) return err(404, 'NOT_FOUND', 'Séance introuvable sur la période partagée.')
    return json(publicDetail(hit.data, hit.share, it), 200, { 'Cache-Control': 'private, no-store' })
  })

  const planWindow = (date: unknown) => typeof date === 'string' && DATE_RE.test(date) && date >= addDays(todayISO(), -7) && date <= addDays(todayISO(), 120)
  const ownedBy = (share: StoredShare, x: { prescribedBy?: { linkId: number | null } | null }) => x.prescribedBy?.linkId === share.id
  const sessionFrom = (b: Partial<PlanEntry>, base?: PlanEntry): string | null => {
    const m = { ...base, ...b }
    if (!planWindow(m.date)) return 'Date hors de la période autorisée (J−7 à J+120).'
    if (!PLAN_SPORTS.includes(m.sport as PlanSport)) return 'Sport invalide.'
    if (!clean(m.title, 120)) return 'Le titre est requis (120 caractères au plus).'
    return null
  }

  pub('POST', /^\/api\/public\/share\/([^/]+)\/plan$/, ({ params, init }) => {
    const hit = linkFor(params[0], 'planning')
    if (hit instanceof Response) return hit
    const b = parseBody<Partial<PlanEntry>>(init) ?? {}
    const e = sessionFrom(b)
    if (e) return err(400, 'BAD_REQUEST', e)
    const p: PlanEntry = {
      id: nextId(),
      date: b.date as string,
      sport: b.sport as PlanSport,
      title: clean(b.title, 120) as string,
      targetDurationS: b.targetDurationS ?? null,
      targetDistanceM: b.targetDistanceM ?? null,
      targetPaceMinSecPerKm: b.targetPaceMinSecPerKm ?? null,
      targetPaceMaxSecPerKm: b.targetPaceMaxSecPerKm ?? null,
      targetCadenceSpm: b.targetCadenceSpm ?? null,
      instructions: clean(b.instructions, 2000) ?? '',
      steps: Array.isArray(b.steps) ? b.steps : [],
      source: 'share',
      garmin: { status: 'none', workoutId: null, pushedAt: null, error: null },
      prescribedBy: { linkId: hit.share.id, label: hit.share.label, authorName: null },
      morph: null,
    }
    hit.data.plan.push(p)
    notify('plan', hit.share.id)
    return json(plannedWithMatch(hit.data, visibleItems(hit.data), todayISO()).find((x) => x.id === p.id), 201)
  })
  pub('PATCH', /^\/api\/public\/share\/([^/]+)\/plan\/(\d+)$/, ({ params, init }) => {
    const hit = linkFor(params[0], 'planning')
    if (hit instanceof Response) return hit
    const p = hit.data.plan.find((x) => x.id === Number(params[1]))
    if (!p) return err(404, 'NOT_FOUND', 'Séance introuvable.')
    if (!ownedBy(hit.share, p)) return err(403, 'SHARE_FORBIDDEN', 'Ce lien ne peut modifier que les séances qu’il a prescrites.')
    const b = parseBody<Partial<PlanEntry>>(init) ?? {}
    const e = sessionFrom(b, p)
    if (e) return err(400, 'BAD_REQUEST', e)
    const { id: _i, source: _s, garmin: _g, prescribedBy: _p, morph: _m, ...rest } = b
    Object.assign(p, rest)
    notify('plan', hit.share.id)
    return json(plannedWithMatch(hit.data, visibleItems(hit.data), todayISO()).find((x) => x.id === p.id))
  })
  pub('DELETE', /^\/api\/public\/share\/([^/]+)\/plan\/(\d+)$/, ({ params }) => {
    const hit = linkFor(params[0], 'planning')
    if (hit instanceof Response) return hit
    const p = hit.data.plan.find((x) => x.id === Number(params[1]))
    if (!p) return err(404, 'NOT_FOUND', 'Séance introuvable.')
    if (!ownedBy(hit.share, p)) return err(403, 'SHARE_FORBIDDEN', 'Ce lien ne peut supprimer que les séances qu’il a prescrites.')
    hit.data.plan = hit.data.plan.filter((x) => x.id !== p.id)
    notify('plan', hit.share.id)
    return noContent()
  })

  const eventError = (m: Partial<CalendarEventInput>): string | null => {
    if (!EVENT_KINDS.includes(m.kind as EventKind)) return 'Type de contrainte invalide.'
    if (!planWindow(m.startDate) || !planWindow(m.endDate) || (m.endDate as string) < (m.startDate as string)) return 'Dates hors de la période autorisée (J−7 à J+120) ou fin avant le début.'
    if (![1, 2, 3].includes(m.severity as number)) return 'Intensité invalide.'
    return null
  }
  pub('POST', /^\/api\/public\/share\/([^/]+)\/events$/, ({ params, init }) => {
    const hit = linkFor(params[0], 'planning')
    if (hit instanceof Response) return hit
    const b = parseBody<Partial<CalendarEventInput>>(init) ?? {}
    const e = eventError(b)
    if (e) return err(400, 'BAD_REQUEST', e)
    const ev: CalendarEvent = { id: nextId(), kind: b.kind as EventKind, startDate: b.startDate as string, endDate: b.endDate as string, severity: b.severity as 1 | 2 | 3, note: clean(b.note, 500) ?? '', prescribedBy: { linkId: hit.share.id, label: hit.share.label, authorName: null } }
    v31Of(hit.data).events.push(ev)
    notify('event', hit.share.id)
    return json(ev, 201)
  })
  pub('PATCH', /^\/api\/public\/share\/([^/]+)\/events\/(\d+)$/, ({ params, init }) => {
    const hit = linkFor(params[0], 'planning')
    if (hit instanceof Response) return hit
    const ev = v31Of(hit.data).events.find((x) => x.id === Number(params[1]))
    if (!ev) return err(404, 'NOT_FOUND', 'Contrainte introuvable.')
    if (!ownedBy(hit.share, ev)) return err(403, 'SHARE_FORBIDDEN', 'Ce lien ne peut modifier que les contraintes qu’il a déclarées.')
    const b = parseBody<Partial<CalendarEventInput>>(init) ?? {}
    const e = eventError({ ...ev, ...b })
    if (e) return err(400, 'BAD_REQUEST', e)
    Object.assign(ev, { kind: b.kind ?? ev.kind, startDate: b.startDate ?? ev.startDate, endDate: b.endDate ?? ev.endDate, severity: b.severity ?? ev.severity, note: b.note !== undefined ? (clean(b.note, 500) ?? '') : ev.note })
    notify('event', hit.share.id)
    return json(ev)
  })
  pub('DELETE', /^\/api\/public\/share\/([^/]+)\/events\/(\d+)$/, ({ params }) => {
    const hit = linkFor(params[0], 'planning')
    if (hit instanceof Response) return hit
    const st = v31Of(hit.data)
    const ev = st.events.find((x) => x.id === Number(params[1]))
    if (!ev) return err(404, 'NOT_FOUND', 'Contrainte introuvable.')
    if (!ownedBy(hit.share, ev)) return err(403, 'SHARE_FORBIDDEN', 'Ce lien ne peut supprimer que les contraintes qu’il a déclarées.')
    st.events = st.events.filter((x) => x.id !== ev.id)
    notify('event', hit.share.id)
    return noContent()
  })

  /* --- athlete: physio/coach notes --- */
  route('GET', /^\/api\/share\/notes$/, ({ me, data, url }) => {
    ensureV42(data, me.id)
    const active = url.searchParams.get('active') === '1'
    const notes = active ? [...activeDirectives(data, me.id), ...v42Of(data).notes.filter((n) => n.kind === 'remark')] : v42Of(data).notes
    return json([...notes].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)))
  })
  route('DELETE', /^\/api\/share\/notes\/(\d+)$/, ({ data, params }) => {
    const st = v42Of(data)
    const before = st.notes.length
    st.notes = st.notes.filter((n) => n.id !== Number(params[0]))
    return before === st.notes.length ? err(404, 'NOT_FOUND', 'Note introuvable.') : noContent()
  })

  /* --- Smart Workout Morphing --- */
  const entry = (data: UserData, id: string) => data.plan.find((x) => x.id === Number(id)) ?? null
  route('GET', /^\/api\/plan\/(\d+)\/morph$/, ({ me, data, params }) => {
    ensureV42(data, me.id)
    const p = entry(data, params[0])
    return p ? json(previewFor(data, me.id, p)) : err(404, 'NOT_FOUND', 'Séance introuvable.')
  })
  route('POST', /^\/api\/plan\/(\d+)\/morph\/refine$/, ({ me, data, params, init }) => {
    const p = entry(data, params[0])
    if (!p) return err(404, 'NOT_FOUND', 'Séance introuvable.')
    if (!llmConfigured(data)) return err(412, 'PROVIDER_NOT_CONNECTED', 'Aucun connecteur IA configuré.')
    const base = previewFor(data, me.id, p)
    if (!base.proposal) return err(409, 'CONFLICT', 'Aucune contrainte : rien à affiner.')
    const note = clean(parseBody<{ note?: string }>(init)?.note, 500)
    const job = newJob(me.id, 'llm.morph_refine', 'Affinage IA de la séance adaptée')
    v4Of(data, me.id).jobs.unshift(job)
    const ref = String(p.id)
    const chunks = [
      'Je garde la structure proposée par les règles. ',
      'Avec la gêne au genou et la chaleur, je déplace l’effort vers le ressenti : ',
      'échauffement articulaire de 8 min (montées de genoux, talons-fesses), ',
      note ? `et je tiens compte de ta remarque (« ${note} »). ` : 'puis course continue en aisance respiratoire. ',
      'Arrêt immédiat si la douleur dépasse 3/10.',
    ]
    const proposal = { ...base.proposal, instructions: 'Échauffement articulaire 8 min, puis course continue en aisance respiratoire. Arrêt si douleur > 3/10. Boire 300 ml avant de partir.' }
    const refined: MorphPreview = {
      ...base,
      proposal,
      changes: [
        ...base.changes.filter((c) => c.field !== 'instructions'),
        { field: 'instructions', stepIndex: null, before: base.original.instructions || '—', after: proposal.instructions, reason: 'Affinage IA : échauffement articulaire, critère d’arrêt et hydratation.' },
      ],
    }
    chunks.forEach((delta, i) => setTimeout(() => emitStreamEvent('llm.token', { jobId: job.id, kind: 'morph_refine', ref, delta }), 60 * (i + 1)))
    setTimeout(() => {
      job.state = 'completed'
      job.progressPct = 100
      job.updatedAt = isoNow()
      emitStreamEvent('llm.done', { jobId: job.id, kind: 'morph_refine', ref, morphPreview: refined })
      emitStreamEvent('job.updated', { ...job })
    }, 60 * (chunks.length + 1) + 30)
    return json(job, 202)
  })
  route('POST', /^\/api\/plan\/(\d+)\/morph\/apply$/, ({ me, data, params, init }) => {
    const p = entry(data, params[0])
    if (!p) return err(404, 'NOT_FOUND', 'Séance introuvable.')
    const proposal = parseBody<{ proposal?: PlannedSession }>(init)?.proposal
    if (!proposal || proposal.id !== p.id) return err(400, 'BAD_REQUEST', 'Proposition invalide pour cette séance.')
    const reasons = constraintsFor(data, me.id, p).map((c) => c.label)
    const original = p.morph?.original ?? originalOf(p)
    for (const k of MORPH_FIELDS) (p as unknown as Record<string, unknown>)[k] = proposal[k]
    p.morph = { at: isoNow(), reasons, original }
    return json(plannedWithMatch(data, visibleItems(data), todayISO()).find((x) => x.id === p.id))
  })
  route('POST', /^\/api\/plan\/(\d+)\/morph\/revert$/, ({ data, params }) => {
    const p = entry(data, params[0])
    if (!p) return err(404, 'NOT_FOUND', 'Séance introuvable.')
    if (!p.morph) return err(409, 'CONFLICT', 'Cette séance n’a pas été adaptée.')
    Object.assign(p, p.morph.original)
    p.morph = null
    return json(plannedWithMatch(data, visibleItems(data), todayISO()).find((x) => x.id === p.id))
  })

  /* --- admin: atomic backups + S3 --- */
  const NAME_RE = /^stride-\d{8}T\d{6}Z-[0-9A-Za-z.+-]+\.sqlite\.gz$/
  const listing = (): BackupList => {
    const g = globalState()
    const synced = g.backups.filter((b) => b.s3.status === 'synced' && b.s3.syncedAt).map((b) => b.s3.syncedAt as string)
    return {
      items: [...g.backups].sort((a, b) => b.createdAt.localeCompare(a.createdAt)),
      s3: publicS3(g.s3),
      lastSuccess: { local: g.backups.map((b) => b.createdAt).sort().at(-1) ?? null, s3: synced.sort().at(-1) ?? null },
      nextScheduledAt: `${addDays(todayISO(), 1)}T03:00:00Z`,
    }
  }
  const pushTo = (b: BackupItem, done: () => void) => {
    b.s3 = { status: 'pending', key: `${globalState().s3.prefix}${b.name}`, syncedAt: null, error: null }
    return () => {
      if (has('s3err')) {
        b.s3 = { ...b.s3, status: 'error', error: 'Envoi refusé par le stockage S3 (403 SignatureDoesNotMatch). Vérifie la clé secrète.' }
        done()
        return 'Envoi S3 refusé (403 SignatureDoesNotMatch).'
      }
      b.s3 = { ...b.s3, status: 'synced', syncedAt: isoNow() }
      done()
      return null
    }
  }

  route('GET', /^\/api\/admin\/backups$/, () => json(listing()))
  route('POST', /^\/api\/admin\/backups$/, ({ me, init }) => {
    const upload = parseBody<{ upload?: boolean }>(init)?.upload === true
    const g = globalState()
    if (upload && !s3Ready(g.s3)) return err(409, 'CONFLICT', 'Configure la cible S3 avant d’envoyer une sauvegarde.')
    const job = newJob(me.id, 'backup.run', upload ? 'Sauvegarde locale + envoi S3' : 'Sauvegarde locale')
    const now = new Date()
    const stamp = now.toISOString().replace(/[-:]/g, '').replace(/\.\d{3}Z$/, 'Z')
    const name = `stride-${stamp}-4.2.0.sqlite.gz`
    playJob(job, upload ? [35, 70, 90] : [35, 70], () => {
      const item: BackupItem = { name, sizeBytes: 18_935_296, sha256: sha256Of(name), createdAt: now.toISOString(), trigger: 'manual', s3: { status: 'none', key: null, syncedAt: null, error: null } }
      g.backups.unshift(item)
      const regular = g.backups.filter((b) => b.trigger !== 'pre_migration')
      if (regular.length > 7) g.backups = g.backups.filter((b) => !regular.slice(7).includes(b))
      if (!upload) return null
      return pushTo(item, () => undefined)()
    })
    return json(job, 202)
  })
  route('GET', /^\/api\/admin\/backups\/([^/]+)\/download$/, ({ params }) => {
    const name = decodeURIComponent(params[0])
    if (!NAME_RE.test(name)) return err(400, 'BAD_REQUEST', 'Nom de sauvegarde invalide.')
    if (!globalState().backups.some((b) => b.name === name)) return err(404, 'NOT_FOUND', 'Sauvegarde introuvable.')
    const bytes = new Uint8Array([0x1f, 0x8b, 0x08, 0, 0, 0, 0, 0, 0, 0x03, 0x53, 0x51, 0x4c, 0x69, 0x74, 0x65])
    return new Response(bytes, { headers: { 'Content-Type': 'application/gzip', 'Content-Disposition': `attachment; filename="${name}"` } })
  })
  route('POST', /^\/api\/admin\/backups\/([^/]+)\/upload$/, ({ me, params }) => {
    const name = decodeURIComponent(params[0])
    if (!NAME_RE.test(name)) return err(400, 'BAD_REQUEST', 'Nom de sauvegarde invalide.')
    const g = globalState()
    const b = g.backups.find((x) => x.name === name)
    if (!b) return err(404, 'NOT_FOUND', 'Sauvegarde introuvable.')
    if (!s3Ready(g.s3)) return err(409, 'CONFLICT', 'Configure la cible S3 avant d’envoyer une sauvegarde.')
    const job = newJob(me.id, 'backup.upload', `Envoi S3 de ${name}`)
    const finish = pushTo(b, () => undefined)
    playJob(job, [40, 80], finish)
    return json(job, 202)
  })
  route('DELETE', /^\/api\/admin\/backups\/([^/]+)$/, ({ params }) => {
    const name = decodeURIComponent(params[0])
    if (!NAME_RE.test(name)) return err(400, 'BAD_REQUEST', 'Nom de sauvegarde invalide.')
    const g = globalState()
    if (!g.backups.some((b) => b.name === name)) return err(404, 'NOT_FOUND', 'Sauvegarde introuvable.')
    g.backups = g.backups.filter((b) => b.name !== name)
    return noContent()
  })
  route('GET', /^\/api\/admin\/backups\/s3$/, () => json(publicS3(globalState().s3)))
  const s3Error = (c: Partial<S3ConfigInput>): string | null => {
    const any = Boolean(c.endpoint || c.bucket || c.accessKeyId)
    if (!any) return null
    if (!/^https?:\/\/[^\s/]+/.test(c.endpoint ?? '')) return 'Endpoint invalide : URL http(s) attendue.'
    if (!/^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$/.test(c.bucket ?? '')) return 'Nom de bucket invalide (3 à 63 caractères : minuscules, chiffres, points, tirets).'
    if (!c.region?.trim()) return 'Indique la région (ex. fr-par, us-east-1, auto).'
    if (!c.accessKeyId?.trim()) return 'Indique l’identifiant de clé d’accès.'
    if (!Number.isInteger(c.retentionDays) || (c.retentionDays as number) < 1 || (c.retentionDays as number) > 3650) return 'Rétention distante : 1 à 3650 jours.'
    return null
  }
  route('PUT', /^\/api\/admin\/backups\/s3$/, ({ init }) => {
    const b = parseBody<S3ConfigInput>(init)
    if (!b) return err(400, 'BAD_REQUEST', 'Configuration invalide.')
    const e = s3Error(b)
    if (e) return err(400, 'BAD_REQUEST', e)
    const g = globalState()
    g.s3 = {
      endpoint: b.endpoint.trim(),
      region: b.region.trim(),
      bucket: b.bucket.trim(),
      prefix: b.prefix.trim(),
      accessKeyId: b.accessKeyId.trim(),
      secretKey: b.secretKey ? b.secretKey : g.s3.secretKey,
      secretKeySet: Boolean(b.secretKey || g.s3.secretKey),
      pathStyle: b.pathStyle === true,
      autoUpload: b.autoUpload === true,
      retentionDays: b.retentionDays,
    }
    return json(publicS3(g.s3))
  })
  route('POST', /^\/api\/admin\/backups\/s3\/test$/, ({ init }) => {
    const input = parseBody<{ config?: S3ConfigInput }>(init)?.config
    const stored = globalState().s3
    const c = input ? { ...input, secretKey: input.secretKey || stored.secretKey } : stored
    if (!c.endpoint || !c.bucket) return json({ ok: false, latencyMs: 0, error: 'Renseigne au moins l’endpoint et le bucket.' })
    const e = s3Error(c)
    if (e) return json({ ok: false, latencyMs: 0, error: e })
    if (!c.secretKey) return json({ ok: false, latencyMs: 0, error: 'Clé secrète manquante.' })
    if (has('s3err')) return json({ ok: false, latencyMs: 212, error: 'Accès refusé par le bucket (403 SignatureDoesNotMatch).' })
    return json({ ok: true, latencyMs: 84 })
  })
}

/* ---------------------------------- Public activity detail ---------------------------------- */

function publicDetail(data: UserData, share: StoredShare, it: CatalogItem): PublicActivityDetail {
  const v = enrichActivityView(buildActivityView(it, data), it, data)
  const a = v.activity
  const notes = notesOf(data, it.id)
  const shoe = notes.shoeId ? data.shoes.find((s) => s.id === notes.shoeId) ?? null : null
  const remark = v42Of(data).notes.find((n) => n.kind === 'remark' && n.linkId === share.id && n.activityId === it.id) ?? null
  const laps = (v.segments.primary === 'blocks' ? v.segments.blocks : v.segments.laps).slice(0, 30)
  const notesOn = share.sections.includes('notes')
  return {
    id: a.id,
    date: a.date,
    startLocal: a.startLocal,
    name: share.anonymize ? a.typeLabel : a.name,
    sport: a.sport,
    typeLabel: a.typeLabel,
    distanceM: a.distanceM,
    durationS: a.durationS,
    avgPaceSecPerKm: a.avgPaceSecPerKm,
    gapSecPerKm: a.gapSecPerKm,
    avgHR: a.avgHR,
    maxHR: a.maxHR,
    efSpeed: v.efficiency?.eligible ? v.efficiency.efSpeed : null,
    avgCadenceSpm: v.biomechanics.cadenceAvgSpm ?? a.avgCadenceSpm,
    gctMs: v.biomechanics.groundContactTimeMs,
    verticalOscillationCm: v.biomechanics.verticalOscillationCm,
    balanceLeftPct: v.biomechanics.gctBalance.leftPct,
    balanceRightPct: v.biomechanics.gctBalance.rightPct,
    elevGainM: a.elevGainM,
    elevLossM: a.elevLossM,
    impactLoad: v.load.impactLoad,
    trainingLoad: a.trainingLoad,
    weather: v.weather ? { temperatureC: v.weather.temperatureC, apparentC: v.weather.apparentC, humidityPct: v.weather.humidityPct, windKph: v.weather.windKph, description: v.weather.description } : null,
    laps: laps.map((l) => ({ index: l.index, label: l.label, durationS: l.durationS, distanceM: l.distanceM, paceSecPerKm: l.paceSecPerKm, avgHR: l.avgHR, avgCadenceSpm: l.avgCadenceSpm, gctMs: l.gctMs, elevGainM: l.elevGainM, elevLossM: l.elevLossM })),
    shoe: shoe ? { name: shoe.name, dropMm: shoe.dropMm, distanceKm: shoe.distanceKm } : null,
    painDuring: notesOn ? notes.painDuring : null,
    painNextMorning: notesOn ? notes.painNextMorning : null,
    remark,
  }
}
