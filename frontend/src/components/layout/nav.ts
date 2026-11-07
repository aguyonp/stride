import { CalendarRange, HeartPulse, LayoutDashboard, Settings, Sparkles, type LucideIcon } from 'lucide-react'

export interface NavItem {
  href: string
  label: string
  short: string
  icon: LucideIcon
  dock: boolean
}

/** The five menus (§14.1). Réglages stays out of the mobile dock: it is in the account menu there. */
export const NAV: NavItem[] = [
  { href: '/', label: 'Tableau de bord', short: 'Accueil', icon: LayoutDashboard, dock: true },
  { href: '/plan', label: 'Planning', short: 'Planning', icon: CalendarRange, dock: true },
  { href: '/health', label: 'Santé & forme', short: 'Santé', icon: HeartPulse, dock: true },
  { href: '/coach', label: 'Coach IA', short: 'Coach', icon: Sparkles, dock: true },
  { href: '/settings', label: 'Réglages', short: 'Réglages', icon: Settings, dock: false },
]

export const isActive = (href: string, path: string) =>
  href === '/' ? path === '/' || path.startsWith('/activities/') : path === href || path.startsWith(`${href}/`)
