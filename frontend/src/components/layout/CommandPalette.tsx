import {
  Activity,
  Bot,
  CalendarRange,
  Compass,
  Copy,
  CornerDownLeft,
  Footprints,
  History,
  LayoutDashboard,
  MessageSquarePlus,
  Plus,
  RefreshCw,
  Search,
  Settings,
  Share2,
  ShieldCheck,
  Sparkles,
  Stethoscope,
  X,
  type LucideIcon,
} from 'lucide-react'
import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react'
import { useLocation } from 'wouter'
import { api } from '../../api/client'
import { useActivities, useProfile, useStartSync } from '../../api/queries'
import { useActivitySearch } from '../../api/queriesV41'
import type { Me } from '../../api/types'
import { copyText } from '../../lib/clipboard'
import { cn } from '../../lib/cn'
import { fmtDateLong, fmtDateShort, fmtKm, todayISO } from '../../lib/format'
import { ALL_INCLUDE, effectiveInclude } from '../../lib/include'
import { SPORT_LABEL } from '../../lib/labels'
import { ui, useUi } from '../../state/ui'
import { useToast } from '../ui/Toast'

type CommandGroup = 'Actions rapides' | 'Navigation' | 'Séances'

interface CommandItem {
  id: string
  group: CommandGroup
  label: string
  subtitle?: string
  keywords: string
  icon: LucideIcon
  meta?: string
  onSelect: () => void | Promise<void>
}

const normalize = (s: string) =>
  s
    .toLowerCase()
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .trim()

/**
 * Universal keyboard-first command palette (Cmd+K / Ctrl+K) providing quick actions,
 * route navigation, and instant session search across Stride.
 */
export function CommandPalette({ me }: { me: Me }) {
  const open = useUi((s) => s.commandOpen)
  const [, navigate] = useLocation()
  const { toast } = useToast()
  const sync = useStartSync()
  const profile = useProfile()
  const today = todayISO(profile.data?.timezone)
  const activities = useActivities({}, open)

  const [query, setQuery] = useState('')
  const [debounced, setDebounced] = useState('')
  useEffect(() => {
    const t = setTimeout(() => setDebounced(query), 200)
    return () => clearTimeout(t)
  }, [query])
  const remote = useActivitySearch(open ? debounced : '')
  const [activeIndex, setActiveIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const listId = useId()
  const titleId = useId()

  useEffect(() => {
    const onGlobalKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        ui.toggleCommand()
      } else if (e.key === 'Escape' && open) {
        e.preventDefault()
        ui.closeCommand()
      }
    }
    window.addEventListener('keydown', onGlobalKeyDown)
    return () => window.removeEventListener('keydown', onGlobalKeyDown)
  }, [open])

  useEffect(() => {
    if (!open) {
      setQuery('')
      setActiveIndex(0)
      return
    }
    const prevOverflow = document.documentElement.style.overflow
    document.documentElement.style.overflow = 'hidden'
    const frame = requestAnimationFrame(() => inputRef.current?.focus())
    return () => {
      cancelAnimationFrame(frame)
      document.documentElement.style.overflow = prevOverflow
    }
  }, [open])

  const allActivities = useMemo(
    () => activities.data?.pages.flatMap((p) => p.items) ?? [],
    [activities.data],
  )

  const staticItems = useMemo<CommandItem[]>(() => {
    const actions: CommandItem[] = [
      {
        id: 'act-sync',
        group: 'Actions rapides',
        label: 'Lancer une synchronisation',
        subtitle: 'Récupérer les dernières séances Garmin et Strava en tâche de fond',
        keywords: 'sync synchro garmin strava actualiser rafraichir',
        icon: RefreshCw,
        onSelect: () => {
          sync.mutate(undefined, {
            onSuccess: () => toast({ title: 'Synchronisation lancée' }),
            onError: (err) => toast({ tone: 'error', title: 'Synchronisation impossible', description: err.message }),
          })
        },
      },
      {
        id: 'act-manual',
        group: 'Actions rapides',
        label: 'Créer une séance manuelle',
        subtitle: 'Enregistrer une sortie sans montre connectée',
        keywords: 'creer ajouter seance activite manuelle course velo',
        icon: Plus,
        onSelect: () => ui.openManualActivity(),
      },
      {
        id: 'act-bodymap',
        group: 'Actions rapides',
        label: 'Enregistrer une gêne corporelle',
        subtitle: 'Noter une douleur ou un ressenti à froid J+1 sur la cartographie',
        keywords: 'douleur gene blessure corps genou achille periostite tfl aponevrose',
        icon: Stethoscope,
        onSelect: () => navigate('/health?new=1'),
      },
      {
        id: 'act-feedback',
        group: 'Actions rapides',
        label: 'Donner un avis',
        subtitle: 'Signaler un bug ou proposer une idée à l’administrateur',
        keywords: 'avis bug idee suggestion retour feedback probleme contact',
        icon: MessageSquarePlus,
        onSelect: () => ui.openFeedback(),
      },
      {
        id: 'act-share',
        group: 'Actions rapides',
        label: 'Créer un lien de partage Kiné / Coach',
        subtitle: 'Générer un accès en lecture seule au dossier athlète',
        keywords: 'partage lien kine coach medecin lecture seule',
        icon: Share2,
        onSelect: () => navigate('/health#partage'),
      },
      {
        id: 'act-history',
        group: 'Actions rapides',
        label: 'Importer tout l’historique',
        subtitle: 'Rapatrier les anciennes séances Garmin et Strava',
        keywords: 'importer historique ancien annees backfill garmin strava',
        icon: History,
        onSelect: () => navigate('/settings#integrations'),
      },
      {
        id: 'act-prompt',
        group: 'Actions rapides',
        label: 'Copier le prompt Coach IA du jour',
        subtitle: 'Générer et copier le rapport complet du jour au format Markdown',
        keywords: 'copier prompt coach ia bilan jour markdown rapport',
        icon: Copy,
        onSelect: async () => {
          try {
            const md = await api.dayReport(today, effectiveInclude(ALL_INCLUDE, 'day'))
            const ok = await copyText(md)
            if (ok) {
              void api.trackAction('prompt.copy').catch(() => undefined)
              toast({ tone: 'success', title: 'Prompt Coach IA du jour copié' })
            } else {
              toast({ tone: 'error', title: 'Impossible de copier dans le presse-papiers' })
            }
          } catch (err) {
            toast({
              tone: 'error',
              title: 'Génération du prompt impossible',
              description: err instanceof Error ? err.message : undefined,
            })
          }
        },
      },
      {
        id: 'act-ai-day',
        group: 'Actions rapides',
        label: 'Lancer l’analyse IA du jour',
        subtitle: 'Ouvrir le coach IA en streaming sur le bilan du jour',
        keywords: 'analyse ia coach streaming bilan jour',
        icon: Bot,
        onSelect: () => ui.triggerDayAnalysis(),
      },
    ]

    const navItems: CommandItem[] = [
      {
        id: 'nav-dashboard',
        group: 'Navigation',
        label: 'Tableau de bord',
        subtitle: 'Séances, readiness, biomécanique et bilan quotidien',
        keywords: 'accueil dashboard tableau de bord seance aujourdhui',
        icon: LayoutDashboard,
        meta: '/',
        onSelect: () => navigate('/'),
      },
      {
        id: 'nav-plan',
        group: 'Navigation',
        label: 'Planning',
        subtitle: 'Séances, cycles, calendrier et imprévus',
        keywords: 'plan planning cycles gantt macrocycle mesocycle semaine mois',
        icon: CalendarRange,
        meta: '/plan',
        onSelect: () => navigate('/plan'),
      },
      {
        id: 'nav-health',
        group: 'Navigation',
        label: 'Santé & forme',
        subtitle: 'État de santé, symptômes, corrélations et partage kiné',
        keywords: 'sante forme corps cartographie anatomie douleur blessure kine correlation partage',
        icon: Stethoscope,
        meta: '/health',
        onSelect: () => navigate('/health'),
      },
      {
        id: 'nav-shoes',
        group: 'Navigation',
        label: 'Chaussures',
        subtitle: 'Rotation active, drop et usure kilométrique',
        keywords: 'chaussures paires drop km equipement materiel',
        icon: Footprints,
        meta: '/shoes',
        onSelect: () => navigate('/shoes'),
      },
      {
        id: 'nav-coach',
        group: 'Navigation',
        label: 'Coach IA',
        subtitle: 'Prompt, analyse IA, historique et export multi-séances',
        keywords: 'coach ia export prompt analyse markdown multi seances rapport',
        icon: Sparkles,
        meta: '/coach',
        onSelect: () => navigate('/coach'),
      },
      {
        id: 'nav-settings',
        group: 'Navigation',
        label: 'Réglages',
        subtitle: 'Profil, seuils, intégrations, IA, notifications et sécurité',
        keywords: 'reglages parametres profil garmin strava passkeys sauvegarde',
        icon: Settings,
        meta: '/settings',
        onSelect: () => navigate('/settings'),
      },
    ]

    if (me.role === 'admin') {
      navItems.push({
        id: 'nav-admin',
        group: 'Navigation',
        label: 'Administration',
        subtitle: 'Comptes, invitations, quotas fournisseurs et journaux',
        keywords: 'admin administration utilisateurs invitations quotas logs',
        icon: ShieldCheck,
        meta: '/admin',
        onSelect: () => navigate('/admin'),
      })
    }

    return [...actions, ...navItems]
  }, [me.role, navigate, sync, toast, today])

  const filteredItems = useMemo<CommandItem[]>(() => {
    const q = normalize(query)
    const tokens = q ? q.split(/\s+/).filter(Boolean) : []

    const matches = (haystack: string) => {
      if (tokens.length === 0) return true
      const normHay = normalize(haystack)
      return tokens.every((t) => normHay.includes(t))
    }

    const matchedStatic = staticItems.filter((item) =>
      matches(`${item.label} ${item.subtitle ?? ''} ${item.keywords} ${item.meta ?? ''}`),
    )

    const sessionItems: CommandItem[] = []
    const maxSessions = tokens.length === 0 ? 5 : 12
    for (const a of allActivities) {
      const sportFr = SPORT_LABEL[a.sport] ?? a.sport
      const dateShort = fmtDateShort(a.date)
      const dateLong = fmtDateLong(a.date)
      const kmTxt = fmtKm(a.distanceM)
      const searchable = `${a.name} ${a.date} ${dateShort} ${dateLong} ${sportFr} ${a.typeLabel} ${kmTxt}`
      if (!matches(searchable)) continue
      sessionItems.push({
        id: `session-${a.id}`,
        group: 'Séances',
        label: a.name,
        subtitle: `${sportFr} · ${a.typeLabel}`,
        keywords: searchable,
        icon: Activity,
        meta: `${dateShort} · ${kmTxt}`,
        onSelect: () => navigate(`/activities/${a.id}`),
      })
      if (sessionItems.length >= maxSessions) break
    }

    // Whole-history matches from GET /api/search (older months and years), after the loaded ones.
    if (tokens.length > 0) {
      for (const h of remote.data?.items ?? []) {
        if (sessionItems.length >= maxSessions) break
        if (sessionItems.some((x) => x.id === `session-${h.id}`)) continue
        const sportFr = SPORT_LABEL[h.sport] ?? h.sport
        sessionItems.push({
          id: `session-${h.id}`,
          group: 'Séances',
          label: h.name,
          subtitle: `${sportFr} · ${h.typeLabel}`,
          keywords: h.name,
          icon: Activity,
          meta: `${fmtDateShort(h.date)} · ${fmtKm(h.distanceM)}`,
          onSelect: () => navigate(`/activities/${h.id}`),
        })
      }
    }

    return [...matchedStatic, ...sessionItems]
  }, [allActivities, navigate, query, remote.data, staticItems])

  const safeIndex = filteredItems.length === 0 ? -1 : Math.min(activeIndex, filteredItems.length - 1)

  useEffect(() => {
    if (safeIndex < 0 || !listRef.current) return
    const el = listRef.current.querySelector<HTMLElement>(`[data-cmd-idx="${safeIndex}"]`)
    el?.scrollIntoView({ block: 'nearest' })
  }, [safeIndex])

  const execute = (item: CommandItem | undefined) => {
    if (!item) return
    ui.closeCommand()
    void item.onSelect()
  }

  const onInputKeyDown = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (filteredItems.length > 0) {
        setActiveIndex((i) => (i + 1) % filteredItems.length)
      }
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (filteredItems.length > 0) {
        setActiveIndex((i) => (i - 1 + filteredItems.length) % filteredItems.length)
      }
    } else if (e.key === 'Home') {
      e.preventDefault()
      setActiveIndex(0)
    } else if (e.key === 'End') {
      e.preventDefault()
      if (filteredItems.length > 0) setActiveIndex(filteredItems.length - 1)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (safeIndex >= 0) execute(filteredItems[safeIndex])
    } else if (e.key === 'Escape') {
      e.preventDefault()
      ui.closeCommand()
    }
  }

  if (!open) return null

  const groups: CommandGroup[] = ['Actions rapides', 'Navigation', 'Séances']
  const activeOptionId = safeIndex >= 0 ? `${listId}-opt-${filteredItems[safeIndex].id}` : undefined

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/55 px-3 pt-[10dvh] pb-6 backdrop-blur-xs sm:px-6 sm:pt-[13dvh]"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) ui.closeCommand()
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className="sheet-panel flex max-h-[76dvh] w-full max-w-xl flex-col overflow-hidden rounded-[22px]"
      >
        <h2 id={titleId} className="sr-only">
          Palette de commandes Stride
        </h2>

        <div className="flex flex-none items-center gap-2.5 border-b border-line px-4 py-3">
          <Search className="size-4 flex-none text-ink-3" aria-hidden="true" />
          <input
            ref={inputRef}
            type="text"
            role="combobox"
            aria-expanded="true"
            aria-controls={listId}
            aria-activedescendant={activeOptionId}
            aria-autocomplete="list"
            aria-label="Rechercher une commande, une page ou une séance"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              setActiveIndex(0)
            }}
            onKeyDown={onInputKeyDown}
            placeholder="Rechercher une séance, une vue ou une action…"
            className="min-w-0 flex-1 bg-transparent text-sm text-ink placeholder:text-ink-3 focus:outline-none"
          />
          {query && (
            <button
              type="button"
              className="icon-btn size-7 text-ink-3"
              onClick={() => {
                setQuery('')
                setActiveIndex(0)
                inputRef.current?.focus()
              }}
              aria-label="Effacer la recherche"
            >
              <X className="size-3.5" />
            </button>
          )}
          <button
            type="button"
            onClick={ui.closeCommand}
            className="rounded-lg border border-line bg-track/60 px-2 py-1 font-mono text-[11px] text-ink-3 hover:text-ink"
            aria-label="Fermer la palette (Échap)"
          >
            Échap
          </button>
        </div>

        <div
          id={listId}
          ref={listRef}
          role="listbox"
          aria-label="Résultats de la palette de commandes"
          className="scroll-thin min-h-0 flex-1 overflow-y-auto overscroll-contain p-2"
        >
          {filteredItems.length === 0 ? (
            <div className="py-10 text-center">
              <Compass className="mx-auto mb-2 size-6 text-ink-3" aria-hidden="true" />
              <p className="text-sm font-medium text-ink-2">Aucun résultat pour « {query} »</p>
              <p className="mt-0.5 text-xs text-ink-3">
                Essaie un titre de sortie, une date (ex. 2026-11), un sport ou le nom d’une page.
              </p>
            </div>
          ) : (
            groups.map((group) => {
              const entries = filteredItems
                .map((item, idx) => ({ item, idx }))
                .filter((x) => x.item.group === group)
              if (entries.length === 0) return null
              return (
                <div key={group} className="mb-2 last:mb-0">
                  <p className="px-3 pt-1.5 pb-1 text-[11px] font-semibold text-ink-3">{group}</p>
                  <ul className="space-y-0.5">
                    {entries.map(({ item, idx }) => {
                      const active = idx === safeIndex
                      const Icon = item.icon
                      return (
                        <li key={item.id}>
                          <button
                            id={`${listId}-opt-${item.id}`}
                            type="button"
                            role="option"
                            aria-selected={active}
                            data-cmd-idx={idx}
                            onMouseEnter={() => setActiveIndex(idx)}
                            onClick={() => execute(item)}
                            className={cn(
                              'flex w-full items-center justify-between gap-3 rounded-xl px-3 py-2.5 text-left transition-colors',
                              active ? 'bg-accent/15 text-ink' : 'text-ink-2 hover:bg-track/60 hover:text-ink',
                            )}
                          >
                            <div className="flex min-w-0 items-center gap-3">
                              <span
                                className={cn(
                                  'grid size-8 flex-none place-items-center rounded-lg border',
                                  active
                                    ? 'border-accent/40 bg-accent/15 text-accent'
                                    : 'border-line bg-track/50 text-ink-3',
                                )}
                                aria-hidden="true"
                              >
                                <Icon className="size-4" />
                              </span>
                              <div className="min-w-0">
                                <p className="truncate text-sm font-medium text-ink">{item.label}</p>
                                {item.subtitle && (
                                  <p className="truncate text-xs text-ink-3">{item.subtitle}</p>
                                )}
                              </div>
                            </div>
                            <div className="flex flex-none items-center gap-2">
                              {item.meta && (
                                <span className="num text-xs text-ink-3">{item.meta}</span>
                              )}
                              {active && (
                                <CornerDownLeft className="size-3.5 text-accent" aria-hidden="true" />
                              )}
                            </div>
                          </button>
                        </li>
                      )
                    })}
                  </ul>
                </div>
              )
            })
          )}
        </div>

        <footer className="flex flex-none items-center justify-between border-t border-line bg-track/30 px-4 py-2 text-[11px] text-ink-3">
          <span>↑↓ naviguer · Entrée valider · Échap fermer</span>
          <span className="font-mono">⌘K / Ctrl+K</span>
        </footer>
      </div>
    </div>
  )
}
