import { Activity, Bone, Footprints, PenLine, Zap, type LucideIcon } from 'lucide-react'
import { useId } from 'react'
import type { FocusArea, FocusPreset, InjuredSide } from '../../api/types'
import { cn } from '../../lib/cn'
import { SegmentedControl } from '../ui/SegmentedControl'
import { FOCUS_HINT, FOCUS_ORDER, focusTitle, SIDE_OPTIONS } from './profileUtils'

const ICON: Record<FocusArea, LucideIcon> = { none: Zap, knee_pfps: Bone, achilles: Footprints, shin_splints: Activity, other: PenLine }

interface FocusPickerProps {
  presets: FocusPreset[] | undefined
  area: FocusArea
  customLabel: string
  side: InjuredSide | null
  onArea: (a: FocusArea) => void
  onCustomLabel: (s: string) => void
  onSide: (s: InjuredSide | null) => void
  compact?: boolean
}

export function FocusPicker({ presets, area, customLabel, side, onArea, onCustomLabel, onSide, compact }: FocusPickerProps) {
  const name = useId()
  const labelId = useId()
  return (
    <div className="space-y-4">
      <fieldset>
        <legend className="mb-2 text-sm font-medium text-ink-2">Zone surveillée</legend>
        <div className={cn('grid gap-2', compact ? 'sm:grid-cols-2' : 'sm:grid-cols-2')} role="radiogroup">
          {FOCUS_ORDER.map((a) => {
            const Icon = ICON[a]
            const checked = a === area
            const emphasis = presets?.find((p) => p.focusArea === a)?.emphasis ?? []
            return (
              <label
                key={a}
                className={cn(
                  'flex min-w-0 cursor-pointer items-start gap-3 rounded-2xl border p-3 transition-colors has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-accent',
                  checked ? 'border-accent/55 bg-accent/10' : 'border-line hover:bg-track/60',
                  a === 'none' && 'sm:col-span-2',
                )}
              >
                <input type="radio" name={name} value={a} checked={checked} onChange={() => onArea(a)} className="sr-only" />
                <span className={cn('grid size-9 flex-none place-items-center rounded-xl', checked ? 'bg-accent text-on-accent' : 'bg-track text-ink-2')} aria-hidden="true">
                  <Icon className="size-[1.125rem]" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block text-sm font-semibold text-ink">{focusTitle(presets, a)}</span>
                  <span className="block text-xs text-ink-3">{FOCUS_HINT[a]}</span>
                  {!compact && emphasis.length > 0 && <span className="mt-1 block truncate text-[11px] text-ink-3">Focus : {emphasis.join(' · ')}</span>}
                </span>
                <span
                  aria-hidden="true"
                  className={cn('mt-1 grid size-4 flex-none place-items-center rounded-full border', checked ? 'border-accent' : 'border-line-strong')}
                >
                  {checked && <span className="size-2 rounded-full bg-accent" />}
                </span>
              </label>
            )
          })}
        </div>
      </fieldset>

      {area === 'other' && (
        <div>
          <label htmlFor={labelId} className="mb-1.5 block text-sm font-medium text-ink-2">
            Nom de la zone
          </label>
          <input
            id={labelId}
            className="field"
            value={customLabel}
            maxLength={60}
            placeholder="Ex. Hanche droite, Fascia plantaire…"
            onChange={(e) => onCustomLabel(e.target.value)}
          />
        </div>
      )}

      {area !== 'none' && (
        <div>
          <p className="mb-1.5 text-sm font-medium text-ink-2" id={`${labelId}-side`}>
            Côté concerné
          </p>
          <SegmentedControl
            ariaLabel="Côté concerné"
            value={side ?? 'none'}
            onChange={(v) => onSide(v === 'none' ? null : (v as InjuredSide))}
            options={SIDE_OPTIONS.map((o) => ({ value: o.value, label: o.label }))}
          />
        </div>
      )}
    </div>
  )
}
