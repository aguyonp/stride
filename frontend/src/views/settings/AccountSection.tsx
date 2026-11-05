import { CircleUser, LogOut, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useLocation } from 'wouter'
import { useDeleteAccount, useLogout } from '../../api/queries'
import type { Me } from '../../api/types'
import { fmtDay } from '../../components/profile/download'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { KeyValue } from '../../components/ui/Misc'
import { useToast } from '../../components/ui/Toast'
import { Section } from './Section'

export function AccountSection({ me }: { me: Me }) {
  const [, navigate] = useLocation()
  const logout = useLogout()
  const del = useDeleteAccount()
  const { toast } = useToast()
  const [confirm, setConfirm] = useState(false)
  return (
    <Section id="compte" title="Compte" icon={<CircleUser />}>
      <KeyValue
        className="max-w-md"
        items={[
          ['Nom', me.displayName],
          ['Rôle', me.role === 'admin' ? 'Administrateur' : 'Athlète'],
          ['Membre depuis', fmtDay(me.createdAt)],
        ]}
      />
      <div className="mt-5 flex flex-wrap gap-2 border-t border-line pt-4">
        <button type="button" className="btn btn-ghost" disabled={logout.isPending} onClick={() => logout.mutate(undefined, { onSettled: () => navigate('/login', { replace: true }) })}>
          <LogOut className="size-4" aria-hidden="true" /> Se déconnecter
        </button>
        <button type="button" className="btn btn-danger" onClick={() => setConfirm(true)}>
          <Trash2 className="size-4" aria-hidden="true" /> Supprimer mon compte
        </button>
      </div>
      <ConfirmDialog
        open={confirm}
        onClose={() => setConfirm(false)}
        title="Supprimer définitivement ton compte ?"
        message={
          <>
            <p>Toutes tes données seront effacées : activités, ressentis, plan, objectifs, chaussures, passkeys et connexions Garmin / Strava.</p>
            <p>Pense à exporter une sauvegarde avant. Cette action est irréversible.</p>
          </>
        }
        confirmLabel="Supprimer mon compte"
        danger
        requireText="SUPPRIMER"
        busy={del.isPending}
        onConfirm={() =>
          del.mutate(undefined, {
            onSuccess: () => navigate('/login', { replace: true }),
            onError: (e) => toast({ tone: 'error', title: 'Suppression impossible', description: e.message }),
          })
        }
      />
    </Section>
  )
}
