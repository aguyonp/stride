import { CalendarRange, HeartPulse, LayoutDashboard, Settings, Sparkles, type LucideIcon } from 'lucide-react'
import { t, type MessageKey } from '../../i18n'

export interface NavItem {
  href: string
  label: string
  short: string
  icon: LucideIcon
  dock: boolean
}

/** The five menus (§14.1). Réglages stays out of the mobile dock: it is in the account menu there. */
const item = (href: string, key: 'dashboard' | 'plan' | 'health' | 'coach' | 'settings', icon: LucideIcon, dock: boolean): NavItem => ({
  href,
  icon,
  dock,
  get label() {
    return t(`layout.nav.${key}` as MessageKey)
  },
  get short() {
    return t(`layout.nav.${key}Short` as MessageKey)
  },
})

export const NAV: NavItem[] = [
  item('/', 'dashboard', LayoutDashboard, true),
  item('/plan', 'plan', CalendarRange, true),
  item('/health', 'health', HeartPulse, true),
  item('/coach', 'coach', Sparkles, true),
  item('/settings', 'settings', Settings, false),
]

export const isActive = (href: string, path: string) =>
  href === '/' ? path === '/' || path.startsWith('/activities/') : path === href || path.startsWith(`${href}/`)
