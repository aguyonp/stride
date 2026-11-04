import type { ReactNode } from 'react'
import { cn } from '../../lib/cn'
import { isNum, ND } from '../../lib/format'

/** Affiche « N/D » grisé si la valeur est absente (jamais 0). */
export function NA({ className }: { className?: string }) {
  return (
    <span className={cn('font-medium text-ink-3', className)} title="Non disponible">
      {ND}
    </span>
  )
}

interface ValueProps {
  value: number | null | undefined
  format: (v: number) => string
  unit?: string
  className?: string
  unitClassName?: string
}

/** Valeur numérique formatée + unité discrète, ou N/D. */
export function Value({ value, format, unit, className, unitClassName }: ValueProps) {
  if (!isNum(value)) return <NA className={className} />
  return (
    <span className={cn('num', className)}>
      {format(value)}
      {unit && <span className={cn('ml-0.5 text-[0.7em] font-medium text-ink-3', unitClassName)}>{unit}</span>}
    </span>
  )
}

interface StatProps {
  label: ReactNode
  children: ReactNode
  sub?: ReactNode
  icon?: ReactNode
  className?: string
  emphasis?: boolean
}

/** Tuile de métrique compacte (libellé, valeur, sous-texte). */
export function Stat({ label, children, sub, icon, className, emphasis }: StatProps) {
  return (
    <div className={cn('glass-inset min-w-0 px-3.5 py-3', className)}>
      <div className="flex items-center gap-1.5 text-xs font-medium text-ink-3">
        {icon && <span aria-hidden="true" className="text-ink-3 [&>svg]:size-3.5">{icon}</span>}
        <span className="leading-tight">{label}</span>
      </div>
      <div className={cn('mt-1 truncate font-semibold tracking-tight text-ink', emphasis ? 'text-2xl' : 'text-lg')}>{children}</div>
      {sub && <div className="mt-0.5 truncate text-xs text-ink-3">{sub}</div>}
    </div>
  )
}
