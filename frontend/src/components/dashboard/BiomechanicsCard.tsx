import { ArrowDownRight, ArrowUpDown, Footprints, Minus, Mountain, Ruler, Scale, Timer, TrendingDown, Waves } from 'lucide-react'
import type { ReactNode } from 'react'
import type { ActivityView, AthleteProfile, FatigueDrift, GctBalance, Slope, SlopeBucket } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtDuration, fmtKm, fmtNumber, fmtPace, fmtSigned, isNum } from '../../lib/format'
import { DRIFT_VERDICT, SIDE_LABEL } from '../../lib/labels'
import { Ring } from '../charts/Ring'
import { Badge } from '../ui/Badge'
import { Card } from '../ui/Card'
import { NA, Stat, Value } from '../ui/Value'
import { garminReason } from './reason'

function Balance({ b, gapMax, injured }: { b: GctBalance; gapMax: number; injured: AthleteProfile['injuredSide'] }) {
  const span = Math.max(2.5, 2 * gapMax)
  const pos = (right: number) => ((Math.min(50 + span, Math.max(50 - span, right)) - (50 - span)) / (2 * span)) * 100
  const head = (
    <p className="flex items-center gap-2 text-sm font-semibold text-ink">
      <Scale className="size-4 text-ink-3" aria-hidden="true" /> Équilibre contact au sol G/D
    </p>
  )
  if (!b.available || b.leftPct === null || b.rightPct === null) {
    return (
      <div className="glass-inset p-3.5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          {head}
          <span className="chip border border-line bg-track text-ink-2">Non mesuré</span>
        </div>
        <p className="mt-2 text-xs text-ink-2">{b.reason ?? 'Donnée non fournie par le capteur.'}</p>
      </div>
    )
  }
  const level = b.alert ?? 'ok'
  const p = pos(b.rightPct)
  const color = level === 'danger' ? 'var(--danger)' : level === 'warn' ? 'var(--warn)' : 'var(--ok)'
  return (
    <div className="glass-inset p-3.5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        {head}
        <Badge level={level}>Écart {fmtNumber(b.gapPct, 1)} pt</Badge>
      </div>
      <div className="num mt-2 flex justify-between text-sm">
        <span>
          G{injured === 'left' || injured === 'both' ? ' · blessé' : ''} <strong className="text-lg">{fmtNumber(b.leftPct, 1)} %</strong>
        </span>
        <span>
          <strong className="text-lg">{fmtNumber(b.rightPct, 1)} %</strong> D{injured === 'right' || injured === 'both' ? ' · blessé' : ''}
        </span>
      </div>
      <div className="relative mt-1.5 h-3 rounded-full bg-track" role="img" aria-label={`Gauche ${fmtNumber(b.leftPct, 1)} %, droite ${fmtNumber(b.rightPct, 1)} %, écart ${fmtNumber(b.gapPct, 1)} point, seuil ${fmtNumber(gapMax, 1)}`}>
        <div className="absolute inset-y-0 bg-ok/25" style={{ left: `${pos(50 - gapMax / 2)}%`, width: `${pos(50 + gapMax / 2) - pos(50 - gapMax / 2)}%` }} />
        <div className="absolute -inset-y-1 left-1/2 w-px bg-ink-3" />
        <div className="absolute top-1/2 size-[18px] -translate-x-1/2 -translate-y-1/2 rounded-full border-[3px] border-surface shadow-md" style={{ left: `${p}%`, background: color }} />
      </div>
      <p className="mt-2 text-xs text-ink-2">
        {b.weakerSide ? `Contact plus court à ${SIDE_LABEL[b.weakerSide]}.` : 'Appuis symétriques.'} Tolérance : écart ≤ {fmtNumber(gapMax, 1)} pt.
      </p>
    </div>
  )
}

const signedCls = (v: number | null, bad: 'up' | 'down', warnAt: number) => (!isNum(v) ? '' : (bad === 'up' ? v > warnAt : v < -warnAt) ? 'text-warn' : '')

function DriftBlock({ d, maxPct, reason }: { d: FatigueDrift | null; maxPct: number; reason: string | null }) {
  const v = d && isNum(d.gctDeltaPct) ? DRIFT_VERDICT[d.verdict] : null
  const row = (label: string, start: ReactNode, end: ReactNode, delta: ReactNode) => (
    <tr className="border-b border-line last:border-0">
      <th scope="row" className="py-1.5 pr-2 text-left font-medium text-ink-2">
        {label}
      </th>
      <td className="px-2 py-1.5 text-right">{start}</td>
      <td className="px-2 py-1.5 text-right">{end}</td>
      <td className="py-1.5 pl-2 text-right font-semibold">{delta}</td>
    </tr>
  )
  return (
    <div className="glass-inset p-3.5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="flex items-center gap-2 text-sm font-semibold text-ink">
          <TrendingDown className="size-4 text-ink-3" aria-hidden="true" /> Dérive de fatigue
        </p>
        {v ? <Badge level={v.level}>{v.label}</Badge> : null}
      </div>
      {!d ? (
        <p className="mt-2 text-sm text-ink-3">
          <NA reason={reason ?? 'séance trop courte ou dynamique de course absente'} />
        </p>
      ) : (
        <>
          <p className="mt-0.5 text-xs text-ink-3">1er tiers vs dernier tiers du temps de course (pauses exclues)</p>
          <table className="num mt-2 w-full text-sm">
            <thead>
              <tr className="text-[11px] text-ink-3">
                <th scope="col" className="sr-only">
                  Métrique
                </th>
                <th scope="col" className="px-2 text-right font-medium">
                  Début
                </th>
                <th scope="col" className="px-2 text-right font-medium">
                  Fin
                </th>
                <th scope="col" className="pl-2 text-right font-medium">
                  Dérive
                </th>
              </tr>
            </thead>
            <tbody>
              {row(
                'GCT',
                <Value value={d.gctStartMs} format={(x) => fmtNumber(x)} unit="ms" />,
                <Value value={d.gctEndMs} format={(x) => fmtNumber(x)} unit="ms" />,
                isNum(d.gctDeltaMs) ? (
                  <span className={signedCls(d.gctDeltaPct, 'up', maxPct)}>
                    {fmtSigned(d.gctDeltaMs)} ms <span className="text-xs font-normal text-ink-3">({fmtSigned(d.gctDeltaPct, 1)} %)</span>
                  </span>
                ) : (
                  <NA />
                ),
              )}
              {row('OV', <Value value={d.voStartCm} format={(x) => fmtNumber(x, 1)} unit="cm" />, <Value value={d.voEndCm} format={(x) => fmtNumber(x, 1)} unit="cm" />, isNum(d.voDeltaCm) ? `${fmtSigned(d.voDeltaCm, 1)} cm` : <NA />)}
              {row(
                'Cadence',
                <Value value={d.cadenceStartSpm} format={(x) => fmtNumber(x)} unit="spm" />,
                <Value value={d.cadenceEndSpm} format={(x) => fmtNumber(x)} unit="spm" />,
                isNum(d.cadenceDeltaSpm) ? <span className={signedCls(d.cadenceDeltaSpm, 'down', 3)}>{fmtSigned(d.cadenceDeltaSpm)} spm</span> : <NA />,
              )}
            </tbody>
          </table>
          <p className="mt-1.5 text-[11px] text-ink-3">Verdict sur la dérive GCT : &lt; 3 % stable · 3–5 % modérée · &gt; 5 % élevée (seuil d’alerte {fmtNumber(maxPct, 1, { trim: true })} %)</p>
        </>
      )}
    </div>
  )
}

function SlopeBlock({ s, dropMax }: { s: Slope | null; dropMax: number }) {
  const head = (
    <p className="flex items-center gap-2 text-sm font-semibold text-ink">
      <Mountain className="size-4 text-ink-3" aria-hidden="true" /> Cadence & foulée selon la pente
    </p>
  )
  if (!s) {
    return (
      <div className="glass-inset p-3.5">
        {head}
        <p className="mt-2 text-sm text-ink-3">
          <NA reason="profil d’altitude absent (tapis, GPS coupé)" />
        </p>
      </div>
    )
  }
  const drop = s.downhillCadenceDropSpm
  const buckets: [string, ReactNode, SlopeBucket | null][] = [
    ['Plat', <Minus key="f" className="size-3.5" aria-hidden="true" />, s.flat],
    ['Descente', <ArrowDownRight key="d" className="size-3.5" aria-hidden="true" />, s.downhill],
    ['Montée', <ArrowUpDown key="u" className="size-3.5 rotate-45" aria-hidden="true" />, s.uphill],
  ]
  return (
    <div className="glass-inset p-3.5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        {head}
        {isNum(drop) && <Badge level={drop > dropMax ? 'warn' : 'ok'}>Chute en descente {fmtSigned(drop)} spm</Badge>}
      </div>
      <p className="mt-0.5 text-xs text-ink-3">Descente &lt; −{fmtNumber(s.thresholdPct, 1)} % · plat ±{fmtNumber(s.thresholdPct, 1)} % · montée &gt; +{fmtNumber(s.thresholdPct, 1)} %</p>
      <div className="scroll-thin mt-2 overflow-x-auto">
        <table className="num w-full min-w-[340px] text-sm whitespace-nowrap">
          <thead>
            <tr className="text-[11px] text-ink-3">
              <th scope="col" className="py-1 text-left font-medium">
                Pente
              </th>
              <th scope="col" className="px-1.5 text-right font-medium">
                Cadence
              </th>
              <th scope="col" className="px-1.5 text-right font-medium">
                Foulée moy / max
              </th>
              <th scope="col" className="px-1.5 text-right font-medium">
                Allure
              </th>
              <th scope="col" className="pl-1.5 text-right font-medium">
                Temps · dist.
              </th>
            </tr>
          </thead>
          <tbody>
            {buckets.map(([label, icon, b]) => (
              <tr key={label} className="border-t border-line">
                <th scope="row" className="py-1.5 text-left font-medium text-ink-2">
                  <span className="flex items-center gap-1.5">
                    {icon}
                    {label}
                  </span>
                </th>
                <td className="px-1.5 text-right font-semibold">{b ? <Value value={b.cadenceSpm} format={(x) => fmtNumber(x)} unit="spm" /> : <NA />}</td>
                <td className="px-1.5 text-right">{b ? <>{isNum(b.strideM) ? fmtNumber(b.strideM, 2) : 'N/D'} / {isNum(b.strideMaxM) ? fmtNumber(b.strideMaxM, 2) : 'N/D'} m</> : <NA />}</td>
                <td className="px-1.5 text-right">{b ? fmtPace(b.paceSecPerKm) : <NA />}</td>
                <td className="pl-1.5 text-right text-ink-3">{b ? `${fmtDuration(b.durationS)} · ${fmtKm(b.distanceM, 1)}` : '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

export function BiomechanicsCard({ view, profile, className }: { view: ActivityView; profile: AthleteProfile | null; className?: string }) {
  const bio = view.biomechanics
  const th = profile?.thresholds
  const tMin = th?.cadenceTargetMin ?? 170
  const tMax = th?.cadenceTargetMax ?? 180
  const cadMin = th?.cadenceMinSpm ?? 168
  const gr = garminReason(view)
  const cadCls = isNum(bio.cadenceAvgSpm) ? (bio.cadenceAvgSpm < cadMin ? 'text-danger' : bio.cadenceAvgSpm < tMin ? 'text-warn' : 'text-ink') : ''
  const voMax = th?.verticalOscillationMaxCm ?? 9.5
  const voHigh = isNum(bio.verticalOscillationCm) && bio.verticalOscillationCm > voMax
  return (
    <Card title="Biomécanique & dérive de fatigue" icon={<Footprints className="size-4" />} subtitle="Temps de course uniquement (marche et arrêts exclus)" className={className}>
      <div className="grid grid-cols-1 gap-3 xl:grid-cols-2 [&>*]:min-w-0">
        <div className="space-y-3">
          <div className="glass-inset flex items-center gap-4 p-3.5">
            <Ring value={bio.cadenceInTargetPct} size={84} stroke={9} label={isNum(bio.cadenceInTargetPct) ? `${bio.cadenceInTargetPct} % du temps dans la cible ${tMin}–${tMax} spm` : 'Temps dans la cible non disponible'}>
              <div>
                <p className="num text-lg leading-none font-bold text-ink">{isNum(bio.cadenceInTargetPct) ? `${bio.cadenceInTargetPct}%` : 'N/D'}</p>
                <p className="mt-0.5 text-[9px] font-medium tracking-wide text-ink-3 uppercase">cible</p>
              </div>
            </Ring>
            <div className="min-w-0 flex-1">
              <p className="text-xs font-medium text-ink-3">Cadence moyenne</p>
              <p className={cn('text-2xl font-semibold tracking-tight', cadCls)}>
                <Value value={bio.cadenceAvgSpm} format={(v) => fmtNumber(v)} unit="spm" />
              </p>
              <p className="num text-xs text-ink-2">
                Max <Value value={bio.cadenceMaxSpm} format={(v) => fmtNumber(v)} unit="spm" className="font-semibold text-ink" /> · cible {tMin}–{tMax} · seuil {cadMin}
              </p>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-2.5">
            <Stat label="Temps de contact" icon={<Timer />}>
              <Value value={bio.groundContactTimeMs} format={(v) => fmtNumber(v)} unit="ms" reason={gr} />
            </Stat>
            <Stat label="Oscillation verticale" icon={<ArrowUpDown />} sub={isNum(bio.verticalOscillationCm) ? `seuil ${fmtNumber(voMax, 1)} cm` : undefined} className={cn(voHigh && 'border-warn/40 bg-warn/10')}>
              <span className={cn(voHigh && 'text-warn')}>
                <Value value={bio.verticalOscillationCm} format={(v) => fmtNumber(v, 1)} unit="cm" reason={gr} />
              </span>
            </Stat>
            <Stat label="Ratio vertical" icon={<Waves />}>
              <Value value={bio.verticalRatioPct} format={(v) => fmtNumber(v, 1)} unit="%" reason={gr} />
            </Stat>
            <Stat label="Longueur de foulée" icon={<Ruler />} sub={view.biomechanics.slope?.downhill && isNum(view.biomechanics.slope.downhill.strideM) ? `descente ${fmtNumber(view.biomechanics.slope.downhill.strideM, 2)} m` : undefined}>
              <Value value={bio.strideLengthM} format={(v) => fmtNumber(v, 2)} unit="m" />
            </Stat>
          </div>
          <Balance b={bio.gctBalance} gapMax={th?.gctBalanceGapPct ?? 1.5} injured={profile?.injuredSide ?? null} />
        </div>
        <div className="space-y-3">
          <DriftBlock d={bio.fatigueDrift} maxPct={th?.gctDriftMaxPct ?? 5} reason={gr} />
          <SlopeBlock s={bio.slope} dropMax={th?.downhillCadenceDropSpm ?? 5} />
        </div>
      </div>
    </Card>
  )
}
