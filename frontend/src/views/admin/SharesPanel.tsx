import { EyeOff, Link2, Loader2, Unlink } from 'lucide-react'
import { useState } from 'react'
import { useAdminControl, useAdminShares } from '../../api/queries'
import type { AdminShareLink } from '../../api/types'
import { Badge } from '../../components/ui/Badge'
import { Card } from '../../components/ui/Card'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { EmptyState } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { fmtDateTime, fmtWhen } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { FilterChips } from './adminUtils'

const STATE = {
  active: { label: 'Actif', level: 'ok' },
  expired: { label: 'Expiré', level: 'info' },
  revoked: { label: 'Révoqué', level: 'danger' },
} as const

const scope = (l: AdminShareLink) => `${l.periodWeeks} sem. · ${l.sections.length}/8 sections`
const expiry = (l: AdminShareLink) => (l.revokedAt ? `révoqué ${fmtWhen(l.revokedAt)}` : l.expiresAt ? fmtDateTime(l.expiresAt) : 'jamais')

export function SharesPanel() {
  const [filter, setFilter] = useState<'active' | 'all'>('active')
  const shares = useAdminShares(filter)
  const ctl = useAdminControl()
  const { toast } = useToast()
  const [revoke, setRevoke] = useState<AdminShareLink | null>(null)
  const items = shares.data ?? []

  const views = (l: AdminShareLink) => (
    <>
      <span className="num text-ink">{l.accessCount}</span>
      {l.lastAccessedAt && <span className="num block text-[11px] text-ink-3">{fmtWhen(l.lastAccessedAt)}</span>}
    </>
  )
  const revokeBtn = (l: AdminShareLink) =>
    l.state === 'active' && (
      <button type="button" className="btn btn-danger min-h-9 px-3 text-xs" onClick={() => setRevoke(l)} disabled={ctl.revokeShare.isPending && ctl.revokeShare.variables === l.id}>
        {ctl.revokeShare.isPending && ctl.revokeShare.variables === l.id ? <Loader2 className="size-3.5 animate-spin" aria-hidden="true" /> : <Unlink className="size-3.5" aria-hidden="true" />}
        Révoquer<span className="sr-only"> le lien {l.label}</span>
      </button>
    )
  const anon = (l: AdminShareLink) =>
    l.anonymize && (
      <span className="chip h-5 border border-line bg-track/60 px-1.5 text-[10px] font-medium text-ink-2">
        <EyeOff className="size-3" aria-hidden="true" />
        anonyme
      </span>
    )

  return (
    <Card title="Liens de partage" icon={<Link2 className="size-4" />} subtitle="Vues publiques en lecture seule, tous comptes · jeton jamais affiché (préfixe seulement)">
      <div className="mb-4">
        <FilterChips<'active' | 'all'>
          ariaLabel="Filtrer les liens"
          value={filter}
          onChange={setFilter}
          options={[
            { value: 'active', label: 'Actifs' },
            { value: 'all', label: 'Tous (expirés et révoqués)' },
          ]}
        />
      </div>
      {shares.isError ? (
        <ErrorView error={shares.error} onRetry={() => void shares.refetch()} title="Liens indisponibles" />
      ) : !shares.data ? (
        <Skeleton className="h-28" />
      ) : items.length === 0 ? (
        <EmptyState icon={<Link2 />} title={filter === 'active' ? 'Aucun lien actif' : 'Aucun lien de partage'}>
          Les athlètes créent ces liens depuis Santé & forme pour leur kiné ou leur coach. Révoquer un lien le coupe immédiatement.
        </EmptyState>
      ) : (
        <>
          <div className="scroll-thin hidden overflow-x-auto md:block">
            <table className="w-full min-w-[720px] border-separate border-spacing-0 text-sm">
              <caption className="sr-only">Liens de partage de tous les comptes</caption>
              <thead>
                <tr className="text-left text-xs text-ink-3">
                  {['Lien', 'Propriétaire', 'Portée', 'Expiration', 'Vues', 'État', ''].map((h, i) => (
                    <th key={i} scope="col" className="border-b border-line px-2 py-2 font-medium first:pl-1">
                      {h || <span className="sr-only">Actions</span>}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {items.map((l) => (
                  <tr key={l.id} className="align-top hover:bg-track/50">
                    <th scope="row" className="border-b border-line px-2 py-2 pl-1 text-left font-normal">
                      <span className="block font-medium text-ink">{l.label}</span>
                      <span className="block font-mono text-[11px] text-ink-3">{l.tokenPrefix}…</span>
                    </th>
                    <td className="border-b border-line px-2 py-2 whitespace-nowrap text-ink-2">{l.userName}</td>
                    <td className="border-b border-line px-2 py-2 text-ink-2">
                      <span className="num block whitespace-nowrap">{scope(l)}</span>
                      {anon(l)}
                    </td>
                    <td className="num border-b border-line px-2 py-2 whitespace-nowrap text-ink-2">{expiry(l)}</td>
                    <td className="border-b border-line px-2 py-2">{views(l)}</td>
                    <td className="border-b border-line px-2 py-2">
                      <Badge level={STATE[l.state].level}>{STATE[l.state].label}</Badge>
                    </td>
                    <td className="border-b border-line px-2 py-2 text-right">{revokeBtn(l)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <ul className="space-y-2 md:hidden">
            {items.map((l) => (
              <li key={l.id} className="glass-inset p-3 text-sm">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <span className="block truncate font-medium text-ink">{l.label}</span>
                    <span className="block text-xs text-ink-3">
                      {l.userName} · <span className="font-mono">{l.tokenPrefix}…</span>
                    </span>
                  </div>
                  <Badge level={STATE[l.state].level}>{STATE[l.state].label}</Badge>
                </div>
                <p className="num mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-ink-3">
                  {scope(l)} · expire {expiry(l)} · {l.accessCount} vue{l.accessCount > 1 ? 's' : ''} {anon(l)}
                </p>
                {l.state === 'active' && <div className="mt-2">{revokeBtn(l)}</div>}
              </li>
            ))}
          </ul>
        </>
      )}

      <ConfirmDialog
        open={revoke !== null}
        onClose={() => setRevoke(null)}
        title="Révoquer ce lien ?"
        message={
          <p>
            « {revoke?.label} » de {revoke?.userName} cessera de fonctionner immédiatement (vue publique et cache). Son propriétaire pourra en créer un nouveau.
          </p>
        }
        confirmLabel="Révoquer"
        danger
        busy={ctl.revokeShare.isPending}
        onConfirm={() =>
          revoke &&
          ctl.revokeShare.mutate(revoke.id, {
            onSuccess: () => {
              toast({ title: 'Lien révoqué', description: `${revoke.label} · ${revoke.userName}` })
              setRevoke(null)
            },
            onError: (e) => toast({ tone: 'error', title: 'Révocation impossible', description: e.message }),
          })
        }
      />
    </Card>
  )
}
