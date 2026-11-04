import { CircleCheck, OctagonAlert, X } from 'lucide-react'
import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { cn } from '../../lib/cn'

type Tone = 'success' | 'error'
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
  if (!ctx) throw new Error('useToast hors ToastProvider')
  return ctx
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])
  const seq = useRef(0)

  const dismiss = useCallback((id: number) => setItems((xs) => xs.filter((x) => x.id !== id)), [])

  const toast = useCallback<ToastApi['toast']>(
    ({ title, description, tone = 'success' }) => {
      const id = ++seq.current
      setItems((xs) => [...xs.slice(-2), { id, title, description, tone }])
      window.setTimeout(() => dismiss(id), tone === 'error' ? 6000 : 3500)
    },
    [dismiss],
  )

  const api = useMemo(() => ({ toast }), [toast])

  return (
    <ToastContext.Provider value={api}>
      {children}
      {/* Zone live toujours présente pour que les lecteurs d'écran annoncent les messages. */}
      <div
        aria-live="polite"
        aria-atomic="false"
        className="pointer-events-none fixed inset-x-0 bottom-[calc(5.75rem+env(safe-area-inset-bottom))] z-[60] flex flex-col items-center gap-2 px-4 lg:right-6 lg:bottom-6 lg:left-auto lg:items-end"
      >
        {items.map((t) => (
          <div
            key={t.id}
            role={t.tone === 'error' ? 'alert' : 'status'}
            className={cn(
              'glass-bar pointer-events-auto flex w-full max-w-sm items-start gap-3 rounded-2xl px-4 py-3 animate-[toast-in_.28s_var(--ease-out-soft)_both]',
            )}
          >
            {t.tone === 'success' ? (
              <CircleCheck className="mt-0.5 size-5 flex-none text-ok" aria-hidden="true" />
            ) : (
              <OctagonAlert className="mt-0.5 size-5 flex-none text-danger" aria-hidden="true" />
            )}
            <div className="min-w-0 flex-1">
              <p className="num text-sm font-semibold text-ink">{t.title}</p>
              {t.description && <p className="mt-0.5 text-xs text-ink-2">{t.description}</p>}
            </div>
            <button type="button" className="-mr-1 grid size-7 place-items-center rounded-full text-ink-3 hover:text-ink" onClick={() => dismiss(t.id)} aria-label="Fermer la notification">
              <X className="size-4" />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}
