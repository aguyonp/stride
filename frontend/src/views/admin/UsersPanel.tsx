import { DatabaseBackup, Download, MoreHorizontal, RefreshCw, Trash2, Users } from 'lucide-react'
import { useState } from 'react'
import { api } from '../../api/client'
import { useAdminMutations, useAdminUsers } from '../../api/queries'
import type { AdminUser, Me, Role } from '../../api/types'
import { downloadBlob, fmtDay } from '../../components/profile/download'
import { RestoreForm } from '../../components/profile/RestoreForm'
import { Card } from '../../components/ui/Card'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { Dialog } from '../../components/ui/Dialog'
import { EmptyState } from '../../components/ui/Misc'
import { MenuItem, Popover } from '../../components/ui/Popover'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { cn } from '../../lib/cn'
import { fmtWhen } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { MiniSwitch } from './adminUtils'

export function UsersPanel({ me }: { me: Me }) {
  const users = useAdminUsers()
  const m = useAdminMutations()
  const { toast } = useToast()
  const [del, setDel] = useState<AdminUser | null>(null)
  const [restoreFor, setRestoreFor] = useState<AdminUser | null>(null)

  const setRole = (u: AdminUser, role: Role) =>
    m.updateUser.mutate(
      { id: u.id, role },
      {
        onSuccess: () => toast({ title: `${u.displayName} est maintenant ${role === 'admin' ? 'administrateur' : 'utilisateur'}` }),
        onError: (e) => toast({ tone: 'error', title: 'Modification refusée', description: e.message }),
      },
    )
  const setActive = (u: AdminUser, active: boolean) =>
    m.updateUser.mutate(
      { id: u.id, status: active ? 'active' : 'disabled' },
      {
        onSuccess: () => toast({ title: active ? `${u.displayName} réactivé` : `${u.displayName} désactivé`, description: active ? undefined : 'Ses sessions ont été révoquées.' }),
        onError: (e) => toast({ tone: 'error', title: 'Modification refusée', description: e.message }),
      },
    )
  const resync = (u: AdminUser) =>
    m.resync.mutate(u.id, {
      onSuccess: () => toast({ title: 'Resynchronisation programmée', description: `Cache vidé et synchronisation relancée pour ${u.displayName}.` }),
      onError: (e) => toast({ tone: 'error', title: 'Resynchronisation impossible', description: e.message }),
    })
  const backup = async (u: AdminUser) => {
    try {
      const { blob, filename } = await api.adminBackup(u.id)
      downloadBlob(blob, filename)
      toast({ title: 'Sauvegarde téléchargée', description: filename })
    } catch (e) {
      toast({ tone: 'error', title: 'Sauvegarde impossible', description: e instanceof Error ? e.message : undefined })
    }
  }

  const list = users.data ?? []
  const busyId = m.updateUser.isPending ? m.updateUser.variables?.id : null

  const actions = (u: AdminUser) => (
    <Popover
      trigger={(p) => (
        <button {...p} type="button" className="icon-btn size-9" aria-label={`Actions pour ${u.displayName}`}>
          <MoreHorizontal className="size-4" />
        </button>
      )}
    >
      {(close) => (
        <>
          <MenuItem icon={<RefreshCw />} onSelect={() => (close(), resync(u))}>
            Resynchroniser
          </MenuItem>
          <MenuItem icon={<Download />} onSelect={() => (close(), void backup(u))}>
            Sauvegarde JSON
          </MenuItem>
          <MenuItem icon={<DatabaseBackup />} onSelect={() => (close(), setRestoreFor(u))}>
            Restaurer…
          </MenuItem>
          {u.id !== me.id && (
            <MenuItem icon={<Trash2 />} danger onSelect={() => (close(), setDel(u))}>
              Supprimer
            </MenuItem>
          )}
        </>
      )}
    </Popover>
  )

  const roleSelect = (u: AdminUser, self: boolean) => (
    <div className="w-44 max-w-full">
      <select className="field min-h-9 py-1 text-sm" aria-label={`Rôle de ${u.displayName}`} value={u.role} disabled={self || busyId === u.id} onChange={(e) => setRole(u, e.target.value as Role)}>
        <option value="user">Utilisateur</option>
        <option value="admin">Administrateur</option>
      </select>
    </div>
  )

  const statusSwitch = (u: AdminUser, self: boolean) => (
    <span className="flex items-center gap-2">
      <MiniSwitch checked={u.status === 'active'} disabled={self || busyId === u.id} onChange={(v) => setActive(u, v)} label={`Compte de ${u.displayName} actif`} />
      <span className={cn('text-xs', u.status === 'active' ? 'text-ink-2' : 'text-danger')}>{u.status === 'active' ? 'Actif' : 'Désactivé'}</span>
    </span>
  )

  const name = (u: AdminUser, self: boolean) => (
    <span className="flex min-w-0 items-center gap-2">
      <span className="truncate font-semibold text-ink">{u.displayName}</span>
      {self && <span className="chip h-5 flex-none border border-accent/40 bg-accent/12 px-2 text-[10px] text-accent">toi</span>}
    </span>
  )

  return (
    <Card title="Utilisateurs" icon={<Users className="size-4" />} subtitle={users.data ? `${list.length} compte${list.length > 1 ? 's' : ''}` : undefined}>
      {users.isError ? (
        <ErrorView error={users.error} onRetry={() => void users.refetch()} title="Utilisateurs indisponibles" />
      ) : !users.data ? (
        <div className="space-y-2">
          <Skeleton className="h-14" />
          <Skeleton className="h-14" />
        </div>
      ) : list.length === 0 ? (
        <EmptyState title="Aucun utilisateur" />
      ) : (
        <>
          <div className="hidden lg:block">
            <table className="w-full border-separate border-spacing-0 text-sm">
              <caption className="sr-only">Comptes Stride</caption>
              <thead>
                <tr className="text-left text-xs text-ink-3">
                  {['Utilisateur', 'Rôle', 'Statut', 'Créé', 'Dernière connexion', 'Passkeys', ''].map((h, i) => (
                    <th key={i} scope="col" className="border-b border-line px-2 py-2 font-medium first:pl-1">
                      {h || <span className="sr-only">Actions</span>}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {list.map((u) => {
                  const self = u.id === me.id
                  return (
                    <tr key={u.id} className="hover:bg-track/50">
                      <th scope="row" className="max-w-[14rem] border-b border-line px-2 py-2 pl-1 text-left font-normal">
                        {name(u, self)}
                      </th>
                      <td className="border-b border-line px-2 py-2">{roleSelect(u, self)}</td>
                      <td className="border-b border-line px-2 py-2">{statusSwitch(u, self)}</td>
                      <td className="num border-b border-line px-2 py-2 text-ink-2">{fmtDay(u.createdAt)}</td>
                      <td className="num border-b border-line px-2 py-2 text-ink-2">{u.lastLoginAt ? fmtWhen(u.lastLoginAt) : 'jamais'}</td>
                      <td className="num border-b border-line px-2 py-2 text-ink-2">{u.passkeys}</td>
                      <td className="border-b border-line px-2 py-2 text-right">{actions(u)}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
          <ul className="space-y-2 lg:hidden">
            {list.map((u) => {
              const self = u.id === me.id
              return (
                <li key={u.id} className="glass-inset p-3">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      {name(u, self)}
                      <p className="num mt-0.5 text-xs text-ink-3">
                        Créé le {fmtDay(u.createdAt)} · {u.passkeys} passkey{u.passkeys > 1 ? 's' : ''}
                      </p>
                      <p className="num text-xs text-ink-3">Connexion : {u.lastLoginAt ? fmtWhen(u.lastLoginAt) : 'jamais'}</p>
                    </div>
                    {actions(u)}
                  </div>
                  <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
                    {roleSelect(u, self)}
                    {statusSwitch(u, self)}
                  </div>
                </li>
              )
            })}
          </ul>
        </>
      )}

      <ConfirmDialog
        open={del !== null}
        onClose={() => setDel(null)}
        title={`Supprimer ${del?.displayName ?? ''} ?`}
        message={<p>Le compte et toutes ses données (activités, ressentis, plan, connexions, passkeys) seront supprimés définitivement.</p>}
        confirmLabel="Supprimer"
        danger
        requireText="SUPPRIMER"
        busy={m.deleteUser.isPending}
        onConfirm={() =>
          del &&
          m.deleteUser.mutate(del.id, {
            onSuccess: () => {
              toast({ title: `${del.displayName} supprimé` })
              setDel(null)
            },
            onError: (e) => toast({ tone: 'error', title: 'Suppression impossible', description: e.message }),
          })
        }
      />
      <Dialog open={restoreFor !== null} onClose={() => setRestoreFor(null)} title={`Restaurer ${restoreFor?.displayName ?? ''}`} description="Sauvegarde stride-backup au format JSON">
        {restoreFor && <RestoreForm targetLabel={restoreFor.displayName} onRestore={(mode, backup) => m.restore.mutateAsync({ id: restoreFor.id, mode, backup })} />}
      </Dialog>
    </Card>
  )
}
