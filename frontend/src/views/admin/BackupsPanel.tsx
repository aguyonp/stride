import { Check, Cloud, CloudUpload, Copy, DatabaseBackup, Download, HardDrive, Loader2, PlugZap, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { api } from '../../api/client'
import { useAdminBackups, useBackupActions } from '../../api/queriesV42'
import type { BackupItem, BackupS3Status, BackupTrigger, S3Config, S3ConfigInput, S3TestResult } from '../../api/types'
import { downloadBlob } from '../../components/profile/download'
import { Badge } from '../../components/ui/Badge'
import { Card } from '../../components/ui/Card'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { Checkbox, Field } from '../../components/ui/Field'
import { EmptyState, InlineError } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { copyText } from '../../lib/clipboard'
import { cn } from '../../lib/cn'
import { fmtDateTime, fmtNumber } from '../../lib/format'
import { useActiveJobs } from '../../state/events'
import { ErrorView } from '../StateViews'
import { fmtBytes } from './adminUtils'
import { lazyRecord, t, tx } from '../../i18n'

const TRIGGER_LABEL: Record<BackupTrigger, string> = lazyRecord(['manual', 'daily', 'pre_migration'] as const, (k) => t(`admin.backups.trigger.${k}`))

const S3_CLS: Record<BackupS3Status, string> = {
  none: 'border-line bg-track/60 text-ink-2',
  pending: 'border-info/30 bg-info/10 text-info',
  synced: 'border-ok/30 bg-ok/10 text-ok',
  error: 'border-danger/30 bg-danger/10 text-danger',
}
const S3_STATE: Record<BackupS3Status, { label: string; cls: string }> = lazyRecord(['none', 'pending', 'synced', 'error'] as const, (k) => ({ label: t(`admin.backups.s3.${k}`), cls: S3_CLS[k] }))

/** « 3f9a12c4…8be0 »: enough to compare by eye, full value copied. */
export const shortSha = (sha: string) => (sha.length > 16 ? `${sha.slice(0, 8)}…${sha.slice(-4)}` : sha)
const s3Configured = (c: S3Config | undefined) => Boolean(c && c.endpoint && c.bucket && c.accessKeyId && c.secretKeySet)

function StatusChip({ b }: { b: BackupItem }) {
  const st = S3_STATE[b.s3.status]
  return (
    <span className={cn('chip h-6 border px-2 text-[11px]', st.cls)} title={b.s3.error ?? (b.s3.key ? `s3://…/${b.s3.key}` : undefined)}>
      {b.s3.status === 'synced' ? <Cloud className="size-3" aria-hidden="true" /> : b.s3.status === 'pending' ? <Loader2 className="size-3 animate-spin" aria-hidden="true" /> : <HardDrive className="size-3" aria-hidden="true" />}
      {st.label}
    </span>
  )
}

function ShaCell({ sha }: { sha: string }) {
  const { toast } = useToast()
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    if (!(await copyText(sha))) return toast({ tone: 'error', title: t('admin.backups.copyFailed'), description: sha })
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1800)
  }
  return (
    <span className="inline-flex items-center gap-1">
      <code className="num font-mono text-xs text-ink-2" title={sha}>
        {shortSha(sha)}
      </code>
      <button type="button" className="icon-btn size-7" onClick={() => void copy()} aria-label={t('admin.backups.copySha')}>
        {copied ? <Check className="size-3.5 text-ok" /> : <Copy className="size-3.5" />}
      </button>
    </span>
  )
}

function S3Form({ s3 }: { s3: S3Config }) {
  const a = useBackupActions()
  const { toast } = useToast()
  const [f, setF] = useState<S3ConfigInput>(() => ({ endpoint: s3.endpoint, region: s3.region, bucket: s3.bucket, prefix: s3.prefix, accessKeyId: s3.accessKeyId, secretKey: '', pathStyle: s3.pathStyle, autoUpload: s3.autoUpload, retentionDays: s3.retentionDays }))
  const [result, setResult] = useState<S3TestResult | null>(null)
  const set = <K extends keyof S3ConfigInput>(k: K, v: S3ConfigInput[K]) => {
    setF((x) => ({ ...x, [k]: v }))
    setResult(null)
  }
  const retentionOk = Number.isInteger(f.retentionDays) && f.retentionDays >= 1 && f.retentionDays <= 3650
  const test = () => a.testS3.mutate(f, { onSuccess: setResult, onError: (e) => setResult({ ok: false, latencyMs: 0, error: e.message }) })
  const save = () =>
    a.saveS3.mutate(f, {
      onSuccess: () => toast({ title: t('admin.backups.targetSaved'), description: f.bucket ? `${f.bucket} · ${f.region}` : t('admin.backups.remoteDisabled') }),
      onError: (e) => toast({ tone: 'error', title: t('admin.backups.saveFailed'), description: e.message }),
    })
  return (
    <form
      className="space-y-3.5"
      onSubmit={(e) => {
        e.preventDefault()
        save()
      }}
      autoComplete="off"
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Endpoint" className="sm:col-span-2" hint={t('admin.backups.endpointHint')}>
          {(p) => <input {...p} className="field" type="url" inputMode="url" value={f.endpoint} onChange={(e) => set('endpoint', e.target.value)} placeholder="https://s3.fr-par.scw.cloud" spellCheck={false} />}
        </Field>
        <Field label={t('admin.backups.region')}>{(p) => <input {...p} className="field" value={f.region} onChange={(e) => set('region', e.target.value)} placeholder="fr-par" spellCheck={false} />}</Field>
        <Field label="Bucket">{(p) => <input {...p} className="field" value={f.bucket} onChange={(e) => set('bucket', e.target.value)} placeholder="stride-backups" spellCheck={false} />}</Field>
        <Field label={t('admin.backups.prefix')} hint={t('admin.backups.prefixHint')}>
          {(p) => <input {...p} className="field" value={f.prefix} onChange={(e) => set('prefix', e.target.value)} placeholder="stride/" spellCheck={false} />}
        </Field>
        <Field label={t('admin.backups.retention')} error={retentionOk ? undefined : t('admin.backups.retentionError')}>
          {(p) => <input {...p} className="field num" type="number" min={1} max={3650} inputMode="numeric" value={Number.isFinite(f.retentionDays) ? f.retentionDays : ''} onChange={(e) => set('retentionDays', Number(e.target.value))} />}
        </Field>
        <Field label={t('admin.backups.accessKey')}>
          {(p) => <input {...p} className="field font-mono text-sm" value={f.accessKeyId} onChange={(e) => set('accessKeyId', e.target.value)} spellCheck={false} autoComplete="off" />}
        </Field>
        <Field label={t('admin.backups.secretKey')} hint={s3.secretKeySet ? t('admin.backups.secretKeySet') : t('admin.backups.secretKeyHint')}>
          {(p) => (
            <input
              {...p}
              className="field font-mono text-sm"
              type="password"
              value={f.secretKey}
              onChange={(e) => set('secretKey', e.target.value)}
              placeholder={s3.secretKeySet ? t('admin.backups.secretSaved') : ''}
              autoComplete="new-password"
              spellCheck={false}
            />
          )}
        </Field>
      </div>
      <div className="grid gap-2 sm:grid-cols-2">
        <Checkbox checked={f.pathStyle} onChange={(v) => set('pathStyle', v)} label={t('admin.backups.pathStyle')} description={t('admin.backups.pathStyleHint')} />
        <Checkbox checked={f.autoUpload} onChange={(v) => set('autoUpload', v)} label={t('admin.backups.autoUpload')} description={t('admin.backups.autoUploadHint')} />
      </div>
      {result && (
        <p className="flex flex-wrap items-center gap-2 text-sm text-ink-2" role="status">
          {result.ok ? <Badge level="ok">{t('admin.backups.connected')}</Badge> : <Badge level="danger">{t('admin.backups.failed')}</Badge>}
          {result.ok ? <span className="num">{t('admin.backups.reachable', { ms: fmtNumber(result.latencyMs) })}</span> : <span>{result.error ?? t('admin.backups.unreachable')}</span>}
        </p>
      )}
      <div className="flex flex-wrap justify-end gap-2">
        <button type="button" className="btn btn-ghost" onClick={test} disabled={a.testS3.isPending}>
          {a.testS3.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <PlugZap className="size-4" aria-hidden="true" />}
          {t('admin.backups.testConnection')}
        </button>
        <button type="submit" className="btn btn-primary" disabled={a.saveS3.isPending || !retentionOk}>
          {a.saveS3.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
          {t('common.actions.save')}
        </button>
      </div>
    </form>
  )
}

/** « Backups & S3 »: atomic SQLite snapshots, retention, S3 target, live job progress from the event store. */
export function BackupsPanel() {
  const q = useAdminBackups()
  const a = useBackupActions()
  const { toast } = useToast()
  const jobs = useActiveJobs().filter((j) => j.kind.startsWith('backup.'))
  const [del, setDel] = useState<{ item: BackupItem; remote: boolean } | null>(null)
  const [downloading, setDownloading] = useState<string | null>(null)
  const data = q.data
  const s3On = s3Configured(data?.s3)
  const busy = jobs.length > 0 || a.create.isPending

  const start = (upload: boolean) =>
    a.create.mutate(upload, {
      onSuccess: () => toast({ title: upload ? t('admin.backups.startedS3') : t('admin.backups.startedLocal'), tone: 'info' }),
      onError: (e) => toast({ tone: 'error', title: t('admin.backups.backupFailed'), description: e.message }),
    })
  const push = (b: BackupItem) =>
    a.upload.mutate(b.name, {
      onSuccess: () => toast({ title: t('admin.backups.uploadStarted'), description: b.name, tone: 'info' }),
      onError: (e) => toast({ tone: 'error', title: t('admin.backups.uploadFailed'), description: e.message }),
    })
  const download = async (b: BackupItem) => {
    setDownloading(b.name)
    try {
      const { blob, filename } = await api.adminDownloadBackup(b.name)
      downloadBlob(blob, filename)
    } catch (e) {
      toast({ tone: 'error', title: t('admin.backups.downloadFailed'), description: e instanceof Error ? e.message : undefined })
    } finally {
      setDownloading(null)
    }
  }

  const rowActions = (b: BackupItem) => (
    <span className="flex items-center justify-end gap-1">
      <button type="button" className="icon-btn size-9" onClick={() => void download(b)} disabled={downloading === b.name} aria-label={t('admin.backups.download', { name: b.name })}>
        {downloading === b.name ? <Loader2 className="size-4 animate-spin" /> : <Download className="size-4" />}
      </button>
      {s3On && b.s3.status !== 'synced' && b.s3.status !== 'pending' && (
        <button type="button" className="icon-btn size-9" onClick={() => push(b)} disabled={a.upload.isPending && a.upload.variables === b.name} aria-label={t('admin.backups.push', { name: b.name })}>
          {a.upload.isPending && a.upload.variables === b.name ? <Loader2 className="size-4 animate-spin" /> : <CloudUpload className="size-4" />}
        </button>
      )}
      <button type="button" className="icon-btn size-9 text-danger" onClick={() => setDel({ item: b, remote: false })} aria-label={t('admin.backups.delete', { name: b.name })}>
        <Trash2 className="size-4" />
      </button>
    </span>
  )

  return (
    <div className="grid gap-5 xl:grid-cols-5">
      <Card className="xl:col-span-3" title={t('admin.backups.title')} icon={<DatabaseBackup className="size-4" />} subtitle={t('admin.backups.subtitle')}>
        {q.isError ? (
          <ErrorView error={q.error} onRetry={() => void q.refetch()} title={t('admin.backups.unavailable')} />
        ) : !data ? (
          <Skeleton className="h-64" />
        ) : (
          <div className="space-y-4">
            <dl className="grid gap-2 sm:grid-cols-3">
              {(
                [
                  [t('admin.backups.lastLocal'), data.lastSuccess.local ? fmtDateTime(data.lastSuccess.local) : t('admin.backups.never')],
                  [t('admin.backups.lastS3'), data.lastSuccess.s3 ? fmtDateTime(data.lastSuccess.s3) : s3On ? t('admin.backups.never') : t('admin.backups.s3Off')],
                  [t('admin.backups.next'), data.nextScheduledAt ? fmtDateTime(data.nextScheduledAt) : '—'],
                ] as const
              ).map(([k, v]) => (
                <div key={k} className="glass-inset flex min-w-0 items-baseline justify-between gap-3 px-3 py-2 sm:block">
                  <dt className="truncate text-[11px] text-ink-3">{k}</dt>
                  <dd className="num truncate text-sm font-semibold text-ink">{v}</dd>
                </div>
              ))}
            </dl>

            <div className="flex flex-wrap items-center gap-2">
              <button type="button" className="btn btn-primary" onClick={() => start(false)} disabled={busy}>
                {a.create.isPending && a.create.variables === false ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <HardDrive className="size-4" aria-hidden="true" />}
                {t('admin.backups.backupLocal')}
              </button>
              <button type="button" className="btn btn-ghost" onClick={() => start(true)} disabled={busy || !s3On} title={s3On ? undefined : t('admin.backups.configureS3')}>
                {a.create.isPending && a.create.variables === true ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <CloudUpload className="size-4" aria-hidden="true" />}
                {t('admin.backups.backupS3')}
              </button>
            </div>
            <p className="text-xs text-ink-3">{t('admin.backups.retentionHint')}</p>

            {jobs.length > 0 && (
              <ul className="space-y-2" aria-label={t('admin.backups.running')}>
                {jobs.map((j) => (
                  <li key={j.id} className="glass-inset px-3 py-2.5">
                    <div className="mb-1.5 flex items-baseline justify-between gap-2 text-xs">
                      <span className="truncate font-medium text-ink">{j.label}</span>
                      <span className="num flex-none text-ink-3">{fmtNumber(j.progressPct ?? 0)} %</span>
                    </div>
                    <div className="h-1.5 overflow-hidden rounded-full bg-track" role="progressbar" aria-label={j.label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={j.progressPct ?? 0}>
                      <div className="h-full rounded-full bg-accent transition-[width] duration-500" style={{ width: `${j.progressPct ?? 0}%` }} />
                    </div>
                  </li>
                ))}
              </ul>
            )}

            {data.items.length === 0 ? (
              <EmptyState icon={<DatabaseBackup />} title={t('admin.backups.none')}>
                {data.nextScheduledAt ? t('admin.backups.firstOn', { date: fmtDateTime(data.nextScheduledAt) }) : t('admin.backups.firstTonight')}
              </EmptyState>
            ) : (
              <>
                <div className="scroll-thin hidden overflow-x-auto md:block">
                  <table className="w-full min-w-[640px] border-separate border-spacing-0 text-sm">
                    <caption className="sr-only">{t('admin.backups.caption')}</caption>
                    <thead>
                      <tr className="text-left text-xs text-ink-3">
                        <th className="border-b border-line pr-3 pb-2 font-medium">{t('admin.backups.date')}</th>
                        <th className="border-b border-line px-2 pb-2 text-right font-medium">{t('admin.backups.size')}</th>
                        <th className="border-b border-line px-2 pb-2 font-medium">SHA-256</th>
                        <th className="border-b border-line px-2 pb-2 font-medium">{t('admin.backups.status')}</th>
                        <th className="border-b border-line pb-2 pl-2 text-right font-medium">
                          <span className="sr-only">{t('admin.backups.actions')}</span>
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {data.items.map((b) => (
                        <tr key={b.name} className="align-middle">
                          <td className="border-b border-line py-2 pr-3">
                            <span className="num block text-ink">{fmtDateTime(b.createdAt)}</span>
                            <span className="block text-[11px] text-ink-3">{TRIGGER_LABEL[b.trigger]}</span>
                          </td>
                          <td className="num border-b border-line px-2 py-2 text-right text-ink-2">{fmtBytes(b.sizeBytes)}</td>
                          <td className="border-b border-line px-2 py-2">
                            <ShaCell sha={b.sha256} />
                          </td>
                          <td className="border-b border-line px-2 py-2">
                            <StatusChip b={b} />
                          </td>
                          <td className="border-b border-line py-2 pl-2">{rowActions(b)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <ul className="divide-y divide-line md:hidden" aria-label={t('admin.backups.caption')}>
                  {data.items.map((b) => (
                    <li key={b.name} className="py-3 first:pt-0 last:pb-0">
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0">
                          <p className="num text-sm font-medium text-ink">{fmtDateTime(b.createdAt)}</p>
                          <p className="num text-xs text-ink-3">
                            {TRIGGER_LABEL[b.trigger]} · {fmtBytes(b.sizeBytes)}
                          </p>
                        </div>
                        <StatusChip b={b} />
                      </div>
                      {b.s3.status === 'error' && b.s3.error && <p className="mt-1 text-xs text-danger">{b.s3.error}</p>}
                      <div className="mt-1.5 flex items-center justify-between gap-2">
                        <ShaCell sha={b.sha256} />
                        {rowActions(b)}
                      </div>
                    </li>
                  ))}
                </ul>
              </>
            )}
          </div>
        )}
      </Card>

      <Card className="xl:col-span-2" title={t('admin.backups.s3Title')} icon={<Cloud className="size-4" />} subtitle={s3On ? `${data?.s3.bucket} · ${data?.s3.region}` : t('admin.backups.s3Subtitle')}>
        {q.isError ? <InlineError>{q.error.message}</InlineError> : data ? <S3Form key={JSON.stringify(data.s3)} s3={data.s3} /> : <Skeleton className="h-72" />}
      </Card>

      <ConfirmDialog
        open={del !== null}
        onClose={() => setDel(null)}
        title={t('admin.backups.deleteTitle')}
        message={
          del && (
            <div className="space-y-3">
              <p>
                {tx('admin.backups.deleteBody', { name: <code className="font-mono text-xs break-all text-ink">{del.item.name}</code> })}
              </p>
              {(del.item.s3.status === 'synced' || del.item.s3.status === 'error') && (
                <Checkbox checked={del.remote} onChange={(v) => setDel({ ...del, remote: v })} label={t('admin.backups.deleteRemote')} description={t('admin.backups.deleteRemoteHint')} />
              )}
            </div>
          )
        }
        confirmLabel={t('common.actions.delete')}
        danger
        busy={a.remove.isPending}
        onConfirm={() =>
          del &&
          a.remove.mutate(
            { name: del.item.name, remote: del.remote },
            {
              onSuccess: () => {
                toast({ title: t('admin.backups.deleted'), description: del.item.name })
                setDel(null)
              },
              onError: (e) => toast({ tone: 'error', title: t('admin.backups.deleteFailed'), description: e.message }),
            },
          )
        }
      />
    </div>
  )
}
