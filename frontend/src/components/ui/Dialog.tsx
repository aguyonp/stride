import { X } from 'lucide-react'
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { cn } from '../../lib/cn'

interface DialogProps {
  open: boolean
  onClose: () => void
  title: ReactNode
  description?: ReactNode
  children: ReactNode
  footer?: ReactNode
  /** sheet : feuille basse (mobile) / modale centrée (desktop) ; drawer : panneau latéral droit sur desktop. */
  variant?: 'sheet' | 'drawer'
  size?: 'md' | 'lg' | 'xl'
  headerExtra?: ReactNode
  bodyClassName?: string
  /** Élément à focaliser à l'ouverture (sélecteur CSS dans la dialog). */
  initialFocus?: string
}

/**
 * Dialog basée sur <dialog> natif : focus piégé, Échap, fond inerte, top-layer.
 * Bas de l'écran sur mobile, centrée (sheet) ou latérale (drawer) sur desktop.
 */
export function Dialog({ open, onClose, title, description, children, footer, variant = 'sheet', size = 'md', headerExtra, bodyClassName, initialFocus }: DialogProps) {
  const ref = useRef<HTMLDialogElement>(null)
  const [mounted, setMounted] = useState(open)
  const titleId = useId()

  if (open && !mounted) setMounted(true)

  useEffect(() => {
    const d = ref.current
    if (!d) return
    if (open) {
      d.removeAttribute('data-closing')
      if (!d.open) {
        d.showModal()
        document.documentElement.style.overflow = 'hidden'
        if (initialFocus) requestAnimationFrame(() => d.querySelector<HTMLElement>(initialFocus)?.focus())
      }
    } else if (d.open) {
      d.setAttribute('data-closing', '')
      const t = window.setTimeout(() => {
        d.close()
        d.removeAttribute('data-closing')
        document.documentElement.style.overflow = ''
        setMounted(false)
      }, 160)
      return () => window.clearTimeout(t)
    }
  }, [open, mounted, initialFocus])

  useEffect(() => () => void (document.documentElement.style.overflow = ''), [])

  if (!mounted) return null

  const sizeCls = size === 'xl' ? 'md:max-w-4xl' : size === 'lg' ? 'md:max-w-2xl' : 'md:max-w-lg'
  const placement =
    variant === 'drawer'
      ? cn('fixed inset-x-0 bottom-0 max-h-[92dvh] rounded-t-[26px] md:inset-y-3 md:right-3 md:left-auto md:max-h-none md:w-[min(560px,calc(100vw-1.5rem))] md:rounded-[24px]')
      : cn('fixed inset-x-0 bottom-0 max-h-[92dvh] rounded-t-[26px] md:inset-x-0 md:top-1/2 md:bottom-auto md:mx-auto md:max-h-[86dvh] md:w-[calc(100vw-3rem)] md:-translate-y-1/2 md:rounded-[24px]', sizeCls)
  const anim = variant === 'drawer' ? 'max-md:animate-[sheet-up_.28s_var(--ease-out-soft)_both] md:animate-[slide-left_.28s_var(--ease-out-soft)_both]' : 'max-md:animate-[sheet-up_.28s_var(--ease-out-soft)_both] md:animate-[pop-in_.24s_var(--ease-out-soft)_both]'

  return (
    <dialog
      ref={ref}
      className="sheet"
      onCancel={(e) => {
        e.preventDefault()
        onClose()
      }}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
      aria-labelledby={titleId}
    >
      <div className={cn('sheet-panel flex flex-col overflow-hidden', placement, anim, '[dialog[data-closing]_&]:opacity-0 [dialog[data-closing]_&]:transition-opacity [dialog[data-closing]_&]:duration-150')}>
        <div className="mx-auto mt-2 h-1 w-10 flex-none rounded-full bg-line-strong md:hidden" aria-hidden="true" />
        <header className="flex flex-none items-start justify-between gap-3 border-b border-line px-5 pt-3 pb-3 md:pt-5">
          <div className="min-w-0">
            <h2 id={titleId} className="text-base font-semibold tracking-tight">
              {title}
            </h2>
            {description && <p className="mt-0.5 text-xs text-ink-3">{description}</p>}
          </div>
          <div className="flex flex-none items-center gap-2">
            {headerExtra}
            <button type="button" className="icon-btn size-9" onClick={onClose} aria-label="Fermer">
              <X className="size-4" />
            </button>
          </div>
        </header>
        <div className={cn('scroll-thin min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 py-4', bodyClassName)}>{children}</div>
        {footer && <footer className="pb-safe flex-none border-t border-line px-5 pt-3">{footer}</footer>}
      </div>
    </dialog>
  )
}
