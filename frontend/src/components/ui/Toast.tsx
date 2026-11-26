import { CircleCheck, Info, OctagonAlert, X } from 'lucide-react'
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { cn } from '../../lib/cn'
import { t } from '../../i18n'

type Tone = 'success' | 'error' | 'info'
interface ToastItem {
  id: number
  title: string
  description?: string
  tone: Tone
}

interface ToastApi {
  toast: (t: { title: string; description?: string; tone?: Tone }) => void
}

const ToastContext = createContext<ToastApi | null>(null)

export function useToast(): ToastApi {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast outside ToastProvider')
  return ctx
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])
  const seq = useRef(0)
  const hostRef = useRef<HTMLDivElement>(null)

  const dismiss = useCallback((id: number) => setItems((xs) => xs.filter((x) => x.id !== id)), [])

  const toast = useCallback<ToastApi['toast']>(
    ({ title, description, tone = 'success' }) => {
      const id = ++seq.current
      setItems((xs) => [...xs.slice(-2), { id, title, description, tone }])
      window.setTimeout(() => dismiss(id), tone === 'error' ? 6000 : 3500)
    },
    [dismiss],
  )

  // A native <dialog> opened with showModal() sits in the browser's TOP LAYER, above any z-index:
  // a toast for an action started from the session sheet was drawn behind it (field report). A
  // popover joins the same layer, opened *after* the sheet, so the confirmation lands on top.
  useEffect(() => {
    const el = hostRef.current as (HTMLDivElement & { showPopover?: () => void; hidePopover?: () => void }) | null
    if (!el || typeof el.showPopover !== 'function') return
    try {
      if (items.length > 0) {
        // React 19 does not always forward the attribute: set it before the first open.
        if (!el.hasAttribute('popover')) el.setAttribute('popover', 'manual')
        if (!el.matches(':popover-open')) el.showPopover()
      } else if (el.matches(':popover-open')) {
        el.hidePopover()
      }
    } catch {
      /* a browser without popover support keeps the plain fixed host */
    }
  }, [items.length])

  const api = useMemo(() => ({ toast }), [toast])

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div
        ref={hostRef}
        popover="manual"
        aria-live="polite"
        aria-atomic="false"
        className="pointer-events-none fixed inset-x-0 top-[calc(4.25rem+env(safe-area-inset-top))] z-[60] m-0 flex flex-col items-center gap-2 border-0 bg-transparent p-0 px-4 lg:top-auto lg:right-6 lg:bottom-6 lg:left-auto lg:items-end"
      >
        {items.map((item) => (
          <div
            key={item.id}
            role={item.tone === 'error' ? 'alert' : 'status'}
            className={cn(
              'glass-bar pointer-events-auto flex w-full max-w-sm items-start gap-3 rounded-2xl px-4 py-3 animate-[toast-in_.28s_var(--ease-out-soft)_both]',
            )}
          >
            {item.tone === 'success' ? (
              <CircleCheck className="mt-0.5 size-5 flex-none text-ok" aria-hidden="true" />
            ) : item.tone === 'info' ? (
              <Info className="mt-0.5 size-5 flex-none text-info" aria-hidden="true" />
            ) : (
              <OctagonAlert className="mt-0.5 size-5 flex-none text-danger" aria-hidden="true" />
            )}
            <div className="min-w-0 flex-1">
              <p className="num text-sm font-semibold text-ink">{item.title}</p>
              {item.description && <p className="mt-0.5 text-xs text-ink-2">{item.description}</p>}
            </div>
            <button type="button" className="-mr-1 grid size-7 place-items-center rounded-full text-ink-3 hover:text-ink" onClick={() => dismiss(item.id)} aria-label={t('common.ui.closeNotification')}>
              <X className="size-4" />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}
