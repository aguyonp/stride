import type { ReactNode } from 'react'
import { cn } from '../../lib/cn'

interface CardProps {
  title?: ReactNode
  icon?: ReactNode
  action?: ReactNode
  subtitle?: ReactNode
  className?: string
  bodyClassName?: string
  children: ReactNode
  as?: 'section' | 'div' | 'article'
  'aria-label'?: string
}

export function Card({ title, icon, action, subtitle, className, bodyClassName, children, as: Tag = 'section', ...rest }: CardProps) {
  return (
    <Tag className={cn('glass animate-in min-w-0 p-4 sm:p-5', className)} aria-label={rest['aria-label']}>
      {(title || action) && (
        <header className="mb-3 flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
          <div className="flex min-w-[min(100%,11rem)] flex-1 items-center gap-2.5">
            {icon && (
              <span className="grid size-8 flex-none place-items-center rounded-xl bg-accent/12 text-accent" aria-hidden="true">
                {icon}
              </span>
            )}
            <div className="min-w-0">
              {title && <h2 className="truncate text-[0.95rem] font-semibold tracking-tight text-ink">{title}</h2>}
              {subtitle && <p className="truncate text-xs text-ink-3">{subtitle}</p>}
            </div>
          </div>
          {action && <div className="ml-auto flex-none">{action}</div>}
        </header>
      )}
      <div className={bodyClassName}>{children}</div>
    </Tag>
  )
}
