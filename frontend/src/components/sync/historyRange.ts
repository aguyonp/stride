import type { JobStatus } from '../../api/types'
import { addDays } from '../../lib/format'
import { t } from '../../i18n'

export type HistoryRange = '12m' | '24m' | 'all' | 'date'

export const historyRanges = (): { value: HistoryRange; label: string }[] =>
  (['12m', '24m', 'all', 'date'] as const).map((value) => ({ value, label: t(`settings.history.ranges.${value}`) }))

/** `from` sent to POST /api/sync/history: null means everything the provider returns; undefined means invalid input. */
export function historyFrom(range: HistoryRange, today: string, custom: string): string | null | undefined {
  if (range === 'all') return null
  if (range === '12m') return addDays(today, -365)
  if (range === '24m') return addDays(today, -730)
  return /^\d{4}-\d{2}-\d{2}$/.test(custom) && custom < today ? custom : undefined
}

export function historyProgress(job: Pick<JobStatus, 'state' | 'processed' | 'total' | 'progressPct'>): { pct: number; label: string } {
  const pct = job.progressPct ?? (job.total ? Math.round(((job.processed ?? 0) / job.total) * 100) : 0)
  if (job.state === 'completed') return { pct: 100, label: t('settings.history.imported', { count: job.processed ?? 0 }) }
  if (job.state === 'failed') return { pct, label: t('settings.history.interrupted') }
  if (job.state === 'queued') return { pct: 0, label: t('settings.history.waiting') }
  return { pct, label: job.total ? t('settings.history.counted', { processed: job.processed ?? 0, total: job.total }) : t('settings.history.running') }
}
