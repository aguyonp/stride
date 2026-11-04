import type { ActivityAnalysis, ActivitySummary, DayAnalysis } from '../api/types'
import { AlertsBanner } from '../components/alerts/AlertsBanner'
import { Warnings } from '../components/alerts/Warnings'
import { DayActivitiesCard, DayActivityCard, HrvWeekCard, SleepCard, TrendTable, VitalsRow } from '../components/recovery/DayCards'
import { BiomechRow } from '../components/sfp/BiomechanicsPanel'
import { KpiGrid } from '../components/sfp/KpiGrid'

/** Mode Santé & Récup — unité d'analyse : le jour. */
export function DayView({ d, onAnalyzeRun }: { d: DayAnalysis; onAnalyzeRun: (a: ActivitySummary) => void }) {
  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-12">
      <AlertsBanner alerts={d.alerts} className="xl:col-span-12" />
      <HrvWeekCard rec={d.recovery} className="xl:col-span-7" />
      <SleepCard rec={d.recovery} className="xl:col-span-5" />
      <VitalsRow rec={d.recovery} day={d.day} className="xl:col-span-12" />
      <DayActivityCard day={d.day} className="xl:col-span-5" />
      <DayActivitiesCard activities={d.activities} onAnalyze={onAnalyzeRun} className="xl:col-span-7" />
      <TrendTable rec={d.recovery} className="xl:col-span-12" />
      <Warnings warnings={d.warnings} className="xl:col-span-12" />
    </div>
  )
}

/** Mode Santé & Récup appliqué à une course : récupération autour de la séance. */
export function ActivityRecoveryView({ a }: { a: ActivityAnalysis }) {
  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-12">
      <AlertsBanner alerts={a.alerts} className="xl:col-span-12" />
      <KpiGrid a={a.activity} className="xl:col-span-12" />
      <BiomechRow bio={a.biomechanics} className="xl:col-span-12" />
      {a.recovery ? (
        <>
          <HrvWeekCard rec={a.recovery} className="xl:col-span-7" />
          <SleepCard rec={a.recovery} className="xl:col-span-5" />
          <VitalsRow rec={a.recovery} day={null} className="xl:col-span-12" />
          <TrendTable rec={a.recovery} className="xl:col-span-12" />
        </>
      ) : (
        <p className="glass p-4 text-sm text-ink-3 xl:col-span-12">Données de récupération indisponibles pour cette séance.</p>
      )}
      <Warnings warnings={a.warnings} sources={a.dataSources} className="xl:col-span-12" />
    </div>
  )
}
