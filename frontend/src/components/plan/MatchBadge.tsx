import type { MatchStatus } from '../../api/types'
import { MATCH_STATUS } from '../../lib/labels'
import { Badge } from '../ui/Badge'

/** Plan vs done status pill (« Réalisée », « Partielle », « Manquée », « À venir », « Hors plan »). */
export function MatchBadge({ status, className }: { status: MatchStatus; className?: string }) {
  const m = MATCH_STATUS[status]
  return (
    <Badge level={m.level} className={className}>
      {m.label}
    </Badge>
  )
}
