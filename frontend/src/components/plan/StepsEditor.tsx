import { ArrowDown, ArrowUp, Plus, Repeat, Trash2 } from 'lucide-react'
import type { StepType, TargetKind, WorkoutStep } from '../../api/types'
import { cn } from '../../lib/cn'
import { isNum, paceInput, parsePace } from '../../lib/format'
import { STEP_LABEL, STEP_TONE, TARGET_LABEL } from './steps'

const TYPES: StepType[] = ['warmup', 'run', 'interval', 'recovery', 'rest', 'cooldown']
const KINDS: TargetKind[] = ['none', 'pace', 'hr', 'cadence', 'power']

export function parseDuration(v: string): number | null {
  const t = v.trim().replace(/[′']/g, ':').replace(/[″"]/g, '')
  if (!t) return null
  const m = /^(\d{1,3})(?::(\d{1,2}))?$/.exec(t)
  if (!m) return null
  const s = Number(m[1]) * 60 + Number(m[2] ?? 0)
  return s > 0 ? s : null
}
const durationInput = (s: number | undefined) => (isNum(s) ? `${Math.floor(s / 60)}:${String(Math.round(s % 60)).padStart(2, '0')}` : '')

function Commit({ value, parse, onCommit, label, placeholder, className }: { value: string; parse: (v: string) => number | null; onCommit: (v: number | undefined) => void; label: string; placeholder?: string; className?: string }) {
  return (
    <input
      key={value}
      defaultValue={value}
      aria-label={label}
      placeholder={placeholder}
      inputMode="numeric"
      className={cn('field num h-9 min-h-9 px-2 text-sm', className)}
      onBlur={(e) => {
        const raw = e.target.value
        const v = parse(raw)
        if (raw.trim() === '') onCommit(undefined)
        else if (v !== null) onCommit(v)
        else e.target.value = value
      }}
    />
  )
}

const intParse = (v: string) => {
  const n = Number(v.replace(',', '.'))
  return Number.isFinite(n) && n > 0 ? Math.round(n) : null
}

function StepRow({ step, onChange, onRemove, onMove, index, count, nested }: { step: WorkoutStep; onChange: (s: WorkoutStep) => void; onRemove: () => void; onMove: (d: -1 | 1) => void; index: number; count: number; nested?: boolean }) {
  const kind = step.target?.kind ?? 'none'
  const byDistance = !isNum(step.durationS) && isNum(step.distanceM)
  const tParse = kind === 'pace' ? parsePace : intParse
  const tShow = (v: number | undefined) => (!isNum(v) ? '' : kind === 'pace' ? paceInput(v) : String(v))
  const setTarget = (patch: Partial<NonNullable<WorkoutStep['target']>>) => onChange({ ...step, target: { kind, ...step.target, ...patch } })
  const label = `Étape ${index + 1}`
  return (
    <li className="rounded-xl border border-line bg-track/30 p-2.5">
      <div className="flex flex-wrap items-center gap-2">
        <span className={cn('h-6 w-1 flex-none rounded-full', STEP_TONE[step.type])} aria-hidden="true" />
        <select aria-label={`${label} : type`} className="field h-9 min-h-9 w-auto flex-1 px-2 text-sm sm:flex-none" value={step.type} onChange={(e) => onChange({ ...step, type: e.target.value as StepType })}>
          {TYPES.map((t) => (
            <option key={t} value={t}>
              {STEP_LABEL[t]}
            </option>
          ))}
        </select>
        <select
          aria-label={`${label} : mesure`}
          className="field h-9 min-h-9 w-auto px-2 text-sm"
          value={byDistance ? 'distance' : 'duration'}
          onChange={(e) => onChange(e.target.value === 'distance' ? { ...step, durationS: undefined, distanceM: step.distanceM ?? 1000 } : { ...step, distanceM: undefined, durationS: step.durationS ?? 300 })}
        >
          <option value="duration">Durée</option>
          <option value="distance">Distance</option>
        </select>
        {byDistance ? (
          <Commit label={`${label} : distance (m)`} value={String(step.distanceM ?? '')} parse={intParse} placeholder="m" className="w-20" onCommit={(v) => onChange({ ...step, distanceM: v })} />
        ) : (
          <Commit label={`${label} : durée (m:ss)`} value={durationInput(step.durationS)} parse={parseDuration} placeholder="m:ss" className="w-20" onCommit={(v) => onChange({ ...step, durationS: v })} />
        )}
        <span className="ml-auto flex gap-0.5">
          <button type="button" className="icon-btn size-8" onClick={() => onMove(-1)} disabled={index === 0} aria-label={`Monter l’${label.toLowerCase()}`}>
            <ArrowUp className="size-3.5" />
          </button>
          <button type="button" className="icon-btn size-8" onClick={() => onMove(1)} disabled={index === count - 1} aria-label={`Descendre l’${label.toLowerCase()}`}>
            <ArrowDown className="size-3.5" />
          </button>
          <button type="button" className="icon-btn size-8" onClick={onRemove} aria-label={`Supprimer l’${label.toLowerCase()}`}>
            <Trash2 className="size-3.5" />
          </button>
        </span>
      </div>
      <div className={cn('mt-2 flex flex-wrap items-center gap-2', !nested && 'pl-3')}>
        <select aria-label={`${label} : cible`} className="field h-9 min-h-9 w-auto px-2 text-sm" value={kind} onChange={(e) => onChange({ ...step, target: e.target.value === 'none' ? undefined : { kind: e.target.value as TargetKind } })}>
          {KINDS.map((k) => (
            <option key={k} value={k}>
              {k === 'none' ? 'Sans cible' : TARGET_LABEL[k]}
            </option>
          ))}
        </select>
        {kind !== 'none' && (
          <>
            <Commit label={`${label} : cible min`} value={tShow(step.target?.min)} parse={tParse} placeholder={kind === 'pace' ? '4:10' : 'min'} className="w-20" onCommit={(v) => setTarget({ min: v })} />
            <span className="text-ink-3" aria-hidden="true">
              –
            </span>
            <Commit label={`${label} : cible max`} value={tShow(step.target?.max)} parse={tParse} placeholder={kind === 'pace' ? '4:20' : 'max'} className="w-20" onCommit={(v) => setTarget({ max: v })} />
            <span className="text-xs text-ink-3">{kind === 'pace' ? '/km' : kind === 'hr' ? 'bpm' : kind === 'cadence' ? 'spm' : 'W'}</span>
          </>
        )}
        <input aria-label={`${label} : note`} className="field h-9 min-h-9 min-w-32 flex-1 px-2 text-sm" placeholder="Note (facultatif)" defaultValue={step.note ?? ''} key={step.note ?? ''} onBlur={(e) => onChange({ ...step, note: e.target.value.trim() || undefined })} maxLength={120} />
      </div>
    </li>
  )
}

const move = <T,>(list: T[], i: number, d: -1 | 1) => {
  const next = [...list]
  const j = i + d
  if (j < 0 || j >= next.length) return next
  ;[next[i], next[j]] = [next[j], next[i]]
  return next
}

/** Structured steps editor (warmup / run / interval / recovery / rest / cooldown / repeat with targets). */
export function StepsEditor({ steps, onChange, nested }: { steps: WorkoutStep[]; onChange: (s: WorkoutStep[]) => void; nested?: boolean }) {
  const setAt = (i: number, s: WorkoutStep) => onChange(steps.map((x, j) => (j === i ? s : x)))
  const removeAt = (i: number) => onChange(steps.filter((_, j) => j !== i))
  return (
    <div className="space-y-2">
      {steps.length > 0 && (
        <ol className="space-y-2">
          {steps.map((s, i) =>
            s.type === 'repeat' ? (
              <li key={i} className="rounded-xl border border-violet/30 bg-violet/[0.05] p-2.5">
                <div className="mb-2 flex flex-wrap items-center gap-2">
                  <Repeat className="size-4 text-violet" aria-hidden="true" />
                  <Commit label={`Répétition ${i + 1} : nombre`} value={String(s.repeat ?? 2)} parse={intParse} className="w-16" onCommit={(v) => setAt(i, { ...s, repeat: v ?? 1 })} />
                  <span className="text-sm text-ink-2">fois</span>
                  <span className="ml-auto flex gap-0.5">
                    <button type="button" className="icon-btn size-8" onClick={() => onChange(move(steps, i, -1))} disabled={i === 0} aria-label="Monter la répétition">
                      <ArrowUp className="size-3.5" />
                    </button>
                    <button type="button" className="icon-btn size-8" onClick={() => onChange(move(steps, i, 1))} disabled={i === steps.length - 1} aria-label="Descendre la répétition">
                      <ArrowDown className="size-3.5" />
                    </button>
                    <button type="button" className="icon-btn size-8" onClick={() => removeAt(i)} aria-label="Supprimer la répétition">
                      <Trash2 className="size-3.5" />
                    </button>
                  </span>
                </div>
                <StepsEditor nested steps={s.steps ?? []} onChange={(inner) => setAt(i, { ...s, steps: inner })} />
              </li>
            ) : (
              <StepRow key={i} step={s} index={i} count={steps.length} nested={nested} onChange={(x) => setAt(i, x)} onRemove={() => removeAt(i)} onMove={(d) => onChange(move(steps, i, d))} />
            ),
          )}
        </ol>
      )}
      <div className="flex flex-wrap gap-2">
        <button type="button" className="btn btn-ghost h-9 min-h-9 px-3 text-xs" onClick={() => onChange([...steps, { type: steps.length === 0 && !nested ? 'warmup' : nested && steps.length ? 'recovery' : 'interval', durationS: nested ? 60 : 600 }])}>
          <Plus className="size-3.5" aria-hidden="true" /> Étape
        </button>
        {!nested && (
          <button
            type="button"
            className="btn btn-ghost h-9 min-h-9 px-3 text-xs"
            onClick={() =>
              onChange([
                ...steps,
                {
                  type: 'repeat',
                  repeat: 6,
                  steps: [
                    { type: 'interval', durationS: 60 },
                    { type: 'recovery', durationS: 90 },
                  ],
                },
              ])
            }
          >
            <Repeat className="size-3.5" aria-hidden="true" /> Répétition
          </button>
        )}
      </div>
    </div>
  )
}
