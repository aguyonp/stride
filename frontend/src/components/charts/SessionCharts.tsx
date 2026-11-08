import { ChartLine } from 'lucide-react'
import { useMemo } from 'react'
import type { ActivityView, AthleteProfile } from '../../api/types'
import { fmtNumber, fmtPaceValue, fmtPct, isNum } from '../../lib/format'
import { isPause } from '../dashboard/segments'
import { Card } from '../ui/Card'
import { TimeSeriesStack, type ChartPanel } from './TimeSeriesStack'
import { t } from '../../i18n'

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
  for (let tick = Math.ceil(lo / step) * step; tick <= hi; tick += step) ticks.push(tick)
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
        const extra = targets.flatMap((tp) => [tp.minSecPerKm, tp.maxSecPerKm])
        const d = paceDomain([...values, ...(gap ?? [])], extra)
        const bands = targets.map((tp, i) => ({ min: tp.minSecPerKm, max: tp.maxSecPerKm, label: tp.label, color: colors[i] }))
        out.push({ id: 'pace', label: t('session.charts.pace'), unit: '/km', values, color: 'var(--c-pace)', format: fmtPaceValue, reverse: true, ...d, bands, height: 96, summary: isNum(a.avgPaceSecPerKm) ? t('session.charts.avg', { value: `${fmtPaceValue(a.avgPaceSecPerKm)} /km` }) : undefined })
        if (gap) out.push({ id: 'gap', label: t('session.charts.gap'), unit: '/km', values: gap, color: 'var(--c-gap)', format: fmtPaceValue, reverse: true, ...d, height: 72, summary: isNum(a.gapSecPerKm) ? t('session.charts.avg', { value: `${fmtPaceValue(a.gapSecPerKm)} /km` }) : undefined })
      }
    }
    if (hasData(s.cadenceSpm) && a.isRunning) {
      const run = s.cadenceSpm.filter((v): v is number => v !== null && v > 140)
      const tMin = th?.cadenceTargetMin ?? 170
      const tMax = th?.cadenceTargetMax ?? 180
      if (run.length) {
        out.push({
          id: 'cad',
          label: t('session.charts.cadence'),
          unit: 'spm',
          values: s.cadenceSpm.map((v) => (v !== null && v < 140 ? null : v)),
          color: 'var(--accent)',
          format: (v) => fmtNumber(v),
          summary: isNum(bio.cadenceAvgSpm) ? t('session.charts.avg', { value: `${bio.cadenceAvgSpm} spm` }) : undefined,
          bands: [{ min: tMin, max: tMax, label: t('session.charts.target', { min: tMin, max: tMax }) }],
          domain: [Math.floor((Math.min(...run, tMin) - 4) / 5) * 5, Math.ceil((Math.max(...run, tMax) + 4) / 5) * 5],
          height: 88,
        })
      }
    }
    if (hasData(s.hr)) out.push({ id: 'hr', label: t('session.charts.hr'), unit: 'bpm', values: s.hr, color: 'var(--c-hr)', format: (v) => fmtNumber(v), summary: isNum(a.avgHR) ? t('session.charts.avg', { value: `${a.avgHR} bpm` }) : undefined })
    if (hasData(s.powerW)) {
      const m = avg(s.powerW)
      out.push({ id: 'pw', label: t('session.charts.power'), unit: 'W', values: s.powerW, color: 'var(--c-power)', format: (v) => fmtNumber(v), summary: m !== null ? t('session.charts.avg', { value: `${Math.round(m)} W` }) : undefined, height: 72 })
    }
    if (hasData(s.gctMs)) out.push({ id: 'gct', label: t('session.charts.gct'), unit: 'ms', values: s.gctMs, color: 'var(--info)', format: (v) => fmtNumber(v), summary: isNum(bio.groundContactTimeMs) ? t('session.charts.avg', { value: `${bio.groundContactTimeMs} ms` }) : undefined, height: 64 })
    if (hasData(s.voCm)) {
      out.push({
        id: 'vo',
        label: t('session.charts.vo'),
        unit: 'cm',
        values: s.voCm,
        color: 'var(--accent-2)',
        format: (v) => fmtNumber(v, 1),
        summary: isNum(bio.verticalOscillationCm) ? t('session.charts.avg', { value: `${fmtNumber(bio.verticalOscillationCm, 1)} cm` }) : undefined,
        threshold: th ? { value: th.verticalOscillationMaxCm, label: t('session.charts.threshold', { value: fmtNumber(th.verticalOscillationMaxCm, 1) }) } : undefined,
        height: 64,
      })
    }
    if (hasData(s.staminaPct)) out.push({ id: 'stamina', label: t('session.charts.stamina'), unit: '%', values: s.staminaPct, color: 'var(--c-stamina)', format: (v) => fmtNumber(v), domain: [0, 100], ticks: [0, 50, 100], height: 64, summary: a.stamina && isNum(a.stamina.endPct) ? t('session.charts.end', { value: fmtPct(a.stamina.endPct, 0) }) : undefined })
    if (hasData(s.elevationM)) out.push({ id: 'elev', label: t('session.charts.elevation'), unit: 'm', values: s.elevationM, color: 'var(--c-elev)', format: (v) => fmtNumber(v), area: true, height: 64, summary: isNum(view.load.elevLossM) ? t('session.charts.loss', { value: view.load.elevLossM }) : undefined })
    return out
  }, [s, bio, profile, a, view.load.elevLossM])

  const blocks = segments.blocks
  const shades = blocks.filter(isPause).map((b) => ({ start: b.startOffsetS, end: b.startOffsetS + b.durationS }))
  const list = blocks.length ? blocks : segments.laps
  const segmentAt = (ts: number) => list.find((b) => ts >= b.startOffsetS && ts < b.startOffsetS + b.durationS)?.label ?? null

  return (
    <Card title={t('session.charts.title')} icon={<ChartLine className="size-4" />} subtitle={t('session.charts.subtitle')} className={className}>
      {panels.length === 0 || s.tS.length < 2 ? (
        <p className="text-sm text-ink-3">{t('session.charts.empty')}</p>
      ) : (
        <TimeSeriesStack
          tS={s.tS}
          panels={panels}
          shades={shades}
          shadeLabel={blocks.some((b) => b.kind === 'walk') ? t('session.charts.walkPauses') : t('session.charts.recoveries')}
          ariaLabel={t('session.charts.aria', { list: panels.map((p) => p.label.toLowerCase()).join(', ') })}
          segmentAt={segmentAt}
        />
      )}
    </Card>
  )
}
