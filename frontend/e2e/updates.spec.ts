import { expect, test } from '@playwright/test'
import { assertNoConsoleErrors, trackConsoleErrors } from './helpers'

// 4.8.0 release check (§22, issue #143): the banner above the content, its two actions, and the
// admin switch that silences it. The mock runs 4.7.0 with a 4.8.0 release published.

test('announces a newer release and postpones it', async ({ page }) => {
  const errors = trackConsoleErrors(page)
  // Deterministic « opens a tab »: record window.open instead of reaching GitHub from the test run.
  await page.addInitScript(() => {
    ;(window as unknown as { open: (u: string) => null }).open = (u: string) => {
      ;(window as unknown as { __opened?: string }).__opened = u
      return null
    }
  })

  await page.goto('/')
  const banner = page.getByRole('status').filter({ hasText: 'New version available' })
  await expect(banner).toBeVisible()
  await expect(banner.getByText('Stride 4.8.0 is published (you run 4.7.0).')).toBeVisible()

  // « See on GitHub » targets the constant repository URL, in a new tab.
  const link = banner.getByRole('button', { name: 'See on GitHub' })
  await expect(link).toBeVisible()
  await link.click()
  expect(await page.evaluate(() => (window as unknown as { __opened?: string }).__opened)).toBe(
    'https://github.com/aguyonp/stride/releases/tag/4.8.0',
  )

  // « Later » hides it without marking the notice read: the centre still has it unread.
  await page.goto('/')
  await page.getByRole('status').filter({ hasText: 'New version available' }).getByRole('button', { name: 'Later' }).click()
  await expect(page.getByRole('status').filter({ hasText: 'New version available' })).toHaveCount(0)
  await page.getByRole('button', { name: /Notifications/ }).click()
  const panel = page.getByRole('dialog', { name: 'Notifications' })
  await expect(panel.getByText('New version available')).toBeVisible()

  assertNoConsoleErrors(errors)
})

test('the admin switch silences the check', async ({ page }) => {
  const errors = trackConsoleErrors(page)

  await page.goto('/admin?tab=invitations')
  const toggle = page.getByRole('switch', { name: 'Check for new versions' })
  await expect(toggle).toHaveAttribute('aria-checked', 'true')
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-checked', 'false')
  await expect(page.getByText('No network call, no banner, no notification.')).toBeVisible()

  // Reload: the setting survived, and the banner is gone.
  await page.goto('/')
  await expect(page.getByRole('status').filter({ hasText: 'New version available' })).toHaveCount(0)

  assertNoConsoleErrors(errors)
})
