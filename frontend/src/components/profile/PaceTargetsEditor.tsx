import { Plus, Trash2 } from 'lucide-react'
import { useId } from 'react'
import type { PaceDraft } from './profileUtils'

interface Props {
  rows: PaceDraft[]
  errors: Record<number, string>
  onChange: (rows: PaceDraft[]) => void
}

export function PaceTargetsEditor({ rows, errors, onChange }: Props) {
  const base = useId()
  const update = (i: number, patch: Partial<PaceDraft>) => onChange(rows.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  return (
    <div>
      {rows.length === 0 && <p className="mb-2 text-sm text-ink-3">Aucune allure cible. Ajoute par exemple « 10 km » (3:35–3:40) ou « Marathon ».</p>}
      <ul className="space-y-2">
        {rows.map((r, i) => {
          const id = `${base}-${i}`
          return (
            <li key={i} className="glass-inset p-3">
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-end">
                <div className="col-span-2 min-w-0 sm:col-span-1">
                  <label htmlFor={`${id}-l`} className="mb-1 block text-xs font-medium text-ink-2">
                    Nom
                  </label>
                  <input id={`${id}-l`} className="field" value={r.label} placeholder="10 km" maxLength={40} onChange={(e) => update(i, { label: e.target.value })} />
                </div>
                <div className="min-w-0">
                  <label htmlFor={`${id}-min`} className="mb-1 block text-xs font-medium text-ink-2">
                    Plus rapide
                  </label>
                  <input id={`${id}-min`} className="field num" inputMode="numeric" placeholder="3:35" value={r.min} aria-invalid={errors[i] ? true : undefined} onChange={(e) => update(i, { min: e.target.value })} />
                </div>
                <div className="min-w-0">
                  <label htmlFor={`${id}-max`} className="mb-1 block text-xs font-medium text-ink-2">
                    Plus lente
                  </label>
                  <input id={`${id}-max`} className="field num" inputMode="numeric" placeholder="3:40" value={r.max} aria-invalid={errors[i] ? true : undefined} onChange={(e) => update(i, { max: e.target.value })} />
                </div>
                <button type="button" className="icon-btn col-span-2 w-full rounded-xl sm:col-span-1 sm:w-11" onClick={() => onChange(rows.filter((_, j) => j !== i))} aria-label={`Supprimer l’allure ${r.label || i + 1}`}>
                  <Trash2 className="size-4" />
                </button>
              </div>
              {errors[i] && (
                <p className="mt-1.5 text-xs text-danger" role="alert">
                  {errors[i]}
                </p>
              )}
            </li>
          )
        })}
      </ul>
      <button type="button" className="btn btn-ghost mt-2" onClick={() => onChange([...rows, { label: '', min: '', max: '' }])}>
        <Plus className="size-4" aria-hidden="true" /> Ajouter une allure cible
      </button>
    </div>
  )
}
