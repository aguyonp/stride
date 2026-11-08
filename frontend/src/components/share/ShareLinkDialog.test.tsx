// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ShareLink, ShareLinkInput } from '../../api/types'
import { clickEl, mountWithProviders, waitFor, type MountedApp } from '../../test/domHarness'
import { ShareLinkDialog } from './ShareLinkDialog'
import { ALL_SHARE_SECTIONS, NO_PERMISSIONS } from './shareScope'
import { setLang } from '../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const button = (label: string) => Array.from(document.querySelectorAll<HTMLButtonElement>('dialog button')).find((b) => b.textContent?.trim() === label)!
const checkbox = (label: string) => Array.from(document.querySelectorAll('dialog label')).find((l) => l.textContent?.startsWith(label))?.querySelector('input') ?? null

const existing: ShareLink = {
  id: 7,
  label: 'Coach club',
  tokenPrefix: 'sh_abc12',
  url: '/share/sh_abc12345',
  expiresAt: null,
  revoked: false,
  revokedAt: null,
  accessCount: 0,
  lastAccessedAt: null,
  createdAt: '2026-11-01T10:00:00Z',
  periodWeeks: 12,
  sections: ALL_SHARE_SECTIONS,
  anonymize: false,
  permissions: { comments: true, planning: false, activityDetails: false },
}

describe('ShareLinkDialog permissions (§15.1)', () => {
  let mounted: MountedApp | null = null
  let calls: { url: string; method: string; body: Partial<ShareLinkInput> }[] = []

  beforeEach(() => {
    calls = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        const body = init?.body ? JSON.parse(String(init.body)) : {}
        calls.push({ url, method: init?.method ?? 'GET', body })
        return new Response(JSON.stringify({ ...existing, ...body, id: 9 }), { status: init?.method === 'POST' ? 201 : 200, headers: { 'Content-Type': 'application/json' } })
      }),
    )
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
  })

  it('offers the three permissions, off by default, each with a one-line explanation', async () => {
    const onCreated = vi.fn()
    mounted = await mountWithProviders(<ShareLinkDialog link={null} onClose={() => undefined} onCreated={onCreated} />)
    await waitFor(() => expect(document.querySelector('dialog[open]')).not.toBeNull())
    const text = document.querySelector('dialog')?.textContent ?? ''
    expect(text).toContain('Le destinataire peut aussi')
    expect(text).toContain('consigne générale et une remarque sur chaque séance')
    expect(text).toContain('prescrire des séances ou déclarer des contraintes')
    expect(text).toContain('tours, biomécanique, météo et chaussure')
    for (const label of ['Consignes et remarques', 'Planning', 'Détail des séances']) expect((checkbox(label) as HTMLInputElement).checked).toBe(false)

    await clickEl(checkbox('Consignes et remarques'))
    await clickEl(checkbox('Détail des séances'))
    await clickEl(button('Créer et copier'))
    await waitFor(() => expect(onCreated).toHaveBeenCalled())
    const post = calls.find((c) => c.method === 'POST' && c.url === '/api/share')!
    expect(post.body.permissions).toEqual({ comments: true, planning: false, activityDetails: true })
  })

  it('edits an existing link and sends the updated permissions in the PATCH', async () => {
    const onClose = vi.fn()
    mounted = await mountWithProviders(<ShareLinkDialog link={existing} onClose={onClose} onCreated={() => undefined} />)
    await waitFor(() => expect(document.querySelector('dialog[open]')).not.toBeNull())
    expect((checkbox('Consignes et remarques') as HTMLInputElement).checked).toBe(true)
    await clickEl(checkbox('Consignes et remarques'))
    await clickEl(checkbox('Planning'))
    await clickEl(button('Enregistrer'))
    await waitFor(() => expect(onClose).toHaveBeenCalled())
    const patch = calls.find((c) => c.method === 'PATCH')!
    expect(patch.url).toBe('/api/share/7')
    expect(patch.body.permissions).toEqual({ ...NO_PERMISSIONS, planning: true })
  })
})
