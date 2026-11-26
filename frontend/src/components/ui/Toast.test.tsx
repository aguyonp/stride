// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clickEl, installDomPolyfills, mountWithProviders, waitFor, type MountedApp } from '../../test/domHarness'
import { ToastProvider, useToast } from './Toast'

/**
 * A toast raised from a native <dialog> (the session sheet, the bridge flow) used to be drawn
 * BEHIND it: showModal() puts the dialog in the browser's top layer, above any z-index. The host is
 * now a manual popover, which joins that layer after the dialog.
 */
function Trigger({ label = 'Envoyer' }: { label?: string }) {
  const { toast } = useToast()
  return (
    <button type="button" onClick={() => toast({ title: 'Séance envoyée' })}>
      {label}
    </button>
  )
}

describe('ToastProvider — pile du navigateur', () => {
  let mounted: MountedApp | null = null
  let shown: string[]
  let hidden: number

  beforeEach(() => {
    installDomPolyfills()
    shown = []
    hidden = 0
    // jsdom has no popover: stub just enough to observe the intent.
    const proto = window.HTMLElement.prototype as unknown as Record<string, unknown>
    proto.showPopover = function showPopover(this: HTMLElement) {
      shown.push(this.getAttribute('popover') ?? '')
    }
    proto.hidePopover = function hidePopover() {
      hidden += 1
    }
    proto.matches = function matches(this: HTMLElement, sel: string) {
      return sel === ':popover-open' ? shown.length > 0 : false
    }
  })

  afterEach(async () => {
    await mounted?.unmount()
    mounted = null
    vi.unstubAllGlobals()
  })

  it('hosts the toasts in a manual popover, opened on the first one', async () => {
    mounted = await mountWithProviders(
      <ToastProvider>
        <Trigger />
      </ToastProvider>,
    )
    const host = document.querySelector('[popover="manual"]')
    expect(host).not.toBeNull()

    await clickEl(document.querySelector('button') as HTMLElement)
    await waitFor(() => expect(document.body.textContent).toContain('Séance envoyée'))
    // Opened, so it sits above the dialog that raised it.
    await waitFor(() => expect(shown).toEqual(['manual']))
  })

  it('closes the popover when the last toast is dismissed', async () => {
    mounted = await mountWithProviders(
      <ToastProvider>
        <Trigger label="Vérifier" />
      </ToastProvider>,
    )
    await clickEl(document.querySelector('button') as HTMLElement)
    await waitFor(() => expect(shown).toEqual(['manual']))

    const close = document.querySelector('[aria-label]') as HTMLElement | null
    if (close) await clickEl(close)
    await waitFor(() => expect(hidden).toBeGreaterThan(0))
  })
})
