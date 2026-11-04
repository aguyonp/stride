import { useRef, type KeyboardEvent, type ReactNode } from 'react'
import { cn } from '../../lib/cn'

export interface SegmentOption<T extends string> {
  value: T
  label: ReactNode
  icon?: ReactNode
  /** Libellé accessible si `label` est abrégé visuellement. */
  ariaLabel?: string
}

interface SegmentedControlProps<T extends string> {
  options: SegmentOption<T>[]
  value: T
  onChange: (v: T) => void
  ariaLabel: string
  className?: string
  size?: 'sm' | 'md'
}

/**
 * Contrôle segmenté accessible (radiogroup) : flèches ←/→, Début/Fin,
 * tabindex itinérant, indicateur glissant.
 */
export function SegmentedControl<T extends string>({ options, value, onChange, ariaLabel, className, size = 'md' }: SegmentedControlProps<T>) {
  const refs = useRef<(HTMLButtonElement | null)[]>([])
  const index = Math.max(0, options.findIndex((o) => o.value === value))

  const move = (next: number) => {
    const i = (next + options.length) % options.length
    onChange(options[i].value)
    refs.current[i]?.focus()
  }

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
      e.preventDefault()
      move(index + 1)
    } else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
      e.preventDefault()
      move(index - 1)
    } else if (e.key === 'Home') {
      e.preventDefault()
      move(0)
    } else if (e.key === 'End') {
      e.preventDefault()
      move(options.length - 1)
    }
  }

  return (
    <div
      role="radiogroup"
      aria-label={ariaLabel}
      onKeyDown={onKeyDown}
      className={cn('relative grid rounded-2xl border border-line bg-track/60 p-1', className)}
      style={{ gridTemplateColumns: `repeat(${options.length}, minmax(0, 1fr))` }}
    >
      <span
        aria-hidden="true"
        className="pointer-events-none absolute top-1 bottom-1 left-1 rounded-xl bg-gradient-to-br from-accent to-accent-2 shadow-[0_6px_18px_-8px_var(--accent)] transition-transform duration-300 ease-[var(--ease-out-soft)]"
        style={{ width: `calc((100% - 0.5rem) / ${options.length})`, transform: `translateX(${index * 100}%)` }}
      />
      {options.map((o, i) => {
        const selected = i === index
        return (
          <button
            key={o.value}
            ref={(el) => {
              refs.current[i] = el
            }}
            type="button"
            role="radio"
            aria-checked={selected}
            aria-label={o.ariaLabel}
            tabIndex={selected ? 0 : -1}
            onClick={() => onChange(o.value)}
            className={cn(
              'relative z-10 flex min-w-0 items-center justify-center gap-1.5 rounded-xl px-2 font-semibold transition-colors duration-200',
              size === 'sm' ? 'h-8 text-xs' : 'h-10 text-[0.8125rem] sm:text-sm',
              selected ? 'text-on-accent' : 'text-ink-2 hover:text-ink',
            )}
          >
            {o.icon && <span aria-hidden="true" className="flex-none [&>svg]:size-4">{o.icon}</span>}
            <span className="truncate">{o.label}</span>
          </button>
        )
      })}
    </div>
  )
}
