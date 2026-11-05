import { BatteryCharging, Bike, Dumbbell, Flame, Footprints, HeartPulse, Scale } from 'lucide-react'
import type { ActivityView, CrossLoad, LoadRatio, Thresholds } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtDayMonth, fmtHM, fmtNumber, fmtSigned, fmtWeekdayShort, isNum } from '../../lib/format'
import { ACWR_VERDICT, DRIFT_VERDICT, teLabel } from '../../lib/labels'
import { Badge } from '../ui/Badge'
import { Card } from '../ui/Card'
import { StairsIcon } from '../ui/icons'
import { NA, Value } from '../ui/Value'
import { garminReason } from './reason'

function RatioGauge({ r, max }: { r: LoadRatio; max: number }) {
  const scale = 2
  const pct = (v: number) => `${Math.min(100, (v / scale) * 100)}%`
  const v = ACWR_VERDICT[r.verdict]
  return (
    <div className="glass-inset p-3.5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="flex items-center gap-2 text-sm font-semibold">
          <Scale className="size-4 text-ink-3" aria-hidden="true" /> Ratio de charge aiguë / chronique
        </p>
        <Badge level={v.level}>{v.label}</Badge>
      </div>
      <p className="num mt-1 text-3xl font-semibold tracking-tight">{isNum(r.ratio) ? fmtNumber(r.ratio, 2) : <NA />}</p>
      <p className="num text-xs text-ink-3">
        Aiguë 7 j {fmtNumber(r.acute)} · chronique {fmtNumber(r.chronic)} (28 j ÷ 4) · calculé par Stride
      </p>
      {isNum(r.ratio) && (
        <div className="mt-2.5" role="img" aria-label={`Ratio ${fmtNumber(r.ratio, 2)}, zone optimale 0,8 à 1,3, seuil d’alerte ${fmtNumber(max, 2)}`}>
          <div className="relative h-2.5 rounded-full bg-track">
            <div className="absolute inset-y-0 bg-ok/30" style={{ left: pct(0.8), width: `calc(${pct(1.3)} - ${pct(0.8)})` }} />
            <div className="absolute -inset-y-1 w-px bg-danger" style={{ left: pct(max) }} />
            <div className="absolute top-1/2 size-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-[3px] border-surface bg-ink shadow" style={{ left: pct(r.ratio) }} />
          </div>
          <div className="num relative mt-1 h-4 text-[10px] text-ink-3" aria-hidden="true">
            <span className="absolute left-0">0</span>
            <span className="absolute -translate-x-1/2" style={{ left: pct(0.8) }}>
              0,8
            </span>
            <span className="absolute -translate-x-1/2" style={{ left: pct(1.3) }}>
              1,3
            </span>
            <span className="absolute right-0">2</span>
          </div>
        </div>
      )}
    </div>
  )
}

export function LoadCard({ view, thresholds, className }: { view: ActivityView; thresholds: Thresholds | null; className?: string }) {
  const l = view.load
  const d = l.drift
  const dv = d ? DRIFT_VERDICT[d.verdict] : null
  const st = l.stamina ?? view.activity.stamina
  const gr = garminReason(view)
  return (
    <Card title="Charge, stamina & impact" icon={<Flame className="size-4" />} className={className}>
      <div className="space-y-3">
        <div className="glass-inset p-3.5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="flex items-center gap-2 text-sm font-semibold">
              <HeartPulse className="size-4 text-ink-3" aria-hidden="true" /> Dérive cardiaque
            </p>
            {dv && <Badge level={dv.level}>{dv.label}</Badge>}
          </div>
          {!d ? (
            <p className="mt-1.5 text-sm text-ink-3">
              <NA reason="séance trop courte ou FC insuffisante" />
            </p>
          ) : (
            <>
              <p className="num mt-1 text-3xl font-semibold tracking-tight">
                {fmtSigned(d.decouplingPct, 1)}
                <span className="ml-0.5 text-base font-medium text-ink-3">%</span>
                <span className="ml-2 text-xs font-medium text-ink-3">{d.method === 'pw:hr' ? 'Pw:Hr' : 'Allure:FC'}</span>
              </p>
              <p className="num text-xs text-ink-3">
                FC {d.firstHalfHR} → {d.secondHalfHR} bpm · dernier tiers {fmtSigned(d.lastThirdHrRiseBpm)} bpm · &lt; 5 % stable, &gt; 10 % élevée
              </p>
            </>
          )}
        </div>

        <div className="glass-inset p-3.5">
          <p className="flex items-center gap-2 text-sm font-semibold">
            <BatteryCharging className="size-4 text-ink-3" aria-hidden="true" /> Stamina
          </p>
          {!st || (!isNum(st.startPct) && !isNum(st.endPct)) ? (
            <p className="mt-1.5 text-sm text-ink-3">
              <NA reason={gr ?? 'non fournie par la montre'} />
            </p>
          ) : (
            <>
              <p className="num mt-1 text-lg font-semibold">
                {isNum(st.startPct) ? `${st.startPct} %` : 'N/D'} → {isNum(st.endPct) ? `${st.endPct} %` : 'N/D'}
                <span className="ml-2 text-xs font-medium text-ink-3">min {isNum(st.minPct) ? `${st.minPct} %` : 'N/D'}</span>
              </p>
              <div className="relative mt-2 h-2.5 rounded-full bg-track" aria-hidden="true">
                <div className="absolute inset-y-0 left-0 rounded-full" style={{ width: `${st.startPct ?? 0}%`, background: 'color-mix(in oklab, var(--c-stamina) 30%, transparent)' }} />
                <div className="absolute inset-y-0 left-0 rounded-full" style={{ width: `${st.endPct ?? 0}%`, background: 'var(--c-stamina)' }} />
              </div>
            </>
          )}
        </div>

        <dl className="num grid grid-cols-2 gap-2 text-sm sm:grid-cols-4">
          {[
            ['Garmin Load', <Value key="l" value={l.trainingLoad} format={(v) => fmtNumber(v)} reason={gr} />],
            ['Impact Load', <Value key="i" value={l.impactLoad} format={(v) => fmtNumber(v)} reason={gr} />],
            ['TE aérobie', isNum(l.aerobicTE) ? <span key="te">{fmtNumber(l.aerobicTE, 1)}<span className="ml-1 text-[11px] text-ink-3">{teLabel(view.activity.trainingEffectLabel) ?? ''}</span></span> : <NA key="te" reason={gr} />],
            ['Strava RE', <Value key="re" value={l.stravaRelativeEffort} format={(v) => fmtNumber(v)} />],
          ].map(([k, v], i) => (
            <div key={i} className="glass-inset min-w-0 rounded-xl px-2.5 py-2">
              <dt className="text-[11px] text-ink-3">{k}</dt>
              <dd className="truncate font-semibold">{v}</dd>
            </div>
          ))}
        </dl>

        {l.loadRatio ? (
          <RatioGauge r={l.loadRatio} max={thresholds?.loadRatioMax ?? 1.3} />
        ) : (
          <p className="glass-inset p-3.5 text-sm text-ink-3">
            Ratio de charge : <NA reason={gr ?? 'pas assez d’historique de charge'} />
          </p>
        )}
      </div>
    </Card>
  )
}

export function CrossLoadCard({ cross, thresholds, className, title = 'Charge croisée 48 h' }: { cross: CrossLoad; thresholds: Thresholds | null; className?: string; title?: string }) {
  const f = cross.floors
  const floorsMax = thresholds?.floorsPrevDayMax ?? 15
  const hot = isNum(f.prevDayUp) && f.prevDayUp > floorsMax
  return (
    <Card title={title} icon={<Bike className="size-4" />} subtitle={`J-1 complet + J jusqu’au départ · ${fmtDayMonth(cross.windowStart.slice(0, 10))} → ${fmtDayMonth(cross.windowEnd.slice(0, 10))}`} className={className}>
      <div className="space-y-3">
        <div className="glass-inset p-3.5">
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <p className="flex items-center gap-2 text-sm font-semibold">
              <Bike className="size-4 text-ink-3" aria-hidden="true" /> Vélo
            </p>
            <p className="num text-sm text-ink-2">
              {cross.bike.rides} sortie{cross.bike.rides > 1 ? 's' : ''} · {fmtNumber(cross.bike.distanceKm, 1)} km · {fmtHM(cross.bike.durationS)}
              {isNum(cross.bike.trainingLoad) && ` · load ${fmtNumber(cross.bike.trainingLoad)}`}
            </p>
          </div>
          {cross.bikeByDay.length > 0 && (
            <ul className="num mt-2 space-y-1 text-xs text-ink-3">
              {cross.bikeByDay.map((b) => (
                <li key={b.date} className="flex justify-between gap-2">
                  <span className="capitalize">
                    {fmtWeekdayShort(b.date)} {fmtDayMonth(b.date)}
                  </span>
                  <span>
                    {b.rides}× · {fmtNumber(b.distanceKm, 1)} km · {fmtHM(b.durationS)}
                    {isNum(b.avgHR) ? ` · ${b.avgHR} bpm` : ''}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
        <dl className="num grid grid-cols-2 gap-2 text-sm">
          <div className={cn('glass-inset rounded-xl px-2.5 py-2', hot && 'border-warn/40 bg-warn/10')}>
            <dt className="flex items-center gap-1 text-[11px] text-ink-3">
              <StairsIcon className="size-3.5" /> Étages J-1 (↑/↓)
            </dt>
            <dd className={cn('font-semibold', hot && 'text-warn')}>
              {isNum(f.prevDayUp) ? f.prevDayUp : 'N/D'} / {isNum(f.prevDayDown) ? f.prevDayDown : 'N/D'}
              <span className="ml-1 text-[11px] font-normal text-ink-3">seuil {floorsMax}</span>
            </dd>
          </div>
          <div className="glass-inset rounded-xl px-2.5 py-2">
            <dt className="flex items-center gap-1 text-[11px] text-ink-3">
              <StairsIcon className="size-3.5" /> Étages J (↑/↓)
            </dt>
            <dd className="font-semibold">
              {isNum(f.dayUp) ? f.dayUp : 'N/D'} / {isNum(f.dayDown) ? f.dayDown : 'N/D'}
            </dd>
          </div>
          <div className="glass-inset rounded-xl px-2.5 py-2">
            <dt className="flex items-center gap-1 text-[11px] text-ink-3">
              <Footprints className="size-3.5" aria-hidden="true" /> Pas J-1 / J
            </dt>
            <dd className="font-semibold">
              <Value value={cross.steps.prevDay} format={(v) => fmtNumber(v)} /> / <Value value={cross.steps.day} format={(v) => fmtNumber(v)} />
            </dd>
          </div>
          <div className="glass-inset rounded-xl px-2.5 py-2">
            <dt className="text-[11px] text-ink-3">Stress J-1</dt>
            <dd className="font-semibold">{isNum(cross.stressPrevDay) ? <>{cross.stressPrevDay}<span className="text-xs text-ink-3">/100</span></> : <NA />}</dd>
          </div>
        </dl>
        {cross.otherActivities.length > 0 && (
          <div>
            <p className="label-caps mb-1.5">Autres activités</p>
            <ul className="space-y-1 text-sm">
              {cross.otherActivities.map((a) => (
                <li key={a.id} className="flex items-center justify-between gap-2 rounded-xl px-1 py-1">
                  <span className="flex min-w-0 items-center gap-2">
                    <Dumbbell className="size-3.5 flex-none text-ink-3" aria-hidden="true" />
                    <span className="truncate">{a.name}</span>
                  </span>
                  <span className="num flex-none text-xs text-ink-3">
                    {fmtDayMonth(a.date)} · {fmtHM(a.durationS)}
                    {isNum(a.trainingLoad) ? ` · load ${a.trainingLoad}` : ''}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </Card>
  )
}
