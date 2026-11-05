import { Database, HardDrive, Radio } from 'lucide-react'
import type { Freshness } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtWhen } from '../../lib/format'
import { FRESHNESS_LABEL } from '../../lib/labels'

const ICON = { db: Database, cache: HardDrive, live: Radio }
const TONE = {
  db: 'border-line bg-track text-ink-2',
  cache: 'border-info/30 bg-info/10 text-info',
  live: 'border-accent/35 bg-accent/12 text-accent',
}

export function FreshnessPill({ freshness, className }: { freshness: Freshness | null | undefined; className?: string }) {
  if (!freshness) return null
  const Icon = ICON[freshness.source]
  const meta = FRESHNESS_LABEL[freshness.source]
  return (
    <span className={cn('chip border', TONE[freshness.source], className)} title={meta.hint}>
      <Icon className="size-3.5" aria-hidden="true" />
      <span>{meta.label}</span>
      <span className="num font-medium opacity-80">· {fmtWhen(freshness.updatedAt)}</span>
      <span className="sr-only">. {meta.hint}</span>
    </span>
  )
}
