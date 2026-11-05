import { CalendarRange, LayoutDashboard, Settings, ShieldCheck, Sparkles, type LucideIcon } from 'lucide-react'

export interface NavItem {
  href: string
  label: string
  short: string
  icon: LucideIcon
  admin?: boolean
}

export const NAV: NavItem[] = [
  { href: '/', label: 'Tableau de bord', short: 'Accueil', icon: LayoutDashboard },
  { href: '/plan', label: 'Planning', short: 'Planning', icon: CalendarRange },
  { href: '/export', label: 'Export IA', short: 'Export', icon: Sparkles },
  { href: '/settings', label: 'Réglages', short: 'Réglages', icon: Settings },
  { href: '/admin', label: 'Admin', short: 'Admin', icon: ShieldCheck, admin: true },
]

export const isActive = (href: string, path: string) => (href === '/' ? path === '/' : path === href || path.startsWith(`${href}/`))
