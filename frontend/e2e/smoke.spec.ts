import { expect, test } from '@playwright/test'
import { assertNoConsoleErrors, trackConsoleErrors } from './helpers'

// Default mock scenario: logged-in admin, onboarding done, Garmin + Strava connected.

test('boots to the dashboard with the demo data', async ({ page }) => {
  const errors = trackConsoleErrors(page)

  await page.goto('/')

  await expect(page).toHaveTitle('Dashboard · Stride')
  // The “demo” badge only exists in mock builds (IS_MOCK): proof the app runs on the mock
  // backend, not a live one.
  await expect(page.getByText('demo', { exact: true })).toBeVisible()
  // The header nav marks the dashboard as the current page.
  const nav = page.getByRole('navigation', { name: 'Main navigation' })
  await expect(nav.getByRole('link', { name: 'Dashboard', exact: true })).toHaveAttribute('aria-current', 'page')

  assertNoConsoleErrors(errors)
  // No error banner on a healthy boot.
  await expect(page.locator('[role="alert"]')).toHaveCount(0)
})
