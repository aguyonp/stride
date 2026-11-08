import { Loader2 } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { Dialog } from './Dialog'
import { t, tx } from '../../i18n'

interface ConfirmDialogProps {
  open: boolean
  onClose: () => void
  title: string
  message: ReactNode
  confirmLabel: string
  danger?: boolean
  requireText?: string
  busy?: boolean
  onConfirm: () => void
}

export function ConfirmDialog({ open, onClose, title, message, confirmLabel, danger, requireText, busy, onConfirm }: ConfirmDialogProps) {
  const [typed, setTyped] = useState('')
  const blocked = requireText !== undefined && typed.trim() !== requireText
  return (
    <Dialog
      open={open}
      onClose={() => {
        setTyped('')
        onClose()
      }}
      title={title}
      footer={
        <div className="flex justify-end gap-2">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button type="button" className={danger ? 'btn btn-danger' : 'btn btn-primary'} disabled={blocked || busy} onClick={onConfirm}>
            {busy && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {confirmLabel}
          </button>
        </div>
      }
    >
      <div className="space-y-3 text-sm text-ink-2">
        {message}
        {requireText !== undefined && (
          <label className="block">
            <span className="mb-1.5 block text-xs text-ink-3">
              {tx('common.ui.typeToConfirm', { text: <strong className="font-mono text-ink">{requireText}</strong> })}
            </span>
            <input className="field" value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" spellCheck={false} />
          </label>
        )}
      </div>
    </Dialog>
  )
}
