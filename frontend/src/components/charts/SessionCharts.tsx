import { ChartLine } from 'lucide-react'
import { useMemo } from 'react'
import type { ActivityAnalysis, Profile } from '../../api/types'
import { fmtNumber, fmtPaceValue, isNum } from '../../lib/format'
import { Card } from '../ui/Card'
import { isPause } from '../sfp/SegmentsTable'
import { TimeSeriesStack, type ChartPanel } from './TimeSeriesStack'

const hasData = (xs: (number | null)[]) => xs.some((v) => v !== null)
const avg = (xs: (number | null)[]) => {
  let s = 0
  let c = 0
  for (const v of xs) if (v !== null) (s += v), c++
  return c ? s / c : null
}

interface Props {
  analysis: ActivityAnalysis
  profile: Profile
  variant: 'sfp' | 'performance'
  className?: string
}

/** Courbes de la séance en petits multiples synchronisés. */
export function SessionCharts({ analysis, profile, variant, className }: Props) {
  const { series: s, segments, biomechanics: bio } = analysis
  const panels = useMemo(() => {
    const out: ChartPanel[] = []
    if (variant === 'performance' && hasData(s.paceSecPerKm)) {
      // Allures > 8:00/km (marche, arrêts) masquées : les pauses sont grisées.
      const values = s.paceSecPerKm.map((v) => (v !== null && v > 480 ? null : v))
      const vals = values.filter((v): v is number => v !== null)
      const fastest = Math.min(...vals, profile.pace10kMinSecPerKm)
      const slowest = Math.max(...vals, profile.paceMarathonMaxSecPerKm)
      const lo = Math.floor((fastest - 5) / 30) * 30
      const hi = Math.ceil((slowest + 5) / 30) * 30
      const step = hi - lo > 150 ? 60 : 30
      const ticks: number[] = []
      for (let t = Math.ceil(lo / step) * step; t <= hi; t += step) ticks.push(t)
      out.push({
        id: 'pace',
        label: 'Allure',
        unit: '/km',
        values,
        color: 'var(--c-pace)',
        format: fmtPaceValue,
        reverse: true,
        domain: [lo, hi],
        ticks,
        summary: isNum(analysis.activity.avgPaceSecPerKm) ? `moy ${fmtPaceValue(analysis.activity.avgPaceSecPerKm)} /km` : undefined,
        bands: [
          { min: profile.pace10kMinSecPerKm, max: profile.pace10kMaxSecPerKm, label: '10 km', color: 'var(--accent)' },
          { min: profile.paceMarathonMinSecPerKm, max: profile.paceMarathonMaxSecPerKm, label: 'marathon', color: 'var(--accent-2)' },
        ],
        height: 96,
      })
    }
    if (variant === 'sfp' && hasData(s.cadenceSpm)) {
      out.push({
        id: 'cad',
        label: 'Cadence de course',
        unit: 'spm',
        // La marche (< 140 spm) est masquée : les pauses sont déjà grisées.
        values: s.cadenceSpm.map((v) => (v !== null && v < 140 ? null : v)),
        color: 'var(--accent)',
        format: (v) => fmtNumber(v),
        summary: isNum(bio.cadenceAvgSpm) ? `moy ${bio.cadenceAvgSpm} spm` : undefined,
        bands: [{ min: profile.cadenceTargetMin, max: profile.cadenceTargetMax, label: `cible ${profile.cadenceTargetMin}–${profile.cadenceTargetMax}` }],
        domain: (() => {
          const run = s.cadenceSpm.filter((v): v is number => v !== null && v > 140)
          const lo = Math.min(...run, profile.cadenceTargetMin) - 4
          const hi = Math.max(...run, profile.cadenceTargetMax) + 4
          return [Math.floor(lo / 5) * 5, Math.ceil(hi / 5) * 5] as [number, number]
        })(),
        height: 96,
      })
    }
    if (hasData(s.hr)) {
      out.push({ id: 'hr', label: 'Fréquence cardiaque', unit: 'bpm', values: s.hr, color: 'var(--c-hr)', format: (v) => fmtNumber(v), summary: isNum(analysis.activity.avgHR) ? `moy ${analysis.activity.avgHR} bpm` : undefined })
    }
    if (hasData(s.powerW)) {
      const a = avg(s.powerW)
      out.push({ id: 'pw', label: 'Puissance', unit: 'W', values: s.powerW, color: 'var(--c-power)', format: (v) => fmtNumber(v), summary: a !== null ? `moy ${Math.round(a)} W` : undefined })
    }
    if (variant === 'sfp' && hasData(s.voCm)) {
      out.push({
        id: 'vo',
        label: 'Oscillation verticale',
        unit: 'cm',
        values: s.voCm,
        color: 'var(--accent-2)',
        format: (v) => fmtNumber(v, 1),
        summary: isNum(bio.verticalOscillationCm) ? `moy ${fmtNumber(bio.verticalOscillationCm, 1)} cm` : undefined,
        threshold: { value: profile.verticalOscillationAlertCm, label: `seuil ${profile.verticalOscillationAlertCm} cm` },
        height: 64,
      })
    }
    if (hasData(s.elevationM)) {
      out.push({ id: 'elev', label: 'Altitude', unit: 'm', values: s.elevationM, color: 'var(--c-elev)', format: (v) => fmtNumber(v), area: true, height: 64, summary: isNum(analysis.load.elevLossM) ? `D− ${analysis.load.elevLossM} m` : undefined })
    }
    return out
  }, [s, bio, profile, variant, analysis.activity, analysis.load.elevLossM])

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
