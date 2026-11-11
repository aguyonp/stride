import { CircleCheck, Info } from 'lucide-react'
import type { PlannedSession } from '../../api/types'
import { fmtDateTime } from '../../lib/format'
import { t } from '../../i18n'

export function GarminStatus({ s, compact }: { s: PlannedSession; compact?: boolean }) {
  const g = s.garmin
  if (!g || g.status === 'none') return null
  if (g.status === 'pushed')
    return (
      <span className="flex items-center gap-1 text-xs text-ok">
        <CircleCheck className="size-3.5" aria-hidden="true" /> {compact ? t('plan.format.onGarmin') : g.pushedAt ? t('plan.format.pushedAt', { date: fmtDateTime(g.pushedAt) }) : t('plan.format.pushed')}
      </span>
    )
  if (compact || g.status === 'unsupported') return null
  return (
    <p className="flex items-start gap-1.5 text-xs text-ink-3">
      <Info className="mt-px size-3.5 flex-none text-info" aria-hidden="true" />
      {g.error ? t('plan.format.pushFailedReason', { error: g.error }) : t('plan.format.pushFailed')}
    </p>
  )
}
