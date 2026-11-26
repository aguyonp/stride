import { cn } from '../../lib/cn'

interface ToggleProps {
  label: string
  /** The line under the label; omit for a bare switch (the label then becomes its accessible name). */
  hint?: string
  checked: boolean
  disabled?: boolean
  onChange: (next: boolean) => void
}

/**
 * The one switch of the app: same geometry, same focus ring, same words everywhere. It is a
 * `role="switch"` button, never wrapped in a `<label>` — a label around a button forwards the click
 * twice, which made the MCP switch look broken (two mutations, no net change).
 */
export function Toggle({ label, hint, checked, disabled, onChange }: ToggleProps) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div className="min-w-0">
        <p className="text-sm font-medium">{label}</p>
        {hint && <p className="text-xs text-ink-3">{hint}</p>}
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={cn(
          'relative h-6 w-11 flex-none rounded-full transition-colors disabled:opacity-60',
          checked ? 'bg-accent' : 'bg-line-strong',
        )}
      >
        <span
          aria-hidden="true"
          className={cn('absolute top-0.5 size-5 rounded-full bg-surface transition-all', checked ? 'left-[1.375rem]' : 'left-0.5')}
        />
      </button>
    </div>
  )
}
