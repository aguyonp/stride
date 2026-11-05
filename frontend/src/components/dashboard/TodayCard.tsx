import { BatteryMedium, BedDouble, Footprints, Gauge, HeartPulse, Loader2, Pencil, Sun } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useDayView, useSaveCheckin } from '../../api/queries'
import type { DayView } from '../../api/types'
import { cn } from '../../lib/cn'
import { capitalize, fmtDateLong, fmtNumber, isNum } from '../../lib/format'
import { hrvStatus } from '../../lib/labels'
import { DoneActivityRow } from '../plan/DoneActivityRow'
import { PlannedSessionCard } from '../plan/PlannedSessionCard'
import { LevelIcon } from '../ui/Badge'
import { Card } from '../ui/Card'
import { Field } from '../ui/Field'
import { FreshnessPill } from '../ui/FreshnessPill'
import { InlineError } from '../ui/Misc'
import { Skeleton } from '../ui/Skeleton'
import { useToast } from '../ui/Toast'
import { NA } from '../ui/Value'
import { StairsIcon } from '../ui/icons'

const STATUSES = ['Faible', 'Modérée', 'Élevée', 'Optimale']

function CheckinForm({ day, onDone }: { day: DayView; onDone: () => void }) {
  const save = useSaveCheckin(day.date)
  const { toast } = useToast()
  const c = day.checkin
  const [score, setScore] = useState<string>(c?.trainingReadiness != null ? String(c.trainingReadiness) : '')
  const [status, setStatus] = useState(c?.readinessStatus ?? '')
  const [note, setNote] = useState(c?.note ?? '')
  const n = score === '' ? null : Number(score)
  const invalid = n !== null && (!Number.isFinite(n) || n < 0 || n > 100)
  return (
    <form
      className="mt-2 space-y-3"
      onSubmit={(e) => {
        e.preventDefault()
        if (invalid) return
        save.mutate({ trainingReadiness: n, readinessStatus: status || null, note }, { onSuccess: () => (toast({ title: 'Check-in enregistré' }), onDone()) })
      }}
    >
      <div className="grid grid-cols-[minmax(0,7rem)_minmax(0,1fr)] gap-2">
        <Field label="Score /100" error={invalid ? '0 à 100' : undefined}>
          {(p) => <input {...p} className="field num" inputMode="numeric" type="number" min={0} max={100} value={score} onChange={(e) => setScore(e.target.value)} placeholder="—" />}
        </Field>
        <Field label="Statut">
          {(p) => (
            <select {...p} className="field" value={status} onChange={(e) => setStatus(e.target.value)}>
              <option value="">—</option>
              {STATUSES.map((s) => (
                <option key={s}>{s}</option>
              ))}
            </select>
          )}
        </Field>
      </div>
      <Field label="Note du matin">
        {(p) => <input {...p} className="field" value={note} onChange={(e) => setNote(e.target.value)} maxLength={500} placeholder="Jambes lourdes, bien dormi…" />}
      </Field>
      {save.isError && <InlineError>{save.error.message}</InlineError>}
      <div className="flex justify-end gap-2">
        <button type="button" className="btn btn-ghost" onClick={onDone}>
          Annuler
        </button>
        <button type="submit" className="btn btn-primary" disabled={save.isPending || invalid}>
          {save.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
          Enregistrer
        </button>
      </div>
    </form>
  )
}

function Mini({ icon, label, value, tone }: { icon: ReactNode; label: string; value: ReactNode; tone?: 'warn' | 'danger' | null }) {
  return (
    <div className="glass-inset min-w-0 rounded-xl px-2.5 py-2">
      <p className="flex items-center gap-1 text-[11px] text-ink-3">
        <span aria-hidden="true" className="[&>svg]:size-3.5">
          {icon}
        </span>
        <span className="truncate">{label}</span>
      </p>
      <p className={cn('num truncate text-sm font-semibold', tone === 'danger' ? 'text-danger' : tone === 'warn' ? 'text-warn' : 'text-ink')}>{value}</p>
    </div>
  )
}

/** Today: planned vs done, readiness check-in, last night's recovery, daily load. */
export function TodayCard({ date, className }: { date: string; className?: string }) {
  const q = useDayView(date)
  const [editing, setEditing] = useState(false)
  const d = q.data && q.data.date === date ? q.data : null
  if (!d) {
    return (
      <Card title="Aujourd’hui" icon={<Sun className="size-4" />} className={className}>
        {q.isError ? <InlineError>{q.error.message}</InlineError> : <Skeleton className="h-48" />}
      </Card>
    )
  }
  const rec = d.recovery
  const r = rec?.readiness
  const hrv = rec?.hrv
  const st = hrv ? hrvStatus(hrv.status, hrv.statusLabel) : null
  const bb = rec?.bodyBattery?.atWake
  const day = d.day
  const stepPct = isNum(day.steps) && isNum(day.stepGoal) && day.stepGoal > 0 ? Math.min(100, (day.steps / day.stepGoal) * 100) : null
  const serious = d.alerts.filter((a) => a.level === 'danger' || a.level === 'warn')
  const doneIds = new Set(d.planned.map((p) => p.match?.activityId).filter(Boolean))
  return (
    <Card title="Aujourd’hui" icon={<Sun className="size-4" />} subtitle={capitalize(fmtDateLong(date))} action={<FreshnessPill freshness={d.freshness} />} className={className}>
      <div className="space-y-3">
        <div>
          <p className="label-caps mb-1.5">Prévu vs réalisé</p>
          {d.planned.length === 0 && d.activities.length === 0 ? (
            <p className="text-sm text-ink-3">Rien de prévu ni réalisé pour l’instant.</p>
          ) : (
            <ul className="space-y-2">
              {d.planned.map((p) => (
                <li key={p.id}>
                  <PlannedSessionCard session={p} compact doneActivity={d.activities.find((a) => a.id === p.match?.activityId) ?? null} />
                </li>
              ))}
              {d.activities
                .filter((a) => !doneIds.has(a.id))
                .map((a) => (
                  <li key={a.id}>
                    <DoneActivityRow activity={a} extra={a.planStatus === 'extra'} />
                  </li>
                ))}
            </ul>
          )}
        </div>

        <div className="glass-inset px-3 py-2.5">
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <p className="flex items-center gap-1 text-[11px] text-ink-3">
                <Gauge className="size-3.5" aria-hidden="true" /> Readiness du jour
              </p>
              {r && isNum(r.score) ? (
                <p className="num text-lg font-semibold">
                  {r.score}
                  <span className="text-xs text-ink-3">/100</span>
                  {r.status && <span className="ml-2 text-sm font-medium text-ink-2">{r.status}</span>}
                </p>
              ) : (
                <p className="text-sm">
                  <NA reason={r?.reason ?? 'Non exposé par l’API Garmin — saisie manuelle'} />
                </p>
              )}
              {d.checkin?.note && !editing && <p className="mt-0.5 truncate text-xs text-ink-3">« {d.checkin.note} »</p>}
            </div>
            {!editing && (
              <button type="button" className="btn btn-ghost h-9 min-h-9 flex-none px-3 text-xs" onClick={() => setEditing(true)}>
                <Pencil className="size-3.5" aria-hidden="true" />
                {d.checkin ? 'Modifier' : 'Check-in'}
              </button>
            )}
          </div>
          {editing && <CheckinForm day={d} onDone={() => setEditing(false)} />}
        </div>

        <div className="grid grid-cols-2 gap-2 xs:grid-cols-4 lg:grid-cols-2 2xl:grid-cols-4">
          <Mini icon={<HeartPulse />} label={`VFC${st ? ` · ${st.label}` : ''}`} value={isNum(hrv?.lastNightAvgMs) ? `${hrv.lastNightAvgMs} ms` : <NA />} tone={st?.level === 'danger' ? 'danger' : st?.level === 'warn' ? 'warn' : null} />
          <Mini icon={<BedDouble />} label="Sommeil" value={isNum(rec?.sleep?.score) ? `${rec.sleep.score}/100` : <NA />} tone={isNum(rec?.sleep?.score) && rec.sleep.score < 60 ? 'warn' : null} />
          <Mini icon={<HeartPulse />} label="FC repos" value={isNum(rec?.restingHR?.bpm) ? `${rec.restingHR.bpm} bpm` : <NA />} tone={isNum(rec?.restingHR?.delta) && rec.restingHR.delta >= 5 ? 'warn' : null} />
          <Mini icon={<BatteryMedium />} label="Body Battery" value={isNum(bb) ? `${bb}/100` : <NA />} tone={isNum(bb) ? (bb < 30 ? 'danger' : bb < 50 ? 'warn' : null) : null} />
        </div>

        <div className="space-y-1.5 text-xs">
          <div className="flex items-center justify-between gap-2 text-ink-2">
            <span className="flex items-center gap-1">
              <Footprints className="size-3.5" aria-hidden="true" /> Pas
            </span>
            <span className="num">
              <strong className="text-ink">{isNum(day.steps) ? fmtNumber(day.steps) : 'N/D'}</strong> / {isNum(day.stepGoal) ? fmtNumber(day.stepGoal) : 'N/D'}
            </span>
          </div>
          {stepPct !== null && (
            <div className="h-1.5 rounded-full bg-track" aria-hidden="true">
              <div className="h-full rounded-full bg-accent" style={{ width: `${stepPct}%` }} />
            </div>
          )}
          <p className="num flex flex-wrap gap-x-3 gap-y-1 pt-0.5 text-ink-3">
            <span className="inline-flex items-center gap-1">
              <StairsIcon className="size-3.5" /> {isNum(day.floorsUp) ? day.floorsUp : 'N/D'} ↑ / {isNum(day.floorsDown) ? day.floorsDown : 'N/D'} ↓ étages
            </span>
            <span>Intensité {isNum(day.intensityModerateMin) ? day.intensityModerateMin : 'N/D'} + {isNum(day.intensityVigorousMin) ? day.intensityVigorousMin : 'N/D'} min</span>
            <span>Stress {isNum(day.stressAvg) ? `${day.stressAvg}/100` : 'N/D'}</span>
            <span>BB +{isNum(day.bodyBatteryCharged) ? day.bodyBatteryCharged : 'N/D'} / −{isNum(day.bodyBatteryDrained) ? day.bodyBatteryDrained : 'N/D'}</span>
          </p>
        </div>

        {serious.length > 0 && (
          <ul className="space-y-1.5 border-t border-line pt-3">
            {serious.slice(0, 3).map((a, i) => (
              <li key={`${a.code}-${i}`} className="flex gap-2 text-xs">
                <LevelIcon level={a.level} className="mt-0.5 size-3.5 flex-none" />
                <span className="text-ink-2">
                  <strong className="font-semibold text-ink">{a.title}</strong> — {a.message}
                </span>
              </li>
            ))}
          </ul>
        )}
        {d.warnings.length > 0 && <p className="text-[11px] text-ink-3">{d.warnings.join(' · ')}</p>}
      </div>
    </Card>
  )
}
