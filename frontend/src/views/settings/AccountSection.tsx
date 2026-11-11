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
import { t } from '../../i18n'

export function AccountSection({ me }: { me: Me }) {
  const [, navigate] = useLocation()
  const logout = useLogout()
  const del = useDeleteAccount()
  const { toast } = useToast()
  const [confirm, setConfirm] = useState(false)
  return (
    <Section id="compte" title={t('settings.account.title')} icon={<CircleUser />}>
      <KeyValue
        className="max-w-md"
        items={[
          [t('settings.account.name'), me.displayName],
          [t('settings.account.role'), me.role === 'admin' ? t('settings.account.admin') : t('settings.account.athlete')],
          [t('settings.account.since'), fmtDay(me.createdAt)],
        ]}
      />
      <div className="mt-5 flex flex-wrap gap-2 border-t border-line pt-4">
        <button type="button" className="btn btn-ghost" disabled={logout.isPending} onClick={() => logout.mutate(undefined, { onSettled: () => navigate('/login', { replace: true }) })}>
          <LogOut className="size-4" aria-hidden="true" /> {t('settings.account.logout')}
        </button>
        <button type="button" className="btn btn-danger" onClick={() => setConfirm(true)}>
          <Trash2 className="size-4" aria-hidden="true" /> {t('settings.account.delete')}
        </button>
      </div>
      <ConfirmDialog
        open={confirm}
        onClose={() => setConfirm(false)}
        title={t('settings.account.deleteTitle')}
        message={
          <>
            <p>{t('settings.account.deleteBody1')}</p>
            <p>{t('settings.account.deleteBody2')}</p>
          </>
        }
        confirmLabel={t('settings.account.delete')}
        danger
        requireText={t('settings.account.deleteWord')}
        busy={del.isPending}
        onConfirm={() =>
          del.mutate(undefined, {
            onSuccess: () => navigate('/login', { replace: true }),
            onError: (e) => toast({ tone: 'error', title: t('settings.account.deleteFailed'), description: e.message }),
          })
        }
      />
    </Section>
  )
}
