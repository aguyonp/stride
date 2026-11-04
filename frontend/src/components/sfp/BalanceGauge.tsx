import { RadioReceiver, Scale } from 'lucide-react'
import type { GctBalance, Profile } from '../../api/types'
import { fmtNumber } from '../../lib/format'
import { SIDE_LABEL } from '../../lib/labels'
import { Badge } from '../ui/Badge'

/** Demi-largeur de l'échelle (en points autour de 50) : 2× le seuil, au moins 2,5. */
const spanFor = (t: number) => Math.max(2.5, 2 * t)

/**
 * Jauge d'équilibre du temps de contact au sol G/D.
 * Barre centrée 50/50 ; le curseur se déplace vers le côté qui reste le plus longtemps au sol.
 * Zone verte = tolérance (écart ≤ seuil), ambre jusqu'à 2× seuil, rouge au-delà.
 */
export function BalanceGauge({ balance: b, profile }: { balance: GctBalance; profile: Pick<Profile, 'gctBalanceAlertPct' | 'injuredSide'> }) {
  const t = profile.gctBalanceAlertPct
  const half = t / 2 // écart |G−D| = 2 × |D − 50|
  const injured = profile.injuredSide
  const span = spanFor(t)
  const LO = 50 - span
  const HI = 50 + span
  const pos = (rightPct: number) => ((Math.min(HI, Math.max(LO, rightPct)) - LO) / (HI - LO)) * 100

  if (!b.available || b.leftPct === null || b.rightPct === null) {
    return (
      <div className="glass-inset p-3.5">
        <div className="flex items-center justify-between gap-2">
          <p className="flex items-center gap-2 text-sm font-semibold text-ink">
            <Scale className="size-4 text-ink-3" aria-hidden="true" />
            Équilibre contact au sol G/D
          </p>
          <span className="chip border border-line bg-track text-ink-2">
            <RadioReceiver className="size-3.5" aria-hidden="true" />
            Non mesuré
          </span>
        </div>
        <div className="relative mt-4 mb-1 h-3 overflow-hidden rounded-full bg-track" aria-hidden="true">
          <div className="absolute inset-0 bg-[repeating-linear-gradient(135deg,transparent_0_6px,var(--line)_6px_8px)]" />
          <div className="absolute inset-y-0 left-1/2 w-px bg-line-strong" />
        </div>
        <div className="flex justify-between text-xs text-ink-3" aria-hidden="true">
          <span>G {injured === 'left' && '· blessé'}</span>
          <span>50 / 50</span>
          <span>D {injured === 'right' && '· blessé'}</span>
        </div>
        <p className="mt-2.5 text-xs text-ink-2">{b.reason ?? 'Donnée non fournie par le capteur.'}</p>
      </div>
    )
  }

  const level = b.alert ?? 'ok'
  const p = pos(b.rightPct)
  const color = level === 'danger' ? 'var(--danger)' : level === 'warn' ? 'var(--warn)' : 'var(--ok)'
  const zone = (from: number, to: number) => ({ left: `${pos(50 + from)}%`, width: `${pos(50 + to) - pos(50 + from)}%` })
  const shorter = b.weakerSide
  const label = `Équilibre gauche ${fmtNumber(b.leftPct, 1)} %, droite ${fmtNumber(b.rightPct, 1)} %, écart ${fmtNumber(b.gapPct, 1)} point${shorter ? `, contact plus court à ${SIDE_LABEL[shorter]}` : ''}. Seuil d'alerte ${fmtNumber(t, 1)} point.`

  return (
    <div className="glass-inset p-3.5">
      <div className="flex items-center justify-between gap-2">
        <p className="flex items-center gap-2 text-sm font-semibold text-ink">
          <Scale className="size-4 text-ink-3" aria-hidden="true" />
          Équilibre contact au sol G/D
        </p>
        <Badge level={level}>Écart {fmtNumber(b.gapPct, 1)} pt</Badge>
      </div>

      <div className="mt-3 flex items-end justify-between">
        <div className="num">
          <span className="text-xs text-ink-3">G{injured === 'left' && ' · blessé'}</span>
          <p className="text-xl font-semibold tracking-tight text-ink">
            {fmtNumber(b.leftPct, 1)}
            <span className="text-sm text-ink-3"> %</span>
          </p>
        </div>
        <div className="num text-right">
          <span className="text-xs text-ink-3">D{injured === 'right' && ' · blessé'}</span>
          <p className="text-xl font-semibold tracking-tight text-ink">
            {fmtNumber(b.rightPct, 1)}
            <span className="text-sm text-ink-3"> %</span>
          </p>
        </div>
      </div>

      <div className="relative mt-2 h-3 rounded-full bg-track" role="img" aria-label={label}>
        {/* zones : danger | warn | ok | warn | danger */}
        <div className="absolute inset-y-0 rounded-l-full bg-danger/12" style={zone(-span, -t)} />
        <div className="absolute inset-y-0 bg-warn/18" style={zone(-t, -half)} />
        <div className="absolute inset-y-0 bg-ok/25" style={zone(-half, half)} />
        <div className="absolute inset-y-0 bg-warn/18" style={zone(half, t)} />
        <div className="absolute inset-y-0 rounded-r-full bg-danger/12" style={zone(t, span)} />
        <div className="absolute -inset-y-1 left-1/2 w-px bg-ink-3" />
        {/* remplissage centre → curseur */}
        <div className="absolute inset-y-[3px] rounded-full transition-all duration-700" style={{ left: `${Math.min(50, p)}%`, width: `${Math.abs(p - 50)}%`, background: color }} />
        <div className="absolute top-1/2 size-[18px] -translate-x-1/2 -translate-y-1/2 rounded-full border-[3px] border-surface shadow-md transition-all duration-700" style={{ left: `${p}%`, background: color }} />
      </div>
      <div className="num mt-1.5 flex justify-between text-[10px] text-ink-3" aria-hidden="true">
        <span>{fmtNumber(LO, 1, { trim: true })}</span>
        <span>50</span>
        <span>{fmtNumber(HI, 1, { trim: true })}</span>
      </div>
      <p className="mt-2 text-xs text-ink-2">
        {shorter ? (
          <>
            Contact plus court à <strong className="font-semibold text-ink">{SIDE_LABEL[shorter]}</strong>
            {shorter === injured ? ' (côté blessé : protection probable)' : ''}.
          </>
        ) : (
          'Appuis parfaitement symétriques.'
        )}{' '}
        Tolérance : écart ≤ {fmtNumber(t, 1)} pt.
      </p>
    </div>
  )
}
