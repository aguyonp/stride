import { Minus, Plus } from 'lucide-react'
import { useId } from 'react'
import { cn } from '../../lib/cn'
import { fmtNumber } from '../../lib/format'

const tone = (v: number | null) => (v === null ? 'text-ink-3' : v >= 5 ? 'text-danger' : v >= 3 ? 'text-warn' : 'text-ok')

/** 0–10 pain scale, step 0.5, with large ± buttons for one-thumb use. */
export function PainScale({ label, value, onChange, hint }: { label: string; value: number | null; onChange: (v: number | null) => void; hint?: string }) {
  const id = useId()
  const set = (v: number) => onChange(Math.min(10, Math.max(0, Math.round(v * 2) / 2)))
  return (
    <div className="glass-inset px-3 py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <label htmlFor={id} className="block text-sm font-semibold text-ink">
            {label}
          </label>
          {hint && <p className="text-xs text-ink-3">{hint}</p>}
        </div>
        <p className={cn('num flex-none text-2xl leading-none font-semibold tracking-tight', tone(value))} aria-hidden="true">
          {value === null ? '—' : fmtNumber(value, 1, { trim: true })}
          <span className="text-sm font-medium text-ink-3">/10</span>
        </p>
      </div>
      <div className="mt-2 flex items-center gap-2">
        <button type="button" className="icon-btn size-11 rounded-2xl" onClick={() => set((value ?? 0.5) - 0.5)} aria-label={`${label} : diminuer de 0,5`} disabled={value === null || value <= 0}>
          <Minus className="size-5" />
        </button>
        <input
          id={id}
          type="range"
          min={0}
          max={10}
          step={0.5}
          value={value ?? 0}
          data-empty={value === null}
          onChange={(e) => set(Number(e.target.value))}
          aria-valuetext={value === null ? 'non renseigné' : `${fmtNumber(value, 1, { trim: true })} sur 10`}
          className="pain-range min-w-0 flex-1"
        />
        <button type="button" className="icon-btn size-11 rounded-2xl" onClick={() => set((value ?? -0.5) + 0.5)} aria-label={`${label} : augmenter de 0,5`} disabled={value !== null && value >= 10}>
          <Plus className="size-5" />
        </button>
      </div>
      <div className="mt-1 flex items-center justify-between text-[11px] text-ink-3">
        <span>0 aucune</span>
        {value !== null ? (
          <button type="button" className="rounded-md px-1.5 py-0.5 font-medium text-ink-2 underline-offset-2 hover:underline" onClick={() => onChange(null)}>
            Effacer
          </button>
        ) : (
          <span>non renseigné</span>
        )}
        <span>10 max</span>
      </div>
    </div>
  )
}
