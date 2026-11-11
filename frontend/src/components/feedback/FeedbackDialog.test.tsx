// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { FeedbackInput } from '../../api/types'
import { clickEl, mountWithProviders, setInputValue, waitFor, type MountedApp } from '../../test/domHarness'
import { FeedbackDialog } from './FeedbackDialog'
import { setLang } from '../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const sendButton = () => Array.from(document.querySelectorAll<HTMLButtonElement>('dialog button')).find((b) => b.textContent?.trim() === 'Envoyer')!

describe('FeedbackDialog', () => {
  let mounted: MountedApp | null = null
  let sent: FeedbackInput[] = []
  let status = 201
  const onClose = vi.fn()

  beforeEach(() => {
    sent = []
    status = 201
    onClose.mockReset()
    try {
      window.localStorage.clear()
    } catch {
      /* no storage */
    }
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        const ok = (v: unknown, s = 200) => new Response(JSON.stringify(v), { status: s, headers: { 'Content-Type': 'application/json' } })
        if (url === '/api/version') return ok({ version: '4.1.0', commit: 'abc1234', builtAt: '', go: 'go1.27', label: '4.1.0 · abc1234' })
        if (url === '/api/feedback' && init?.method === 'POST') {
          if (status !== 201) return ok({ error: 'Merci ! Tu as déjà envoyé plusieurs avis : réessaie dans une heure.', code: 'RATE_LIMITED' }, status)
          sent.push(JSON.parse(String(init.body)))
          return ok({ id: 1, createdAt: '2026-11-07T10:00:00Z' }, 201)
        }
        return ok({})
      }),
    )
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
  })

  it('sends the message with a non-sensitive context and no e-mail without consent', async () => {
    mounted = await mountWithProviders(<FeedbackDialog open onClose={onClose} />, '/plan?invite=SECRET-CODE')
    await waitFor(() => expect(document.querySelector('dialog[open]')).not.toBeNull())
    expect(sendButton().disabled).toBe(true)
    expect(document.querySelector('input[type="email"]')).toBeNull()

    await clickEl(document.querySelector('[role="radio"][aria-checked="false"]'))
    await setInputValue(document.querySelector('textarea')!, '  Pouvoir comparer deux blocs.  ')
    await waitFor(() => expect(document.body.textContent).toContain('4.1.0'))
    expect(sendButton().disabled).toBe(false)
    await clickEl(sendButton())

    await waitFor(() => expect(sent).toHaveLength(1))
    expect(sent[0]).toMatchObject({ category: 'idea', message: 'Pouvoir comparer deux blocs.', contactOk: false, context: { appVersion: '4.1.0', route: '/plan' } })
    expect(sent[0]).not.toHaveProperty('contactEmail')
    expect(JSON.stringify(sent[0])).not.toContain('SECRET-CODE')
    expect(sent[0].context.viewport).toMatch(/^\d+×\d+$/)
    await waitFor(() => expect(document.body.textContent).toContain('Merci pour ton avis !'))
    expect(onClose).toHaveBeenCalled()
  })

  it('requires a valid e-mail once contact is accepted and shows server errors', async () => {
    mounted = await mountWithProviders(<FeedbackDialog open onClose={onClose} />, '/settings')
    await waitFor(() => expect(document.querySelector('dialog[open]')).not.toBeNull())
    await setInputValue(document.querySelector('textarea')!, 'Le planning ne se met pas à jour.')
    await clickEl(document.querySelector('input[type="checkbox"]'))
    const email = document.querySelector<HTMLInputElement>('input[type="email"]')!
    expect(email).not.toBeNull()
    expect(sendButton().disabled).toBe(true)
    await setInputValue(email, 'pas-une-adresse')
    expect(sendButton().disabled).toBe(true)
    await setInputValue(email, 'sam@exemple.fr')
    expect(sendButton().disabled).toBe(false)

    status = 429
    await clickEl(sendButton())
    await waitFor(() => expect(document.querySelector('[role="alert"]')?.textContent).toContain('réessaie dans une heure'))
    expect(onClose).not.toHaveBeenCalled()

    status = 201
    await clickEl(sendButton())
    await waitFor(() => expect(sent).toHaveLength(1))
    expect(sent[0]).toMatchObject({ category: 'bug', contactOk: true, contactEmail: 'sam@exemple.fr' })
  })
})
