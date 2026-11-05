import { ChartLine } from 'lucide-react'
import { useMemo } from 'react'
import type { ActivityView, AthleteProfile } from '../../api/types'
import { fmtNumber, fmtPaceValue, isNum } from '../../lib/format'
import { isPause } from '../dashboard/segments'
import { Card } from '../ui/Card'
import { TimeSeriesStack, type ChartPanel } from './TimeSeriesStack'

const hasData = (xs: (number | null)[] | undefined) => !!xs && xs.some((v) => v !== null)
const avg = (xs: (number | null)[]) => {
  let s = 0
  let c = 0
  for (const v of xs) if (v !== null) (s += v), c++
  return c ? s / c : null
}

function paceDomain(values: (number | null)[], extra: number[]): { domain: [number, number]; ticks: number[] } {
  const vals = values.filter((v): v is number => v !== null)
  const lo = Math.floor((Math.min(...vals, ...extra) - 5) / 30) * 30
  const hi = Math.ceil((Math.max(...vals, ...extra) + 5) / 30) * 30
  const step = hi - lo > 150 ? 60 : 30
  const ticks: number[] = []
  for (let t = Math.ceil(lo / step) * step; t <= hi; t += step) ticks.push(t)
  return { domain: [lo, hi], ticks }
}

/** Session curves as synchronised small multiples (walking/stops shaded). */
export function SessionCharts({ view, profile, className }: { view: ActivityView; profile: AthleteProfile | null; className?: string }) {
  const { series: s, segments, biomechanics: bio, activity: a } = view
  const panels = useMemo(() => {
    const out: ChartPanel[] = []
    const th = profile?.thresholds
    const targets = (profile?.paceTargets ?? []).slice(0, 2)
    const colors = ['var(--accent)', 'var(--violet)']
    if (hasData(s.paceSecPerKm)) {
      const values = s.paceSecPerKm.map((v) => (v !== null && v > 480 ? null : v))
      const gap = hasData(s.gapSecPerKm) ? s.gapSecPerKm.map((v) => (v !== null && v > 480 ? null : v)) : null
      if (values.some((v) => v !== null)) {
        const extra = targets.flatMap((t) => [t.minSecPerKm, t.maxSecPerKm])
        const d = paceDomain([...values, ...(gap ?? [])], extra)
        const bands = targets.map((t, i) => ({ min: t.minSecPerKm, max: t.maxSecPerKm, label: t.label, color: colors[i] }))
        out.push({ id: 'pace', label: 'Allure', unit: '/km', values, color: 'var(--c-pace)', format: fmtPaceValue, reverse: true, ...d, bands, height: 96, summary: isNum(a.avgPaceSecPerKm) ? `moy ${fmtPaceValue(a.avgPaceSecPerKm)} /km` : undefined })
        if (gap) out.push({ id: 'gap', label: 'Allure ajustée à la pente (GAP)', unit: '/km', values: gap, color: 'var(--c-gap)', format: fmtPaceValue, reverse: true, ...d, height: 72, summary: isNum(a.gapSecPerKm) ? `moy ${fmtPaceValue(a.gapSecPerKm)} /km` : undefined })
      }
    }
    if (hasData(s.cadenceSpm) && a.isRunning) {
      const run = s.cadenceSpm.filter((v): v is number => v !== null && v > 140)
      const tMin = th?.cadenceTargetMin ?? 170
      const tMax = th?.cadenceTargetMax ?? 180
      if (run.length) {
        out.push({
          id: 'cad',
          label: 'Cadence de course',
          unit: 'spm',
          values: s.cadenceSpm.map((v) => (v !== null && v < 140 ? null : v)),
          color: 'var(--accent)',
          format: (v) => fmtNumber(v),
          summary: isNum(bio.cadenceAvgSpm) ? `moy ${bio.cadenceAvgSpm} spm` : undefined,
          bands: [{ min: tMin, max: tMax, label: `cible ${tMin}–${tMax}` }],
          domain: [Math.floor((Math.min(...run, tMin) - 4) / 5) * 5, Math.ceil((Math.max(...run, tMax) + 4) / 5) * 5],
          height: 88,
        })
      }
    }
    if (hasData(s.hr)) out.push({ id: 'hr', label: 'Fréquence cardiaque', unit: 'bpm', values: s.hr, color: 'var(--c-hr)', format: (v) => fmtNumber(v), summary: isNum(a.avgHR) ? `moy ${a.avgHR} bpm` : undefined })
    if (hasData(s.powerW)) {
      const m = avg(s.powerW)
      out.push({ id: 'pw', label: 'Puissance', unit: 'W', values: s.powerW, color: 'var(--c-power)', format: (v) => fmtNumber(v), summary: m !== null ? `moy ${Math.round(m)} W` : undefined, height: 72 })
    }
    if (hasData(s.gctMs)) out.push({ id: 'gct', label: 'Temps de contact au sol', unit: 'ms', values: s.gctMs, color: 'var(--info)', format: (v) => fmtNumber(v), summary: isNum(bio.groundContactTimeMs) ? `moy ${bio.groundContactTimeMs} ms` : undefined, height: 64 })
    if (hasData(s.voCm)) {
      out.push({
        id: 'vo',
        label: 'Oscillation verticale',
        unit: 'cm',
        values: s.voCm,
        color: 'var(--accent-2)',
        format: (v) => fmtNumber(v, 1),
        summary: isNum(bio.verticalOscillationCm) ? `moy ${fmtNumber(bio.verticalOscillationCm, 1)} cm` : undefined,
        threshold: th ? { value: th.verticalOscillationMaxCm, label: `seuil ${fmtNumber(th.verticalOscillationMaxCm, 1)} cm` } : undefined,
        height: 64,
      })
    }
    if (hasData(s.staminaPct)) out.push({ id: 'stamina', label: 'Stamina', unit: '%', values: s.staminaPct, color: 'var(--c-stamina)', format: (v) => fmtNumber(v), domain: [0, 100], ticks: [0, 50, 100], height: 64, summary: a.stamina && isNum(a.stamina.endPct) ? `fin ${a.stamina.endPct} %` : undefined })
    if (hasData(s.elevationM)) out.push({ id: 'elev', label: 'Altitude', unit: 'm', values: s.elevationM, color: 'var(--c-elev)', format: (v) => fmtNumber(v), area: true, height: 64, summary: isNum(view.load.elevLossM) ? `D− ${view.load.elevLossM} m` : undefined })
    return out
  }, [s, bio, profile, a, view.load.elevLossM])

  const blocks = segments.blocks
  const shades = blocks.filter(isPause).map((b) => ({ start: b.startOffsetS, end: b.startOffsetS + b.durationS }))
  const list = blocks.length ? blocks : segments.laps
  const segmentAt = (t: number) => list.find((b) => t >= b.startOffsetS && t < b.startOffsetS + b.durationS)?.label ?? null

  return (
    <Card title="Courbes de la séance" icon={<ChartLine className="size-4" />} subtitle="Survole ou utilise les flèches pour lire les valeurs" className={className}>
      {panels.length === 0 || s.tS.length < 2 ? (
        <p className="text-sm text-ink-3">Pas de série temporelle disponible pour cette activité.</p>
      ) : (
        <TimeSeriesStack
          tS={s.tS}
          panels={panels}
          shades={shades}
          shadeLabel={blocks.some((b) => b.kind === 'walk') ? 'Pauses marche' : 'Récupérations'}
          ariaLabel={`Graphiques de la séance : ${panels.map((p) => p.label.toLowerCase()).join(', ')}`}
          segmentAt={segmentAt}
        />
      )}
    </Card>
  )
}
