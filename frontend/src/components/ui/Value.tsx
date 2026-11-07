import type { ReactNode } from 'react'
import { cn } from '../../lib/cn'
import { isNum, ND } from '../../lib/format'

/** Greyed « N/D » for missing values (never 0). */
export function NA({ className, reason }: { className?: string; reason?: string | null }) {
  return (
    <span className={cn('font-medium text-ink-3', className)} title={reason ?? 'Non disponible'}>
      {ND}
      {reason && <span className="ml-1 text-[0.72em] font-normal">· {reason}</span>}
    </span>
  )
}

/** Discreet notice for a health metric the backend could not compute (never a fallback number). */
export function Insufficient({ className }: { className?: string }) {
  return <span className={cn('text-xs font-normal text-ink-3', className)}>Données insuffisantes</span>
}

interface ValueProps {
  value: number | null | undefined
  format: (v: number) => string
  unit?: string
  className?: string
  unitClassName?: string
  reason?: string | null
}

export function Value({ value, format, unit, className, unitClassName, reason }: ValueProps) {
  if (!isNum(value)) return <NA className={className} reason={reason} />
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
