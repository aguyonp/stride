import type { ReactNode } from 'react'
import { cn } from '../../lib/cn'

export function Section({ id, title, description, icon, actions, children, className }: { id: string; title: string; description?: ReactNode; icon?: ReactNode; actions?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section id={id} aria-labelledby={`${id}-title`} className={cn('glass min-w-0 scroll-mt-24 p-4 sm:p-6', className)}>
      <header className="mb-4 flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
        <div className="flex min-w-0 items-start gap-3">
          {icon && (
            <span className="grid size-9 flex-none place-items-center rounded-xl bg-accent/12 text-accent [&>svg]:size-[1.125rem]" aria-hidden="true">
              {icon}
            </span>
          )}
          <div className="min-w-0">
            <h2 id={`${id}-title`} className="text-lg font-semibold tracking-tight">
              {title}
            </h2>
            {description && <p className="mt-0.5 text-sm text-ink-3">{description}</p>}
          </div>
        </div>
        {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
      </header>
      {children}
    </section>
  )
}
