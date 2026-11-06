import { Activity, HeartCrack, Loader2, Pencil, Plus, Stethoscope, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'wouter'
import { useActivities, useBodyMap, useBodyMapMutations, useProfile } from '../../api/queries'
import type { BodySide, BodyStressEntry, BodyStressInput, BodyZone } from '../../api/types'
import { BodyAnatomySvg } from '../../components/bodymap/BodyAnatomySvg'
import { BodyCorrelationList } from '../../components/bodymap/BodyCorrelationList'
import {
  ALL_BODY_ZONES,
  BODY_SIDE_LABEL,
  BODY_ZONE_META,
  TREND_META,
  effectiveZoneScore,
  painScoreTone,
} from '../../components/bodymap/bodyMapMeta'
import { PainScale } from '../../components/feeling/PainScale'
import { Card } from '../../components/ui/Card'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { Dialog } from '../../components/ui/Dialog'
import { Field } from '../../components/ui/Field'
import { EmptyState, InlineError, PageHeader } from '../../components/ui/Misc'
import { SegmentedControl } from '../../components/ui/SegmentedControl'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { cn } from '../../lib/cn'
import { capitalize, fmtDateLong, fmtDateShort, fmtNumber, todayISO } from '../../lib/format'

type WindowDays = 30 | 90 | 180

interface EntryDrawerProps {
  open: boolean
  entry: BodyStressEntry | null
  initialZone: BodyZone
  initialSide: BodySide
  today: string
  onClose: () => void
}

function BodyEntryDrawer({ open, entry, initialZone, initialSide, today, onClose }: EntryDrawerProps) {
  const mut = useBodyMapMutations()
  const activities = useActivities({}, open)
  const { toast } = useToast()

  const recentItems = useMemo(
    () => (activities.data?.pages.flatMap((p) => p.items) ?? []).slice(0, 30),
    [activities.data],
  )

  const [date, setDate] = useState(entry?.date ?? today)
  const [zone, setZone] = useState<BodyZone>(entry?.zone ?? initialZone)
  const [side, setSide] = useState<BodySide>(entry?.side ?? initialSide)
  const [painScore, setPainScore] = useState<number | null>(entry?.painScore ?? 2)
  const [coldNextDayScore, setColdNextDayScore] = useState<number | null>(entry?.coldNextDayScore ?? null)
  const [activityId, setActivityId] = useState<number | null>(entry?.activityId ?? null)
  const [note, setNote] = useState(entry?.note ?? '')
  const [errMsg, setErrMsg] = useState<string | null>(null)

  const busy = mut.createEntry.isPending || mut.updateEntry.isPending

  const submit = () => {
    if (!date) return setErrMsg('Indique une date.')
    const score = painScore ?? 0
    if (score < 0 || score > 10) return setErrMsg('Douleur entre 0 et 10.')
    setErrMsg(null)

    const payload: BodyStressInput = {
      date,
      zone,
      side,
      painScore: score,
      coldNextDayScore,
      activityId,
      note: note.trim(),
    }

    if (entry) {
      mut.updateEntry.mutate(
        { id: entry.id, ...payload },
        {
          onSuccess: () => {
            toast({ title: 'Entrée corporelle mise à jour', tone: 'success' })
            onClose()
          },
          onError: (e) => setErrMsg(e.message),
        },
      )
    } else {
      mut.createEntry.mutate(payload, {
        onSuccess: () => {
          toast({ title: 'Signal corporel enregistré', description: BODY_ZONE_META[zone].label, tone: 'success' })
          onClose()
        },
        onError: (e) => setErrMsg(e.message),
      })
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      variant="drawer"
      title={entry ? 'Modifier le signal corporel' : 'Enregistrer un signal corporel'}
      description="Documente la gêne à chaud et le ressenti à froid le lendemain matin (J+1) pour affiner les corrélations."
      footer={
        <div className="flex justify-end gap-2 pb-2">
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
      <div className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Date">
            {(p) => <input {...p} type="date" className="field num" value={date} max={today} onChange={(e) => setDate(e.target.value)} />}
          </Field>
          <Field label="Zone anatomique">
            {(p) => (
              <select {...p} className="field" value={zone} onChange={(e) => setZone(e.target.value as BodyZone)}>
                {ALL_BODY_ZONES.map((z) => (
                  <option key={z} value={z}>
                    {BODY_ZONE_META[z].label}
                  </option>
                ))}
              </select>
            )}
          </Field>
        </div>

        <div>
          <p className="mb-1.5 text-sm font-medium text-ink-2">Côté concerné</p>
          <SegmentedControl<BodySide>
            ariaLabel="Côté anatomique"
            value={side}
            onChange={setSide}
            options={[
              { value: 'left', label: 'Gauche' },
              { value: 'right', label: 'Droite' },
              { value: 'both', label: 'Bilatéral' },
            ]}
          />
        </div>

        <PainScale
          label="Douleur ressentie"
          hint="Pendant la sortie ou au cours de la journée (0 = aucune, 10 = maximale)"
          value={painScore}
          onChange={(v) => setPainScore(v ?? 0)}
        />

        <PainScale
          label="Ressenti à froid J+1"
          hint="Raideur ou douleur aux premiers pas le lendemain matin (facultatif)"
          value={coldNextDayScore}
          onChange={setColdNextDayScore}
        />

        <Field label="Séance associée (facultatif)">
          {(p) => (
            <select
              {...p}
              className="field"
              value={activityId ?? ''}
              onChange={(e) => setActivityId(e.target.value ? Number(e.target.value) : null)}
            >
              <option value="">Aucune séance liée</option>
              {recentItems.map((a) => (
                <option key={a.id} value={a.id}>
                  {fmtDateShort(a.date)} · {a.name}
                </option>
              ))}
            </select>
          )}
        </Field>

        <Field label="Note clinique (facultatif)">
          {(p) => (
            <textarea
              {...p}
              className="field min-h-20 resize-y"
              value={note}
              onChange={(e) => setNote(e.target.value)}
              maxLength={800}
              placeholder="Ex. tiraillement dans les escaliers en descente, disparaît après 10 min d’échauffement"
            />
          )}
        </Field>

        {errMsg && <InlineError>{errMsg}</InlineError>}
      </div>
    </Dialog>
  )
}

export default function BodyMapView() {
  useDocumentTitle('Cartographie corporelle')
  const profile = useProfile()
  const today = todayISO(profile.data?.timezone)
  const [params, setParams] = useSearchParams()
  const [windowDays, setWindowDays] = useState<WindowDays>(90)
  const [zoneFilter, setZoneFilter] = useState<BodyZone | null>(null)
  const [drawer, setDrawer] = useState<{ open: boolean; entry: BodyStressEntry | null; zone: BodyZone; side: BodySide }>({
    open: false,
    entry: null,
    zone: 'knee_pfps',
    side: 'left',
  })
  const [deleteTarget, setDeleteTarget] = useState<BodyStressEntry | null>(null)

  const mapQuery = useBodyMap(windowDays, null)
  const mut = useBodyMapMutations()
  const { toast } = useToast()

  useEffect(() => {
    if (params.get('new') === '1') {
      const defZone: BodyZone =
        profile.data?.focusArea && ALL_BODY_ZONES.includes(profile.data.focusArea as BodyZone)
          ? (profile.data.focusArea as BodyZone)
          : 'knee_pfps'
      const defSide: BodySide = profile.data?.injuredSide ?? 'left'
      setDrawer({ open: true, entry: null, zone: defZone, side: defSide })
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev)
          next.delete('new')
          return next
        },
        { replace: true },
      )
    }
  }, [params, profile.data?.focusArea, profile.data?.injuredSide, setParams])

  const data = mapQuery.data
  const zones = data?.zones ?? []
  const filteredEntries = useMemo(() => {
    const all = data?.entries ?? []
    return zoneFilter ? all.filter((e) => e.zone === zoneFilter) : all
  }, [data?.entries, zoneFilter])

  const filteredCorrelations = useMemo(() => {
    const all = data?.correlations ?? []
    return zoneFilter ? all.filter((c) => c.zone === zoneFilter) : all
  }, [data?.correlations, zoneFilter])

  const openNewForZone = (zone?: BodyZone, side?: BodySide) => {
    const z =
      zone ??
      zoneFilter ??
      (profile.data?.focusArea && ALL_BODY_ZONES.includes(profile.data.focusArea as BodyZone)
        ? (profile.data.focusArea as BodyZone)
        : 'knee_pfps')
    setDrawer({ open: true, entry: null, zone: z, side: side ?? profile.data?.injuredSide ?? 'left' })
  }

  return (
    <div className="animate-in space-y-5">
      <PageHeader
        title="Cartographie corporelle"
        subtitle="Suis tes zones de tension mécanique, le ressenti à froid J+1 et les facteurs de charge ou de chaussage associés."
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <SegmentedControl<string>
              ariaLabel="Fenêtre d’analyse"
              size="sm"
              value={String(windowDays)}
              onChange={(v) => setWindowDays(Number(v) as WindowDays)}
              options={[
                { value: '30', label: '30 j' },
                { value: '90', label: '90 j' },
                { value: '180', label: '180 j' },
              ]}
            />
            <button type="button" className="btn btn-primary" onClick={() => openNewForZone()}>
              <Plus className="size-4" aria-hidden="true" /> Signaler une gêne
            </button>
          </div>
        }
      />

      {mapQuery.isError && !data ? (
        <InlineError>{mapQuery.error.message}</InlineError>
      ) : !data ? (
        <div className="grid gap-5 lg:grid-cols-12">
          <Skeleton className="h-[28rem] lg:col-span-6" />
          <Skeleton className="h-[28rem] lg:col-span-6" />
        </div>
      ) : (
        <>
          <div className="grid gap-5 lg:grid-cols-12">
            <Card
              className="lg:col-span-7"
              icon={<Stethoscope className="size-4" />}
              title="Zones anatomiques"
              subtitle="Clique une région pour filtrer l’historique ou enregistrer un signal"
              action={
                zoneFilter && (
                  <button type="button" className="btn btn-ghost h-8 min-h-8 px-2.5 text-xs" onClick={() => setZoneFilter(null)}>
                    Toutes les zones
                  </button>
                )
              }
            >
              <BodyAnatomySvg
                zones={zones}
                selectedZone={zoneFilter}
                onSelectZone={(z, sideHint) => {
                  if (sideHint) {
                    setZoneFilter(z)
                    openNewForZone(z, sideHint)
                    return
                  }
                  setZoneFilter((cur) => (cur === z ? null : z))
                }}
              />
            </Card>

            <Card
              className="lg:col-span-5"
              icon={<Activity className="size-4" />}
              title="Synthèse par région"
              subtitle={`État actuel et tendance sur ${data.windowDays} jours`}
            >
              <ul className="divide-y divide-line">
                {ALL_BODY_ZONES.map((zoneKey) => {
                  const z = zones.find((x) => x.zone === zoneKey)
                  const meta = BODY_ZONE_META[zoneKey]
                  const score = effectiveZoneScore(z)
                  const tone = painScoreTone(score)
                  const trend = TREND_META[z?.trend ?? 'none']
                  const selected = zoneFilter === zoneKey
                  return (
                    <li key={zoneKey} className="py-2.5 first:pt-0 last:pb-0">
                      <div className="flex items-start justify-between gap-2">
                        <button
                          type="button"
                          onClick={() => setZoneFilter((cur) => (cur === zoneKey ? null : zoneKey))}
                          className={cn(
                            'min-w-0 flex-1 text-left transition-colors hover:text-accent',
                            selected && 'text-accent',
                          )}
                        >
                          <span className="block truncate text-sm font-semibold">{z?.zoneLabel ?? meta.label}</span>
                          <span className="num mt-0.5 block text-xs text-ink-3">
                            {z?.latestDate
                              ? `Dernier : ${fmtDateShort(z.latestDate)}${z.latestSide ? ` (${BODY_SIDE_LABEL[z.latestSide]})` : ''} · ${z.entriesCount90d} ép. / 90 j`
                              : 'Aucun épisode enregistré'}
                          </span>
                        </button>
                        <div className="flex flex-none items-center gap-1.5">
                          {z && z.latestScore !== null && (
                            <span className={cn('chip num h-6 border px-2 text-xs', tone.badgeCls)}>
                              {fmtNumber(z.latestScore, 1, { trim: true })}/10
                              {z.latestColdNextDayScore !== null && ` · J+1 ${fmtNumber(z.latestColdNextDayScore, 1, { trim: true })}`}
                            </span>
                          )}
                          <span className={cn('chip h-6 border px-2 text-[10px]', trend.cls)}>{trend.label}</span>
                          <button
                            type="button"
                            className="icon-btn size-8"
                            onClick={() => openNewForZone(zoneKey, z?.latestSide ?? undefined)}
                            aria-label={`Ajouter une entrée pour ${meta.label}`}
                          >
                            <Plus className="size-3.5" />
                          </button>
                        </div>
                      </div>
                    </li>
                  )
                })}
              </ul>
            </Card>
          </div>

          <div className="grid gap-5 lg:grid-cols-12">
            <Card
              className="lg:col-span-6"
              icon={<Activity className="size-4" />}
              title="Corrélations biomécaniques & chaussage"
              subtitle={zoneFilter ? `Facteurs observés sur : ${BODY_ZONE_META[zoneFilter].label}` : `Comparaison des sorties avec gêne vs référence (${windowDays} j)`}
            >
              <BodyCorrelationList correlations={filteredCorrelations} />
            </Card>

            <Card
              className="lg:col-span-6"
              icon={<HeartCrack className="size-4" />}
              title="Journal des signaux corporels"
              subtitle={zoneFilter ? `${filteredEntries.length} entrée(s) · ${BODY_ZONE_META[zoneFilter].label}` : `${filteredEntries.length} entrée(s) sur ${windowDays} jours`}
            >
              {filteredEntries.length === 0 ? (
                <EmptyState
                  icon={<Stethoscope />}
                  title="Aucun signal sur cette période"
                  action={
                    <button type="button" className="btn btn-primary" onClick={() => openNewForZone()}>
                      <Plus className="size-4" aria-hidden="true" /> Enregistrer une gêne
                    </button>
                  }
                >
                  Note tes gênes articulaires ou tendineuses même légères (1–3/10) ainsi que la réaction à froid J+1 pour suivre l’adaptation tissulaire.
                </EmptyState>
              ) : (
                <ul className="divide-y divide-line">
                  {filteredEntries.map((e) => {
                    const tone = painScoreTone(Math.max(e.painScore, e.coldNextDayScore ?? 0))
                    return (
                      <li key={e.id} className="flex items-start justify-between gap-3 py-3 first:pt-0 last:pb-0">
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="font-semibold text-ink">{e.zoneLabel || BODY_ZONE_META[e.zone].label}</span>
                            <span className="chip h-5 border border-line bg-track px-2 text-[11px] text-ink-2">
                              {BODY_SIDE_LABEL[e.side]}
                            </span>
                            <span className={cn('chip num h-5 border px-2 text-[11px]', tone.badgeCls)}>
                              Effort {fmtNumber(e.painScore, 1, { trim: true })}/10
                              {e.coldNextDayScore !== null && ` · J+1 ${fmtNumber(e.coldNextDayScore, 1, { trim: true })}/10`}
                            </span>
                          </div>
                          <p className="num mt-0.5 text-xs text-ink-3">
                            {capitalize(fmtDateLong(e.date))}
                            {e.activityId && e.activityName && (
                              <>
                                {' · '}
                                <Link href={`/activities/${e.activityId}`} className="text-accent underline-offset-2 hover:underline">
                                  {e.activityName}
                                </Link>
                              </>
                            )}
                          </p>
                          {e.note && <p className="mt-1 text-sm text-ink-2">{e.note}</p>}
                        </div>
                        <div className="flex flex-none items-center gap-1">
                          <button
                            type="button"
                            className="icon-btn size-8"
                            onClick={() => setDrawer({ open: true, entry: e, zone: e.zone, side: e.side })}
                            aria-label={`Modifier le signal du ${fmtDateShort(e.date)}`}
                          >
                            <Pencil className="size-3.5" />
                          </button>
                          <button
                            type="button"
                            className="icon-btn size-8"
                            onClick={() => setDeleteTarget(e)}
                            aria-label={`Supprimer le signal du ${fmtDateShort(e.date)}`}
                          >
                            <Trash2 className="size-3.5" />
                          </button>
                        </div>
                      </li>
                    )
                  })}
                </ul>
              )}
            </Card>
          </div>
        </>
      )}

      {drawer.open && (
        <BodyEntryDrawer
          key={drawer.entry ? `edit-${drawer.entry.id}` : `new-${drawer.zone}-${drawer.side}`}
          open
          entry={drawer.entry}
          initialZone={drawer.zone}
          initialSide={drawer.side}
          today={today}
          onClose={() => setDrawer((d) => ({ ...d, open: false, entry: null }))}
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        title="Supprimer cette entrée corporelle ?"
        message={
          deleteTarget && (
            <p>
              Le signal « {deleteTarget.zoneLabel || BODY_ZONE_META[deleteTarget.zone].label} » du {fmtDateLong(deleteTarget.date)} sera retiré de l’historique et du calcul des corrélations.
            </p>
          )
        }
        confirmLabel="Supprimer"
        danger
        busy={mut.removeEntry.isPending}
        onConfirm={() =>
          deleteTarget &&
          mut.removeEntry.mutate(deleteTarget.id, {
            onSuccess: () => {
              toast({ title: 'Signal corporel supprimé' })
              setDeleteTarget(null)
            },
            onError: (e) => toast({ tone: 'error', title: 'Suppression impossible', description: e.message }),
          })
        }
      />
    </div>
  )
}
