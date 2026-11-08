import { Activity, HeartCrack, Loader2, Pencil, Plus, Stethoscope, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'wouter'
import { useHealthStatus } from '../../api/queriesV41'
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
import { HealthStatusCard } from '../../components/health/HealthStatusCard'
import { primaryZone } from '../../components/health/healthStatus'
import { ShareLinksPanel } from '../../components/share/ShareLinksPanel'
import { ShareNotesCard } from '../../components/share/ShareNotesCard'
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
import { t } from '../../i18n'

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
    if (!date) return setErrMsg(t('health.view.dateRequired'))
    const score = painScore ?? 0
    if (score < 0 || score > 10) return setErrMsg(t('health.view.painRange'))
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
            toast({ title: t('health.view.entryUpdated'), tone: 'success' })
            onClose()
          },
          onError: (e) => setErrMsg(e.message),
        },
      )
    } else {
      mut.createEntry.mutate(payload, {
        onSuccess: () => {
          toast({ title: t('health.view.entrySaved'), description: BODY_ZONE_META[zone].label, tone: 'success' })
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
      title={entry ? t('health.view.editTitle') : t('health.view.newTitle')}
      description={t('health.view.formHint')}
      footer={
        <div className="flex justify-end gap-2 pb-2">
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
      <div className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t('health.view.date')}>
            {(p) => <input {...p} type="date" className="field num" value={date} max={today} onChange={(e) => setDate(e.target.value)} />}
          </Field>
          <Field label={t('health.view.zone')}>
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
          <p className="mb-1.5 text-sm font-medium text-ink-2">{t('health.view.side')}</p>
          <SegmentedControl<BodySide>
            ariaLabel={t('health.view.sideAria')}
            value={side}
            onChange={setSide}
            options={[
              { value: 'left', label: BODY_SIDE_LABEL.left },
              { value: 'right', label: BODY_SIDE_LABEL.right },
              { value: 'both', label: BODY_SIDE_LABEL.both },
            ]}
          />
        </div>

        <PainScale
          label={t('health.view.pain')}
          hint={t('health.view.painHint')}
          value={painScore}
          onChange={(v) => setPainScore(v ?? 0)}
        />

        <PainScale
          label={t('health.view.cold')}
          hint={t('health.view.coldHint')}
          value={coldNextDayScore}
          onChange={setColdNextDayScore}
        />

        <Field label={t('health.view.session')}>
          {(p) => (
            <select
              {...p}
              className="field"
              value={activityId ?? ''}
              onChange={(e) => setActivityId(e.target.value ? Number(e.target.value) : null)}
            >
              <option value="">{t('health.view.noSession')}</option>
              {recentItems.map((a) => (
                <option key={a.id} value={a.id}>
                  {fmtDateShort(a.date)} · {a.name}
                </option>
              ))}
            </select>
          )}
        </Field>

        <Field label={t('health.view.note')}>
          {(p) => (
            <textarea
              {...p}
              className="field min-h-20 resize-y"
              value={note}
              onChange={(e) => setNote(e.target.value)}
              maxLength={800}
              placeholder={t('health.view.notePlaceholder')}
            />
          )}
        </Field>

        {errMsg && <InlineError>{errMsg}</InlineError>}
      </div>
    </Dialog>
  )
}

export default function HealthView() {
  useDocumentTitle(t('health.view.title'))
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

  const health = useHealthStatus()
  const defaults = primaryZone(health.data)
  const mapQuery = useBodyMap(windowDays, null)
  const mut = useBodyMapMutations()
  const { toast } = useToast()

  useEffect(() => {
    if (params.get('new') === '1') {
      setDrawer({ open: true, entry: null, zone: defaults.zone, side: defaults.side })
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev)
          next.delete('new')
          return next
        },
        { replace: true },
      )
    }
  }, [params, defaults.zone, defaults.side, setParams])

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
    setDrawer({ open: true, entry: null, zone: zone ?? zoneFilter ?? defaults.zone, side: side ?? defaults.side })
  }


  return (
    <div className="animate-in space-y-5">
      <PageHeader
        title={t('health.view.title')}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <SegmentedControl<string>
              ariaLabel={t('health.view.window')}
              size="sm"
              value={String(windowDays)}
              onChange={(v) => setWindowDays(Number(v) as WindowDays)}
              options={[
                { value: '30', label: t('health.view.days', { n: 30 }) },
                { value: '90', label: t('health.view.days', { n: 90 }) },
                { value: '180', label: t('health.view.days', { n: 180 }) },
              ]}
            />
            <button type="button" className="btn btn-primary" onClick={() => openNewForZone()}>
              <Plus className="size-4" aria-hidden="true" /> {t('health.view.report')}
            </button>
          </div>
        }
      />

      <HealthStatusCard today={today} />

      <ShareNotesCard today={today} />

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
              title={t('health.view.zonesTitle')}
              subtitle={t('health.view.zonesSubtitle')}
              action={
                zoneFilter && (
                  <button type="button" className="btn btn-ghost h-8 min-h-8 px-2.5 text-xs" onClick={() => setZoneFilter(null)}>
                    {t('health.view.allZones')}
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
              title={t('health.view.summaryTitle')}
              subtitle={t('health.view.summarySubtitle', { days: data.windowDays })}
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
                          <span className="block truncate text-sm font-semibold">{meta.label}</span>
                          <span className="num mt-0.5 block text-xs text-ink-3">
                            {z?.latestDate
                              ? t('health.view.last', { date: fmtDateShort(z.latestDate), side: z.latestSide ? ` (${BODY_SIDE_LABEL[z.latestSide]})` : '', count: z.entriesCount90d })
                              : t('health.view.noEpisode')}
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
                            aria-label={t('health.view.addFor', { zone: meta.label })}
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
              title={t('health.view.corrTitle')}
              subtitle={zoneFilter ? t('health.view.corrZone', { zone: BODY_ZONE_META[zoneFilter].label }) : t('health.view.corrAll', { days: windowDays })}
            >
              <BodyCorrelationList correlations={filteredCorrelations} />
            </Card>

            <Card
              className="lg:col-span-6"
              icon={<HeartCrack className="size-4" />}
              title={t('health.view.journal')}
              subtitle={zoneFilter ? t('health.view.journalZone', { count: filteredEntries.length, zone: BODY_ZONE_META[zoneFilter].label }) : t('health.view.journalAll', { count: filteredEntries.length, days: windowDays })}
            >
              {filteredEntries.length === 0 ? (
                <EmptyState
                  icon={<Stethoscope />}
                  title={t('health.view.emptyTitle')}
                  action={
                    <button type="button" className="btn btn-primary" onClick={() => openNewForZone()}>
                      <Plus className="size-4" aria-hidden="true" /> {t('health.view.logPain')}
                    </button>
                  }
                >
                  {t('health.view.emptyHint')}
                </EmptyState>
              ) : (
                <ul className="divide-y divide-line">
                  {filteredEntries.map((e) => {
                    const tone = painScoreTone(Math.max(e.painScore, e.coldNextDayScore ?? 0))
                    return (
                      <li key={e.id} className="flex items-start justify-between gap-3 py-3 first:pt-0 last:pb-0">
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="font-semibold text-ink">{BODY_ZONE_META[e.zone]?.label ?? e.zoneLabel}</span>
                            <span className="chip h-5 border border-line bg-track px-2 text-[11px] text-ink-2">
                              {BODY_SIDE_LABEL[e.side]}
                            </span>
                            <span className={cn('chip num h-5 border px-2 text-[11px]', tone.badgeCls)}>
                              {t('health.view.effort', { value: fmtNumber(e.painScore, 1, { trim: true }) })}
                              {e.coldNextDayScore !== null && t('health.view.nextDay', { value: fmtNumber(e.coldNextDayScore, 1, { trim: true }) })}
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
                            aria-label={t('health.view.editEntry', { date: fmtDateShort(e.date) })}
                          >
                            <Pencil className="size-3.5" />
                          </button>
                          <button
                            type="button"
                            className="icon-btn size-8"
                            onClick={() => setDeleteTarget(e)}
                            aria-label={t('health.view.deleteEntry', { date: fmtDateShort(e.date) })}
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

      <ShareLinksPanel />

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        title={t('health.view.deleteTitle')}
        message={
          deleteTarget && (
            <p>
              {t('health.view.deleteBody', { zone: BODY_ZONE_META[deleteTarget.zone]?.label ?? deleteTarget.zoneLabel, date: fmtDateLong(deleteTarget.date) })}
            </p>
          )
        }
        confirmLabel={t('common.actions.delete')}
        danger
        busy={mut.removeEntry.isPending}
        onConfirm={() =>
          deleteTarget &&
          mut.removeEntry.mutate(deleteTarget.id, {
            onSuccess: () => {
              toast({ title: t('health.view.deleted') })
              setDeleteTarget(null)
            },
            onError: (e) => toast({ tone: 'error', title: t('health.view.deleteFailed'), description: e.message }),
          })
        }
      />
    </div>
  )
}
