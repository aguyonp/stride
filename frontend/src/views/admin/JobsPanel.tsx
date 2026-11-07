import { Ban, ListChecks, Loader2, RefreshCw, RotateCcw } from 'lucide-react'
import { useState } from 'react'
import { useAdminControl, useAdminJobs } from '../../api/queries'
import type { AdminJob, AdminJobFilter } from '../../api/types'
import { Badge } from '../../components/ui/Badge'
import { Card } from '../../components/ui/Card'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { EmptyState } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { cn } from '../../lib/cn'
import { fmtNumber, fmtWhen } from '../../lib/format'
import { ErrorView } from '../StateViews'
import { FilterChips, JOB_GROUP_LABEL, JOB_STATE } from './adminUtils'

const EMPTY: Record<AdminJobFilter, { title: string; body: string }> = {
  active: { title: 'Aucune tâche en cours', body: 'Synchronisations, imports, analyses IA et notifications apparaissent ici pendant leur exécution, tous comptes confondus.' },
  failed: { title: 'Aucun échec récent', body: 'Les tâches en échec sont conservées 14 jours. Les synchronisations échouées peuvent être relancées d’ici.' },
  all: { title: 'Aucune tâche enregistrée', body: 'Les tâches terminées sont purgées après 14 jours.' },
}

function Progress({ j }: { j: AdminJob }) {
  if (j.state !== 'running' || j.progressPct === null) return null
  return (
    <div className="mt-1.5 flex items-center gap-2">
      <div className="h-1 w-24 overflow-hidden rounded-full bg-track" role="img" aria-label={`Progression ${Math.round(j.progressPct)} %`}>
        <div className="h-full rounded-full bg-accent transition-[width] duration-500" style={{ width: `${j.progressPct}%` }} />
      </div>
      <span className="num text-[11px] text-ink-3">
        {j.processed !== null && j.total !== null ? `${j.processed}/${j.total}` : `${Math.round(j.progressPct)} %`}
      </span>
    </div>
  )
}

export function JobsPanel() {
  const [filter, setFilter] = useState<AdminJobFilter>('active')
  const jobs = useAdminJobs(filter)
  const ctl = useAdminControl()
  const { toast } = useToast()
  const [cancel, setCancel] = useState<AdminJob | null>(null)
  const items = jobs.data?.items ?? []

  const retry = (j: AdminJob) =>
    ctl.retryJob.mutate(j.id, {
      onSuccess: () => toast({ title: 'Tâche relancée', description: `${j.label} · ${j.userName}` }),
      onError: (e) => toast({ tone: 'error', title: 'Relance impossible', description: e.message }),
    })
  const busy = (j: AdminJob) => (ctl.retryJob.isPending && ctl.retryJob.variables === j.id) || (ctl.cancelJob.isPending && ctl.cancelJob.variables === j.id)

  const state = (j: AdminJob) => (
    <span className="inline-flex flex-col items-start gap-0.5">
      <Badge level={JOB_STATE[j.state].level}>{JOB_STATE[j.state].label}</Badge>
      {j.maxAttempts > 1 && <span className="num text-[11px] text-ink-3">essai {j.attempts}/{j.maxAttempts}</span>}
    </span>
  )
  const rowActions = (j: AdminJob) => (
    <span className="flex justify-end gap-1.5">
      {j.retryable && (
        <button type="button" className="btn btn-ghost min-h-9 px-3 text-xs" disabled={busy(j)} onClick={() => retry(j)}>
          {busy(j) ? <Loader2 className="size-3.5 animate-spin" aria-hidden="true" /> : <RotateCcw className="size-3.5" aria-hidden="true" />}
          Relancer<span className="sr-only"> {j.label} de {j.userName}</span>
        </button>
      )}
      {j.cancellable && (
        <button type="button" className="btn btn-danger min-h-9 px-3 text-xs" disabled={busy(j)} onClick={() => setCancel(j)}>
          <Ban className="size-3.5" aria-hidden="true" />
          Annuler<span className="sr-only"> {j.label} de {j.userName}</span>
        </button>
      )}
    </span>
  )
  const what = (j: AdminJob) => (
    <>
      <span className="block font-medium text-ink">{j.label}</span>
      <span className="block font-mono text-[11px] text-ink-3">{j.kind}</span>
      <Progress j={j} />
    </>
  )

  return (
    <div className="space-y-5">
      <Card
        title="Tâches de fond"
        icon={<ListChecks className="size-4" />}
        subtitle="Tous les comptes · actualisation automatique"
        action={
          <button type="button" className="icon-btn size-9" onClick={() => void jobs.refetch()} disabled={jobs.isFetching} aria-label="Actualiser les tâches">
            <RefreshCw className={cn('size-4', jobs.isFetching && 'animate-spin')} aria-hidden="true" />
          </button>
        }
      >
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <FilterChips<AdminJobFilter>
            ariaLabel="Filtrer les tâches"
            value={filter}
            onChange={setFilter}
            options={[
              { value: 'active', label: 'En cours' },
              { value: 'failed', label: 'En échec' },
              { value: 'all', label: 'Toutes' },
            ]}
          />
          {jobs.data && (
            <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink-3" aria-label="Charge des files">
              {jobs.data.groups.map((g) => (
                <li key={g.group} className="num">
                  <span className="text-ink-2">{JOB_GROUP_LABEL[g.group] ?? g.group}</span> {g.running}/{g.workers}
                  {g.queued > 0 && ` · ${g.queued} en attente`}
                  {g.delayed > 0 && ` · ${g.delayed} à réessayer`}
                </li>
              ))}
            </ul>
          )}
        </div>

        {jobs.isError ? (
          <ErrorView error={jobs.error} onRetry={() => void jobs.refetch()} title="Tâches indisponibles" />
        ) : !jobs.data ? (
          <div className="space-y-2">
            <Skeleton className="h-14" />
            <Skeleton className="h-14" />
          </div>
        ) : items.length === 0 ? (
          <EmptyState icon={<ListChecks />} title={EMPTY[filter].title}>
            {EMPTY[filter].body}
          </EmptyState>
        ) : (
          <>
            <div className="scroll-thin hidden overflow-x-auto md:block">
              <table className="w-full min-w-[720px] border-separate border-spacing-0 text-sm">
                <caption className="sr-only">Tâches de fond de tous les comptes</caption>
                <thead>
                  <tr className="text-left text-xs text-ink-3">
                    {['Tâche', 'Compte', 'État', 'Mise à jour', 'Message', ''].map((h, i) => (
                      <th key={i} scope="col" className="border-b border-line px-2 py-2 font-medium first:pl-1">
                        {h || <span className="sr-only">Actions</span>}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {items.map((j) => (
                    <tr key={j.id} className="align-top hover:bg-track/50">
                      <th scope="row" className="border-b border-line px-2 py-2 pl-1 text-left font-normal">
                        {what(j)}
                      </th>
                      <td className="border-b border-line px-2 py-2 whitespace-nowrap text-ink-2">{j.userName || `#${j.userId}`}</td>
                      <td className="border-b border-line px-2 py-2">{state(j)}</td>
                      <td className="num border-b border-line px-2 py-2 whitespace-nowrap text-ink-2">{fmtWhen(j.updatedAt)}</td>
                      <td className="max-w-[18rem] border-b border-line px-2 py-2 text-xs text-ink-2">{j.error ?? <span className="text-ink-3">—</span>}</td>
                      <td className="border-b border-line px-2 py-2 text-right">{rowActions(j)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <ul className="space-y-2 md:hidden">
              {items.map((j) => (
                <li key={j.id} className="glass-inset p-3 text-sm">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">{what(j)}</div>
                    {state(j)}
                  </div>
                  <p className="num mt-1 text-xs text-ink-3">
                    {j.userName || `#${j.userId}`} · {fmtWhen(j.updatedAt)}
                  </p>
                  {j.error && <p className="mt-1 text-xs text-ink-2">{j.error}</p>}
                  {(j.retryable || j.cancellable) && <div className="mt-2">{rowActions(j)}</div>}
                </li>
              ))}
            </ul>
            {items.length >= 100 && <p className="mt-3 text-xs text-ink-3">Les {fmtNumber(100)} tâches les plus récentes sont affichées.</p>}
          </>
        )}
      </Card>

      <ConfirmDialog
        open={cancel !== null}
        onClose={() => setCancel(null)}
        title="Annuler cette tâche ?"
        message={
          <p>
            « {cancel?.label} » de {cancel?.userName}. Une tâche en attente est retirée de la file ; une tâche en cours est interrompue et marquée en échec sans nouvel essai.
          </p>
        }
        confirmLabel="Annuler la tâche"
        danger
        busy={ctl.cancelJob.isPending}
        onConfirm={() =>
          cancel &&
          ctl.cancelJob.mutate(cancel.id, {
            onSuccess: () => {
              toast({ title: 'Tâche annulée', description: `${cancel.label} · ${cancel.userName}` })
              setCancel(null)
            },
            onError: (e) => toast({ tone: 'error', title: 'Annulation impossible', description: e.message }),
          })
        }
      />
    </div>
  )
}
