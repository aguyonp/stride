import { ChevronDown, Download, Footprints, GitMerge, Loader2, Pencil, Plus, RefreshCw, ShieldAlert, Trash2 } from 'lucide-react'
import { useId, useState } from 'react'
import { useLocation } from 'wouter'
import { useIntegrations, useShoeMutations, useShoes } from '../../api/queries'
import { useMergeShoe } from '../../api/queriesV3'
import type { Shoe, ShoeInput, ShoeType } from '../../api/types'
import { StravaConnectButton } from '../../components/profile/StravaConnect'
import { Checkbox } from '../../components/ui/Field'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { Dialog } from '../../components/ui/Dialog'
import { EmptyState, InlineError } from '../../components/ui/Misc'
import { SegmentedControl } from '../../components/ui/SegmentedControl'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { cn } from '../../lib/cn'
import { fmtNumber, fmtWhen, ND } from '../../lib/format'
import { SHOE_TYPE_LABEL } from '../../lib/labels'
import { ErrorView } from '../StateViews'
import { Section } from './Section'
import { getLang, lazyRecord, t } from '../../i18n'

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
      title={t('shoes.section.mergeTitle')}
      description={source ? t('shoes.section.mergeHint', { name: source.name }) : undefined}
      footer={
        <div className="flex justify-end gap-2 pb-3">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            {t('common.actions.cancel')}
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
                    toast({ title: t('shoes.section.merged'), tone: 'success' })
                    onClose()
                  },
                },
              )
            }
          >
            {merge.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <GitMerge className="size-4" aria-hidden="true" />}
            {t('shoes.section.merge')}
          </button>
        </div>
      }
    >
      <label htmlFor="merge-into" className="mb-1.5 block text-sm font-medium text-ink-2">
        {t('shoes.section.mergeInto')}
      </label>
      <select id="merge-into" className="field" value={target ?? ''} onChange={(e) => setInto(Number(e.target.value))}>
        {others.map((x) => (
          <option key={x.id} value={x.id}>
            {x.name} · {fmtNumber(x.distanceKm)} km{x.id === dup?.id ? t('shoes.section.likelyDupSuffix') : ''}
          </option>
        ))}
      </select>
      {merge.isError && <InlineError className="mt-3">{merge.error.message}</InlineError>}
    </Dialog>
  )
}

const EMPTY: ShoeInput = { name: '', brand: '', model: '', dropMm: null, type: 'soft', retired: false, note: '' }

/** Active pairs: the Strava default first, then the most worn. */
const byUse = (a: Shoe, b: Shoe) => Number(b.stravaPrimary) - Number(a.stravaPrimary) || b.distanceKm - a.distanceKm || a.name.localeCompare(b.name, getLang())

const brandModel = (s: Shoe) => [s.brand, s.model].filter(Boolean).join(' ')

/** Where the total comes from, never presenting Stride's partial sum as the pair's mileage. */
function mileageLabel(s: Shoe): { total: string; detail: string | null } {
  if (s.distanceSource === 'strava') {
    const analyzed = s.analyzedKm > 0 && s.analyzedKm < s.distanceKm ? t('shoes.section.analyzedPart', { km: fmtNumber(s.analyzedKm, 1) }) : null
    return { total: s.stravaState === 'deleted' ? t('shoes.section.lastStravaTotal') : t('shoes.section.stravaTotal'), detail: analyzed }
  }
  if (s.stravaGearId) return { total: t('shoes.section.analyzed'), detail: t('shoes.section.stravaPending') }
  return { total: t('shoes.section.strideTotal'), detail: t('shoes.section.strideTotalHint') }
}

const RETIRED_LABEL = lazyRecord(['retired', 'deleted'] as const, (k) => t(k === 'retired' ? 'shoes.section.retiredStrava' : 'shoes.section.deletedStrava'))

function ShoeCard({ s, dup, canMerge, onMerge, onEdit, onDelete }: { s: Shoe; dup: boolean; canMerge: boolean; onMerge: () => void; onEdit: () => void; onDelete: () => void }) {
  const bm = brandModel(s)
  const m = mileageLabel(s)
  const retiredLabel = s.retired ? (s.stravaState === 'retired' || s.stravaState === 'deleted' ? RETIRED_LABEL[s.stravaState] : t('shoes.section.retired')) : null
  return (
    <li className="glass-inset flex min-w-0 items-start gap-3 p-3">
      <div className="min-w-0 flex-1">
        <p className="truncate font-semibold text-ink">{s.name}</p>
        {bm && norm(bm) !== norm(s.name) && <p className="truncate text-xs text-ink-3">{bm}</p>}
        <p className="mt-2 flex flex-wrap items-baseline gap-x-1.5">
          <span className="num text-lg font-semibold tracking-tight text-ink">{fmtNumber(s.distanceKm, 1)}&nbsp;km</span>
          <span className="text-xs text-ink-3">{m.total}</span>
        </p>
        {m.detail && <p className="text-xs text-ink-3">{m.detail}</p>}
        <div className="mt-2 flex flex-wrap gap-1.5">
          {s.stravaPrimary && !s.retired && <span className="chip border border-accent/35 bg-accent/10 text-ink">{t('shoes.section.primary')}</span>}
          {retiredLabel && <span className="chip border border-line bg-track text-ink-2">{retiredLabel}</span>}
          <span className="chip border border-line bg-track text-ink-2">{SHOE_TYPE_LABEL[s.type]}</span>
          <span className="chip num border border-line bg-track text-ink-2">{t('shoes.section.drop', { value: s.dropMm === null ? ND : `${fmtNumber(s.dropMm)} mm` })}</span>
          {s.stravaGearId && <span className="chip border border-[#cc3d00]/40 bg-[#cc3d00]/10 text-ink-2">Strava</span>}
          {dup && <span className="chip border border-warn/35 bg-warn/10 text-warn">{t('shoes.section.likelyDup')}</span>}
        </div>
      </div>
      <div className="flex flex-none gap-1">
        {canMerge && (
          <button type="button" className={cn('icon-btn size-9', dup && 'text-warn')} onClick={onMerge} aria-label={t('shoes.section.mergeLabel', { name: s.name })}>
            <GitMerge className="size-4" />
          </button>
        )}
        <button type="button" className="icon-btn size-9" onClick={onEdit} aria-label={t('shoes.section.editLabel', { name: s.name })}>
          <Pencil className="size-4" />
        </button>
        <button type="button" className="icon-btn size-9" onClick={onDelete} aria-label={t('shoes.section.deleteLabel', { name: s.name })}>
          <Trash2 className="size-4" />
        </button>
      </div>
    </li>
  )
}

export function ShoesSection() {
  const shoes = useShoes()
  const integ = useIntegrations()
  const m = useShoeMutations()
  const { toast } = useToast()
  const [path] = useLocation()
  const retiredId = useId()
  const [edit, setEdit] = useState<Shoe | 'new' | null>(null)
  const [del, setDel] = useState<Shoe | null>(null)
  const [merging, setMerging] = useState<Shoe | null>(null)
  const [showRetired, setShowRetired] = useState(false)
  const stravaInfo = integ.data?.strava
  const strava = stravaInfo?.connected ?? false
  const gearScopeMissing = strava && (stravaInfo?.missingScopes ?? []).includes('profile:read_all')
  const list = shoes.data ?? []
  const active = list.filter((x) => !x.retired).sort(byUse)
  const retired = list.filter((x) => x.retired).sort((a, b) => b.distanceKm - a.distanceKm || a.name.localeCompare(b.name, getLang()))
  const linked = list.some((x) => x.stravaGearId)
  const lastSync = list.reduce<string | null>((acc, x) => (x.stravaSyncedAt && (!acc || x.stravaSyncedAt > acc) ? x.stravaSyncedAt : acc), null)
  const keyCount = new Map<string, number>()
  for (const x of list) keyCount.set(shoeKey(x), (keyCount.get(shoeKey(x)) ?? 0) + 1)
  const isDup = (x: Shoe) => (keyCount.get(shoeKey(x)) ?? 0) > 1
  const card = (s: Shoe) => (
    <ShoeCard key={s.id} s={s} dup={isDup(s)} canMerge={list.length > 1} onMerge={() => setMerging(s)} onEdit={() => setEdit(s)} onDelete={() => setDel(s)} />
  )

  const importStrava = () =>
    m.importStrava.mutate(undefined, {
      onSuccess: (r) => {
        const off = r.filter((x) => x.retired).length
        const on = r.length - off
        toast({
          title: t('shoes.section.stravaUpdated'),
          description: t('shoes.section.stravaCounts', { active: t('shoes.section.activePairs', { count: on }), retired: t('shoes.section.retiredPairs', { count: off }) }),
        })
      },
      onError: (e) => toast({ tone: 'error', title: t('shoes.section.stravaFailed'), description: e.message }),
    })

  return (
    <Section
      id="chaussures"
      title={t('shoes.section.title')}
      icon={<Footprints />}
      description={t('shoes.section.description')}
      actions={
        <>
          {strava && !gearScopeMissing && (
            <button type="button" className="btn btn-ghost" onClick={importStrava} disabled={m.importStrava.isPending}>
              {m.importStrava.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : linked ? <RefreshCw className="size-4" aria-hidden="true" /> : <Download className="size-4" aria-hidden="true" />}
              {linked ? t('shoes.section.refreshStrava') : t('shoes.section.importStrava')}
            </button>
          )}
          <button type="button" className="btn btn-primary" onClick={() => setEdit('new')}>
            <Plus className="size-4" aria-hidden="true" /> {t('shoes.section.add')}
          </button>
        </>
      }
    >
      {gearScopeMissing && (
        <div role="status" className="mb-4 rounded-xl border border-warn/30 bg-warn/[0.08] p-3 text-sm text-ink-2">
          <p className="flex items-center gap-1.5 font-semibold text-ink">
            <ShieldAlert className="size-4 flex-none text-warn" aria-hidden="true" /> {t('shoes.section.scopeTitle')}
          </p>
          <p className="mt-1 text-xs">
            {t('shoes.section.scopeBody')}
          </p>
          <div className="mt-2.5">
            <StravaConnectButton strava={stravaInfo} returnPath={path} label={t('shoes.section.reconnect')} />
          </div>
        </div>
      )}
      {shoes.isError ? (
        <ErrorView error={shoes.error} onRetry={() => void shoes.refetch()} title={t('shoes.section.unavailable')} />
      ) : !shoes.data ? (
        <div className="grid gap-2 sm:grid-cols-2">
          <Skeleton className="h-24" />
          <Skeleton className="h-24" />
        </div>
      ) : list.length === 0 ? (
        <EmptyState icon={<Footprints />} title={t('shoes.section.empty')}>
          {strava ? t('shoes.section.emptyHintStrava') : t('shoes.section.emptyHint')}
        </EmptyState>
      ) : (
        <>
          {active.length > 0 ? (
            <ul className="grid gap-2 sm:grid-cols-2" aria-label={t('shoes.section.activeList')}>
              {active.map(card)}
            </ul>
          ) : (
            <p className="text-sm text-ink-3">{t('shoes.section.noActive')}</p>
          )}
          {retired.length > 0 && (
            <div className="mt-4">
              <button type="button" className="btn btn-ghost -ml-2 px-2" aria-expanded={showRetired} aria-controls={retiredId} onClick={() => setShowRetired((v) => !v)}>
                <ChevronDown className={cn('size-4 transition-transform duration-200', showRetired && 'rotate-180')} aria-hidden="true" />
                {t('shoes.section.retiredToggle', { count: retired.length })}
              </button>
              <ul id={retiredId} hidden={!showRetired} className="mt-2 grid gap-2 sm:grid-cols-2" aria-label={t('shoes.section.retiredList')}>
                {showRetired && retired.map(card)}
              </ul>
            </div>
          )}
          {lastSync && <p className="mt-3 text-xs text-ink-3">{t('shoes.section.lastSync', { when: fmtWhen(lastSync) })}</p>}
        </>
      )}

      <MergeDialog key={merging?.id ?? 'none'} source={merging} shoes={list} onClose={() => setMerging(null)} />
      <ShoeDialog key={edit === null ? 'closed' : edit === 'new' ? 'new' : edit.id} shoe={edit} onClose={() => setEdit(null)} />
      <ConfirmDialog
        open={del !== null}
        onClose={() => setDel(null)}
        title={t('shoes.section.deleteTitle')}
        message={
          <>
            <p>{t('shoes.section.deleteBody', { name: del?.name })}</p>
            {del?.stravaGearId && del.stravaState === 'active' && (
              <p>{t('shoes.section.deleteStravaHint')}</p>
            )}
          </>
        }
        confirmLabel={t('common.actions.delete')}
        danger
        busy={m.remove.isPending}
        onConfirm={() =>
          del &&
          m.remove.mutate(del.id, {
            onSuccess: () => {
              setDel(null)
              toast({ title: t('shoes.section.deleted') })
            },
            onError: (e) => toast({ tone: 'error', title: t('shoes.section.deleteFailed'), description: e.message }),
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
    if (!v.name.trim()) return setError(t('shoes.section.nameRequired'))
    const d = drop.trim() === '' ? null : Number(drop.replace(',', '.'))
    if (d !== null && (!Number.isFinite(d) || d < 0 || d > 20)) return setError(t('shoes.section.dropRange'))
    setError(null)
    const body: ShoeInput = { ...v, name: v.name.trim(), brand: v.brand.trim(), model: v.model.trim(), note: v.note.trim(), dropMm: d }
    const done = {
      onSuccess: () => {
        toast({ title: shoe === 'new' ? t('shoes.section.added') : t('shoes.section.updated') })
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
      title={shoe === 'new' ? t('shoes.section.newTitle') : t('shoes.section.editTitle')}
      initialFocus={`#${CSS.escape(`${ids}-name`)}`}
      footer={
        <div className="flex justify-end gap-2">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button type="button" className="btn btn-primary" onClick={submit} disabled={busy}>
            {busy && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {t('common.actions.save')}
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
            {t('shoes.section.name')}
          </label>
          <input id={`${ids}-name`} className="field" value={v.name} maxLength={60} placeholder={t('shoes.section.namePlaceholder')} onChange={(e) => setV({ ...v, name: e.target.value })} />
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <label htmlFor={`${ids}-brand`} className="mb-1.5 block text-sm font-medium text-ink-2">
              {t('shoes.section.brand')}
            </label>
            <input id={`${ids}-brand`} className="field" value={v.brand} maxLength={40} onChange={(e) => setV({ ...v, brand: e.target.value })} />
          </div>
          <div>
            <label htmlFor={`${ids}-model`} className="mb-1.5 block text-sm font-medium text-ink-2">
              {t('shoes.section.model')}
            </label>
            <input id={`${ids}-model`} className="field" value={v.model} maxLength={60} onChange={(e) => setV({ ...v, model: e.target.value })} />
          </div>
        </div>
        <div className="grid gap-3 sm:grid-cols-[8rem_minmax(0,1fr)]">
          <div>
            <label htmlFor={`${ids}-drop`} className="mb-1.5 block text-sm font-medium text-ink-2">
              {t('shoes.section.dropMm')}
            </label>
            <input id={`${ids}-drop`} className="field num" type="number" inputMode="decimal" min={0} max={20} step={0.5} value={drop} onChange={(e) => setDrop(e.target.value)} />
          </div>
          <div className="min-w-0">
            <p className="mb-1.5 text-sm font-medium text-ink-2">{t('shoes.section.type')}</p>
            <SegmentedControl<ShoeType>
              ariaLabel={t('shoes.section.typeLabel')}
              value={v.type}
              onChange={(type) => setV({ ...v, type })}
              options={(Object.keys(SHOE_TYPE_LABEL) as ShoeType[]).map((k) => ({ value: k, label: SHOE_TYPE_LABEL[k] }))}
            />
          </div>
        </div>
        <div>
          <label htmlFor={`${ids}-note`} className="mb-1.5 block text-sm font-medium text-ink-2">
            {t('shoes.section.note')}
          </label>
          <textarea id={`${ids}-note`} className="field min-h-20 resize-y" value={v.note} maxLength={300} onChange={(e) => setV({ ...v, note: e.target.value })} />
        </div>
        <Checkbox checked={v.retired} onChange={(retired) => setV({ ...v, retired })} label={t('shoes.section.retiredPair')} description={t('shoes.section.retiredPairHint')} />
        {error && <InlineError>{error}</InlineError>}
      </form>
    </Dialog>
  )
}
