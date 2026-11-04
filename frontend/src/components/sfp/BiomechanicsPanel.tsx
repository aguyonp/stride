import { ArrowUpDown, Footprints, Ruler, Timer, Waves } from 'lucide-react'
import type { ReactNode } from 'react'
import type { Biomechanics, Profile } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtNumber, isNum } from '../../lib/format'
import { Ring } from '../charts/Ring'
import { Card } from '../ui/Card'
import { NA, Stat, Value } from '../ui/Value'
import { BalanceGauge } from './BalanceGauge'

interface Props {
  bio: Biomechanics
  profile: Profile
  className?: string
}

/** Panneau biomécanique (mode SFP) : équilibre G/D, cadence, GCT, OV, ratio vertical, foulée. */
export function BiomechanicsPanel({ bio, profile, className }: Props) {
  const voHigh = isNum(bio.verticalOscillationCm) && bio.verticalOscillationCm > profile.verticalOscillationAlertCm
  const voDanger = isNum(bio.verticalOscillationCm) && bio.verticalOscillationCm > profile.verticalOscillationAlertCm + 1
  const cadLow = isNum(bio.cadenceAvgSpm) && bio.cadenceAvgSpm < profile.cadenceTargetMin
  const cadDanger = isNum(bio.cadenceAvgSpm) && bio.cadenceAvgSpm < 170
  const inTarget = bio.cadenceInTargetPct

  return (
    <Card title="Biomécanique rotule & impacts" icon={<Footprints className="size-4" />} className={className} subtitle="Hors marche et arrêts">
      <BalanceGauge balance={bio.gctBalance} profile={profile} />

      <div className="glass-inset mt-3 flex items-center gap-4 p-3.5">
        <Ring
          value={inTarget}
          size={84}
          stroke={9}
          label={isNum(inTarget) ? `${inTarget} % du temps de course dans la cible ${profile.cadenceTargetMin}–${profile.cadenceTargetMax} spm` : 'Temps dans la cible non disponible'}
        >
          <div>
            <p className="num text-lg leading-none font-bold text-ink">{isNum(inTarget) ? `${inTarget}%` : 'N/D'}</p>
            <p className="mt-0.5 text-[9px] font-medium tracking-wide text-ink-3 uppercase">cible</p>
          </div>
        </Ring>
        <div className="min-w-0 flex-1">
          <p className="text-xs font-medium text-ink-3">Cadence moyenne</p>
          <p className={cn('text-2xl font-semibold tracking-tight', cadDanger ? 'text-danger' : cadLow ? 'text-warn' : 'text-ink')}>
            <Value value={bio.cadenceAvgSpm} format={(v) => fmtNumber(v)} unit="spm" />
          </p>
          <p className="num text-xs text-ink-2">
            Max <Value value={bio.cadenceMaxSpm} format={(v) => fmtNumber(v)} unit="spm" className="font-semibold text-ink" /> · cible {profile.cadenceTargetMin}–{profile.cadenceTargetMax}
          </p>
          {isNum(bio.cadenceBelow170Pct) && bio.cadenceBelow170Pct > 0 && <p className="num mt-0.5 text-xs text-ink-3">{bio.cadenceBelow170Pct} % du temps sous 170 spm</p>}
        </div>
      </div>

      <div className="mt-3 grid grid-cols-2 gap-2.5">
        <Stat label="Temps de contact" icon={<Timer />}>
          <Value value={bio.groundContactTimeMs} format={(v) => fmtNumber(v)} unit="ms" />
        </Stat>
        <Stat
          label="Oscillation verticale"
          icon={<ArrowUpDown />}
          sub={isNum(bio.verticalOscillationCm) ? `seuil ${fmtNumber(profile.verticalOscillationAlertCm, 1)} cm` : undefined}
          className={cn(voDanger ? 'border-danger/40 bg-danger/10' : voHigh && 'border-warn/40 bg-warn/10')}
        >
          <span className={cn(voDanger ? 'text-danger' : voHigh && 'text-warn')}>
            <Value value={bio.verticalOscillationCm} format={(v) => fmtNumber(v, 1)} unit="cm" />
          </span>
        </Stat>
        <Stat label="Ratio vertical" icon={<Waves />}>
          <Value value={bio.verticalRatioPct} format={(v) => fmtNumber(v, 1)} unit="%" />
        </Stat>
        <Stat label="Longueur de foulée" icon={<Ruler />}>
          <Value value={bio.strideLengthM} format={(v) => fmtNumber(v, 2)} unit="m" />
        </Stat>
      </div>
    </Card>
  )
}

/** Rangée biomécanique compacte (modes Performance / Récup). */
export function BiomechRow({ bio, className }: { bio: Biomechanics; className?: string }) {
  const g = bio.gctBalance
  const items: [string, ReactNode][] = [
    ['Cadence', <Value value={bio.cadenceAvgSpm} format={(v) => fmtNumber(v)} unit="spm" />],
    ['GCT', <Value value={bio.groundContactTimeMs} format={(v) => fmtNumber(v)} unit="ms" />],
    ['OV', <Value value={bio.verticalOscillationCm} format={(v) => fmtNumber(v, 1)} unit="cm" />],
    ['RV', <Value value={bio.verticalRatioPct} format={(v) => fmtNumber(v, 1)} unit="%" />],
    ['Foulée', <Value value={bio.strideLengthM} format={(v) => fmtNumber(v, 2)} unit="m" />],
    ['G/D', g.available && isNum(g.leftPct) && isNum(g.rightPct) ? <span className="num">{`${fmtNumber(g.leftPct, 1)}/${fmtNumber(g.rightPct, 1)}`}</span> : <NA />],
  ]
  return (
    <section className={cn('glass animate-in px-4 py-3', className)} aria-label="Biomécanique (résumé)">
      <div className="flex items-center gap-2">
        <Footprints className="size-4 text-accent" aria-hidden="true" />
        <h2 className="text-sm font-semibold">Biomécanique</h2>
      </div>
      <dl className="mt-2 grid grid-cols-3 gap-x-3 gap-y-2 sm:grid-cols-6">
        {items.map(([k, v]) => (
          <div key={k} className="min-w-0">
            <dt className="text-[11px] text-ink-3">{k}</dt>
            <dd className="truncate text-sm font-semibold text-ink">{v}</dd>
          </div>
        ))}
      </dl>
    </section>
  )
}
