import { expect, test } from '@playwright/test'
import { assertNoConsoleErrors, trackConsoleErrors } from './helpers'

// The five menus of the header nav (desktop viewport: the nav lives in the header bar).
// The dashboard auto-selects the first session once the data loads, so its title is the
// session name — assert the stable “· Stride” suffix instead of an exact title.
const BASE = 'http://localhost:4173' // fixed in playwright.config.ts
const VIEWS: Array<[route: string, label: string, title: string | RegExp]> = [
  ['/plan', 'Planning', 'Planning · Stride'],
  ['/health', 'Health & fitness', 'Health & fitness · Stride'],
  ['/coach', 'AI Coach', 'AI Coach · Stride'],
  ['/settings', 'Settings', 'Settings · Stride'],
  ['/', 'Dashboard', /· Stride$/],
]

test('the five main views open and render', async ({ page }) => {
  const errors = trackConsoleErrors(page)
  const nav = page.locator('header nav[aria-label="Main navigation"]')

  await page.goto('/')
  await expect(page).toHaveTitle(/· Stride$/)

  for (const [route, label, title] of VIEWS) {
    await nav.getByRole('link', { name: label, exact: true }).click()
    await expect(page).toHaveURL(BASE + route)
    await expect(page).toHaveTitle(title)
    // The visited view is marked as the current page in the nav.
    await expect(nav.getByRole('link', { name: label, exact: true })).toHaveAttribute('aria-current', 'page')
  }

  assertNoConsoleErrors(errors)
})
