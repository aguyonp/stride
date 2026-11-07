import type { JobState, JobStatus } from './types'

// SSE is the primary channel; these intervals are safety nets only.
export const FALLBACK_POLL_MS = 30_000
export const ADMIN_METER_POLL_MS = 15_000

export const isJobActive = (state: JobState) => state === 'queued' || state === 'running' || state === 'retrying'

export function hasActiveJob(jobs: Iterable<Pick<JobStatus, 'state'>>): boolean {
  for (const j of jobs) if (isJobActive(j.state)) return true
  return false
}

/** Slow polling while the event stream is down or work is in flight; none once the stream is healthy and idle. */
export function fallbackPollInterval(p: { streamConnected: boolean; busy: boolean }): number | false {
  return !p.streamConnected || p.busy ? FALLBACK_POLL_MS : false
}
