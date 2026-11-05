import { useSyncExternalStore } from 'react'
import { readStored, STORAGE_KEYS, writeStored } from '../lib/storage'

export type ThemePref = 'system' | 'light' | 'dark'
export type Theme = 'light' | 'dark'

const THEME_COLOR: Record<Theme, string> = { dark: '#070a13', light: '#eaeff7' }
const listeners = new Set<() => void>()

function readPref(): ThemePref {
  const raw = readStored(STORAGE_KEYS.theme)
  return raw === 'light' || raw === 'dark' ? raw : 'system'
}

let pref: ThemePref = typeof window === 'undefined' ? 'system' : readPref()

const mq = () => (typeof window !== 'undefined' ? window.matchMedia?.('(prefers-color-scheme: light)') : undefined)
const systemTheme = (): Theme => (mq()?.matches ? 'light' : 'dark')
const resolved = (): Theme => (pref === 'system' ? systemTheme() : pref)

export function applyTheme() {
  const theme = resolved()
  const root = document.documentElement
  root.dataset.theme = theme
  root.style.colorScheme = theme
  document.querySelectorAll('meta[name="theme-color"]').forEach((m) => m.setAttribute('content', THEME_COLOR[theme]))
}

function emit() {
  applyTheme()
  listeners.forEach((l) => l())
}

function subscribe(cb: () => void) {
  listeners.add(cb)
  const m = mq()
  const onChange = () => emit()
  m?.addEventListener('change', onChange)
  return () => {
    listeners.delete(cb)
    m?.removeEventListener('change', onChange)
  }
}

function setThemePref(p: ThemePref) {
  pref = p
  writeStored(STORAGE_KEYS.theme, p === 'system' ? null : p)
  emit()
}

export function useTheme() {
  const current = useSyncExternalStore(subscribe, () => `${pref}:${resolved()}`)
  const [p, theme] = current.split(':') as [ThemePref, Theme]
  return { pref: p, theme, setPref: setThemePref, toggle: () => setThemePref(theme === 'dark' ? 'light' : 'dark') }
}
