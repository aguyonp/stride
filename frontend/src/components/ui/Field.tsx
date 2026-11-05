import { useId, type ReactNode } from 'react'
import { cn } from '../../lib/cn'

interface FieldProps {
  label: ReactNode
  hint?: ReactNode
  error?: ReactNode
  className?: string
  children: (props: { id: string; 'aria-describedby'?: string; 'aria-invalid'?: boolean }) => ReactNode
}

/** Label + control + hint/error, wired for screen readers. */
export function Field({ label, hint, error, className, children }: FieldProps) {
  const id = useId()
  const hintId = `${id}-hint`
  const describedBy = error || hint ? hintId : undefined
  return (
    <div className={cn('min-w-0', className)}>
      <label htmlFor={id} className="mb-1.5 block text-sm font-medium text-ink-2">
        {label}
      </label>
      {children({ id, 'aria-describedby': describedBy, 'aria-invalid': error ? true : undefined })}
      {(error || hint) && (
        <p id={hintId} className={cn('mt-1.5 text-xs', error ? 'text-danger' : 'text-ink-3')} role={error ? 'alert' : undefined}>
          {error || hint}
        </p>
      )}
    </div>
  )
}

export function Checkbox({ checked, onChange, label, description, className, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: ReactNode; description?: ReactNode; className?: string; disabled?: boolean }) {
  const id = useId()
  return (
    <label
      htmlFor={id}
      className={cn(
        'flex min-h-11 cursor-pointer items-start gap-3 rounded-2xl border px-3 py-2.5 transition-colors',
        checked ? 'border-accent/45 bg-accent/10' : 'border-line hover:bg-track/60',
        disabled && 'cursor-not-allowed opacity-60',
        className,
      )}
    >
      <input id={id} type="checkbox" className="peer sr-only" checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} />
      <span
        aria-hidden="true"
        className={cn(
          'mt-0.5 grid size-5 flex-none place-items-center rounded-md border transition-colors peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-accent',
          checked ? 'border-transparent bg-accent text-on-accent' : 'border-line-strong bg-transparent',
        )}
      >
        {checked && (
          <svg viewBox="0 0 16 16" className="size-3.5" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round">
            <path d="m3.5 8.5 3 3 6-7" />
          </svg>
        )}
      </span>
      <span className="min-w-0">
        <span className="block text-sm font-medium text-ink">{label}</span>
        {description && <span className="block text-xs text-ink-3">{description}</span>}
      </span>
    </label>
  )
}

/** Toggle chip (aria-pressed). */
export function ChipToggle({ pressed, onChange, children, disabled, title, className }: { pressed: boolean; onChange: (v: boolean) => void; children: ReactNode; disabled?: boolean; title?: string; className?: string }) {
  return (
    <button
      type="button"
      aria-pressed={pressed}
      disabled={disabled}
      title={title}
      onClick={() => onChange(!pressed)}
      className={cn(
        'inline-flex min-h-9 items-center gap-1.5 rounded-full border px-3 text-[0.8125rem] font-semibold transition-colors',
        pressed ? 'border-accent/50 bg-accent/15 text-ink' : 'border-line bg-transparent text-ink-3 hover:text-ink-2',
        disabled && 'opacity-45',
        className,
      )}
    >
      <span
        aria-hidden="true"
        className={cn('grid size-4 flex-none place-items-center rounded-full border', pressed ? 'border-transparent bg-accent text-on-accent' : 'border-line-strong')}
      >
        {pressed && (
          <svg viewBox="0 0 16 16" className="size-3" fill="none" stroke="currentColor" strokeWidth={3} strokeLinecap="round" strokeLinejoin="round">
            <path d="m3.5 8.5 3 3 6-7" />
          </svg>
        )}
      </span>
      {children}
    </button>
  )
}
