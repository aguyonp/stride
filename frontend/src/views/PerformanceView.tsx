import { Zap } from 'lucide-react'
import type { ActivityAnalysis, Profile } from '../api/types'
import { AlertsBanner } from '../components/alerts/AlertsBanner'
import { Warnings } from '../components/alerts/Warnings'
import { SessionCharts } from '../components/charts/SessionCharts'
import { AcwrCard, BestEffortsCard, TargetPacesCard, ZonesCard } from '../components/performance/PerfCards'
import { RecoveryNightCard } from '../components/recovery/RecoveryNightCard'
import { BiomechRow } from '../components/sfp/BiomechanicsPanel'
import { KpiGrid } from '../components/sfp/KpiGrid'
import { DriftCard } from '../components/sfp/LoadCards'
import { SegmentsTable } from '../components/sfp/SegmentsTable'
import { Card } from '../components/ui/Card'
import { Value } from '../components/ui/Value'
import { fmtNumber } from '../lib/format'
import { teLabel } from '../lib/labels'

/** Mode Performance : zones, puissance, allures cibles, ACWR. */
export function PerformanceView({ a, profile }: { a: ActivityAnalysis; profile: Profile }) {
  const p = a.performance
  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-12">
      <AlertsBanner alerts={a.alerts} className="xl:col-span-12" max={2} />
      <KpiGrid a={a.activity} className="xl:col-span-12" />
      <ZonesCard hr={p?.timeInZonesS ?? a.load.hrZonesS} power={a.load.powerZonesS} className="xl:col-span-6" />
      <div className="grid content-start gap-4 xl:col-span-6">
        <AcwrCard acwr={p?.acwr ?? null} />
        <Card title="Puissance & effet d’entraînement" icon={<Zap className="size-4" />}>
          <dl className="num grid grid-cols-2 gap-2 sm:grid-cols-4">
            {[
              ['Moyenne', <Value value={p?.avgPower ?? a.activity.avgPower} format={(v) => fmtNumber(v)} unit="W" />],
              ['Normalisée', <Value value={p?.normPower ?? a.activity.normPower} format={(v) => fmtNumber(v)} unit="W" />],
              ['TE aérobie', <Value value={a.load.aerobicTE} format={(v) => fmtNumber(v, 1)} />],
              ['TE anaérobie', <Value value={a.load.anaerobicTE} format={(v) => fmtNumber(v, 1)} />],
            ].map(([k, v], i) => (
              <div key={i} className="glass-inset rounded-xl px-3 py-2">
                <dt className="text-[11px] text-ink-3">{k}</dt>
                <dd className="text-lg font-semibold">{v}</dd>
              </div>
            ))}
          </dl>
          <p className="num mt-2 text-xs text-ink-3">
            Training Load séance : <Value value={a.load.trainingLoad} format={(v) => fmtNumber(v)} className="font-semibold text-ink-2" />
            {a.activity.trainingEffectLabel && ` · effet principal : ${teLabel(a.activity.trainingEffectLabel)}`}
          </p>
        </Card>
      </div>
      <TargetPacesCard paces={p?.targetPaces ?? []} className="xl:col-span-6" />
      <div className="grid content-start gap-4 xl:col-span-6">
        <BestEffortsCard efforts={p?.bestEfforts ?? []} />
        <DriftCard drift={a.load.drift} compact />
      </div>
      <BiomechRow bio={a.biomechanics} className="xl:col-span-12" />
      <SessionCharts analysis={a} profile={profile} variant="performance" className="xl:col-span-12" />
      <SegmentsTable key={a.activity.id} segments={a.segments} showDynamics={false} className="xl:col-span-12" />
      <RecoveryNightCard rec={a.recovery} wide className="xl:col-span-12" />
      <Warnings warnings={a.warnings} sources={a.dataSources} className="xl:col-span-12" />
    </div>
  )
}
