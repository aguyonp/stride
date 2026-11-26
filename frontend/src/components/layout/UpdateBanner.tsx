import { Download, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useInboxMutations, useUpdates } from '../../api/queries'
import type { Me } from '../../api/types'
import { t } from '../../i18n'

/** `later` in sessionStorage: « Plus tard » hides the banner until the next visit. */
const LATER_KEY = 'stride.update.later'

/**
 * Release banner (§22, issue #143): a newer GitHub release than the running binary, for the
 * administrators only. It disappears once the notice is read (here or from the centre), and
 * « Plus tard » postpones it without marking anything read.
 */
export function UpdateBanner({ me }: { me: Me }) {
  const admin = me.role === 'admin'
  const updates = useUpdates(admin)
  const inbox = useInboxMutations()
  const [dismissed, setDismissed] = useState(false)

  // A new release must be able to show up after a « Plus tard » on a previous one.
  useEffect(() => {
    if (!updates.data?.latest) return
    try {
      if (sessionStorage.getItem(LATER_KEY) !== updates.data.latest) setDismissed(false)
    } catch {
      // Private mode: no storage, the banner simply stays.
    }
  }, [updates.data?.latest])

  const st = updates.data
  if (!admin || !st?.available || st.read || dismissed || !st.url) return null

  const later = () => {
    setDismissed(true)
    try {
      sessionStorage.setItem(LATER_KEY, st.latest ?? '')
    } catch {
      // ignore: the banner is hidden for this page anyway
    }
  }

  const open = () => {
    // Opening the release is reading the notice: the row stays in the history, marked read.
    if (st.notificationId) inbox.markRead.mutate([st.notificationId])
    window.open(st.url, '_blank', 'noopener,noreferrer')
  }

  return (
    <div className="mx-auto w-full max-w-[1440px] px-4 pt-3 sm:px-6 lg:px-8" role="status" aria-live="polite">
      <div className="glass flex flex-wrap items-center gap-3 border-accent/30 p-3 sm:px-4">
        <span className="grid size-8 flex-none place-items-center rounded-xl bg-accent/12 text-accent">
          <Download className="size-4" aria-hidden="true" />
        </span>
        <p className="min-w-0 flex-1 text-sm">
          <span className="font-semibold">{t('layout.updates.title')}</span>{' '}
          <span className="text-ink-2">{t('layout.updates.body', { version: st.latest ?? '', current: st.current })}</span>
        </p>
        <div className="flex items-center gap-2">
          <button type="button" className="btn btn-primary h-9 px-3 text-sm" onClick={open}>
            {t('layout.updates.open')}
          </button>
          <button type="button" className="btn btn-ghost h-9 px-3 text-sm" onClick={later}>
            <X className="size-4" aria-hidden="true" /> {t('layout.updates.later')}
          </button>
        </div>
      </div>
    </div>
  )
}
