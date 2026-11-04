import '@fontsource-variable/inter'
import '@fontsource-variable/jetbrains-mono'
import './index.css'

import { QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { isMode } from './api/defaults'
import { queryClient } from './api/queries'
import App from './App'
import { ToastProvider } from './components/ui/Toast'
import { readStored, STORAGE_KEYS } from './lib/storage'

// Préférences appliquées avant le premier rendu (évite un flash de thème/couleur).
// Sans choix explicite, le CSS suit prefers-color-scheme (sombre par défaut).
const root = document.documentElement
const storedMode = readStored(STORAGE_KEYS.mode)
root.dataset.mode = isMode(storedMode) ? storedMode : 'sfp'
const storedTheme = readStored(STORAGE_KEYS.theme)
if (storedTheme === 'light' || storedTheme === 'dark') root.dataset.theme = storedTheme

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <App />
      </ToastProvider>
    </QueryClientProvider>
  </StrictMode>,
)
