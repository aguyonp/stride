import { useCallback } from 'react'
import { api } from '../../api/client'
import { canShare, copyText, downloadText, shareText } from '../../lib/clipboard'
import { fmtNumber } from '../../lib/format'
import { useToast } from '../ui/Toast'

export interface ReportPayload {
  report: string
  tokens: number
  filename: string
  title: string
}

export function useReportExport(payload: ReportPayload | null, onCopyFailed?: () => void) {
  const { toast } = useToast()

  const copy = useCallback(async () => {
    if (!payload) return
    const ok = await copyText(payload.report)
    if (ok) {
      void api.trackAction('prompt.copy').catch(() => {})
      toast({ title: `Prompt copié ✓ · ≈ ${fmtNumber(payload.tokens)} tokens`, description: 'Colle-le dans ta conversation IA.' })
    } else {
      toast({ tone: 'error', title: 'Copie impossible', description: 'Le navigateur bloque le presse-papiers : sélectionne le texte dans l’aperçu.' })
      onCopyFailed?.()
    }
  }, [payload, toast, onCopyFailed])

  const download = useCallback(() => {
    if (!payload) return
    downloadText(payload.filename, payload.report)
    void api.trackAction('prompt.download').catch(() => {})
    toast({ title: 'Fichier téléchargé', description: payload.filename })
  }, [payload, toast])

  const share = useCallback(async () => {
    if (!payload) return
    const res = await shareText(payload.title, payload.report)
    if (res === 'error') toast({ tone: 'error', title: 'Partage impossible' })
    else if (res === 'shared') void api.trackAction('prompt.share').catch(() => {})
  }, [payload, toast])

  return { copy, download, share, canShare: canShare() }
}
