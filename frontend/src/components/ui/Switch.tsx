import { useId, type ReactNode } from 'react'
import { cn } from '../../lib/cn'

interface SwitchProps {
  checked: boolean
  onChange: (v: boolean) => void
  label: ReactNode
  description?: ReactNode
  className?: string
  compact?: boolean
}

export function Switch({ checked, onChange, label, description, className, compact }: SwitchProps) {
  const id = useId()
  return (
    <div className={cn('flex items-center justify-between gap-3', className)}>
      <div className="min-w-0">
        <label htmlFor={id} className={cn('block font-medium text-ink', compact ? 'text-sm' : 'text-[0.9rem]')}>
          {label}
        </label>
        {description && <p className="text-xs text-ink-3">{description}</p>}
      </div>
      <button
        id={id}
        type="button"
        role="switch"
        aria-checked={checked}
        onClick={() => onChange(!checked)}
        className={cn(
          'relative h-6 w-11 flex-none rounded-full border transition-colors duration-200',
          checked ? 'border-transparent bg-accent' : 'border-line-strong bg-track',
        )}
      >
        <span
          aria-hidden="true"
          className={cn(
            'absolute top-1/2 left-0.5 size-[1.125rem] -translate-y-1/2 rounded-full shadow-sm transition-transform duration-200',
            checked ? 'translate-x-5 bg-on-accent' : 'translate-x-0 bg-ink-2',
          )}
        />
      </button>
    </div>
  )
}
