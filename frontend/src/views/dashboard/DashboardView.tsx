import { Link2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useSearchParams } from 'wouter'
import { useActivities, useActivityView, useProfile } from '../../api/queries'
import type { ActivitySummary, Me } from '../../api/types'
import { AlertsBanner } from '../../components/alerts/AlertsBanner'
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
import { EmptyState } from '../../components/ui/Misc'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { cn } from '../../lib/cn'
import { todayISO } from '../../lib/format'
import { ui } from '../../state/ui'
import { ErrorView, LoadingView } from '../StateViews'

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

/** Unified dashboard: selected session (all V2 metrics), today, week, goals, ressenti and the prompt generator. */
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

  useEffect(() => {
    ui.selectActivity(selectedId)
    return () => ui.selectActivity(null)
  }, [selectedId])

  const view = useActivityView(selectedId)
  const v = view.data ?? null
  const stale = view.isPlaceholderData || (v !== null && v.activity.id !== selectedId)
  const summary = items.find((a) => a.id === selectedId) ?? (picked?.id === selectedId ? picked : null)
  useDocumentTitle(v ? v.activity.name : 'Tableau de bord')

  const prompt = usePrompt({ activity: v && !stale ? { id: v.activity.id, name: v.activity.name, date: v.activity.date } : summary ? { id: summary.id, name: summary.name, date: summary.date } : null, day: today })
  const [promptRef, promptVisible] = useInView<HTMLElement>()

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
  else
    main = (
      <div className={cn('space-y-4 lg:space-y-5', stale && 'is-refetching')} aria-busy={view.isFetching}>
        <KpiGrid view={v} />
        <div className="grid gap-4 md:grid-cols-2 lg:gap-5">
          <WeatherCard view={v} />
          <StravaCard view={v} />
        </div>
        {v.activity.isRunning && <BiomechanicsCard view={v} profile={profile.data ?? null} />}
        <div className="grid gap-4 xl:grid-cols-2 lg:gap-5">
          <LoadCard view={v} thresholds={th} />
          <CrossLoadCard cross={v.crossLoad} thresholds={th} />
        </div>
        <SegmentsTable key={v.activity.id} segments={v.segments} />
        <SessionCharts view={v} profile={profile.data ?? null} />
        <div className="grid gap-4 xl:grid-cols-2 lg:gap-5">
          <RecoveryCard rec={v.recovery} title="Récupération (nuit précédente)" emptyReason={v.activity.sources.includes('garmin') ? undefined : 'Garmin non connecté'} />
          <PerformanceCard perf={v.performance} />
        </div>
        <PlanMatchCard match={v.plan} upcoming={v.upcoming} />
        <HistoryTable rows={v.history} currentId={v.activity.id} />
        <Warnings warnings={v.warnings} sources={v.dataSources} />
      </div>
    )

  return (
    <div className="space-y-4 lg:space-y-5">
      <h1 className="sr-only">Tableau de bord de {me.displayName}</h1>
      {!noActivities && (
        <SessionHeader
          summary={summary}
          view={v && !stale ? v : null}
          loading={list.isPending || view.isPending}
          onOpenPicker={() => setPickerOpen(true)}
          onPrev={older ? () => select(older.id) : null}
          onNext={newer ? () => select(newer.id) : null}
        />
      )}
      {v && !stale && <AlertsBanner alerts={v.alerts} />}

      <div className="grid gap-4 lg:grid-cols-12 lg:grid-rows-[auto_1fr] lg:gap-5">
        <div className="min-w-0 space-y-4 lg:col-span-4 lg:col-start-9 lg:row-start-1 lg:space-y-5">
          {v && !stale && <FeelingCard key={v.activity.id} view={v} />}
          <PromptGenerator ref={promptRef} p={prompt} />
        </div>
        <div className="min-w-0 lg:col-span-8 lg:col-start-1 lg:row-span-2 lg:row-start-1">{main}</div>
        <div className="min-w-0 space-y-4 self-start lg:col-span-4 lg:col-start-9 lg:row-start-2 lg:space-y-5">
          <TodayCard date={today} />
          <WeekCard date={today} today={today} />
          <GoalsCard today={today} goals={v?.goals} />
        </div>
      </div>
      <div className="h-14 lg:hidden" aria-hidden="true" />
      <MobilePromptBar p={prompt} hidden={promptVisible} />

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
