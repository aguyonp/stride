import { Pencil, Plus, ShieldCheck, Sparkles, TrendingDown, TrendingUp, Minus } from 'lucide-react'
import { useState } from 'react'
import type { PublicShareView } from '../../../api/types'
import { effectiveZoneScore } from '../../../components/bodymap/bodyMapMeta'
import { authorLine, restrictionChips, validityLine } from '../../../components/share/shareNotes'
import { cn } from '../../../lib/cn'
import { fmtDateTime, fmtNumber, isNum, ND } from '../../../lib/format'
import { ACWR_VERDICT, SIDE_LABEL } from '../../../lib/labels'
import { DirectiveEditor } from './PortalEditors'
import { painTrend, permissionsOf } from './portalUtils'
import { t } from '../../../i18n'

const LEVEL_CHIP = { ok: 'border-ok/35 bg-ok/12 text-ok', warn: 'border-warn/35 bg-warn/12 text-warn', danger: 'border-danger/35 bg-danger/12 text-danger', info: 'border-info/35 bg-info/12 text-info' } as const

/** Shown wherever the recipient can write: the athlete's assistant reads what they leave. */
export function AiTransparency({ className }: { className?: string }) {
  return (
    <p className={cn('flex items-start gap-2 text-xs text-ink-3', className)}>
      <Sparkles className="mt-px size-3.5 flex-none text-accent" aria-hidden="true" />
      {t('share.portal.aiTransparency')}
    </p>
  )
}

/** 1. Vital summary: who, health & pathologies, ACWR, J+1 pain trend, and this link's directive. */
export function PortalSummary({ data, token, inline, today }: { data: PublicShareView; token: string; inline: boolean; today: string }) {
  const perms = permissionsOf(data)
  const [editing, setEditing] = useState(false)
  const { athlete, loadSummary, directive } = data
  const trend = painTrend(data)
  const acwr = loadSummary?.verdict ? ACWR_VERDICT[loadSummary.verdict] : null
  const health = data.sections.includes('health')
  const zones = (data.bodyMap?.zones ?? []).filter((z) => effectiveZoneScore(z) > 0 || z.entriesCount90d > 0).map((z) => z.zoneLabel)
  const TrendIcon = trend?.direction === 'up' ? TrendingUp : trend?.direction === 'down' ? TrendingDown : Minus
  const showDirective = Boolean(directive) || perms.comments
  const editorOpen = editing && perms.comments

  return (
    <section className="glass p-5 sm:p-6" aria-labelledby="portal-athlete">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <h1 id="portal-athlete" className="text-xl font-bold tracking-tight text-ink sm:text-2xl">
            {athlete.displayName}
          </h1>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <span className="chip border border-line bg-track/60 text-ink-2">{t('share.portal.recipient', { label: data.shareLabel })}</span>
            <span className="chip border border-ok/30 bg-ok/10 text-ok">
              <ShieldCheck className="size-3.5" aria-hidden="true" />
              {t('share.portal.noGps')}
            </span>
          </div>
        </div>
        <p className="num text-xs text-ink-3">{t('share.portal.refreshed', { date: fmtDateTime(data.generatedAt) })}</p>
      </div>

      {(health || loadSummary || trend) && (
        <dl className="mt-4 grid divide-y divide-line border-y border-line sm:grid-cols-3 sm:divide-x sm:divide-y-0">
          {health && (
            <div className="min-w-0 py-3 sm:px-4 sm:first:pl-0">
              <dt className="text-xs text-ink-3">{t('share.portal.health')}</dt>
              <dd className="mt-1 text-sm font-semibold text-ink">
                {athlete.focusLabel && athlete.focusArea !== 'none' ? `${athlete.focusLabel}${athlete.injuredSide ? ` · ${SIDE_LABEL[athlete.injuredSide]}` : ''}` : t('share.portal.healthy')}
              </dd>
              {zones.length > 0 && <dd className="mt-0.5 truncate text-xs text-ink-3">{t('share.portal.zones', { list: zones.join(', ') })}</dd>}
            </div>
          )}
          {loadSummary && (
            <div className="min-w-0 py-3 sm:px-4 sm:first:pl-0">
              <dt className="text-xs text-ink-3">{t('share.portal.acwr')}</dt>
              <dd className="mt-1 flex flex-wrap items-center gap-2">
                <span className="num text-lg font-bold text-ink">{isNum(loadSummary.acwr) ? fmtNumber(loadSummary.acwr, 2) : ND}</span>
                {acwr && <span className={cn('chip h-5 border px-2 text-[11px]', LEVEL_CHIP[acwr.level as keyof typeof LEVEL_CHIP] ?? LEVEL_CHIP.info)}>{acwr.label}</span>}
              </dd>
            </div>
          )}
          {trend && (
            <div className="min-w-0 py-3 sm:px-4 sm:first:pl-0">
              <dt className="text-xs text-ink-3">{t('share.portal.morningPain')}</dt>
              <dd className="mt-1 flex flex-wrap items-center gap-2">
                <span className="num text-lg font-bold text-ink">{fmtNumber(trend.latest, 1, { trim: true })}/10</span>
                {trend.previous !== null && (
                  <span className={cn('inline-flex items-center gap-1 text-xs font-medium', trend.direction === 'up' ? 'text-warn' : trend.direction === 'down' ? 'text-ok' : 'text-ink-3')}>
                    <TrendIcon className="size-3.5" aria-hidden="true" />
                    {trend.direction === 'up' ? t('share.portal.up') : trend.direction === 'down' ? t('share.portal.down') : t('share.portal.stable')}
                  </span>
                )}
              </dd>
            </div>
          )}
        </dl>
      )}

      {showDirective && (
        <div className="mt-4">
          {editorOpen && inline ? (
            <DirectiveEditor token={token} directive={directive ?? null} inline open onClose={() => setEditing(false)} today={today} />
          ) : directive ? (
            <div>
              <div className="flex items-start justify-between gap-3">
                <h2 className="text-sm font-semibold text-ink">{t('share.portal.directive')}</h2>
                {perms.comments && (
                  <button type="button" className="btn btn-ghost h-8 min-h-8 flex-none px-3 text-xs" onClick={() => setEditing(true)}>
                    <Pencil className="size-3.5" aria-hidden="true" /> {t('share.portal.edit')}
                  </button>
                )}
              </div>
              <p className="mt-1.5 max-w-prose text-sm break-words whitespace-pre-line text-ink">{directive.body}</p>
              {restrictionChips(directive.restrictions).length > 0 && (
                <ul className="mt-2 flex flex-wrap gap-1" aria-label={t('share.portal.restrictions')}>
                  {restrictionChips(directive.restrictions).map((r) => (
                    <li key={r} className="rounded-lg bg-track px-1.5 py-0.5 text-[11px] font-semibold text-ink-2">
                      {r}
                    </li>
                  ))}
                </ul>
              )}
              <p className="num mt-2 text-xs text-ink-3">
                {authorLine(directive)} · {validityLine(directive)}
              </p>
            </div>
          ) : (
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-sm text-ink-3">{t('share.portal.noDirective')}</p>
              <button type="button" className="btn btn-primary h-9 min-h-9 px-3.5 text-sm" onClick={() => setEditing(true)}>
                <Plus className="size-4" aria-hidden="true" /> {t('share.portal.writeDirective')}
              </button>
            </div>
          )}
          {!inline && perms.comments && <DirectiveEditor key={directive?.updatedAt ?? 'new'} token={token} directive={directive ?? null} inline={false} open={editing} onClose={() => setEditing(false)} today={today} />}
        </div>
      )}

      {(perms.comments || perms.planning) && <AiTransparency className="mt-4" />}
    </section>
  )
}
