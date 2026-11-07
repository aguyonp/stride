import { Plus, X } from 'lucide-react'
import { useId } from 'react'
import { useHealthConditions } from '../../api/queriesV41'
import type { BodySide, BodyZone, HealthCondition, HealthStatusInput, HealthZone } from '../../api/types'
import { cn } from '../../lib/cn'
import { ALL_BODY_ZONES, BODY_SIDE_LABEL, BODY_ZONE_META } from '../bodymap/bodyMapMeta'
import { SegmentedControl } from '../ui/SegmentedControl'
import { addZone, changeZone, CONDITION_OTHER, CUSTOM_LABEL_MAX, setMode } from './healthStatus'

const SIDES: BodySide[] = ['left', 'right', 'both']

/** Pathology picker of one tracked zone: the zone's catalog conditions, « Autre (préciser) » revealing a free-text field. */
function ConditionPicker({
  z,
  conditions,
  unavailable,
  onChange,
}: {
  z: HealthZone
  conditions: HealthCondition[] | undefined
  unavailable: boolean
  onChange: (p: Partial<HealthZone>) => void
}) {
  const customId = useId()
  const current = z.condition ?? ''
  // Without the catalog (loading or offline) the current choice stays visible and editable as is.
  const options = conditions ?? (current ? [{ key: current, label: z.conditionLabel || (current === CONDITION_OTHER ? 'Autre (préciser)' : current) }] : [])
  const other = current === CONDITION_OTHER
  return (
    <div className={cn('grid gap-2 sm:col-span-4', other && 'sm:grid-cols-2')}>
      <select
        className="field"
        aria-label={`Pathologie (${BODY_ZONE_META[z.zone].short})`}
        value={current}
        disabled={!conditions && !current}
        onChange={(e) => {
          const condition = e.target.value || null
          onChange({ condition, customLabel: condition === CONDITION_OTHER ? (z.customLabel ?? '') : null, conditionLabel: null })
          if (condition === CONDITION_OTHER) requestAnimationFrame(() => document.getElementById(customId)?.focus())
        }}
      >
        <option value="">{conditions ? 'Pathologie non précisée' : unavailable ? 'Pathologies indisponibles' : 'Chargement des pathologies…'}</option>
        {options.map((c) => (
          <option key={c.key} value={c.key}>
            {c.label}
          </option>
        ))}
      </select>
      {other && (
        <input
          id={customId}
          className="field"
          aria-label="Pathologie (préciser)"
          placeholder="Préciser la pathologie"
          required
          maxLength={CUSTOM_LABEL_MAX}
          value={z.customLabel ?? ''}
          onChange={(e) => onChange({ customLabel: e.target.value })}
        />
      )}
    </div>
  )
}

/** Controlled editor for the health status: good health, or tracked zones with side, start date and pathology. */
export function HealthStatusForm({ value, onChange, today }: { value: HealthStatusInput; onChange: (v: HealthStatusInput) => void; today: string }) {
  const catalog = useHealthConditions(value.mode === 'tracking')
  const conditionsOf = (zone: BodyZone) => catalog.data?.zones.find((c) => c.zone === zone)?.conditions
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
              <select
                className="field"
                aria-label="Zone"
                value={z.zone}
                onChange={(e) => onChange({ ...value, zones: value.zones.map((x, j) => (j === i ? changeZone(x, e.target.value as BodyZone) : x)) })}
              >
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
              <ConditionPicker z={z} conditions={conditionsOf(z.zone)} unavailable={catalog.isError} onChange={(p) => patch(i, p)} />
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
