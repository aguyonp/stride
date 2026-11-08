import { Heart, Loader2, Pencil, RotateCcw, Trophy } from 'lucide-react'
import { useState } from 'react'
import { useRecordMutations, useRecords } from '../../api/queriesV31'
import type { PersonalRecord } from '../../api/types'
import { cn } from '../../lib/cn'
import { durationInput, fmtDateShort, fmtDuration, fmtKm, fmtPace, parseDuration } from '../../lib/format'
import { RECORD_SOURCE } from '../../lib/labels'
import { Card } from '../ui/Card'
import { Field } from '../ui/Field'
import { InlineError } from '../ui/Misc'
import { Skeleton } from '../ui/Skeleton'
import { useToast } from '../ui/Toast'
import { t, tDyn } from '../../i18n'

const SOURCE_CLS: Record<PersonalRecord['source'], string> = { computed: 'bg-track text-ink-3', strava: 'bg-[#fc4c02]/12 text-[#c2410c] dark:text-[#fb923c]', manual: 'bg-violet/12 text-violet' }

/** Strava kudos count (Garmin likes are not exposed). */
export function KudosBadge({ count, className }: { count: number | null | undefined; className?: string }) {
  if (count === null || count === undefined) return null
  return (
    <span className={cn('inline-flex items-center gap-0.5 text-xs font-semibold', count > 0 ? 'text-[#c2410c] dark:text-[#fb923c]' : 'text-ink-3', className)} title={t('dashboard.records.kudosTitle')} aria-label={t('dashboard.records.kudos', { count })}>
      <Heart className={cn('size-3.5', count > 0 && 'fill-current')} aria-hidden="true" />
      <span className="num">{count}</span>
    </span>
  )
}

/** Records are labelled by stable key in the active language (the API label is French). */
export const recordLabel = (r: Pick<PersonalRecord, 'key' | 'label'>) => tDyn(`dashboard.records.keys.${r.key}`, undefined, r.label)

const mainValue = (r: PersonalRecord) => (r.key === 'longest' ? (r.distanceM > 0 ? fmtKm(r.distanceM, 1) : '—') : r.timeS ? fmtDuration(r.timeS) : '—')
const subValue = (r: PersonalRecord) => (r.key === 'longest' ? (r.timeS ? fmtDuration(r.timeS) : null) : r.paceSecPerKm ? fmtPace(r.paceSecPerKm) : null)

export function recordsSummary(records: PersonalRecord[] | undefined): string | undefined {
  if (!records) return undefined
  const pick = records.filter((r) => (r.key === '5k' || r.key === '10k' || r.key === 'half') && r.timeS)
  return pick.length ? pick.map((r) => `${r.key === 'half' ? t('dashboard.records.halfShort') : recordLabel(r)} ${fmtDuration(r.timeS)}`).join(' · ') : t('dashboard.records.none')
}

function Editor({ r, onDone }: { r: PersonalRecord; onDone: () => void }) {
  const { save, reset } = useRecordMutations()
  const { toast } = useToast()
  const [time, setTime] = useState(durationInput(r.timeS))
  const [km, setKm] = useState(r.distanceM > 0 ? String(Math.round(r.distanceM / 100) / 10).replace('.', ',') : '')
  const [date, setDate] = useState(r.date ?? '')
  const longest = r.key === 'longest'
  const timeS = parseDuration(time)
  const distM = Number(km.replace(',', '.')) * 1000
  const invalid = longest ? !(distM > 0) || (time.trim() !== '' && timeS === null) : timeS === null
  const submit = () =>
    save.mutate(
      { key: r.key, timeS: timeS ?? undefined, distanceM: longest ? Math.round(distM) : undefined, date: date || undefined },
      { onSuccess: () => (toast({ title: t('dashboard.records.saved'), description: recordLabel(r) }), onDone()) },
    )
  return (
    <div className="mt-3 rounded-2xl bg-track/50 p-3">
      <p className="mb-2 text-sm font-semibold text-ink">{recordLabel(r)}</p>
      <div className="grid gap-3 sm:grid-cols-3">
        {longest && (
          <Field label={t('dashboard.records.distanceKm')}>{(p) => <input {...p} className="field num" inputMode="decimal" value={km} onChange={(e) => setKm(e.target.value)} placeholder="21,1" />}</Field>
        )}
        <Field label={longest ? t('dashboard.records.timeOptional') : t('dashboard.records.time')} hint={t('dashboard.records.timeHint')}>
          {(p) => <input {...p} className="field num" inputMode="numeric" value={time} onChange={(e) => setTime(e.target.value)} placeholder={r.key === 'marathon' ? '3:45:00' : '22:41'} />}
        </Field>
        <Field label={t('dashboard.records.date')}>{(p) => <input {...p} type="date" className="field" value={date} onChange={(e) => setDate(e.target.value)} />}</Field>
      </div>
      {(save.error || reset.error) && <InlineError className="mt-2">{(save.error ?? reset.error)!.message}</InlineError>}
      <div className="mt-3 flex flex-wrap justify-between gap-2">
        {r.source === 'manual' ? (
          <button type="button" className="btn btn-ghost h-9 min-h-9 px-3 text-xs" onClick={() => reset.mutate(r.key, { onSuccess: () => (toast({ title: t('dashboard.records.manualRemoved'), description: t('dashboard.records.backToComputed') }), onDone()) })} disabled={reset.isPending}>
            <RotateCcw className="size-3.5" aria-hidden="true" /> {t('dashboard.records.useComputed')}
          </button>
        ) : (
          <span />
        )}
        <span className="flex gap-2">
          <button type="button" className="btn btn-ghost h-9 min-h-9 px-3 text-xs" onClick={onDone}>
            {t('common.actions.cancel')}
          </button>
          <button type="button" className="btn btn-primary h-9 min-h-9 px-3 text-xs" onClick={submit} disabled={invalid || save.isPending}>
            {save.isPending && <Loader2 className="size-3.5 animate-spin" aria-hidden="true" />}
            {t('common.actions.save')}
          </button>
        </span>
      </div>
    </div>
  )
}

/** Personal records (1 km → marathon, longest run) with their source; manual values can be set or reset. */
export function RecordsCard({ className }: { className?: string }) {
  const q = useRecords()
  const [editing, setEditing] = useState<string | null>(null)
  const current = q.data?.find((r) => r.key === editing) ?? null
  return (
    <Card title={t('dashboard.records.title')} icon={<Trophy className="size-4" />} subtitle={t('dashboard.records.subtitle')} className={className}>
      {q.isError ? (
        <InlineError>{q.error.message}</InlineError>
      ) : !q.data ? (
        <div className="grid grid-cols-2 gap-2">
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className="h-16" />
          ))}
        </div>
      ) : (
        <>
          <ul className="grid grid-cols-2 gap-x-3 gap-y-1">
            {q.data.map((r) => {
              const sub = subValue(r)
              return (
                <li key={r.key}>
                  <button
                    type="button"
                    onClick={() => setEditing(editing === r.key ? null : r.key)}
                    aria-expanded={editing === r.key}
                    className={cn('group w-full rounded-xl px-2 py-1.5 text-left transition-colors hover:bg-track/60', editing === r.key && 'bg-track/60')}
                    aria-label={t('dashboard.records.aria', { label: recordLabel(r), value: mainValue(r), date: r.date ? t('dashboard.records.ariaDate', { date: fmtDateShort(r.date) }) : '', source: RECORD_SOURCE[r.source] })}
                  >
                    <span className="flex items-center justify-between gap-1">
                      <span className="truncate text-[11px] font-semibold tracking-wide text-ink-3 uppercase">{recordLabel(r)}</span>
                      <Pencil className="size-3 flex-none text-ink-3 opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100" aria-hidden="true" />
                    </span>
                    <span className={cn('num block text-lg leading-tight font-semibold tracking-tight', mainValue(r) === '—' ? 'text-ink-3' : 'text-ink')}>{mainValue(r)}</span>
                    <span className="flex flex-wrap items-center gap-x-1.5 text-[11px] text-ink-3">
                      {sub && <span className="num">{sub}</span>}
                      {r.date && <span className="num">{fmtDateShort(r.date)}</span>}
                      {(r.timeS || r.distanceM > 0) && <span className={cn('rounded-full px-1.5 leading-4 font-semibold', SOURCE_CLS[r.source])}>{RECORD_SOURCE[r.source]}</span>}
                    </span>
                  </button>
                </li>
              )
            })}
          </ul>
          {current && <Editor key={current.key} r={current} onDone={() => setEditing(null)} />}
        </>
      )}
    </Card>
  )
}
