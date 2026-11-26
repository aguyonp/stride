import { Loader2, Send } from 'lucide-react'
import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { useGarminBridge } from '../../api/queries'
import type { PushTodayResult } from '../../api/types'
import { fmtDateLong } from '../../lib/format'
import { t } from '../../i18n'
import { Dialog } from '../ui/Dialog'
import { useToast } from '../ui/Toast'

/**
 * « Send today's sessions » (§23, issue #147): the bulk action of the planning.
 *
 * Only shown with a connected bridge — the Personal API cannot push a workout, so offering it there
 * would be a dead button. The result is reported per session, never as a single pass/fail.
 */
export function PushTodayButton({ date }: { date: string }) {
  const bridge = useGarminBridge()
  const { toast } = useToast()
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [results, setResults] = useState<PushTodayResult[] | null>(null)

  if (bridge.data?.state !== 'connected') return null

  const run = async () => {
    setBusy(true)
    try {
      const out = await api.pushToday(date)
      setResults(out.results)
      const sent = out.results.filter((r) => r.status === 'pushed').length
      const failed = out.results.filter((r) => r.status === 'failed' || r.status === 'unsupported').length
      const skipped = out.results.filter((r) => r.status === 'skipped').length
      if (out.results.length === 0) toast({ title: t('plan.view.pushTodayNone') })
      else
        toast({
          title: t('plan.view.pushTodayDone'),
          description: t('plan.view.pushTodaySummary', { sent, failed, skipped }),
          tone: failed > 0 ? 'error' : 'success',
        })
      void qc.invalidateQueries({ queryKey: ['plan'] })
    } catch (e) {
      toast({ title: t('plan.view.pushTodayDone'), description: e instanceof Error ? e.message : undefined, tone: 'error' })
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <button type="button" className="btn btn-ghost" onClick={() => setOpen(true)}>
        <Send className="size-4 text-accent" aria-hidden="true" /> {t('plan.view.pushToday')}
      </button>
      <Dialog open={open} onClose={() => (busy ? undefined : setOpen(false))} title={t('plan.view.pushTodayTitle')} size="md">
        <p className="text-sm text-ink-2">{t('plan.view.pushTodayBody', { date: fmtDateLong(date) })}</p>
        {results && (
          <ul className="mt-3 space-y-1 text-sm">
            {results.map((r) => (
              <li key={r.sessionId} className="flex items-baseline gap-2">
                <span className={r.status === 'pushed' ? 'text-ok' : r.status === 'skipped' ? 'text-ink-3' : 'text-danger'}>•</span>
                <span className="min-w-0 flex-1 truncate">{r.title || `#${r.sessionId}`}</span>
                <span className="flex-none text-xs text-ink-3">{r.message || r.status}</span>
              </li>
            ))}
          </ul>
        )}
        <div className="mt-4 flex justify-end gap-2">
          <button type="button" className="btn btn-ghost" onClick={() => setOpen(false)} disabled={busy}>
            {t('common.actions.close')}
          </button>
          <button type="button" className="btn btn-primary" onClick={() => void run()} disabled={busy}>
            {busy && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {t('plan.view.pushTodayRun')}
          </button>
        </div>
      </Dialog>
    </>
  )
}
