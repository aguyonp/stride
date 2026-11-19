import { defineConfig, devices } from '@playwright/test'

// Browser smoke tests (issue #91): the production build against the built-in mock backend
// (`npm run build:mock` → dist-mock, VITE_MOCK=1). The app makes zero network calls in that
// mode, so no Go backend, no secrets — hermetic and reproducible.
//
// Local: `npm run test:e2e` (builds dist-mock first, then runs Chromium headless).
export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    baseURL: 'http://localhost:4173',
    // i18n (src/i18n/lang.ts) picks the language from the browser locale: pin it so the
    // label assertions stay deterministic.
    locale: 'en-US',
    viewport: { width: 1440, height: 900 },
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    // Always rebuild so the smoke tests exercise the current code, not a stale build.
    command: 'npm run build:mock && npx vite preview --outDir dist-mock --port 4173 --strictPort',
    url: 'http://localhost:4173',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
})
