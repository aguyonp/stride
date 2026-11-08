import { Bell, Bot, CircleUser, DatabaseBackup, FileUp, Footprints, KeyRound, Monitor, Moon, Palette, Plug, Sun, UserRound, type LucideIcon } from 'lucide-react'
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
import { ShoesSection } from './ShoesSection'
import { VersionFooter } from './VersionFooter'
import { t } from '../../i18n'
import { useLanguageSwitch, type LanguagePref } from '../../i18n/useLanguage'

type SectionId = 'profil' | 'apparence' | 'integrations' | 'ia' | 'notifications' | 'chaussures' | 'passkeys' | 'sauvegarde' | 'import' | 'compte'
const SECTIONS: { id: SectionId; icon: LucideIcon; readonly label: string }[] = (
  [
    ['profil', UserRound],
    ['apparence', Palette],
    ['integrations', Plug],
    ['ia', Bot],
    ['notifications', Bell],
    ['chaussures', Footprints],
    ['passkeys', KeyRound],
    ['sauvegarde', DatabaseBackup],
    ['import', FileUp],
    ['compte', CircleUser],
  ] as const
).map(([id, icon]) => ({
  id,
  icon,
  get label() {
    return t(`settings.view.sections.${id}`)
  },
}))

function LanguageSetting() {
  const { pref, choose } = useLanguageSwitch()
  return (
    <div className="mt-4">
      <p className="mb-1.5 text-xs font-medium text-ink-3">{t('settings.view.language')}</p>
      <SegmentedControl<LanguagePref>
        ariaLabel={t('settings.view.language')}
        className="max-w-md"
        value={pref}
        onChange={choose}
        options={[
          { value: 'auto', label: t('common.lang.auto'), icon: <Monitor /> },
          { value: 'fr', label: t('common.lang.fr') },
          { value: 'en', label: t('common.lang.en') },
        ]}
      />
      <p className="mt-1.5 text-xs text-ink-3">{t('settings.view.languageHint')}</p>
    </div>
  )
}

function scrollToHash(smooth = false) {
  const id = decodeURIComponent(window.location.hash.slice(1))
  if (!id) return
  document.getElementById(id)?.scrollIntoView({ block: 'start', behavior: smooth ? 'smooth' : 'auto' })
}

function useActiveSection(): string {
  const [active, setActive] = useState<string>(SECTIONS[0].id)
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
  useDocumentTitle(t('settings.view.title'))
  const { toast } = useToast()
  const { pref, setPref } = useTheme()
  const active = useActiveSection()

  useEffect(() => {
    const strava = consumeStravaResult()
    if (strava) {
      if (strava.status === 'connected') toast({ title: t('settings.view.stravaConnected'), description: t('settings.view.stravaConnectedHint') })
      else toast({ tone: 'error', title: t('settings.view.stravaFailed'), description: strava.reason ?? undefined })
      if (!window.location.hash) window.history.replaceState(window.history.state, '', `${window.location.pathname}${window.location.search}#integrations`)
    }
    scrollToHash()
    const timer = window.setTimeout(scrollToHash, 450)
    const onNav = () => window.setTimeout(() => scrollToHash(true), 0)
    window.addEventListener('hashchange', onNav)
    window.addEventListener('pushState', onNav)
    window.addEventListener('replaceState', onNav)
    return () => {
      window.clearTimeout(timer)
      window.removeEventListener('hashchange', onNav)
      window.removeEventListener('pushState', onNav)
      window.removeEventListener('replaceState', onNav)
    }
  }, [toast])

  return (
    <div>
      <PageHeader title={t('settings.view.title')} subtitle={t('settings.view.subtitle')} />
      <nav aria-label={t('settings.view.sectionsNav')} className="scroll-thin -mx-4 mb-4 overflow-x-auto px-4 pb-1 sm:-mx-6 sm:px-6 lg:hidden">
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
        <nav aria-label={t('settings.view.sectionsNav')} className="hidden self-start lg:sticky lg:top-24 lg:block">
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
          <Section id="apparence" title={t('settings.view.sections.apparence')} icon={<Palette />} description={t('settings.view.appearanceHint')}>
            <SegmentedControl<ThemePref>
              ariaLabel={t('settings.view.theme')}
              className="max-w-md"
              value={pref}
              onChange={setPref}
              options={[
                { value: 'system', label: t('settings.view.themeSystem'), icon: <Monitor /> },
                { value: 'light', label: t('settings.view.themeLight'), icon: <Sun /> },
                { value: 'dark', label: t('settings.view.themeDark'), icon: <Moon /> },
              ]}
            />
            <LanguageSetting />
          </Section>
          <IntegrationsSection />
          <LlmSection />
          <NotificationsSection />
          <ShoesSection />
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
