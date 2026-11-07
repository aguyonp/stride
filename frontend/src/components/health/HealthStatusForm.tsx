import { Plus, X } from 'lucide-react'
import type { BodySide, BodyZone, HealthStatusInput, HealthZone } from '../../api/types'
import { ALL_BODY_ZONES, BODY_SIDE_LABEL, BODY_ZONE_META } from '../bodymap/bodyMapMeta'
import { SegmentedControl } from '../ui/SegmentedControl'
import { addZone, setMode } from './healthStatus'

const SIDES: BodySide[] = ['left', 'right', 'both']

/** Controlled editor for the health status: good health, or one or more tracked zones with side and start date. */
export function HealthStatusForm({ value, onChange, today }: { value: HealthStatusInput; onChange: (v: HealthStatusInput) => void; today: string }) {
  const patch = (i: number, p: Partial<HealthZone>) => onChange({ ...value, zones: value.zones.map((z, j) => (j === i ? { ...z, ...p } : z)) })
  const remove = (i: number) => {
    const zones = value.zones.filter((_, j) => j !== i)
    onChange({ mode: zones.length ? 'tracking' : 'performance', zones })
  }
  return (
    <div className="space-y-3">
      <SegmentedControl<HealthStatusInput['mode']>
        ariaLabel="État de santé"
        value={value.mode}
        onChange={(m) => onChange(setMode(value, m, today))}
        options={[
          { value: 'performance', label: 'Bonne santé' },
          { value: 'tracking', label: 'Souci suivi' },
        ]}
      />
      {value.mode === 'tracking' && (
        <ul className="space-y-2" aria-label="Zones suivies">
          {value.zones.map((z, i) => (
            <li key={`${z.zone}-${i}`} className="glass-inset grid gap-2 p-3 sm:grid-cols-[minmax(0,1fr)_auto_auto_auto] sm:items-center">
              <select className="field" aria-label="Zone" value={z.zone} onChange={(e) => patch(i, { zone: e.target.value as BodyZone })}>
                {ALL_BODY_ZONES.map((k) => (
                  <option key={k} value={k} disabled={k !== z.zone && value.zones.some((x) => x.zone === k)}>
                    {BODY_ZONE_META[k].label}
                  </option>
                ))}
              </select>
              <SegmentedControl<BodySide> ariaLabel="Côté" size="sm" value={z.side} onChange={(side) => patch(i, { side })} options={SIDES.map((s) => ({ value: s, label: BODY_SIDE_LABEL[s] }))} />
              <input type="date" className="field num" aria-label="Depuis le" value={z.since ?? ''} max={today} onChange={(e) => patch(i, { since: e.target.value || null })} />
              <button type="button" className="icon-btn size-9 justify-self-end" onClick={() => remove(i)} aria-label={`Retirer ${BODY_ZONE_META[z.zone].short}`}>
                <X className="size-4" />
              </button>
              <input className="field sm:col-span-4" aria-label="Note" placeholder="Note (facultatif)" maxLength={280} value={z.note} onChange={(e) => patch(i, { note: e.target.value })} />
            </li>
          ))}
        </ul>
      )}
      {value.mode === 'tracking' && value.zones.length < ALL_BODY_ZONES.length && (
        <button type="button" className="btn btn-ghost h-9 min-h-9 px-3 text-xs" onClick={() => onChange(addZone(value, today))}>
          <Plus className="size-4" aria-hidden="true" /> Ajouter une zone
        </button>
      )}
    </div>
  )
}
