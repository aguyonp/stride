import { CalendarRange, LayoutDashboard, Settings, ShieldCheck, Sparkles, Stethoscope, type LucideIcon } from 'lucide-react'

export interface NavItem {
  href: string
  label: string
  short: string
  icon: LucideIcon
  admin?: boolean
  mobile?: boolean
}

export const NAV: NavItem[] = [
  { href: '/', label: 'Tableau de bord', short: 'Accueil', icon: LayoutDashboard, mobile: true },
  { href: '/plan', label: 'Planning', short: 'Planning', icon: CalendarRange, mobile: true },
  { href: '/body', label: 'Cartographie', short: 'Corps', icon: Stethoscope, mobile: true },
  { href: '/export', label: 'Export IA', short: 'Export', icon: Sparkles, mobile: false },
  { href: '/settings', label: 'Réglages', short: 'Réglages', icon: Settings, mobile: true },
  { href: '/admin', label: 'Admin', short: 'Admin', icon: ShieldCheck, admin: true, mobile: false },
]

export const isActive = (href: string, path: string) =>
  href === '/' ? path === '/' || path.startsWith('/activities/') : path === href || path.startsWith(`${href}/`)
