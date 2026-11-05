import { ChevronDown, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import type { Alert, AlertLevel } from '../../api/types'
import { cn } from '../../lib/cn'
import { LEVEL_BG, LEVEL_LABEL, LevelIcon } from '../ui/Badge'

const ORDER: Record<AlertLevel, number> = { danger: 0, warn: 1, info: 2, ok: 3 }

export function sortAlerts(alerts: Alert[]): Alert[] {
  return [...alerts].sort((a, b) => ORDER[a.level] - ORDER[b.level])
}

export function AlertsBanner({ alerts, className, max = 3 }: { alerts: Alert[]; className?: string; max?: number }) {
  const [expanded, setExpanded] = useState(false)
  const sorted = sortAlerts(alerts)
  const serious = sorted.filter((a) => a.level === 'danger' || a.level === 'warn')
  const infos = sorted.filter((a) => a.level === 'info')
  const ok = sorted.find((a) => a.level === 'ok')

  if (serious.length === 0) {
    return (
      <section aria-label="Alertes" className={cn('glass animate-in overflow-hidden p-0', className)}>
        <div className="flex items-center gap-3 bg-ok/10 px-4 py-3.5 sm:px-5">
          <span className="grid size-9 flex-none place-items-center rounded-xl bg-ok/15 text-ok">
            <ShieldCheck className="size-5" aria-hidden="true" />
          </span>
          <div className="min-w-0">
            <p className="font-semibold text-ink">
              <span aria-hidden="true">✅ </span>
              {ok?.title ?? 'Aucun signal d’alerte'}
            </p>
            <p className="text-sm text-ink-2">{ok?.message ?? 'Tous les indicateurs sont dans les seuils de ton profil.'}</p>
          </div>
        </div>
        {infos.length > 0 && (
          <ul className="divide-y divide-line border-t border-line">
            {infos.map((a, i) => (
              <AlertRow key={`${a.code}-${i}`} alert={a} compact />
            ))}
          </ul>
        )}
      </section>
    )
  }

  const all = [...serious, ...infos]
  const visible = expanded ? all : all.slice(0, max)
  const hidden = all.length - visible.length
  const dangerCount = serious.filter((a) => a.level === 'danger').length
  const warnCount = serious.length - dangerCount

  return (
    <section aria-label="Alertes" className={cn('glass animate-in overflow-hidden p-0', className)}>
      <header className="flex items-center justify-between gap-3 px-4 pt-3.5 pb-2 sm:px-5">
        <h2 className="text-[0.95rem] font-semibold tracking-tight">Signaux à surveiller</h2>
        <div className="flex items-center gap-1.5 text-xs font-semibold">
          {dangerCount > 0 && (
            <span className={cn('chip border', LEVEL_BG.danger, 'text-danger')}>
              <LevelIcon level="danger" className="size-3.5" />
              {dangerCount}
            </span>
          )}
          {warnCount > 0 && (
            <span className={cn('chip border', LEVEL_BG.warn, 'text-warn')}>
              <LevelIcon level="warn" className="size-3.5" />
              {warnCount}
            </span>
          )}
        </div>
      </header>
      <ul className="space-y-2 px-3 pb-3 sm:px-4">
        {visible.map((a, i) => (
          <AlertRow key={`${a.code}-${i}`} alert={a} />
        ))}
      </ul>
      {(hidden > 0 || expanded) && all.length > max && (
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="flex w-full items-center justify-center gap-1.5 border-t border-line py-2.5 text-sm font-medium text-ink-2 hover:text-ink"
          aria-expanded={expanded}
        >
          {expanded ? 'Réduire' : `Voir ${hidden} autre${hidden > 1 ? 's signaux' : ' signal'}`}
          <ChevronDown className={cn('size-4 transition-transform', expanded && 'rotate-180')} aria-hidden="true" />
        </button>
      )}
    </section>
  )
}

function AlertRow({ alert, compact }: { alert: Alert; compact?: boolean }) {
  if (compact) {
    return (
      <li className="flex items-start gap-2.5 px-4 py-2.5 text-sm sm:px-5">
        <LevelIcon level={alert.level} className="mt-0.5 flex-none" />
        <p className="min-w-0 text-ink-2">
          <span className="font-semibold text-ink">{alert.title}</span>
          <span className="sr-only"> ({LEVEL_LABEL[alert.level]})</span> — {alert.message}
        </p>
      </li>
    )
  }
  return (
    <li className={cn('flex items-start gap-3 rounded-2xl border px-3 py-2.5', LEVEL_BG[alert.level])}>
      <LevelIcon level={alert.level} className="mt-0.5 size-[1.125rem] flex-none" />
      <div className="min-w-0">
        <p className="text-sm font-semibold text-ink">
          {alert.title}
          <span className="sr-only"> — niveau {LEVEL_LABEL[alert.level]}</span>
        </p>
        <p className="text-sm text-ink-2">{alert.message}</p>
      </div>
    </li>
  )
}
