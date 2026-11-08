// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { BackupList, JobStatus } from '../../api/types'
import { emitStreamEvent, resetEventStore } from '../../state/events'
import { clickEl, mountWithProviders, setInputValue, waitFor, type MountedApp } from '../../test/domHarness'
import { BackupsPanel, shortSha } from './BackupsPanel'
import { setLang } from '../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const SHA = '3f9a12c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e8be0'
const list = (): BackupList => ({
  items: [
    { name: 'stride-20261107T030000Z-4.2.0.sqlite.gz', sizeBytes: 18_903_552, sha256: SHA, createdAt: '2026-11-07T03:00:00Z', trigger: 'daily', s3: { status: 'synced', key: 'prod/stride-20261107T030000Z-4.2.0.sqlite.gz', syncedAt: '2026-11-07T03:00:42Z', error: null } },
    { name: 'stride-20261104T091200Z-4.1.0.sqlite.gz', sizeBytes: 18_790_400, sha256: SHA.replace('3f', 'aa'), createdAt: '2026-11-04T09:12:00Z', trigger: 'pre_migration', s3: { status: 'none', key: null, syncedAt: null, error: null } },
  ],
  s3: { endpoint: 'https://s3.fr-par.scw.cloud', region: 'fr-par', bucket: 'stride-backups', prefix: 'prod/', accessKeyId: 'SCWDEMOACCESSKEY0000', secretKeySet: true, pathStyle: false, autoUpload: true, retentionDays: 30 },
  lastSuccess: { local: '2026-11-07T03:00:00Z', s3: '2026-11-07T03:00:42Z' },
  nextScheduledAt: '2026-11-08T03:00:00Z',
})
const job = (over: Partial<JobStatus>): JobStatus => ({ id: 'job_backup_1', userId: 1, kind: 'backup.run', queue: 'low', state: 'running', label: 'Sauvegarde locale', progressPct: 35, processed: null, total: null, attempts: 1, maxAttempts: 4, error: null, result: null, createdAt: '2026-11-07T10:00:00Z', updatedAt: '2026-11-07T10:00:01Z', ...over })
const byText = (label: string, root: ParentNode = document) => Array.from(root.querySelectorAll<HTMLButtonElement>('button')).find((b) => b.textContent?.trim() === label)!

describe('BackupsPanel (§15.4)', () => {
  let mounted: MountedApp | null = null
  let calls: { url: string; method: string; body: Record<string, unknown> | null }[] = []

  beforeEach(() => {
    calls = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        const method = init?.method ?? 'GET'
        const body = init?.body ? JSON.parse(String(init.body)) : null
        calls.push({ url, method, body })
        const ok = (v: unknown, s = 200) => new Response(v === null ? null : JSON.stringify(v), { status: s, headers: { 'Content-Type': 'application/json' } })
        if (url === '/api/admin/backups' && method === 'GET') return ok(list())
        if (url === '/api/admin/backups' && method === 'POST') return ok(job({ label: body?.upload ? 'Sauvegarde locale + envoi S3' : 'Sauvegarde locale', progressPct: 5 }), 202)
        if (url === '/api/admin/backups/s3/test') return ok({ ok: false, latencyMs: 212, error: 'Accès refusé par le bucket (403 SignatureDoesNotMatch).' })
        if (url === '/api/admin/backups/s3' && method === 'PUT') return ok({ ...list().s3, ...body, secretKey: undefined, secretKeySet: true })
        if (method === 'DELETE') return ok(null, 204)
        return ok({})
      }),
    )
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    resetEventStore()
    vi.unstubAllGlobals()
  })

  it('lists backups with date, size, truncated SHA-256 and Local / Synced S3 status, never echoing the secret', async () => {
    mounted = await mountWithProviders(<BackupsPanel />, '/admin?tab=backups')
    await waitFor(() => expect(document.body.textContent).toContain('Synced S3'))
    const text = document.body.textContent ?? ''
    expect(text).toContain('Local')
    expect(text).toContain('Avant migration')
    expect(text).toContain('18 Mo')
    expect(shortSha(SHA)).toBe('3f9a12c4…8be0')
    expect(text).toContain('3f9a12c4…8be0')
    expect(text).not.toContain(SHA)
    const secret = document.querySelector<HTMLInputElement>('input[type="password"]')!
    expect(secret.value).toBe('')
    expect(secret.placeholder).toContain('enregistrée')
    // Synced items cannot be pushed again; local ones can
    expect(document.querySelectorAll('button[aria-label^="Envoyer stride-20261107"]')).toHaveLength(0)
    expect(document.querySelectorAll('button[aria-label^="Envoyer stride-20261104"]').length).toBeGreaterThan(0)
  })

  it('tests the S3 target with the form values and saves without sending an empty secret over the stored one', async () => {
    mounted = await mountWithProviders(<BackupsPanel />, '/admin?tab=backups')
    await waitFor(() => expect(document.querySelector('input[type="password"]')).not.toBeNull())
    await setInputValue(document.querySelector<HTMLInputElement>('input[placeholder="stride-backups"]')!, 'stride-offsite')
    await clickEl(byText('Tester la connexion'))
    await waitFor(() => expect(document.body.textContent).toContain('403 SignatureDoesNotMatch'))
    const test = calls.find((c) => c.url === '/api/admin/backups/s3/test')!
    expect(test.body).toMatchObject({ config: { bucket: 'stride-offsite', secretKey: '' } })

    await clickEl(byText('Enregistrer'))
    await waitFor(() => expect(calls.some((c) => c.method === 'PUT')).toBe(true))
    expect(calls.find((c) => c.method === 'PUT')!.body).toMatchObject({ bucket: 'stride-offsite', secretKey: '' })
  })

  it('starts a local + S3 backup and shows its progress from the event store', async () => {
    mounted = await mountWithProviders(<BackupsPanel />, '/admin?tab=backups')
    await waitFor(() => expect(document.body.textContent).toContain('Synced S3'))
    await clickEl(byText('Backup local + S3'))
    await waitFor(() => expect(calls.some((c) => c.url === '/api/admin/backups' && c.method === 'POST')).toBe(true))
    expect(calls.find((c) => c.method === 'POST')!.body).toEqual({ upload: true })
    await waitFor(() => expect(document.querySelector('[role="progressbar"]')?.getAttribute('aria-valuenow')).toBe('5'))
    emitStreamEvent('job.updated', job({ label: 'Sauvegarde locale + envoi S3', progressPct: 70 }))
    await waitFor(() => expect(document.querySelector('[role="progressbar"]')?.getAttribute('aria-valuenow')).toBe('70'))
    expect(byText('Backup local').disabled).toBe(true)
    emitStreamEvent('job.updated', job({ state: 'completed', progressPct: 100 }))
    await waitFor(() => expect(document.querySelector('[role="progressbar"]')).toBeNull())
  })

  it('deletes after confirmation, optionally with the S3 copy', async () => {
    mounted = await mountWithProviders(<BackupsPanel />, '/admin?tab=backups')
    await waitFor(() => expect(document.body.textContent).toContain('Synced S3'))
    await clickEl(document.querySelector('button[aria-label="Supprimer stride-20261107T030000Z-4.2.0.sqlite.gz"]'))
    await waitFor(() => expect(document.querySelector('dialog[open]')?.textContent).toContain('Supprimer cette sauvegarde ?'))
    expect(calls.some((c) => c.method === 'DELETE')).toBe(false)
    const dlg = document.querySelector('dialog[open]')!
    await clickEl(dlg.querySelector('input[type="checkbox"]'))
    await clickEl(byText('Supprimer', dlg))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE')).toBe(true))
    expect(calls.find((c) => c.method === 'DELETE')!.url).toBe('/api/admin/backups/stride-20261107T030000Z-4.2.0.sqlite.gz?remote=1')
  })
})
