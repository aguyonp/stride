import { CircleCheck, Info, OctagonAlert, TriangleAlert } from 'lucide-react'
import type { ReactNode } from 'react'
import type { AlertLevel } from '../../api/types'
import { cn } from '../../lib/cn'

export const LEVEL_TEXT: Record<AlertLevel, string> = {
  ok: 'text-ok',
  info: 'text-info',
  warn: 'text-warn',
  danger: 'text-danger',
}

export const LEVEL_BG: Record<AlertLevel, string> = {
  ok: 'bg-ok/12 border-ok/25',
  info: 'bg-info/12 border-info/25',
  warn: 'bg-warn/12 border-warn/30',
  danger: 'bg-danger/12 border-danger/30',
}

export const LEVEL_LABEL: Record<AlertLevel, string> = {
  ok: 'OK',
  info: 'Info',
  warn: 'Attention',
  danger: 'Danger',
}

export function LevelIcon({ level, className }: { level: AlertLevel; className?: string }) {
  const Icon = level === 'ok' ? CircleCheck : level === 'info' ? Info : level === 'warn' ? TriangleAlert : OctagonAlert
  return <Icon className={cn('size-4', LEVEL_TEXT[level], className)} aria-hidden="true" />
}

/** Status pill: icon + label, never colour alone. */
export function Badge({ level, children, className }: { level: AlertLevel; children: ReactNode; className?: string }) {
  return (
    <span className={cn('chip border', LEVEL_BG[level], LEVEL_TEXT[level], className)}>
      <LevelIcon level={level} className="size-3.5" />
      {children}
    </span>
  )
}
