import type { JobStatus } from '../../api/types'
import { addDays } from '../../lib/format'

export type HistoryRange = '12m' | '24m' | 'all' | 'date'

export const HISTORY_RANGES: { value: HistoryRange; label: string }[] = [
  { value: '12m', label: '12 mois' },
  { value: '24m', label: '24 mois' },
  { value: 'all', label: 'Tout' },
  { value: 'date', label: 'Depuis une date' },
]

/** `from` sent to POST /api/sync/history: null means everything the provider returns; undefined means invalid input. */
export function historyFrom(range: HistoryRange, today: string, custom: string): string | null | undefined {
  if (range === 'all') return null
  if (range === '12m') return addDays(today, -365)
  if (range === '24m') return addDays(today, -730)
  return /^\d{4}-\d{2}-\d{2}$/.test(custom) && custom < today ? custom : undefined
}

export function historyProgress(job: Pick<JobStatus, 'state' | 'processed' | 'total' | 'progressPct'>): { pct: number; label: string } {
  const pct = job.progressPct ?? (job.total ? Math.round(((job.processed ?? 0) / job.total) * 100) : 0)
  if (job.state === 'completed') return { pct: 100, label: `${job.processed ?? 0} séances importées` }
  if (job.state === 'failed') return { pct, label: 'Import interrompu' }
  if (job.state === 'queued') return { pct: 0, label: 'En attente…' }
  return { pct, label: job.total ? `${job.processed ?? 0} / ${job.total} séances` : 'Import en cours…' }
}
