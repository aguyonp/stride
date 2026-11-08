import { RotateCcw } from 'lucide-react'
import { useId } from 'react'
import type { Thresholds } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtNumber } from '../../lib/format'
import { THRESHOLD_FIELDS, type ThresholdDraft } from './profileUtils'
import { t } from '../../i18n'

interface Props {
  draft: ThresholdDraft
  defaults: Thresholds | null
  errors: Partial<Record<keyof Thresholds, string>>
  onChange: (d: ThresholdDraft) => void
  onReset?: () => void
}

export function ThresholdsEditor({ draft, defaults, errors, onChange, onReset }: Props) {
  const base = useId()
  return (
    <div>
      <div className="grid gap-x-3 gap-y-3 sm:grid-cols-2 xl:grid-cols-3">
        {THRESHOLD_FIELDS.map((f) => {
          const id = `${base}-${f.key}`
          const err = errors[f.key]
          const def = defaults?.[f.key]
          const custom = def !== undefined && draft[f.key] !== '' && Number(draft[f.key].replace(',', '.')) !== def
          return (
            <div key={f.key} className="min-w-0">
              <label htmlFor={id} className="mb-1 block truncate text-xs font-medium text-ink-2" title={f.label}>
                {f.label}
              </label>
              <div className="relative">
                <input
                  id={id}
                  type="number"
                  inputMode="decimal"
                  step={f.step}
                  min={0}
                  className={cn('field num pr-16', custom && 'border-accent/60')}
                  value={draft[f.key]}
                  aria-invalid={err ? true : undefined}
                  aria-describedby={`${id}-hint`}
                  onChange={(e) => onChange({ ...draft, [f.key]: e.target.value })}
                />
                <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-xs text-ink-3">{f.unit}</span>
              </div>
              <p id={`${id}-hint`} className={cn('mt-1 text-[11px]', err ? 'text-danger' : 'text-ink-3')}>
                {err ?? (def !== undefined ? `${t('settings.thresholds.preset', { value: fmtNumber(def, f.step < 1 ? 2 : 0, { trim: true }) })}${custom ? t('settings.thresholds.custom') : ''}` : ' ')}
              </p>
            </div>
          )
        })}
      </div>
      {onReset && defaults && (
        <button type="button" className="btn btn-ghost mt-2" onClick={onReset}>
          <RotateCcw className="size-4" aria-hidden="true" />
          {t('settings.thresholds.reset')}
        </button>
      )}
    </div>
  )
}
