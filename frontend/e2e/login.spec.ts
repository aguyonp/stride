import { expect, test } from '@playwright/test'
import { assertNoConsoleErrors, trackConsoleErrors } from './helpers'

// `?mock=out`: logged out, invite-only registration. The passkey ceremony is mocked
// (~600 ms, src/mocks/webauthn.ts) — no real WebAuthn in CI.

test('signs in with a passkey from the logged-out state', async ({ page }) => {
  const errors = trackConsoleErrors(page)

  await page.goto('/?mock=out')
  await expect(page).toHaveTitle('Sign in · Stride')

  const button = page.getByRole('button', { name: 'Sign in with a passkey' })
  await expect(button).toBeEnabled()
  await button.click()

  // The ceremony shows a spinner, then lands on the dashboard (admin, onboarding done).
  await expect(page).toHaveTitle('Dashboard · Stride')
  await expect(page.getByText('demo', { exact: true })).toBeVisible()

  assertNoConsoleErrors(errors)
})
