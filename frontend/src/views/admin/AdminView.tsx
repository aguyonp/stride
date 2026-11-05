import { useState } from 'react'
import type { Me } from '../../api/types'
import { PageHeader } from '../../components/ui/Misc'
import { SegmentedControl } from '../../components/ui/SegmentedControl'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { InvitationsPanel } from './InvitationsPanel'
import { MaintenancePanel } from './MaintenancePanel'
import { ProvidersPanel } from './ProvidersPanel'
import { UsersPanel } from './UsersPanel'

type Tab = 'users' | 'invitations' | 'providers' | 'maintenance'

const TABS: { value: Tab; short: string; long: string }[] = [
  { value: 'users', short: 'Comptes', long: 'Utilisateurs' },
  { value: 'invitations', short: 'Invit.', long: 'Invitations' },
  { value: 'providers', short: 'API', long: 'Fournisseurs' },
  { value: 'maintenance', short: 'Outils', long: 'Maintenance' },
]

function initialTab(): Tab {
  const t = new URLSearchParams(window.location.search).get('tab')
  return TABS.some((x) => x.value === t) ? (t as Tab) : 'users'
}

export default function AdminView({ me }: { me: Me }) {
  useDocumentTitle('Administration')
  const [tab, setTabState] = useState<Tab>(initialTab)
  const setTab = (t: Tab) => {
    setTabState(t)
    const sp = new URLSearchParams(window.location.search)
    sp.set('tab', t)
    window.history.replaceState(window.history.state, '', `${window.location.pathname}?${sp.toString()}`)
  }
  return (
    <div>
      <PageHeader title="Administration" subtitle="Comptes, invitations, fournisseurs et maintenance du serveur Stride." />
      <SegmentedControl<Tab>
        ariaLabel="Section d’administration"
        className="mb-5 w-full max-w-2xl"
        value={tab}
        onChange={setTab}
        options={TABS.map((t) => ({
          value: t.value,
          ariaLabel: t.long,
          label: (
            <>
              <span className="sm:hidden">{t.short}</span>
              <span className="hidden sm:inline">{t.long}</span>
            </>
          ),
        }))}
      />
      <div className="min-w-0">
        {tab === 'users' && <UsersPanel me={me} />}
        {tab === 'invitations' && <InvitationsPanel />}
        {tab === 'providers' && <ProvidersPanel />}
        {tab === 'maintenance' && <MaintenancePanel />}
      </div>
    </div>
  )
}
