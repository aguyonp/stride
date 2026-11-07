import { Gauge, LayoutDashboard, Link2, ListChecks, MailPlus, MessageSquareText, ScrollText, Users, Wrench, type LucideIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useAdminOverview } from '../../api/queries'
import type { AdminOverview, Me } from '../../api/types'
import { PageHeader } from '../../components/ui/Misc'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { cn } from '../../lib/cn'
import { FeedbackPanel } from './FeedbackPanel'
import { InvitationsPanel } from './InvitationsPanel'
import { JobsPanel } from './JobsPanel'
import { LogsPanel } from './LogsPanel'
import { MaintenancePanel } from './MaintenancePanel'
import { OverviewPanel, overallLevel } from './OverviewPanel'
import { ProvidersPanel } from './ProvidersPanel'
import { SharesPanel } from './SharesPanel'
import { UsersPanel } from './UsersPanel'

export type AdminTab = 'overview' | 'users' | 'jobs' | 'shares' | 'feedback' | 'invitations' | 'providers' | 'logs' | 'maintenance'

const TABS: { value: AdminTab; label: string; icon: LucideIcon }[] = [
  { value: 'overview', label: 'Vue d’ensemble', icon: LayoutDashboard },
  { value: 'users', label: 'Utilisateurs', icon: Users },
  { value: 'jobs', label: 'Tâches', icon: ListChecks },
  { value: 'shares', label: 'Partages', icon: Link2 },
  { value: 'feedback', label: 'Avis', icon: MessageSquareText },
  { value: 'invitations', label: 'Invitations', icon: MailPlus },
  { value: 'providers', label: 'Fournisseurs', icon: Gauge },
  { value: 'logs', label: 'Journaux', icon: ScrollText },
  { value: 'maintenance', label: 'Maintenance', icon: Wrench },
]

function initialTab(): AdminTab {
  const t = new URLSearchParams(window.location.search).get('tab')
  return TABS.some((x) => x.value === t) ? (t as AdminTab) : 'overview'
}

/** Small count next to a tab: live work in accent, failures in warning, plain totals muted. */
function tabCount(tab: AdminTab, ov: AdminOverview | undefined): { n: number; tone: 'accent' | 'warn' | 'muted'; label: string } | null {
  if (!ov) return null
  switch (tab) {
    case 'users':
      return { n: ov.counts.users, tone: 'muted', label: 'comptes' }
    case 'jobs':
      if (ov.jobs.active > 0) return { n: ov.jobs.active, tone: 'accent', label: 'tâches actives' }
      return ov.jobs.failed24h > 0 ? { n: ov.jobs.failed24h, tone: 'warn', label: 'échecs sur 24 h' } : null
    case 'feedback':
      return ov.counts.feedbackNew > 0 ? { n: ov.counts.feedbackNew, tone: 'accent', label: 'avis non lus' } : null
    case 'shares':
      return ov.counts.activeShareLinks > 0 ? { n: ov.counts.activeShareLinks, tone: 'muted', label: 'liens actifs' } : null
  }
  return null
}

const COUNT_TONE = { accent: 'bg-accent text-on-accent', warn: 'bg-warn/20 text-warn', muted: 'bg-track text-ink-3' }
const DOT_TONE = { ok: 'bg-ok', info: 'bg-ok', warn: 'bg-warn', danger: 'bg-danger' }

export default function AdminView({ me }: { me: Me }) {
  useDocumentTitle('Administration')
  const overview = useAdminOverview()
  const [tab, setTabState] = useState<AdminTab>(initialTab)
  const navRef = useRef<HTMLUListElement>(null)
  const setTab = (t: AdminTab) => {
    setTabState(t)
    const sp = new URLSearchParams(window.location.search)
    sp.set('tab', t)
    window.history.replaceState(window.history.state, '', `${window.location.pathname}?${sp.toString()}`)
  }
  // Keep the current tab visible in the scrollable row (again once counts widen the chips).
  const loaded = overview.isSuccess
  useEffect(() => {
    navRef.current?.querySelector('[aria-current="page"]')?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
  }, [tab, loaded])
  const level = overview.data ? overallLevel(overview.data) : null

  return (
    <div>
      <PageHeader title="Administration" subtitle="Centre de contrôle du serveur Stride : santé, comptes, tâches, partages, avis et maintenance." />
      <nav aria-label="Sections d’administration" className="scroll-thin -mx-4 mb-5 overflow-x-auto px-4 pb-1 sm:-mx-6 sm:px-6">
        <ul ref={navRef} className="flex w-max gap-2">
          {TABS.map((t) => {
            const on = tab === t.value
            const count = tabCount(t.value, overview.data)
            return (
              <li key={t.value}>
                <button
                  type="button"
                  aria-current={on ? 'page' : undefined}
                  onClick={() => setTab(t.value)}
                  className={cn('chip relative h-9 border px-3 transition-colors', on ? 'border-accent/50 bg-accent/15 text-ink' : 'border-line bg-track/60 text-ink-2 hover:text-ink')}
                >
                  <t.icon className={cn('size-3.5', on ? 'text-accent' : 'text-ink-3')} aria-hidden="true" />
                  {t.label}
                  {t.value === 'overview' && level && level !== 'ok' && level !== 'info' && (
                    <span className={cn('size-2 rounded-full', DOT_TONE[level])} role="img" aria-label={level === 'danger' ? 'incident' : 'à surveiller'} />
                  )}
                  {count && (
                    <span className={cn('num grid h-5 min-w-5 place-items-center rounded-full px-1.5 text-[11px] font-semibold', COUNT_TONE[count.tone])}>
                      {count.n}
                      <span className="sr-only"> {count.label}</span>
                    </span>
                  )}
                </button>
              </li>
            )
          })}
        </ul>
      </nav>
      <div className="min-w-0">
        {tab === 'overview' && <OverviewPanel query={overview} onNavigate={setTab} />}
        {tab === 'users' && <UsersPanel me={me} />}
        {tab === 'jobs' && <JobsPanel />}
        {tab === 'shares' && <SharesPanel />}
        {tab === 'feedback' && <FeedbackPanel />}
        {tab === 'invitations' && <InvitationsPanel />}
        {tab === 'providers' && <ProvidersPanel />}
        {tab === 'logs' && <LogsPanel />}
        {tab === 'maintenance' && <MaintenancePanel overview={overview.data} />}
      </div>
    </div>
  )
}
