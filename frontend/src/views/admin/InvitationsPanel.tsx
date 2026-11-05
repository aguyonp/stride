import { Ban, Copy, DoorOpen, Loader2, Mail, Plus, TriangleAlert } from 'lucide-react'
import { useId, useState } from 'react'
import { useAdminInvitations, useAdminMutations, useAdminSettings } from '../../api/queries'
import type { Invitation, InvitationCreated, RegistrationMode } from '../../api/types'
import { fmtDay } from '../../components/profile/download'
import { Card } from '../../components/ui/Card'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { Dialog } from '../../components/ui/Dialog'
import { EmptyState, InlineError } from '../../components/ui/Misc'
import { SegmentedControl } from '../../components/ui/SegmentedControl'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { copyText } from '../../lib/clipboard'
import { cn } from '../../lib/cn'
import { fmtDateTime } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { INVITATION_STATE, invitationState } from './adminUtils'

const EXPIRY = [
  { value: '24', label: '24 heures' },
  { value: '72', label: '3 jours' },
  { value: '168', label: '7 jours' },
  { value: '720', label: '30 jours' },
]

export function InvitationsPanel() {
  return (
    <div className="space-y-5">
      <RegistrationCard />
      <CreateInvitationCard />
      <InvitationList />
    </div>
  )
}

function RegistrationCard() {
  const settings = useAdminSettings()
  const m = useAdminMutations()
  const { toast } = useToast()
  const mode = settings.data?.registrationMode
  return (
    <Card title="Inscriptions" icon={<DoorOpen className="size-4" />} subtitle="Qui peut créer un compte">
      {settings.isError ? (
        <ErrorView error={settings.error} onRetry={() => void settings.refetch()} />
      ) : !mode ? (
        <Skeleton className="h-11 max-w-md" />
      ) : (
        <>
          <SegmentedControl<RegistrationMode>
            ariaLabel="Mode d’inscription"
            className="max-w-md"
            value={mode}
            onChange={(v) =>
              m.updateSettings.mutate(
                { registrationMode: v },
                {
                  onSuccess: () => toast({ title: v === 'open' ? 'Inscriptions ouvertes' : 'Inscriptions sur invitation' }),
                  onError: (e) => toast({ tone: 'error', title: 'Modification impossible', description: e.message }),
                },
              )
            }
            options={[
              { value: 'invite', label: 'Sur invitation' },
              { value: 'open', label: 'Ouvertes' },
            ]}
          />
          <p className="mt-2 text-sm text-ink-3">
            {mode === 'invite'
              ? 'Seules les personnes disposant d’un code ou d’un lien d’invitation valide peuvent créer un compte.'
              : 'Toute personne qui accède à Stride peut créer un compte avec une passkey. À réserver à un réseau de confiance.'}
          </p>
        </>
      )}
    </Card>
  )
}

function CreateInvitationCard() {
  const ids = useId()
  const m = useAdminMutations()
  const [label, setLabel] = useState('')
  const [maxUses, setMaxUses] = useState('1')
  const [hours, setHours] = useState('168')
  const [created, setCreated] = useState<InvitationCreated | null>(null)
  const uses = Number(maxUses)
  const usesError = !Number.isInteger(uses) || uses < 1 || uses > 100

  const submit = () => {
    if (usesError) return
    m.createInvitation.mutate(
      { label: label.trim() || undefined, maxUses: uses, expiresInHours: Number(hours) },
      {
        onSuccess: (r) => {
          setCreated(r)
          setLabel('')
          setMaxUses('1')
        },
      },
    )
  }

  return (
    <Card title="Nouvelle invitation" icon={<Mail className="size-4" />} subtitle="Le code et le lien ne sont affichés qu’une seule fois">
      <form
        className="grid gap-3 sm:grid-cols-[minmax(0,1.6fr)_minmax(0,0.7fr)_minmax(0,1fr)_auto] sm:items-end"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="min-w-0">
          <label htmlFor={`${ids}-l`} className="mb-1.5 block text-sm font-medium text-ink-2">
            Libellé
          </label>
          <input id={`${ids}-l`} className="field" value={label} maxLength={60} placeholder="Ex. Club du mardi" onChange={(e) => setLabel(e.target.value)} />
        </div>
        <div className="min-w-0">
          <label htmlFor={`${ids}-u`} className="mb-1.5 block text-sm font-medium text-ink-2">
            Utilisations
          </label>
          <input id={`${ids}-u`} className="field num" type="number" inputMode="numeric" min={1} max={100} value={maxUses} aria-invalid={usesError ? true : undefined} onChange={(e) => setMaxUses(e.target.value)} />
        </div>
        <div className="min-w-0">
          <label htmlFor={`${ids}-h`} className="mb-1.5 block text-sm font-medium text-ink-2">
            Expire après
          </label>
          <select id={`${ids}-h`} className="field" value={hours} onChange={(e) => setHours(e.target.value)}>
            {EXPIRY.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
        <button type="submit" className="btn btn-primary min-h-11" disabled={usesError || m.createInvitation.isPending}>
          {m.createInvitation.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Plus className="size-4" aria-hidden="true" />}
          Créer
        </button>
      </form>
      {usesError && <p className="mt-2 text-xs text-danger">Entre 1 et 100 utilisations.</p>}
      {m.createInvitation.isError && <InlineError className="mt-3">{m.createInvitation.error.message}</InlineError>}
      <CreatedDialog created={created} onClose={() => setCreated(null)} />
    </Card>
  )
}

function CopyField({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  const id = useId()
  const { toast } = useToast()
  return (
    <div>
      <label htmlFor={id} className="mb-1.5 block text-sm font-medium text-ink-2">
        {label}
      </label>
      <div className="flex gap-2">
        <input id={id} readOnly value={value} className={cn('field min-w-0 flex-1', mono && 'font-mono')} onFocus={(e) => e.currentTarget.select()} />
        <button
          type="button"
          className="btn btn-ghost flex-none"
          onClick={async () => {
            const ok = await copyText(value)
            toast(ok ? { title: `${label} copié` } : { tone: 'error', title: 'Copie impossible', description: 'Sélectionne le texte et copie-le manuellement.' })
          }}
        >
          <Copy className="size-4" aria-hidden="true" />
          <span className="hidden sm:inline">Copier</span>
          <span className="sr-only sm:hidden">Copier {label.toLowerCase()}</span>
        </button>
      </div>
    </div>
  )
}

function CreatedDialog({ created, onClose }: { created: InvitationCreated | null; onClose: () => void }) {
  return (
    <Dialog
      open={created !== null}
      onClose={onClose}
      title="Invitation créée"
      description={created?.invitation.label || undefined}
      footer={
        <div className="flex justify-end">
          <button type="button" className="btn btn-primary" onClick={onClose}>
            J’ai copié le lien
          </button>
        </div>
      }
    >
      {created && (
        <div className="space-y-4">
          <p className="flex items-start gap-2 rounded-xl border border-warn/35 bg-warn/10 px-3 py-2.5 text-sm text-ink">
            <TriangleAlert className="mt-0.5 size-4 flex-none text-warn" aria-hidden="true" />
            Code et lien affichés une seule fois : copie-les maintenant, Stride ne conserve qu’une empreinte.
          </p>
          <CopyField label="Lien d’invitation" value={created.link} />
          <CopyField label="Code" value={created.code} mono />
          <p className="num text-xs text-ink-3">
            {created.invitation.maxUses} utilisation{created.invitation.maxUses > 1 ? 's' : ''} · expire {created.invitation.expiresAt ? `le ${fmtDateTime(created.invitation.expiresAt)}` : 'jamais'}
          </p>
        </div>
      )}
    </Dialog>
  )
}

function InvitationList() {
  const list = useAdminInvitations()
  const m = useAdminMutations()
  const { toast } = useToast()
  const [revoke, setRevoke] = useState<Invitation | null>(null)
  const items = [...(list.data ?? [])].sort((a, b) => b.createdAt.localeCompare(a.createdAt))

  const badge = (inv: Invitation) => {
    const st = INVITATION_STATE[invitationState(inv)]
    return <span className={cn('chip border', st.cls)}>{st.label}</span>
  }
  const revokeBtn = (inv: Invitation) =>
    invitationState(inv) === 'active' ? (
      <button type="button" className="btn btn-ghost min-h-9 px-3 text-xs" onClick={() => setRevoke(inv)}>
        <Ban className="size-3.5" aria-hidden="true" /> Révoquer
      </button>
    ) : null

  return (
    <Card title="Invitations" icon={<Mail className="size-4" />} subtitle={list.data ? `${items.filter((i) => invitationState(i) === 'active').length} active(s)` : undefined}>
      {list.isError ? (
        <ErrorView error={list.error} onRetry={() => void list.refetch()} />
      ) : !list.data ? (
        <Skeleton className="h-24" />
      ) : items.length === 0 ? (
        <EmptyState icon={<Mail />} title="Aucune invitation">
          Crée une invitation pour permettre à quelqu’un de rejoindre Stride.
        </EmptyState>
      ) : (
        <>
          <div className="scroll-thin hidden overflow-x-auto md:block">
            <table className="w-full min-w-[680px] border-separate border-spacing-0 text-sm">
              <caption className="sr-only">Invitations</caption>
              <thead>
                <tr className="text-left text-xs text-ink-3">
                  {['Libellé', 'Code', 'Utilisations', 'Créée', 'Expire', 'Statut', ''].map((h, i) => (
                    <th key={i} scope="col" className="border-b border-line px-2 py-2 font-medium first:pl-1">
                      {h || <span className="sr-only">Actions</span>}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {items.map((inv) => (
                  <tr key={inv.id} className="hover:bg-track/50">
                    <th scope="row" className="max-w-[14rem] truncate border-b border-line px-2 py-2 pl-1 text-left font-semibold text-ink">
                      {inv.label || <span className="font-normal text-ink-3">Sans libellé</span>}
                    </th>
                    <td className="border-b border-line px-2 py-2 font-mono text-ink-2">{inv.codePrefix}…</td>
                    <td className="num border-b border-line px-2 py-2 text-ink-2">
                      {inv.uses} / {inv.maxUses}
                    </td>
                    <td className="num border-b border-line px-2 py-2 text-ink-2">{fmtDay(inv.createdAt)}</td>
                    <td className="num border-b border-line px-2 py-2 text-ink-2">{inv.expiresAt ? fmtDateTime(inv.expiresAt) : 'jamais'}</td>
                    <td className="border-b border-line px-2 py-2">{badge(inv)}</td>
                    <td className="border-b border-line px-2 py-2 text-right">{revokeBtn(inv)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <ul className="space-y-2 md:hidden">
            {items.map((inv) => (
              <li key={inv.id} className="glass-inset p-3">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <p className="truncate font-semibold text-ink">{inv.label || 'Sans libellé'}</p>
                    <p className="num text-xs text-ink-3">
                      <span className="font-mono">{inv.codePrefix}…</span> · {inv.uses}/{inv.maxUses} · expire {inv.expiresAt ? fmtDateTime(inv.expiresAt) : 'jamais'}
                    </p>
                  </div>
                  {badge(inv)}
                </div>
                {revokeBtn(inv) && <div className="mt-2">{revokeBtn(inv)}</div>}
              </li>
            ))}
          </ul>
        </>
      )}
      <ConfirmDialog
        open={revoke !== null}
        onClose={() => setRevoke(null)}
        title="Révoquer cette invitation ?"
        message={<p>Le lien « {revoke?.label || revoke?.codePrefix} » ne permettra plus de créer de compte. Les comptes déjà créés ne sont pas affectés.</p>}
        confirmLabel="Révoquer"
        danger
        busy={m.revokeInvitation.isPending}
        onConfirm={() =>
          revoke &&
          m.revokeInvitation.mutate(revoke.id, {
            onSuccess: () => {
              setRevoke(null)
              toast({ title: 'Invitation révoquée' })
            },
            onError: (e) => toast({ tone: 'error', title: 'Révocation impossible', description: e.message }),
          })
        }
      />
    </Card>
  )
}
