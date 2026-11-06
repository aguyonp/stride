import { Check, Copy, Link2Off, Loader2, Plus, Share2 } from 'lucide-react'
import { useId, useState } from 'react'
import { useShareLinks, useShareMutations } from '../../api/queries'
import type { ShareLink } from '../../api/types'
import { EmptyState, InlineError } from '../../components/ui/Misc'
import { SegmentedControl } from '../../components/ui/SegmentedControl'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { copyText } from '../../lib/clipboard'
import { cn } from '../../lib/cn'
import { fmtDateTime, fmtNumber } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { Section } from './Section'

type ExpiryChoice = '7' | '30' | '90' | 'none'

const EXPIRY_OPTIONS: { value: ExpiryChoice; label: string }[] = [
  { value: '7', label: '7 jours' },
  { value: '30', label: '30 jours' },
  { value: '90', label: '90 jours' },
  { value: 'none', label: 'Aucune expiration' },
]

const PRESET_LABELS = ['Kiné du sport', 'Coach club', 'Médecin du sport']

function fullShareUrl(urlOrPath: string): string {
  if (typeof window === 'undefined') return urlOrPath
  try {
    return new URL(urlOrPath, window.location.origin).toString()
  } catch {
    return urlOrPath
  }
}

function linkStatus(link: ShareLink, nowMs: number): { label: string; cls: string; active: boolean } {
  if (link.revoked) {
    return { label: 'Révoqué', cls: 'border-line bg-track text-ink-3', active: false }
  }
  if (link.expiresAt && new Date(link.expiresAt).getTime() <= nowMs) {
    return { label: 'Expiré', cls: 'border-warn/35 bg-warn/12 text-warn', active: false }
  }
  return { label: 'Actif', cls: 'border-ok/35 bg-ok/12 text-ok', active: true }
}

/**
 * Settings section (#partage) for creating, copying, auditing, and revoking
 * read-only physiotherapist & coach share links.
 */
export function ShareSection() {
  const ids = useId()
  const links = useShareLinks()
  const m = useShareMutations()
  const { toast } = useToast()

  const [label, setLabel] = useState('Kiné du sport')
  const [expiry, setExpiry] = useState<ExpiryChoice>('30')
  const [formError, setFormError] = useState<string | null>(null)
  const [copiedId, setCopiedId] = useState<number | null>(null)
  const [revokingId, setRevokingId] = useState<number | null>(null)

  const nowMs = Date.now()
  const items = links.data ?? []

  const copyLink = async (link: ShareLink, announce = true) => {
    const url = fullShareUrl(link.url)
    const ok = await copyText(url)
    if (ok) {
      setCopiedId(link.id)
      window.setTimeout(() => setCopiedId((cur) => (cur === link.id ? null : cur)), 2000)
      if (announce) toast({ title: 'Lien copié dans le presse-papiers', description: url })
    } else if (announce) {
      toast({ tone: 'error', title: 'Copie impossible', description: url })
    }
  }

  const handleCreate = () => {
    const trimmed = label.trim()
    if (trimmed.length < 2) {
      setFormError('Indique un intitulé explicite (au moins 2 caractères).')
      return
    }
    setFormError(null)
    const expiresInDays = expiry === 'none' ? null : Number(expiry)
    m.create.mutate(
      { label: trimmed, expiresInDays },
      {
        onSuccess: (created) => {
          void copyLink(created, false)
          toast({
            tone: 'success',
            title: 'Lien de partage créé et copié',
            description: `Dossier « ${created.label} » prêt à envoyer.`,
          })
        },
        onError: (e) => setFormError(e.message),
      },
    )
  }

  const handleRevoke = (link: ShareLink) => {
    setRevokingId(link.id)
    m.revoke.mutate(link.id, {
      onSuccess: () => {
        setRevokingId(null)
        toast({ title: `Lien « ${link.label} » révoqué` })
      },
      onError: (e) => {
        setRevokingId(null)
        toast({ tone: 'error', title: 'Révocation impossible', description: e.message })
      },
    })
  }

  return (
    <Section
      id="partage"
      title="Partage Kiné & Coach"
      icon={<Share2 />}
      description="Crée un accès en lecture seule à ton dossier athlète (charge ACWR, cycles, cartographie corporelle, chaussures et séances récentes, sans trace GPS ni identifiants)."
    >
      <form
        className="glass-inset space-y-3.5 p-4"
        onSubmit={(e) => {
          e.preventDefault()
          handleCreate()
        }}
      >
        <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
          <div>
            <label htmlFor={`${ids}-label`} className="mb-1.5 block text-sm font-medium text-ink-2">
              Destinataire ou intitulé du lien
            </label>
            <input
              id={`${ids}-label`}
              className="field"
              value={label}
              maxLength={80}
              placeholder="Ex. Kiné du sport, Coach club…"
              onChange={(e) => setLabel(e.target.value)}
            />
          </div>
          <button type="submit" className="btn btn-primary h-11" disabled={m.create.isPending}>
            {m.create.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Plus className="size-4" aria-hidden="true" />}
            Créer le lien
          </button>
        </div>

        <div className="flex flex-wrap items-center gap-1.5" aria-label="Suggestions d’intitulé">
          {PRESET_LABELS.map((preset) => (
            <button
              key={preset}
              type="button"
              onClick={() => setLabel(preset)}
              className={cn(
                'chip h-7 border px-2.5 text-xs transition-colors',
                label === preset ? 'border-accent/45 bg-accent/14 text-ink' : 'border-line bg-track/60 text-ink-2 hover:text-ink',
              )}
            >
              {preset}
            </button>
          ))}
        </div>

        <div>
          <p className="mb-1.5 text-xs font-medium text-ink-3">Durée de validité</p>
          <SegmentedControl<ExpiryChoice>
            ariaLabel="Durée de validité du lien de partage"
            value={expiry}
            onChange={setExpiry}
            options={EXPIRY_OPTIONS}
          />
        </div>

        {formError && <InlineError>{formError}</InlineError>}
      </form>

      <div className="mt-4">
        {links.isError ? (
          <ErrorView error={links.error} onRetry={() => void links.refetch()} title="Liens de partage indisponibles" />
        ) : !links.data ? (
          <div className="space-y-2">
            <Skeleton className="h-20" />
            <Skeleton className="h-20" />
          </div>
        ) : items.length === 0 ? (
          <EmptyState icon={<Share2 />} title="Aucun lien de partage">
            Génère un lien nommé ci-dessus pour partager ton suivi de charge et ta cartographie corporelle avec ton kiné ou ton entraîneur.
          </EmptyState>
        ) : (
          <ul className="space-y-2.5" aria-label="Liens de partage créés">
            {items.map((link) => {
              const status = linkStatus(link, nowMs)
              const fullUrl = fullShareUrl(link.url)
              const isRevoking = revokingId === link.id && m.revoke.isPending
              const isCopied = copiedId === link.id
              return (
                <li
                  key={link.id}
                  className={cn(
                    'glass-inset flex flex-col gap-3 p-3.5 sm:flex-row sm:items-center sm:justify-between',
                    !status.active && 'opacity-75',
                  )}
                >
                  <div className="min-w-0 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="truncate font-semibold text-ink">{link.label}</p>
                      <span className={cn('chip h-5 border px-2 text-[11px]', status.cls)}>{status.label}</span>
                      <code className="rounded-md bg-track px-1.5 py-0.5 font-mono text-[11px] text-ink-3">
                        /share/{link.tokenPrefix}…
                      </code>
                    </div>
                    <p className="num flex flex-wrap gap-x-3 gap-y-1 text-xs text-ink-3">
                      <span>
                        {fmtNumber(link.accessCount)} consultation{link.accessCount > 1 ? 's' : ''}
                      </span>
                      <span>
                        Dernier accès : {link.lastAccessedAt ? fmtDateTime(link.lastAccessedAt) : 'jamais ouvert'}
                      </span>
                      <span>
                        {link.expiresAt ? `Expire le ${fmtDateTime(link.expiresAt)}` : 'Aucune expiration'}
                      </span>
                    </p>
                  </div>

                  <div className="flex flex-none flex-wrap items-center gap-2">
                    {status.active && (
                      <button
                        type="button"
                        className="btn btn-ghost h-9 px-3 text-xs"
                        onClick={() => void copyLink(link)}
                        aria-label={`Copier le lien ${link.label}`}
                        title={fullUrl}
                      >
                        {isCopied ? <Check className="size-3.5 text-ok" aria-hidden="true" /> : <Copy className="size-3.5" aria-hidden="true" />}
                        {isCopied ? 'Copié' : 'Copier le lien'}
                      </button>
                    )}
                    {!link.revoked && (
                      <button
                        type="button"
                        className="btn btn-ghost h-9 px-3 text-xs text-danger hover:bg-danger/10"
                        disabled={isRevoking}
                        onClick={() => handleRevoke(link)}
                        aria-label={`Révoquer le lien ${link.label}`}
                      >
                        {isRevoking ? (
                          <Loader2 className="size-3.5 animate-spin" aria-hidden="true" />
                        ) : (
                          <Link2Off className="size-3.5" aria-hidden="true" />
                        )}
                        Révoquer
                      </button>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </div>
    </Section>
  )
}
