import { Footprints, KeyRound, LogOut, MessageSquarePlus, Search, Settings, Share2, ShieldCheck } from 'lucide-react'
import { useLocation } from 'wouter'
import { useLogout } from '../../api/queries'
import type { Me } from '../../api/types'
import { initials } from '../../lib/format'
import { ui } from '../../state/ui'
import { MenuItem, Popover } from '../ui/Popover'
import { t } from '../../i18n'

export function UserMenu({ me }: { me: Me }) {
  const [, navigate] = useLocation()
  const logout = useLogout()
  return (
    <Popover
      trigger={(p) => (
        <button {...p} type="button" className="icon-btn bg-accent/15 text-[0.8rem] font-semibold text-accent" aria-label={t('layout.userMenu.account', { name: me.displayName })}>
          {initials(me.displayName)}
        </button>
      )}
    >
      {(close) => (
        <>
          <div className="border-b border-line px-3 pt-2 pb-2.5">
            <p className="truncate text-sm font-semibold text-ink">{me.displayName}</p>
            <p className="text-xs text-ink-3">{me.role === 'admin' ? t('layout.userMenu.admin') : t('layout.userMenu.athlete')}</p>
          </div>
          <div className="pt-1.5">
            <MenuItem icon={<Search />} onSelect={() => (close(), ui.openCommand())}>
              {t('layout.userMenu.search')}
            </MenuItem>
            <MenuItem icon={<Footprints />} onSelect={() => (close(), navigate('/shoes'))}>
              {t('layout.userMenu.shoes')}
            </MenuItem>
            <MenuItem icon={<Share2 />} onSelect={() => (close(), navigate('/health#partage'))}>
              {t('layout.userMenu.share')}
            </MenuItem>
            <MenuItem icon={<Settings />} onSelect={() => (close(), navigate('/settings'))}>
              {t('layout.userMenu.settings')}
            </MenuItem>
            <MenuItem icon={<MessageSquarePlus />} onSelect={() => (close(), ui.openFeedback())}>
              {t('layout.userMenu.feedback')}
            </MenuItem>
            <MenuItem icon={<KeyRound />} onSelect={() => (close(), navigate('/settings#passkeys'))}>
              {t('layout.userMenu.passkeys')}
            </MenuItem>
            {me.role === 'admin' && (
              <MenuItem icon={<ShieldCheck />} onSelect={() => (close(), navigate('/admin'))}>
                {t('layout.userMenu.administration')}
              </MenuItem>
            )}
            <MenuItem
              icon={<LogOut />}
              danger
              onSelect={() => {
                close()
                logout.mutate(undefined, { onSettled: () => navigate('/login', { replace: true }) })
              }}
            >
              {t('layout.userMenu.logout')}
            </MenuItem>
          </div>
        </>
      )}
    </Popover>
  )
}
