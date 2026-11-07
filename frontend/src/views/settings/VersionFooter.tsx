import { Check, Copy, MessageSquarePlus } from 'lucide-react'
import { useState } from 'react'
import { useVersion } from '../../api/queries'
import { copyText } from '../../lib/clipboard'
import { ui } from '../../state/ui'

export function VersionFooter() {
  const v = useVersion()
  const [copied, setCopied] = useState(false)
  const feedback = (
    <p className="flex justify-center pt-2">
      <button type="button" className="inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-xs font-medium text-ink-2 hover:bg-track hover:text-ink" onClick={ui.openFeedback}>
        <MessageSquarePlus className="size-3.5" aria-hidden="true" />
        Donner un avis
      </button>
    </p>
  )
  if (!v.data) return feedback
  const text = `Stride ${v.data.label} (${v.data.go})`
  return (
    <>
      {feedback}
      <p className="flex items-center justify-center gap-1.5 pt-1 text-[11px] text-ink-3" aria-label="Version / Diagnostic">
        <span>Version</span>
        <code className="font-mono">{v.data.label}</code>
        <button
          type="button"
          className="rounded-md p-1 hover:bg-track hover:text-ink-2"
          aria-label="Copier la version"
          title="Copier la version"
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
