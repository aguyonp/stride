// Active UI language (§15.2). Dependency-free store shared by i18n/index.ts and lib/format.ts.
// Resolution: profile.language → localStorage `stride.lang` → navigator.language (fr* → fr) → en.

import { readStored, STORAGE_KEYS, writeStored } from '../lib/storage'

export type Lang = 'fr' | 'en'
export const LANGS: readonly Lang[] = ['fr', 'en']

export const isLang = (v: unknown): v is Lang => v === 'fr' || v === 'en'

/** BCP 47 locale used for Intl formatting (24 h clock and day-month order in English). */
export const LOCALE: Record<Lang, string> = { fr: 'fr-FR', en: 'en-GB' }

function browserLang(): Lang {
  const nav = typeof navigator !== 'undefined' ? navigator.language : ''
  return /^fr\b/i.test(nav ?? '') ? 'fr' : 'en'
}

/** Language without profile: localStorage → navigator → en. */
export function detectLang(): Lang {
  const stored = typeof window !== 'undefined' ? readStored(STORAGE_KEYS.lang) : null
  return isLang(stored) ? stored : browserLang()
}

/** profile.language wins; null/undefined falls back to detectLang(). */
export function resolveLang(profile?: Lang | null): Lang {
  return isLang(profile) ? profile : detectLang()
}

let current: Lang = detectLang()
const listeners = new Set<(l: Lang) => void>()

export const getLang = (): Lang => current

export function subscribeLang(fn: (l: Lang) => void): () => void {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

function applyDocumentLang(l: Lang) {
  if (typeof document !== 'undefined') document.documentElement.lang = l
}
applyDocumentLang(current)

/** Switches the active language; `persist` (default) also stores it in localStorage. */
export function setLang(l: Lang, opts: { persist?: boolean } = {}) {
  if (opts.persist !== false && typeof window !== 'undefined') writeStored(STORAGE_KEYS.lang, l)
  if (l === current) return
  current = l
  applyDocumentLang(l)
  for (const fn of [...listeners]) fn(l)
}
