import { expect, test, type Page } from '@playwright/test'
import { assertNoConsoleErrors, trackConsoleErrors } from './helpers'

// 4.8.0 release check (§22, issue #143): the banner above the content, its two actions, and the
// admin switch that silences it. The mock runs 4.7.0 with a 4.8.0 release published, and the
// notice it writes feeds the notification centre.

/** The unread badge of the bell, or null when every entry is read. */
async function unreadCount(page: Page): Promise<number | null> {
  const label = (await page.getByRole('button', { name: /^Notifications(, \d+ unread)?$/ }).getAttribute('aria-label')) ?? ''
  const m = /(\d+) unread/.exec(label)
  return m ? Number(m[1]) : null
}

/** « See on GitHub » opens a tab: record window.open instead of reaching GitHub from the runner. */
async function recordWindowOpen(page: Page) {
  await page.addInitScript(() => {
    ;(window as unknown as { open: (u: string) => null }).open = (u: string) => {
      ;(window as unknown as { __opened?: string }).__opened = u
      return null
    }
  })
}

const banner = (page: Page) => page.getByRole('region', { name: 'New version available' })

test('announces a newer release and opens the release page', async ({ page }) => {
  const errors = trackConsoleErrors(page)
  await recordWindowOpen(page)

  await page.goto('/')
  await expect(banner(page)).toBeVisible()
  await expect(banner(page).getByText('Stride 4.8.0 is published (you run 4.7.0).')).toBeVisible()

  // The notice exists in the centre (the badge caught up), and opening the release reads it.
  await expect.poll(() => unreadCount(page)).toBe(3)
  await banner(page).getByRole('button', { name: 'See on GitHub' }).click()
  expect(await page.evaluate(() => (window as unknown as { __opened?: string }).__opened)).toBe(
    'https://github.com/aguyonp/stride/releases/tag/4.8.0',
  )
  await expect.poll(() => unreadCount(page)).toBe(2)
  // Read means hidden, and it stays hidden after a reload.
  await expect(banner(page)).toHaveCount(0)
  await page.reload()
  await expect(banner(page)).toHaveCount(0)

  assertNoConsoleErrors(errors)
})

test('« Later » postpones the banner without reading the notice', async ({ page }) => {
  const errors = trackConsoleErrors(page)

  await page.goto('/')
  await expect(banner(page)).toBeVisible()
  await expect.poll(() => unreadCount(page)).toBe(3)

  await banner(page).getByRole('button', { name: 'Later' }).click()
  await expect(banner(page)).toHaveCount(0)
  // Nothing was read: the notice is still unread, and the history still lists it.
  expect(await unreadCount(page)).toBe(3)
  await page.getByRole('button', { name: /^Notifications/ }).click()
  await expect(page.getByRole('dialog', { name: 'Notifications' }).getByText('New version available')).toBeVisible()

  // Postponed for the visit; a later release would show up again.
  await page.reload()
  await expect(banner(page)).toHaveCount(0)

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
  await expect(banner(page)).toHaveCount(0)

  assertNoConsoleErrors(errors)
})
