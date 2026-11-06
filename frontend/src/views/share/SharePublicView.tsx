import { Activity, CalendarRange, Eye, Footprints, Gauge, Link2Off, Moon, RotateCw, ShieldCheck, Stethoscope, Sun } from 'lucide-react'
import { isApiError } from '../../api/client'
import { usePublicShareView } from '../../api/queries'
import type { PublicLoadPoint, PublicShareView } from '../../api/types'
import { BodyAnatomySvg } from '../../components/bodymap/BodyAnatomySvg'
import { BodyCorrelationList } from '../../components/bodymap/BodyCorrelationList'
import { BODY_SIDE_LABEL, BODY_ZONE_META, effectiveZoneScore, painScoreTone, TREND_META } from '../../components/bodymap/bodyMapMeta'
import { Ambient } from '../../components/layout/Ambient'
import { PHASE_TYPE_META } from '../../components/plan/CyclesTimeline'
import { Card } from '../../components/ui/Card'
import { StrideMark } from '../../components/ui/icons'
import { Skeleton } from '../../components/ui/Skeleton'
import { ReadinessBanner } from '../../components/v3/ReadinessBanner'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { useTheme } from '../../hooks/useTheme'
import { cn } from '../../lib/cn'
import { fmtDateShort, fmtDateTime, fmtDayMonth, fmtKm, fmtNumber, fmtPace, fmtSigned, isNum } from '../../lib/format'
import { ACWR_VERDICT, SHOE_TYPE_LABEL, SIDE_LABEL } from '../../lib/labels'

function LoadTrendSvg({ series }: { series: PublicLoadPoint[] }) {
  if (series.length < 2) {
    return (
      <p className="py-6 text-center text-sm text-ink-3">
        Historique de charge sur 42 jours insuffisant pour tracer la tendance.
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
            Charge aiguë (7 j)
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="size-2.5 rounded-full bg-info" aria-hidden="true" />
            Charge chronique (28 j)
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="size-2.5 rounded-full bg-ok" aria-hidden="true" />
            Ratio ACWR (zone optimale 0,8–1,3)
          </span>
        </div>
        <span className="num">{series.length} jours</span>
      </div>

      <svg
        viewBox={`0 0 ${width} ${height}`}
        className="h-auto w-full select-none"
        role="img"
        aria-label="Évolution sur 42 jours de la charge aiguë, chronique et du ratio ACWR"
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
              Stride · Dossier Athlète Partagé
            </p>
            {data && (
              <p className="num truncate text-[11px] text-ink-3">
                {data.shareLabel}
                {data.expiresAt ? ` · Expire le ${fmtDateTime(data.expiresAt)}` : ' · Sans date d’expiration'}
              </p>
            )}
          </div>
        </div>

        <div className="flex flex-none items-center gap-2">
          <span className="chip h-7 border border-info/35 bg-info/12 px-2.5 text-xs font-medium text-info">
            <Eye className="size-3.5" aria-hidden="true" />
            Lecture seule
          </span>
          <button
            type="button"
            className="icon-btn"
            onClick={toggle}
            aria-label={theme === 'dark' ? 'Passer en thème clair' : 'Passer en thème sombre'}
          >
            {theme === 'dark' ? <Sun className="size-[1.125rem]" /> : <Moon className="size-[1.125rem]" />}
          </button>
        </div>
      </div>
    </header>
  )
}

function ShareDossierContent({ data }: { data: PublicShareView }) {
  const { athlete, loadSummary, cycles, bodyMap } = data
  const shoes = data.shoes ?? []
  const recentSessions = data.recentSessions ?? []
  const phases = cycles?.phases ?? []
  const acwrMeta = loadSummary?.verdict ? ACWR_VERDICT[loadSummary.verdict] : null
  const activePlan = cycles?.activePlan
  const activeZones = (bodyMap?.zones ?? []).filter((z) => effectiveZoneScore(z) > 0 || z.entriesCount90d > 0)

  return (
    <div className="space-y-5 lg:space-y-6">
      <section className="glass p-5 sm:p-6">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <h1 className="text-xl font-bold tracking-tight text-ink sm:text-2xl">{athlete.displayName}</h1>
            <div className="mt-2 flex flex-wrap items-center gap-2">
              <span className="chip border border-accent/35 bg-accent/12 text-ink">
                Focus : {athlete.focusLabel}
                {athlete.injuredSide ? ` (${SIDE_LABEL[athlete.injuredSide]})` : ''}
              </span>
              <span className="chip border border-line bg-track/60 text-ink-2">
                Destinataire : {data.shareLabel}
              </span>
              <span className="chip border border-ok/30 bg-ok/10 text-ok">
                <ShieldCheck className="size-3.5" aria-hidden="true" />
                Sans traces GPS ni données d’accès
              </span>
            </div>
          </div>
          <p className="num text-xs text-ink-3">Actualisé le {fmtDateTime(data.generatedAt)}</p>
        </div>
      </section>

      <ReadinessBanner r={data.readiness} />

      <Card
        title="Charge d’entraînement & Efficience"
        subtitle="Synthèse aiguë / chronique, facteur d’efficience aérobie et contrainte mécanique sur 7 jours"
        icon={<Gauge />}
      >
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
          <div className="glass-inset p-3">
            <dt className="text-xs text-ink-3">Ratio ACWR</dt>
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
            <dt className="text-xs text-ink-3">Charge aiguë / chronique</dt>
            <dd className="num mt-1 text-lg font-bold text-ink">
              {fmtNumber(loadSummary.acute, 0)}{' '}
              <span className="text-sm font-normal text-ink-3">/ {fmtNumber(loadSummary.chronic, 0)}</span>
            </dd>
          </div>

          <div className="glass-inset p-3">
            <dt className="text-xs text-ink-3">Efficiency Factor</dt>
            <dd className="num mt-1 flex items-baseline gap-1.5 text-lg font-bold text-ink">
              <span>{isNum(loadSummary.efLatest) ? fmtNumber(loadSummary.efLatest, 2) : 'N/D'}</span>
              {isNum(loadSummary.efTrendPct) && (
                <span className={cn('text-xs font-semibold', loadSummary.efTrendPct >= 0 ? 'text-ok' : 'text-warn')}>
                  {fmtSigned(loadSummary.efTrendPct, 1)} %
                </span>
              )}
            </dd>
          </div>

          <div className="glass-inset p-3">
            <dt className="text-xs text-ink-3">Impact Load (7 j)</dt>
            <dd className="num mt-1 text-lg font-bold text-ink">
              {isNum(loadSummary.impactLoad7d) ? `${fmtNumber(loadSummary.impactLoad7d, 0)} pts` : 'N/D'}
            </dd>
          </div>

          <div className="glass-inset p-3">
            <dt className="text-xs text-ink-3">Indice articulaire (7 j)</dt>
            <dd className="num mt-1 text-lg font-bold text-ink">
              {isNum(loadSummary.jointIndex7d) ? fmtNumber(loadSummary.jointIndex7d, 1) : 'N/D'}
            </dd>
          </div>
        </dl>

        <div className="glass-inset mt-4 p-4">
          <LoadTrendSvg series={data.loadSeries} />
        </div>
      </Card>

      <Card
        title="Cycle d’entraînement actif"
        subtitle={
          activePlan
            ? `${activePlan.name}${activePlan.targetEventLabel ? ` · Objectif : ${activePlan.targetEventLabel}` : ''}`
            : 'Planification macro et mésocycles'
        }
        icon={<CalendarRange />}
      >
        {!activePlan && phases.length === 0 ? (
          <p className="text-sm text-ink-3">Aucun macrocycle structuré actif pour le moment.</p>
        ) : (
          <div className="space-y-4">
            {activePlan && cycles.totals && (
              <div className="glass-inset grid gap-4 p-4 sm:grid-cols-3">
                <div>
                  <p className="text-xs text-ink-3">Période du macrocycle</p>
                  <p className="num mt-1 text-sm font-semibold text-ink">
                    {fmtDateShort(activePlan.startDate)} → {fmtDateShort(activePlan.endDate)}
                  </p>
                  <p className="num mt-1 text-xs text-ink-3">
                    {cycles.totals.sessionsCompleted} / {cycles.totals.sessionsPlanned} séances complétées
                  </p>
                </div>
                <DualCompletionBar
                  label="Volume cumulé"
                  actual={cycles.totals.actualVolumeKm}
                  planned={cycles.totals.plannedVolumeKm}
                  unit="km"
                  decimals={1}
                />
                <DualCompletionBar
                  label="Charge cumulée"
                  actual={cycles.totals.actualLoad}
                  planned={cycles.totals.plannedLoad}
                  unit="pts"
                />
              </div>
            )}

            {phases.length > 0 && (
              <ul className="grid gap-2.5 sm:grid-cols-2 lg:grid-cols-3" aria-label="Mésocycles du plan">
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
                        label="Volume"
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

      <div className="grid gap-5 lg:grid-cols-12">
        <Card
          className="lg:col-span-6"
          title="Cartographie corporelle"
          subtitle={`Synthèse sur ${bodyMap.windowDays} jours (vue en lecture seule)`}
          icon={<Stethoscope />}
        >
          <BodyAnatomySvg zones={bodyMap.zones} readOnly />
          <div className="mt-4 space-y-2">
            {activeZones.length === 0 ? (
              <p className="text-sm text-ink-3">Aucune zone douloureuse signalée sur la période observée.</p>
            ) : (
              <ul className="grid gap-2 sm:grid-cols-2" aria-label="Zones signalées">
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
                          ? `À froid J+1 : ${fmtNumber(z.latestColdNextDayScore, 1, { trim: true })}/10 · `
                          : ''}
                        {z.entriesCount90d} signalement{z.entriesCount90d > 1 ? 's' : ''}
                      </p>
                      <span className={cn('chip mt-2 h-5 border px-2 text-[10px]', trend.cls)}>{trend.label}</span>
                    </li>
                  )
                })}
              </ul>
            )}
          </div>
        </Card>

        <Card
          className="lg:col-span-6"
          title="Corrélations biomécaniques & Chaussures"
          subtitle="Comparaison automatique des sorties avec gêne vs sorties de référence"
          icon={<Footprints />}
        >
          <BodyCorrelationList correlations={bodyMap.correlations} />

          <div className="mt-5 border-t border-line pt-4">
            <h3 className="mb-2.5 text-sm font-semibold text-ink">Rotation chaussures</h3>
            {shoes.length === 0 ? (
              <p className="text-sm text-ink-3">Aucune paire enregistrée.</p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-xs">
                  <thead>
                    <tr className="border-b border-line text-ink-3">
                      <th className="pb-2 pr-3 font-medium">Paire</th>
                      <th className="pb-2 px-2 font-medium">Type</th>
                      <th className="pb-2 px-2 text-right font-medium">Drop</th>
                      <th className="pb-2 pl-2 text-right font-medium">Kilométrage</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-line">
                    {shoes.map((s) => (
                      <tr key={s.id} className={cn(s.retired && 'opacity-60')}>
                        <td className="py-2 pr-3">
                          <span className="font-semibold text-ink">{s.name}</span>
                          {(s.brand || s.model) && (
                            <span className="ml-1.5 text-ink-3">{[s.brand, s.model].filter(Boolean).join(' ')}</span>
                          )}
                          {s.retired && <span className="ml-1.5 text-ink-3">(retirée)</span>}
                        </td>
                        <td className="py-2 px-2 text-ink-2">{SHOE_TYPE_LABEL[s.type]}</td>
                        <td className="num py-2 px-2 text-right text-ink-2">
                          {isNum(s.dropMm) ? `${fmtNumber(s.dropMm, 1, { trim: true })} mm` : 'N/D'}
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
          </div>
        </Card>
      </div>

      <Card
        title="Séances récentes"
        subtitle="Métriques physiologiques, biomécaniques, matériel et douleur rapportée"
        icon={<Activity />}
      >
        {recentSessions.length === 0 ? (
          <p className="text-sm text-ink-3">Aucune séance récente enregistrée.</p>
        ) : (
          <div className="scroll-thin -mx-4 overflow-x-auto px-4 sm:mx-0 sm:px-0">
            <table className="w-full min-w-[780px] text-left text-xs">
              <thead>
                <tr className="border-b border-line text-ink-3">
                  <th className="pb-2.5 pr-3 font-medium">Date</th>
                  <th className="pb-2.5 px-2 font-medium">Séance</th>
                  <th className="pb-2.5 px-2 text-right font-medium">Distance</th>
                  <th className="pb-2.5 px-2 text-right font-medium">Allure / GAP</th>
                  <th className="pb-2.5 px-2 text-right font-medium">FC</th>
                  <th className="pb-2.5 px-2 text-right font-medium">GCT</th>
                  <th className="pb-2.5 px-2 text-right font-medium">D−</th>
                  <th className="pb-2.5 px-2 text-right font-medium">Charge</th>
                  <th className="pb-2.5 px-2 font-medium">Chaussure</th>
                  <th className="pb-2.5 pl-2 text-right font-medium">Douleur / J+1</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {recentSessions.map((s) => {
                  const maxPain = Math.max(s.painDuring ?? 0, s.painNextMorning ?? 0)
                  const painTone = painScoreTone(maxPain)
                  return (
                    <tr key={s.id} className="hover:bg-track/30">
                      <td className="num whitespace-nowrap py-2.5 pr-3 text-ink-2">{fmtDateShort(s.date)}</td>
                      <td className="py-2.5 px-2">
                        <p className="max-w-[14rem] truncate font-semibold text-ink">{s.name}</p>
                        <p className="text-[11px] text-ink-3">{s.typeLabel}</p>
                      </td>
                      <td className="num whitespace-nowrap py-2.5 px-2 text-right font-medium text-ink">
                        {fmtKm(s.distanceM)}
                      </td>
                      <td className="num whitespace-nowrap py-2.5 px-2 text-right text-ink-2">
                        {fmtPace(s.avgPaceSecPerKm)}
                        {isNum(s.gapSecPerKm) && s.gapSecPerKm !== s.avgPaceSecPerKm && (
                          <span className="block text-[11px] text-ink-3">GAP {fmtPace(s.gapSecPerKm)}</span>
                        )}
                      </td>
                      <td className="num whitespace-nowrap py-2.5 px-2 text-right text-ink-2">
                        {isNum(s.avgHR) ? `${fmtNumber(s.avgHR)} bpm` : 'N/D'}
                      </td>
                      <td className="num whitespace-nowrap py-2.5 px-2 text-right text-ink-2">
                        {isNum(s.gctMs) ? `${fmtNumber(s.gctMs)} ms` : 'N/D'}
                      </td>
                      <td className="num whitespace-nowrap py-2.5 px-2 text-right text-ink-2">
                        {isNum(s.elevLossM) ? `${fmtNumber(s.elevLossM)} m` : 'N/D'}
                      </td>
                      <td className="num whitespace-nowrap py-2.5 px-2 text-right font-medium text-ink">
                        {isNum(s.trainingLoad) ? fmtNumber(s.trainingLoad, 0) : 'N/D'}
                      </td>
                      <td className="py-2.5 px-2 text-ink-2">
                        {s.shoeName ? (
                          <span className="block max-w-[9rem] truncate">
                            {s.shoeName}
                            {isNum(s.shoeDropMm) ? ` (${fmtNumber(s.shoeDropMm, 0)} mm)` : ''}
                          </span>
                        ) : (
                          <span className="text-ink-3">—</span>
                        )}
                      </td>
                      <td className="num whitespace-nowrap py-2.5 pl-2 text-right">
                        {s.painDuring !== null || s.painNextMorning !== null ? (
                          <span className={cn('chip h-5 border px-2 text-[11px]', painTone.badgeCls)}>
                            {s.painDuring !== null ? fmtNumber(s.painDuring, 1, { trim: true }) : '—'} /{' '}
                            {s.painNextMorning !== null ? fmtNumber(s.painNextMorning, 1, { trim: true }) : '—'}
                          </span>
                        ) : (
                          <span className="text-ink-3">0 / 0</span>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  )
}

export default function SharePublicView({ token }: { token: string }) {
  const share = usePublicShareView(token)
  useDocumentTitle(
    share.data ? `${share.data.athlete.displayName} · Dossier partagé` : 'Dossier Athlète Partagé',
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
          <div className="space-y-4" role="status" aria-label="Chargement du dossier partagé…">
            <Skeleton className="h-28" />
            <Skeleton className="h-64" />
            <div className="grid gap-4 lg:grid-cols-2">
              <Skeleton className="h-80" />
              <Skeleton className="h-80" />
            </div>
            <span className="sr-only">Chargement du dossier partagé…</span>
          </div>
        ) : unavailable ? (
          <section className="glass mx-auto mt-8 max-w-xl p-7 text-center" role="alert">
            <span className="mx-auto mb-3.5 grid size-12 place-items-center rounded-2xl bg-warn/14 text-warn">
              <Link2Off className="size-6" aria-hidden="true" />
            </span>
            <h1 className="text-lg font-semibold text-ink">
              Lien de partage introuvable, expiré ou révoqué
            </h1>
            <p className="mt-1.5 text-sm text-ink-2">
              Ce dossier athlète en lecture seule n’est plus accessible. Demande un nouveau lien de partage à l’athlète.
            </p>
          </section>
        ) : share.isError || !share.data ? (
          <section className="glass mx-auto mt-8 max-w-xl p-7 text-center" role="alert">
            <h1 className="text-lg font-semibold text-ink">Impossible de charger ce dossier partagé</h1>
            <p className="mt-1.5 text-sm text-ink-2">
              {share.error instanceof Error ? share.error.message : 'Erreur temporaire de connexion.'}
            </p>
            <button type="button" className="btn btn-ghost mt-4" onClick={() => void share.refetch()}>
              <RotateCw className="size-4" aria-hidden="true" /> Réessayer
            </button>
          </section>
        ) : (
          <ShareDossierContent data={share.data} />
        )}
      </main>
    </div>
  )
}
