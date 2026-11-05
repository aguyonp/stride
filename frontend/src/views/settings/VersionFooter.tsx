import { Check, Copy } from 'lucide-react'
import { useState } from 'react'
import { useVersion } from '../../api/queries'
import { copyText } from '../../lib/clipboard'

export function VersionFooter() {
  const v = useVersion()
  const [copied, setCopied] = useState(false)
  if (!v.data) return null
  const text = `Stride ${v.data.label} (${v.data.go})`
  return (
    <p className="flex items-center justify-center gap-1.5 pt-2 text-[11px] text-ink-3" aria-label="Version / Diagnostic">
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
  )
}
