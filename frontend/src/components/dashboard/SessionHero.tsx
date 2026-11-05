import type { ActivityView, AlertLevel, Thresholds } from '../../api/types'
import { fmtDuration, fmtNumber, fmtPaceValue, isNum } from '../../lib/format'
import { LevelIcon } from '../ui/Badge'

interface Signal {
  label: string
  value: string
  unit?: string
  hint?: string
  level: AlertLevel | null
}

function signals(v: ActivityView, t: Thresholds | null): Signal[] {
  const a = v.activity
  const out: Signal[] = []
  const cad = v.biomechanics.cadenceAvgSpm ?? a.avgCadenceSpm
  if (a.isRunning && isNum(cad)) {
    const level: AlertLevel | null = !t ? null : cad < t.cadenceMinSpm ? 'danger' : cad < t.cadenceTargetMin ? 'warn' : 'ok'
    out.push({ label: 'Cadence', value: fmtNumber(cad), unit: 'spm', hint: t ? `cible ${t.cadenceTargetMin}–${t.cadenceTargetMax}` : undefined, level })
  } else if (isNum(a.avgPower)) out.push({ label: 'Puissance', value: fmtNumber(a.avgPower), unit: 'W', level: null })
  if (isNum(a.avgHR)) {
    const drift = v.load.drift?.decouplingPct
    out.push({ label: 'FC moy.', value: fmtNumber(a.avgHR), unit: 'bpm', hint: isNum(drift) ? `dérive ${fmtNumber(drift, 1)} %` : undefined, level: isNum(drift) ? (drift > 10 ? 'danger' : drift >= 5 ? 'warn' : 'ok') : null })
  }
  const ef = v.efficiency
  if (ef?.eligible && isNum(ef.efSpeed)) {
    const ch = ef.trendChangePct
    out.push({ label: 'Efficience', value: fmtNumber(ef.efSpeed, 2), hint: isNum(ch) ? `${ch >= 0 ? '+' : '−'}${fmtNumber(Math.abs(ch), 1)} % / 8 sem.` : 'm/min/bpm', level: isNum(ch) ? (ch < -3 ? 'warn' : 'ok') : null })
  } else if (isNum(a.trainingLoad)) {
    out.push({ label: 'Charge', value: fmtNumber(a.trainingLoad), hint: isNum(a.aerobicTE) ? `TE ${fmtNumber(a.aerobicTE, 1)}` : undefined, level: null })
  } else if (isNum(a.elevLossM)) {
    out.push({ label: 'D−', value: `−${fmtNumber(a.elevLossM)}`, unit: 'm', level: null })
  }
  return out.slice(0, 3)
}

/** Compact hero: the three numbers people look for, then three key signals. */
export function SessionHero({ view, thresholds }: { view: ActivityView; thresholds: Thresholds | null }) {
  const a = view.activity
  const main: [string, string, string?][] = [
    ['Distance', isNum(a.distanceM) ? fmtNumber(a.distanceM / 1000, 2) : 'N/D', 'km'],
    ['Durée', isNum(a.durationS) ? fmtDuration(a.durationS) : 'N/D'],
    ['Allure', isNum(a.avgPaceSecPerKm) ? fmtPaceValue(a.avgPaceSecPerKm) : 'N/D', '/km'],
  ]
  const sig = signals(view, thresholds)
  return (
    <div className="mt-4 border-t border-line pt-4">
      <dl className="grid grid-cols-3 gap-3">
        {main.map(([k, v, u]) => (
          <div key={k} className="min-w-0">
            <dt className="text-xs text-ink-3">{k}</dt>
            <dd className="num truncate text-2xl font-semibold tracking-tight text-ink sm:text-[2rem] sm:leading-tight">
              {v}
              {u && v !== 'N/D' && <span className="ml-0.5 text-sm font-medium text-ink-3">{u}</span>}
            </dd>
          </div>
        ))}
      </dl>
      {sig.length > 0 && (
        <ul className="mt-3 grid grid-cols-3 gap-2" aria-label="Signaux clés">
          {sig.map((s) => (
            <li key={s.label} className="flex min-w-0 items-center gap-2 rounded-xl bg-track/60 px-2.5 py-2 sm:px-3">
              {s.level && <LevelIcon level={s.level} className="size-3.5 flex-none max-sm:hidden" />}
              <div className="min-w-0">
                <p className="flex items-center gap-1 text-[11px] leading-tight text-ink-3">
                  {s.level && <LevelIcon level={s.level} className="size-3 flex-none sm:hidden" />}
                  {s.label}
                </p>
                <p className="num truncate text-sm font-semibold text-ink">
                  {s.value}
                  {s.unit && <span className="ml-0.5 text-xs font-medium text-ink-3">{s.unit}</span>}
                  {s.hint && <span className="ml-1.5 text-xs font-normal text-ink-3 max-sm:block max-sm:ml-0 max-sm:truncate">{s.hint}</span>}
                </p>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
