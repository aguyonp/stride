import { Database, Eraser, Loader2, Settings2, ShieldCheck, Sparkles, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useAdminControl, useAdminMutations } from '../../api/queries'
import type { AdminOverview, IntegrityResult, OptimizeResult, PurgeResult } from '../../api/types'
import { Badge } from '../../components/ui/Badge'
import { Card } from '../../components/ui/Card'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { KeyValue } from '../../components/ui/Misc'
import { useToast } from '../../components/ui/Toast'
import { fmtNumber } from '../../lib/format'
import { fmtBytes } from './adminUtils'
import { lazyRecord, t } from '../../i18n'

const PURGE_LABEL: Record<keyof PurgeResult['removed'], string> = lazyRecord(['sessions', 'invitations', 'apiEvents', 'logs', 'jobs', 'shareLinks'] as const, (k) => t(`admin.maintenance.purge.${k}`))

const yesNo = (b: boolean) => (b ? t('admin.maintenance.yes') : t('admin.maintenance.no'))

export function MaintenancePanel({ overview }: { overview?: AdminOverview }) {
  const m = useAdminMutations()
  const ctl = useAdminControl()
  const { toast } = useToast()
  const [confirmFlush, setConfirmFlush] = useState(false)
  const [vacuum, setVacuum] = useState(false)
  const [purged, setPurged] = useState<PurgeResult | null>(null)
  const [optimized, setOptimized] = useState<OptimizeResult | null>(null)
  const [checked, setChecked] = useState<IntegrityResult | null>(null)
  const db = optimized?.after ?? overview?.database
  const cfg = overview?.config

  const purge = () =>
    ctl.purge.mutate(undefined, {
      onSuccess: (r) => {
        setPurged(r)
        const total = Object.values(r.removed).reduce((a, b) => a + b, 0)
        toast({ title: t('admin.maintenance.purged'), description: t('admin.maintenance.purgedRows', { count: fmtNumber(total) }) })
      },
      onError: (e) => toast({ tone: 'error', title: t('admin.maintenance.purgeFailed'), description: e.message }),
    })
  const optimize = () =>
    ctl.optimize.mutate(vacuum, {
      onSuccess: (r) => {
        setOptimized(r)
        const gain = r.before.sizeBytes + r.before.walBytes - (r.after.sizeBytes + r.after.walBytes)
        toast({ title: t('admin.maintenance.optimized'), description: gain > 0 ? t('admin.maintenance.freed', { size: fmtBytes(gain), ms: fmtNumber(r.durationMs) }) : t('admin.maintenance.doneIn', { ms: fmtNumber(r.durationMs) }) })
      },
      onError: (e) => toast({ tone: 'error', title: t('admin.maintenance.optimizeFailed'), description: e.message }),
    })
  const check = () =>
    ctl.check.mutate(undefined, {
      onSuccess: (r) => setChecked(r),
      onError: (e) => toast({ tone: 'error', title: t('admin.maintenance.checkFailed'), description: e.message }),
    })

  return (
    <div className="grid gap-5 lg:grid-cols-2">
      <Card title={t('admin.maintenance.db')} icon={<Database className="size-4" />} subtitle={t('admin.maintenance.dbHint')} className="lg:col-span-2">
        {db && (
          <dl className="mb-4 grid grid-cols-3 gap-2 sm:max-w-lg">
            {(
              [
                [t('admin.maintenance.data'), db.sizeBytes],
                [t('admin.maintenance.wal'), db.walBytes],
                [t('admin.maintenance.reclaimable'), db.freeBytes],
              ] as const
            ).map(([k, v]) => (
              <div key={k} className="glass-inset min-w-0 px-3 py-2">
                <dt className="truncate text-[11px] text-ink-3">{k}</dt>
                <dd className="num truncate text-base font-semibold text-ink">{fmtBytes(v)}</dd>
              </div>
            ))}
          </dl>
        )}
        <div className="flex flex-wrap items-center gap-3">
          <button type="button" className="btn btn-primary" onClick={optimize} disabled={ctl.optimize.isPending}>
            {ctl.optimize.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Sparkles className="size-4" aria-hidden="true" />}
            {t('admin.maintenance.optimize')}
          </button>
          <label className="flex items-center gap-2 text-sm text-ink-2">
            <input type="checkbox" className="size-4 accent-[var(--accent)]" checked={vacuum} onChange={(e) => setVacuum(e.target.checked)} />
            {t('admin.maintenance.vacuum')}
          </label>
          <button type="button" className="btn btn-ghost sm:ml-auto" onClick={check} disabled={ctl.check.isPending}>
            {ctl.check.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <ShieldCheck className="size-4" aria-hidden="true" />}
            {t('admin.maintenance.check')}
          </button>
        </div>
        <p className="mt-2 text-xs text-ink-3">
          {t('admin.maintenance.optimizeHint')}
        </p>
        {checked && (
          <div className="mt-4 border-t border-line pt-3" role="status">
            {checked.ok ? (
              <p className="flex flex-wrap items-center gap-2 text-sm text-ink-2">
                <Badge level="ok">{t('admin.maintenance.healthy')}</Badge>
                {t('admin.maintenance.noAnomaly', { ms: fmtNumber(checked.durationMs) })}
              </p>
            ) : (
              <>
                <p className="mb-2 flex flex-wrap items-center gap-2 text-sm text-ink-2">
                  <Badge level="danger">
                    {t('admin.maintenance.problems', { count: checked.total })}
                  </Badge>
                  {t('admin.maintenance.backupFirst')}
                </p>
                <ul className="scroll-thin max-h-48 space-y-1 overflow-y-auto font-mono text-xs break-all text-ink-2">
                  {checked.problems.map((p, i) => (
                    <li key={i}>{p}</li>
                  ))}
                </ul>
              </>
            )}
          </div>
        )}
      </Card>

      <Card title={t('admin.maintenance.expired')} icon={<Trash2 className="size-4" />} subtitle={t('admin.maintenance.expiredHint')}>
        <p className="text-sm text-ink-2">{t('admin.maintenance.expiredBody')}</p>
        <button type="button" className="btn btn-ghost mt-4" onClick={purge} disabled={ctl.purge.isPending}>
          {ctl.purge.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Trash2 className="size-4" aria-hidden="true" />}
          {t('admin.maintenance.purgeNow')}
        </button>
        {purged && (
          <KeyValue
            className="mt-4 border-t border-line pt-3"
            items={(Object.keys(PURGE_LABEL) as (keyof PurgeResult['removed'])[]).map((k) => [PURGE_LABEL[k], <span className="num">{fmtNumber(purged.removed[k] ?? 0)}</span>])}
          />
        )}
      </Card>

      <Card title={t('admin.maintenance.cache')} icon={<Eraser className="size-4" />} subtitle={t('admin.maintenance.cacheKeys', { backend: cfg?.cacheBackend === 'valkey' ? 'Valkey' : t('admin.maintenance.memory') })}>
        <p className="text-sm text-ink-2">
          {t('admin.maintenance.cacheBody')}
        </p>
        <button type="button" className="btn btn-danger mt-4" onClick={() => setConfirmFlush(true)} disabled={m.flushCache.isPending}>
          {m.flushCache.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Eraser className="size-4" aria-hidden="true" />}
          {t('admin.maintenance.flush')}
        </button>
      </Card>

      <Card title={t('admin.maintenance.config')} icon={<Settings2 className="size-4" />} subtitle={t('admin.maintenance.configHint')} className="lg:col-span-2">
        {cfg ? (
          <KeyValue
            className="sm:max-w-2xl"
            items={[
              ['URL publique', <span className="font-mono text-xs break-all">{cfg.publicUrl || '—'}</span>],
              [t('admin.maintenance.rpId'), <span className="font-mono text-xs">{cfg.rpId || '—'}</span>],
              [t('admin.maintenance.registration'), overview?.registrationMode === 'open' ? t('admin.maintenance.open') : t('admin.maintenance.invite')],
              [t('admin.maintenance.secureCookies'), yesNo(cfg.secureCookies)],
              [t('admin.maintenance.sessionTtl'), t('admin.maintenance.days', { n: fmtNumber(cfg.sessionTtlHours / 24, 1, { trim: true }) })],
              [t('admin.maintenance.timezone'), cfg.timezone],
              [t('admin.maintenance.morningSync'), `${String(cfg.syncHour).padStart(2, '0')}:00`],
              [t('admin.maintenance.cache'), cfg.cacheBackend === 'valkey' ? 'Valkey' : t('admin.maintenance.memorySingle')],
              [t('admin.maintenance.metrics'), cfg.metricsEnabled ? t('admin.maintenance.metricsOn') : t('admin.maintenance.metricsOff')],
              ['Strava', cfg.stravaEnabled ? t('admin.maintenance.configured') : t('admin.maintenance.notConfigured')],
              [t('admin.maintenance.weather'), cfg.weatherProvider || t('admin.maintenance.disabled')],
              [t('admin.maintenance.privateEgress'), yesNo(cfg.allowPrivateEgress)],
              [t('admin.maintenance.logs'), `${cfg.logLevel} · ${cfg.logFormat}`],
            ]}
          />
        ) : (
          <p className="text-sm text-ink-3">{t('admin.maintenance.loadingConfig')}</p>
        )}
      </Card>

      <ConfirmDialog
        open={confirmFlush}
        onClose={() => setConfirmFlush(false)}
        title={t('admin.maintenance.flushTitle')}
        message={<p>{t('admin.maintenance.flushBody')}</p>}
        confirmLabel={t('admin.maintenance.flush')}
        danger
        busy={m.flushCache.isPending}
        onConfirm={() =>
          m.flushCache.mutate(undefined, {
            onSuccess: (r) => {
              setConfirmFlush(false)
              toast({ title: t('admin.maintenance.flushed'), description: t('admin.maintenance.flushedKeys', { count: fmtNumber(r.removed) }) })
            },
            onError: (e) => toast({ tone: 'error', title: t('admin.maintenance.flushFailed'), description: e.message }),
          })
        }
      />
    </div>
  )
}
