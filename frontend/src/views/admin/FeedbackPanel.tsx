import { Bug, Lightbulb, Loader2, Mail, MessageCircle, MessageSquareText, type LucideIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useAdminControl, useAdminFeedback } from '../../api/queries'
import type { FeedbackCategory, FeedbackItem, FeedbackStatus } from '../../api/types'
import { Card } from '../../components/ui/Card'
import { Dialog } from '../../components/ui/Dialog'
import { Field } from '../../components/ui/Field'
import { EmptyState, KeyValue } from '../../components/ui/Misc'
import { SegmentedControl } from '../../components/ui/SegmentedControl'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { cn } from '../../lib/cn'
import { fmtDateTime, fmtWhen } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { FilterChips } from './adminUtils'

const CATEGORY: Record<FeedbackCategory, { label: string; icon: LucideIcon; cls: string }> = {
  bug: { label: 'Bug', icon: Bug, cls: 'border-danger/30 bg-danger/10 text-danger' },
  idea: { label: 'Idée', icon: Lightbulb, cls: 'border-info/30 bg-info/10 text-info' },
  other: { label: 'Autre', icon: MessageCircle, cls: 'border-line bg-track/60 text-ink-2' },
}
const STATUS_LABEL: Record<FeedbackStatus, string> = { new: 'Nouveau', read: 'Lu', done: 'Traité' }

function CategoryChip({ c }: { c: FeedbackCategory }) {
  const m = CATEGORY[c]
  return (
    <span className={cn('chip h-6 flex-none border px-2 text-[11px]', m.cls)}>
      <m.icon className="size-3.5" aria-hidden="true" />
      {m.label}
    </span>
  )
}

function mailto(f: FeedbackItem): string {
  const quoted = f.message
    .slice(0, 600)
    .split('\n')
    .map((l) => `> ${l}`)
    .join('\n')
  return `mailto:${f.contactEmail}?subject=${encodeURIComponent('Re : ton avis sur Stride')}&body=${encodeURIComponent(`Bonjour ${f.userName},\n\n${quoted}\n\n`)}`
}

function FeedbackDetail({ item, onClose }: { item: FeedbackItem; onClose: () => void }) {
  const ctl = useAdminControl()
  const { toast } = useToast()
  const [status, setStatus] = useState<FeedbackStatus>(item.status === 'new' ? 'read' : item.status)
  const [note, setNote] = useState(item.adminNote)
  const marked = useRef(false)
  const update = ctl.updateFeedback
  const { mutate } = update

  // Opening an unread feedback marks it as read.
  useEffect(() => {
    if (item.status === 'new' && !marked.current) {
      marked.current = true
      mutate({ id: item.id, status: 'read' })
    }
  }, [item.id, item.status, mutate])

  const setAndSave = (s: FeedbackStatus) => {
    setStatus(s)
    update.mutate({ id: item.id, status: s }, { onError: (e) => toast({ tone: 'error', title: 'Statut non enregistré', description: e.message }) })
  }
  const saveNote = () =>
    update.mutate(
      { id: item.id, adminNote: note },
      { onSuccess: () => toast({ title: 'Note enregistrée' }), onError: (e) => toast({ tone: 'error', title: 'Note non enregistrée', description: e.message }) },
    )

  return (
    <Dialog open onClose={onClose} variant="drawer" title={`Avis de ${item.userName || `#${item.userId}`}`} description={`${CATEGORY[item.category].label} · ${fmtDateTime(item.createdAt)}`}>
      <div className="space-y-5">
        <p className="rounded-2xl border border-line bg-track/40 px-4 py-3 text-sm leading-relaxed break-words whitespace-pre-wrap text-ink">{item.message}</p>

        <section aria-label="Contact">
          {item.contactOk && item.contactEmail ? (
            <div className="flex flex-wrap items-center justify-between gap-3">
              <span className="min-w-0 text-sm text-ink-2">
                Accepte d’être recontacté : <span className="font-medium break-all text-ink">{item.contactEmail}</span>
              </span>
              <a className="btn btn-primary" href={mailto(item)}>
                <Mail className="size-4" aria-hidden="true" />
                Répondre par e-mail
              </a>
            </div>
          ) : (
            <p className="text-sm text-ink-3">Pas de contact souhaité : aucune adresse e-mail n’a été transmise.</p>
          )}
        </section>

        <div>
          <p className="mb-1.5 text-sm font-medium text-ink-2">Statut</p>
          <SegmentedControl<FeedbackStatus>
            ariaLabel="Statut de l’avis"
            className="max-w-sm"
            value={status}
            onChange={setAndSave}
            options={(['new', 'read', 'done'] as const).map((s) => ({ value: s, label: STATUS_LABEL[s] }))}
          />
        </div>

        <Field label="Note interne" hint="Visible des administrateurs uniquement.">
          {(p) => <textarea {...p} className="field min-h-24 resize-y text-sm" maxLength={2000} value={note} onChange={(e) => setNote(e.target.value)} placeholder="Ex. : corrigé en 4.1.1, ticket ouvert…" />}
        </Field>
        <button type="button" className="btn btn-ghost" onClick={saveNote} disabled={update.isPending || note === item.adminNote}>
          {update.isPending && update.variables?.adminNote !== undefined && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
          Enregistrer la note
        </button>

        <section aria-label="Contexte technique" className="border-t border-line pt-4">
          <p className="label-caps mb-2">Contexte joint</p>
          <KeyValue
            items={[
              ['Version', <span className="num">{item.context.appVersion || '—'}</span>],
              ['Page', <span className="font-mono text-xs break-all">{item.context.route || '—'}</span>],
              ['Écran', <span className="num">{item.context.viewport || '—'}</span>],
              ['Navigateur', <span className="text-xs break-all">{item.context.userAgent || '—'}</span>],
            ]}
          />
        </section>
      </div>
    </Dialog>
  )
}

export function FeedbackPanel() {
  const [status, setStatus] = useState<FeedbackStatus | 'all'>('new')
  const [category, setCategory] = useState<FeedbackCategory | 'all'>('all')
  // Snapshot of the opened entry: it stays open even when « mark as read » moves it out of the filter.
  const [openItem, setOpenItem] = useState<FeedbackItem | null>(null)
  const list = useAdminFeedback(status, category)
  const items = list.data?.items ?? []
  const counts = list.data?.counts

  return (
    <Card
      title="Avis des utilisateurs"
      icon={<MessageSquareText className="size-4" />}
      subtitle={counts ? `${counts.new} non lu${counts.new > 1 ? 's' : ''} · ${counts.total} au total` : 'Bugs, idées et remarques envoyés depuis l’application'}
    >
      <div className="mb-4 flex flex-wrap items-center gap-x-4 gap-y-2.5">
        <FilterChips<FeedbackStatus | 'all'>
          ariaLabel="Filtrer par statut"
          value={status}
          onChange={setStatus}
          options={[
            { value: 'new', label: 'Non lus', count: counts?.new },
            { value: 'read', label: 'Lus', count: counts?.read },
            { value: 'done', label: 'Traités', count: counts?.done },
            { value: 'all', label: 'Tous', count: counts?.total },
          ]}
        />
        <label className="flex items-center gap-2 text-xs text-ink-3 sm:ml-auto">
          Type
          <select className="field min-h-9 w-auto py-1 text-sm" value={category} onChange={(e) => setCategory(e.target.value as FeedbackCategory | 'all')}>
            <option value="all">Tous</option>
            <option value="bug">Bugs</option>
            <option value="idea">Idées</option>
            <option value="other">Autres</option>
          </select>
        </label>
      </div>

      {list.isError ? (
        <ErrorView error={list.error} onRetry={() => void list.refetch()} title="Avis indisponibles" />
      ) : !list.data ? (
        <div className="space-y-2">
          <Skeleton className="h-16" />
          <Skeleton className="h-16" />
        </div>
      ) : items.length === 0 ? (
        <EmptyState icon={<MessageSquareText />} title={status === 'new' ? 'Aucun avis non lu' : 'Aucun avis'}>
          Les utilisateurs envoient leurs avis depuis le menu du compte, la palette de recherche ou le bas des Réglages.
        </EmptyState>
      ) : (
        <ul className="divide-y divide-line border-y border-line">
          {items.map((f) => (
            <li key={f.id}>
              <button type="button" onClick={() => setOpenItem(f)} className="flex w-full items-start gap-3 px-1 py-3 text-left transition-colors hover:bg-track/50 focus-visible:bg-track/50">
                <span className={cn('mt-2 size-2 flex-none rounded-full', f.status === 'new' ? 'bg-accent' : 'bg-transparent')} aria-hidden="true" />
                <span className="min-w-0 flex-1">
                  <span className="flex flex-wrap items-center gap-2">
                    <CategoryChip c={f.category} />
                    <span className={cn('text-sm', f.status === 'new' ? 'font-semibold text-ink' : 'font-medium text-ink-2')}>{f.userName || `#${f.userId}`}</span>
                    {f.contactOk && <Mail className="size-3.5 text-ink-3" aria-label="Accepte d’être recontacté" />}
                    {f.status === 'done' && <span className="text-xs text-ok">Traité</span>}
                    <span className="num ml-auto text-xs text-ink-3">{fmtWhen(f.createdAt)}</span>
                  </span>
                  <span className="mt-1 line-clamp-2 block text-sm break-words text-ink-2">{f.message}</span>
                  {f.status === 'new' && <span className="sr-only">Non lu.</span>}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {openItem && <FeedbackDetail key={openItem.id} item={openItem} onClose={() => setOpenItem(null)} />}
    </Card>
  )
}
