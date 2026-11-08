import '@fontsource-variable/inter'
import '@fontsource-variable/jetbrains-mono'
import './index.css'

import { QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { IS_MOCK } from './api/client'
import { queryClient } from './api/queries'
import App from './App'
import { ToastProvider } from './components/ui/Toast'
import { applyTheme } from './hooks/useTheme'
import { I18nBoundary } from './i18n/I18nBoundary'

applyTheme()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <I18nBoundary>
        <ToastProvider>
          <App />
        </ToastProvider>
      </I18nBoundary>
    </QueryClientProvider>
  </StrictMode>,
)

if (import.meta.env.PROD && !IS_MOCK && 'serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(() => undefined)
  })
}
