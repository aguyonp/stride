import { useMutation, useQueryClient } from '@tanstack/react-query'
import { FileUp, Loader2, Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { Link } from 'wouter'
import { api } from '../../api/client'
import { invalidateDomain } from '../../api/queries'
import type { ImportResult } from '../../api/types'
import { InlineError } from '../../components/ui/Misc'
import { isJobStatus, upsertJobInStore, useEventStore } from '../../state/events'
import { Section } from './Section'

const MAX = 25 * 1024 * 1024

/** Discreet rescue import of a .fit / .gpx / .tcx file (deduplicated server-side, async worker aware). */
export function ImportSection() {
  const qc = useQueryClient()
  const input = useRef<HTMLInputElement>(null)
  const [tooBig, setTooBig] = useState(false)
  const [jobId, setJobId] = useState<string | null>(null)
  const [directRes, setDirectRes] = useState<ImportResult | null>(null)
  const job = useEventStore((s) => (jobId ? (s.jobs[jobId] ?? null) : null))

  const imp = useMutation({
    mutationFn: (f: File) => api.importFileAsync(f, true),
    onSuccess: (res) => {
      if (isJobStatus(res)) {
        upsertJobInStore(res)
        setJobId(res.id)
        return
      }
      setDirectRes(res)
      void invalidateDomain(qc)
    },
  })

  const jobRunning = Boolean(job && (job.state === 'queued' || job.state === 'running' || job.state === 'retrying'))
  const jobFailed = job?.state === 'failed' ? (job.error ?? 'Échec de l’import.') : null
  const jobResult: ImportResult | null =
    job?.state === 'completed' && job.result && typeof job.result.activityId === 'number'
      ? { activityId: job.result.activityId, merged: Boolean(job.result.merged) }
      : null
  const res = directRes ?? jobResult
  const busy = imp.isPending || jobRunning

  return (
    <Section
      id="import"
      title="Import de secours"
      icon={<FileUp />}
      description="Quand une synchro échoue : importe le fichier exporté de ta montre ou d’une autre app. Les doublons sont fusionnés automatiquement."
    >
      <div className="flex flex-wrap items-center gap-3">
        <input
          ref={input}
          type="file"
          accept=".fit,.gpx,.tcx"
          className="sr-only"
          id="rescue-file"
          onChange={(e) => {
            const f = e.target.files?.[0]
            e.target.value = ''
            if (!f) return
            imp.reset()
            setJobId(null)
            setDirectRes(null)
            setTooBig(f.size > MAX)
            if (f.size <= MAX) imp.mutate(f)
          }}
        />
        <button type="button" className="btn btn-ghost" onClick={() => input.current?.click()} disabled={busy}>
          {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Upload className="size-4" aria-hidden="true" />}
          Choisir un fichier
        </button>
        <span className="text-xs text-ink-3">.fit, .gpx ou .tcx · 25 Mo max</span>
      </div>
      <div aria-live="polite" className="mt-3">
        {tooBig && <InlineError>Fichier trop volumineux (25 Mo max).</InlineError>}
        {(imp.isError || jobFailed) && <InlineError>{jobFailed ?? imp.error?.message}</InlineError>}
        {jobRunning && job && <p className="num text-sm text-ink-2">{job.label}… traitement en priorité critique.</p>}
        {res && (
          <p className="text-sm text-ink-2">
            {res.merged ? 'Fusionné avec une séance existante.' : 'Séance importée.'}{' '}
            <Link href={`/?s=${res.activityId}`} className="font-semibold text-accent underline underline-offset-2">
              Voir la séance
            </Link>
          </p>
        )}
      </div>
    </Section>
  )
}
