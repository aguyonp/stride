// Tolerant localStorage access (private mode, blocked storage). UI preferences only, never secrets.

export function readStored(key: string): string | null {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

export function writeStored(key: string, value: string | null) {
  try {
    if (value === null) window.localStorage.removeItem(key)
    else window.localStorage.setItem(key, value)
  } catch {
    /* ignore */
  }
}

export const STORAGE_KEYS = {
  theme: 'stride.theme',
  include: 'stride.include',
  multiInclude: 'stride.multiInclude',
  promptSubject: 'stride.promptSubject',
  lang: 'stride.lang',
} as const
