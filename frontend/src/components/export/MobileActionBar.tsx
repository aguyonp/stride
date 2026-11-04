import { Download, Eye, History, MoreHorizontal, Share2, Sparkles } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { cn } from '../../lib/cn'
import type { ReportPayload } from './useReportExport'

interface MobileActionBarProps {
  payload: ReportPayload | null
  loading: boolean
  withHistory: boolean
  onToggleHistory: (v: boolean) => void
  onCopy: () => void
  onPreview: () => void
  onDownload: () => void
  onShare: () => void
  canShare: boolean
}

/** Barre d'action collante (mobile) : gros bouton d'export + aperçu + menu. */
export function MobileActionBar({ payload, loading, withHistory, onToggleHistory, onCopy, onPreview, onDownload, onShare, canShare }: MobileActionBarProps) {
  const [menu, setMenu] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!menu) return
    const close = (e: Event) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setMenu(false)
    }
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && setMenu(false)
    document.addEventListener('pointerdown', close)
    document.addEventListener('keydown', esc)
    return () => {
      document.removeEventListener('pointerdown', close)
      document.removeEventListener('keydown', esc)
    }
  }, [menu])

  return (
    <div className="pointer-events-none fixed inset-x-0 bottom-0 z-40 lg:hidden">
      <div className="glass-bar pb-safe pointer-events-auto mx-auto rounded-t-[24px] border-b-0 px-3 pt-3 sm:max-w-xl sm:px-4">
        <div className="flex items-center gap-2">
          <button type="button" className="btn btn-primary h-12 min-w-0 flex-1 text-[0.95rem]" onClick={onCopy} disabled={!payload || loading}>
            <Sparkles className="size-[1.125rem] flex-none" aria-hidden="true" />
            <span className="truncate">Exporter pour l’IA</span>
          </button>
          <button type="button" className="icon-btn size-12 rounded-2xl" onClick={onPreview} disabled={!payload} aria-label="Aperçu du rapport">
            <Eye className="size-5" />
          </button>
          <div className="relative" ref={menuRef}>
            <button
              type="button"
              className={cn('icon-btn size-12 rounded-2xl', withHistory && 'text-accent')}
              onClick={() => setMenu((v) => !v)}
              aria-label="Plus d’options d’export"
              aria-haspopup="menu"
              aria-expanded={menu}
            >
              <MoreHorizontal className="size-5" />
              {withHistory && <span className="absolute top-2 right-2 size-2 rounded-full bg-accent" aria-hidden="true" />}
            </button>
            {menu && (
              <div role="menu" className="sheet-panel absolute right-0 bottom-[calc(100%+0.5rem)] w-64 rounded-2xl p-1.5 animate-[pop-in_.18s_var(--ease-out-soft)_both]">
                <button
                  type="button"
                  role="menuitemcheckbox"
                  aria-checked={withHistory}
                  className="flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm hover:bg-track"
                  onClick={() => onToggleHistory(!withHistory)}
                >
                  <History className="size-4 text-ink-2" aria-hidden="true" />
                  <span className="flex-1">Historique 14 j</span>
                  <span className={cn('chip h-5 px-2 text-[10px]', withHistory ? 'bg-accent text-on-accent' : 'bg-track text-ink-3')}>{withHistory ? 'OUI' : 'NON'}</span>
                </button>
                <button
                  type="button"
                  role="menuitem"
                  className="flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm hover:bg-track disabled:opacity-50"
                  disabled={!payload}
                  onClick={() => {
                    setMenu(false)
                    onDownload()
                  }}
                >
                  <Download className="size-4 text-ink-2" aria-hidden="true" />
                  Télécharger (.md)
                </button>
                {canShare && (
                  <button
                    type="button"
                    role="menuitem"
                    className="flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm hover:bg-track disabled:opacity-50"
                    disabled={!payload}
                    onClick={() => {
                      setMenu(false)
                      onShare()
                    }}
                  >
                    <Share2 className="size-4 text-ink-2" aria-hidden="true" />
                    Partager…
                  </button>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
