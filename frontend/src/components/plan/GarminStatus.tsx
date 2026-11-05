import { CircleCheck, Info } from 'lucide-react'
import type { PlannedSession } from '../../api/types'
import { fmtDateTime } from '../../lib/format'

export function GarminStatus({ s, compact }: { s: PlannedSession; compact?: boolean }) {
  const g = s.garmin
  if (!g || g.status === 'none') return null
  if (g.status === 'pushed')
    return (
      <span className="flex items-center gap-1 text-xs text-ok">
        <CircleCheck className="size-3.5" aria-hidden="true" /> {compact ? 'Sur Garmin' : `Envoyée sur Garmin${g.pushedAt ? ` le ${fmtDateTime(g.pushedAt)}` : ''}`}
      </span>
    )
  if (compact || g.status === 'unsupported') return null
  return (
    <p className="flex items-start gap-1.5 text-xs text-ink-3">
      <Info className="mt-px size-3.5 flex-none text-info" aria-hidden="true" />
      Dernier envoi échoué{g.error ? ` : ${g.error}` : '.'}
    </p>
  )
}
