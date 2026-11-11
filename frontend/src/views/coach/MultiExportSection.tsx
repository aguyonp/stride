import { CalendarRange, Copy, Download, Eye, FileText, ListChecks, Loader2, Minus, Plus, RefreshCw, Share2, Sparkles, TriangleAlert } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useActivities, useMultiExport } from '../../api/queries'
import type { ActivitySummary, IncludeKey, MultiExportRequest, MultiExportResponse } from '../../api/types'
import { ReportPreview } from '../../components/export/ReportPreview'
import { useReportExport, type ReportPayload } from '../../components/export/useReportExport'
import { Card } from '../../components/ui/Card'
import { ChipToggle } from '../../components/ui/Field'
import { ActivityIcon } from '../../components/ui/icons'
import { EmptyState, InlineError } from '../../components/ui/Misc'
import { SegmentedControl } from '../../components/ui/SegmentedControl'
import { Skeleton } from '../../components/ui/Skeleton'
import { useStoredState } from '../../hooks/useStoredState'
import { cn } from '../../lib/cn'
import { capitalize, fmtDuration, fmtKm, fmtNumber, fmtPace, fmtRelative } from '../../lib/format'
import { ALL_INCLUDE, effectiveInclude, INCLUDE_CHIPS, parseInclude, serializeInclude } from '../../lib/include'
import { STORAGE_KEYS } from '../../lib/storage'
import { t } from '../../i18n'

type Mode = 'weeks' | 'manual'
type Filter = 'run' | 'all'

const MULTI_CHIPS = INCLUDE_CHIPS.filter((c) => c.key !== 'splits')
const DEFAULT_INCLUDE = ALL_INCLUDE.filter((k) => k !== 'splits')
const MIN_WEEKS = 1
const MAX_WEEKS = 12

function WeeksStepper({ value, onChange }: { value: number; onChange: (v: number) => void }) {
  return (
    <div className="flex items-center gap-3">
      <button type="button" className="icon-btn size-11" onClick={() => onChange(Math.max(MIN_WEEKS, value - 1))} disabled={value <= MIN_WEEKS} aria-label={t('coach.multi.fewer')}>
        <Minus className="size-4" />
      </button>
      <output className="num min-w-[7.5rem] text-center" aria-live="polite">
        <span className="text-3xl font-semibold tracking-tight text-ink">{value}</span>
        <span className="ml-1.5 text-sm text-ink-3">{t('coach.multi.weeksUnit', { count: value })}</span>
      </output>
      <button type="button" className="icon-btn size-11" onClick={() => onChange(Math.min(MAX_WEEKS, value + 1))} disabled={value >= MAX_WEEKS} aria-label={t('coach.multi.more')}>
        <Plus className="size-4" />
      </button>
    </div>
  )
}

function ActivityCheckRow({ activity: a, checked, onToggle }: { activity: ActivitySummary; checked: boolean; onToggle: () => void }) {
  return (
    <li>
      <label
        className={cn(
          'flex min-h-14 cursor-pointer items-center gap-3 rounded-2xl border px-3 py-2.5 transition-colors',
          checked ? 'border-accent/45 bg-accent/10' : 'border-transparent hover:bg-track/70',
        )}
      >
        <input type="checkbox" className="peer sr-only" checked={checked} onChange={onToggle} />
        <span
          aria-hidden="true"
          className={cn(
            'grid size-5 flex-none place-items-center rounded-md border transition-colors peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-accent',
            checked ? 'border-transparent bg-accent text-on-accent' : 'border-line-strong',
          )}
        >
          {checked && (
            <svg viewBox="0 0 16 16" className="size-3.5" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round">
              <path d="m3.5 8.5 3 3 6-7" />
            </svg>
          )}
        </span>
        <span className={cn('hidden size-9 flex-none place-items-center rounded-xl xs:grid', a.isRunning ? 'bg-accent/15 text-accent' : 'bg-track text-ink-2')}>
          <ActivityIcon sport={a.sport} type={a.type} className="size-4" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-semibold text-ink">{a.name}</span>
          <span className="block truncate text-xs text-ink-3">
            {capitalize(fmtRelative(a.startLocal))} · {a.typeLabel}
          </span>
        </span>
        <span className="num flex-none text-right text-xs">
          <span className="block font-semibold text-ink">{a.distanceM ? fmtKm(a.distanceM, 1) : fmtDuration(a.durationS)}</span>
          <span className="block text-ink-3">{a.distanceM ? (a.isRunning && a.avgPaceSecPerKm ? fmtPace(a.avgPaceSecPerKm) : fmtDuration(a.durationS)) : '—'}</span>
        </span>
      </label>
    </li>
  )
}

function ManualSelection({ selected, onChange }: { selected: Set<number>; onChange: (next: Set<number>) => void }) {
  const [filter, setFilter] = useState<Filter>('run')
  const list = useActivities(filter === 'run' ? { sport: 'run' } : {})
  const items = useMemo(() => {
    const all = list.data?.pages.flatMap((p) => p.items) ?? []
    return filter === 'run' ? all.filter((a) => a.isRunning) : all
  }, [list.data, filter])
  const allVisibleSelected = items.length > 0 && items.every((a) => selected.has(a.id))

  const toggle = (id: number) => {
    const next = new Set(selected)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    onChange(next)
  }
  const toggleVisible = () => {
    const next = new Set(selected)
    for (const a of items) {
      if (allVisibleSelected) next.delete(a.id)
      else next.add(a.id)
    }
    onChange(next)
  }

  return (
    <div>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="flex gap-1.5" role="group" aria-label={t('coach.multi.filter')}>
          <ChipToggle pressed={filter === 'run'} onChange={() => setFilter('run')}>
            {t('coach.multi.run')}
          </ChipToggle>
          <ChipToggle pressed={filter === 'all'} onChange={() => setFilter('all')}>
            {t('coach.multi.all')}
          </ChipToggle>
        </div>
        <div className="flex items-center gap-1">
          <button type="button" className="btn btn-ghost h-9 min-h-9 px-3 text-xs" onClick={toggleVisible} disabled={items.length === 0}>
            {allVisibleSelected ? t('coach.multi.unselectAll') : t('coach.multi.selectAll')}
          </button>
          {selected.size > 0 && (
            <button type="button" className="btn h-9 min-h-9 px-2.5 text-xs text-ink-3 hover:text-ink" onClick={() => onChange(new Set())}>
              {t('common.misc.clear')}
            </button>
          )}
        </div>
      </div>
      <p className="num mb-2 text-sm text-ink-2" aria-live="polite">
        {selected.size === 0 ? t('coach.multi.noneSelected') : t('coach.multi.selected', { count: selected.size })}
      </p>
      {list.isPending ? (
        <div className="space-y-2">
          {Array.from({ length: 5 }, (_, i) => (
            <Skeleton key={i} className="h-14" />
          ))}
        </div>
      ) : list.isError ? (
        <InlineError>{list.error.message}</InlineError>
      ) : items.length === 0 ? (
        <p className="rounded-2xl border border-dashed border-line px-3 py-6 text-center text-sm text-ink-3">{filter === 'run' ? t('coach.multi.emptyRun') : t('coach.multi.emptyAll')}</p>
      ) : (
        <ul className="scroll-thin -mx-1 max-h-[26rem] space-y-1 overflow-y-auto px-1" aria-label={t('coach.multi.activities')}>
          {items.map((a) => (
            <ActivityCheckRow key={a.id} activity={a} checked={selected.has(a.id)} onToggle={() => toggle(a.id)} />
          ))}
        </ul>
      )}
      {list.hasNextPage && (
        <button type="button" className="btn btn-ghost mt-3 w-full" onClick={() => void list.fetchNextPage()} disabled={list.isFetchingNextPage}>
          {list.isFetchingNextPage && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
          {list.isFetchingNextPage ? t('common.actions.loading') : t('coach.multi.loadMore')}
        </button>
      )}
    </div>
  )
}

interface Generated {
  key: string
  res: MultiExportResponse
}

/** Multi-session prompt export: weeks or hand-picked sessions assembled into one prompt. */
export function MultiExportSection({ today }: { today: string }) {
  const [mode, setMode] = useState<Mode>('weeks')
  const [weeks, setWeeks] = useState(4)
  const [selected, setSelected] = useState<Set<number>>(() => new Set())
  const [include, setInclude] = useStoredState<IncludeKey[]>(STORAGE_KEYS.multiInclude, DEFAULT_INCLUDE, parseInclude, serializeInclude)
  const [result, setResult] = useState<Generated | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  const exporter = useMultiExport()

  const request: MultiExportRequest = useMemo(() => {
    const inc = effectiveInclude(include, 'multi')
    return mode === 'weeks' ? { weeks, include: inc } : { activityIds: [...selected].sort((a, b) => a - b), include: inc }
  }, [mode, weeks, selected, include])
  const requestKey = JSON.stringify(request)
  const outdated = result !== null && result.key !== requestKey
  const canGenerate = mode === 'weeks' || selected.size > 0

  const payload: ReportPayload | null = result
    ? { report: result.res.markdown, tokens: result.res.tokensEstimate, filename: t('coach.multi.file', { date: today }), title: t('coach.multi.fileTitle') }
    : null
  const actions = useReportExport(payload, () => setPreviewOpen(true))

  const generate = () => {
    if (!canGenerate) return
    const key = requestKey
    exporter.mutate(request, { onSuccess: (res) => setResult({ key, res }) })
  }

  const toggleChip = (k: IncludeKey, on: boolean) => setInclude(on ? [...include, k] : include.filter((x) => x !== k))

  return (
    <section aria-labelledby="multi-export-title">
      <h2 id="multi-export-title" className="mb-3 text-base font-semibold tracking-tight text-ink">{t('coach.multi.heading')}</h2>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)] lg:gap-6">
        <div className="min-w-0 space-y-4">
          <Card title={t('coach.multi.sessions')} icon={<ListChecks className="size-4" />}>
            <SegmentedControl<Mode>
              ariaLabel={t('coach.multi.selectionMode')}
              className="mb-4"
              value={mode}
              onChange={setMode}
              options={[
                { value: 'weeks', label: t('coach.multi.lastWeeks'), icon: <CalendarRange /> },
              { value: 'manual', label: t('coach.multi.manual'), icon: <ListChecks />, ariaLabel: t('coach.multi.manualAria') },
              ]}
            />
            {mode === 'weeks' ? (
              <div className="flex flex-col items-center gap-2 py-2 text-center">
                <WeeksStepper value={weeks} onChange={setWeeks} />
                <p className="max-w-sm text-xs text-ink-3">{t('coach.multi.weeksHint', { count: weeks })}</p>
              </div>
            ) : (
              <ManualSelection selected={selected} onChange={setSelected} />
            )}
          </Card>

          <Card title={t('coach.multi.content')} icon={<FileText className="size-4" />} subtitle={t('coach.multi.remembered')}>
          <div className="flex flex-wrap gap-2" role="group" aria-label={t('coach.multi.sectionsAria')}>
              {MULTI_CHIPS.map((c) => (
                <ChipToggle key={c.key} pressed={include.includes(c.key)} onChange={(on) => toggleChip(c.key, on)} title={c.hint}>
                  {c.label}
                </ChipToggle>
              ))}
            </div>
            <p className="mt-3 text-xs text-ink-3">{t('coach.multi.directive')}</p>
          </Card>

          <button type="button" className="btn btn-primary h-12 w-full text-[0.95rem]" onClick={generate} disabled={!canGenerate || exporter.isPending}>
            {exporter.isPending ? <Loader2 className="size-[1.125rem] animate-spin" aria-hidden="true" /> : <Sparkles className="size-[1.125rem]" aria-hidden="true" />}
            {exporter.isPending ? t('coach.multi.generating') : result ? t('coach.multi.regenerate') : t('coach.multi.generate')}
          </button>
          {!canGenerate && <p className="-mt-2 text-center text-xs text-ink-3">{t('coach.multi.selectOne')}</p>}
          {exporter.isError && <InlineError>{exporter.error.message}</InlineError>}
        </div>

        <div className="min-w-0 lg:sticky lg:top-24 lg:self-start">
          <Card
            title={t('coach.multi.promptTitle')}
            icon={<Sparkles className="size-4" />}
            subtitle={result ? t('coach.multi.promptMeta', { count: fmtNumber(result.res.sessions), tokens: fmtNumber(result.res.tokensEstimate) }) : t('coach.multi.promptEmpty')}
          >
            {!result ? (
              <EmptyState icon={<FileText />} title={t('coach.multi.placeholderTitle')}>
                {t('coach.multi.placeholderHint')}
              </EmptyState>
            ) : (
              <>
                {outdated && (
                  <div className="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-xl border border-warn/30 bg-warn/10 px-3 py-2" role="status">
                    <p className="flex items-center gap-2 text-sm text-warn">
                      <TriangleAlert className="size-4 flex-none" aria-hidden="true" />
                      {t('coach.multi.changed')}
                    </p>
                    <button type="button" className="btn btn-ghost h-8 min-h-8 px-3 text-xs" onClick={generate} disabled={!canGenerate || exporter.isPending}>
                      <RefreshCw className={cn('size-3.5', exporter.isPending && 'animate-spin')} aria-hidden="true" /> {t('coach.multi.regenerateShort')}
                    </button>
                  </div>
                )}
                {result.res.sessions === 0 && <p className="mb-3 rounded-xl border border-line bg-track px-3 py-2 text-sm text-ink-2">{t('coach.multi.noSession')}</p>}
                <pre
                  tabIndex={0}
                  aria-label={t('coach.multi.textAria')}
                  className={cn(
                    'scroll-thin glass-inset max-h-[26rem] overflow-auto px-3.5 py-3 font-mono text-[12px] leading-[1.6] whitespace-pre-wrap text-ink-2 select-text lg:max-h-[calc(100dvh-19rem)]',
                    (outdated || exporter.isPending) && 'is-refetching',
                  )}
                  style={{ overflowWrap: 'anywhere' }}
                >
                  {result.res.markdown}
                </pre>
                <div className={cn('mt-3 grid gap-2', actions.canShare ? 'grid-cols-2 sm:grid-cols-4' : 'grid-cols-3')}>
                  <button type="button" className={cn('btn btn-primary px-3', actions.canShare && 'col-span-2 sm:col-span-1')} onClick={() => void actions.copy()}>
                    <Copy className="size-4" aria-hidden="true" /> {t('coach.multi.copy')}
                  </button>
                  <button type="button" className="btn btn-ghost px-3" onClick={() => setPreviewOpen(true)}>
                    <Eye className="size-4" aria-hidden="true" /> {t('coach.multi.preview')}
                  </button>
                  <button type="button" className="btn btn-ghost px-3" onClick={actions.download} aria-label={t('coach.multi.download')}>
                    <Download className="size-4" aria-hidden="true" /> .md
                  </button>
                  {actions.canShare && (
                    <button type="button" className="btn btn-ghost px-3" onClick={() => void actions.share()}>
                      <Share2 className="size-4" aria-hidden="true" /> {t('coach.multi.share')}
                    </button>
                  )}
                </div>
              </>
            )}
          </Card>
        </div>
      </div>

      <ReportPreview
        open={previewOpen}
        onClose={() => setPreviewOpen(false)}
        title={t('coach.multi.promptTitle')}
        payload={payload}
        refreshing={exporter.isPending}
        onCopy={() => void actions.copy()}
        onDownload={actions.download}
        onShare={() => void actions.share()}
        canShare={actions.canShare}
      />
    </section>
  )
}
