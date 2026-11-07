import { HeartPulse, Loader2, Pencil } from 'lucide-react'
import { useState } from 'react'
import { useHealthStatus, useSaveHealthStatus } from '../../api/queriesV41'
import type { HealthStatusInput } from '../../api/types'
import { cn } from '../../lib/cn'
import { Card } from '../ui/Card'
import { InlineError } from '../ui/Misc'
import { Skeleton } from '../ui/Skeleton'
import { useToast } from '../ui/Toast'
import { HealthStatusForm } from './HealthStatusForm'
import { draftFrom, HEALTH_MODE_LABEL, toHealthInput, validateHealthDraft, zoneText } from './healthStatus'

/** Single place where the health status is shown and changed (golden source for presets, prompts and the Ressenti form). */
export function HealthStatusCard({ today }: { today: string }) {
  const status = useHealthStatus()
  const save = useSaveHealthStatus()
  const { toast } = useToast()
  const [draft, setDraft] = useState<HealthStatusInput | null>(null)
  const [error, setError] = useState<string | null>(null)
  const s = status.data

  const submit = () => {
    if (!draft) return
    const invalid = validateHealthDraft(draft)
    setError(invalid)
    if (invalid) return
    save.mutate(toHealthInput(draft), {
      onSuccess: () => {
        setDraft(null)
        toast({ title: 'État de santé enregistré', tone: 'success' })
      },
      onError: (e) => setError(e.message),
    })
  }

  return (
    <Card
      title="État de santé"
      icon={<HeartPulse className="size-4" />}
      action={
        s && !draft ? (
          <button type="button" className="btn btn-ghost h-9 min-h-9 px-3 text-xs" onClick={() => setDraft(draftFrom(s))}>
            <Pencil className="size-3.5" aria-hidden="true" /> Modifier
          </button>
        ) : undefined
      }
    >
      {status.isError ? (
        <InlineError>{status.error.message}</InlineError>
      ) : !s ? (
        <Skeleton className="h-10" />
      ) : draft ? (
        <div className="space-y-3">
          <HealthStatusForm value={draft} onChange={setDraft} today={today} />
          {error && <InlineError>{error}</InlineError>}
          <div className="flex justify-end gap-2">
            <button type="button" className="btn btn-ghost" onClick={() => (setDraft(null), setError(null))}>
              Annuler
            </button>
            <button type="button" className="btn btn-primary" onClick={submit} disabled={save.isPending}>
              {save.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
              Enregistrer
            </button>
          </div>
        </div>
      ) : (
        <div className="flex flex-wrap items-center gap-2">
          <span className={cn('chip border', s.mode === 'performance' ? 'border-ok/35 bg-ok/12 text-ok' : 'border-warn/35 bg-warn/12 text-warn')}>{HEALTH_MODE_LABEL[s.mode]}</span>
          {s.mode === 'tracking' &&
            s.zones.map((z) => (
              <span key={z.zone} className="chip border border-line bg-track/60 text-ink-2">
                {zoneText(z)}
              </span>
            ))}
        </div>
      )}
    </Card>
  )
}
