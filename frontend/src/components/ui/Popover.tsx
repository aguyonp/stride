import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { cn } from '../../lib/cn'

interface PopoverProps {
  trigger: (props: { 'aria-expanded': boolean; 'aria-controls': string; 'aria-haspopup': 'menu' | 'dialog'; onClick: () => void }) => ReactNode
  children: (close: () => void) => ReactNode
  align?: 'left' | 'right'
  kind?: 'menu' | 'dialog'
  className?: string
  panelClassName?: string
}

/** Anchored panel closed by outside click, Escape or item selection. */
export function Popover({ trigger, children, align = 'right', kind = 'menu', className, panelClassName }: PopoverProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const id = useId()

  useEffect(() => {
    if (!open) return
    const onDown = (e: Event) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setOpen(false)
        ref.current?.querySelector<HTMLElement>('[aria-expanded]')?.focus()
      }
    }
    document.addEventListener('pointerdown', onDown)
    document.addEventListener('keydown', onKey)
    requestAnimationFrame(() => ref.current?.querySelector<HTMLElement>('[data-autofocus], [role="menuitem"], button:not([aria-expanded])')?.focus())
    return () => {
      document.removeEventListener('pointerdown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  return (
    <div ref={ref} className={cn('relative', className)}>
      {trigger({ 'aria-expanded': open, 'aria-controls': id, 'aria-haspopup': kind, onClick: () => setOpen((v) => !v) })}
      {open && (
        <div
          id={id}
          role={kind}
          className={cn(
            'sheet-panel absolute top-[calc(100%+0.5rem)] z-50 w-[min(18rem,calc(100vw-2rem))] rounded-2xl p-1.5 animate-[pop-in_.18s_var(--ease-out-soft)_both]',
            align === 'right' ? 'right-0' : 'left-0',
            panelClassName,
          )}
        >
          {children(() => setOpen(false))}
        </div>
      )}
    </div>
  )
}

export function MenuItem({ icon, children, onSelect, danger, href }: { icon?: ReactNode; children: ReactNode; onSelect?: () => void; danger?: boolean; href?: string }) {
  const cls = cn('flex w-full min-h-10 items-center gap-3 rounded-xl px-3 py-2 text-left text-sm hover:bg-track focus-visible:bg-track', danger ? 'text-danger' : 'text-ink')
  const content = (
    <>
      {icon && <span className={cn('flex-none [&>svg]:size-4', danger ? 'text-danger' : 'text-ink-3')} aria-hidden="true">{icon}</span>}
      <span className="min-w-0 flex-1 truncate">{children}</span>
    </>
  )
  if (href) {
    return (
      <a role="menuitem" href={href} className={cls} onClick={onSelect}>
        {content}
      </a>
    )
  }
  return (
    <button type="button" role="menuitem" className={cls} onClick={onSelect}>
      {content}
    </button>
  )
}
