import { Repeat } from 'lucide-react'
import type { WorkoutStep } from '../../api/types'
import { cn } from '../../lib/cn'
import { STEP_LABEL, STEP_TONE, stepAmount, targetText } from './steps'
import { t } from '../../i18n'

/** Readable workout structure: one row per step, repeats as an indented group. */
export function StepsView({ steps, className }: { steps: WorkoutStep[]; className?: string }) {
  if (!steps.length) return null
  return (
    <ol className={cn('space-y-1.5', className)} aria-label={t('plan.format.structure')}>
      {steps.map((s, i) => (
        <li key={i}>
          {s.type === 'repeat' ? (
            <div className="rounded-xl border border-violet/25 bg-violet/[0.06] p-2">
              <p className="mb-1.5 flex items-center gap-1.5 px-1 text-xs font-semibold text-violet">
                <Repeat className="size-3.5" aria-hidden="true" />
                {s.repeat ?? 1} ×
              </p>
              <StepsView steps={s.steps ?? []} />
            </div>
          ) : (
            <div className="flex items-start gap-2.5 rounded-xl bg-track/50 px-3 py-2">
              <span className={cn('mt-1 h-3.5 w-1 flex-none rounded-full', STEP_TONE[s.type])} aria-hidden="true" />
              <div className="min-w-0 flex-1 text-sm">
                <p className="flex flex-wrap items-baseline gap-x-2">
                  <span className="font-medium text-ink">{STEP_LABEL[s.type]}</span>
                  {stepAmount(s) && <span className="num text-ink-2">{stepAmount(s)}</span>}
                  {targetText(s.target) && <span className="num text-xs text-ink-3">@ {targetText(s.target)}</span>}
                </p>
                {s.note && <p className="text-xs text-ink-3">{s.note}</p>}
              </div>
            </div>
          )}
        </li>
      ))}
    </ol>
  )
}
