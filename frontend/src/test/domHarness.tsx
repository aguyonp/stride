import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, type ReactElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { ToastProvider } from '../components/ui/Toast'

export function installDomPolyfills() {
  ;(globalThis as Record<string, unknown>).IS_REACT_ACT_ENVIRONMENT = true

  if (typeof HTMLDialogElement !== 'undefined') {
    if (typeof HTMLDialogElement.prototype.showModal !== 'function') {
      HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
        this.setAttribute('open', '')
      }
    }
    if (typeof HTMLDialogElement.prototype.close !== 'function') {
      HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
        this.removeAttribute('open')
      }
    }
  }

  if (typeof Element !== 'undefined' && typeof Element.prototype.scrollIntoView !== 'function') {
    Element.prototype.scrollIntoView = () => undefined
  }

  if (typeof globalThis.CSS === 'undefined' || typeof globalThis.CSS.escape !== 'function') {
    ;(globalThis as unknown as { CSS: { escape: (s: string) => string } }).CSS = {
      escape: (s: string) => s.replace(/[^a-zA-Z0-9_-]/g, '\\$&'),
    }
  }

  if (typeof window !== 'undefined' && typeof window.matchMedia !== 'function') {
    window.matchMedia = (query: string) =>
      ({
        matches: false,
        media: query,
        onchange: null,
        addListener: () => undefined,
        removeListener: () => undefined,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
        dispatchEvent: () => false,
      }) as MediaQueryList
  }

  if (typeof globalThis.ResizeObserver === 'undefined') {
    class FakeResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
    ;(globalThis as unknown as { ResizeObserver: typeof FakeResizeObserver }).ResizeObserver = FakeResizeObserver
  }

  if (typeof globalThis.IntersectionObserver === 'undefined') {
    class FakeIntersectionObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
      takeRecords() {
        return []
      }
    }
    ;(globalThis as unknown as { IntersectionObserver: typeof FakeIntersectionObserver }).IntersectionObserver =
      FakeIntersectionObserver
  }
}

export interface MountedApp {
  container: HTMLDivElement
  queryClient: QueryClient
  unmount: () => Promise<void>
}

export async function mountWithProviders(
  ui: ReactElement,
  initialUrl = '/',
): Promise<MountedApp> {
  installDomPolyfills()
  window.history.replaceState(null, '', initialUrl)

  const container = document.createElement('div')
  document.body.appendChild(container)

  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 60_000, staleTime: 0 },
      mutations: { retry: false },
    },
  })

  let root: Root | null = null
  await act(async () => {
    root = createRoot(container)
    root.render(
      <QueryClientProvider client={queryClient}>
        <ToastProvider>{ui}</ToastProvider>
      </QueryClientProvider>,
    )
  })

  return {
    container,
    queryClient,
    unmount: async () => {
      await act(async () => {
        root?.unmount()
      })
      queryClient.clear()
      container.remove()
    },
  }
}

export async function waitFor(assertion: () => void, timeoutMs = 2500): Promise<void> {
  const deadline = Date.now() + timeoutMs
  let lastErr: unknown
  while (Date.now() < deadline) {
    try {
      await act(async () => {
        await Promise.resolve()
      })
      assertion()
      return
    } catch (err) {
      lastErr = err
      await act(async () => {
        await new Promise((r) => setTimeout(r, 20))
      })
    }
  }
  throw lastErr instanceof Error ? lastErr : new Error('Condition non atteinte dans waitFor')
}

export async function clickEl(el: Element | null) {
  if (!el) throw new Error('Élément introuvable pour clickEl')
  await act(async () => {
    el.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
  })
}

export async function setInputValue(input: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement, value: string) {
  await act(async () => {
    const proto =
      input instanceof HTMLSelectElement
        ? HTMLSelectElement.prototype
        : input instanceof HTMLTextAreaElement
          ? HTMLTextAreaElement.prototype
          : HTMLInputElement.prototype
    const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set
    if (setter) setter.call(input, value)
    else input.value = value
    input.dispatchEvent(new Event('input', { bubbles: true }))
    input.dispatchEvent(new Event('change', { bubbles: true }))
  })
}

export async function keyDownEl(target: EventTarget, key: string, opts: KeyboardEventInit = {}) {
  await act(async () => {
    target.dispatchEvent(
      new KeyboardEvent('keydown', {
        key,
        bubbles: true,
        cancelable: true,
        ...opts,
      }),
    )
  })
}
