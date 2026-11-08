import { History, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useProfile } from '../../api/queries'
import { useSyncHistory } from '../../api/queriesV41'
import { cn } from '../../lib/cn'
import { todayISO } from '../../lib/format'
import { useEventStore } from '../../state/events'
import { InlineError } from '../ui/Misc'
import { SegmentedControl } from '../ui/SegmentedControl'
import { historyRanges, historyFrom, historyProgress, type HistoryRange } from './historyRange'
import { t } from '../../i18n'

/** Backfills older activities (summaries only) as a low-priority job with live progress. */
export function HistoryImport() {
  const profile = useProfile()
  const today = todayISO(profile.data?.timezone)
  const start = useSyncHistory()
  const [range, setRange] = useState<HistoryRange>('12m')
  const [custom, setCustom] = useState('')
  const jobId = start.data?.id ?? null
  const job = useEventStore((s) => (jobId ? (s.jobs[jobId] ?? null) : null)) ?? start.data ?? null
  const running = job !== null && (job.state === 'queued' || job.state === 'running' || job.state === 'retrying')
  const from = historyFrom(range, today, custom)
  const progress = job ? historyProgress(job) : null

  return (
    <div className="glass-inset space-y-3 p-4">
      <div className="flex items-center gap-2">
        <History className="size-4 text-accent" aria-hidden="true" />
        <h3 className="text-sm font-semibold text-ink">{t('settings.history.title')}</h3>
      </div>
      <SegmentedControl<HistoryRange> ariaLabel={t('settings.history.range')} size="sm" value={range} onChange={setRange} options={historyRanges()} />
      {range === 'date' && (
        <input type="date" className="field num max-w-48" aria-label={t('settings.history.since')} value={custom} max={today} onChange={(e) => setCustom(e.target.value)} />
      )}
      <button type="button" className="btn btn-primary" disabled={running || start.isPending || from === undefined} onClick={() => from !== undefined && start.mutate(from)}>
        {(running || start.isPending) && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
        {t('settings.history.submit')}
      </button>
      {start.isError && <InlineError>{start.error.message}</InlineError>}
      {progress && job && (
        <div aria-live="polite">
          <div className="h-1.5 overflow-hidden rounded-full bg-track" role="progressbar" aria-label={t('settings.history.progress')} aria-valuemin={0} aria-valuemax={100} aria-valuenow={progress.pct}>
            <div className={cn('h-full rounded-full transition-[width] duration-300', job.state === 'failed' ? 'bg-danger' : 'bg-accent')} style={{ width: `${progress.pct}%` }} />
          </div>
          <p className="num mt-1 text-xs text-ink-3">{job.state === 'failed' && job.error ? job.error : progress.label}</p>
        </div>
      )}
    </div>
  )
}
