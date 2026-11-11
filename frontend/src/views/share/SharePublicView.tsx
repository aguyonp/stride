import { CalendarRange, Eye, Footprints, Gauge, Link2Off, Moon, PenLine, RotateCw, Stethoscope, Sun, Trophy } from 'lucide-react'
import { isApiError } from '../../api/client'
import { usePublicShareView } from '../../api/queries'
import type { PersonalRecord, PublicLoadPoint, PublicShareView } from '../../api/types'
import { BodyAnatomySvg } from '../../components/bodymap/BodyAnatomySvg'
import { BodyCorrelationList } from '../../components/bodymap/BodyCorrelationList'
import { BODY_SIDE_LABEL, BODY_ZONE_META, effectiveZoneScore, painScoreTone, TREND_META } from '../../components/bodymap/bodyMapMeta'
import { Ambient } from '../../components/layout/Ambient'
import { PHASE_TYPE_META } from '../../components/plan/cycles/cycleMeta'
import { Card } from '../../components/ui/Card'
import { StrideMark } from '../../components/ui/icons'
import { Skeleton } from '../../components/ui/Skeleton'
import { Insufficient } from '../../components/ui/Value'
import { LanguageToggle } from '../../components/ui/LanguageToggle'
import { recordLabel } from '../../components/dashboard/RecordsCard'
import { ReadinessBanner } from '../../components/v3/ReadinessBanner'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { useMediaQuery } from '../../hooks/useMediaQuery'
import { useTheme } from '../../hooks/useTheme'
import { cn } from '../../lib/cn'
import { fmtDateShort, fmtDateTime, fmtDayMonth, fmtDuration, fmtKm, fmtNumber, fmtSignedPct, isNum, ND, todayISO } from '../../lib/format'
import { ACWR_VERDICT, SHOE_TYPE_LABEL } from '../../lib/labels'
import { PortalCalendar } from './portal/PortalCalendar'
import { PortalSessions } from './portal/PortalSessions'
import { PortalSummary } from './portal/PortalSummary'
import { permissionsOf } from './portal/portalUtils'
import { t } from '../../i18n'

function LoadTrendSvg({ series }: { series: PublicLoadPoint[] }) {
  if (series.length < 2) {
    return (
      <p className="py-6 text-center text-sm text-ink-3">
        {t('common.misc.noLoadHistory')}
      </p>
    )
  }

  const width = 760
  const height = 210
  const padL = 40
  const padR = 40
  const padT = 18
  const padB = 28
  const plotW = width - padL - padR
  const plotH = height - padT - padB

  const maxLoad = Math.max(
    10,
    ...series.map((p) => Math.max(p.acute, p.chronic)),
  )
  const acwrMax = 2.0

  const xAt = (idx: number) => padL + (idx / (series.length - 1)) * plotW
  const yLoad = (val: number) => padT + plotH - Math.min(1, Math.max(0, val / (maxLoad * 1.1))) * plotH
  const yAcwr = (val: number) => padT + plotH - Math.min(1, Math.max(0, val / acwrMax)) * plotH

  const chronicPath = series
    .map((p, i) => `${i === 0 ? 'M' : 'L'}${xAt(i).toFixed(1)},${yLoad(p.chronic).toFixed(1)}`)
    .join(' ')
  const chronicArea = `${chronicPath} L${xAt(series.length - 1).toFixed(1)},${(padT + plotH).toFixed(1)} L${xAt(0).toFixed(1)},${(padT + plotH).toFixed(1)} Z`
  const acutePath = series
    .map((p, i) => `${i === 0 ? 'M' : 'L'}${xAt(i).toFixed(1)},${yLoad(p.acute).toFixed(1)}`)
    .join(' ')

  const acwrPoints = series
    .map((p, i) => (isNum(p.acwr) ? { x: xAt(i), y: yAcwr(p.acwr) } : null))
    .filter((p): p is { x: number; y: number } => p !== null)
  const acwrPath = acwrPoints.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ')

  const bandTop = yAcwr(1.3)
  const bandBottom = yAcwr(0.8)

  const tickIndices = [0, Math.floor((series.length - 1) / 3), Math.floor(((series.length - 1) * 2) / 3), series.length - 1]
    .filter((v, idx, arr) => arr.indexOf(v) === idx)

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-ink-3">
        <div className="flex flex-wrap items-center gap-3">
          <span className="inline-flex items-center gap-1.5">
            <span className="size-2.5 rounded-full bg-accent" aria-hidden="true" />
            {t('share.public.acute')}
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="size-2.5 rounded-full bg-info" aria-hidden="true" />
            {t('share.public.chronic')}
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="size-2.5 rounded-full bg-ok" aria-hidden="true" />
            {t('share.public.acwrLegend')}
          </span>
        </div>
        <span className="num">{t('share.public.days', { count: series.length })}</span>
      </div>

      <svg
        viewBox={`0 0 ${width} ${height}`}
        className="h-auto w-full select-none"
        role="img"
        aria-label={t('share.public.trendAria')}
      >
        <rect
          x={padL}
          y={bandTop}
          width={plotW}
          height={Math.max(2, bandBottom - bandTop)}
          fill="color-mix(in oklab, var(--ok) 11%, transparent)"
        />
        {[0.25, 0.5, 0.75, 1].map((f) => {
          const y = padT + plotH * (1 - f)
          return (
            <line
              key={f}
              x1={padL}
              x2={width - padR}
              y1={y}
              y2={y}
              stroke="var(--line)"
              strokeDasharray="3 3"
            />
          )
        })}
        <line x1={padL} x2={width - padR} y1={padT + plotH} y2={padT + plotH} stroke="var(--line-strong)" />

        <path d={chronicArea} fill="color-mix(in oklab, var(--info) 14%, transparent)" />
        <path d={chronicPath} fill="none" stroke="var(--info)" strokeWidth="2" strokeLinecap="round" />
        <path d={acutePath} fill="none" stroke="var(--accent)" strokeWidth="2.2" strokeLinecap="round" />
        {acwrPath && (
          <path
            d={acwrPath}
            fill="none"
            stroke="var(--ok)"
            strokeWidth="1.75"
            strokeDasharray="4 3"
            strokeLinecap="round"
          />
        )}

        <text x={padL - 6} y={padT + 8} textAnchor="end" className="fill-ink-3 text-[10px]">
          {fmtNumber(Math.round(maxLoad * 1.1))}
        </text>
        <text x={padL - 6} y={padT + plotH} textAnchor="end" className="fill-ink-3 text-[10px]">
          0
        </text>
        <text x={width - padR + 6} y={bandTop + 4} textAnchor="start" className="fill-ok text-[10px]">
          1,3
        </text>
        <text x={width - padR + 6} y={bandBottom + 4} textAnchor="start" className="fill-ok text-[10px]">
          0,8
        </text>

        {tickIndices.map((idx) => (
          <text
            key={idx}
            x={xAt(idx)}
            y={height - 6}
            textAnchor={idx === 0 ? 'start' : idx === series.length - 1 ? 'end' : 'middle'}
            className="fill-ink-3 text-[10px]"
          >
            {fmtDayMonth(series[idx].date)}
          </text>
        ))}
      </svg>
    </div>
  )
}

function DualCompletionBar({
  label,
  actual,
  planned,
  unit,
  decimals = 0,
}: {
  label: string
  actual: number
  planned: number
  unit: string
  decimals?: number
}) {
  const pct = planned > 0 ? Math.min(140, Math.round((actual / planned) * 100)) : 0
  return (
    <div className="space-y-1.5">
      <div className="flex items-baseline justify-between gap-2 text-xs">
        <span className="font-medium text-ink-2">{label}</span>
        <span className="num text-ink">
          <strong className="font-semibold">{fmtNumber(actual, decimals, { trim: true })}</strong>
          <span className="text-ink-3">
            {' '}
            / {fmtNumber(planned, decimals, { trim: true })} {unit}
          </span>
          {planned > 0 && <span className="ml-1.5 text-ink-3">({pct} %)</span>}
        </span>
      </div>
      <div className="h-2 overflow-hidden rounded-full bg-track">
        <div
          className="h-full rounded-full bg-accent transition-all"
          style={{ width: `${Math.min(100, pct)}%` }}
        />
      </div>
    </div>
  )
}

function PublicHeader({ data }: { data: PublicShareView | null }) {
  const { theme, toggle } = useTheme()
  const p = data ? permissionsOf(data) : null
  const writable = Boolean(p && (p.comments || p.planning))
  return (
    <header className="app-header">
      <div className="mx-auto flex h-14 max-w-[1440px] items-center justify-between gap-3 px-4 sm:px-6 lg:h-16 lg:px-8">
        <div className="flex min-w-0 items-center gap-2.5">
          <span
            className="grid size-9 flex-none place-items-center rounded-xl bg-accent/15 text-accent ring-1 ring-accent/20 ring-inset"
            aria-hidden="true"
          >
            <StrideMark className="w-6" />
          </span>
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold tracking-tight text-ink sm:text-base">
              {t('share.public.brand')}
            </p>
            {data && (
              <p className="num truncate text-[11px] text-ink-3">
                {data.shareLabel}
                {data.expiresAt ? t('share.public.expires', { date: fmtDateTime(data.expiresAt) }) : t('share.public.noExpiry')}
              </p>
            )}
          </div>
        </div>

        <div className="flex flex-none items-center gap-2">
          {writable ? (
            <span className="chip h-7 border border-accent/35 bg-accent/12 px-2.5 text-xs font-medium text-ink">
              <PenLine className="size-3.5 text-accent" aria-hidden="true" />
              <span className="max-sm:sr-only">{t('share.public.annotations')}</span>
            </span>
          ) : (
            <span className="chip h-7 border border-info/35 bg-info/12 px-2.5 text-xs font-medium text-info">
              <Eye className="size-3.5" aria-hidden="true" />
              <span className="max-sm:sr-only">{t('share.public.readOnly')}</span>
            </span>
          )}
          <LanguageToggle />
          <button
            type="button"
            className="icon-btn"
            onClick={toggle}
            aria-label={theme === 'dark' ? t('layout.nav.lightTheme') : t('layout.nav.darkTheme')}
          >
            {theme === 'dark' ? <Sun className="size-[1.125rem]" /> : <Moon className="size-[1.125rem]" />}
          </button>
        </div>
      </div>
    </header>
  )
}

function ShareDossierContent({ data, token }: { data: PublicShareView; token: string }) {
  const { loadSummary, cycles, bodyMap } = data
  const shoes = data.shoes
  const phases = cycles?.phases ?? []
  const acwrMeta = loadSummary?.verdict ? ACWR_VERDICT[loadSummary.verdict] : null
  const activePlan = cycles?.activePlan
  const activeZones = (bodyMap?.zones ?? []).filter((z) => effectiveZoneScore(z) > 0 || z.entriesCount90d > 0)
  const inline = useMediaQuery('(min-width: 768px)')
  const today = todayISO()

  return (
    <div className="space-y-5 lg:space-y-6">
      <PortalSummary data={data} token={token} inline={inline} today={today} />

      <ReadinessBanner r={data.readiness} />

      <div className="grid gap-5 lg:grid-cols-12">
        {bodyMap && (
        <Card
          className="lg:col-span-6"
          title={t('share.public.bodyMap')}
          subtitle={t('share.public.bodyMapSubtitle', { days: bodyMap.windowDays })}
          icon={<Stethoscope />}
        >
          <BodyAnatomySvg zones={bodyMap.zones} readOnly />
          <div className="mt-4 space-y-2">
            {activeZones.length === 0 ? (
              <p className="text-sm text-ink-3">{t('share.public.noZone')}</p>
            ) : (
              <ul className="grid gap-2 sm:grid-cols-2" aria-label={t('share.public.zones')}>
                {activeZones.map((z) => {
                  const score = effectiveZoneScore(z)
                  const tone = painScoreTone(score)
                  const trend = TREND_META[z.trend]
                  return (
                    <li key={z.zone} className="glass-inset p-3 text-xs">
                      <div className="flex items-center justify-between gap-2">
                        <span className="font-semibold text-ink">{BODY_ZONE_META[z.zone].short}</span>
                        <span className={cn('chip num h-5 border px-2 text-[11px]', tone.badgeCls)}>
                          {fmtNumber(score, 1, { trim: true })}/10
                        </span>
                      </div>
                      <p className="num mt-1 text-ink-3">
                        {z.latestSide ? `${BODY_SIDE_LABEL[z.latestSide]} · ` : ''}
                        {z.latestColdNextDayScore !== null
                          ? t('share.public.cold', { value: fmtNumber(z.latestColdNextDayScore, 1, { trim: true }) })
                          : ''}
                        {t('share.public.reports', { count: z.entriesCount90d })}
                      </p>
                      <span className={cn('chip mt-2 h-5 border px-2 text-[11px]', trend.cls)}>{trend.label}</span>
                    </li>
                  )
                })}
              </ul>
            )}
          </div>
        </Card>
        )}
        {bodyMap && (
          <Card
            className="lg:col-span-6"
            title={t('share.public.correlations')}
            subtitle={t('share.public.correlationsSubtitle')}
            icon={<Footprints />}
          >
            <BodyCorrelationList correlations={bodyMap.correlations} />
          </Card>
        )}
      </div>

      <PortalCalendar data={data} token={token} inline={inline} today={today} />

      <PortalSessions data={data} token={token} inline={inline} />

      {loadSummary && (
      <Card
        title={t('share.public.loadTitle')}
        subtitle={t('share.public.loadSubtitle')}
        icon={<Gauge />}
      >
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
          <div className="glass-inset p-3">
            <dt className="text-xs text-ink-3">{t('share.public.acwr')}</dt>
            <dd className="mt-1 flex flex-wrap items-center gap-2">
              <span className="num text-lg font-bold text-ink">
                {isNum(loadSummary.acwr) ? fmtNumber(loadSummary.acwr, 2) : 'N/D'}
              </span>
              {acwrMeta && (
                <span
                  className={cn(
                    'chip h-5 border px-2 text-[11px]',
                    acwrMeta.level === 'ok'
                      ? 'border-ok/35 bg-ok/12 text-ok'
                      : acwrMeta.level === 'warn'
                        ? 'border-warn/35 bg-warn/12 text-warn'
                        : acwrMeta.level === 'danger'
                          ? 'border-danger/35 bg-danger/12 text-danger'
                          : 'border-info/35 bg-info/12 text-info',
                  )}
                >
                  {acwrMeta.label}
                </span>
              )}
            </dd>
          </div>

          <div className="glass-inset p-3">
            <dt className="text-xs text-ink-3">{t('share.public.acuteChronic')}</dt>
            <dd className="num mt-1 text-lg font-bold text-ink">
              {fmtNumber(loadSummary.acute, 0)}{' '}
              <span className="text-sm font-normal text-ink-3">/ {fmtNumber(loadSummary.chronic, 0)}</span>
            </dd>
          </div>

          <div className="glass-inset p-3">
            <dt className="text-xs text-ink-3">{t('share.public.ef')}</dt>
            <dd className="num mt-1 flex items-baseline gap-1.5 text-lg font-bold text-ink">
              {isNum(loadSummary.efLatest) ? <span>{fmtNumber(loadSummary.efLatest, 2)}</span> : <Insufficient />}
              {isNum(loadSummary.efLatest) && isNum(loadSummary.efTrendPct) && (
                <span className={cn('text-xs font-semibold', loadSummary.efTrendPct >= 0 ? 'text-ok' : 'text-warn')}>
                  {fmtSignedPct(loadSummary.efTrendPct, 1)}
                </span>
              )}
            </dd>
          </div>

          <div className="glass-inset p-3">
            <dt className="text-xs text-ink-3">{t('share.public.impact7d')}</dt>
            <dd className="num mt-1 text-lg font-bold text-ink">
              {isNum(loadSummary.impactLoad7d) ? `${fmtNumber(loadSummary.impactLoad7d, 0)} pts` : <Insufficient />}
            </dd>
          </div>

          <div className="glass-inset p-3">
            <dt className="text-xs text-ink-3">{t('share.public.joint7d')}</dt>
            <dd className="num mt-1 text-lg font-bold text-ink">
              {isNum(loadSummary.jointIndex7d) ? fmtNumber(loadSummary.jointIndex7d, 1) : <Insufficient />}
            </dd>
          </div>
        </dl>

        <div className="glass-inset mt-4 p-4">
          <LoadTrendSvg series={data.loadSeries ?? []} />
        </div>
      </Card>
      )}

      {cycles && (
      <Card
        title={t('share.public.cycleTitle')}
        subtitle={
          activePlan
            ? `${activePlan.name}${activePlan.targetEventLabel ? t('share.public.cycleGoal', { label: activePlan.targetEventLabel }) : ''}`
            : t('share.public.cycleDefault')
        }
        icon={<CalendarRange />}
      >
        {!activePlan && phases.length === 0 ? (
          <p className="text-sm text-ink-3">{t('share.public.noCycle')}</p>
        ) : (
          <div className="space-y-4">
            {activePlan && cycles.totals && (
              <div className="glass-inset grid gap-4 p-4 sm:grid-cols-3">
                <div>
                  <p className="text-xs text-ink-3">{t('share.public.cyclePeriod')}</p>
                  <p className="num mt-1 text-sm font-semibold text-ink">
                    {fmtDateShort(activePlan.startDate)} → {fmtDateShort(activePlan.endDate)}
                  </p>
                  <p className="num mt-1 text-xs text-ink-3">
                    {t('share.public.sessionsDone', { done: cycles.totals.sessionsCompleted, planned: cycles.totals.sessionsPlanned })}
                  </p>
                </div>
                <DualCompletionBar
                  label={t('share.public.cumVolume')}
                  actual={cycles.totals.actualVolumeKm}
                  planned={cycles.totals.plannedVolumeKm}
                  unit="km"
                  decimals={1}
                />
                <DualCompletionBar
                  label={t('share.public.cumLoad')}
                  actual={cycles.totals.actualLoad}
                  planned={cycles.totals.plannedLoad}
                  unit="pts"
                />
              </div>
            )}

            {phases.length > 0 && (
              <ul className="grid gap-2.5 sm:grid-cols-2 lg:grid-cols-3" aria-label={t('share.public.mesos')}>
                {phases.map((phase) => {
                  const meta = PHASE_TYPE_META[phase.phaseType]
                  return (
                    <li key={phase.id} className="glass-inset space-y-2 p-3.5">
                      <div className="flex items-center justify-between gap-2">
                        <h3 className="truncate text-sm font-semibold text-ink">{phase.name}</h3>
                        <span className={cn('chip h-5 border px-2 text-[11px]', meta.chipCls)}>{meta.label}</span>
                      </div>
                      <p className="num text-xs text-ink-3">
                        {fmtDateShort(phase.startDate)} → {fmtDateShort(phase.endDate)}
                        {phase.focus ? ` · ${phase.focus}` : ''}
                      </p>
                      <DualCompletionBar
                        label={t('share.public.volume')}
                        actual={phase.actuals.actualVolumeKm}
                        planned={phase.actuals.plannedVolumeKm}
                        unit="km"
                        decimals={1}
                      />
                    </li>
                  )
                })}
              </ul>
            )}
          </div>
        )}
      </Card>

      )}

      <div className="grid gap-5 lg:grid-cols-12">
        {shoes && (
          <Card className="lg:col-span-6" title={t('share.public.shoes')} icon={<Footprints />}>
            {shoes.length === 0 ? (
              <p className="text-sm text-ink-3">{t('share.public.noShoe')}</p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-xs">
                  <thead>
                    <tr className="border-b border-line text-ink-3">
                      <th className="pb-2 pr-3 font-medium">{t('share.public.pair')}</th>
                      <th className="pb-2 px-2 font-medium">{t('share.public.type')}</th>
                      <th className="pb-2 px-2 text-right font-medium">{t('share.public.drop')}</th>
                      <th className="pb-2 pl-2 text-right font-medium">{t('share.public.mileage')}</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-line">
                    {shoes.map((s, i) => (
                      <tr key={`${s.name}-${i}`} className={cn(s.retired && 'opacity-60')}>
                        <td className="py-2 pr-3">
                          <span className="font-semibold text-ink">{s.name}</span>
                          {(s.brand || s.model) && (
                            <span className="ml-1.5 text-ink-3">{[s.brand, s.model].filter(Boolean).join(' ')}</span>
                          )}
                          {s.retired && <span className="ml-1.5 text-ink-3">{t('share.public.retired')}</span>}
                        </td>
                        <td className="py-2 px-2 text-ink-2">{SHOE_TYPE_LABEL[s.type]}</td>
                        <td className="num py-2 px-2 text-right text-ink-2">
                          {isNum(s.dropMm) ? `${fmtNumber(s.dropMm, 1, { trim: true })} mm` : ND}
                        </td>
                        <td className="num py-2 pl-2 text-right font-medium text-ink">
                          {fmtNumber(s.distanceKm, 1)} km
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        )}
        {data.records && <RecordsCard records={data.records} />}
      </div>
    </div>
  )
}

function RecordsCard({ records }: { records: PersonalRecord[] }) {
  const known = records.filter((r) => r.timeS !== null || r.key === 'longest')
  return (
    <Card className="lg:col-span-6" title={t('share.public.records')} icon={<Trophy />}>
      {known.length === 0 ? (
        <p className="text-sm text-ink-3">{t('share.public.noRecord')}</p>
      ) : (
        <dl className="grid grid-cols-2 gap-2 sm:grid-cols-3">
          {known.map((r) => (
            <div key={r.key} className="glass-inset p-3">
              <dt className="text-xs text-ink-3">{recordLabel(r)}</dt>
              <dd className="num mt-0.5 font-semibold text-ink">{r.timeS !== null ? fmtDuration(r.timeS) : fmtKm(r.distanceM)}</dd>
              {r.date && <dd className="num text-[11px] text-ink-3">{fmtDateShort(r.date)}</dd>}
            </div>
          ))}
        </dl>
      )}
    </Card>
  )
}

export default function SharePublicView({ token }: { token: string }) {
  const share = usePublicShareView(token)
  useDocumentTitle(
    share.data ? t('share.public.docTitle', { name: share.data.athlete.displayName }) : t('share.public.docTitleDefault'),
  )

  const unavailable =
    share.isError &&
    isApiError(share.error) &&
    (share.error.status === 404 || share.error.status === 410)

  return (
    <div className="app-root">
      <Ambient />
      <PublicHeader data={share.data ?? null} />
      <main
        id="main"
        className="mx-auto w-full max-w-[1440px] px-4 pt-5 pb-14 sm:px-6 lg:px-8 lg:pt-7"
      >
        {share.isPending ? (
          <div className="space-y-4" role="status" aria-label={t('share.public.loading')}>
            <Skeleton className="h-28" />
            <Skeleton className="h-64" />
            <div className="grid gap-4 lg:grid-cols-2">
              <Skeleton className="h-80" />
              <Skeleton className="h-80" />
            </div>
            <span className="sr-only">{t('share.public.loading')}</span>
          </div>
        ) : unavailable ? (
          <section className="glass mx-auto mt-8 max-w-xl p-7 text-center" role="alert">
            <span className="mx-auto mb-3.5 grid size-12 place-items-center rounded-2xl bg-warn/14 text-warn">
              <Link2Off className="size-6" aria-hidden="true" />
            </span>
            <h1 className="text-lg font-semibold text-ink">
              {t('share.public.notFound')}
            </h1>
            <p className="mt-1.5 text-sm text-ink-2">
              {t('share.public.notFoundHint')}
            </p>
          </section>
        ) : share.isError || !share.data ? (
          <section className="glass mx-auto mt-8 max-w-xl p-7 text-center" role="alert">
            <h1 className="text-lg font-semibold text-ink">{t('share.public.loadFailed')}</h1>
            <p className="mt-1.5 text-sm text-ink-2">
              {share.error instanceof Error ? share.error.message : t('share.public.tempError')}
            </p>
            <button type="button" className="btn btn-ghost mt-4" onClick={() => void share.refetch()}>
              <RotateCw className="size-4" aria-hidden="true" /> {t('common.actions.retry')}
            </button>
          </section>
        ) : (
          <ShareDossierContent data={share.data} token={token} />
        )}
      </main>
    </div>
  )
}
