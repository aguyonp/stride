import { useQueryClient } from '@tanstack/react-query'
import { Fragment, useEffect, useRef, type ReactNode } from 'react'
import { useI18n } from '.'

/**
 * Remounts its subtree when the language changes (labels computed at module or memo level follow)
 * and refetches queries, whose payloads may depend on `Accept-Language` (catalog, share labels).
 */
export function I18nBoundary({ children }: { children: ReactNode }) {
  const { lang } = useI18n()
  const qc = useQueryClient()
  const prev = useRef(lang)
  useEffect(() => {
    if (prev.current === lang) return
    prev.current = lang
    void qc.invalidateQueries()
  }, [lang, qc])
  return <Fragment key={lang}>{children}</Fragment>
}
