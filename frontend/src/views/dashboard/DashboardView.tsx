import { Activity, BarChart3, CalendarCheck, CloudSun, Flame, Gauge, History, Link2, Map as MapIcon, Moon, Sun, Target, Timer } from 'lucide-react'
import { lazy, Suspense, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useSearchParams } from 'wouter'
import { useActivities, useActivityView, useDayView, useProfile } from '../../api/queries'
import { useLlmConfig } from '../../api/queriesV3'
import type { ActivitySummary, ActivityView, AthleteProfile, Me } from '../../api/types'
import { TopAlerts } from '../../components/alerts/TopAlerts'
import { SessionHero } from '../../components/dashboard/SessionHero'
import { Disclosure } from '../../components/ui/Disclosure'
import { ReadinessBanner } from '../../components/v3/ReadinessBanner'
import { Warnings } from '../../components/alerts/Warnings'
import { SessionCharts } from '../../components/charts/SessionCharts'
import { BiomechanicsCard } from '../../components/dashboard/BiomechanicsCard'
import { FeelingCard } from '../../components/dashboard/FeelingCard'
import { HistoryTable } from '../../components/dashboard/HistoryTable'
import { CrossLoadCard, LoadCard } from '../../components/dashboard/LoadCards'
import { PerformanceCard } from '../../components/dashboard/PerformanceCard'
import { GoalsCard, PlanMatchCard, WeekCard } from '../../components/dashboard/PlanCards'
import { RecoveryCard } from '../../components/dashboard/RecoveryCard'
import { SegmentsTable } from '../../components/dashboard/SegmentsTable'
import { KpiGrid, StravaCard, WeatherCard } from '../../components/dashboard/SessionCards'
import { SessionHeader } from '../../components/dashboard/SessionHeader'
import { TodayCard } from '../../components/dashboard/TodayCard'
import { MobilePromptBar, PromptGenerator } from '../../components/prompt/PromptGenerator'
import { usePrompt } from '../../components/prompt/usePrompt'
import { SessionPicker } from '../../components/session/SessionPicker'
import { EmptyState, Spinner } from '../../components/ui/Misc'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { cn } from '../../lib/cn'
import { fmtNumber, isNum, todayISO } from '../../lib/format'
import { ui } from '../../state/ui'
import { ErrorView, LoadingView } from '../StateViews'

const AnalysisDrawer = lazy(() => import('../../components/ai/AnalysisDrawer').then((m) => ({ default: m.AnalysisDrawer })))
const GpsTrack = lazy(() => import('../../components/session/GpsTrack').then((m) => ({ default: m.GpsTrack })))
const EfficiencyImpact = lazy(() => import('../../components/session/EfficiencyImpact').then((m) => ({ default: m.EfficiencyImpact })))
const fallback = <Spinner label="Chargement…" />

function useInView<T extends Element>() {
  const ref = useRef<T | null>(null)
  const [inView, setInView] = useState(false)
  useEffect(() => {
    const el = ref.current
    if (!el || typeof IntersectionObserver === 'undefined') return
    const io = new IntersectionObserver(([e]) => setInView(e.isIntersecting), { threshold: 0.15 })
    io.observe(el)
    return () => io.disconnect()
  }, [])
  return [ref, inView] as const
}

const ALERT_SECTION: Record<string, string> = {
  CADENCE_LOW: 'biomech',
  DOWNHILL_CADENCE_DROP: 'biomech',
  GCT_DRIFT: 'biomech',
  VO_HIGH: 'biomech',
  GCT_BALANCE: 'biomech',
  GCT_BALANCE_NA: 'biomech',
  DESCENT_HIGH: 'biomech',
  HRV_LOW: 'recovery',
  RHR_HIGH: 'recovery',
  SLEEP_LOW: 'recovery',
  BB_LOW: 'recovery',
  LOAD_RATIO: 'load',
  FLOORS_HIGH: 'load',
  DRIFT_HIGH: 'load',
  FADE_END: 'splits',
  PLAN_DEVIATION: 'plan',
  PLAN_MISSED: 'plan',
  EF_DROP: 'efficiency',
  IMPACT_HIGH: 'efficiency',
}

function sectionForAlert(code: string, v: ActivityView): string | null {
  const s = ALERT_SECTION[code] ?? (/CADENCE|GCT|VO_|STRIDE|SLOPE|DESCENT/.test(code) ? 'biomech' : /HRV|SLEEP|RHR|BB_/.test(code) ? 'recovery' : /LOAD|FLOOR|DRIFT/.test(code) ? 'load' : null)
  if (s === 'biomech' && !v.activity.isRunning) return null
  return s
}

const n = (x: number | null | undefined, d = 0, unit = '') => (isNum(x) ? `${fmtNumber(x, d)}${unit}` : null)
const join = (...xs: (string | null | false | undefined)[]) => xs.filter(Boolean).join(' · ')

function useOpenSections(v: ActivityView | null) {
  const [state, setState] = useState<{ id: number | null; open: Set<string> }>({ id: null, open: new Set() })
  const auto = useMemo(() => {
    const set = new Set<string>()
    if (v) for (const a of v.alerts) if (a.level === 'danger') set.add(sectionForAlert(a.code, v) ?? '')
    set.delete('')
    return set
  }, [v])
  const id = v?.activity.id ?? null
  const open = state.id === id ? state.open : auto
  const set = (key: string, on: boolean) => {
    const next = new Set(open)
    if (on) next.add(key)
    else next.delete(key)
    setState({ id, open: next })
  }
  const reveal = (key: string) => {
    set(key, true)
    requestAnimationFrame(() => {
      const el = document.getElementById(`sec-${key}`)
      el?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      el?.querySelector<HTMLButtonElement>('button[aria-expanded]')?.focus({ preventScroll: true })
    })
  }
  return { open, set, reveal }
}

function SessionSections({ v, profile, sections }: { v: ActivityView; profile: AthleteProfile | null; sections: ReturnType<typeof useOpenSections> }) {
  const th = profile?.thresholds ?? null
  const b = v.biomechanics
  const flags = new Map<string, 'warn' | 'danger'>()
  for (const a of v.alerts) {
    const s = sectionForAlert(a.code, v)
    if (s && (a.level === 'danger' || (a.level === 'warn' && flags.get(s) !== 'danger'))) flags.set(s, a.level)
  }
  const p = (key: string) => ({ id: `sec-${key}`, open: sections.open.has(key), onToggle: (o: boolean) => sections.set(key, o), flag: flags.get(key) ?? null })
  const segs = v.segments.primary === 'blocks' ? v.segments.blocks : v.segments.laps
  const hasTrack = v.activity.sources.includes('garmin') || v.series.tS.length > 1
  return (
    <div className="section-group" aria-label="Détails de la séance">
      {v.activity.isRunning && (
        <Disclosure {...p('biomech')} title="Biomécanique, dérive & pente" icon={<Activity />} summary={join(n(b.cadenceAvgSpm, 0, ' spm'), n(b.groundContactTimeMs, 0, ' ms GCT'), n(b.verticalOscillationCm, 1, ' cm OV'))} cardTitles="hide">
          <BiomechanicsCard view={v} profile={profile} />
        </Disclosure>
      )}
      {segs.length > 0 && (
        <Disclosure {...p('splits')} title="Intervalles & splits" icon={<Timer />} summary={`${segs.length} ${v.segments.primary === 'blocks' ? 'blocs' : 'tours'}`} cardTitles="hide">
          <SegmentsTable key={v.activity.id} segments={v.segments} />
        </Disclosure>
      )}
      <Disclosure {...p('charts')} title="Courbes" icon={<BarChart3 />} summary="FC, allure, cadence, puissance, altitude" cardTitles="hide">
        <SessionCharts view={v} profile={profile} />
      </Disclosure>
      {hasTrack && (
        <Disclosure {...p('gps')} title="Tracé GPS" icon={<MapIcon />} summary="Coloré par cadence ou par pente">
          <Suspense fallback={fallback}>
            <GpsTrack activityId={v.activity.id} thresholds={th} />
          </Suspense>
        </Disclosure>
      )}
      {(v.efficiency || v.impact) && (
        <Disclosure
          {...p('efficiency')}
          title="Efficience & impacts"
          icon={<Gauge />}
          summary={join(v.efficiency?.eligible ? `EF ${fmtNumber(v.efficiency.efSpeed, 2)}` : v.efficiency ? 'EF non calculée' : null, n(v.impact?.steps, 0, ' foulées'), n(v.impact?.jointIndex, 1) && `indice ${fmtNumber(v.impact?.jointIndex, 1)}`)}
        >
          <Suspense fallback={fallback}>
            <EfficiencyImpact view={v} />
          </Suspense>
        </Disclosure>
      )}
      <Disclosure {...p('load')} title="Charge & charge croisée" icon={<Flame />} summary={join(v.load.loadRatio?.ratio != null && `ratio ${fmtNumber(v.load.loadRatio.ratio, 2)}`, n(v.load.trainingLoad, 0) && `TL ${fmtNumber(v.load.trainingLoad)}`, 'étages, pas, autres sports')}>
        <div className="grid gap-5 xl:grid-cols-2">
          <LoadCard view={v} thresholds={th} />
          <CrossLoadCard cross={v.crossLoad} thresholds={th} />
        </div>
      </Disclosure>
      <Disclosure {...p('recovery')} title="Récupération & performance" icon={<Moon />} summary={join(v.readiness && v.readiness.level !== 'unknown' && v.readiness.title, n(v.recovery?.hrv?.lastNightAvgMs, 0, ' ms VFC'), n(v.recovery?.sleep?.score, 0, '/100 sommeil'))}>
        <div className="space-y-5">
          {v.readiness && <ReadinessBanner r={v.readiness} label="Préparation avant la séance" compact />}
          <div className="grid gap-5 xl:grid-cols-2">
            <RecoveryCard rec={v.recovery} title="Nuit précédente" emptyReason={v.activity.sources.includes('garmin') ? undefined : 'Garmin non connecté'} />
            <PerformanceCard perf={v.performance} />
          </div>
        </div>
      </Disclosure>
      <Disclosure {...p('conditions')} title="Mesures, météo & Strava" icon={<CloudSun />} summary={join(n(v.activity.gapSecPerKm) && 'GAP', n(v.weather?.temperatureC, 1, ' °C'), v.activity.strava?.rpe != null && `RPE ${v.activity.strava.rpe}`)}>
        <div className="space-y-5">
          <KpiGrid view={v} />
          <div className="grid gap-5 md:grid-cols-2">
            <WeatherCard view={v} />
            <StravaCard view={v} />
          </div>
        </div>
      </Disclosure>
      <Disclosure {...p('plan')} title="Prévu vs réalisé" icon={<CalendarCheck />} summary={v.plan?.summary ?? 'Séance hors plan'} cardTitles="hide">
        <PlanMatchCard match={v.plan} upcoming={v.upcoming} />
      </Disclosure>
      <Disclosure {...p('history')} title="Historique 14 jours" icon={<History />} summary={`${v.history.length} séances`} cardTitles="hide">
        <HistoryTable rows={v.history} currentId={v.activity.id} />
      </Disclosure>
    </div>
  )
}

/** Light dashboard: readiness, compact session hero, top alerts, ressenti, AI actions; details folded. */
export function DashboardView({ me }: { me: Me }) {
  const profile = useProfile()
  const today = todayISO(profile.data?.timezone)
  const [params, setParams] = useSearchParams()
  const sParam = Number(params.get('s')) || null
  const list = useActivities()
  const items = useMemo(() => list.data?.pages.flatMap((p) => p.items) ?? [], [list.data])
  const auto = (items.find((a) => a.isRunning) ?? items[0])?.id ?? null
  const selectedId = sParam ?? auto
  const [picked, setPicked] = useState<ActivitySummary | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)
  const day = useDayView(today)
  const llm = useLlmConfig()
  const [ai, setAi] = useState({ open: false, run: 0 })

  useEffect(() => {
    ui.selectActivity(selectedId)
    return () => ui.selectActivity(null)
  }, [selectedId])

  const view = useActivityView(selectedId)
  const v = view.data ?? null
  const stale = view.isPlaceholderData || (v !== null && v.activity.id !== selectedId)
  const summary = items.find((a) => a.id === selectedId) ?? (picked?.id === selectedId ? picked : null)
  useDocumentTitle(v ? v.activity.name : 'Tableau de bord')
  const sections = useOpenSections(v && !stale ? v : null)

  const prompt = usePrompt({ activity: v && !stale ? { id: v.activity.id, name: v.activity.name, date: v.activity.date } : summary ? { id: summary.id, name: summary.name, date: summary.date } : null, day: today })
  const [promptRef, promptVisible] = useInView<HTMLElement>()
  const llmOn = llm.data?.configured === true
  const analyze = () => setAi((s) => ({ open: true, run: s.run + 1 }))
  const aiSubject =
    prompt.kind === 'activity' && v && !stale
      ? ({ kind: 'activity', activityId: v.activity.id, label: v.activity.name } as const)
      : ({ kind: 'day', date: today, label: 'Bilan du jour' } as const)

  const select = (id: number) => {
    setParams((p) => {
      const next = new URLSearchParams(p)
      next.set('s', String(id))
      return next
    })
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }
  const idx = selectedId === null ? -1 : items.findIndex((a) => a.id === selectedId)
  const older = idx >= 0 && idx < items.length - 1 ? items[idx + 1] : null
  const newer = idx > 0 ? items[idx - 1] : null

  const noActivities = list.isSuccess && items.length === 0 && sParam === null
  const th = profile.data?.thresholds ?? null
  const ready = v && !stale

  let main
  if (list.isError && !items.length) main = <ErrorView error={list.error} onRetry={() => void list.refetch()} />
  else if (noActivities)
    main = (
      <EmptyState
        icon={<Link2 />}
        title="Aucune activité pour l’instant"
        action={
          <Link href="/settings#integrations" className="btn btn-primary">
            Connecter Garmin ou Strava
          </Link>
        }
      >
        Connecte une source de données pour voir tes séances. Le bilan du jour, le planning et les objectifs restent disponibles.
      </EmptyState>
    )
  else if (view.isError && !v) main = <ErrorView error={view.error} onRetry={() => void view.refetch()} />
  else if (!v) main = <LoadingView label="Chargement de la séance…" />
  else main = <TopAlerts alerts={v.alerts} sectionOf={(c) => sectionForAlert(c, v)} onOpen={sections.reveal} />
  const details = v && !(view.isError && !v) && !noActivities && (
    <div className={cn('space-y-4 lg:space-y-5', stale && 'is-refetching')} aria-busy={view.isFetching}>
      <SessionSections v={v} profile={profile.data ?? null} sections={sections} />
      <Warnings warnings={v.warnings} sources={v.dataSources} />
    </div>
  )

  return (
    <div className="space-y-4 lg:space-y-6">
      <h1 className="sr-only">Tableau de bord de {me.displayName}</h1>
      {day.data?.readiness && <ReadinessBanner r={day.data.readiness} />}

      <div className="grid gap-4 lg:grid-cols-12 lg:grid-rows-[auto_1fr] lg:gap-x-6 lg:gap-y-5">
        <div className="min-w-0 space-y-4 lg:col-span-8 lg:row-start-1 lg:space-y-5">
          {!noActivities && (
            <SessionHeader
              summary={summary}
              view={ready ? v : null}
              loading={list.isPending || view.isPending}
              onOpenPicker={() => setPickerOpen(true)}
              onPrev={older ? () => select(older.id) : null}
              onNext={newer ? () => select(newer.id) : null}
            >
              {ready && <SessionHero view={v} thresholds={th} />}
            </SessionHeader>
          )}
          <div className={cn(stale && 'is-refetching')}>{main}</div>
        </div>
        <aside className="min-w-0 space-y-4 self-start lg:col-span-4 lg:col-start-9 lg:row-span-2 lg:row-start-1 lg:space-y-5" aria-label="Ressenti, coach IA et semaine">
          {ready && <FeelingCard key={v.activity.id} view={v} />}
          <PromptGenerator ref={promptRef} p={prompt} llmConfigured={llmOn} onAnalyze={analyze} />
          <div className="section-group">
            <Disclosure title="Aujourd’hui" icon={<Sun />} summary={day.data ? `${day.data.planned.length} prévue(s) · ${day.data.activities.length} réalisée(s)` : undefined} cardTitles="hide">
              <TodayCard date={today} showReadiness={false} />
            </Disclosure>
            <Disclosure title="Semaine" icon={<CalendarCheck />} summary="Prévu vs réalisé, 7 jours" cardTitles="hide">
              <WeekCard date={today} today={today} />
            </Disclosure>
            <Disclosure title="Objectifs" icon={<Target />} summary={v?.goals?.length ? `${v.goals.length} actif(s)` : undefined} cardTitles="hide">
              <GoalsCard today={today} goals={v?.goals} />
            </Disclosure>
          </div>
        </aside>
        {details && <div className="min-w-0 lg:col-span-8 lg:col-start-1 lg:row-start-2">{details}</div>}
      </div>
      <div className="h-14 lg:hidden" aria-hidden="true" />
      <MobilePromptBar p={prompt} hidden={promptVisible} onAnalyze={llmOn ? analyze : undefined} />
      {ai.run > 0 && (
        <Suspense fallback={null}>
          <AnalysisDrawer open={ai.open} onClose={() => setAi((s) => ({ ...s, open: false }))} subject={aiSubject} include={prompt.include} runToken={ai.run} />
        </Suspense>
      )}

      <SessionPicker
        open={pickerOpen}
        onClose={() => setPickerOpen(false)}
        selectedId={selectedId}
        selectedDate={summary?.date ?? v?.activity.date ?? null}
        today={today}
        onSelect={(a) => {
          setPicked(a)
          select(a.id)
        }}
      />
    </div>
  )
}
