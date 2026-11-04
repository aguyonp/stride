import type { ActivityAnalysis, Profile } from '../api/types'
import { AlertsBanner } from '../components/alerts/AlertsBanner'
import { Warnings } from '../components/alerts/Warnings'
import { SessionCharts } from '../components/charts/SessionCharts'
import { RecoveryNightCard } from '../components/recovery/RecoveryNightCard'
import { BiomechanicsPanel } from '../components/sfp/BiomechanicsPanel'
import { KpiGrid } from '../components/sfp/KpiGrid'
import { DescentCard, DriftCard } from '../components/sfp/LoadCards'
import { SegmentsTable } from '../components/sfp/SegmentsTable'

/** Mode Rééducation SFP : sécurité articulaire d'abord. */
export function SfpView({ a, profile }: { a: ActivityAnalysis; profile: Profile }) {
  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-12">
      <AlertsBanner alerts={a.alerts} className="xl:col-span-12" />
      <KpiGrid a={a.activity} className="xl:col-span-12" />
      <BiomechanicsPanel bio={a.biomechanics} profile={profile} className="xl:col-span-7" />
      <div className="grid content-start gap-4 md:grid-cols-2 xl:col-span-5 xl:grid-cols-1">
        <DescentCard gain={a.load.elevGainM} loss={a.load.elevLossM} profile={profile} />
        <DriftCard drift={a.load.drift} />
      </div>
      <SessionCharts analysis={a} profile={profile} variant="sfp" className="xl:col-span-12" />
      <SegmentsTable key={a.activity.id} segments={a.segments} className="xl:col-span-12" />
      <RecoveryNightCard rec={a.recovery} wide className="xl:col-span-12" />
      <Warnings warnings={a.warnings} sources={a.dataSources} className="xl:col-span-12" />
    </div>
  )
}
