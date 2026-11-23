import { Check, Copy, Loader2, Plus, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { useRecoveryMutations, useRecoveryStatus } from '../../api/queries'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { InlineError } from '../../components/ui/Misc'
import { Skeleton } from '../../components/ui/Skeleton'
import { useToast } from '../../components/ui/Toast'
import { copyText } from '../../lib/clipboard'
import { ErrorView } from '../StateViews'
import { Section } from './Section'
import { t } from '../../i18n'

export function RecoverySection() {
  const status = useRecoveryStatus()
  const { generate } = useRecoveryMutations()
  const { toast } = useToast()
  const [fresh, setFresh] = useState<string[] | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [copied, setCopied] = useState<string | null>(null)
  const remaining = status.data?.remaining ?? 0

  const doGenerate = () =>
    generate.mutate(undefined, {
      onSuccess: (set) => {
        setFresh(set.codes)
        setConfirmOpen(false)
        toast({ title: t('settings.recovery.generated') })
      },
      onError: (e) => {
        setConfirmOpen(false)
        toast({ tone: 'error', title: remaining > 0 ? t('settings.recovery.regenerateFailed') : t('settings.recovery.generateFailed'), description: e.message })
      },
    })

  const copy = async (c: string) => {
    if (await copyText(c)) {
      setCopied(c)
      setTimeout(() => setCopied((v) => (v === c ? null : v)), 2000)
    }
  }

  return (
    <Section
      id="recovery"
      title={t('settings.recovery.title')}
      icon={<ShieldCheck />}
      description={t('settings.recovery.description')}
      actions={
        <button type="button" className="btn btn-primary" disabled={generate.isPending} onClick={() => (remaining > 0 ? setConfirmOpen(true) : doGenerate())}>
          {generate.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Plus className="size-4" aria-hidden="true" />}
          {remaining > 0 ? t('settings.recovery.regenerate') : t('settings.recovery.generate')}
        </button>
      }
    >
      {generate.isError && <InlineError className="mb-3">{String(generate.error)}</InlineError>}
      {status.isError ? (
        <ErrorView error={status.error} onRetry={() => void status.refetch()} title={t('settings.recovery.unavailable')} />
      ) : !status.data && !fresh ? (
        <div className="space-y-2">
          <Skeleton className="h-16" />
        </div>
      ) : (
        <>
          {fresh ? (
            <div className="mb-3 rounded-2xl border border-warn/35 bg-warn/10 p-3.5">
              <p className="text-sm font-semibold text-ink">{t('settings.recovery.showOnce')}</p>
              <ul className="mt-2 grid gap-1.5 sm:grid-cols-2">
                {fresh.map((c) => (
                  <li key={c} className="flex items-center gap-2">
                    <code className="min-w-0 flex-1 truncate font-mono text-sm tracking-wider">{c}</code>
                    <button type="button" className="icon-btn size-8 flex-none" onClick={() => void copy(c)} aria-label={t('settings.recovery.copy', { code: c })}>
                      {copied === c ? <Check className="size-4 text-ok" aria-hidden="true" /> : <Copy className="size-4" aria-hidden="true" />}
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          ) : (
            <p className="mb-3 text-sm text-ink-2">{remaining > 0 ? t('settings.recovery.remaining', { count: remaining }) : t('settings.recovery.none')}</p>
          )}
        </>
      )}
      <ConfirmDialog
        open={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        title={t('settings.recovery.confirmTitle')}
        message={<p>{t('settings.recovery.confirmBody', { count: remaining })}</p>}
        confirmLabel={t('settings.recovery.regenerate')}
        danger
        busy={generate.isPending}
        onConfirm={doGenerate}
      />
    </Section>
  )
}
