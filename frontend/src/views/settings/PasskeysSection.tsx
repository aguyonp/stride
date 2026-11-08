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
import { t } from '../../i18n'

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
      toast({ title: t('settings.passkeys.added'), description: p.name })
    },
  })
  const items = list.data ?? []
  const last = items.length <= 1

  return (
    <Section
      id="passkeys"
      title={t('settings.passkeys.title')}
      icon={<KeyRound />}
      description={t('settings.passkeys.description')}
      actions={
        <button type="button" className="btn btn-primary" onClick={() => add.mutate()} disabled={add.isPending}>
          {add.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Plus className="size-4" aria-hidden="true" />}
          {t('settings.passkeys.add')}
        </button>
      }
    >
      {add.isError && <InlineError className="mb-3">{passkeyErrorMessage(add.error, 'add')}</InlineError>}
      {list.isError ? (
        <ErrorView error={list.error} onRetry={() => void list.refetch()} title={t('settings.passkeys.unavailable')} />
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
          {last && <p className="mt-3 text-xs text-ink-3">{t('settings.passkeys.lastOne')}</p>}
        </>
      )}
      <ConfirmDialog
        open={del !== null}
        onClose={() => setDel(null)}
        title={t('settings.passkeys.deleteTitle')}
        message={<p>{t('settings.passkeys.deleteBody', { name: del?.name })}</p>}
        confirmLabel={t('common.actions.delete')}
        danger
        busy={m.remove.isPending}
        onConfirm={() =>
          del &&
          m.remove.mutate(del.id, {
            onSuccess: () => {
              setDel(null)
              toast({ title: t('settings.passkeys.deleted') })
            },
            onError: (e) => {
              setDel(null)
              toast({ tone: 'error', title: t('settings.passkeys.deleteRefused'), description: isApiError(e, 'CONFLICT') ? t('settings.passkeys.cannotDeleteLast') : e.message })
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
          toast({ title: t('settings.passkeys.renamed') })
        },
        onError: (e) => toast({ tone: 'error', title: t('settings.passkeys.renameFailed'), description: e.message }),
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
            <input className="field min-h-9 py-1" value={name} maxLength={60} autoFocus aria-label={t('settings.passkeys.newName')} onChange={(e) => setName(e.target.value)} onKeyDown={(e) => e.key === 'Escape' && (setName(p.name), setEditing(false))} />
            <button type="submit" className="icon-btn size-9" aria-label={t('settings.passkeys.saveName')} disabled={rename.isPending}>
              {rename.isPending ? <Loader2 className="size-4 animate-spin" /> : <Check className="size-4" />}
            </button>
            <button type="button" className="icon-btn size-9" aria-label={t('common.actions.cancel')} onClick={() => (setName(p.name), setEditing(false))}>
              <X className="size-4" />
            </button>
          </form>
        ) : (
          <>
            <p className="flex min-w-0 items-center gap-2">
              <span className="truncate font-semibold text-ink">{p.name}</span>
              {p.backedUp && (
                <span className="chip h-5 flex-none border border-ok/30 bg-ok/10 px-2 text-[10px] text-ok">
                  <CloudCheck className="size-3" aria-hidden="true" /> {t('settings.passkeys.backedUp')}
                </span>
              )}
            </p>
            <p className="text-xs text-ink-3">
              {t('settings.passkeys.created', { date: fmtDay(p.createdAt) })} · {p.lastUsedAt ? t('settings.passkeys.used', { when: fmtWhen(p.lastUsedAt) }) : t('settings.passkeys.neverUsed')}
            </p>
          </>
        )}
      </div>
      {!editing && (
        <div className="flex flex-none gap-1">
          <button type="button" className="icon-btn size-9" onClick={() => setEditing(true)} aria-label={t('settings.passkeys.rename', { name: p.name })}>
            <Pencil className="size-4" />
          </button>
          <button type="button" className="icon-btn size-9" onClick={onDelete} disabled={last} aria-label={last ? t('settings.passkeys.cannotDeleteLabel') : t('settings.passkeys.deleteLabel', { name: p.name })} title={last ? t('settings.passkeys.lastHint') : undefined}>
            <Trash2 className="size-4" />
          </button>
        </div>
      )}
    </li>
  )
}
