import { expect, test } from '@playwright/test'
import { assertNoConsoleErrors, trackConsoleErrors } from './helpers'

/**
 * Issue #21: a session opened from the calendar is older than every loaded page, so the
 * previous/next arrows of the session header must still reach its neighbours.
 *
 * Mock scenario: the demo history holds a marathon on 2025-04-14, far beyond the first page
 * (and beyond the 82 synced days), which is exactly the reported situation.
 */
test('navigates to the neighbours of a session opened from the calendar', async ({ page }) => {
  const errors = trackConsoleErrors(page)

  await page.goto('/')
  await expect(page).toHaveTitle(/· Stride$/)

  // Open the picker and jump to the month of the marathon. (`dialog.sheet` is a transparent,
  // zero-sized shell around the panel: assert on its content, not on the dialog box. On a
  // desktop viewport the calendar and the recent list sit side by side, with no tabs.)
  await page.getByRole('button', { name: 'Change', exact: true }).click()
  const year = page.getByLabel('Year', { exact: true })
  await expect(year).toBeVisible()
  await year.selectOption('2025')
  await page.getByLabel('Month', { exact: true }).selectOption('3')

  const day = page.locator('[data-date="2025-04-14"]')
  await expect(day).toHaveAttribute('aria-label', /1 activity/)
  await day.click()
  await page.getByRole('button', { name: /Marathon/ }).click()

  // The session is selected from the calendar: it is not in the loaded pages.
  await expect(page).toHaveURL(/[?&]s=100\b/)
  await expect(page.getByRole('heading', { name: 'Marathon', level: 1 })).toBeVisible()

  // Nothing older in the demo history: the arrow stays disabled, as before.
  const prev = page.getByRole('button', { name: 'Previous session (older)' })
  const next = page.getByRole('button', { name: 'Next session (newer)' })
  await expect(prev).toBeDisabled()
  await expect(next).toBeEnabled()

  // Stepping forward reaches the next session of the history, not the top of the list.
  const before = page.url()
  await next.click()
  await expect(page).not.toHaveURL(before)
  await expect(page.getByRole('heading', { name: 'Marathon', level: 1 })).toHaveCount(0)
  await expect(page).toHaveURL(/[?&]s=\d+/)
  await expect(prev).toBeEnabled()

  // And back to the marathon.
  await prev.click()
  await expect(page).toHaveURL(/[?&]s=100\b/)

  assertNoConsoleErrors(errors)
})
