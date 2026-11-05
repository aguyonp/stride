import { FileUp, Loader2, RotateCcw } from 'lucide-react'
import { useId, useRef, useState } from 'react'
import type { RestoreResult } from '../../api/types'
import { fmtNumber } from '../../lib/format'
import { ConfirmDialog } from '../ui/ConfirmDialog'
import { InlineError } from '../ui/Misc'
import { SegmentedControl } from '../ui/SegmentedControl'
import { fmtDay, inspectBackup, readFileText, type BackupSummary } from './download'

type Mode = 'merge' | 'replace'

export function restoreSummary(r: RestoreResult): string {
  const i = r.imported
  const parts = [
    `${fmtNumber(i.goals)} objectif${i.goals > 1 ? 's' : ''}`,
    `${fmtNumber(i.plan)} séance${i.plan > 1 ? 's' : ''} prévue${i.plan > 1 ? 's' : ''}`,
    `${fmtNumber(i.shoes)} chaussure${i.shoes > 1 ? 's' : ''}`,
    `${fmtNumber(i.notes)} ressenti${i.notes > 1 ? 's' : ''}`,
    `${fmtNumber(i.checkins)} check-in${i.checkins > 1 ? 's' : ''}`,
  ]
  if (i.profile) parts.push('profil')
  return parts.join(' · ')
}

/** Pick a stride-backup JSON, validate it locally, then merge or replace (replace needs confirmation). */
export function RestoreForm({ onRestore, targetLabel = 'ton compte' }: { onRestore: (mode: Mode, text: string) => Promise<RestoreResult>; targetLabel?: string }) {
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
      setError('Impossible de lire ce fichier.')
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
      setError(e instanceof Error ? e.message : 'Restauration impossible.')
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
          {file ? 'Choisir un autre fichier' : 'Choisir un fichier .json'}
        </label>
      </div>
      {error && <InlineError>{error}</InlineError>}
      {file && s && (
        <div className="glass-inset space-y-3 p-3">
          <div className="min-w-0 text-sm">
            <p className="truncate font-semibold text-ink">{file.name}</p>
            <p className="text-xs text-ink-3">
              Exportée le {fmtDay(s.exportedAt)} · {s.goals} objectifs · {s.plan} séances prévues · {s.shoes} chaussures · {s.notes} ressentis · {s.checkins} check-ins
            </p>
          </div>
          <SegmentedControl
            ariaLabel="Mode de restauration"
            size="sm"
            value={mode}
            onChange={setMode}
            options={[
              { value: 'merge', label: 'Fusionner' },
              { value: 'replace', label: 'Remplacer' },
            ]}
          />
          <p className="text-xs text-ink-3">
            {mode === 'merge'
              ? 'Ajoute ou met à jour les éléments de la sauvegarde sans rien supprimer.'
              : `Efface les données manuelles actuelles de ${targetLabel} (profil, objectifs, plan, chaussures, ressentis, check-ins) puis importe la sauvegarde.`}
          </p>
          <button type="button" className={mode === 'replace' ? 'btn btn-danger' : 'btn btn-primary'} disabled={busy} onClick={() => (mode === 'replace' ? setConfirm(true) : void run())}>
            {busy ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <RotateCcw className="size-4" aria-hidden="true" />}
            Restaurer
          </button>
        </div>
      )}
      {result && (
        <p role="status" className="rounded-xl border border-ok/30 bg-ok/10 px-3 py-2 text-sm text-ink">
          Restauration terminée : {result}.
        </p>
      )}
      <ConfirmDialog
        open={confirm}
        onClose={() => setConfirm(false)}
        title="Remplacer les données ?"
        message={<p>Les données manuelles actuelles de {targetLabel} seront supprimées puis remplacées par le contenu de la sauvegarde. Cette action est irréversible.</p>}
        confirmLabel="Remplacer"
        danger
        busy={busy}
        onConfirm={() => void run()}
      />
    </div>
  )
}
