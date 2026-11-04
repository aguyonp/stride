import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { isAuthError } from './api/client'
import { DEFAULT_PROFILE, isMode } from './api/defaults'
import { useActivities, useActivityAnalysis, useDayAnalysis, useSettings } from './api/queries'
import type { ActivitySummary, Mode } from './api/types'
import { ExportPanel } from './components/export/ExportPanel'
import { MobileActionBar } from './components/export/MobileActionBar'
import { ReportPreview } from './components/export/ReportPreview'
import { useReportExport, type ReportPayload } from './components/export/useReportExport'
import { Ambient } from './components/layout/Ambient'
import { AppHeader } from './components/layout/AppHeader'
import { ModeSwitch } from './components/layout/ModeSwitch'
import { SessionPicker } from './components/session/SessionPicker'
import { ActivitySubjectCard, DaySubjectCard } from './components/session/SubjectCard'
import { Onboarding } from './components/settings/Onboarding'
import { SettingsDialog } from './components/settings/SettingsDialog'
import { useStoredState } from './hooks/useStoredState'
import { useTheme } from './hooks/useTheme'
import { cn } from './lib/cn'
import { fmtDateShort, todayISO } from './lib/format'
import { MODE_LABEL } from './lib/labels'
import { STORAGE_KEYS } from './lib/storage'
import { PerformanceView } from './views/PerformanceView'
import { ActivityRecoveryView, DayView } from './views/RecoveryViews'
import { SfpView } from './views/SfpView'
import { EmptyView, ErrorView, LoadingView } from './views/StateViews'

const parseMode = (raw: string) => (isMode(raw) ? raw : undefined)
const parseBool = (raw: string) => (raw === '1' ? true : raw === '0' ? false : undefined)
const serializeBool = (v: boolean) => (v ? '1' : '0')

export default function App() {
  const qc = useQueryClient()
  const { pref, theme, setPref, toggle } = useTheme()
  const [mode, setModeState] = useStoredState<Mode>(STORAGE_KEYS.mode, 'sfp', parseMode)
  const [withHistory, setWithHistory] = useStoredState<boolean>(STORAGE_KEYS.history, false, parseBool, serializeBool)
  const history = withHistory ? 14 : 0

  const settings = useSettings()
  const profile = settings.data?.profile ?? DEFAULT_PROFILE
  const today = todayISO(settings.data?.timezone)

  /* --- Sélection de la séance ------------------------------------- */
  const activities = useActivities()
  const firstPage = activities.data?.pages[0]?.items
  // Par défaut : la dernière activité de course.
  const autoId = useMemo(() => (firstPage ? ((firstPage.find((a) => a.isRunning) ?? firstPage[0])?.id ?? null) : null), [firstPage])
  const [pickedId, setPickedId] = useState<number | null>(null)
  const [picked, setPicked] = useState<ActivitySummary | null>(null)
  const selectedId = pickedId ?? autoId

  /* --- Mode Santé & Récup : jour (+ course éventuelle du jour) ---- */
  const [dayPicked, setDayPicked] = useState<string | null>(null)
  const day = dayPicked ?? today
  const [dayRun, setDayRun] = useState<ActivitySummary | null>(null)

  const subject: 'activity' | 'day' = mode === 'recovery' && !dayRun ? 'day' : 'activity'
  const analysisId = mode === 'recovery' ? (dayRun?.id ?? null) : selectedId
  const analysis = useActivityAnalysis(subject === 'activity' ? analysisId : null, mode, history)
  const dayAnalysis = useDayAnalysis(subject === 'day' ? day : null, history)

  const setMode = useCallback(
    (m: Mode) => {
      setModeState(m)
      if (m === 'recovery') setDayRun(null)
    },
    [setModeState],
  )

  useEffect(() => {
    document.documentElement.dataset.mode = mode
  }, [mode])

  /* --- Dialogs ---------------------------------------------------- */
  const [pickerOpen, setPickerOpen] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [focusKey, setFocusKey] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(false)
  const openSettings = (focus = false) => {
    setFocusKey(focus)
    setSettingsOpen(true)
  }

  /* --- Données affichées ------------------------------------------ */
  const current = subject === 'day' ? dayAnalysis : analysis
  const authError = [activities.error, analysis.error, dayAnalysis.error, settings.error].find(isAuthError) ?? null
  const stale = current.isPlaceholderData || (current.isFetching && !current.isPending && !current.data)

  const summary: ActivitySummary | null = useMemo(() => {
    const id = analysisId
    if (id === null) return null
    if (analysis.data?.activity.id === id) return analysis.data.activity
    const fromList = activities.data?.pages.flatMap((p) => p.items).find((a) => a.id === id)
    return fromList ?? (picked?.id === id ? picked : null) ?? (dayRun?.id === id ? dayRun : null)
  }, [analysisId, analysis.data, activities.data, picked, dayRun])

  const payload: ReportPayload | null = useMemo(() => {
    if (subject === 'day') {
      const d = dayAnalysis.data
      if (!d || d.date !== day) return null
      return { report: d.report, tokens: d.reportTokensEstimate, filename: `stride-${d.date}-recovery.md`, title: `Stride · bilan du ${fmtDateShort(d.date)}` }
    }
    const a = analysis.data
    if (!a || a.activity.id !== analysisId || a.mode !== mode) return null
    return { report: a.report, tokens: a.reportTokensEstimate, filename: `stride-${a.activity.date}-${a.mode}.md`, title: `Stride · ${a.activity.name}` }
  }, [subject, dayAnalysis.data, day, analysis.data, analysisId, mode])

  const exportLoading = current.isFetching && (!payload || current.isPlaceholderData)
  const exporter = useReportExport(exportLoading ? null : payload, () => setPreviewOpen(true))

  useEffect(() => {
    const base = 'Stride'
    if (subject === 'day') document.title = `${base} · Bilan du ${fmtDateShort(day)}`
    else if (summary) document.title = `${base} · ${summary.name} · ${MODE_LABEL[mode]}`
    else document.title = base
  }, [subject, day, summary, mode])

  const dayRuns = (dayAnalysis.data?.date === day ? dayAnalysis.data.activities : []).filter((a) => a.isRunning)
  const analyzeRun = (a: ActivitySummary) => {
    setDayRun(a)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  /* --- Contenu principal ------------------------------------------ */
  let content: ReactNode
  if (subject === 'day') {
    const d = dayAnalysis.data
    if (dayAnalysis.isError && !d) content = <ErrorView error={dayAnalysis.error} onRetry={() => void dayAnalysis.refetch()} />
    else if (!d) content = <LoadingView label="Chargement du bilan du jour…" />
    else content = <DayView d={d} onAnalyzeRun={analyzeRun} />
  } else {
    const a = analysis.data
    if (activities.isError && !firstPage) content = <ErrorView error={activities.error} onRetry={() => void activities.refetch()} />
    else if (firstPage && firstPage.length === 0 && analysisId === null) content = <EmptyView message="Aucune activité trouvée sur Garmin Connect pour le moment." />
    else if (analysis.isError && !a) content = <ErrorView error={analysis.error} onRetry={() => void analysis.refetch()} />
    else if (!a) content = <LoadingView />
    else if (a.mode === 'performance') content = <PerformanceView a={a} profile={profile} />
    else if (a.mode === 'recovery') content = <ActivityRecoveryView a={a} />
    else content = <SfpView a={a} profile={profile} />
  }

  const exportProps = {
    payload: exportLoading ? null : payload,
    loading: exportLoading,
    withHistory,
    onToggleHistory: setWithHistory,
    onCopy: () => void exporter.copy(),
    onPreview: () => setPreviewOpen(true),
    onDownload: exporter.download,
    onShare: () => void exporter.share(),
    canShare: exporter.canShare,
  }

  return (
    <>
      <a href="#main" className="sr-only z-50 rounded-xl bg-surface px-4 py-2 focus:not-sr-only focus:fixed focus:top-3 focus:left-3">
        Aller au contenu
      </a>
      <Ambient />
      <div className="mx-auto max-w-[1600px] px-4 pt-2 pb-36 sm:px-6 lg:px-8 lg:pt-3 lg:pb-12">
        <AppHeader theme={theme} onToggleTheme={toggle} onOpenSettings={() => openSettings(false)} center={<ModeSwitch mode={mode} onChange={setMode} className="w-full max-w-[540px]" />} />
        <div className="mt-1 mb-4 lg:hidden">
          <ModeSwitch mode={mode} onChange={setMode} className="w-full" />
        </div>

        {authError ? (
          <main id="main" className="mt-2 lg:mt-10">
            <Onboarding
              error={authError}
              onOpenSettings={() => openSettings(true)}
              onRetry={() => {
                void qc.resetQueries()
              }}
            />
          </main>
        ) : (
          <div className="grid grid-cols-1 gap-4 lg:mt-6 lg:grid-cols-12 lg:gap-6">
            <aside className="space-y-4 self-start lg:sticky lg:top-[96px] lg:col-span-4 xl:col-span-3" aria-label="Sélection et export">
              {mode === 'recovery' ? (
                <DaySubjectCard
                  date={day}
                  today={today}
                  fetching={current.isFetching}
                  onChangeDate={(d) => {
                    setDayPicked(d)
                    setDayRun(null)
                  }}
                  onOpenPicker={() => setPickerOpen(true)}
                  runs={dayRuns}
                  onAnalyzeRun={analyzeRun}
                  analyzingRunId={dayRun?.id ?? null}
                  onBackToDay={() => setDayRun(null)}
                />
              ) : (
                <ActivitySubjectCard
                  activity={summary}
                  locationName={analysis.data?.activity.id === analysisId ? analysis.data.activity.locationName : null}
                  loading={!summary && (activities.isPending || analysis.isPending)}
                  fetching={analysis.isFetching}
                  onOpenPicker={() => setPickerOpen(true)}
                />
              )}
              <ExportPanel {...exportProps} className="hidden lg:block" />
            </aside>
            <main id="main" className={cn('min-w-0 lg:col-span-8 xl:col-span-9', stale && 'is-refetching')} aria-busy={current.isFetching}>
              {content}
            </main>
          </div>
        )}
      </div>

      {!authError && <MobileActionBar {...exportProps} />}

      <SessionPicker
        open={pickerOpen}
        onClose={() => setPickerOpen(false)}
        purpose={mode === 'recovery' ? 'day' : 'activity'}
        selectedId={selectedId}
        selectedDate={mode === 'recovery' ? day : (summary?.date ?? null)}
        today={today}
        onSelectActivity={(a) => {
          setPickedId(a.id)
          setPicked(a)
        }}
        onSelectDate={(d) => {
          setDayPicked(d)
          setDayRun(null)
        }}
      />
      <SettingsDialog open={settingsOpen} onClose={() => setSettingsOpen(false)} focusApiKey={focusKey} themePref={pref} onThemePref={setPref} />
      <ReportPreview
        open={previewOpen}
        onClose={() => setPreviewOpen(false)}
        payload={payload}
        refreshing={exportLoading}
        withHistory={withHistory}
        onToggleHistory={setWithHistory}
        onCopy={exportProps.onCopy}
        onDownload={exportProps.onDownload}
        onShare={exportProps.onShare}
        canShare={exporter.canShare}
      />
    </>
  )
}
