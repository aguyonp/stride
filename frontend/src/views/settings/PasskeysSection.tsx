import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Check, CloudCheck, KeyRound, Loader2, Pencil, Plus, Trash2, X } from 'lucide-react'
import { useState } from 'react'
import { isApiError } from '../../api/client'
import { qk, usePasskeyMutations, usePasskeys } from '../../api/queries'
import type { Passkey } from '../../api/types'
import { addPasskey, passkeyErrorMessage } from '../../api/webauthn'
import { fmtDay } from '../../components/profile/download'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { InlineError } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { fmtWhen } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { Section } from './Section'

export function PasskeysSection() {
  const qc = useQueryClient()
  const list = usePasskeys()
  const m = usePasskeyMutations()
  const { toast } = useToast()
  const [del, setDel] = useState<Passkey | null>(null)
  const add = useMutation({
    mutationFn: addPasskey,
    onSuccess: (p) => {
      void qc.invalidateQueries({ queryKey: qk.passkeys })
      toast({ title: 'Passkey ajoutée', description: p.name })
    },
  })
  const items = list.data ?? []
  const last = items.length <= 1

  return (
    <Section
      id="passkeys"
      title="Passkeys"
      icon={<KeyRound />}
      description="Stride se connecte uniquement par passkey. Ajoute-en une par appareil (téléphone, ordinateur, clé de sécurité)."
      actions={
        <button type="button" className="btn btn-primary" onClick={() => add.mutate()} disabled={add.isPending}>
          {add.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Plus className="size-4" aria-hidden="true" />}
          Ajouter une passkey
        </button>
      }
    >
      {add.isError && <InlineError className="mb-3">{passkeyErrorMessage(add.error, 'add')}</InlineError>}
      {list.isError ? (
        <ErrorView error={list.error} onRetry={() => void list.refetch()} title="Passkeys indisponibles" />
      ) : !list.data ? (
        <div className="space-y-2">
          <Skeleton className="h-16" />
          <Skeleton className="h-16" />
        </div>
      ) : (
        <>
          <ul className="space-y-2">
            {items.map((p) => (
              <PasskeyRow key={p.id} p={p} last={last} onDelete={() => setDel(p)} />
            ))}
          </ul>
          {last && <p className="mt-3 text-xs text-ink-3">Ta seule passkey ne peut pas être supprimée : ajoute d’abord une autre passkey pour ne pas perdre l’accès à ton compte.</p>}
        </>
      )}
      <ConfirmDialog
        open={del !== null}
        onClose={() => setDel(null)}
        title="Supprimer cette passkey ?"
        message={<p>« {del?.name} » ne pourra plus servir à se connecter à Stride. Pense aussi à la retirer du gestionnaire de mots de passe de l’appareil.</p>}
        confirmLabel="Supprimer"
        danger
        busy={m.remove.isPending}
        onConfirm={() =>
          del &&
          m.remove.mutate(del.id, {
            onSuccess: () => {
              setDel(null)
              toast({ title: 'Passkey supprimée' })
            },
            onError: (e) => {
              setDel(null)
              toast({ tone: 'error', title: 'Suppression refusée', description: isApiError(e, 'CONFLICT') ? 'Impossible de supprimer ta dernière passkey.' : e.message })
            },
          })
        }
      />
    </Section>
  )
}

function PasskeyRow({ p, last, onDelete }: { p: Passkey; last: boolean; onDelete: () => void }) {
  const { rename } = usePasskeyMutations()
  const { toast } = useToast()
  const [editing, setEditing] = useState(false)
  const [name, setName] = useState(p.name)

  const save = () => {
    const n = name.trim()
    if (!n || n === p.name) return setEditing(false)
    rename.mutate(
      { id: p.id, name: n },
      {
        onSuccess: () => {
          setEditing(false)
          toast({ title: 'Passkey renommée' })
        },
        onError: (e) => toast({ tone: 'error', title: 'Renommage impossible', description: e.message }),
      },
    )
  }

  return (
    <li className="glass-inset flex min-w-0 flex-wrap items-center gap-3 p-3">
      <span className="grid size-9 flex-none place-items-center rounded-xl bg-track text-ink-2" aria-hidden="true">
        <KeyRound className="size-4" />
      </span>
      <div className="min-w-0 flex-1 basis-40">
        {editing ? (
          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              save()
            }}
          >
            <input className="field min-h-9 py-1" value={name} maxLength={60} autoFocus aria-label="Nouveau nom de la passkey" onChange={(e) => setName(e.target.value)} onKeyDown={(e) => e.key === 'Escape' && (setName(p.name), setEditing(false))} />
            <button type="submit" className="icon-btn size-9" aria-label="Enregistrer le nom" disabled={rename.isPending}>
              {rename.isPending ? <Loader2 className="size-4 animate-spin" /> : <Check className="size-4" />}
            </button>
            <button type="button" className="icon-btn size-9" aria-label="Annuler" onClick={() => (setName(p.name), setEditing(false))}>
              <X className="size-4" />
            </button>
          </form>
        ) : (
          <>
            <p className="flex min-w-0 items-center gap-2">
              <span className="truncate font-semibold text-ink">{p.name}</span>
              {p.backedUp && (
                <span className="chip h-5 flex-none border border-ok/30 bg-ok/10 px-2 text-[10px] text-ok">
                  <CloudCheck className="size-3" aria-hidden="true" /> Sauvegardée
                </span>
              )}
            </p>
            <p className="text-xs text-ink-3">
              Créée le {fmtDay(p.createdAt)} · {p.lastUsedAt ? `utilisée ${fmtWhen(p.lastUsedAt)}` : 'jamais utilisée'}
            </p>
          </>
        )}
      </div>
      {!editing && (
        <div className="flex flex-none gap-1">
          <button type="button" className="icon-btn size-9" onClick={() => setEditing(true)} aria-label={`Renommer ${p.name}`}>
            <Pencil className="size-4" />
          </button>
          <button type="button" className="icon-btn size-9" onClick={onDelete} disabled={last} aria-label={last ? 'Impossible de supprimer la dernière passkey' : `Supprimer ${p.name}`} title={last ? 'Dernière passkey : ajoute-en une autre avant de la supprimer' : undefined}>
            <Trash2 className="size-4" />
          </button>
        </div>
      )}
    </li>
  )
}
