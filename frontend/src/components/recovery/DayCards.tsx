import { Activity, BatteryMedium, BedDouble, Brain, ChevronRight, Footprints, HeartPulse, Waves } from 'lucide-react'
import type { ActivitySummary, DaySummary, RecoveryContext } from '../../api/types'
import { cn } from '../../lib/cn'
import { addDays, capitalize, fmtDayMonth, fmtDuration, fmtHM, fmtKm, fmtNumber, fmtSigned, fmtTimeOfDay, fmtWeekdayShort, isNum } from '../../lib/format'
import { hrvStatus, SLEEP_QUALIFIER } from '../../lib/labels'
import { BarTrend } from '../charts/BarTrend'
import { SegmentedBar } from '../charts/SegmentedBar'
import { Badge } from '../ui/Badge'
import { Card } from '../ui/Card'
import { ActivityIcon, StairsIcon } from '../ui/icons'
import { NA, Stat, Value } from '../ui/Value'

/* ------------------------------------------------------------------ */
/* VFC 7 jours                                                        */
/* ------------------------------------------------------------------ */

export function HrvWeekCard({ rec, className }: { rec: RecoveryContext; className?: string }) {
  const h = rec.hrv
  const st = h ? hrvStatus(h.status, h.statusLabel) : null
  const band = h && isNum(h.baselineLowMs) && isNum(h.baselineHighMs) ? { min: h.baselineLowMs, max: h.baselineHighMs, label: `baseline ${h.baselineLowMs}–${h.baselineHighMs}` } : null
  return (
    <Card title="VFC — 7 jours" icon={<Activity className="size-4" />} subtitle="Variabilité cardiaque nocturne" className={className} action={st ? <Badge level={st.level}>{st.label}</Badge> : undefined}>
      <div className="mb-3 flex flex-wrap items-end gap-x-6 gap-y-1">
        <div>
          <p className="text-xs text-ink-3">Nuit dernière</p>
          <p className="text-4xl font-semibold tracking-tight">
            <Value value={h?.lastNightAvgMs} format={(v) => fmtNumber(v)} unit="ms" />
          </p>
        </div>
        <dl className="num flex gap-5 pb-1 text-sm">
          <div>
            <dt className="text-xs text-ink-3">Moy. 7 j</dt>
            <dd className="font-semibold">
              <Value value={h?.weeklyAvgMs} format={(v) => fmtNumber(v)} unit="ms" />
            </dd>
          </div>
          <div>
            <dt className="text-xs text-ink-3">vs 7 j</dt>
            <dd className={cn('font-semibold', isNum(h?.deltaVsWeeklyPct) && h.deltaVsWeeklyPct < -8 ? 'text-warn' : '')}>{isNum(h?.deltaVsWeeklyPct) ? `${fmtSigned(h.deltaVsWeeklyPct, 1)} %` : <NA />}</dd>
          </div>
          <div>
            <dt className="text-xs text-ink-3">Pic 5 min</dt>
            <dd className="font-semibold">
              <Value value={h?.lastNight5MinHighMs} format={(v) => fmtNumber(v)} unit="ms" />
            </dd>
          </div>
        </dl>
      </div>
      <BarTrend
        ariaLabel="VFC des 7 derniers jours"
        format={(v) => fmtNumber(v)}
        band={band}
        data={rec.trend.map((d, i) => {
          const s = hrvStatus(d.hrvStatus)
          return {
            key: d.date,
            label: `${fmtWeekdayShort(d.date).replace('.', '')} ${Number(d.date.slice(8))}`,
            value: d.hrvMs,
            highlight: i === rec.trend.length - 1,
            tooltip: `${fmtDayMonth(d.date)} · ${d.hrvMs === null ? 'N/D' : `${d.hrvMs} ms`}${s ? ` · ${s.label}` : ''}`,
          }
        })}
      />
    </Card>
  )
}

/* ------------------------------------------------------------------ */
/* Sommeil                                                            */
/* ------------------------------------------------------------------ */

export function SleepCard({ rec, className }: { rec: RecoveryContext; className?: string }) {
  const s = rec.sleep
  const q = s?.qualifier ? SLEEP_QUALIFIER[s.qualifier] : null
  return (
    <Card title="Sommeil" icon={<BedDouble className="size-4" />} subtitle={`Nuit du ${fmtDayMonth(addDays(rec.date, -1))} au ${fmtDayMonth(rec.date)}`} className={className} action={q ? <Badge level={q.level}>{q.label}</Badge> : undefined}>
      {!s ? (
        <p className="text-sm text-ink-3">Sommeil indisponible pour cette nuit.</p>
      ) : (
        <>
          <div className="flex items-end justify-between gap-3">
            <p className="text-4xl font-semibold tracking-tight">
              {isNum(s.score) ? (
                <span className="num">
                  {s.score}
                  <span className="text-base font-medium text-ink-3">/100</span>
                </span>
              ) : (
                <NA />
              )}
            </p>
            <div className="num pb-1 text-right text-sm">
              <p className="font-semibold text-ink">{fmtHM(s.durationS)}</p>
              <p className="text-xs text-ink-3">
                {s.bedtimeLocal ?? '—'} → {s.wakeLocal ?? '—'}
              </p>
            </div>
          </div>
          <SegmentedBar
            className="mt-3"
            ariaLabel={`Phases : profond ${fmtHM(s.deepS)}, léger ${fmtHM(s.lightS)}, paradoxal ${fmtHM(s.remS)}, éveil ${fmtHM(s.awakeS)}`}
            parts={[
              { key: 'deep', label: 'Profond', value: s.deepS ?? 0, color: 'var(--sleep-deep)', display: fmtHM(s.deepS) },
              { key: 'light', label: 'Léger', value: s.lightS ?? 0, color: 'var(--sleep-light)', display: fmtHM(s.lightS) },
              { key: 'rem', label: 'Paradoxal', value: s.remS ?? 0, color: 'var(--sleep-rem)', display: fmtHM(s.remS) },
              { key: 'awake', label: 'Éveil', value: s.awakeS ?? 0, color: 'var(--sleep-awake)', display: fmtHM(s.awakeS) },
            ]}
          />
          <p className="num mt-3 text-xs text-ink-3">
            FC moyenne pendant le sommeil : <Value value={s.avgSleepHR} format={(v) => fmtNumber(v)} unit="bpm" className="font-semibold text-ink-2" />
            {isNum(s.deepS) && s.deepS < 3600 && <span className="ml-2 text-info">· profond &lt; 1 h</span>}
          </p>
        </>
      )}
    </Card>
  )
}

/* ------------------------------------------------------------------ */
/* Constantes du jour                                                 */
/* ------------------------------------------------------------------ */

export function VitalsRow({ rec, day, className }: { rec: RecoveryContext; day: DaySummary | null; className?: string }) {
  const r = rec.restingHR
  const bb = rec.bodyBattery
  const bbLevel = !isNum(bb?.atWake) ? null : bb.atWake < 30 ? 'text-danger' : bb.atWake < 50 ? 'text-warn' : ''
  const stress = day?.stressAvg ?? rec.stressAvg
  return (
    <div className={cn('grid grid-cols-2 gap-2.5 sm:grid-cols-4', className)}>
      <Stat label="FC repos" icon={<HeartPulse />} emphasis sub={isNum(r?.avg7d) ? `moy. 7 j ${r.avg7d} bpm (${fmtSigned(r.delta)})` : undefined} className="glass">
        <Value value={r?.bpm} format={(v) => fmtNumber(v)} unit="bpm" />
      </Stat>
      <Stat label="Body Battery réveil" icon={<BatteryMedium />} emphasis sub={isNum(bb?.min) ? `min ${bb.min} · max ${bb?.max ?? 'N/D'}` : undefined} className="glass">
        <span className={cn(bbLevel)}>{isNum(bb?.atWake) ? <span className="num">{bb.atWake}<span className="text-sm text-ink-3">/100</span></span> : <NA />}</span>
      </Stat>
      <Stat label="Body Battery jour" icon={<Waves />} emphasis sub="rechargé / dépensé" className="glass">
        <span className="num">
          <Value value={day ? day.bodyBatteryCharged : bb?.charged} format={(v) => `+${v}`} />
          <span className="mx-1 text-ink-3">/</span>
          <Value value={day ? day.bodyBatteryDrained : bb?.drained} format={(v) => `−${v}`} />
        </span>
      </Stat>
      <Stat label="Stress moyen" icon={<Brain />} emphasis sub={isNum(stress) ? (stress < 26 ? 'repos' : stress < 51 ? 'faible' : stress < 76 ? 'moyen' : 'élevé') : undefined} className="glass">
        <Value value={stress} format={(v) => fmtNumber(v)} />
      </Stat>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* Activité du jour : pas, étages (mis en avant), intensité           */
/* ------------------------------------------------------------------ */

export function DayActivityCard({ day, className }: { day: DaySummary; className?: string }) {
  const stepsPct = isNum(day.steps) && isNum(day.stepGoal) && day.stepGoal > 0 ? Math.min(100, (day.steps / day.stepGoal) * 100) : null
  const floorsHigh = isNum(day.floorsUp) && day.floorsUp > 20
  const totalIntensity = (day.intensityModerateMin ?? 0) + 2 * (day.intensityVigorousMin ?? 0)
  return (
    <Card title="Activité du jour" icon={<Footprints className="size-4" />} className={className}>
      <div className={cn('rounded-2xl border p-4', floorsHigh ? 'border-warn/40 bg-warn/10' : 'border-accent/30 bg-accent/8')}>
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="flex items-center gap-1.5 text-xs font-semibold text-ink-2">
              <StairsIcon className="size-4" /> Étages montés
            </p>
            <p className={cn('num mt-1 text-5xl font-semibold tracking-tight', floorsHigh ? 'text-warn' : 'text-ink')}>{isNum(day.floorsUp) ? day.floorsUp : <NA />}</p>
            <p className="num text-xs text-ink-3">{isNum(day.floorsDown) ? `${day.floorsDown} descendus` : 'descente N/D'}</p>
          </div>
          {floorsHigh ? <Badge level="warn">&gt; 20</Badge> : isNum(day.floorsUp) ? <Badge level="ok">≤ 20</Badge> : null}
        </div>
        <p className="mt-2 text-xs text-ink-2">Facteur d’irritation rotulienne : chaque étage charge l’articulation fémoro-patellaire en flexion.</p>
      </div>

      <div className="mt-3">
        <div className="flex items-baseline justify-between text-sm">
          <span className="font-medium text-ink-2">Pas</span>
          <span className="num">
            <Value value={day.steps} format={(v) => fmtNumber(v)} className="font-semibold text-ink" />
            <span className="text-ink-3"> / {isNum(day.stepGoal) ? fmtNumber(day.stepGoal) : 'N/D'}</span>
          </span>
        </div>
        <div className="mt-1.5 h-2.5 overflow-hidden rounded-full bg-track" role="img" aria-label={stepsPct === null ? 'Objectif de pas non disponible' : `${Math.round(stepsPct)} % de l'objectif de pas`}>
          {stepsPct !== null && <div className="h-full rounded-full bg-gradient-to-r from-accent to-accent-2 transition-all duration-700" style={{ width: `${stepsPct}%` }} />}
        </div>
      </div>

      <div className="mt-3 grid grid-cols-3 gap-2 text-center">
        <div className="glass-inset rounded-xl px-2 py-2">
          <p className="text-[11px] text-ink-3">Modérée</p>
          <p className="num text-sm font-semibold">{isNum(day.intensityModerateMin) ? `${day.intensityModerateMin} min` : <NA />}</p>
        </div>
        <div className="glass-inset rounded-xl px-2 py-2">
          <p className="text-[11px] text-ink-3">Vigoureuse</p>
          <p className="num text-sm font-semibold">{isNum(day.intensityVigorousMin) ? `${day.intensityVigorousMin} min` : <NA />}</p>
        </div>
        <div className="glass-inset rounded-xl px-2 py-2">
          <p className="text-[11px] text-ink-3">Kcal actives</p>
          <p className="num text-sm font-semibold">{isNum(day.activeCalories) ? fmtNumber(day.activeCalories) : <NA />}</p>
        </div>
      </div>
      <p className="num mt-2 text-[11px] text-ink-3">Minutes d’intensité équivalentes : {totalIntensity} min (vigoureuse × 2)</p>
    </Card>
  )
}

/* ------------------------------------------------------------------ */
/* Activités du jour                                                  */
/* ------------------------------------------------------------------ */

export function DayActivitiesCard({ activities, onAnalyze, className }: { activities: ActivitySummary[]; onAnalyze: (a: ActivitySummary) => void; className?: string }) {
  return (
    <Card title="Activités du jour" icon={<Activity className="size-4" />} className={className}>
      {activities.length === 0 ? (
        <p className="text-sm text-ink-3">Aucune activité enregistrée ce jour.</p>
      ) : (
        <ul className="space-y-1.5">
          {activities.map((a) => (
            <li key={a.id} className="glass-inset flex items-center gap-3 px-3 py-2.5">
              <span className={cn('grid size-9 flex-none place-items-center rounded-xl', a.isRunning ? 'bg-accent/15 text-accent' : 'bg-track text-ink-2')}>
                <ActivityIcon type={a.type} className="size-4" />
              </span>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-semibold">{a.name}</p>
                <p className="num truncate text-xs text-ink-3">
                  {fmtTimeOfDay(a.startLocal)} · {a.typeLabel} · {fmtDuration(a.durationS)}
                  {isNum(a.distanceM) && ` · ${fmtKm(a.distanceM)}`}
                </p>
              </div>
              {a.isRunning && (
                <button type="button" className="btn btn-ghost h-8 min-h-0 px-2.5 text-xs" onClick={() => onAnalyze(a)}>
                  Analyser <ChevronRight className="size-3.5" aria-hidden="true" />
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}

/* ------------------------------------------------------------------ */
/* Tableau tendance 7 j                                               */
/* ------------------------------------------------------------------ */

export function TrendTable({ rec, className }: { rec: RecoveryContext; className?: string }) {
  const last = rec.trend.length - 1
  return (
    <Card title="Tendance 7 jours" icon={<Activity className="size-4" />} className={className}>
      <div className="scroll-thin -mx-4 overflow-x-auto px-4 sm:-mx-5 sm:px-5">
        <table className="num w-full min-w-[640px] border-separate border-spacing-0 text-sm whitespace-nowrap">
          <caption className="sr-only">Tendance des 7 derniers jours</caption>
          <thead>
            <tr className="text-left text-xs text-ink-3">
              {['Jour', 'VFC', 'Statut', 'Sommeil', 'Profond', 'FC repos', 'Pas', 'Étages', 'BB max', 'Load'].map((h) => (
                <th key={h} scope="col" className="border-b border-line px-2 py-2 font-medium first:pl-0">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rec.trend.map((d, i) => {
              const st = hrvStatus(d.hrvStatus)
              return (
                <tr key={d.date} className={cn(i === last && 'bg-accent/8')}>
                  <th scope="row" className="border-b border-line px-2 py-2 pl-0 text-left font-semibold whitespace-nowrap">
                    {capitalize(fmtWeekdayShort(d.date))} {fmtDayMonth(d.date)}
                  </th>
                  <td className="border-b border-line px-2 py-2">{d.hrvMs === null ? <NA /> : `${d.hrvMs} ms`}</td>
                  <td className="border-b border-line px-2 py-2">{st ? <span className={cn('text-xs font-semibold', { ok: 'text-ok', warn: 'text-warn', danger: 'text-danger', info: 'text-info' }[st.level])}>{st.label}</span> : <NA />}</td>
                  <td className="border-b border-line px-2 py-2">{d.sleepScore ?? <NA />}</td>
                  <td className="border-b border-line px-2 py-2">{d.deepS === null ? <NA /> : fmtHM(d.deepS)}</td>
                  <td className="border-b border-line px-2 py-2">{d.restingHR ?? <NA />}</td>
                  <td className="border-b border-line px-2 py-2">{d.steps === null ? <NA /> : fmtNumber(d.steps)}</td>
                  <td className={cn('border-b border-line px-2 py-2', isNum(d.floorsUp) && d.floorsUp > 20 && 'font-semibold text-warn')}>{d.floorsUp ?? <NA />}</td>
                  <td className="border-b border-line px-2 py-2">{d.bodyBatteryMax ?? <NA />}</td>
                  <td className="border-b border-line px-2 py-2">{d.trainingLoad ?? <span className="text-ink-3">—</span>}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </Card>
  )
}
