// 4.0.0 mock endpoints (§13): background jobs + synthetic SSE streaming, macro/meso/micro training cycles,
// anatomical body stress map & biomechanical correlations, and read-only physio/coach share links.
// Flag: sparse (too little data: correlations flagged insufficientData, share-view EF / impact / joint index null).

import type {
  ApiErrorBody,
  BodyCorrelationFactor,
  BodyMapOverview,
  BodySide,
  BodyStressEntry,
  BodyStressInput,
  BodyZone,
  BodyZoneSummary,
  CycleActuals,
  CyclePlanStatus,
  ErrorCode,
  JobStatus,
  LLMAnalysis,
  PhaseType,
  PublicActivityItem,
  PublicLoadPoint,
  PublicShareView,
  ShareLink,
  ShareLinkInput,
  TrainingCycleOverview,
  TrainingPhase,
  TrainingPhaseInput,
  TrainingPlan,
  TrainingPlanInput,
  TrainingWeek,
  TrainingWeekInput,
} from '../api/types'
import { ALL_BODY_ZONES, BODY_ZONE_META } from '../components/bodymap/bodyMapMeta'
import { addDays, diffDays, isNum, mondayOf, todayISO } from '../lib/format'
import { emitStreamEvent } from '../state/events'
import type { CatalogItem } from './catalog'
import { db, nextId, userData, type MockUser, type UserData } from './db'
import { avgGct, gapSummary, loadRatio, notesOf, plannedWithMatch, sourceMode, summarize, visibleItems } from './domain'
import { has } from './flags'
import { enrichActivityView, enrichDayView, llmConfigured } from './v3'
import { buildActivityView, buildDayView } from './view'

interface Ctx {
  url: URL
  params: string[]
  init?: RequestInit
  me: MockUser
  data: UserData
}

type PublicCtx = Omit<Ctx, 'me' | 'data'>
type RouteFn = (method: string, re: RegExp, handler: (c: Ctx) => Response | Promise<Response>) => void
type PubFn = (method: string, re: RegExp, handler: (c: PublicCtx) => Response) => void

interface StoredWeekOverride {
  id: number
  planId: number | null
  phaseId: number | null
  weekStart: string
  isAssimilation: boolean
  targetVolumeKm: number | null
  targetLoad: number | null
  targetDurationS: number | null
  note: string
}

interface StoredPhase {
  id: number
  planId: number | null
  name: string
  phaseType: PhaseType
  orderIndex: number
  startDate: string
  endDate: string
  focus: string
  weeklyVolumeKm: number | null
  targetWeeklyLoad: number | null
  targetWeeklyDurationS: number | null
  note: string
}

interface StoredShare extends ShareLink {
  token: string
}

export interface V4Data {
  jobs: JobStatus[]
  plans: TrainingPlan[]
  phases: StoredPhase[]
  weeks: StoredWeekOverride[]
  bodyEntries: BodyStressEntry[]
  shares: StoredShare[]
}

const json = (body: unknown, status = 200, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json; charset=utf-8', ...headers },
  })
const noContent = (status = 204) => new Response(null, { status })
const err = (status: number, code: ErrorCode, error: string) =>
  json({ error, code } satisfies ApiErrorBody, status)
const isoNow = () => new Date().toISOString()
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/

function parseBody<T>(init?: RequestInit): T | null {
  if (!init?.body || typeof init.body !== 'string') return null
  try {
    return JSON.parse(init.body) as T
  } catch {
    return null
  }
}

const PHASE_TYPES: PhaseType[] = [
  'foundation',
  'build',
  'threshold',
  'specific',
  'assimilation',
  'taper',
  'recovery',
]

function scaffoldPhasesForPlan(
  plan: TrainingPlan,
  peakKm: number,
  peakLoad: number,
): { phases: StoredPhase[]; weeks: StoredWeekOverride[] } {
  const startMon = mondayOf(plan.startDate)
  const endMon = mondayOf(plan.endDate)
  const totalWeeks = Math.max(1, Math.floor(diffDays(startMon, endMon) / 7) + 1)

  const phases: StoredPhase[] = []
  const weeks: StoredWeekOverride[] = []

  const specs: { name: string; type: PhaseType; focus: string; share: number; volFactor: number }[] =
    totalWeeks >= 6
      ? [
          { name: 'Fondation aérobie', type: 'foundation', focus: 'Endurance fondamentale & régularité', share: 0.34, volFactor: 0.78 },
          { name: 'Développement & Seuil', type: 'build', focus: 'Volume progressif & tempo contrôlé', share: 0.34, volFactor: 0.92 },
          { name: 'Spécifique objectif', type: 'specific', focus: 'Allure cible & économie de course', share: 0.22, volFactor: 1.0 },
          { name: 'Affûtage', type: 'taper', focus: 'Fraîcheur neuromusculaire avant course', share: 0.1, volFactor: 0.62 },
        ]
      : [
          { name: 'Bloc principal', type: 'specific', focus: 'Travail spécifique objectif', share: 0.75, volFactor: 0.95 },
          { name: 'Affûtage', type: 'taper', focus: 'Assimilation pré-compétition', share: 0.25, volFactor: 0.65 },
        ]

  let cursorWeek = 0
  specs.forEach((sp, idx) => {
    const remaining = totalWeeks - cursorWeek
    if (remaining <= 0) return
    const count =
      idx === specs.length - 1
        ? remaining
        : Math.max(1, Math.min(remaining - (specs.length - 1 - idx), Math.round(totalWeeks * sp.share)))
    const pStart = addDays(startMon, cursorWeek * 7)
    const pEnd = addDays(pStart, count * 7 - 1)
    const baseKm = Math.round(peakKm * sp.volFactor)
    const baseLoad = Math.round(peakLoad * sp.volFactor)
    const phase: StoredPhase = {
      id: nextId(),
      planId: plan.id,
      name: sp.name,
      phaseType: sp.type,
      orderIndex: idx,
      startDate: pStart < plan.startDate ? plan.startDate : pStart,
      endDate: pEnd > plan.endDate ? plan.endDate : pEnd,
      focus: sp.focus,
      weeklyVolumeKm: baseKm,
      targetWeeklyLoad: baseLoad,
      targetWeeklyDurationS: Math.round(baseKm * 330),
      note: '',
    }
    phases.push(phase)

    for (let w = 0; w < count; w++) {
      const globalWeek = cursorWeek + w + 1
      const wStart = addDays(pStart, w * 7)
      const isAssim = sp.type !== 'taper' && globalWeek % 4 === 0
      const factor = isAssim ? 0.74 : 0.94 + Math.min(0.1, w * 0.04)
      const wkKm = Math.round(baseKm * factor * 10) / 10
      const wkLoad = Math.round(baseLoad * factor)
      weeks.push({
        id: nextId(),
        planId: plan.id,
        phaseId: phase.id,
        weekStart: wStart,
        isAssimilation: isAssim,
        targetVolumeKm: wkKm,
        targetLoad: wkLoad,
        targetDurationS: Math.round(wkKm * 330),
        note: isAssim ? 'Semaine d’assimilation (-25 % volume)' : '',
      })
    }
    cursorWeek += count
  })

  return { phases, weeks }
}

function v4Of(data: UserData, userId = 1): V4Data {
  if (!data.v4) {
    const today = todayISO()
    const seeded = !has('empty') && data.shoes.length > 0
    const plans: TrainingPlan[] = []
    const phases: StoredPhase[] = []
    const weeks: StoredWeekOverride[] = []
    const bodyEntries: BodyStressEntry[] = []
    const shares: StoredShare[] = []
    const jobs: JobStatus[] = []

    if (seeded) {
      const raceGoal = data.goals.find((g) => g.kind === 'race') ?? data.goals[0] ?? null
      const startMon = mondayOf(addDays(today, -35))
      const endSun = addDays(startMon, 12 * 7 - 1)
      const plan: TrainingPlan = {
        id: nextId(),
        goalId: raceGoal?.id ?? null,
        name: 'Prépa Semi-Marathon d’Automne',
        sport: 'run',
        startDate: startMon,
        endDate: endSun,
        targetEventLabel: raceGoal?.title ?? 'Semi-marathon · Objectif 1h38',
        status: 'active',
        note: 'Progression 3:1 avec surveillance rotulienne et contrôle du D−.',
        createdAt: new Date(Date.now() - 35 * 86_400_000).toISOString(),
        updatedAt: isoNow(),
      }
      plans.push(plan)
      const sc = scaffoldPhasesForPlan(plan, 54, 410)
      phases.push(...sc.phases)
      weeks.push(...sc.weeks)

      const recentRuns = visibleItems(data, today).filter((it) => it.isRunning)
      const r0 = recentRuns[0]
      const r2 = recentRuns[2] ?? recentRuns[0]
      const r4 = recentRuns[4] ?? recentRuns[1]

      bodyEntries.push(
        {
          id: nextId(),
          date: r0?.date ?? addDays(today, -1),
          activityId: r0?.id ?? null,
          activityName: r0?.name ?? null,
          zone: 'knee_pfps',
          zoneLabel: BODY_ZONE_META.knee_pfps.label,
          side: 'left',
          painScore: 4,
          coldNextDayScore: 3,
          note: 'Gêne rotulienne externe en fin de descente, calme après 20 min de repos.',
          createdAt: isoNow(),
          updatedAt: isoNow(),
        },
        {
          id: nextId(),
          date: r2?.date ?? addDays(today, -6),
          activityId: r2?.id ?? null,
          activityName: r2?.name ?? null,
          zone: 'knee_pfps',
          zoneLabel: BODY_ZONE_META.knee_pfps.label,
          side: 'left',
          painScore: 3.5,
          coldNextDayScore: 2.5,
          note: 'Sensibilité légère aux premiers escaliers le lendemain matin.',
          createdAt: isoNow(),
          updatedAt: isoNow(),
        },
        {
          id: nextId(),
          date: r4?.date ?? addDays(today, -12),
          activityId: r4?.id ?? null,
          activityName: r4?.name ?? null,
          zone: 'achilles',
          zoneLabel: BODY_ZONE_META.achilles.label,
          side: 'right',
          painScore: 2,
          coldNextDayScore: 2.5,
          note: 'Raideur matinale modérée pendant 5 minutes au lever après séance tempo.',
          createdAt: isoNow(),
          updatedAt: isoNow(),
        },
      )

      if (userId === 1) {
        shares.push(
          {
            id: nextId(),
            label: 'Kiné du sport',
            token: 'demo-kine',
            tokenPrefix: 'demo-kin',
            url: '/share/demo-kine',
            expiresAt: new Date(Date.now() + 30 * 86_400_000).toISOString(),
            revoked: false,
            revokedAt: null,
            accessCount: 4,
            lastAccessedAt: new Date(Date.now() - 2 * 86_400_000).toISOString(),
            createdAt: new Date(Date.now() - 10 * 86_400_000).toISOString(),
          },
          {
            id: nextId(),
            label: 'Ancien bilan été',
            token: 'expired-token',
            tokenPrefix: 'expired-',
            url: '/share/expired-token',
            expiresAt: new Date(Date.now() - 5 * 86_400_000).toISOString(),
            revoked: false,
            revokedAt: null,
            accessCount: 2,
            lastAccessedAt: new Date(Date.now() - 12 * 86_400_000).toISOString(),
            createdAt: new Date(Date.now() - 45 * 86_400_000).toISOString(),
          },
        )
      }

      jobs.push({
        id: 'job_seed_sync_1',
        userId,
        kind: 'sync.provider',
        queue: 'default',
        state: 'completed',
        label: 'Synchro Garmin & Strava terminée',
        progressPct: 100,
        processed: 4,
        total: 4,
        attempts: 1,
        maxAttempts: 4,
        error: null,
        result: { newActivities: 2 },
        createdAt: new Date(Date.now() - 180_000).toISOString(),
        updatedAt: new Date(Date.now() - 175_000).toISOString(),
      })
    }

    const v4State: V4Data = { jobs, plans, phases, weeks, bodyEntries, shares }
    data.v4 = v4State
    return v4State
  }
  return data.v4 as V4Data
}

/* ---------------------------------- Cycles computation ---------------------------------- */

function computeActualsForRange(
  data: UserData,
  from: string,
  to: string,
  targetVolumeKm: number | null,
  targetLoad: number | null,
  targetDurationS: number | null,
) {
  const today = todayISO()
  const items = visibleItems(data, today)
  const planned = plannedWithMatch(data, items, today, from, to)
  const doneInRange = items.filter((it) => it.date >= from && it.date <= to)

  const plannedSessions = planned.filter((p) => p.sport !== 'rest')
  const sumPlannedKm =
    Math.round(
      plannedSessions.reduce((acc, p) => acc + (p.targetDistanceM ?? 0) / 1000, 0) * 10,
    ) / 10
  const plannedVolumeKm = targetVolumeKm ?? sumPlannedKm
  const actualVolumeKm =
    Math.round(
      doneInRange
        .filter((it) => it.isRunning)
        .reduce((acc, it) => acc + (it.base.distanceM ?? 0) / 1000, 0) * 10,
    ) / 10

  const sumPlannedDur = plannedSessions.reduce((acc, p) => acc + (p.targetDurationS ?? 0), 0)
  const plannedDurationS = targetDurationS ?? sumPlannedDur
  const actualDurationS = doneInRange.reduce((acc, it) => acc + (it.base.durationS ?? 0), 0)

  const estPlannedLoad = Math.round(plannedVolumeKm * 7.5)
  const plannedLoadVal = targetLoad ?? estPlannedLoad
  const actualLoad = Math.round(
    doneInRange.reduce((acc, it) => acc + (it.base.trainingLoad ?? ((it.base.durationS ?? 0) / 60) * 1.4), 0),
  )

  const sessionsCompleted =
    plannedSessions.filter((p) => p.match?.status === 'done' || p.match?.status === 'partial').length ||
    Math.min(plannedSessions.length, doneInRange.length)

  const actuals: CycleActuals = {
    plannedVolumeKm,
    actualVolumeKm,
    volumeCompletionPct:
      plannedVolumeKm > 0 ? Math.round((actualVolumeKm / plannedVolumeKm) * 1000) / 10 : null,
    plannedLoad: plannedLoadVal,
    actualLoad,
    loadCompletionPct:
      plannedLoadVal > 0 ? Math.round((actualLoad / plannedLoadVal) * 1000) / 10 : null,
    plannedDurationS,
    actualDurationS,
    sessionsPlanned: plannedSessions.length,
    sessionsCompleted,
  }

  return { actuals, workouts: planned }
}

function buildCyclesOverview(
  data: UserData,
  userId = 1,
  requestedPlanId?: number | null,
): TrainingCycleOverview {
  const st = v4Of(data, userId)
  const activePlan =
    (requestedPlanId ? st.plans.find((p) => p.id === requestedPlanId) : null) ??
    st.plans.find((p) => p.status === 'active') ??
    st.plans[0] ??
    null

  const rawPhases = st.phases
    .filter((ph) => (activePlan ? ph.planId === activePlan.id : ph.planId === null))
    .sort((a, b) => a.orderIndex - b.orderIndex || a.startDate.localeCompare(b.startDate))

  const planStartMon = activePlan ? mondayOf(activePlan.startDate) : null

  const builtPhases: TrainingPhase[] = rawPhases.map((ph) => {
    const startMon = mondayOf(ph.startDate)
    const endMon = mondayOf(ph.endDate)
    const count = Math.max(1, Math.floor(diffDays(startMon, endMon) / 7) + 1)
    const phaseWeeks: TrainingWeek[] = []

    for (let i = 0; i < count; i++) {
      const wStart = addDays(startMon, i * 7)
      const wEnd = addDays(wStart, 6)
      const ov = st.weeks.find(
        (w) => w.weekStart === wStart && (activePlan ? w.planId === activePlan.id : true),
      )
      const targetKm = ov?.targetVolumeKm ?? ph.weeklyVolumeKm
      const targetLd = ov?.targetLoad ?? ph.targetWeeklyLoad
      const targetDur = ov?.targetDurationS ?? ph.targetWeeklyDurationS
      const { actuals, workouts } = computeActualsForRange(
        data,
        wStart,
        wEnd,
        targetKm,
        targetLd,
        targetDur,
      )
      const weekNumber = planStartMon ? Math.max(1, Math.floor(diffDays(planStartMon, wStart) / 7) + 1) : i + 1
      phaseWeeks.push({
        id: ov?.id ?? 100_000 + ph.id * 100 + i,
        planId: activePlan?.id ?? null,
        phaseId: ph.id,
        weekStart: wStart,
        weekEnd: wEnd,
        weekNumber,
        isAssimilation: ov?.isAssimilation ?? ph.phaseType === 'assimilation',
        targetVolumeKm: targetKm,
        targetLoad: targetLd,
        targetDurationS: targetDur,
        note: ov?.note ?? '',
        actuals,
        workouts,
      })
    }

    const pPlannedKm = Math.round(phaseWeeks.reduce((s, w) => s + w.actuals.plannedVolumeKm, 0) * 10) / 10
    const pActualKm = Math.round(phaseWeeks.reduce((s, w) => s + w.actuals.actualVolumeKm, 0) * 10) / 10
    const pPlannedLd = Math.round(phaseWeeks.reduce((s, w) => s + w.actuals.plannedLoad, 0))
    const pActualLd = Math.round(phaseWeeks.reduce((s, w) => s + w.actuals.actualLoad, 0))
    const pPlannedDur = phaseWeeks.reduce((s, w) => s + w.actuals.plannedDurationS, 0)
    const pActualDur = phaseWeeks.reduce((s, w) => s + w.actuals.actualDurationS, 0)
    const pSessPlan = phaseWeeks.reduce((s, w) => s + w.actuals.sessionsPlanned, 0)
    const pSessComp = phaseWeeks.reduce((s, w) => s + w.actuals.sessionsCompleted, 0)

    return {
      ...ph,
      actuals: {
        plannedVolumeKm: pPlannedKm,
        actualVolumeKm: pActualKm,
        volumeCompletionPct: pPlannedKm > 0 ? Math.round((pActualKm / pPlannedKm) * 1000) / 10 : null,
        plannedLoad: pPlannedLd,
        actualLoad: pActualLd,
        loadCompletionPct: pPlannedLd > 0 ? Math.round((pActualLd / pPlannedLd) * 1000) / 10 : null,
        plannedDurationS: pPlannedDur,
        actualDurationS: pActualDur,
        sessionsPlanned: pSessPlan,
        sessionsCompleted: pSessComp,
      },
      weeks: phaseWeeks,
    }
  })

  const coveredStarts = new Set(builtPhases.flatMap((p) => p.weeks.map((w) => w.weekStart)))
  const standaloneWeeks: TrainingWeek[] = st.weeks
    .filter((ov) => (activePlan ? ov.planId === activePlan.id : ov.planId === null) && !coveredStarts.has(ov.weekStart))
    .sort((a, b) => a.weekStart.localeCompare(b.weekStart))
    .map((ov, idx) => {
      const wEnd = addDays(ov.weekStart, 6)
      const { actuals, workouts } = computeActualsForRange(
        data,
        ov.weekStart,
        wEnd,
        ov.targetVolumeKm,
        ov.targetLoad,
        ov.targetDurationS,
      )
      return {
        id: ov.id,
        planId: ov.planId,
        phaseId: ov.phaseId,
        weekStart: ov.weekStart,
        weekEnd: wEnd,
        weekNumber: idx + 1,
        isAssimilation: ov.isAssimilation,
        targetVolumeKm: ov.targetVolumeKm,
        targetLoad: ov.targetLoad,
        targetDurationS: ov.targetDurationS,
        note: ov.note,
        actuals,
        workouts,
      }
    })

  const allWeeks = [...builtPhases.flatMap((p) => p.weeks), ...standaloneWeeks]
  const totPlannedKm = Math.round(allWeeks.reduce((s, w) => s + w.actuals.plannedVolumeKm, 0) * 10) / 10
  const totActualKm = Math.round(allWeeks.reduce((s, w) => s + w.actuals.actualVolumeKm, 0) * 10) / 10
  const totPlannedLd = Math.round(allWeeks.reduce((s, w) => s + w.actuals.plannedLoad, 0))
  const totActualLd = Math.round(allWeeks.reduce((s, w) => s + w.actuals.actualLoad, 0))

  return {
    plans: st.plans,
    activePlan,
    phases: builtPhases,
    standaloneWeeks,
    totals: {
      plannedVolumeKm: totPlannedKm,
      actualVolumeKm: totActualKm,
      volumeCompletionPct: totPlannedKm > 0 ? Math.round((totActualKm / totPlannedKm) * 1000) / 10 : null,
      plannedLoad: totPlannedLd,
      actualLoad: totActualLd,
      loadCompletionPct: totPlannedLd > 0 ? Math.round((totActualLd / totPlannedLd) * 1000) / 10 : null,
      plannedDurationS: allWeeks.reduce((s, w) => s + w.actuals.plannedDurationS, 0),
      actualDurationS: allWeeks.reduce((s, w) => s + w.actuals.actualDurationS, 0),
      sessionsPlanned: allWeeks.reduce((s, w) => s + w.actuals.sessionsPlanned, 0),
      sessionsCompleted: allWeeks.reduce((s, w) => s + w.actuals.sessionsCompleted, 0),
    },
  }
}

/* ---------------------------------- Body Map computation ---------------------------------- */

const deltaPct = (value: number, baseline: number) => Math.round((value / baseline - 1) * 1000) / 10

const PRIMARY_FACTOR: Record<BodyZone, Pick<BodyCorrelationFactor, 'factor' | 'factorLabel' | 'unit'>> = {
  knee_pfps: { factor: 'elev_loss', factorLabel: 'Dénivelé négatif (D−)', unit: 'm' },
  achilles: { factor: 'shoe_drop', factorLabel: 'Drop chaussure', unit: 'mm' },
  plantar_fascia: { factor: 'shoe_drop', factorLabel: 'Drop chaussure', unit: 'mm' },
  shin_splints: { factor: 'gct', factorLabel: 'Temps de contact au sol (GCT)', unit: 'ms' },
  iliotibial_band: { factor: 'gct', factorLabel: 'Temps de contact au sol (GCT)', unit: 'ms' },
  hamstrings_calves: { factor: 'gct', factorLabel: 'Temps de contact au sol (GCT)', unit: 'ms' },
}

// Too few episodes to compare symptomatic and reference runs: the backend sends nulls, never estimates.
function insufficientFactor(z: BodyZoneSummary): BodyCorrelationFactor {
  return {
    zone: z.zone,
    zoneLabel: z.zoneLabel,
    ...PRIMARY_FACTOR[z.zone],
    symptomaticMean: null,
    baselineMean: null,
    deltaPct: null,
    symptomaticRuns: has('sparse') ? null : z.entriesCount90d,
    baselineRuns: null,
    confidence: null,
    shoeHighlight: null,
    summary: null,
    insufficientData: true,
  }
}

function buildBodyMapOverview(
  data: UserData,
  userId: number,
  windowDays: number,
  zoneFilter: BodyZone | null,
): BodyMapOverview {
  const st = v4Of(data, userId)
  const today = todayISO()
  const cutoffWindow = addDays(today, -windowDays)
  const cutoff30 = addDays(today, -30)
  const cutoff90 = addDays(today, -90)

  const sortedEntries = [...st.bodyEntries].sort((a, b) => b.date.localeCompare(a.date) || b.id - a.id)

  const zones: BodyZoneSummary[] = ALL_BODY_ZONES.map((zone) => {
    const zEntries90 = sortedEntries.filter((e) => e.zone === zone && e.date >= cutoff90)
    const zEntries30 = zEntries90.filter((e) => e.date >= cutoff30)
    const latest = zEntries90[0] ?? null
    const prev = zEntries90[1] ?? null
    const maxScore30d =
      zEntries30.length > 0
        ? Math.max(...zEntries30.map((e) => Math.max(e.painScore, e.coldNextDayScore ?? 0)))
        : null

    let trend: BodyZoneSummary['trend'] = 'none'
    if (latest) {
      if (!prev) trend = 'stable'
      else if (latest.painScore - prev.painScore >= 1) trend = 'worsening'
      else if (prev.painScore - latest.painScore >= 1) trend = 'improving'
      else trend = 'stable'
    }

    return {
      zone,
      zoneLabel: BODY_ZONE_META[zone].label,
      latestScore: latest?.painScore ?? null,
      latestColdNextDayScore: latest?.coldNextDayScore ?? null,
      latestSide: latest?.side ?? null,
      latestDate: latest?.date ?? null,
      maxScore30d,
      entriesCount90d: zEntries90.length,
      trend,
    }
  })

  const entriesInWindow = sortedEntries.filter(
    (e) => e.date >= cutoffWindow && (!zoneFilter || e.zone === zoneFilter),
  )

  const correlations: BodyCorrelationFactor[] = []
  const primaryShoe = data.shoes[0] ?? null
  const activeZonesWithPain = zones.filter(
    (z) => z.entriesCount90d > 0 && (!zoneFilter || z.zone === zoneFilter),
  )

  for (const z of activeZonesWithPain) {
    if (has('sparse') || z.entriesCount90d < 2) {
      correlations.push(insufficientFactor(z))
    } else if (z.zone === 'knee_pfps') {
      correlations.push(
        {
          zone: 'knee_pfps',
          zoneLabel: z.zoneLabel,
          ...PRIMARY_FACTOR.knee_pfps,
          symptomaticMean: 168,
          baselineMean: 54,
          deltaPct: 211.1,
          symptomaticRuns: z.entriesCount90d,
          baselineRuns: 14,
          confidence: 'high',
          shoeHighlight: null,
          summary: 'Les épisodes rotuliens surviennent sur des sorties comportant plus de 3× le D− habituel.',
        },
        {
          zone: 'knee_pfps',
          zoneLabel: z.zoneLabel,
          factor: 'shoe_wear',
          factorLabel: 'Usure chaussure',
          unit: 'km',
          symptomaticMean: primaryShoe ? Math.round(primaryShoe.distanceKm) : null,
          baselineMean: 245,
          deltaPct: primaryShoe ? deltaPct(primaryShoe.distanceKm, 245) : null,
          symptomaticRuns: z.entriesCount90d,
          baselineRuns: 14,
          confidence: 'moderate',
          shoeHighlight: primaryShoe
            ? {
                shoeId: primaryShoe.id,
                shoeName: primaryShoe.name,
                dropMm: primaryShoe.dropMm,
                distanceKm: primaryShoe.distanceKm,
                painEpisodes: z.entriesCount90d,
              }
            : null,
          summary: 'Gêne plus fréquente avec les paires dépassant 500 km de roulage.',
        },
      )
    } else if (z.zone === 'achilles' || z.zone === 'plantar_fascia') {
      const lowDropShoe = data.shoes.find((s) => (s.dropMm ?? 8) <= 6) ?? primaryShoe
      correlations.push({
        zone: z.zone,
        zoneLabel: z.zoneLabel,
        ...PRIMARY_FACTOR[z.zone],
        symptomaticMean: lowDropShoe?.dropMm ?? null,
        baselineMean: 8.5,
        deltaPct: isNum(lowDropShoe?.dropMm) ? deltaPct(lowDropShoe.dropMm, 8.5) : null,
        symptomaticRuns: z.entriesCount90d,
        baselineRuns: 15,
        confidence: 'moderate',
        shoeHighlight: lowDropShoe
          ? {
              shoeId: lowDropShoe.id,
              shoeName: lowDropShoe.name,
              dropMm: lowDropShoe.dropMm,
              distanceKm: lowDropShoe.distanceKm,
              painEpisodes: z.entriesCount90d,
            }
          : null,
        summary: 'Raideur matinale accrue après les séances courues avec un drop réduit.',
      })
    } else {
      correlations.push({
        zone: z.zone,
        zoneLabel: z.zoneLabel,
        ...PRIMARY_FACTOR[z.zone],
        symptomaticMean: 258,
        baselineMean: 241,
        deltaPct: 7.1,
        symptomaticRuns: z.entriesCount90d,
        baselineRuns: 14,
        confidence: z.entriesCount90d >= 3 ? 'moderate' : 'low',
        shoeHighlight: null,
        summary: 'Temps d’appui légèrement rallongé lors des sorties associées à cette gêne.',
      })
    }
  }

  return {
    zones,
    entries: entriesInWindow,
    correlations,
    windowDays,
  }
}

/* ---------------------------------- Public Share View builder ---------------------------------- */

interface SessionHealth {
  date: string
  ef: number | null
  impact: number | null
  joint: number | null
}

const round = (v: number, decimals: number) => Math.round(v * 10 ** decimals) / 10 ** decimals

function sumOrNull(values: (number | null)[]): number | null {
  const known = values.filter(isNum)
  return known.length ? known.reduce((a, b) => a + b, 0) : null
}

function meanOrNull(values: (number | null)[], min = 1): number | null {
  const known = values.filter(isNum)
  return known.length >= min ? known.reduce((a, b) => a + b, 0) / known.length : null
}

// Health metrics are derived from sessions only; with `sparse` (or no eligible session) they stay null.
function sessionHealth(it: CatalogItem, data: UserData): SessionHealth {
  const rich = enrichActivityView(buildActivityView(it, data), it, data)
  const steps = rich.impact?.steps
  return {
    date: it.date,
    ef: rich.efficiency?.eligible ? rich.efficiency.efSpeed : null,
    impact: steps ? Math.round(steps / 25) : null,
    joint: rich.impact?.jointIndex ?? null,
  }
}

function healthWindow(health: SessionHealth[], date: string) {
  const week = health.filter((h) => h.date <= date && h.date > addDays(date, -7))
  const ef = health.find((h) => h.date <= date && h.date > addDays(date, -28) && isNum(h.ef))?.ef ?? null
  const impact = sumOrNull(week.map((h) => h.impact))
  const joint = sumOrNull(week.map((h) => h.joint))
  return {
    efSpeed: isNum(ef) ? round(ef, 2) : null,
    impactLoad7d: isNum(impact) ? Math.round(impact) : null,
    jointIndex7d: isNum(joint) ? round(joint, 1) : null,
  }
}

function efTrendPct(health: SessionHealth[], today: string): number | null {
  const efIn = (from: number, to: number) =>
    health.filter((h) => h.date > addDays(today, -from) && h.date <= addDays(today, -to)).map((h) => h.ef)
  const recent = meanOrNull(efIn(28, 0), 2)
  const previous = meanOrNull(efIn(56, 28), 2)
  return isNum(recent) && isNum(previous) ? round((recent / previous - 1) * 100, 1) : null
}

function buildPublicShareView(me: MockUser, data: UserData, share: StoredShare): PublicShareView {
  const today = todayISO()
  const mode = sourceMode(data)
  const dayView = enrichDayView(buildDayView(today, data), data)
  const items = visibleItems(data, today)
  const planned = plannedWithMatch(data, items, today)
  const lr = loadRatio(items, today, mode)

  const sparse = has('sparse')
  const health = sparse ? [] : items.filter((it) => it.date > addDays(today, -56)).map((it) => sessionHealth(it, data))

  const loadSeries: PublicLoadPoint[] = []
  for (let d = 41; d >= 0; d--) {
    const dt = addDays(today, -d)
    const pointLr = loadRatio(items, dt, mode)
    loadSeries.push({
      date: dt,
      acute: Math.round(pointLr?.acute ?? 0),
      chronic: Math.round(pointLr?.chronic ?? 0),
      acwr: pointLr && pointLr.ratio !== null ? Math.round(pointLr.ratio * 100) / 100 : null,
      ...healthWindow(health, dt),
    })
  }
  const latestHealth = healthWindow(health, today)

  const recentSessions: PublicActivityItem[] = items.slice(0, 12).map((it) => {
    const sum = summarize(it, data, planned)
    const h = sparse ? null : sessionHealth(it, data)
    const notes = notesOf(data, it.id)
    const shoe = notes.shoeId ? data.shoes.find((s) => s.id === notes.shoeId) ?? null : null
    return {
      id: it.id,
      date: it.date,
      startLocal: it.startLocal,
      name: sum.name,
      sport: it.sport,
      typeLabel: sum.typeLabel,
      distanceM: it.base.distanceM,
      durationS: it.base.durationS,
      avgPaceSecPerKm: it.base.avgPaceSecPerKm,
      gapSecPerKm: gapSummary(it),
      avgHR: it.base.avgHR,
      avgCadenceSpm: it.base.avgCadenceSpm,
      gctMs: avgGct(it),
      elevGainM: it.base.elevGainM,
      elevLossM: it.base.elevLossM,
      trainingLoad: it.base.trainingLoad,
      impactLoad: h?.impact ?? null,
      jointIndex: h?.joint ?? null,
      efSpeed: h?.ef ?? null,
      shoeName: shoe?.name ?? null,
      shoeDropMm: shoe?.dropMm ?? null,
      painDuring: notes.painDuring,
      painNextMorning: notes.painNextMorning,
    }
  })

  return {
    generatedAt: isoNow(),
    shareLabel: share.label,
    expiresAt: share.expiresAt,
    athlete: {
      displayName: me.displayName,
      focusArea: data.profile.focusArea,
      focusLabel: data.profile.focusLabel,
      injuredSide: data.profile.injuredSide,
    },
    readiness: dayView.readiness ?? {
      level: 'green',
      title: 'Prêt pour la séance prévue',
      reasons: ['Charge aiguë / chronique équilibrée et récupération stable.'],
      hrv7dAvg: 62,
      hrvBaselineLow: 55,
      nightsBelowBaseline: 0,
      sleep7dAvg: 7.6,
      loadRatio: lr?.ratio ?? null,
    },
    loadSummary: {
      acute: Math.round(lr?.acute ?? 0),
      chronic: Math.round(lr?.chronic ?? 0),
      acwr: lr && lr.ratio !== null ? Math.round(lr.ratio * 100) / 100 : null,
      verdict: lr?.verdict ?? null,
      efLatest: latestHealth.efSpeed,
      efTrendPct: latestHealth.efSpeed === null ? null : efTrendPct(health, today),
      impactLoad7d: latestHealth.impactLoad7d,
      jointIndex7d: latestHealth.jointIndex7d,
    },
    loadSeries,
    cycles: buildCyclesOverview(data, me.id),
    recentSessions,
    bodyMap: buildBodyMapOverview(data, me.id, 90, null),
    shoes: data.shoes.map((s) => ({
      id: s.id,
      name: s.name,
      brand: s.brand,
      model: s.model,
      dropMm: s.dropMm,
      type: s.type,
      distanceKm: s.distanceKm,
      retired: s.retired,
    })),
  }
}

/* ---------------------------------- Registration ---------------------------------- */

export function tryAsyncMockV4(c: Ctx): Response | null {
  if (c.url.searchParams.get('async') !== '1') return null

  if (c.url.pathname === '/api/llm/analyze') {
    if (!llmConfigured(c.data)) {
      return err(412, 'PROVIDER_NOT_CONNECTED', 'Aucun connecteur IA configuré.')
    }
    const req = parseBody<{ kind?: string; activityId?: number; date?: string; question?: string }>(c.init) ?? {}
    const kind = req.kind ?? 'day'
    const ref = kind === 'activity' ? String(req.activityId ?? '') : (req.date ?? todayISO())
    const jobId = `job_llm_${nextId()}`
    const job: JobStatus = {
      id: jobId,
      userId: c.me.id,
      kind: 'llm.analyze',
      queue: 'default',
      state: 'running',
      label: 'Analyse Coach IA en cours…',
      progressPct: 15,
      processed: null,
      total: null,
      attempts: 1,
      maxAttempts: 2,
      error: null,
      result: null,
      createdAt: isoNow(),
      updatedAt: isoNow(),
    }
    v4Of(c.data, c.me.id).jobs.unshift(job)

    const chunks = [
      '### Synthèse biomécanique & charge\n\n',
      'La charge aiguë reste bien contenue par rapport à ta moyenne 28 jours. ',
      'Surveille la cadence en descente pour limiter la contrainte fémoro-patellaire.\n\n',
      '- **Prochaine séance** : conserve une endurance souple sur terrain plat.\n',
      '- **Vigilance J+1** : note le ressenti à froid demain matin dans la cartographie.',
    ]
    const fullMarkdown = chunks.join('')
    const analysis: LLMAnalysis = {
      id: nextId(),
      kind,
      ref,
      provider: 'gemini',
      model: 'gemini-2.5-flash',
      markdown: fullMarkdown,
      promptTokensEstimate: 840,
      durationMs: 420,
      createdAt: isoNow(),
    }

    chunks.forEach((delta, idx) => {
      setTimeout(() => {
        emitStreamEvent('llm.token', { jobId, kind, ref, delta })
      }, 25 * (idx + 1))
    })

    setTimeout(() => {
      c.data.v3?.analyses.unshift(analysis)
      job.state = 'completed'
      job.progressPct = 100
      job.updatedAt = isoNow()
      emitStreamEvent('llm.done', { jobId, kind, ref, analysis })
      emitStreamEvent('job.updated', { ...job })
    }, 25 * (chunks.length + 1) + 20)

    return json(job, 202)
  }

  return null
}

export function registerV4(route: RouteFn, pub: PubFn) {
  pub('GET', /^\/version$/, () =>
    json({
      version: '4.0.0',
      commit: 'v4mock01',
      builtAt: '2026-11-06T20:00:00Z',
    }),
  )

  pub('GET', /^\/api\/public\/share\/([^/]+)$/, ({ params }) => {
    const token = decodeURIComponent(params[0])
    const state = db()
    for (const u of state.users) {
      const d = userData(u.id)
      const st = v4Of(d, u.id)
      const found = st.shares.find((s) => s.token === token)
      if (!found) continue
      if (found.revoked) {
        return err(404, 'NOT_FOUND', 'Ce lien de partage a été révoqué.')
      }
      if (found.expiresAt && new Date(found.expiresAt).getTime() <= Date.now()) {
        return err(410, 'GONE', 'Ce lien de partage a expiré.')
      }
      found.accessCount += 1
      found.lastAccessedAt = isoNow()
      return json(buildPublicShareView(u, d, found), 200, {
        'Cache-Control': 'private, no-store',
      })
    }
    return err(404, 'NOT_FOUND', 'Lien de partage introuvable ou révoqué.')
  })

  /* --- Jobs --- */
  route('GET', /^\/api\/jobs$/, ({ me, data, url }) => {
    const limit = Math.max(1, Math.min(100, Number(url.searchParams.get('limit') ?? 20) || 20))
    return json({ items: v4Of(data, me.id).jobs.slice(0, limit) })
  })

  route('GET', /^\/api\/jobs\/([^/]+)$/, ({ me, data, params }) => {
    const id = decodeURIComponent(params[0])
    const found = v4Of(data, me.id).jobs.find((j) => j.id === id)
    return found ? json(found) : err(404, 'NOT_FOUND', 'Tâche introuvable.')
  })

  /* --- Cycles --- */
  route('GET', /^\/api\/cycles$/, ({ me, data, url }) => {
    const rawPlanId = url.searchParams.get('planId')
    const planId = rawPlanId ? Number(rawPlanId) || null : null
    return json(buildCyclesOverview(data, me.id, planId))
  })

  route('POST', /^\/api\/cycles\/plans$/, ({ me, data, init }) => {
    const b = parseBody<TrainingPlanInput>(init)
    if (!b || !b.name?.trim() || !DATE_RE.test(b.startDate ?? '') || !DATE_RE.test(b.endDate ?? '') || b.endDate < b.startDate) {
      return err(400, 'BAD_REQUEST', 'Macrocycle invalide : nom et dates cohérentes requis.')
    }
    const st = v4Of(data, me.id)
    const status: CyclePlanStatus = b.status ?? 'active'
    if (status === 'active') {
      for (const p of st.plans) if (p.status === 'active') p.status = 'archived'
    }
    const plan: TrainingPlan = {
      id: nextId(),
      goalId: b.goalId ?? null,
      name: b.name.trim(),
      sport: b.sport ?? 'run',
      startDate: b.startDate,
      endDate: b.endDate,
      targetEventLabel: b.targetEventLabel?.trim() ?? '',
      status,
      note: b.note?.trim() ?? '',
      createdAt: isoNow(),
      updatedAt: isoNow(),
    }
    st.plans.unshift(plan)
    if (b.autoScaffold !== false) {
      const sc = scaffoldPhasesForPlan(plan, b.peakWeeklyVolumeKm ?? 50, b.peakWeeklyLoad ?? 380)
      st.phases.push(...sc.phases)
      st.weeks.push(...sc.weeks)
    }
    return json(buildCyclesOverview(data, me.id, plan.id), 201)
  })

  route('PATCH', /^\/api\/cycles\/plans\/(\d+)$/, ({ me, data, params, init }) => {
    const st = v4Of(data, me.id)
    const plan = st.plans.find((p) => p.id === Number(params[0]))
    if (!plan) return err(404, 'NOT_FOUND', 'Macrocycle introuvable.')
    const b = parseBody<Partial<TrainingPlanInput>>(init) ?? {}
    if (b.name !== undefined) {
      if (!b.name.trim()) return err(400, 'BAD_REQUEST', 'Le nom du macrocycle est requis.')
      plan.name = b.name.trim()
    }
    if (b.goalId !== undefined) plan.goalId = b.goalId
    if (b.sport !== undefined) plan.sport = b.sport
    if (b.startDate !== undefined && DATE_RE.test(b.startDate)) plan.startDate = b.startDate
    if (b.endDate !== undefined && DATE_RE.test(b.endDate)) plan.endDate = b.endDate
    if (b.targetEventLabel !== undefined) plan.targetEventLabel = b.targetEventLabel.trim()
    if (b.status !== undefined) {
      if (b.status === 'active') {
        for (const other of st.plans) if (other.id !== plan.id && other.status === 'active') other.status = 'archived'
      }
      plan.status = b.status
    }
    if (b.note !== undefined) plan.note = b.note
    plan.updatedAt = isoNow()
    return json(plan)
  })

  route('DELETE', /^\/api\/cycles\/plans\/(\d+)$/, ({ me, data, params }) => {
    const st = v4Of(data, me.id)
    const id = Number(params[0])
    const before = st.plans.length
    st.plans = st.plans.filter((p) => p.id !== id)
    if (st.plans.length === before) return err(404, 'NOT_FOUND', 'Macrocycle introuvable.')
    st.phases = st.phases.filter((ph) => ph.planId !== id)
    st.weeks = st.weeks.filter((w) => w.planId !== id)
    return noContent()
  })

  route('POST', /^\/api\/cycles\/phases$/, ({ me, data, init }) => {
    const b = parseBody<TrainingPhaseInput>(init)
    if (!b || !b.name?.trim() || !DATE_RE.test(b.startDate ?? '') || !DATE_RE.test(b.endDate ?? '') || b.endDate < b.startDate) {
      return err(400, 'BAD_REQUEST', 'Mésocycle invalide : nom et dates cohérentes requis.')
    }
    const st = v4Of(data, me.id)
    const phaseType: PhaseType = b.phaseType && PHASE_TYPES.includes(b.phaseType) ? b.phaseType : 'build'
    const planId = b.planId ?? st.plans.find((p) => p.status === 'active')?.id ?? null
    const ph: StoredPhase = {
      id: nextId(),
      planId,
      name: b.name.trim(),
      phaseType,
      orderIndex: b.orderIndex ?? st.phases.filter((x) => x.planId === planId).length,
      startDate: b.startDate,
      endDate: b.endDate,
      focus: b.focus?.trim() ?? '',
      weeklyVolumeKm: b.weeklyVolumeKm ?? null,
      targetWeeklyLoad: b.targetWeeklyLoad ?? null,
      targetWeeklyDurationS: b.targetWeeklyDurationS ?? null,
      note: b.note?.trim() ?? '',
    }
    st.phases.push(ph)
    const built = buildCyclesOverview(data, me.id, planId).phases.find((x) => x.id === ph.id)
    return json(built, 201)
  })

  route('PATCH', /^\/api\/cycles\/phases\/(\d+)$/, ({ me, data, params, init }) => {
    const st = v4Of(data, me.id)
    const ph = st.phases.find((x) => x.id === Number(params[0]))
    if (!ph) return err(404, 'NOT_FOUND', 'Mésocycle introuvable.')
    const b = parseBody<Partial<TrainingPhaseInput>>(init) ?? {}
    if (b.name !== undefined) {
      if (!b.name.trim()) return err(400, 'BAD_REQUEST', 'Le nom du mésocycle est requis.')
      ph.name = b.name.trim()
    }
    if (b.phaseType !== undefined && PHASE_TYPES.includes(b.phaseType)) ph.phaseType = b.phaseType
    if (b.orderIndex !== undefined) ph.orderIndex = b.orderIndex
    if (b.startDate !== undefined && DATE_RE.test(b.startDate)) ph.startDate = b.startDate
    if (b.endDate !== undefined && DATE_RE.test(b.endDate)) ph.endDate = b.endDate
    if (b.focus !== undefined) ph.focus = b.focus
    if (b.weeklyVolumeKm !== undefined) ph.weeklyVolumeKm = b.weeklyVolumeKm
    if (b.targetWeeklyLoad !== undefined) ph.targetWeeklyLoad = b.targetWeeklyLoad
    if (b.targetWeeklyDurationS !== undefined) ph.targetWeeklyDurationS = b.targetWeeklyDurationS
    if (b.note !== undefined) ph.note = b.note
    const built = buildCyclesOverview(data, me.id, ph.planId).phases.find((x) => x.id === ph.id)
    return json(built)
  })

  route('DELETE', /^\/api\/cycles\/phases\/(\d+)$/, ({ me, data, params }) => {
    const st = v4Of(data, me.id)
    const id = Number(params[0])
    const before = st.phases.length
    st.phases = st.phases.filter((x) => x.id !== id)
    if (st.phases.length === before) return err(404, 'NOT_FOUND', 'Mésocycle introuvable.')
    st.weeks = st.weeks.filter((w) => w.phaseId !== id)
    return noContent()
  })

  route('PUT', /^\/api\/cycles\/weeks$/, ({ me, data, init }) => {
    const b = parseBody<TrainingWeekInput>(init)
    if (!b || !DATE_RE.test(b.weekStart ?? '')) {
      return err(400, 'BAD_REQUEST', 'Date de début de semaine invalide.')
    }
    const st = v4Of(data, me.id)
    const wStart = mondayOf(b.weekStart)
    const planId = b.planId === undefined ? (st.plans.find((p) => p.status === 'active')?.id ?? null) : b.planId
    let ov = st.weeks.find((w) => w.weekStart === wStart && w.planId === planId)
    if (!ov) {
      ov = {
        id: nextId(),
        planId,
        phaseId: b.phaseId ?? null,
        weekStart: wStart,
        isAssimilation: b.isAssimilation ?? false,
        targetVolumeKm: b.targetVolumeKm ?? null,
        targetLoad: b.targetLoad ?? null,
        targetDurationS: b.targetDurationS ?? null,
        note: b.note ?? '',
      }
      st.weeks.push(ov)
    } else {
      if (b.phaseId !== undefined) ov.phaseId = b.phaseId
      if (b.isAssimilation !== undefined) ov.isAssimilation = b.isAssimilation
      if (b.targetVolumeKm !== undefined) ov.targetVolumeKm = b.targetVolumeKm
      if (b.targetLoad !== undefined) ov.targetLoad = b.targetLoad
      if (b.targetDurationS !== undefined) ov.targetDurationS = b.targetDurationS
      if (b.note !== undefined) ov.note = b.note
    }
    const overview = buildCyclesOverview(data, me.id, planId)
    const allW = [...overview.phases.flatMap((p) => p.weeks), ...overview.standaloneWeeks]
    const built = allW.find((w) => w.weekStart === wStart)
    return json(built)
  })

  /* --- Body Stress Map --- */
  route('GET', /^\/api\/bodymap$/, ({ me, data, url }) => {
    const days = Math.max(7, Math.min(365, Number(url.searchParams.get('days') ?? 90) || 90))
    const rawZone = url.searchParams.get('zone') as BodyZone | null
    const zone = rawZone && ALL_BODY_ZONES.includes(rawZone) ? rawZone : null
    return json(buildBodyMapOverview(data, me.id, days, zone))
  })

  route('POST', /^\/api\/bodymap\/entries$/, ({ me, data, init }) => {
    const b = parseBody<BodyStressInput>(init)
    if (!b || !DATE_RE.test(b.date ?? '') || !ALL_BODY_ZONES.includes(b.zone)) {
      return err(400, 'BAD_REQUEST', 'Saisie corporelle invalide : date et zone requises.')
    }
    if (typeof b.painScore !== 'number' || b.painScore < 0 || b.painScore > 10) {
      return err(400, 'BAD_REQUEST', 'Score de douleur invalide (0 à 10).')
    }
    const side: BodySide = b.side && ['left', 'right', 'both'].includes(b.side) ? b.side : 'both'
    const act = b.activityId ? visibleItems(data).find((it) => it.id === b.activityId) ?? null : null
    const entry: BodyStressEntry = {
      id: nextId(),
      date: b.date,
      activityId: act?.id ?? b.activityId ?? null,
      activityName: act?.name ?? null,
      zone: b.zone,
      zoneLabel: BODY_ZONE_META[b.zone].label,
      side,
      painScore: b.painScore,
      coldNextDayScore: b.coldNextDayScore ?? null,
      note: b.note?.trim() ?? '',
      createdAt: isoNow(),
      updatedAt: isoNow(),
    }
    v4Of(data, me.id).bodyEntries.unshift(entry)
    return json(entry, 201)
  })

  route('PATCH', /^\/api\/bodymap\/entries\/(\d+)$/, ({ me, data, params, init }) => {
    const st = v4Of(data, me.id)
    const entry = st.bodyEntries.find((e) => e.id === Number(params[0]))
    if (!entry) return err(404, 'NOT_FOUND', 'Entrée introuvable.')
    const b = parseBody<Partial<BodyStressInput>>(init) ?? {}
    if (b.date !== undefined && DATE_RE.test(b.date)) entry.date = b.date
    if (b.zone !== undefined && ALL_BODY_ZONES.includes(b.zone)) {
      entry.zone = b.zone
      entry.zoneLabel = BODY_ZONE_META[b.zone].label
    }
    if (b.side !== undefined && ['left', 'right', 'both'].includes(b.side)) entry.side = b.side
    if (b.painScore !== undefined) entry.painScore = b.painScore
    if (b.coldNextDayScore !== undefined) entry.coldNextDayScore = b.coldNextDayScore
    if (b.activityId !== undefined) {
      const act = b.activityId ? visibleItems(data).find((it) => it.id === b.activityId) ?? null : null
      entry.activityId = b.activityId
      entry.activityName = act?.name ?? null
    }
    if (b.note !== undefined) entry.note = b.note.trim()
    entry.updatedAt = isoNow()
    return json(entry)
  })

  route('DELETE', /^\/api\/bodymap\/entries\/(\d+)$/, ({ me, data, params }) => {
    const st = v4Of(data, me.id)
    const id = Number(params[0])
    const before = st.bodyEntries.length
    st.bodyEntries = st.bodyEntries.filter((e) => e.id !== id)
    return st.bodyEntries.length === before ? err(404, 'NOT_FOUND', 'Entrée introuvable.') : noContent()
  })

  /* --- Share Links --- */
  const toShareLink = ({ token: _token, ...rest }: StoredShare): ShareLink => rest

  route('GET', /^\/api\/share$/, ({ me, data }) => {
    return json(v4Of(data, me.id).shares.map(toShareLink))
  })

  route('POST', /^\/api\/share$/, ({ me, data, init }) => {
    const b = parseBody<ShareLinkInput>(init)
    const label = b?.label?.trim() ?? ''
    if (label.length < 2 || label.length > 80) {
      return err(400, 'BAD_REQUEST', 'Intitulé du lien invalide (2 à 80 caractères).')
    }
    const token = `sh_${nextId()}_${Math.random().toString(36).slice(2, 10)}`
    let expiresAt: string | null = null
    if (b?.expiresAt) {
      expiresAt = b.expiresAt
    } else if (typeof b?.expiresInDays === 'number' && b.expiresInDays > 0) {
      expiresAt = new Date(Date.now() + b.expiresInDays * 86_400_000).toISOString()
    }
    const created: StoredShare = {
      id: nextId(),
      label,
      token,
      tokenPrefix: token.slice(0, 8),
      url: `/share/${token}`,
      expiresAt,
      revoked: false,
      revokedAt: null,
      accessCount: 0,
      lastAccessedAt: null,
      createdAt: isoNow(),
    }
    v4Of(data, me.id).shares.unshift(created)
    return json(toShareLink(created), 201)
  })

  route('DELETE', /^\/api\/share\/(\d+)$/, ({ me, data, params }) => {
    const st = v4Of(data, me.id)
    const found = st.shares.find((s) => s.id === Number(params[0]))
    if (!found) return err(404, 'NOT_FOUND', 'Lien de partage introuvable.')
    found.revoked = true
    found.revokedAt = isoNow()
    return noContent()
  })
}
