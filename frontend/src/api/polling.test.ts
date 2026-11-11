import { describe, expect, it } from 'vitest'
import { FALLBACK_POLL_MS, fallbackPollInterval, hasActiveJob, isJobActive } from './polling'
import { setLang } from '../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

describe('stream fallback polling', () => {
  it('polls slowly while the stream is down or work is in flight, never when healthy and idle', () => {
    expect(fallbackPollInterval({ streamConnected: false, busy: false })).toBe(FALLBACK_POLL_MS)
    expect(fallbackPollInterval({ streamConnected: true, busy: true })).toBe(FALLBACK_POLL_MS)
    expect(fallbackPollInterval({ streamConnected: false, busy: true })).toBe(FALLBACK_POLL_MS)
    expect(fallbackPollInterval({ streamConnected: true, busy: false })).toBe(false)
    expect(FALLBACK_POLL_MS).toBe(30_000)
  })

  it('treats queued, running and retrying jobs as in flight', () => {
    expect((['queued', 'running', 'retrying'] as const).every(isJobActive)).toBe(true)
    expect(isJobActive('completed')).toBe(false)
    expect(isJobActive('failed')).toBe(false)
    expect(hasActiveJob([])).toBe(false)
    expect(hasActiveJob([{ state: 'completed' }, { state: 'failed' }])).toBe(false)
    expect(hasActiveJob([{ state: 'completed' }, { state: 'retrying' }])).toBe(true)
  })
})
