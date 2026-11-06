import { Bell, Bot, CircleUser, DatabaseBackup, FileUp, Footprints, KeyRound, Monitor, Moon, Palette, Plug, Share2, Sun, UserRound, type LucideIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import type { Me } from '../../api/types'
import { consumeStravaResult } from '../../components/profile/StravaConnect'
import { PageHeader } from '../../components/ui/Misc'
import { SegmentedControl } from '../../components/ui/SegmentedControl'
import { useToast } from '../../components/ui/Toast'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { useTheme, type ThemePref } from '../../hooks/useTheme'
import { cn } from '../../lib/cn'
import { AccountSection } from './AccountSection'
import { BackupSection } from './BackupSection'
import { ImportSection } from './ImportSection'
import { IntegrationsSection } from './IntegrationsSection'
import { LlmSection } from './LlmSection'
import { NotificationsSection } from './NotificationsSection'
import { PasskeysSection } from './PasskeysSection'
import { ProfileSection } from './ProfileSection'
import { Section } from './Section'
import { ShareSection } from './ShareSection'
import { ShoesSection } from './ShoesSection'
import { VersionFooter } from './VersionFooter'

const SECTIONS: { id: string; label: string; icon: LucideIcon }[] = [
  { id: 'profil', label: 'Profil & seuils', icon: UserRound },
  { id: 'apparence', label: 'Apparence', icon: Palette },
  { id: 'integrations', label: 'Intégrations', icon: Plug },
  { id: 'ia', label: 'IA', icon: Bot },
  { id: 'notifications', label: 'Notifications', icon: Bell },
  { id: 'chaussures', label: 'Chaussures', icon: Footprints },
  { id: 'partage', label: 'Partage Kiné & Coach', icon: Share2 },
  { id: 'passkeys', label: 'Passkeys', icon: KeyRound },
  { id: 'sauvegarde', label: 'Sauvegarde', icon: DatabaseBackup },
  { id: 'import', label: 'Import de secours', icon: FileUp },
  { id: 'compte', label: 'Compte', icon: CircleUser },
]

function scrollToHash(smooth = false) {
  const id = decodeURIComponent(window.location.hash.slice(1))
  if (!id) return
  document.getElementById(id)?.scrollIntoView({ block: 'start', behavior: smooth ? 'smooth' : 'auto' })
}

function useActiveSection(): string {
  const [active, setActive] = useState(SECTIONS[0].id)
  useEffect(() => {
    const els = SECTIONS.map((s) => document.getElementById(s.id)).filter((e): e is HTMLElement => !!e)
    const io = new IntersectionObserver(
      (entries) => {
        const visible = entries.filter((e) => e.isIntersecting).sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
        if (visible[0]) setActive(visible[0].target.id)
      },
      { rootMargin: '-25% 0px -65% 0px' },
    )
    els.forEach((e) => io.observe(e))
    return () => io.disconnect()
  }, [])
  return active
}

export default function SettingsView({ me }: { me: Me }) {
  useDocumentTitle('Réglages')
  const { toast } = useToast()
  const { pref, setPref } = useTheme()
  const active = useActiveSection()

  useEffect(() => {
    const strava = consumeStravaResult()
    if (strava) {
      if (strava.status === 'connected') toast({ title: 'Strava connecté', description: 'Tes séances Strava seront fusionnées avec Garmin.' })
      else toast({ tone: 'error', title: 'Connexion Strava échouée', description: strava.reason ?? undefined })
      if (!window.location.hash) window.history.replaceState(window.history.state, '', `${window.location.pathname}${window.location.search}#integrations`)
    }
    scrollToHash()
    const t = window.setTimeout(scrollToHash, 450)
    const onNav = () => window.setTimeout(() => scrollToHash(true), 0)
    window.addEventListener('hashchange', onNav)
    window.addEventListener('pushState', onNav)
    window.addEventListener('replaceState', onNav)
    return () => {
      window.clearTimeout(t)
      window.removeEventListener('hashchange', onNav)
      window.removeEventListener('pushState', onNav)
      window.removeEventListener('replaceState', onNav)
    }
  }, [toast])

  return (
    <div>
      <PageHeader title="Réglages" subtitle="Profil, intégrations, IA, notifications, équipement, sécurité et données." />
      <nav aria-label="Sections des réglages" className="scroll-thin -mx-4 mb-4 overflow-x-auto px-4 pb-1 sm:-mx-6 sm:px-6 lg:hidden">
        <ul className="flex w-max gap-2">
          {SECTIONS.map((s) => (
            <li key={s.id}>
              <a href={`#${s.id}`} aria-current={active === s.id ? 'location' : undefined} className={cn('chip h-9 border px-3', active === s.id ? 'border-accent/50 bg-accent/15 text-ink' : 'border-line bg-track/60 text-ink-2')}>
                <s.icon className="size-3.5" aria-hidden="true" />
                {s.label}
              </a>
            </li>
          ))}
        </ul>
      </nav>
      <div className="grid gap-6 lg:grid-cols-[13.5rem_minmax(0,1fr)]">
        <nav aria-label="Sections des réglages" className="hidden self-start lg:sticky lg:top-24 lg:block">
          <ul className="glass space-y-0.5 p-2">
            {SECTIONS.map((s) => (
              <li key={s.id}>
                <a
                  href={`#${s.id}`}
                  aria-current={active === s.id ? 'location' : undefined}
                  className={cn('flex items-center gap-2.5 rounded-xl px-3 py-2 text-sm font-medium transition-colors', active === s.id ? 'bg-accent/15 text-ink' : 'text-ink-2 hover:bg-track hover:text-ink')}
                >
                  <s.icon className={cn('size-4 flex-none', active === s.id ? 'text-accent' : 'text-ink-3')} aria-hidden="true" />
                  {s.label}
                </a>
              </li>
            ))}
          </ul>
        </nav>
        <div className="min-w-0 space-y-5">
          <ProfileSection me={me} />
          <Section id="apparence" title="Apparence" icon={<Palette />} description="Thème de l’interface sur cet appareil.">
            <SegmentedControl<ThemePref>
              ariaLabel="Thème"
              className="max-w-md"
              value={pref}
              onChange={setPref}
              options={[
                { value: 'system', label: 'Système', icon: <Monitor /> },
                { value: 'light', label: 'Clair', icon: <Sun /> },
                { value: 'dark', label: 'Sombre', icon: <Moon /> },
              ]}
            />
          </Section>
          <IntegrationsSection />
          <LlmSection />
          <NotificationsSection />
          <ShoesSection />
          <ShareSection />
          <PasskeysSection />
          <BackupSection />
          <ImportSection />
          <AccountSection me={me} />
          <VersionFooter />
        </div>
      </div>
    </div>
  )
}
