import { Check, Copy, MessageSquarePlus } from 'lucide-react'
import { useState } from 'react'
import { useVersion } from '../../api/queries'
import { copyText } from '../../lib/clipboard'
import { ui } from '../../state/ui'
import { t } from '../../i18n'

export function VersionFooter() {
  const v = useVersion()
  const [copied, setCopied] = useState(false)
  const feedback = (
    <p className="flex justify-center pt-2">
      <button type="button" className="inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-xs font-medium text-ink-2 hover:bg-track hover:text-ink" onClick={ui.openFeedback}>
        <MessageSquarePlus className="size-3.5" aria-hidden="true" />
        {t('settings.account.feedback')}
      </button>
    </p>
  )
  if (!v.data) return feedback
  const text = `Stride ${v.data.label} (${v.data.go})`
  return (
    <>
      {feedback}
      <p className="flex items-center justify-center gap-1.5 pt-1 text-[11px] text-ink-3" aria-label={t('settings.account.diagnostic')}>
        <span>{t('settings.account.version')}</span>
        <code className="font-mono">{v.data.label}</code>
        <button
          type="button"
          className="rounded-md p-1 hover:bg-track hover:text-ink-2"
          aria-label={t('settings.account.copyVersion')}
          title={t('settings.account.copyVersion')}
          onClick={async () => {
            if (await copyText(text)) {
              setCopied(true)
              window.setTimeout(() => setCopied(false), 1500)
            }
          }}
        >
          {copied ? <Check className="size-3.5" aria-hidden="true" /> : <Copy className="size-3.5" aria-hidden="true" />}
        </button>
      </p>
    </>
  )
}
