import { DatabaseBackup, Download, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { api } from '../../api/client'
import { useRestore } from '../../api/queries'
import { downloadBlob } from '../../components/profile/download'
import { RestoreForm } from '../../components/profile/RestoreForm'
import { useToast } from '../../components/ui/Toast'
import { Section } from './Section'
import { t } from '../../i18n'

export function BackupSection() {
  const { toast } = useToast()
  const restore = useRestore()
  const [busy, setBusy] = useState(false)

  const exportJson = async () => {
    setBusy(true)
    try {
      const { blob, filename } = await api.backup()
      downloadBlob(blob, filename)
      toast({ title: t('settings.backup.downloaded'), description: filename })
    } catch (e) {
      toast({ tone: 'error', title: t('settings.backup.exportFailed'), description: e instanceof Error ? e.message : undefined })
    } finally {
      setBusy(false)
    }
  }

  return (
    <Section id="sauvegarde" title={t('settings.backup.title')} icon={<DatabaseBackup />} description={t('settings.backup.description')}>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="glass-inset p-4">
          <h3 className="text-sm font-semibold text-ink">{t('settings.backup.export')}</h3>
          <p className="mt-1 mb-3 text-xs text-ink-3">{t('settings.backup.exportHint')}</p>
          <button type="button" className="btn btn-primary" onClick={() => void exportJson()} disabled={busy}>
            {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Download className="size-4" aria-hidden="true" />}
            {t('settings.backup.exportJson')}
          </button>
        </div>
        <div className="glass-inset p-4">
          <h3 className="text-sm font-semibold text-ink">{t('settings.backup.restore')}</h3>
          <p className="mt-1 mb-3 text-xs text-ink-3">{t('settings.backup.restoreHint')}</p>
          <RestoreForm onRestore={(mode, backup) => restore.mutateAsync({ mode, backup })} />
        </div>
      </div>
    </Section>
  )
}
