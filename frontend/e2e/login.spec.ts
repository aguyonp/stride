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
  await expect(page).toHaveTitle(/· Stride$/)
  await expect(page.getByText('demo', { exact: true })).toBeVisible()

  assertNoConsoleErrors(errors)
})

test('signs in with a recovery code after losing every device (4.4)', async ({ page }) => {
  const errors = trackConsoleErrors(page)

  await page.goto('/?mock=out')
  await expect(page).toHaveTitle('Sign in · Stride')

  // Seed one known code in the mock store (stands in for codes generated before the loss).
  await page.evaluate(() => {
    const raw = sessionStorage.getItem('stride.mock.v2')
    const s = JSON.parse(raw)
    s.users[0].recovery = [{ code: 'AAAA-BBBB-CCCC-DDDD', used: false }]
    sessionStorage.setItem('stride.mock.v2', JSON.stringify(s))
  })
  await page.reload()

  await page.getByRole('button', { name: 'Recovery code' }).click()
  await page.getByLabel('Recovery code').fill('aaaa-bbbb-cccc-dddd') // lowercase: input is normalized
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()

  // A session is issued like for a passkey: dashboard for the code owner (admin).
  await expect(page).toHaveTitle(/· Stride$/)
  await expect(page.getByText('demo', { exact: true })).toBeVisible()

  assertNoConsoleErrors(errors)
})
