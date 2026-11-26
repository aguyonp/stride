import {
  DatabaseBackup,
  Gauge,
  LayoutDashboard,
  Link2,
  ListChecks,
  MailPlus,
  MessageSquareText,
  ScrollText,
  SlidersHorizontal,
  type LucideIcon,
  Users,
  Webhook,
  Wrench,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useAdminOverview } from '../../api/queries'
import type { AdminOverview, Me } from '../../api/types'
import { PageHeader } from '../../components/ui/Misc'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { cn } from '../../lib/cn'
import { BackupsPanel } from './BackupsPanel'
import { FeedbackPanel } from './FeedbackPanel'
import { InvitationsPanel } from './InvitationsPanel'
import { SystemPanel } from './SystemPanel'
import { JobsPanel } from './JobsPanel'
import { LogsPanel } from './LogsPanel'
import { McpAdminPanel } from './McpAdminPanel'
import { MaintenancePanel } from './MaintenancePanel'
import { OverviewPanel, overallLevel } from './OverviewPanel'
import { ProvidersPanel } from './ProvidersPanel'
import { SharesPanel } from './SharesPanel'
import { UsersPanel } from './UsersPanel'
import { t } from '../../i18n'

export type AdminTab = 'overview' | 'users' | 'jobs' | 'shares' | 'feedback' | 'invitations' | 'providers' | 'mcp' | 'system' | 'logs' | 'maintenance' | 'backups'

const TABS: { value: AdminTab; label: string; icon: LucideIcon }[] = [
  { value: 'overview', icon: LayoutDashboard, get label() { return t('admin.common.tabs.overview') } },
  { value: 'users', icon: Users, get label() { return t('admin.common.tabs.users') } },
  { value: 'jobs', icon: ListChecks, get label() { return t('admin.common.tabs.jobs') } },
  { value: 'shares', icon: Link2, get label() { return t('admin.common.tabs.shares') } },
  { value: 'feedback', icon: MessageSquareText, get label() { return t('admin.common.tabs.feedback') } },
  { value: 'invitations', icon: MailPlus, get label() { return t('admin.common.tabs.invitations') } },
  { value: 'providers', icon: Gauge, get label() { return t('admin.common.tabs.providers') } },
  { value: 'mcp', icon: Webhook, get label() { return t('admin.common.tabs.mcp') } },
  { value: 'system', icon: SlidersHorizontal, get label() { return t('admin.common.tabs.system') } },
  { value: 'logs', icon: ScrollText, get label() { return t('admin.common.tabs.logs') } },
  { value: 'maintenance', icon: Wrench, get label() { return t('admin.common.tabs.maintenance') } },
  { value: 'backups', icon: DatabaseBackup, get label() { return t('admin.common.tabs.backups') } },
]

function initialTab(): AdminTab {
  const q = new URLSearchParams(window.location.search).get('tab')
  return TABS.some((x) => x.value === q) ? (q as AdminTab) : 'overview'
}

/** Small count next to a tab: live work in accent, failures in warning, plain totals muted. */
function tabCount(tab: AdminTab, ov: AdminOverview | undefined): { n: number; tone: 'accent' | 'warn' | 'muted'; label: string } | null {
  if (!ov) return null
  switch (tab) {
    case 'users':
      return { n: ov.counts.users, tone: 'muted', label: t('admin.common.countAccounts') }
    case 'jobs':
      if (ov.jobs.active > 0) return { n: ov.jobs.active, tone: 'accent', label: t('admin.common.countActiveJobs') }
      return ov.jobs.failed24h > 0 ? { n: ov.jobs.failed24h, tone: 'warn', label: t('admin.common.countFailures') } : null
    case 'feedback':
      return ov.counts.feedbackNew > 0 ? { n: ov.counts.feedbackNew, tone: 'accent', label: t('admin.common.countUnread') } : null
    case 'shares':
      return ov.counts.activeShareLinks > 0 ? { n: ov.counts.activeShareLinks, tone: 'muted', label: t('admin.common.countLinks') } : null
  }
  return null
}

const COUNT_TONE = { accent: 'bg-accent text-on-accent', warn: 'bg-warn/20 text-warn', muted: 'bg-track text-ink-3' }
const DOT_TONE = { ok: 'bg-ok', info: 'bg-ok', warn: 'bg-warn', danger: 'bg-danger' }

export default function AdminView({ me }: { me: Me }) {
  useDocumentTitle(t('admin.common.title'))
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
      <PageHeader title={t('admin.common.title')} subtitle={t('admin.common.subtitle')} />
      <nav aria-label={t('admin.common.sections')} className="scroll-thin -mx-4 mb-5 overflow-x-auto px-4 pb-1 sm:-mx-6 sm:px-6">
        <ul ref={navRef} className="flex w-max gap-2">
          {TABS.map((tb) => {
            const on = tab === tb.value
            const count = tabCount(tb.value, overview.data)
            return (
              <li key={tb.value}>
                <button
                  type="button"
                  aria-current={on ? 'page' : undefined}
                  onClick={() => setTab(tb.value)}
                  className={cn('chip relative h-9 border px-3 transition-colors', on ? 'border-accent/50 bg-accent/15 text-ink' : 'border-line bg-track/60 text-ink-2 hover:text-ink')}
                >
                  <tb.icon className={cn('size-3.5', on ? 'text-accent' : 'text-ink-3')} aria-hidden="true" />
                  {tb.label}
                  {tb.value === 'overview' && level && level !== 'ok' && level !== 'info' && (
                    <span className={cn('size-2 rounded-full', DOT_TONE[level])} role="img" aria-label={level === 'danger' ? t('admin.common.incident') : t('admin.common.watch')} />
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
        {tab === 'system' && <SystemPanel />}
        {tab === 'providers' && <ProvidersPanel />}
        {tab === 'mcp' && <McpAdminPanel />}
        {tab === 'logs' && <LogsPanel />}
        {tab === 'maintenance' && <MaintenancePanel overview={overview.data} />}
        {tab === 'backups' && <BackupsPanel />}
      </div>
    </div>
  )
}
