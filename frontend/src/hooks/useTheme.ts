import { useCallback, useEffect, useState } from 'react'
import { readStored, STORAGE_KEYS, writeStored } from '../lib/storage'

export type ThemePref = 'system' | 'light' | 'dark'
export type Theme = 'light' | 'dark'

const THEME_COLOR: Record<Theme, string> = { dark: '#070a13', light: '#eef2f8' }

function systemTheme(): Theme {
  // Sombre par défaut : on ne passe en clair que si le système le demande explicitement.
  return window.matchMedia?.('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

function readPref(): ThemePref {
  const raw = readStored(STORAGE_KEYS.theme)
  return raw === 'light' || raw === 'dark' ? raw : 'system'
}

export function useTheme() {
  const [pref, setPrefState] = useState<ThemePref>(readPref)
  const [system, setSystem] = useState<Theme>(systemTheme)

  useEffect(() => {
    const mq = window.matchMedia?.('(prefers-color-scheme: light)')
    if (!mq) return
    const onChange = () => setSystem(mq.matches ? 'light' : 'dark')
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const theme: Theme = pref === 'system' ? system : pref

  useEffect(() => {
    const root = document.documentElement
    root.dataset.theme = theme
    root.style.colorScheme = theme
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', THEME_COLOR[theme])
  }, [theme])

  const setPref = useCallback((p: ThemePref) => {
    setPrefState(p)
    writeStored(STORAGE_KEYS.theme, p === 'system' ? null : p)
  }, [])

  const toggle = useCallback(() => setPref(theme === 'dark' ? 'light' : 'dark'), [theme, setPref])

  return { pref, theme, setPref, toggle }
}
