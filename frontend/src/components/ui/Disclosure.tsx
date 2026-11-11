import { ChevronDown } from 'lucide-react'
import { useId, useState, type ReactNode } from 'react'
import { cn } from '../../lib/cn'
import { CardNesting } from './Card'
import { t } from '../../i18n'

interface DisclosureProps {
  title: ReactNode
  summary?: ReactNode
  icon?: ReactNode
  open?: boolean
  defaultOpen?: boolean
  onToggle?: (open: boolean) => void
  /** Small coloured marker when an alert points to this section. */
  flag?: 'warn' | 'danger' | null
  cardTitles?: 'sub' | 'hide'
  id?: string
  className?: string
  bodyClassName?: string
  children: ReactNode
}

/** Collapsible section: h2 > button[aria-expanded]; body mounted on first open and kept afterwards. */
export function Disclosure({ title, summary, icon, open: controlled, defaultOpen = false, onToggle, flag, cardTitles = 'sub', id, className, bodyClassName, children }: DisclosureProps) {
  const [inner, setInner] = useState(defaultOpen)
  const open = controlled ?? inner
  const [seen, setSeen] = useState(open)
  if (open && !seen) setSeen(true)
  const bodyId = useId()
  const toggle = () => {
    if (controlled === undefined) setInner(!open)
    onToggle?.(!open)
  }
  return (
    <section id={id} className={cn('min-w-0 scroll-mt-24', className)}>
      <h2 className="m-0">
        <button
          type="button"
          aria-expanded={open}
          aria-controls={bodyId}
          onClick={toggle}
          className="group flex w-full items-center gap-3 rounded-[inherit] px-4 py-3.5 text-left transition-colors hover:bg-white/[0.03] focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent sm:px-5"
        >
          {icon && (
            <span className="grid size-8 flex-none place-items-center rounded-xl bg-track text-ink-2 [&_svg]:size-4" aria-hidden="true">
              {icon}
            </span>
          )}
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-2 text-[0.95rem] font-semibold tracking-tight text-ink">
              {title}
              {flag && <span className={cn('size-2 flex-none rounded-full', flag === 'danger' ? 'bg-danger' : 'bg-warn')} aria-label={t('common.ui.linkedSignal')} role="img" />}
            </span>
            {summary && <span className="mt-0.5 block truncate text-xs font-normal text-ink-3">{summary}</span>}
          </span>
          <ChevronDown className={cn('size-4 flex-none text-ink-3 transition-transform duration-200', open && 'rotate-180')} aria-hidden="true" />
        </button>
      </h2>
      <div id={bodyId} hidden={!open} className={cn('section-body px-4 pt-1 pb-5 sm:px-5', bodyClassName)}>
        {seen && <CardNesting.Provider value={cardTitles}>{children}</CardNesting.Provider>}
      </div>
    </section>
  )
}
