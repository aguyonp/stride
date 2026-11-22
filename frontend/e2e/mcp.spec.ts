import { expect, test } from '@playwright/test'
import { assertNoConsoleErrors, trackConsoleErrors } from './helpers'

// 4.3.0 admin MCP tab: usage of the public route POST /mcp/{token} and the tunable quota
// (mock endpoints /api/admin/mcp/usage + /api/admin/mcp/quota). Default mock state: signed in
// as the administrator (user 1), like the other admin screens.

test('shows the MCP usage and saves the quota from the admin tab', async ({ page }) => {
  const errors = trackConsoleErrors(page)

  await page.goto('/admin?tab=mcp')
  // The tab chip is marked as the current page.
  await expect(page.getByRole('button', { name: 'MCP', exact: true })).toHaveAttribute('aria-current', 'page')

  // Usage card: the 24 h totals and the per-token rows (mock users 1 and 2, 14 + id * 9 requests).
  // The per-token list is rendered twice (desktop table + mobile cards): scope the prefix
  // assertions to the table so the strict-mode locator stays unambiguous.
  await expect(page.getByRole('heading', { name: 'MCP usage' })).toBeVisible()
  const table = page.getByRole('table')
  await expect(table.getByText('sm4_m1ab')).toBeVisible()
  await expect(table.getByText('sm4_m2ab')).toBeVisible()
  await expect(page.getByText('429 · 24 h').first()).toBeVisible()

  // Quota card: the form is pre-filled with the stored defaults.
  await expect(page.getByRole('heading', { name: 'Route quota' })).toBeVisible()
  const nums = page.locator('input[type="number"]')
  await expect(nums).toHaveCount(4)
  await expect(nums.nth(0)).toHaveValue('100')
  await expect(nums.nth(1)).toHaveValue('10')

  // Saving a lower per-token limit: the PUT is accepted and the form follows the stored row.
  const save = page.getByRole('button', { name: 'Save' })
  await expect(save).toBeDisabled()
  await nums.nth(0).fill('5')
  await expect(save).toBeEnabled()
  await save.click()
  await expect(nums.nth(0)).toHaveValue('5')

  assertNoConsoleErrors(errors)
})
