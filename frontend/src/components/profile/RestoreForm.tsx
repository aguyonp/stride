import { FileUp, Loader2, RotateCcw } from 'lucide-react'
import { useId, useRef, useState } from 'react'
import type { RestoreResult } from '../../api/types'
import { fmtNumber } from '../../lib/format'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { InlineError } from '../ui/Misc'
import { SegmentedControl } from '../ui/SegmentedControl'
import { fmtDay, inspectBackup, readFileText, type BackupSummary } from './download'
import { t } from '../../i18n'

type Mode = 'merge' | 'replace'

function restoreSummary(r: RestoreResult): string {
  const i = r.imported
  const parts = [
    t('settings.restore.goals', { count: fmtNumber(i.goals) }),
    t('settings.restore.plan', { count: fmtNumber(i.plan) }),
    t('settings.restore.shoes', { count: fmtNumber(i.shoes) }),
    t('settings.restore.notes', { count: fmtNumber(i.notes) }),
    t('settings.restore.checkins', { count: fmtNumber(i.checkins) }),
  ]
  if (i.profile) parts.push(t('settings.restore.profile'))
  return parts.join(' · ')
}

/** Pick a stride-backup JSON, validate it locally, then merge or replace (replace needs confirmation). */
export function RestoreForm({ onRestore, targetLabel = t('settings.restore.yourAccount') }: { onRestore: (mode: Mode, text: string) => Promise<RestoreResult>; targetLabel?: string }) {
  const inputId = useId()
  const fileRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<{ name: string; text: string; summary: BackupSummary } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [mode, setMode] = useState<Mode>('merge')
  const [confirm, setConfirm] = useState(false)
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<string | null>(null)

  const pick = async (f: File | undefined) => {
    setResult(null)
    setError(null)
    setFile(null)
    if (!f) return
    try {
      const text = await readFileText(f)
      const check = inspectBackup(text)
      if (!check.ok) setError(check.error)
      else setFile({ name: f.name, text, summary: check.summary })
    } catch {
      setError(t('settings.restore.readFailed'))
    }
  }

  const run = async () => {
    if (!file) return
    setBusy(true)
    setError(null)
    try {
      const r = await onRestore(mode, file.text)
      setResult(restoreSummary(r))
      setFile(null)
      if (fileRef.current) fileRef.current.value = ''
    } catch (e) {
      setError(e instanceof Error ? e.message : t('settings.restore.failed'))
    } finally {
      setBusy(false)
      setConfirm(false)
    }
  }

  const s = file?.summary
  return (
    <div className="space-y-3">
      <div>
        <input ref={fileRef} id={inputId} type="file" accept="application/json,.json" className="peer sr-only" onChange={(e) => void pick(e.target.files?.[0])} />
        <label htmlFor={inputId} className="btn btn-ghost peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-accent">
          <FileUp className="size-4" aria-hidden="true" />
          {file ? t('settings.restore.pickOther') : t('settings.restore.pick')}
        </label>
      </div>
      {error && <InlineError>{error}</InlineError>}
      {file && s && (
        <div className="glass-inset space-y-3 p-3">
          <div className="min-w-0 text-sm">
            <p className="truncate font-semibold text-ink">{file.name}</p>
            <p className="text-xs text-ink-3">
              {t('settings.restore.fileSummary', { date: fmtDay(s.exportedAt), goals: s.goals, plan: s.plan, shoes: s.shoes, notes: s.notes, checkins: s.checkins })}
            </p>
          </div>
          <SegmentedControl
            ariaLabel={t('settings.restore.mode')}
            size="sm"
            value={mode}
            onChange={setMode}
            options={[
              { value: 'merge', label: t('settings.restore.merge') },
              { value: 'replace', label: t('settings.restore.replace') },
            ]}
          />
          <p className="text-xs text-ink-3">
            {mode === 'merge'
              ? t('settings.restore.mergeHint')
              : t('settings.restore.replaceHint', { target: targetLabel })}
          </p>
          <button type="button" className={mode === 'replace' ? 'btn btn-danger' : 'btn btn-primary'} disabled={busy} onClick={() => (mode === 'replace' ? setConfirm(true) : void run())}>
            {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <RotateCcw className="size-4" aria-hidden="true" />}
            {t('settings.restore.restore')}
          </button>
        </div>
      )}
      {result && (
        <p role="status" className="rounded-xl border border-ok/30 bg-ok/10 px-3 py-2 text-sm text-ink">
          {t('settings.restore.done', { result })}
        </p>
      )}
      <ConfirmDialog
        open={confirm}
        onClose={() => setConfirm(false)}
        title={t('settings.restore.confirmTitle')}
        message={<p>{t('settings.restore.confirmBody', { target: targetLabel })}</p>}
        confirmLabel={t('settings.restore.replace')}
        danger
        busy={busy}
        onConfirm={() => void run()}
      />
    </div>
  )
}
