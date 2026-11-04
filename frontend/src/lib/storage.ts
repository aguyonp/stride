// Accès localStorage tolérant (navigation privée, stockage bloqué…).
// Ne stocke QUE des préférences d'interface — jamais de clé ni de secret.

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
  mode: 'stride.mode',
  theme: 'stride.theme',
  history: 'stride.history14',
} as const
