import { DatabaseBackup, Download, LogOut, MoreHorizontal, RefreshCw, RotateCcw, Search, Trash2, Users } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { api } from '../../api/client'
import { useAdminControl, useAdminMutations, useAdminUsers } from '../../api/queries'
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
import { fmtNumber, fmtWhen } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { FilterChips, fmtBytes, INTEGRATION_LABEL, isLastActiveAdmin, MiniSwitch } from './adminUtils'

type StatusFilter = 'all' | 'active' | 'disabled' | 'admin'
type SortKey = 'seen' | 'name' | 'created' | 'activities' | 'storage'

const SORTS: { value: SortKey; label: string }[] = [
  { value: 'seen', label: 'Dernière activité' },
  { value: 'name', label: 'Nom' },
  { value: 'created', label: 'Inscription récente' },
  { value: 'activities', label: 'Activités' },
  { value: 'storage', label: 'Stockage' },
]

const lastSeen = (u: AdminUser) => u.lastSeenAt ?? u.lastLoginAt
const time = (s: string | null) => (s ? new Date(s).getTime() : 0)
const fold = (s: string) => s.normalize('NFD').replace(/\p{Diacritic}/gu, '').toLowerCase()

function compare(key: SortKey): (a: AdminUser, b: AdminUser) => number {
  switch (key) {
    case 'name':
      return (a, b) => a.displayName.localeCompare(b.displayName, 'fr')
    case 'created':
      return (a, b) => time(b.createdAt) - time(a.createdAt)
    case 'activities':
      return (a, b) => b.activities - a.activities
    case 'storage':
      return (a, b) => b.storageBytes - a.storageBytes
    default:
      return (a, b) => time(lastSeen(b)) - time(lastSeen(a))
  }
}

/** A pending access change waiting for confirmation. */
type Pending = { u: AdminUser; kind: 'role'; role: Role } | { u: AdminUser; kind: 'status'; active: boolean } | { u: AdminUser; kind: 'sessions' }

function confirmCopy(p: Pending): { title: string; message: ReactNode; label: string; danger: boolean } {
  const n = p.u.displayName
  if (p.kind === 'role')
    return p.role === 'admin'
      ? { title: `Promouvoir ${n} administrateur ?`, message: <p>Ce compte pourra gérer les utilisateurs, les invitations, les tâches, les journaux et la maintenance du serveur.</p>, label: 'Promouvoir', danger: false }
      : { title: `Retirer les droits d’administration de ${n} ?`, message: <p>Le compte redevient un utilisateur standard. Ses données ne changent pas.</p>, label: 'Rétrograder', danger: true }
  if (p.kind === 'status')
    return p.active
      ? { title: `Réactiver ${n} ?`, message: <p>La personne pourra de nouveau se connecter avec ses passkeys.</p>, label: 'Réactiver', danger: false }
      : {
          title: `Désactiver ${n} ?`,
          message: <p>Toutes ses sessions sont fermées immédiatement et la connexion est refusée tant que le compte reste désactivé. Ses données sont conservées.</p>,
          label: 'Désactiver',
          danger: true,
        }
  return {
    title: `Fermer les sessions de ${n} ?`,
    message: (
      <p>
        {p.u.sessions} session{p.u.sessions > 1 ? 's' : ''} ouverte{p.u.sessions > 1 ? 's' : ''} sur ses appareils. La personne devra se reconnecter avec une passkey ; le compte reste actif.
      </p>
    ),
    label: 'Fermer les sessions',
    danger: true,
  }
}

export function UsersPanel({ me }: { me: Me }) {
  const users = useAdminUsers()
  const m = useAdminMutations()
  const ctl = useAdminControl()
  const { toast } = useToast()
  const [q, setQ] = useState('')
  const [filter, setFilter] = useState<StatusFilter>('all')
  const [sort, setSort] = useState<SortKey>('seen')
  const [pending, setPending] = useState<Pending | null>(null)
  const [del, setDel] = useState<AdminUser | null>(null)
  const [restoreFor, setRestoreFor] = useState<AdminUser | null>(null)

  const all = useMemo(() => users.data ?? [], [users.data])
  const list = useMemo(() => {
    const needle = fold(q.trim())
    return all
      .filter((u) => (filter === 'active' ? u.status === 'active' : filter === 'disabled' ? u.status === 'disabled' : filter === 'admin' ? u.role === 'admin' : true))
      .filter((u) => !needle || fold(u.displayName).includes(needle))
      .sort(compare(sort))
  }, [all, q, filter, sort])

  const fail = (title: string) => (e: Error) => toast({ tone: 'error', title, description: e.message })
  const runPending = () => {
    if (!pending) return
    const { u } = pending
    const done = () => setPending(null)
    if (pending.kind === 'role') {
      const role = pending.role
      m.updateUser.mutate({ id: u.id, role }, { onSuccess: () => (done(), toast({ title: `${u.displayName} est maintenant ${role === 'admin' ? 'administrateur' : 'utilisateur'}` })), onError: fail('Modification refusée') })
    } else if (pending.kind === 'status') {
      const active = pending.active
      m.updateUser.mutate(
        { id: u.id, status: active ? 'active' : 'disabled' },
        { onSuccess: () => (done(), toast({ title: active ? `${u.displayName} réactivé` : `${u.displayName} désactivé`, description: active ? undefined : 'Ses sessions ont été fermées.' })), onError: fail('Modification refusée') },
      )
    } else {
      ctl.revokeSessions.mutate(u.id, {
        onSuccess: (r) => (done(), toast({ title: 'Sessions fermées', description: `${r.revoked} session${r.revoked > 1 ? 's' : ''} révoquée${r.revoked > 1 ? 's' : ''} pour ${u.displayName}.` })),
        onError: fail('Révocation impossible'),
      })
    }
  }
  const syncNow = (u: AdminUser) =>
    ctl.syncUser.mutate(u.id, {
      onSuccess: () => toast({ title: 'Synchronisation lancée', description: `Suis-la dans l’onglet Tâches (${u.displayName}).` }),
      onError: fail('Synchronisation impossible'),
    })
  const resync = (u: AdminUser) =>
    m.resync.mutate(u.id, {
      onSuccess: () => toast({ title: 'Resynchronisation programmée', description: `Cache vidé et synchronisation complète relancée pour ${u.displayName}.` }),
      onError: fail('Resynchronisation impossible'),
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

  const busyId = m.updateUser.isPending ? m.updateUser.variables?.id : null
  const guard = (u: AdminUser) => {
    const self = u.id === me.id
    const last = isLastActiveAdmin(all, u)
    return { self, last, locked: self || last, why: self ? 'Ton propre compte se gère dans Réglages.' : last ? 'Dernier administrateur actif : promeus d’abord un autre compte.' : undefined }
  }

  const actions = (u: AdminUser) => {
    const g = guard(u)
    const connected = u.integrations.includes('garmin') || u.integrations.includes('strava')
    return (
      <Popover
        trigger={(p) => (
          <button {...p} type="button" className="icon-btn size-9" aria-label={`Actions pour ${u.displayName}`}>
            <MoreHorizontal className="size-4" />
          </button>
        )}
      >
        {(close) => (
          <>
            {connected && u.status === 'active' && (
              <MenuItem icon={<RefreshCw />} onSelect={() => (close(), syncNow(u))}>
                Synchroniser maintenant
              </MenuItem>
            )}
            <MenuItem icon={<RotateCcw />} onSelect={() => (close(), resync(u))}>
              Resynchronisation complète
            </MenuItem>
            {!g.self && u.sessions > 0 && (
              <MenuItem icon={<LogOut />} onSelect={() => (close(), setPending({ u, kind: 'sessions' }))}>
                Fermer les sessions ({u.sessions})
              </MenuItem>
            )}
            <MenuItem icon={<Download />} onSelect={() => (close(), void backup(u))}>
              Sauvegarde JSON
            </MenuItem>
            <MenuItem icon={<DatabaseBackup />} onSelect={() => (close(), setRestoreFor(u))}>
              Restaurer…
            </MenuItem>
            {!g.locked && (
              <MenuItem icon={<Trash2 />} danger onSelect={() => (close(), setDel(u))}>
                Supprimer…
              </MenuItem>
            )}
          </>
        )}
      </Popover>
    )
  }

  const roleSelect = (u: AdminUser) => {
    const g = guard(u)
    return (
      <select
        className="field min-h-9 w-40 max-w-full py-1 text-sm"
        aria-label={`Rôle de ${u.displayName}`}
        title={g.why}
        value={u.role}
        disabled={g.locked || busyId === u.id}
        onChange={(e) => setPending({ u, kind: 'role', role: e.target.value as Role })}
      >
        <option value="user">Utilisateur</option>
        <option value="admin">Administrateur</option>
      </select>
    )
  }

  const statusSwitch = (u: AdminUser) => {
    const g = guard(u)
    return (
      <span className="flex items-center gap-2" title={g.why}>
        <MiniSwitch checked={u.status === 'active'} disabled={g.locked || busyId === u.id} onChange={(v) => setPending({ u, kind: 'status', active: v })} label={`Compte de ${u.displayName} actif`} />
        <span className={cn('text-xs', u.status === 'active' ? 'text-ink-2' : 'font-medium text-danger')}>{u.status === 'active' ? 'Actif' : 'Désactivé'}</span>
      </span>
    )
  }

  const name = (u: AdminUser) => (
    <span className="block min-w-0">
      <span className="flex min-w-0 items-center gap-2">
        <span className="truncate font-semibold text-ink">{u.displayName}</span>
        {u.id === me.id && <span className="chip h-5 flex-none border border-accent/40 bg-accent/12 px-2 text-[10px] text-accent">toi</span>}
      </span>
      <span className="mt-0.5 flex flex-wrap items-center gap-1">
        {u.integrations.length === 0 ? (
          <span className="text-xs text-ink-3">Aucune intégration</span>
        ) : (
          u.integrations.map((i) => (
            <span key={i} className="chip h-5 border border-line bg-track/60 px-1.5 text-[10px] font-medium text-ink-2">
              {INTEGRATION_LABEL[i] ?? i}
            </span>
          ))
        )}
      </span>
    </span>
  )

  const seen = (u: AdminUser) => (lastSeen(u) ? fmtWhen(lastSeen(u)) : 'jamais')
  const confirm = pending ? confirmCopy(pending) : null
  const confirmBusy = m.updateUser.isPending || ctl.revokeSessions.isPending
  const counts = { all: all.length, active: all.filter((u) => u.status === 'active').length, disabled: all.filter((u) => u.status === 'disabled').length, admin: all.filter((u) => u.role === 'admin').length }

  return (
    <Card title="Utilisateurs" icon={<Users className="size-4" />} subtitle={users.data ? `${all.length} compte${all.length > 1 ? 's' : ''} · ${counts.disabled} désactivé${counts.disabled > 1 ? 's' : ''}` : undefined}>
      {users.isError ? (
        <ErrorView error={users.error} onRetry={() => void users.refetch()} title="Utilisateurs indisponibles" />
      ) : !users.data ? (
        <div className="space-y-2">
          <Skeleton className="h-14" />
          <Skeleton className="h-14" />
        </div>
      ) : all.length === 0 ? (
        <EmptyState title="Aucun utilisateur" />
      ) : (
        <>
          <div className="mb-4 flex flex-wrap items-center gap-x-3 gap-y-2.5">
            <label className="relative w-full sm:w-60">
              <span className="sr-only">Rechercher un utilisateur</span>
              <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-3" aria-hidden="true" />
              <input type="search" className="field min-h-10 pl-9 text-sm" placeholder="Rechercher un nom" value={q} onChange={(e) => setQ(e.target.value)} />
            </label>
            <FilterChips<StatusFilter>
              ariaLabel="Filtrer les comptes"
              value={filter}
              onChange={setFilter}
              options={[
                { value: 'all', label: 'Tous', count: counts.all },
                { value: 'active', label: 'Actifs', count: counts.active },
                { value: 'disabled', label: 'Désactivés', count: counts.disabled },
                { value: 'admin', label: 'Admins', count: counts.admin },
              ]}
            />
            <label className="flex items-center gap-2 text-xs text-ink-3 sm:ml-auto">
              Trier par
              <select className="field min-h-9 w-auto py-1 text-sm" value={sort} onChange={(e) => setSort(e.target.value as SortKey)}>
                {SORTS.map((s) => (
                  <option key={s.value} value={s.value}>
                    {s.label}
                  </option>
                ))}
              </select>
            </label>
          </div>

          {list.length === 0 ? (
            <EmptyState icon={<Search />} title="Aucun compte ne correspond">
              Modifie la recherche ou le filtre.
            </EmptyState>
          ) : (
            <>
              <div className="hidden xl:block">
                <table className="w-full border-separate border-spacing-0 text-sm">
                  <caption className="sr-only">Comptes Stride</caption>
                  <thead>
                    <tr className="text-left text-xs text-ink-3">
                      {['Utilisateur', 'Rôle', 'Statut', 'Vu', 'Activités', 'Stockage', 'Sessions', ''].map((h, i) => (
                        <th key={i} scope="col" className={cn('border-b border-line px-2 py-2 font-medium first:pl-1', i >= 4 && i <= 6 && 'text-right')}>
                          {h || <span className="sr-only">Actions</span>}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {list.map((u) => (
                      <tr key={u.id} className={cn('hover:bg-track/50', u.status === 'disabled' && 'text-ink-3')}>
                        <th scope="row" className="max-w-[16rem] border-b border-line px-2 py-2 pl-1 text-left font-normal">
                          {name(u)}
                        </th>
                        <td className="border-b border-line px-2 py-2">{roleSelect(u)}</td>
                        <td className="border-b border-line px-2 py-2">{statusSwitch(u)}</td>
                        <td className="num border-b border-line px-2 py-2 whitespace-nowrap text-ink-2" title={`Inscrit le ${fmtDay(u.createdAt)}`}>
                          {seen(u)}
                        </td>
                        <td className="num border-b border-line px-2 py-2 text-right text-ink-2">{fmtNumber(u.activities)}</td>
                        <td className="num border-b border-line px-2 py-2 text-right whitespace-nowrap text-ink-2">{fmtBytes(u.storageBytes)}</td>
                        <td className="num border-b border-line px-2 py-2 text-right text-ink-2">{u.sessions}</td>
                        <td className="border-b border-line px-2 py-2 text-right">{actions(u)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <ul className="space-y-2 xl:hidden">
                {list.map((u) => (
                  <li key={u.id} className="glass-inset p-3">
                    <div className="flex items-start justify-between gap-2">
                      {name(u)}
                      {actions(u)}
                    </div>
                    <p className="num mt-1.5 text-xs text-ink-3">
                      Vu {seen(u)} · {fmtNumber(u.activities)} activité{u.activities > 1 ? 's' : ''} · {fmtBytes(u.storageBytes)} · {u.sessions} session{u.sessions > 1 ? 's' : ''}
                    </p>
                    <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
                      {roleSelect(u)}
                      {statusSwitch(u)}
                    </div>
                  </li>
                ))}
              </ul>
            </>
          )}
        </>
      )}

      {confirm && pending && (
        <ConfirmDialog open onClose={() => setPending(null)} title={confirm.title} message={confirm.message} confirmLabel={confirm.label} danger={confirm.danger} busy={confirmBusy} onConfirm={runPending} />
      )}
      <ConfirmDialog
        open={del !== null}
        onClose={() => setDel(null)}
        title={`Supprimer ${del?.displayName ?? ''} ?`}
        message={
          <p>
            Le compte et toutes ses données (activités, ressentis, plan, connexions, liens de partage, passkeys) seront supprimés définitivement. Télécharge une sauvegarde JSON avant si besoin.
          </p>
        }
        confirmLabel="Supprimer définitivement"
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
            onError: fail('Suppression impossible'),
          })
        }
      />
      <Dialog open={restoreFor !== null} onClose={() => setRestoreFor(null)} title={`Restaurer ${restoreFor?.displayName ?? ''}`} description="Sauvegarde stride-backup au format JSON">
        {restoreFor && <RestoreForm targetLabel={restoreFor.displayName} onRestore={(mode, backup) => m.restore.mutateAsync({ id: restoreFor.id, mode, backup })} />}
      </Dialog>
    </Card>
  )
}
