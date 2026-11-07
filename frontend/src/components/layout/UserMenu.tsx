import { Footprints, KeyRound, LogOut, MessageSquarePlus, Search, Settings, Share2, ShieldCheck } from 'lucide-react'
import { useLocation } from 'wouter'
import { useLogout } from '../../api/queries'
import type { Me } from '../../api/types'
import { initials } from '../../lib/format'
import { ui } from '../../state/ui'
import { MenuItem, Popover } from '../ui/Popover'

export function UserMenu({ me }: { me: Me }) {
  const [, navigate] = useLocation()
  const logout = useLogout()
  return (
    <Popover
      trigger={(p) => (
        <button {...p} type="button" className="icon-btn bg-accent/15 text-[0.8rem] font-semibold text-accent" aria-label={`Compte : ${me.displayName}`}>
          {initials(me.displayName)}
        </button>
      )}
    >
      {(close) => (
        <>
          <div className="border-b border-line px-3 pt-2 pb-2.5">
            <p className="truncate text-sm font-semibold text-ink">{me.displayName}</p>
            <p className="text-xs text-ink-3">{me.role === 'admin' ? 'Administrateur' : 'Athlète'}</p>
          </div>
          <div className="pt-1.5">
            <MenuItem icon={<Search />} onSelect={() => (close(), ui.openCommand())}>
              Rechercher
            </MenuItem>
            <MenuItem icon={<Footprints />} onSelect={() => (close(), navigate('/shoes'))}>
              Chaussures
            </MenuItem>
            <MenuItem icon={<Share2 />} onSelect={() => (close(), navigate('/health#partage'))}>
              Partage Kiné & Coach
            </MenuItem>
            <MenuItem icon={<Settings />} onSelect={() => (close(), navigate('/settings'))}>
              Réglages
            </MenuItem>
            <MenuItem icon={<MessageSquarePlus />} onSelect={() => (close(), ui.openFeedback())}>
              Donner un avis
            </MenuItem>
            <MenuItem icon={<KeyRound />} onSelect={() => (close(), navigate('/settings#passkeys'))}>
              Mes passkeys
            </MenuItem>
            {me.role === 'admin' && (
              <MenuItem icon={<ShieldCheck />} onSelect={() => (close(), navigate('/admin'))}>
                Administration
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
              Se déconnecter
            </MenuItem>
          </div>
        </>
      )}
    </Popover>
  )
}
