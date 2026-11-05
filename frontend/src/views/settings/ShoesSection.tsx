import { Download, Footprints, GitMerge, Loader2, Pencil, Plus, Trash2 } from 'lucide-react'
import { useId, useState } from 'react'
import { useIntegrations, useShoeMutations, useShoes } from '../../api/queries'
import { useMergeShoe } from '../../api/queriesV3'
import type { Shoe, ShoeInput, ShoeType } from '../../api/types'
import { Checkbox } from '../../components/ui/Field'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { Dialog } from '../../components/ui/Dialog'
import { EmptyState, InlineError } from '../../components/ui/Misc'
import { SegmentedControl } from '../../components/ui/SegmentedControl'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { cn } from '../../lib/cn'
import { fmtNumber } from '../../lib/format'
import { SHOE_TYPE_LABEL } from '../../lib/labels'
import { ErrorView } from '../StateViews'
import { Section } from './Section'

const norm = (x: string) =>
  x
    .toLowerCase()
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/\(strava\)|[^a-z0-9]+/g, '')
const shoeKey = (s: Shoe) => norm(`${s.brand}${s.model}`) || norm(s.name)

function MergeDialog({ source, shoes, onClose }: { source: Shoe | null; shoes: Shoe[]; onClose: () => void }) {
  const merge = useMergeShoe()
  const { toast } = useToast()
  const others = shoes.filter((x) => x.id !== source?.id)
  const dup = source ? others.find((x) => shoeKey(x) === shoeKey(source)) : undefined
  const [into, setInto] = useState<number | null>(null)
  const target = into ?? dup?.id ?? others[0]?.id ?? null
  return (
    <Dialog
      open={source !== null}
      onClose={onClose}
      title="Fusionner une paire"
      description={source ? `« ${source.name} » sera supprimée ; ses ressentis et son kilométrage passent sur la paire choisie.` : undefined}
      footer={
        <div className="flex justify-end gap-2 pb-3">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Annuler
          </button>
          <button
            type="button"
            className="btn btn-primary"
            disabled={!source || target === null || merge.isPending}
            onClick={() =>
              source &&
              target !== null &&
              merge.mutate(
                { id: source.id, intoId: target },
                {
                  onSuccess: () => {
                    toast({ title: 'Paires fusionnées', tone: 'success' })
                    onClose()
                  },
                },
              )
            }
          >
            {merge.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <GitMerge className="size-4" aria-hidden="true" />}
            Fusionner
          </button>
        </div>
      }
    >
      <label htmlFor="merge-into" className="mb-1.5 block text-sm font-medium text-ink-2">
        Fusionner dans
      </label>
      <select id="merge-into" className="field" value={target ?? ''} onChange={(e) => setInto(Number(e.target.value))}>
        {others.map((x) => (
          <option key={x.id} value={x.id}>
            {x.name} · {fmtNumber(x.distanceKm)} km{x.id === dup?.id ? ' (doublon probable)' : ''}
          </option>
        ))}
      </select>
      {merge.isError && <InlineError className="mt-3">{merge.error.message}</InlineError>}
    </Dialog>
  )
}

const EMPTY: ShoeInput = { name: '', brand: '', model: '', dropMm: null, type: 'soft', retired: false, note: '' }

export function ShoesSection() {
  const shoes = useShoes()
  const integ = useIntegrations()
  const m = useShoeMutations()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Shoe | 'new' | null>(null)
  const [del, setDel] = useState<Shoe | null>(null)
  const [merging, setMerging] = useState<Shoe | null>(null)
  const strava = integ.data?.strava.connected ?? false
  const list = [...(shoes.data ?? [])].sort((a, b) => Number(a.retired) - Number(b.retired) || a.name.localeCompare(b.name, 'fr'))
  const keyCount = new Map<string, number>()
  for (const x of list) keyCount.set(shoeKey(x), (keyCount.get(shoeKey(x)) ?? 0) + 1)
  const isDup = (x: Shoe) => (keyCount.get(shoeKey(x)) ?? 0) > 1

  const importStrava = () =>
    m.importStrava.mutate(undefined, {
      onSuccess: (r) => toast({ title: `${r.length} chaussure${r.length > 1 ? 's' : ''} importée${r.length > 1 ? 's' : ''} depuis Strava` }),
      onError: (e) => toast({ tone: 'error', title: 'Import Strava impossible', description: e.message }),
    })

  return (
    <Section
      id="chaussures"
      title="Chaussures"
      icon={<Footprints />}
      description="Tes paires (propres à ton compte) : drop, type et kilométrage apparaissent dans les rapports et l’historique 14 jours."
      actions={
        <>
          {strava && (
            <button type="button" className="btn btn-ghost" onClick={importStrava} disabled={m.importStrava.isPending}>
              {m.importStrava.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Download className="size-4" aria-hidden="true" />}
              Importer depuis Strava
            </button>
          )}
          <button type="button" className="btn btn-primary" onClick={() => setEdit('new')}>
            <Plus className="size-4" aria-hidden="true" /> Ajouter
          </button>
        </>
      }
    >
      {shoes.isError ? (
        <ErrorView error={shoes.error} onRetry={() => void shoes.refetch()} title="Chaussures indisponibles" />
      ) : !shoes.data ? (
        <div className="grid gap-2 sm:grid-cols-2">
          <Skeleton className="h-24" />
          <Skeleton className="h-24" />
        </div>
      ) : list.length === 0 ? (
        <EmptyState icon={<Footprints />} title="Aucune chaussure">
          Ajoute tes paires pour suivre leur kilométrage et les lier à tes ressentis{strava ? ', ou importe-les depuis Strava' : ''}.
        </EmptyState>
      ) : (
        <ul className="grid gap-2 sm:grid-cols-2">
          {list.map((s) => (
            <li key={s.id} className={cn('glass-inset flex min-w-0 items-start gap-3 p-3', s.retired && 'opacity-70')}>
              <div className="min-w-0 flex-1">
                <p className="truncate font-semibold text-ink">{s.name}</p>
                <p className="truncate text-xs text-ink-3">{[s.brand, s.model].filter(Boolean).join(' ') || '—'}</p>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  <span className="chip border border-line bg-track text-ink-2">{SHOE_TYPE_LABEL[s.type]}</span>
                  <span className="chip num border border-line bg-track text-ink-2">Drop {s.dropMm === null ? 'N/D' : `${fmtNumber(s.dropMm)} mm`}</span>
                  <span className="chip num border border-line bg-track text-ink-2">{fmtNumber(s.distanceKm)} km</span>
                  {s.retired && <span className="chip border border-line bg-track text-ink-3">Retirée</span>}
                  {s.stravaGearId && <span className="chip border border-[#cc3d00]/40 bg-[#cc3d00]/10 text-ink-2">Strava</span>}
                  {isDup(s) && <span className="chip border border-warn/35 bg-warn/10 text-warn">Doublon probable</span>}
                </div>
              </div>
              <div className="flex flex-none gap-1">
                {list.length > 1 && (
                  <button type="button" className={cn('icon-btn size-9', isDup(s) && 'text-warn')} onClick={() => setMerging(s)} aria-label={`Fusionner ${s.name} dans une autre paire`}>
                    <GitMerge className="size-4" />
                  </button>
                )}
                <button type="button" className="icon-btn size-9" onClick={() => setEdit(s)} aria-label={`Modifier ${s.name}`}>
                  <Pencil className="size-4" />
                </button>
                <button type="button" className="icon-btn size-9" onClick={() => setDel(s)} aria-label={`Supprimer ${s.name}`}>
                  <Trash2 className="size-4" />
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}

      <MergeDialog key={merging?.id ?? 'none'} source={merging} shoes={list} onClose={() => setMerging(null)} />
      <ShoeDialog key={edit === null ? 'closed' : edit === 'new' ? 'new' : edit.id} shoe={edit} onClose={() => setEdit(null)} />
      <ConfirmDialog
        open={del !== null}
        onClose={() => setDel(null)}
        title="Supprimer cette paire ?"
        message={<p>« {del?.name} » sera retirée de la liste. Les ressentis déjà saisis gardent le nom de la chaussure dans l’historique si le serveur le conserve.</p>}
        confirmLabel="Supprimer"
        danger
        busy={m.remove.isPending}
        onConfirm={() =>
          del &&
          m.remove.mutate(del.id, {
            onSuccess: () => {
              setDel(null)
              toast({ title: 'Chaussure supprimée' })
            },
            onError: (e) => toast({ tone: 'error', title: 'Suppression impossible', description: e.message }),
          })
        }
      />
    </Section>
  )
}

function ShoeDialog({ shoe, onClose }: { shoe: Shoe | 'new' | null; onClose: () => void }) {
  const ids = useId()
  const m = useShoeMutations()
  const { toast } = useToast()
  const initial: ShoeInput = shoe && shoe !== 'new' ? { name: shoe.name, brand: shoe.brand, model: shoe.model, dropMm: shoe.dropMm, type: shoe.type, retired: shoe.retired, note: shoe.note } : EMPTY
  const [v, setV] = useState<ShoeInput>(initial)
  const [drop, setDrop] = useState(initial.dropMm === null ? '' : String(initial.dropMm))
  const [error, setError] = useState<string | null>(null)
  const busy = m.create.isPending || m.update.isPending

  const submit = () => {
    if (!v.name.trim()) return setError('Donne un nom à cette paire.')
    const d = drop.trim() === '' ? null : Number(drop.replace(',', '.'))
    if (d !== null && (!Number.isFinite(d) || d < 0 || d > 20)) return setError('Drop entre 0 et 20 mm.')
    setError(null)
    const body: ShoeInput = { ...v, name: v.name.trim(), brand: v.brand.trim(), model: v.model.trim(), note: v.note.trim(), dropMm: d }
    const done = {
      onSuccess: () => {
        toast({ title: shoe === 'new' ? 'Chaussure ajoutée' : 'Chaussure mise à jour' })
        onClose()
      },
      onError: (e: Error) => setError(e.message),
    }
    if (shoe === 'new') m.create.mutate(body, done)
    else if (shoe) m.update.mutate({ id: shoe.id, ...body }, done)
  }

  return (
    <Dialog
      open={shoe !== null}
      onClose={onClose}
      title={shoe === 'new' ? 'Nouvelle paire' : 'Modifier la paire'}
      initialFocus={`#${CSS.escape(`${ids}-name`)}`}
      footer={
        <div className="flex justify-end gap-2">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Annuler
          </button>
          <button type="button" className="btn btn-primary" onClick={submit} disabled={busy}>
            {busy && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            Enregistrer
          </button>
        </div>
      }
    >
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div>
          <label htmlFor={`${ids}-name`} className="mb-1.5 block text-sm font-medium text-ink-2">
            Nom
          </label>
          <input id={`${ids}-name`} className="field" value={v.name} maxLength={60} placeholder="Ex. Paire du quotidien" onChange={(e) => setV({ ...v, name: e.target.value })} />
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <label htmlFor={`${ids}-brand`} className="mb-1.5 block text-sm font-medium text-ink-2">
              Marque
            </label>
            <input id={`${ids}-brand`} className="field" value={v.brand} maxLength={40} onChange={(e) => setV({ ...v, brand: e.target.value })} />
          </div>
          <div>
            <label htmlFor={`${ids}-model`} className="mb-1.5 block text-sm font-medium text-ink-2">
              Modèle
            </label>
            <input id={`${ids}-model`} className="field" value={v.model} maxLength={60} onChange={(e) => setV({ ...v, model: e.target.value })} />
          </div>
        </div>
        <div className="grid gap-3 sm:grid-cols-[8rem_minmax(0,1fr)]">
          <div>
            <label htmlFor={`${ids}-drop`} className="mb-1.5 block text-sm font-medium text-ink-2">
              Drop (mm)
            </label>
            <input id={`${ids}-drop`} className="field num" type="number" inputMode="decimal" min={0} max={20} step={0.5} value={drop} onChange={(e) => setDrop(e.target.value)} />
          </div>
          <div className="min-w-0">
            <p className="mb-1.5 text-sm font-medium text-ink-2">Type</p>
            <SegmentedControl<ShoeType>
              ariaLabel="Type de chaussure"
              value={v.type}
              onChange={(type) => setV({ ...v, type })}
              options={(Object.keys(SHOE_TYPE_LABEL) as ShoeType[]).map((t) => ({ value: t, label: SHOE_TYPE_LABEL[t] }))}
            />
          </div>
        </div>
        <div>
          <label htmlFor={`${ids}-note`} className="mb-1.5 block text-sm font-medium text-ink-2">
            Note
          </label>
          <textarea id={`${ids}-note`} className="field min-h-20 resize-y" value={v.note} maxLength={300} onChange={(e) => setV({ ...v, note: e.target.value })} />
        </div>
        <Checkbox checked={v.retired} onChange={(retired) => setV({ ...v, retired })} label="Paire retirée" description="Elle n’est plus proposée dans le formulaire de ressenti." />
        {error && <InlineError>{error}</InlineError>}
      </form>
    </Dialog>
  )
}
